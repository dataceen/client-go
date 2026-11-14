package dataceen

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeDirectory answers MSAL as the directory does: the tenant's metadata, and
// a new token for every call to the token endpoint, which it counts. An answer
// takes a moment, so callers that start together overlap in it. Nothing leaves
// the machine.
type fakeDirectory struct {
	tokenCalls atomic.Int64
	failNext   atomic.Bool
}

func (f *fakeDirectory) CloseIdleConnections() {}

func (f *fakeDirectory) Do(req *http.Request) (*http.Response, error) {
	const base = "https://login.microsoftonline.com/22222222-2222-2222-2222-222222222222"
	code, body := http.StatusNotFound, "{}"
	switch {
	case strings.HasSuffix(req.URL.Path, "/token"):
		n := f.tokenCalls.Add(1)
		time.Sleep(100 * time.Millisecond)
		if f.failNext.Swap(false) {
			code, body = http.StatusBadRequest, `{"error":"invalid_client","error_description":"the fake refuses this one"}`
			break
		}
		code = http.StatusOK
		body = fmt.Sprintf(`{"token_type":"Bearer","expires_in":3600,"ext_expires_in":3600,"access_token":"fake-%d"}`, n)
	case strings.HasSuffix(req.URL.Path, "/openid-configuration"):
		code = http.StatusOK
		body = fmt.Sprintf(`{"authorization_endpoint":"%s/oauth2/v2.0/authorize","token_endpoint":"%s/oauth2/v2.0/token","issuer":"%s/v2.0"}`, base, base, base)
	}
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

// testClock is the provider's clock in a test, moved by hand.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newFakedProvider(t *testing.T) (*msalTokenProvider, *fakeDirectory, *testClock) {
	t.Helper()
	dir := &fakeDirectory{}
	tp, err := newMSALTokenProvider(Config{
		TenantID:     "22222222-2222-2222-2222-222222222222",
		ClientID:     "11111111-1111-1111-1111-111111111111",
		ClientSecret: "not-a-secret",
	}, dir)
	if err != nil {
		t.Fatalf("newMSALTokenProvider: %v", err)
	}
	clock := &testClock{t: time.Now()}
	tp.now = clock.Now
	return tp, dir, clock
}

// burst asks for a token from n goroutines released together and answers how
// many times the token endpoint was called, the distinct tokens and the errors.
func burst(tp TokenProvider, dir *fakeDirectory, n int, scopes []string, force bool) (calls int64, tokens map[string]bool, errs []error) {
	before := dir.tokenCalls.Load()
	gate := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	tokens = map[string]bool{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			tok, err := tp.AcquireToken(context.Background(), scopes, force)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			tokens[tok] = true
		}()
	}
	close(gate)
	wg.Wait()
	return dir.tokenCalls.Load() - before, tokens, errs
}

// Production, 2026-11-13: an application fetched five to thirty tokens an hour
// where one was enough. This provider asked the directory on every call: MSAL
// for Go's AcquireTokenByCredential does not read its cache, and each GraphQL
// request and each subscription connect calls AcquireToken. Twenty callers at
// once were twenty token calls, cold or warm.
func TestMSALTokenProvider_TwentyCallersAtOnceAreOneTokenCall(t *testing.T) {
	tp, dir, clock := newFakedProvider(t)
	scopes := []string{"api://example/.default"}

	calls, tokens, errs := burst(tp, dir, 20, scopes, false)
	if len(errs) > 0 {
		t.Fatalf("cold cache: %v", errs[0])
	}
	if calls != 1 || len(tokens) != 1 {
		t.Fatalf("cold cache: %d token call(s) and %d distinct token(s) for 20 callers, want 1 and 1", calls, len(tokens))
	}

	calls, _, errs = burst(tp, dir, 20, scopes, false)
	if len(errs) > 0 {
		t.Fatalf("warm cache: %v", errs[0])
	}
	if calls != 0 {
		t.Fatalf("warm cache: %d token call(s) for 20 callers, want 0", calls)
	}

	// The fake's tokens live an hour and are renewed five minutes before their end.
	clock.Advance(54 * time.Minute)
	if calls, _, _ = burst(tp, dir, 20, scopes, false); calls != 0 {
		t.Fatalf("six minutes before the end: %d token call(s), want 0", calls)
	}
	clock.Advance(2 * time.Minute)
	calls, tokens, errs = burst(tp, dir, 20, scopes, false)
	if len(errs) > 0 {
		t.Fatalf("after expiry: %v", errs[0])
	}
	if calls != 1 || len(tokens) != 1 {
		t.Fatalf("after expiry: %d token call(s) and %d distinct token(s) for 20 callers, want 1 and 1", calls, len(tokens))
	}
	if tokens["fake-1"] {
		t.Fatalf("after expiry the callers got the expired token again")
	}
}

