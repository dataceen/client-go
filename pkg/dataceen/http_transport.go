package dataceen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// httpTransport ports the resilience logic from C# DataceenClient.PostGraphQLAsync:
//   - one forced token refresh on HTTP 401, retry once
//   - optional transient retry (network errors + 408/429/5xx) with exponential
//     backoff; never used for mutations.
type httpTransport struct {
	endpoint            string
	apiScope            string
	httpClient          *http.Client
	httpTimeout         time.Duration
	maxTransientRetries int
	transientRetryBase  time.Duration
	logger              Logger
	tokens              TokenProvider
}

func newHTTPTransport(cfg Config, opts clientOptions) *httpTransport {
	apiPath := fmt.Sprintf("%s/%s/%s/%s", cfg.APIPath, cfg.Domain, cfg.Model, cfg.Scope)
	return &httpTransport{
		endpoint:            cfg.APIURL + apiPath,
		apiScope:            cfg.APIScope,
		httpClient:          opts.httpClient,
		httpTimeout:         opts.httpTimeout,
		maxTransientRetries: opts.maxTransientRetries,
		transientRetryBase:  opts.transientRetryBase,
		logger:              opts.logger,
		tokens:              opts.tokenProvider,
	}
}

// postGraphQL sends req and returns the raw response body. It implements:
//   - one forced 401-refresh
//   - optional transient retry (only when retryOnTransient=true)
//   - exponential backoff (base*2^n)
//   - context propagation (cancellation aborts everything in flight)
func (t *httpTransport) postGraphQL(ctx context.Context, req GraphQL, retryOnTransient bool) ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, newError(fmt.Sprintf("marshal GraphQL request: %v", err), -1, req.Query, err)
	}

	maxTransient := 0
	if retryOnTransient {
		maxTransient = t.maxTransientRetries
	}

	transientAttempt := 0
	triedAuthRefresh := false
	delay := t.transientRetryBase

	for {
		token, terr := t.tokens.AcquireToken(ctx, []string{t.apiScope}, triedAuthRefresh)
		if terr != nil {
			return nil, newError("acquire token: "+terr.Error(), -1, req.Query, terr)
		}

		resp, doErr := t.do(ctx, body, token)
		if doErr != nil {
			// Context errors are never retried — caller asked us to stop.
			if errors.Is(doErr, context.Canceled) || errors.Is(doErr, context.DeadlineExceeded) {
				return nil, doErr
			}
			if transientAttempt < maxTransient {
				transientAttempt++
				t.logger.Log(LevelWarn, fmt.Sprintf(
					"Transient HTTP error (attempt %d/%d): %v. Retrying in %s.",
					transientAttempt, maxTransient, doErr, delay), nil)
				if err := sleepCtx(ctx, delay); err != nil {
					return nil, err
				}
				delay *= 2
				continue
			}
			return nil, newError("HTTP transport: "+doErr.Error(), -1, req.Query, doErr)
		}

		if resp.StatusCode == http.StatusUnauthorized && !triedAuthRefresh {
			triedAuthRefresh = true
			t.logger.Log(LevelWarn, "401 Unauthorized — forcing token refresh and retrying once.", nil)
			drain(resp)
			continue
		}

		if retryOnTransient && !ok(resp.StatusCode) && isTransientStatus(resp.StatusCode) && transientAttempt < maxTransient {
			transientAttempt++
			t.logger.Log(LevelWarn, fmt.Sprintf(
				"Transient HTTP status %d (attempt %d/%d). Retrying in %s.",
				resp.StatusCode, transientAttempt, maxTransient, delay), nil)
			drain(resp)
			if err := sleepCtx(ctx, delay); err != nil {
				return nil, err
			}
			delay *= 2
			continue
		}

		respBody, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, newError("read response: "+readErr.Error(), -1, req.Query, readErr)
		}
		if closeErr != nil {
			// Non-fatal but worth logging.
			t.logger.Log(LevelWarn, "close response body: "+closeErr.Error(), nil)
		}

		if !ok(resp.StatusCode) {
			preview := string(respBody)
			if len(preview) > 500 {
				preview = preview[:500]
			}
			return nil, newError(
				fmt.Sprintf("HTTP %d: %s", resp.StatusCode, preview),
				-resp.StatusCode,
				req.Query,
				nil,
			)
		}

		return respBody, nil
	}
}

func (t *httpTransport) do(ctx context.Context, body []byte, token string) (*http.Response, error) {
	reqCtx := ctx
	if t.httpTimeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, t.httpTimeout)
		// The timeout context must outlive the http call but be canceled
		// once the response is consumed. Wrap in a body-closer.
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, t.endpoint, bytes.NewReader(body))
		if err != nil {
			cancel()
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := t.httpClient.Do(req)
		if err != nil {
			cancel()
			return nil, err
		}
		resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
		return resp, nil
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	return t.httpClient.Do(req)
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func ok(status int) bool {
	return status >= 200 && status < 300
}

func isTransientStatus(status int) bool {
	return status == 408 || status == 429 || status >= 500
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
