package subscriptions

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/dataceen/client-go/pkg/dataceenevent"
)

func TestReconnectDelay_DoublesFromTheBaseToItsCap(t *testing.T) {
	want := []time.Duration{1, 2, 4, 8, 16, 32, 60, 60, 60}
	for i, w := range want {
		if got := reconnectDelay(time.Second, 60*time.Second, i+1); got != w*time.Second {
			t.Errorf("failure %d: %s, want %s", i+1, got, w*time.Second)
		}
	}
	want = []time.Duration{1, 2, 4, 8, 16, 32, 64, 128, 256, 300, 300}
	for i, w := range want {
		if got := reconnectDelay(time.Second, 5*time.Minute, i+1); got != w*time.Second {
			t.Errorf("refused, failure %d: %s, want %s", i+1, got, w*time.Second)
		}
	}
	// A count far past the cap neither overflows nor leaves it.
	if got := reconnectDelay(time.Second, 60*time.Second, 5000); got != 60*time.Second {
		t.Errorf("failure 5000: %s, want 1m0s", got)
	}
	if got := reconnectDelay(5*time.Second, 60*time.Second, 0); got != 5*time.Second {
		t.Errorf("no failure yet: %s, want the base", got)
	}
}

func TestReconnectBackoff_StartsOverAfterAWorkingStream(t *testing.T) {
	b := reconnectBackoff{base: time.Second, max: 60 * time.Second, maxRefused: 5 * time.Minute}
	for i, want := range []time.Duration{1, 2, 4, 8} {
		if got := b.next(false, false); got != want*time.Second {
			t.Fatalf("failure %d in a row: %s, want %s", i+1, got, want*time.Second)
		}
	}
	// The stream before this failure delivered an event or stayed open a minute.
	if got := b.next(true, false); got != time.Second {
		t.Fatalf("after a working stream: %s, want the base", got)
	}
	if got := b.next(false, false); got != 2*time.Second {
		t.Fatalf("the failure after it: %s, want 2s", got)
	}
}

func TestReconnectBackoff_ARefusalWaitsUpToFiveMinutes(t *testing.T) {
	b := reconnectBackoff{base: time.Second, max: 60 * time.Second, maxRefused: 5 * time.Minute}
	var got time.Duration
	for i := 0; i < 8; i++ {
		got = b.next(false, true)
	}
	if got != 128*time.Second {
		t.Fatalf("the eighth refusal in a row: %s, want 2m8s", got)
	}
	// An ordinary failure in the same run keeps the ordinary cap.
	if got = b.next(false, false); got != 60*time.Second {
		t.Fatalf("an ordinary failure after eight refusals: %s, want 1m0s", got)
	}
	for i := 0; i < 5; i++ {
		got = b.next(false, true)
	}
	if got != 5*time.Minute {
		t.Fatalf("a long run of refusals: %s, want 5m0s", got)
	}
}

func TestIsRefused(t *testing.T) {
	for code, want := range map[codes.Code]bool{
		codes.Unauthenticated:  true,
		codes.PermissionDenied: true,
		codes.Unavailable:      false,
		codes.Internal:         false,
		codes.Unknown:          false,
	} {
		if got := isRefused(status.Error(code, "x")); got != want {
			t.Errorf("%s: refused = %v, want %v", code, got, want)
		}
	}
	if isRefused(errors.New("connection reset")) {
		t.Error("an error that is no status counted as a refusal")
	}
}

// scriptedDialer hands out one stream per connect, in order.
type scriptedDialer struct {
	mu      sync.Mutex
	streams []*fakeStream
	dials   int
}

func (d *scriptedDialer) dial(_, _ string) (func() error, streamClient, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.streams[d.dials]
	d.dials++
	return func() error { return nil }, &fakeStreamClient{stream: s}, nil
}

type recordingLogger struct {
	mu    sync.Mutex
	lines []string // "level: message"
}

func (l *recordingLogger) Log(level, message string, _ error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, level+": "+message)
}

func (l *recordingLogger) count(prefix string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, line := range l.lines {
		if strings.HasPrefix(line, prefix) {
			n++
		}
	}
	return n
}

type recordingTokens struct {
	mu     sync.Mutex
	forced []bool
}

func (r *recordingTokens) AcquireToken(_ context.Context, _ []string, force bool) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.forced = append(r.forced, force)
	return "fake-token", nil
}

