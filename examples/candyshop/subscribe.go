package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/dataceen/client-go/pkg/dataceen"
	"github.com/dataceen/client-go/pkg/dataceen/subscriptions"
	"github.com/dataceen/client-go/pkg/generated"
)

// runSubscribe opens a baseload subscription on the Customer topic, prints
// the first N events with parsed payloads, then cancels cleanly. Read-only.
func runSubscribe(ctx context.Context, client *dataceen.Client) error {
	fmt.Println("\n--- 6. Subscription: BASELOAD on Customer (cap 5 events) ---")

	sub := client.CreateSubscription(subscriptions.Request{
		Topics:          []string{"Customer"},
		BaseloadTopics:  []string{"Customer"},
		StartMode:       subscriptions.StartPositionBeginning,
		IncludeComplete: true,
		IncludeDelta:    true,
	})

	const maxEvents = 5
	var received int32
	subCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	subscriptions.On(sub, "Customer", func(evt *subscriptions.Event[generated.Customer]) error {
		n := atomic.AddInt32(&received, 1)
		fmt.Printf("  event %d: type=%s id=%s CustomerId=%q present=%t\n",
			n, evt.EventType, evt.ID, evt.Complete.CustomerId, evt.CompletePresent)
		if n >= maxEvents {
			cancel()
		}
		return nil
	})

	if err := client.Subscribe(subCtx, sub); err != nil {
		return err
	}

	fmt.Printf("  clean shutdown after %d events; lastPosition=%s\n",
		atomic.LoadInt32(&received), sub.LastPosition())
	return nil
}
