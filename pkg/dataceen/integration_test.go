package dataceen_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"

	"github.com/dataceen/client-go/pkg/dataceen"
)

// TestIntegration_FindCustomer hits the live Dataceen backend. Gated by
// RUN_INTEGRATION=1; set credentials via .env (mirror of the TS port's .env).
//
//	RUN_INTEGRATION=1 go test ./pkg/dataceen -run Integration -v
func TestIntegration_FindCustomer(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION") != "1" {
		t.Skip("set RUN_INTEGRATION=1 to run live-backend tests")
	}

	// Try the repo-root .env (test runs from package dir, so go up two).
	for _, p := range []string{"../../.env", ".env"} {
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
		t.Fatal("DATACEEN_CLIENT_SECRET is empty — cannot run integration test")
	}

	client, err := dataceen.NewClient(cfg, dataceen.WithLogger(dataceen.SilentLogger))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	type customer struct {
		ID         string `json:"_id"`
		CustomerID string `json:"CustomerId"`
	}
	var res dataceen.FindResult[customer]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = client.ExecuteQuery(ctx, dataceen.GraphQL{
		OperationName: "FindCustomer",
		Query:         `query { FindCustomer(size: 5, cursor: "null", where: null) { Items { _id CustomerId } Cursor } }`,
	}, &res)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	if len(res.Items) != 5 {
		t.Fatalf("Items=%d, want 5", len(res.Items))
	}
	if res.Items[0].CustomerID == "" {
		t.Fatal("first item has empty CustomerId")
	}
	t.Logf("got %d customers, first=%s, cursor=%s", len(res.Items), res.Items[0].CustomerID, res.Cursor)
}
