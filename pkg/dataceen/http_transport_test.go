package dataceen

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeTokenProvider hands out "cached-token" or "fresh-token" depending on
// the forceRefresh flag, and counts how many times forceRefresh was set.
type fakeTokenProvider struct {
	refreshCount int32
	failNext     atomic.Bool
}

func (f *fakeTokenProvider) AcquireToken(_ context.Context, _ []string, forceRefresh bool) (string, error) {
	if f.failNext.Load() {
		f.failNext.Store(false)
		return "", errors.New("simulated MSAL failure")
	}
	if forceRefresh {
		atomic.AddInt32(&f.refreshCount, 1)
		return "fresh-token", nil
	}
	return "cached-token", nil
}

// scriptedHandler returns one response per call from a scripted list.
type scriptedHandler struct {
	t       *testing.T
	steps   []func(http.ResponseWriter, *http.Request)
	calls   int32
	headers []http.Header
}

func (s *scriptedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	idx := atomic.AddInt32(&s.calls, 1) - 1
	if int(idx) >= len(s.steps) {
		s.t.Fatalf("scripted handler ran out of steps at call %d", idx+1)
	}
	// Snapshot headers for assertions.
	hcopy := http.Header{}
	for k, v := range r.Header {
		hcopy[k] = append([]string{}, v...)
	}
	s.headers = append(s.headers, hcopy)
	s.steps[idx](w, r)
}

func newTransport(t *testing.T, server *httptest.Server, tokens TokenProvider, retries int) *httpTransport {
	t.Helper()
	cfg := Config{
		APIURL:   server.URL,
		APIPath:  "/graphql",
		Domain:   "Domain",
		Model:    "Model",
		Scope:    "Scope",
		APIScope: "api://test/.default",
	}
	opts := defaultOptions()
	opts.logger = SilentLogger
	opts.maxTransientRetries = retries
	opts.transientRetryBase = 1 * time.Millisecond
	opts.tokenProvider = tokens
	opts.httpClient = server.Client()
	return newHTTPTransport(cfg, opts)
}

var sampleReq = GraphQL{
	OperationName: "FindCustomer",
	Query:         "query { FindCustomer { Items { _id } } }",
}

func TestPostGraphQL_FirstTrySuccess(t *testing.T) {
	tokens := &fakeTokenProvider{}
	h := &scriptedHandler{t: t, steps: []func(http.ResponseWriter, *http.Request){
		func(w http.ResponseWriter, _ *http.Request) {
			io.WriteString(w, "OK-BODY")
		},
	}}
	srv := httptest.NewServer(h)
	defer srv.Close()
	tr := newTransport(t, srv, tokens, 3)

	body, err := tr.postGraphQL(context.Background(), sampleReq, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != "OK-BODY" {
		t.Fatalf("body = %q, want %q", body, "OK-BODY")
	}
	if got := atomic.LoadInt32(&h.calls); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&tokens.refreshCount); got != 0 {
		t.Fatalf("refreshCount = %d, want 0", got)
	}
}

func TestPostGraphQL_RefreshOn401(t *testing.T) {
	tokens := &fakeTokenProvider{}
	h := &scriptedHandler{t: t, steps: []func(http.ResponseWriter, *http.Request){
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(401) },
		func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "OK") },
	}}
	srv := httptest.NewServer(h)
	defer srv.Close()
	tr := newTransport(t, srv, tokens, 3)

	body, err := tr.postGraphQL(context.Background(), sampleReq, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != "OK" {
		t.Fatalf("body = %q, want OK", body)
	}
	if got := atomic.LoadInt32(&h.calls); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
	if got := atomic.LoadInt32(&tokens.refreshCount); got != 1 {
		t.Fatalf("refreshCount = %d, want 1", got)
	}
	if got := h.headers[1].Get("Authorization"); got != "Bearer fresh-token" {
		t.Fatalf("auth header on retry = %q, want Bearer fresh-token", got)
	}
}

func TestPostGraphQL_DoubleAuth401(t *testing.T) {
	tokens := &fakeTokenProvider{}
	h := &scriptedHandler{t: t, steps: []func(http.ResponseWriter, *http.Request){
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(401) },
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(401)
			io.WriteString(w, "still unauthorized")
		},
	}}
	srv := httptest.NewServer(h)
	defer srv.Close()
	tr := newTransport(t, srv, tokens, 3)

	_, err := tr.postGraphQL(context.Background(), sampleReq, true)
	if err == nil {
		t.Fatal("expected error")
	}
	var de *Error
	if !errors.As(err, &de) {
		t.Fatalf("err type = %T, want *Error", err)
	}
	if got := atomic.LoadInt32(&h.calls); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
	if got := atomic.LoadInt32(&tokens.refreshCount); got != 1 {
		t.Fatalf("refreshCount = %d, want 1", got)
	}
}

