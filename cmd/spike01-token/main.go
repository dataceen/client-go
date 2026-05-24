// Spike 1: fetch an Azure AD token via client-credentials.
// Mirrors C# TokenProvider.GetAccessTokenAsync and the TS spike 01-token.ts,
// using the Microsoft Authentication Library for Go (MSAL).
package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"

	"github.com/dataceen/client-go/internal/spikecfg"
)

func main() {
	cfg, err := spikecfg.Load()
	if err != nil {
		log.Fatalf("[spike-01] config: %v", err)
	}

	cred, err := confidential.NewCredFromSecret(cfg.ClientSecret)
	if err != nil {
		log.Fatalf("[spike-01] cred: %v", err)
	}

	app, err := confidential.New(cfg.Authority(), cfg.ClientID, cred)
	if err != nil {
		log.Fatalf("[spike-01] confidential.New: %v", err)
	}

	fmt.Printf("[spike-01] Acquiring token for scope=%s\n", cfg.APIScope)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := app.AcquireTokenByCredential(ctx, []string{cfg.APIScope})
	if err != nil {
		log.Fatalf("[spike-01] AcquireTokenByCredential: %v", err)
	}
	if res.AccessToken == "" {
		log.Fatalf("[spike-01] empty access token")
	}

	parts := strings.Split(res.AccessToken, ".")
	fmt.Printf("[spike-01] Got token (%d JWT segments), length=%d\n", len(parts), len(res.AccessToken))
	fmt.Printf("[spike-01] Expires on: %s\n", res.ExpiresOn.UTC().Format(time.RFC3339))
	fmt.Println("[spike-01] OK")
}