func runScripted(t *testing.T, streams ...*fakeStream) (*recordingLogger, *recordingTokens) {
	t.Helper()
	d := &scriptedDialer{streams: streams}
	logger, tokens := &recordingLogger{}, &recordingTokens{}
	err := Run(context.Background(), newSub(t), RunOptions{
		Tokens:        tokens,
		Logger:        logger,
		ReconnectBase: time.Millisecond,
		dialer:        d.dial,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if d.dials != len(streams) {
		t.Fatalf("%d connect(s), want %d", d.dials, len(streams))
	}
	return logger, tokens
}

// Production, 2026-11-13: an application whose registration lacked the scope's
// role was refused on every connect and wrote one warning per retry. The
// refusal is said once; the retries that meet it again are debug lines.
func TestManager_ARefusalIsSaidOnce(t *testing.T) {
	denied := func() *fakeStream {
		return &fakeStream{err: status.Error(codes.PermissionDenied, "unexpected HTTP status code received from server: 403 (Forbidden)")}
	}
	served := &fakeStream{events: []*pb.EventData{mkEvent(pb.EventType_SUBSCRIPTION_EVENT, "Customer", "c1", `{}`)}}

	logger, tokens := runScripted(t, denied(), denied(), denied(), denied(), served)

	if n := logger.count("error: "); n != 1 {
		t.Fatalf("%d error line(s) for four refusals, want 1: %v", n, logger.lines)
	}
	said := logger.lines[0]
	for _, part := range []string{"was refused", "lacks the role of scope Thomas.CandyShopModel.All for live updates"} {
		if !strings.Contains(said, part) {
			t.Errorf("the refusal line does not say %q: %s", part, said)
		}
	}
	if n := logger.count("debug: "); n != 3 {
		t.Errorf("%d debug line(s) for the three refusals after the first, want 3: %v", n, logger.lines)
	}
	if n := logger.count("warn: "); n != 0 {
		t.Errorf("%d warning(s) for a refusal, want 0: %v", n, logger.lines)
	}
	// The role is not in the token; asking for another one changes nothing.
	for i, forced := range tokens.forced {
		if forced {
			t.Errorf("connect %d forced a new token after PERMISSION_DENIED", i+1)
		}
	}
}

// A token the service does not accept is fetched anew once, for the connect
// after the first refusal, and not for the retries after that.
func TestManager_AnUnacceptedTokenIsFetchedAnewOnce(t *testing.T) {
	unauthenticated := func() *fakeStream {
		return &fakeStream{err: status.Error(codes.Unauthenticated, "unexpected HTTP status code received from server: 401 (Unauthorized)")}
	}
	served := &fakeStream{events: []*pb.EventData{mkEvent(pb.EventType_SUBSCRIPTION_EVENT, "Customer", "c1", `{}`)}}

	logger, tokens := runScripted(t, unauthenticated(), unauthenticated(), unauthenticated(), served)

	want := []bool{false, true, false, false}
	if len(tokens.forced) != len(want) {
		t.Fatalf("%d token request(s), want %d", len(tokens.forced), len(want))
	}
	for i := range want {
		if tokens.forced[i] != want[i] {
			t.Errorf("connect %d: forced = %v, want %v", i+1, tokens.forced[i], want[i])
		}
	}
	if n := logger.count("error: "); n != 1 {
		t.Errorf("%d error line(s) for three refusals, want 1: %v", n, logger.lines)
	}
}

// A refusal after a stream that worked is a new one and is said again; an
// ordinary failure keeps its warning per attempt.
func TestManager_ARefusalAfterAWorkingStreamIsSaidAgain(t *testing.T) {
	denied := func() *fakeStream { return &fakeStream{err: status.Error(codes.PermissionDenied, "Denied.")} }
	servedThenLost := &fakeStream{
		events: []*pb.EventData{mkEvent(pb.EventType_SUBSCRIPTION_EVENT, "Customer", "c1", `{}`)},
		err:    status.Error(codes.Unavailable, "connection reset"),
	}
	served := &fakeStream{events: []*pb.EventData{mkEvent(pb.EventType_SUBSCRIPTION_EVENT, "Customer", "c2", `{}`)}}

	logger, _ := runScripted(t, denied(), servedThenLost, denied(), denied(), served)

	if n := logger.count("error: "); n != 2 {
		t.Errorf("%d error line(s) for two separate refusals, want 2: %v", n, logger.lines)
	}
	if n := logger.count("warn: "); n != 1 {
		t.Errorf("%d warning(s) for the one lost connection, want 1: %v", n, logger.lines)
	}
	if n := logger.count("debug: "); n != 1 {
		t.Errorf("%d debug line(s), want 1: %v", n, logger.lines)
	}
}
