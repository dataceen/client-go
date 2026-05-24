package main

import (
	"context"
	"fmt"

	"github.com/dataceen/client-go/pkg/dataceen/dsl"
	"github.com/dataceen/client-go/pkg/generated"
)

// runSearch demonstrates the Search query plus a single Bucket_Terms
// aggregation on IsActive. Read-only.
func runSearch(ctx context.Context, api *generated.AllClient) error {
	fmt.Println("\n--- 8. Search with aggregations ---")

	res, err := api.Customer.Search(ctx, generated.SearchCustomerParams{
		Size:   5,
		Filter: dsl.F("CustomerId").Like("cst"),
		Fields: generated.CustomerFields{
			MetaId:     true,
			CustomerId: true,
			IsActive:   true,
		},
		Aggregations: []generated.SearchCustomerAggregation{
			{
				IsActive: &generated.SearchCustomerAggregationDetails{
					Name: "by-active",
					Type: "Bucket_Terms",
					Size: 10,
				},
			},
		},
		AggregationDepth: 2,
	})
	if err != nil {
		return err
	}

	fmt.Printf("  search returned %d Items, %d Aggregations\n",
		len(res.Items), len(res.Aggregations))
	for i, c := range res.Items {
		fmt.Printf("    Item %d: %s (IsActive=%t)\n", i+1, c.CustomerId, c.IsActive)
	}

	aggs, err := dsl.DecodeAggregations(res.Aggregations)
	if err != nil {
		return fmt.Errorf("decode aggregations: %w", err)
	}
	for _, a := range aggs {
		fmt.Printf("  Agg %q (%d buckets):\n", a.Name, len(a.Buckets))
		for _, bucket := range a.Buckets {
			active, ok := bucket.KeyAsBool()
			if ok {
				fmt.Printf("    IsActive=%t  Count=%d\n", active, bucket.Count)
			} else {
				fmt.Printf("    Key=%s  Count=%d\n", string(bucket.Key), bucket.Count)
			}
		}
	}
	return nil
}
