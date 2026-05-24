package main

import (
	"context"
	"fmt"
	"time"

	"github.com/dataceen/client-go/pkg/dataceen"
	"github.com/dataceen/client-go/pkg/dataceen/dsl"
	"github.com/dataceen/client-go/pkg/generated"
)

// runDestructive demonstrates Create → Update → Delete on a brand-new
// Customer using a unique CustomerId so the example doesn't clash with
// existing rows. Cleans up by deleting the row at the end (deferred so
// the cleanup runs even when an intermediate step fails).
func runDestructive(ctx context.Context, api *generated.AllClient) error {
	customerID := fmt.Sprintf("cst-go-example-%d", time.Now().Unix())
	fmt.Println("\n--- 7. CREATE / UPDATE / DELETE (destructive) ---")
	fmt.Printf("    using unique CustomerId=%s\n", customerID)

	// Create
	fmt.Println("\n  7a. Create")
	createRes, err := api.Customer.Create(ctx, &generated.CustomerCreate{
		CustomerId:        customerID,
		ExternalReference: "go-example",
		IsActive:          true,
		CreatedAt:         time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	fmt.Printf("     ResponseCode=%d, Result=%v, ResponseTime=%dms\n",
		createRes.ResponseCode, createRes.Result, createRes.ResponseTime)
	if len(createRes.Result) == 0 {
		return fmt.Errorf("create returned no _id")
	}
	newID := createRes.Result[0]

	// Defer cleanup — runs even if Update or the final find fail.
	defer func() {
		fmt.Println("\n  7d. Cleanup: Delete (deferred)")
		delRes, err := api.Customer.Delete(ctx, dsl.F("_id").Eq(newID))
		if err != nil {
			fmt.Printf("     ! delete failed: %v\n", err)
			return
		}
		fmt.Printf("     ResponseCode=%d, Result=%v\n", delRes.ResponseCode, delRes.Result)
	}()

	// Verify the row exists
	fmt.Println("\n  7b. Verify created row via Find")
	findRes, err := api.Customer.Find(ctx, generated.FindCustomerParams{
		Filter: dsl.F("_id").Eq(newID),
		Fields: generated.CustomerFields{
			MetaId:            true,
			CustomerId:        true,
			ExternalReference: true,
			IsActive:          true,
		},
	})
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	if len(findRes.Items) != 1 {
		return fmt.Errorf("expected 1 item after Create, got %d", len(findRes.Items))
	}
	c := findRes.Items[0]
	fmt.Printf("     %s (ExternalReference=%q, IsActive=%t)\n",
		c.CustomerId, c.ExternalReference, c.IsActive)

	// Update — flip IsActive and overwrite ExternalReference
	fmt.Println("\n  7c. Update IsActive=false, ExternalReference=updated")
	upd := &generated.CustomerUpdate{
		IsActive:          dataceen.Ptr(false),
		ExternalReference: dataceen.Ptr("updated-by-go-example"),
	}
	updRes, err := api.Customer.Update(ctx, dsl.F("_id").Eq(newID), upd)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	fmt.Printf("     ResponseCode=%d, Result=%v\n", updRes.ResponseCode, updRes.Result)

	// Re-fetch to confirm Update took effect
	c2, err := api.Customer.FindByID(ctx, newID, generated.CustomerFields{
		CustomerId:        true,
		ExternalReference: true,
		IsActive:          true,
	})
	if err != nil {
		return err
	}
	if c2 == nil {
		return fmt.Errorf("post-update: row vanished")
	}
	fmt.Printf("     post-update: %s (ExternalReference=%q, IsActive=%t)\n",
		c2.CustomerId, c2.ExternalReference, c2.IsActive)
	if c2.IsActive {
		return fmt.Errorf("Update didn't take effect — IsActive still true")
	}

	// Compound-create demo: link the test Customer to an existing Order via
	// CustomerPlacedOrder. Don't fail the whole demo if this step has
	// trouble — the CRUD part above already proved the main surface.
	if err := runRelationCreate(ctx, api, newID); err != nil {
		fmt.Printf("     ! relation step skipped: %v\n", err)
	}
	return nil
}

func runRelationCreate(ctx context.Context, api *generated.AllClient, customerID string) error {
	fmt.Println("\n  7e. Compound-create CustomerPlacedOrder relation")

	// Find any existing Order to link against.
	orders, err := api.Order.Find(ctx, generated.FindOrderParams{
		Size:   1,
		Fields: generated.OrderFields{MetaId: true, OrderId: true},
	})
	if err != nil {
		return fmt.Errorf("locate Order: %w", err)
	}
	if len(orders.Items) == 0 {
		return fmt.Errorf("no Orders found to link against")
	}
	orderID := orders.Items[0].MetaId
	fmt.Printf("     linking Customer (id=%s) to Order (id=%s, OrderId=%s)\n",
		customerID, orderID, orders.Items[0].OrderId)

	res, err := api.CustomerPlacedOrder.Create(ctx, &generated.CustomerPlacedOrderRelationCreate{
		FromID:   customerID,
		ToID:     orderID,
		PlacedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("CreateCustomerPlacedOrder: %w", err)
	}
	fmt.Printf("     ResponseCode=%d, Result=%v\n", res.ResponseCode, res.Result)
	if len(res.Result) == 0 {
		return fmt.Errorf("relation create returned no _id")
	}

	// Cleanup: delete the relation we just created.
	relID := res.Result[0]
	delRes, err := api.CustomerPlacedOrder.Delete(ctx, dsl.F("_id").Eq(relID))
	if err != nil {
		return fmt.Errorf("delete relation: %w", err)
	}
	fmt.Printf("     cleanup ResponseCode=%d\n", delRes.ResponseCode)
	return nil
}
