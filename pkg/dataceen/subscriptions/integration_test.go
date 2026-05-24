package subscriptions_test

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joho/godotenv"

	"github.com/dataceen/client-go/pkg/dataceen"
	"github.com/dataceen/client-go/pkg/dataceen/subscriptions"
)

// TestIntegration_SubscribeCustomer opens a baseload subscription on the
// Customer topic and verifies that the typed handler receives at least 5
// events with parsed payloads before clean cancellation.
//
//	RUN_INTEGRATION=1 go test ./pkg/dataceen/subscriptions -run Integration -v
//
// KNOWN ISSUE (2026-05-04): the EventData proto was renumbered to insert
// `domain` at tag 8, shifting model/topic/id/etc. by one position. The
// server has not yet rolled out the matching schema, so on-the-wire data
// arrives at the OLD tag numbers — the client decodes Topic where the
// server sent the old `id`, etc. As a result this test expects e.Topic
// == "Customer" but receives "Customer_Example_00000".
//
// Until the server rolls forward, this test is expected to fail with
// "received 0 events" — the topic-filter inside the handler ignores
// events whose topic doesn't match. Re-run once the server is updated.
func TestIntegration_SubscribeCustomer(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION") != "1" {
		t.Skip("set RUN_INTEGRATION=1 to run live-backend tests")
	}
	for _, p := range []string{".env", "../../.env", "../../../.env"} {
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Load(p)
			break
		}
	}

	cfg := dataceen.Config{
		TenantID:          os.Getenv("DATACEEN_TENANT_ID"),
		ClientID:          os.Getenv("DATACEEN_CLIENT_ID"),
		ClientSecret:      os.Getenv("DATACEEN_CLIENT_SECRET"),
		Domain:            os.Getenv("DATACEEN_DOMAIN"),
		Model:             os.Getenv("DATACEEN_MODEL"),
		Scope:             os.Getenv("DATACEEN_SCOPE"),
		APIURL:            os.Getenv("DATACEEN_API_URL"),
		APIPath:           os.Getenv("DATACEEN_API_PATH"),
		APIScope:          os.Getenv("DATACEEN_API_SCOPE"),
		SubscriptionURL:   os.Getenv("DATACEEN_SUBSCRIPTION_URL"),
		SubscriptionScope: os.Getenv("DATACEEN_SUBSCRIPTION_SCOPE"),
	}
	if cfg.ClientSecret == "" {
		t.Fatal("DATACEEN_CLIENT_SECRET is empty")
	}

	client, err := dataceen.NewClient(cfg, dataceen.WithLogger(dataceen.SilentLogger))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	type customer struct {
		ID         string `json:"_id"`
		CustomerID string `json:"CustomerId"`
	}

	sub := client.CreateSubscription(subscriptions.Request{
		Topics:          []string{"Customer"},
		BaseloadTopics:  []string{"Customer"},
		StartMode:       subscriptions.StartPositionBeginning,
		IncludeComplete: true,
		IncludeDelta:    true,
	})

	var received int32
	const maxEvents = 5
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	subscriptions.On(sub, "Customer", func(evt *subscriptions.Event[customer]) error {
		n := atomic.AddInt32(&received, 1)
		t.Logf("event %d: type=%s topic=%s id=%s CustomerId=%q present=%t",
			n, evt.EventType, evt.Topic, evt.ID, evt.Complete.CustomerID, evt.CompletePresent)
		if n >= maxEvents {
			cancel() // we've seen enough — exit cleanly
		}
		return nil
	})

	if err := client.Subscribe(ctx, sub); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	got := atomic.LoadInt32(&received)
	if got < maxEvents {
		t.Fatalf("received %d events, want ≥%d", got, maxEvents)
	}
	t.Logf("clean shutdown after %d events; lastPosition=%s", got, sub.LastPosition())
}
