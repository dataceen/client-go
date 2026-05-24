package main

import (
	"context"
	"fmt"

	"github.com/dataceen/client-go/pkg/dataceen/dsl"
	"github.com/dataceen/client-go/pkg/generated"
)

// runFinds exercises the read paths: Find, FindByID, like-filter,
// pagination, ordered fields. Read-only — safe in any environment.
func runFinds(ctx context.Context, api *generated.AllClient) error {
	if err := scenarioFindFirstPage(ctx, api); err != nil {
		return err
	}
	if err := scenarioFindByID(ctx, api); err != nil {
		return err
	}
	if err := scenarioLikeFilter(ctx, api); err != nil {
		return err
	}
	if err := scenarioOrdered(ctx, api); err != nil {
		return err
	}
	if err := scenarioPagination(ctx, api); err != nil {
		return err
	}
	return nil
}

func scenarioFindFirstPage(ctx context.Context, api *generated.AllClient) error {
	fmt.Println("\n--- 1. Find first 5 customers ---")
	res, err := api.Customer.Find(ctx, generated.FindCustomerParams{
		Size: 5,
		Fields: generated.CustomerFields{
			MetaId:     true,
			CustomerId: true,
			IsActive:   true,
		},
	})
	if err != nil {
		return err
	}
	for i, c := range res.Items {
		fmt.Printf("  %d. %s  (id=%s, IsActive=%t)\n", i+1, c.CustomerId, c.MetaId, c.IsActive)
	}
	fmt.Printf("  cursor=%s\n", res.Cursor)
	return nil
}

func scenarioFindByID(ctx context.Context, api *generated.AllClient) error {
	fmt.Println("\n--- 2. FindByID(\"Customer_Example_00000\") ---")
	c, err := api.Customer.FindByID(ctx, "Customer_Example_00000", generated.CustomerFields{
		MetaId:     true,
		CustomerId: true,
		IsActive:   true,
	})
	if err != nil {
		return err
	}
	if c == nil {
		fmt.Println("  not found")
		return nil
	}
	fmt.Printf("  found: %s  (id=%s, IsActive=%t)\n", c.CustomerId, c.MetaId, c.IsActive)
	return nil
}

func scenarioLikeFilter(ctx context.Context, api *generated.AllClient) error {
	fmt.Println("\n--- 3. Find with `like` filter (Elasticsearch token-match) ---")
	fmt.Println("    Note: never use wildcards (% *) or delimiters (- . space) in `like` values.")
	// Using generated.CustomerField.CustomerId instead of the literal
	// "CustomerId" gives compile-time safety against typos.
	res, err := api.Customer.Find(ctx, generated.FindCustomerParams{
		Size:   100,
		Filter: dsl.F(generated.CustomerField.CustomerId).Like("cst"),
		Fields: generated.CustomerFields{CustomerId: true},
	})
	if err != nil {
		return err
	}
	fmt.Printf("  matched %d customers (token \"cst\")\n", len(res.Items))
	if len(res.Items) > 0 {
		fmt.Printf("  first three: %s, %s, %s\n",
			res.Items[0].CustomerId,
			pickOr(res.Items, 1),
			pickOr(res.Items, 2))
	}
	return nil
}

func scenarioOrdered(ctx context.Context, api *generated.AllClient) error {
	fmt.Println("\n--- 4. Find with order_by CustomerId Descending ---")
	res, err := api.Customer.Find(ctx, generated.FindCustomerParams{
		Size:    3,
		OrderBy: dsl.OrderBy().Desc(generated.CustomerField.CustomerId),
		Fields:  generated.CustomerFields{CustomerId: true},
	})
	if err != nil {
		return err
	}
	for i, c := range res.Items {
		fmt.Printf("  %d. %s\n", i+1, c.CustomerId)
	}
	return nil
}

func scenarioPagination(ctx context.Context, api *generated.AllClient) error {
	fmt.Println("\n--- 5. Pagination: 3 pages of size 5 via cursor ---")
	cursor := "null"
	total := 0
	for page := 1; page <= 3; page++ {
		res, err := api.Customer.Find(ctx, generated.FindCustomerParams{
			Size:   5,
			Cursor: cursor,
			Fields: generated.CustomerFields{CustomerId: true},
		})
		if err != nil {
			return err
		}
		fmt.Printf("  page %d: %d items (last=%s)\n", page, len(res.Items),
			pickOr(res.Items, len(res.Items)-1))
		total += len(res.Items)
		if res.Cursor == "" || res.Cursor == "null" {
			break
		}
		cursor = res.Cursor
	}
	fmt.Printf("  total across pages: %d\n", total)
	return nil
}

func pickOr(items []generated.Customer, idx int) string {
	if idx < 0 || idx >= len(items) {
		return "(none)"
	}
	return items[idx].CustomerId
}
