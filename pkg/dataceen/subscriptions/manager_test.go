package subscriptions

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/dataceen/client-go/pkg/dataceenevent"
)

// fakeStream replays a scripted slice of events, then returns io.EOF.
type fakeStream struct {
	events []*pb.EventData
	idx    int
	err    error // returned in place of io.EOF if non-nil
}

func (f *fakeStream) Recv() (*pb.EventData, error) {
	if f.idx >= len(f.events) {
		if f.err != nil {
			return nil, f.err
		}
		return nil, io.EOF
	}
	e := f.events[f.idx]
	f.idx++
	return e, nil
}

type fakeStreamClient struct {
	stream *fakeStream
}

func (f *fakeStreamClient) Subscribe(_ context.Context, _ *pb.SubscriptionRequest) (eventStream, error) {
	return f.stream, nil
}

type fakeTokenAcquirer struct{}

func (fakeTokenAcquirer) AcquireToken(context.Context, []string, bool) (string, error) {
	return "fake-token", nil
}

func newSub(t *testing.T) *Subscription {
	t.Helper()
	return NewSubscription("test", Request{
		Domain: "Thomas",
		Model:  "CandyShopModel",
		Scope:  "All",
		Topics: []string{"Customer"},
	})
}

func mkEvent(eventType pb.EventType, topic, id, complete string) *pb.EventData {
	return &pb.EventData{
		Eventtype:   eventType,
		Messagetype: pb.MessageType_NORMAL,
		Topic:       topic,
		Id:          id,
		Complete:    complete,
		Position:    "pos-" + id,
		Eventtime:   timestamppb.New(time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)),
	}
}

func runWithFake(ctx context.Context, sub *Subscription, stream *fakeStream) error {
	dialer := func(_, _ string) (func() error, streamClient, error) {
		return func() error { return nil }, &fakeStreamClient{stream: stream}, nil
	}
	return Run(ctx, sub, RunOptions{
		Tokens:        fakeTokenAcquirer{},
		Logger:        silentLogger{},
		MaxReconnects: 0, // never retry in tests
		ReconnectBase: 1 * time.Millisecond,
		dialer:        dialer,
	})
}

func TestManager_DispatchesTypedHandler(t *testing.T) {
	sub := newSub(t)

	type customer struct {
		ID         string `json:"_id"`
		CustomerID string `json:"CustomerId"`
	}
	var got []*Event[customer]
	On(sub, "Customer", func(evt *Event[customer]) error {
		got = append(got, evt)
		return nil
	})

	stream := &fakeStream{events: []*pb.EventData{
		mkEvent(pb.EventType_BASELOAD_EVENT, "Customer", "c1", `{"_id":"c1","CustomerId":"cst-1"}`),
		mkEvent(pb.EventType_BASELOAD_EVENT, "Customer", "c2", `{"_id":"c2","CustomerId":"cst-2"}`),
		mkEvent(pb.EventType_BASELOAD_END, "Customer", "", ""),
	}}

	if err := runWithFake(context.Background(), sub, stream); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	if got[0].Complete.CustomerID != "cst-1" {
		t.Errorf("event[0].Complete.CustomerID = %q, want cst-1", got[0].Complete.CustomerID)
	}
	if got[0].EventType != EventBaseload {
		t.Errorf("event[0].EventType = %s, want BASELOAD_EVENT", got[0].EventType)
	}
	if got[2].EventType != EventBaseloadEnd {
		t.Errorf("event[2].EventType = %s, want BASELOAD_END", got[2].EventType)
	}
	// BASELOAD_END has empty complete; CompletePresent should be false.
	if got[2].CompletePresent {
		t.Error("BASELOAD_END should have CompletePresent=false")
	}
}

func TestManager_RawHandlerSeesEveryEvent(t *testing.T) {
	sub := newSub(t)
	var n int32
	sub.OnRaw(func(*LowLevelEvent) error {
		atomic.AddInt32(&n, 1)
		return nil
	})

	stream := &fakeStream{events: []*pb.EventData{
		mkEvent(pb.EventType_BASELOAD_EVENT, "Customer", "c1", `{}`),
		mkEvent(pb.EventType_SUBSCRIPTION_EVENT, "Order", "o1", `{}`),
		mkEvent(pb.EventType_BASELOAD_END, "Customer", "", ""),
	}}
	if err := runWithFake(context.Background(), sub, stream); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt32(&n); got != 3 {
		t.Errorf("raw handler invoked %d times, want 3", got)
	}
}

