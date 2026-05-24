// Command candyshop demonstrates the Dataceen Go client end-to-end against
// the live CandyShopModel backend. It covers:
//
//   - Find / FindByID / pagination / order-by (read-only)
//   - Like-filter via the DSL
//   - Subscription with a typed handler (read-only)
//   - Create / Update / Delete (gated by RUN_DESTRUCTIVE=1)
//
// Run from the repo root:
//
//	make codegen          # if pkg/generated is empty
//	go run ./examples/candyshop                 # read-only
//	RUN_DESTRUCTIVE=1 go run ./examples/candyshop  # adds CRUD demo
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dataceen/client-go/pkg/dataceen"
	"github.com/dataceen/client-go/pkg/generated"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	client, err := dataceen.NewClient(cfg, dataceen.WithLogger(dataceen.SilentLogger))
	if err != nil {
		return fmt.Errorf("NewClient: %w", err)
	}
	api := generated.NewAllClient(client)

	fmt.Printf("Dataceen Go client — CandyShop example\n")
	fmt.Printf("model: %s/%s/%s @ %s\n", cfg.Domain, cfg.Model, cfg.Scope, cfg.APIURL)

	rootCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := runFinds(rootCtx, api); err != nil {
		return fmt.Errorf("finds: %w", err)
	}

	if err := runSubscribe(rootCtx, client); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	if os.Getenv("RUN_DESTRUCTIVE") != "1" {
		fmt.Println("\n--- 7. Destructive scenarios skipped ---")
		fmt.Println("    Set RUN_DESTRUCTIVE=1 to run Create/Update/Delete.")
	} else {
		if err := runDestructive(rootCtx, api); err != nil {
			return fmt.Errorf("destructive: %w", err)
		}
		if err := runBulkDestructive(rootCtx, api); err != nil {
			return fmt.Errorf("bulk: %w", err)
		}
	}

	if err := runSearch(rootCtx, api); err != nil {
		return fmt.Errorf("search: %w", err)
	}

	fmt.Println("\nAll scenarios passed.")
	return nil
}
