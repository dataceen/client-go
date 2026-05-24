package prototypes

// Hand-rolled subset of the Customer/Order/CustomerPlacedOrder types used by
// the prototypes. The real generator (Phase 3) emits the full schema.
//
// JSON tags follow Dataceen wire format (PascalCase + leading-underscore
// metadata fields).

// Customer is the read-model.
type Customer struct {
	ID                  string                 `json:"_id,omitempty"`
	CustomerID          string                 `json:"CustomerId"`
	ExternalReference   string                 `json:"ExternalReference,omitempty"`
	Status              string                 `json:"Status,omitempty"`
	IsActive            bool                   `json:"IsActive"`
	CustomerPlacedOrder []CustomerPlacedOrder  `json:"CustomerPlacedOrder,omitempty"`
}

// Order is a related node.
type Order struct {
	ID      string `json:"_id,omitempty"`
	OrderID string `json:"OrderId"`
	Total   string `json:"Total,omitempty"` // Decimal → string per decisions.md #29
}

// CustomerPlacedOrder is the relationship Customer↔Order with a target
// reference and edge metadata.
type CustomerPlacedOrder struct {
	FromID     string `json:"_fromId"`
	ToID       string `json:"_toId"`
	FromLabel  string `json:"_fromLabel,omitempty"`
	ToLabel    string `json:"_toLabel,omitempty"`
	PlacedAt   string `json:"PlacedAt,omitempty"` // DateTimeOffset → string
	Order      Order  `json:"Order"`
}