func TestPostGraphQL_Retry5xxUntilSuccess(t *testing.T) {
	tokens := &fakeTokenProvider{}
	h := &scriptedHandler{t: t, steps: []func(http.ResponseWriter, *http.Request){
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) },
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) },
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) },
		func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "FINAL") },
	}}
	srv := httptest.NewServer(h)
	defer srv.Close()
	tr := newTransport(t, srv, tokens, 3)

	body, err := tr.postGraphQL(context.Background(), sampleReq, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != "FINAL" {
		t.Fatalf("body = %q, want FINAL", body)
	}
	if got := atomic.LoadInt32(&h.calls); got != 4 {
		t.Fatalf("calls = %d, want 4", got)
	}
}

func TestPostGraphQL_GivesUpAfterMaxTransient(t *testing.T) {
	tokens := &fakeTokenProvider{}
	h := &scriptedHandler{t: t, steps: []func(http.ResponseWriter, *http.Request){
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(503)
			io.WriteString(w, "boom")
		},
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(503)
			io.WriteString(w, "boom")
		},
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(503)
			io.WriteString(w, "boom")
		},
	}}
	srv := httptest.NewServer(h)
	defer srv.Close()
	tr := newTransport(t, srv, tokens, 2)

	_, err := tr.postGraphQL(context.Background(), sampleReq, true)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("err = %v, want substring HTTP 503", err)
	}
	if got := atomic.LoadInt32(&h.calls); got != 3 {
		t.Fatalf("calls = %d, want 3 (initial + 2 retries)", got)
	}
}

func TestPostGraphQL_NoRetry5xxOnMutation(t *testing.T) {
	tokens := &fakeTokenProvider{}
	h := &scriptedHandler{t: t, steps: []func(http.ResponseWriter, *http.Request){
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(503)
			io.WriteString(w, "boom")
		},
	}}
	srv := httptest.NewServer(h)
	defer srv.Close()
	tr := newTransport(t, srv, tokens, 3)

	_, err := tr.postGraphQL(context.Background(), sampleReq, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("err = %v, want HTTP 503", err)
	}
	if got := atomic.LoadInt32(&h.calls); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
}

func TestPostGraphQL_NoRetry4xxOtherThan401(t *testing.T) {
	tokens := &fakeTokenProvider{}
	h := &scriptedHandler{t: t, steps: []func(http.ResponseWriter, *http.Request){
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(400)
			io.WriteString(w, "bad")
		},
	}}
	srv := httptest.NewServer(h)
	defer srv.Close()
	tr := newTransport(t, srv, tokens, 3)

	_, err := tr.postGraphQL(context.Background(), sampleReq, true)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("err = %v, want HTTP 400", err)
	}
	if got := atomic.LoadInt32(&h.calls); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
}

func TestPostGraphQL_AuthHeader(t *testing.T) {
	tokens := &fakeTokenProvider{}
	h := &scriptedHandler{t: t, steps: []func(http.ResponseWriter, *http.Request){
		func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "{}") },
	}}
	srv := httptest.NewServer(h)
	defer srv.Close()
	tr := newTransport(t, srv, tokens, 3)

	if _, err := tr.postGraphQL(context.Background(), sampleReq, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := h.headers[0].Get("Authorization"); got != "Bearer cached-token" {
		t.Fatalf("Authorization = %q, want Bearer cached-token", got)
	}
	if got := h.headers[0].Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func TestPostGraphQL_TokenAcquireError(t *testing.T) {
	tokens := &fakeTokenProvider{}
	tokens.failNext.Store(true)
	h := &scriptedHandler{t: t, steps: []func(http.ResponseWriter, *http.Request){
		// no requests should reach here
	}}
	srv := httptest.NewServer(h)
	defer srv.Close()
	tr := newTransport(t, srv, tokens, 3)

	_, err := tr.postGraphQL(context.Background(), sampleReq, true)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "acquire token") {
		t.Fatalf("err = %v, want acquire-token failure", err)
	}
	if got := atomic.LoadInt32(&h.calls); got != 0 {
		t.Fatalf("calls = %d, want 0", got)
	}
}
