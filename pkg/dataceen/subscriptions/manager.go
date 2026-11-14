package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/dataceen/client-go/pkg/dataceenevent"
)

// Logger is the minimal interface the manager needs. Mirrors
// dataceen.Logger so adapters can pass it directly.
type Logger interface {
	Log(level string, message string, err error)
}

// silentLogger discards all log entries.
type silentLogger struct{}

func (silentLogger) Log(string, string, error) {}

// TokenAcquirer abstracts MSAL token acquisition. The dataceen.TokenProvider
// interface satisfies it via a small adapter (see Client.Subscribe in
// dataceen/client.go).
type TokenAcquirer interface {
	AcquireToken(ctx context.Context, scopes []string, forceRefresh bool) (string, error)
}

// RunOptions tunes the subscription manager.
type RunOptions struct {
	SubscriptionURL      string
	SubscriptionScope    string
	Tokens               TokenAcquirer
	Logger               Logger
	SlowHandlerThreshold time.Duration // default 500ms
	MaxReconnects        int           // default 10
	ReconnectBase        time.Duration // default 1s
	ReconnectMax         time.Duration // default 60s
	// ReconnectMaxRefused is the longest wait between attempts while the
	// subscription service refuses the subscription (default 5m).
	ReconnectMaxRefused time.Duration
	// dialer overrides the default gRPC dial path. Used by tests.
	dialer func(subscriptionURL, token string) (closeFn func() error, _ streamClient, _ error)
}

// Run drives a subscription until ctx is canceled or reconnects are exhausted.
//
// Lifecycle:
//   - opens a gRPC stream
//   - reads EventData, skips KEEPALIVEs, dispatches to handlers
//   - on stream error: emits synthetic CONNECTION_LOST to handlers, sleeps
//     with exponential backoff, retries up to MaxReconnects. The wait starts
//     at ReconnectBase and doubles for each failure in a row, up to
//     ReconnectMax, or ReconnectMaxRefused while the service refuses the
//     subscription (UNAUTHENTICATED or PERMISSION_DENIED); it starts over
//     when a stream delivered a message (the keepalive counts) or stayed
//     open a minute.
//   - first event after a successful reconnect emits CONNECTION_RECONNECTED
//   - returns nil when ctx is canceled or the server closes the stream cleanly
func Run(ctx context.Context, sub *Subscription, opts RunOptions) error {
	o := withDefaults(opts)
	backoff := reconnectBackoff{base: o.ReconnectBase, max: o.ReconnectMax, maxRefused: o.ReconnectMaxRefused}
	refusalSaid := false
	forceToken := false

	for {
		if ctx.Err() != nil {
			return nil
		}

		token, err := o.Tokens.AcquireToken(ctx, []string{o.SubscriptionScope}, forceToken)
		forceToken = false
		if err != nil {
			return fmt.Errorf("subscriptions: acquire token: %w", err)
		}

		closer, client, err := dialOrTest(o, token)
		if err != nil {
			return fmt.Errorf("subscriptions: dial: %w", err)
		}

		opened := time.Now()
		received, streamErr := readUntilDone(ctx, sub, client, o)
		healthy := received || time.Since(opened) >= reconnectResetAfter
		if closer != nil {
			_ = closer()
		}

		if ctx.Err() != nil {
			return nil
		}
		if streamErr == nil {
			return nil
		}
		if isCancelled(streamErr) {
			return nil
		}

		attempt := sub.incRetry()
		if attempt > o.MaxReconnects {
			o.Logger.Log("error", fmt.Sprintf(
				"subscription %s: exceeded %d reconnect attempts, giving up",
				sub.ID, o.MaxReconnects), streamErr)
			return streamErr
		}

		// Notify handlers.
		dispatchToAll(sub, syntheticEvent(EventConnectionLost, sub), o)

		refused := isRefused(streamErr)
		if healthy {
			refusalSaid = false
		}
		delay := backoff.next(healthy, refused)
		switch {
		case refused && !refusalSaid:
			// Said once at error level; the retries that meet the same
			// refusal are debug lines.
			refusalSaid = true
			// The token is kept between connects. A token the service does
			// not accept is asked for anew once, not on every retry.
			forceToken = status.Code(streamErr) == codes.Unauthenticated
			o.Logger.Log("error", refusalMessage(sub, streamErr, o.ReconnectMaxRefused), nil)
		case refused:
			o.Logger.Log("debug", fmt.Sprintf(
				"subscription %s: still refused (attempt %d/%d). Trying again in %s.",
				sub.ID, attempt, o.MaxReconnects, delay), nil)
		default:
			o.Logger.Log("warn", fmt.Sprintf(
				"subscription %s: stream error (attempt %d/%d): %v. Reconnecting in %s.",
				sub.ID, attempt, o.MaxReconnects, streamErr, delay), nil)
		}

		if err := sleepCtx(ctx, delay); err != nil {
			return nil
		}

		// Mutate the request so the next iteration resumes instead of
		// restarting from scratch (avoids re-running the entire baseload
		// after a transient drop). Mirrors DataceenClient.cs:ConnectionLostAsync.
		prepareRequestForReconnect(sub)
	}
}