func TestManager_SkipsKeepalive(t *testing.T) {
	sub := newSub(t)
	var n int32
	sub.OnRaw(func(*LowLevelEvent) error {
		atomic.AddInt32(&n, 1)
		return nil
	})

	keepalive := mkEvent(pb.EventType_SUBSCRIPTION_EVENT, "Customer", "k1", `{}`)
	keepalive.Messagetype = pb.MessageType_KEEPALIVE
	stream := &fakeStream{events: []*pb.EventData{
		keepalive,
		mkEvent(pb.EventType_BASELOAD_EVENT, "Customer", "c1", `{}`),
	}}
	if err := runWithFake(context.Background(), sub, stream); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt32(&n); got != 1 {
		t.Errorf("raw handler invoked %d times, want 1 (KEEPALIVE filtered)", got)
	}
}

func TestManager_BaseloadStateTracking(t *testing.T) {
	sub := newSub(t)
	stream := &fakeStream{events: []*pb.EventData{
		mkEvent(pb.EventType_BASELOAD_EVENT, "Customer", "c1", ""),
		mkEvent(pb.EventType_BASELOAD_EVENT, "Customer", "c2", ""),
		mkEvent(pb.EventType_BASELOAD_END, "Customer", "", ""),
		mkEvent(pb.EventType_SUBSCRIPTION_EVENT, "Customer", "c3", ""),
	}}

	// Snapshots: capture state mid-stream via a raw handler.
	var snapshots []bool
	sub.OnRaw(func(evt *LowLevelEvent) error {
		// snapshot the *post-update* state so handlers see consistent values
		snapshots = append(snapshots, sub.IsBaseLoading())
		return nil
	})

	if err := runWithFake(context.Background(), sub, stream); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []bool{true, true, false, false}
	if len(snapshots) != len(want) {
		t.Fatalf("snapshots = %v, want %v", snapshots, want)
	}
	for i, w := range want {
		if snapshots[i] != w {
			t.Errorf("snapshot[%d] = %v, want %v (event=%d)", i, snapshots[i], w, i)
		}
	}
	if sub.LastPosition() != "pos-c3" {
		t.Errorf("LastPosition = %q, want pos-c3", sub.LastPosition())
	}
}

func TestManager_HandlerErrorDoesNotKillStream(t *testing.T) {
	sub := newSub(t)
	var seen int32
	sub.OnRaw(func(*LowLevelEvent) error {
		n := atomic.AddInt32(&seen, 1)
		if n == 1 {
			return errors.New("first handler crash")
		}
		return nil
	})

	stream := &fakeStream{events: []*pb.EventData{
		mkEvent(pb.EventType_BASELOAD_EVENT, "Customer", "c1", `{}`),
		mkEvent(pb.EventType_BASELOAD_EVENT, "Customer", "c2", `{}`),
	}}
	if err := runWithFake(context.Background(), sub, stream); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt32(&seen); got != 2 {
		t.Errorf("handler invoked %d times, want 2 (one error didn't stop the loop)", got)
	}
}

func TestManager_ContextCancelExitsCleanly(t *testing.T) {
	sub := newSub(t)
	stream := &fakeStream{
		// Provide a long stream — we'll cancel mid-flight.
		events: make([]*pb.EventData, 1000),
	}
	for i := range stream.events {
		stream.events[i] = mkEvent(pb.EventType_BASELOAD_EVENT, "Customer", "c", `{}`)
	}

	count := int32(0)
	sub.OnRaw(func(*LowLevelEvent) error {
		atomic.AddInt32(&count, 1)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Cancel after some events have been consumed.
		for atomic.LoadInt32(&count) < 5 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	if err := runWithFake(ctx, sub, stream); err != nil {
		t.Fatalf("Run returned error on cancel: %v", err)
	}
	got := atomic.LoadInt32(&count)
	if got < 5 {
		t.Errorf("got %d events, expected ≥5 before cancel", got)
	}
	if got >= int32(len(stream.events)) {
		t.Errorf("cancel did not stop the stream — saw all %d events", got)
	}
}
