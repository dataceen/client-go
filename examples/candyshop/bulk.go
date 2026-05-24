package main

import (
	"context"
	"fmt"
	"time"

	"github.com/dataceen/client-go/pkg/dataceen"
	"github.com/dataceen/client-go/pkg/dataceen/dsl"
	"github.com/dataceen/client-go/pkg/generated"
)

// runBulkDestructive demonstrates CreateBulk and UpdateBulk on Customer.
// Cleans up via DeleteCustomer at the end. Skip-friendly — failures here
// don't take down the rest of the demo.
func runBulkDestructive(ctx context.Context, api *generated.AllClient) error {
	fmt.Println("\n  7f. Bulk Create + Bulk Update")
	stamp := time.Now().Unix()
	idA := fmt.Sprintf("cst-go-bulk-a-%d", stamp)
	idB := fmt.Sprintf("cst-go-bulk-b-%d", stamp)
	idC := fmt.Sprintf("cst-go-bulk-c-%d", stamp)

	createRes, err := api.Customer.CreateBulk(ctx, []*generated.CustomerCreate{
		{CustomerId: idA, IsActive: true, ExternalReference: "bulk-a", CreatedAt: time.Now().UTC().Format(time.RFC3339)},
		{CustomerId: idB, IsActive: true, ExternalReference: "bulk-b", CreatedAt: time.Now().UTC().Format(time.RFC3339)},
		{CustomerId: idC, IsActive: true, ExternalReference: "bulk-c", CreatedAt: time.Now().UTC().Format(time.RFC3339)},
	})
	if err != nil {
		return fmt.Errorf("CreateBulk: %w", err)
	}
	fmt.Printf("     CreateBulk: ResponseCode=%d, %d ids returned\n",
		createRes.ResponseCode, len(createRes.Result))
	newIDs := createRes.Result
	if len(newIDs) != 3 {
		return fmt.Errorf("expected 3 ids, got %d", len(newIDs))
	}

	// Cleanup at the end via Delete-by-filter. Use a deferred closure so
	// cleanup runs even when UpdateBulk fails.
	defer func() {
		fmt.Println("\n  7g. Bulk cleanup: Delete-by-filter")
		filter := dsl.F("CustomerId").In(any(idA), any(idB), any(idC))
		delRes, err := api.Customer.Delete(ctx, filter)
		if err != nil {
			fmt.Printf("     ! delete failed: %v\n", err)
			return
		}
		fmt.Printf("     ResponseCode=%d, %d rows deleted\n",
			delRes.ResponseCode, len(delRes.Result))
	}()

	// UpdateBulk: deactivate two of the three rows. Each Update item
	// carries its MetaId (the server uses it to locate the row).
	updates := []*generated.CustomerUpdate{
		{MetaId: dataceen.Ptr(newIDs[0]), IsActive: dataceen.Ptr(false), ExternalReference: dataceen.Ptr("bulk-a-updated")},
		{MetaId: dataceen.Ptr(newIDs[1]), IsActive: dataceen.Ptr(false), ExternalReference: dataceen.Ptr("bulk-b-updated")},
	}
	updRes, err := api.Customer.UpdateBulk(ctx, updates)
	if err != nil {
		return fmt.Errorf("UpdateBulk: %w", err)
	}
	fmt.Printf("     UpdateBulk: ResponseCode=%d, %d rows updated\n",
		updRes.ResponseCode, len(updRes.Result))
	return nil
}