// reconnectResetAfter: a stream that stayed open this long was a working
// connection, and the failure that ended it starts the waits over.
const reconnectResetAfter = 60 * time.Second

// reconnectDelay is the wait before connect number failures in a row
// (1 = after the first failure): base, doubled for each failure after the
// first, never above max.
func reconnectDelay(base, max time.Duration, failures int) time.Duration {
	d := base
	for i := 1; i < failures && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	return d
}

// reconnectBackoff counts the failed streams in a row.
type reconnectBackoff struct {
	base, max, maxRefused time.Duration
	failures              int
}

// next takes one failed stream and answers the wait before the next connect.
// healthy: the stream delivered a message or stayed open reconnectResetAfter,
// so this failure is the first of a new run. refused: the service refused the
// subscription, which no retry cures by itself, so the waits may grow to
// maxRefused.
func (b *reconnectBackoff) next(healthy, refused bool) time.Duration {
	if healthy {
		b.failures = 0
	}
	b.failures++
	if refused {
		return reconnectDelay(b.base, b.maxRefused, b.failures)
	}
	return reconnectDelay(b.base, b.max, b.failures)
}

// isRefused: the subscription service answered that this caller may not
// subscribe. A missing or unaccepted token is HTTP 401 (UNAUTHENTICATED); a
// token without the scope's role, or an unpublished scope, is HTTP 403 or the
// status itself (PERMISSION_DENIED).
func isRefused(err error) bool {
	switch status.Code(err) {
	case codes.Unauthenticated, codes.PermissionDenied:
		return true
	}
	return false
}

func refusalMessage(sub *Subscription, err error, every time.Duration) string {
	scope := sub.Request.Domain + "." + sub.Request.Model + "." + sub.Request.Scope
	why := "the application's registration lacks the role of scope " + scope + " for live updates"
	if status.Code(err) == codes.Unauthenticated {
		why = "its token was not accepted; check the application's registration and that it has the role of scope " + scope + " for live updates"
	}
	return fmt.Sprintf(
		"subscription %s was refused by the subscription service (%v): %s. Trying again with growing waits, at most every %s; the retries are logged at debug level.",
		sub.ID, err, why, every)
}

func prepareRequestForReconnect(sub *Subscription) {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	lastPos := sub.lastPosition
	if sub.isBaseLoading {
		sub.Request.BaseloadContinueObjectType = sub.baseloadLabel
		if lastPos != "" {
			sub.Request.Position = lastPos
		}
		return
	}
	sub.Request.BaseloadTopics = nil
	sub.Request.BaseloadContinueObjectType = ""
	sub.Request.PositionBeforeBaseload = ""
	switch sub.Request.StartMode {
	case StartPositionBeginning, StartPositionEnd, StartPositionTime:
		sub.Request.StartMode = StartPositionExact
	}
	if lastPos != "" {
		sub.Request.Position = lastPos
	}
}

func dialOrTest(o RunOptions, token string) (func() error, streamClient, error) {
	if o.dialer != nil {
		return o.dialer(o.SubscriptionURL, token)
	}
	conn, client, err := dialSubscription(o.SubscriptionURL, token)
	if err != nil {
		return nil, nil, err
	}
	return conn.Close, client, nil
}

func withDefaults(in RunOptions) RunOptions {
	if in.SlowHandlerThreshold == 0 {
		in.SlowHandlerThreshold = 500 * time.Millisecond
	}
	if in.MaxReconnects == 0 {
		in.MaxReconnects = 10
	}
	if in.ReconnectBase == 0 {
		in.ReconnectBase = 1 * time.Second
	}
	if in.ReconnectMax == 0 {
		in.ReconnectMax = 60 * time.Second
	}
	if in.ReconnectMaxRefused == 0 {
		in.ReconnectMaxRefused = 5 * time.Minute
	}
	if in.Logger == nil {
		in.Logger = silentLogger{}
	}
	return in
}

