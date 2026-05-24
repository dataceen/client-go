// Spike 2: POST a GraphQL query to Dataceen and print the result.
// Mirrors DataceenClient.FindNodesAsync<Customer>(size: 5) at the wire level.
//
// Note on naming: C# method is FindCustomerNodesAsync, but the wire query is
// `FindCustomer`. The "Nodes" suffix is a C# surface choice only.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"

	"github.com/dataceen/client-go/internal/spikecfg"
)

func acquireToken(ctx context.Context, cfg spikecfg.Config) (string, error) {
	cred, err := confidential.NewCredFromSecret(cfg.ClientSecret)
	if err != nil {
		return "", fmt.Errorf("cred: %w", err)
	}
	app, err := confidential.New(cfg.Authority(), cfg.ClientID, cred)
	if err != nil {
		return "", fmt.Errorf("confidential.New: %w", err)
	}
	res, err := app.AcquireTokenByCredential(ctx, []string{cfg.APIScope})
	if err != nil {
		return "", fmt.Errorf("AcquireTokenByCredential: %w", err)
	}
	return res.AccessToken, nil
}

type graphqlReq struct {
	OperationName string `json:"operationName"`
	Query         string `json:"query"`
}

func main() {
	cfg, err := spikecfg.Load()
	if err != nil {
		log.Fatalf("[spike-02] config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	token, err := acquireToken(ctx, cfg)
	if err != nil {
		log.Fatalf("[spike-02] token: %v", err)
	}
	fmt.Println("[spike-02] Got token")

	body, err := json.Marshal(graphqlReq{
		OperationName: "FindCustomer",
		Query:         `query {FindCustomer(size: 5, cursor: "null", where: null) { Items { _id CustomerId } }}`,
	})
	if err != nil {
		log.Fatalf("[spike-02] marshal: %v", err)
	}

	url := cfg.APIURL + cfg.FullAPIPath()
	fmt.Printf("[spike-02] POST %s\n", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Fatalf("[spike-02] new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("[spike-02] do: %v", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("[spike-02] read body: %v", err)
	}

	fmt.Printf("[spike-02] HTTP %d\n", resp.StatusCode)
	preview := string(respBytes)
	if len(preview) > 800 {
		preview = preview[:800] + "…(truncated)"
	}
	fmt.Printf("[spike-02] Body: %s\n", preview)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Fatalf("[spike-02] non-2xx: %d", resp.StatusCode)
	}

	var parsed struct {
		Data struct {
			FindCustomer struct {
				Items []struct {
					ID         string `json:"_id"`
					CustomerID string `json:"CustomerId"`
				} `json:"Items"`
			} `json:"FindCustomer"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		log.Fatalf("[spike-02] parse: %v", err)
	}
	if len(parsed.Errors) > 0 {
		fmt.Printf("[spike-02] GraphQL errors: %+v\n", parsed.Errors)
	}
	fmt.Printf("[spike-02] Items returned: %d\n", len(parsed.Data.FindCustomer.Items))
	fmt.Println("[spike-02] OK")
}
