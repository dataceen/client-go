package dataceen

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"
)

// TokenProvider abstracts Azure AD token acquisition. The default
// implementation uses MSAL with the client-credentials flow; tests inject
// fakes via WithTokenProvider.
//
// AcquireToken receives the OAuth scopes plus a forceRefresh flag that
// bypasses the cache (mirrors C#'s AcquireTokenForClient
// .WithForceRefresh(true)).
//
// The default implementation keeps each token until five minutes before its
// end and asks the directory once per scope set however many callers arrive
// together: MSAL for Go's AcquireTokenByCredential goes to the directory on
// every call, and every request and every subscription connect asks here.
// A forced refresh always asks the directory; callers forcing at the same
// moment share the one answer.
type TokenProvider interface {
	AcquireToken(ctx context.Context, scopes []string, forceRefresh bool) (string, error)
}

// tokenRefreshBefore is how long before its end a token stops being handed
// out. Five minutes is MSAL's own margin in the other clients.
const tokenRefreshBefore = 5 * time.Minute

// tokenHTTPClient is what MSAL asks of the HTTP client it is given. Tests pass
// a fake that answers as the directory does; nil means MSAL's own client.
type tokenHTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
	CloseIdleConnections()
}

type cachedToken struct {
	value     string
	expiresOn time.Time
}

// tokenFlight is one call to the directory and the callers waiting on it.
// token, err and abandoned are written before done is closed and read after.
type tokenFlight struct {
	done  chan struct{}
	token string
	err   error
	// abandoned: the caller who went to the directory gave up (its own
	// context ended) before an answer came. That is no answer for the
	// others; they ask again.
	abandoned bool
}

type msalTokenProvider struct {
	authority    string
	clientID     string
	clientSecret string
	httpClient   tokenHTTPClient
	now          func() time.Time

	// mu guards app, cache and flights. It is never held across a call to
	// the directory.
	mu      sync.Mutex
	app     confidential.Client
	cache   map[string]cachedToken
	flights map[string]*tokenFlight
}

// NewMSALTokenProvider builds a TokenProvider backed by the Microsoft
// Authentication Library for Go (client-credentials flow). It is the default
// when constructing a Client without WithTokenProvider.
func NewMSALTokenProvider(cfg Config) (TokenProvider, error) {
	return newMSALTokenProvider(cfg, nil)
}

func newMSALTokenProvider(cfg Config, httpClient tokenHTTPClient) (*msalTokenProvider, error) {
	if cfg.TenantID == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("dataceen: TenantID, ClientID, ClientSecret are required for MSAL token provider")
	}
	authority := "https://login.microsoftonline.com/" + cfg.TenantID
	tp := &msalTokenProvider{
		authority:    authority,
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		httpClient:   httpClient,
		now:          time.Now,
		cache:        map[string]cachedToken{},
		flights:      map[string]*tokenFlight{},
	}
	if err := tp.rebuild(); err != nil {
		return nil, err
	}
	return tp, nil
}

func (m *msalTokenProvider) rebuild() error {
	cred, err := confidential.NewCredFromSecret(m.clientSecret)
	if err != nil {
		return fmt.Errorf("dataceen: build client-secret cred: %w", err)
	}
	var opts []confidential.Option
	if m.httpClient != nil {
		opts = append(opts, confidential.WithHTTPClient(m.httpClient))
	}
	app, err := confidential.New(m.authority, m.clientID, cred, opts...)
	if err != nil {
		return fmt.Errorf("dataceen: confidential.New: %w", err)
	}
	m.app = app
	return nil
}

func (m *msalTokenProvider) AcquireToken(ctx context.Context, scopes []string, forceRefresh bool) (string, error) {
	key := scopeKey(scopes)
	for {
		m.mu.Lock()
		if !forceRefresh {
			if t, ok := m.cache[key]; ok && m.now().Before(t.expiresOn.Add(-tokenRefreshBefore)) {
				m.mu.Unlock()
				return t.value, nil
			}
		}
		// A call to the directory already on its way answers with a token
		// newer than any this caller has seen, forced or not: wait for it.
		flight, underway := m.flights[key]
		if !underway {
			flight = &tokenFlight{done: make(chan struct{})}
			m.flights[key] = flight
		}
		m.mu.Unlock()

		if !underway {
			return m.fetch(ctx, key, scopes, forceRefresh, flight)
		}
		select {
		case <-flight.done:
		case <-ctx.Done():
			return "", fmt.Errorf("dataceen: waiting for the token: %w", ctx.Err())
		}
		if flight.abandoned {
			continue
		}
		return flight.token, flight.err
	}
}

// fetch asks the directory on behalf of every caller waiting on flight. The
// flight is taken down whatever happens, so a failure is the answer for the
// callers who waited on it and the next call asks again.
func (m *msalTokenProvider) fetch(ctx context.Context, key string, scopes []string, forceRefresh bool, flight *tokenFlight) (string, error) {
	flight.abandoned = true
	defer func() {
		m.mu.Lock()
		delete(m.flights, key)
		m.mu.Unlock()
		close(flight.done)
	}()
	answer := func(token string, err error) (string, error) {
		flight.token, flight.err, flight.abandoned = token, err, false
		return token, err
	}

	m.mu.Lock()
	if forceRefresh {
		// MSAL for Go has no force-refresh option. A new client has an empty
		// cache, which matters for the MSAL versions that read theirs.
		if err := m.rebuild(); err != nil {
			m.mu.Unlock()
			return answer("", err)
		}
	}
	app := m.app
	m.mu.Unlock()

	res, err := app.AcquireTokenByCredential(ctx, scopes)
	if err != nil {
		err = fmt.Errorf("dataceen: MSAL AcquireTokenByCredential: %w", err)
		if ctx.Err() != nil {
			return "", err // this caller gave up; the flight stays abandoned
		}
		return answer("", err)
	}
	if res.AccessToken == "" {
		return answer("", errors.New("dataceen: MSAL returned empty access token"))
	}

	m.mu.Lock()
	m.cache[key] = cachedToken{value: res.AccessToken, expiresOn: res.ExpiresOn}
	m.mu.Unlock()
	return answer(res.AccessToken, nil)
}

// scopeKey names a scope set whatever order the scopes are given in.
func scopeKey(scopes []string) string {
	sorted := append([]string(nil), scopes...)
	sort.Strings(sorted)
	return strings.Join(sorted, " ")
}
