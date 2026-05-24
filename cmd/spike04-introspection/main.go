// Spike 4: probe whether the Dataceen GraphQL server supports introspection.
// Mirrors the TS spike 04-introspection.ts.
//
// Goal: confirm a schema source for the Phase 3 codegen without having to
// reverse-engineer the C# generated files.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
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

func post(ctx context.Context, url, token string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview := string(respBytes)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, preview)
	}
	return respBytes, nil
}

func main() {
	cfg, err := spikecfg.Load()
	if err != nil {
		log.Fatalf("[spike-04] config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	token, err := acquireToken(ctx, cfg)
	if err != nil {
		log.Fatalf("[spike-04] token: %v", err)
	}
	fmt.Println("[spike-04] Got token")

	url := cfg.APIURL + cfg.FullAPIPath()

	// ---------------------------------------------------------------------
	// Step 1: minimal introspection probe
	// ---------------------------------------------------------------------
	fmt.Println("\n[spike-04] Step 1: minimal introspection ({ __schema { queryType { name } } })")
	step1Body, err := post(ctx, url, token, map[string]string{
		"operationName": "Introspect",
		"query":         "{ __schema { queryType { name } } }",
	})
	if err != nil {
		log.Fatalf("[spike-04] step 1: %v", err)
	}

	var step1 struct {
		Data struct {
			Schema struct {
				QueryType struct {
					Name string `json:"name"`
				} `json:"queryType"`
			} `json:"__schema"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(step1Body, &step1); err != nil {
		log.Fatalf("[spike-04] parse step 1: %v", err)
	}
	if len(step1.Errors) > 0 {
		var msgs []string
		for _, e := range step1.Errors {
			msgs = append(msgs, e.Message)
		}
		fmt.Printf("[spike-04] Introspection BLOCKED: %s\n", strings.Join(msgs, "; "))
		fmt.Println("[spike-04] Conclusion: use plan B (reverse-engineer from C# files)")
		return
	}
	queryType := step1.Data.Schema.QueryType.Name
	if queryType == "" {
		fmt.Printf("[spike-04] Unexpected response: %s\n", string(step1Body))
		return
	}
	fmt.Printf("[spike-04] Introspection ENABLED. Query root type: %s\n", queryType)

	// ---------------------------------------------------------------------
	// Step 2: list type names
	// ---------------------------------------------------------------------
	fmt.Println("\n[spike-04] Step 2: list all types")
	step2Body, err := post(ctx, url, token, map[string]string{
		"operationName": "AllTypes",
		"query":         "{ __schema { types { name kind } } }",
	})
	if err != nil {
		log.Fatalf("[spike-04] step 2: %v", err)
	}
	var step2 struct {
		Data struct {
			Schema struct {
				Types []struct {
					Name string `json:"name"`
					Kind string `json:"kind"`
				} `json:"types"`
			} `json:"__schema"`
		} `json:"data"`
	}
	if err := json.Unmarshal(step2Body, &step2); err != nil {
		log.Fatalf("[spike-04] parse step 2: %v", err)
	}
	types := step2.Data.Schema.Types
	fmt.Printf("[spike-04] Found %d types total\n", len(types))

	byKind := map[string]int{}
	for _, t := range types {
		byKind[t.Kind]++
	}
	fmt.Printf("[spike-04] By kind: %+v\n", byKind)

	builtin := map[string]bool{"String": true, "Int": true, "Float": true, "Boolean": true, "ID": true}
	model := types[:0:0]
	for _, t := range types {
		if strings.HasPrefix(t.Name, "__") {
			continue
		}
		if builtin[t.Name] {
			continue
		}
		model = append(model, t)
	}
	sort.Slice(model, func(i, j int) bool { return model[i].Name < model[j].Name })

	fmt.Printf("\n[spike-04] Model-looking types (%d):\n", len(model))
	limit := 40
	if len(model) < limit {
		limit = len(model)
	}
	for _, t := range model[:limit] {
		fmt.Printf("  %-12s %s\n", t.Kind, t.Name)
	}
	if len(model) > limit {
		fmt.Printf("  ... and %d more\n", len(model)-limit)
	}

	// ---------------------------------------------------------------------
	// Step 3: Customer fields (if present)
	// ---------------------------------------------------------------------
	hasCustomer := false
	for _, t := range model {
		if t.Name == "Customer" {
			hasCustomer = true
			break
		}
	}
	if !hasCustomer {
		fmt.Println(`[spike-04] No "Customer" type — generator will need a different name pattern`)
		fmt.Println("\n[spike-04] OK")
		return
	}

	fmt.Println("\n[spike-04] Step 3: Customer type fields")
	step3Body, err := post(ctx, url, token, map[string]string{
		"operationName": "CustomerType",
		"query":         `{ __type(name: "Customer") { name kind fields { name type { name kind ofType { name kind } } } } }`,
	})
	if err != nil {
		log.Fatalf("[spike-04] step 3: %v", err)
	}
	var step3 struct {
		Data struct {
			Type struct {
				Fields []struct {
					Name string `json:"name"`
					Type struct {
						Name   string `json:"name"`
						Kind   string `json:"kind"`
						OfType *struct {
							Name string `json:"name"`
							Kind string `json:"kind"`
						} `json:"ofType"`
					} `json:"type"`
				} `json:"fields"`
			} `json:"__type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(step3Body, &step3); err != nil {
		log.Fatalf("[spike-04] parse step 3: %v", err)
	}
	fields := step3.Data.Type.Fields
	fmt.Printf("[spike-04] Customer has %d fields:\n", len(fields))
	flimit := 30
	if len(fields) < flimit {
		flimit = len(fields)
	}
	for _, f := range fields[:flimit] {
		t := f.Type.Name
		if t == "" {
			of := "?"
			if f.Type.OfType != nil {
				if f.Type.OfType.Name != "" {
					of = f.Type.OfType.Name
				} else {
					of = f.Type.OfType.Kind
				}
			}
			t = fmt.Sprintf("%s(%s)", f.Type.Kind, of)
		}
		fmt.Printf("  %-30s : %s\n", f.Name, t)
	}
	if len(fields) > flimit {
		fmt.Printf("  ... and %d more\n", len(fields)-flimit)
	}

	fmt.Println("\n[spike-04] OK")
}