// A forced refresh (the retry after a 401) asks the directory although a token
// is cached, and callers forcing at the same moment share that one answer.
func TestMSALTokenProvider_ForcedRefreshFetchesAndIsShared(t *testing.T) {
	tp, dir, _ := newFakedProvider(t)
	scopes := []string{"api://example/.default"}
	ctx := context.Background()

	first, err := tp.AcquireToken(ctx, scopes, false)
	if err != nil {
		t.Fatalf("first token: %v", err)
	}
	forced, err := tp.AcquireToken(ctx, scopes, true)
	if err != nil {
		t.Fatalf("forced token: %v", err)
	}
	if forced == first || dir.tokenCalls.Load() != 2 {
		t.Fatalf("a forced refresh answered %q after %q with %d token call(s), want a new token and 2", forced, first, dir.tokenCalls.Load())
	}
	if again, _ := tp.AcquireToken(ctx, scopes, false); again != forced {
		t.Fatalf("after a forced refresh the cache answered %q, want %q", again, forced)
	}

	calls, tokens, errs := burst(tp, dir, 20, scopes, true)
	if len(errs) > 0 {
		t.Fatalf("forced burst: %v", errs[0])
	}
	if calls != 1 || len(tokens) != 1 {
		t.Fatalf("20 forced refreshes at once: %d token call(s) and %d distinct token(s), want 1 and 1", calls, len(tokens))
	}
}

// A refused fetch is the answer for the callers who waited on it and for
// nobody after them: the next call asks again.
func TestMSALTokenProvider_FailedFetchIsNotKept(t *testing.T) {
	tp, dir, _ := newFakedProvider(t)
	scopes := []string{"api://example/.default"}

	dir.failNext.Store(true)
	calls, tokens, errs := burst(tp, dir, 20, scopes, false)
	if calls != 1 || len(errs) != 20 || len(tokens) != 0 {
		t.Fatalf("a refused fetch: %d token call(s), %d error(s), %d token(s) for 20 callers, want 1, 20 and 0", calls, len(errs), len(tokens))
	}

	calls, tokens, errs = burst(tp, dir, 20, scopes, false)
	if len(errs) > 0 {
		t.Fatalf("after the refusal: %v", errs[0])
	}
	if calls != 1 || len(tokens) != 1 {
		t.Fatalf("after the refusal: %d token call(s) and %d distinct token(s), want 1 and 1", calls, len(tokens))
	}
}

// The API's scope and the subscription service's scope are two tokens, each
// fetched once and neither handed out for the other.
func TestMSALTokenProvider_EachScopeSetHasItsOwnToken(t *testing.T) {
	tp, dir, _ := newFakedProvider(t)
	ctx := context.Background()

	api, err := tp.AcquireToken(ctx, []string{"api://api/.default"}, false)
	if err != nil {
		t.Fatalf("api token: %v", err)
	}
	sub, err := tp.AcquireToken(ctx, []string{"api://subscription/.default"}, false)
	if err != nil {
		t.Fatalf("subscription token: %v", err)
	}
	if api == sub {
		t.Fatalf("two scopes answered one token %q", api)
	}
	apiAgain, _ := tp.AcquireToken(ctx, []string{"api://api/.default"}, false)
	subAgain, _ := tp.AcquireToken(ctx, []string{"api://subscription/.default"}, false)
	if apiAgain != api || subAgain != sub || dir.tokenCalls.Load() != 2 {
		t.Fatalf("second round: %q/%q after %q/%q with %d token call(s), want the same two and 2", apiAgain, subAgain, api, sub, dir.tokenCalls.Load())
	}
}
