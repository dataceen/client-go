// Package dataceen is the public Go client for the Dataceen GraphQL/gRPC API.
//
// Mirrors the surface of the C# DataceenClient (NuGet v1.1.33+) and the
// TypeScript port (DataceenClientTs). Phase 1 here covers the GraphQL HTTP
// runtime — typed per-entity methods are produced by the codegen in Phase 3.
package dataceen

import (
	"net/http"
	"time"
)

// Config mirrors the values under e.g. "AllClient" in the C# example's
// appsettings.json. All fields are required.
type Config struct {
	TenantID     string
	ClientID     string
	ClientSecret string

	Domain string
	Model  string
	Scope  string

	// APIURL is the base URL, e.g. "https://api.dataceen.com" (no trailing slash).
	APIURL string
	// APIPath is the path prefix, e.g. "/graphql". Domain/Model/Scope are
	// appended by the client to build the full endpoint.
	APIPath string
	// APIScope is the OAuth scope for the GraphQL endpoint, e.g. "api://.../.default".
	APIScope string

	// SubscriptionURL is the base URL for the gRPC subscription service.
	SubscriptionURL string
	// SubscriptionScope is the OAuth scope for the subscription service.
	// Distinct from APIScope.
	SubscriptionScope string
}

// clientOptions holds the resolved settings for a Client. Callers tune them
// via the With* functional options passed to NewClient.
type clientOptions struct {
	logger                 Logger
	httpClient             *http.Client
	httpTimeout            time.Duration
	maxTransientRetries    int
	transientRetryBase     time.Duration
	slowHandlerThreshold   time.Duration
	tokenProvider          TokenProvider // overridable for tests
}

func defaultOptions() clientOptions {
	return clientOptions{
		logger:               ConsoleLogger,
		httpClient:           http.DefaultClient,
		httpTimeout:          60 * time.Second,
		maxTransientRetries:  3,
		transientRetryBase:   200 * time.Millisecond,
		slowHandlerThreshold: 500 * time.Millisecond,
	}
}

// Option configures a Client at construction time.
type Option func(*clientOptions)

// WithLogger replaces the default ConsoleLogger.
func WithLogger(l Logger) Option {
	return func(o *clientOptions) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithHTTPClient overrides the *http.Client used for GraphQL requests.
// Useful for tests (httptest), custom transports, proxies, or instrumentation.
func WithHTTPClient(c *http.Client) Option {
	return func(o *clientOptions) {
		if c != nil {
			o.httpClient = c
		}
	}
}

// WithHTTPTimeout sets the per-request timeout. Default: 60s.
func WithHTTPTimeout(d time.Duration) Option {
	return func(o *clientOptions) {
		if d > 0 {
			o.httpTimeout = d
		}
	}
}

// WithMaxTransientRetries sets the cap for transient-error retries on queries.
// Mutations never retry. Default: 3.
func WithMaxTransientRetries(n int) Option {
	return func(o *clientOptions) {
		if n >= 0 {
			o.maxTransientRetries = n
		}
	}
}

// WithTransientRetryBase sets the base backoff between transient retries;
// each retry doubles. Default: 200ms.
func WithTransientRetryBase(d time.Duration) Option {
	return func(o *clientOptions) {
		if d > 0 {
			o.transientRetryBase = d
		}
	}
}

// WithSlowHandlerThreshold warns when a subscription handler exceeds this
// duration. Default: 500ms. Applies in Phase 4 (subscriptions) — accepted
// here so the option doesn't move later.
func WithSlowHandlerThreshold(d time.Duration) Option {
	return func(o *clientOptions) {
		if d > 0 {
			o.slowHandlerThreshold = d
		}
	}
}

// WithTokenProvider injects a custom TokenProvider. The default uses MSAL
// against the configured tenant. Tests use this to bypass MSAL entirely.
func WithTokenProvider(tp TokenProvider) Option {
	return func(o *clientOptions) {
		if tp != nil {
			o.tokenProvider = tp
		}
	}
}
