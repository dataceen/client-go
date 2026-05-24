// Package spikecfg loads .env-based config for the Phase 0 spike binaries.
// Mirrors the role of src/spike/_config.ts in the TypeScript port.
package spikecfg

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	TenantID     string
	ClientID     string
	ClientSecret string

	Domain string
	Model  string
	Scope  string

	APIURL  string
	APIPath string
	APIScope string

	SubscriptionURL   string
	SubscriptionScope string
}

// Authority is the Microsoft login URL for client-credentials flow.
func (c Config) Authority() string {
	return "https://login.microsoftonline.com/" + c.TenantID
}

// FullAPIPath mirrors the C# DataceenClient construction:
// $"{ApiPath}/{Domain}/{Model}/{Scope}".
func (c Config) FullAPIPath() string {
	return fmt.Sprintf("%s/%s/%s/%s", c.APIPath, c.Domain, c.Model, c.Scope)
}

// SubscriptionHost converts the SubscriptionURL into a host:port string
// suitable for grpc.NewClient. https→443, http→80.
func (c Config) SubscriptionHost() (string, error) {
	u, err := url.Parse(c.SubscriptionURL)
	if err != nil {
		return "", fmt.Errorf("parse subscription URL: %w", err)
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		switch strings.ToLower(u.Scheme) {
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}
	return host + ":" + port, nil
}

// Load reads .env (if present) and pulls the required vars.
// Returns an error listing every missing variable in one shot.
func Load() (Config, error) {
	// godotenv.Load is non-fatal if the file isn't present; env vars from the
	// shell still win.
	_ = godotenv.Load()

	required := []string{
		"DATACEEN_TENANT_ID",
		"DATACEEN_CLIENT_ID",
		"DATACEEN_CLIENT_SECRET",
		"DATACEEN_DOMAIN",
		"DATACEEN_MODEL",
		"DATACEEN_SCOPE",
		"DATACEEN_API_URL",
		"DATACEEN_API_PATH",
		"DATACEEN_API_SCOPE",
		"DATACEEN_SUBSCRIPTION_URL",
		"DATACEEN_SUBSCRIPTION_SCOPE",
	}
	var missing []string
	for _, k := range required {
		if os.Getenv(k) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing env vars: %s", strings.Join(missing, ", "))
	}

	return Config{
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
	}, nil
}
