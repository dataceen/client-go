package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"github.com/dataceen/client-go/pkg/dataceen"
)

// loadConfig hunts for a .env at the example dir, the repo root, or one
// level above. Mirrors how the integration tests find credentials.
func loadConfig() (dataceen.Config, error) {
	for _, p := range []string{".env", "../../.env", "../../../.env", "../.env"} {
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
		return cfg, fmt.Errorf("DATACEEN_CLIENT_SECRET is empty (looked for .env in cwd, ../, ../../, ../../../)")
	}
	return cfg, nil
}