// readUntilDone opens the stream and reads events until the server ends it
// or an error occurs. ctx cancellation aborts the stream cleanly. received
// says whether the stream delivered a message; the keepalive counts, since the
// service sends one only on a stream it has accepted.
func readUntilDone(ctx context.Context, sub *Subscription, client streamClient, o RunOptions) (received bool, _ error) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := client.Subscribe(streamCtx, toProtoRequest(sub.Request))
	if err != nil {
		return false, err
	}

	for {
		raw, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return received, nil
			}
			return received, err
		}
		if ctx.Err() != nil {
			return received, nil
		}
		received = true
		if raw.GetMessagetype() == pb.MessageType_KEEPALIVE {
			continue
		}

		evt := toLowLevel(raw)
		if sub.resetRetry() {
			dispatchToAll(sub, syntheticEvent(EventConnectionReconnected, sub), o)
		}

		sub.updateState(evt.EventType, evt.Topic, evt.Position)
		dispatchOne(sub, evt, o)
	}
}

// dispatchOne invokes the topic-specific handler (if any) plus all raw
// handlers for one real event.
func dispatchOne(sub *Subscription, evt *LowLevelEvent, o RunOptions) {
	topics, raws := sub.snapshotHandlers()
	if h, ok := topics[evt.Topic]; ok {
		runWithTiming(o, fmt.Sprintf("typed handler (topic=%s)", evt.Topic), func() error {
			return h(evt)
		})
	}
	for i, rh := range raws {
		i, rh := i, rh
		runWithTiming(o, fmt.Sprintf("raw handler #%d", i+1), func() error {
			return rh(evt)
		})
	}
}

// dispatchToAll fires a (typically synthetic) event at every typed handler
// and every raw handler. Used for CONNECTION_LOST / CONNECTION_RECONNECTED
// so apps can react to lifecycle changes.
func dispatchToAll(sub *Subscription, evt *LowLevelEvent, o RunOptions) {
	topics, raws := sub.snapshotHandlers()
	for topic, h := range topics {
		clone := *evt
		clone.Topic = topic
		runWithTiming(o, fmt.Sprintf("typed handler (synthetic %s, topic=%s)", evt.EventType, topic), func() error {
			return h(&clone)
		})
	}
	for i, rh := range raws {
		i, rh := i, rh
		runWithTiming(o, fmt.Sprintf("raw handler (synthetic %s) #%d", evt.EventType, i+1), func() error {
			return rh(evt)
		})
	}
}

// runWithTiming invokes action and logs a warning if it exceeds the slow
// threshold. Handler-thrown errors are caught and logged so one bad handler
// can't kill the stream.
func runWithTiming(o RunOptions, label string, action func() error) {
	start := time.Now()
	err := safeCall(action)
	if err != nil {
		o.Logger.Log("error", fmt.Sprintf("handler threw (%s)", label), err)
		return
	}
	if elapsed := time.Since(start); elapsed > o.SlowHandlerThreshold {
		o.Logger.Log("warn", fmt.Sprintf("slow %s: %s", label, elapsed), nil)
	}
}

// safeCall converts a panic into an error so the stream loop can keep going.
func safeCall(action func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return action()
}

func toLowLevel(raw *pb.EventData) *LowLevelEvent {
	return &LowLevelEvent{
		Position:      raw.GetPosition(),
		EventDateTime: timestampToISO(raw),
		OperationType: OperationType(raw.GetOperationtype().String()),
		ClientID:      raw.GetClientid(),
		Domain:        raw.GetDomain(),
		Topic:         raw.GetTopic(),
		ID:            raw.GetId(),
		FromID:        raw.GetFromid(),
		ToID:          raw.GetToid(),
		Delta:         raw.GetDelta(),
		Complete:      raw.GetComplete(),
		EventType:     EventType(raw.GetEventtype().String()),
		ObjectType:    ObjectType(raw.GetObjecttype().String()),
		Model:         raw.GetModel(),
	}
}

func timestampToISO(raw *pb.EventData) string {
	ts := raw.GetEventtime()
	if ts == nil {
		return ""
	}
	return ts.AsTime().UTC().Format(time.RFC3339Nano)
}

func syntheticEvent(t EventType, sub *Subscription) *LowLevelEvent {
	return &LowLevelEvent{
		Position:      sub.LastPosition(),
		EventDateTime: time.Now().UTC().Format(time.RFC3339Nano),
		OperationType: OpUpdate,
		EventType:     t,
		ObjectType:    ObjectNode,
		Domain:        sub.Request.Domain,
		Model:         sub.Request.Model,
	}
}

func isCancelled(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if s, ok := status.FromError(err); ok {
		return s.Code() == codes.Canceled || s.Code() == codes.DeadlineExceeded
	}
	return false
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
