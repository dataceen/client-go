package dataceen

import "encoding/json"

// GraphQL result shapes.
//
// JSON tags use PascalCase to match the Dataceen wire format (the server
// returns `{ "Items": [...], "Cursor": "..." }`). Decision flagged for
// revisit if a camelCase facade is ever desired — see decisions.md #34.

// FindResult is the envelope for Find{Entity} queries.
type FindResult[T any] struct {
	Items        []T    `json:"Items"`
	Cursor       string `json:"Cursor"`
	ResponseTime int64  `json:"ResponseTime"`
	ResponseCode int    `json:"ResponseCode"`
	Message      string `json:"Message"`
}

// SearchResult is the envelope for Search{Entity} queries. Aggregations is
// populated when the request included an aggregations argument; the server
// returns it even though introspection does not advertise it (decisions.md #47).
type SearchResult[T any] struct {
	Items        []T               `json:"Items"`
	Cursor       string            `json:"Cursor"`
	Aggregations []json.RawMessage `json:"Aggregations"`
	ResponseTime int64             `json:"ResponseTime"`
	ResponseCode int               `json:"ResponseCode"`
	Message      string            `json:"Message"`
}

// QueryResult wraps an arbitrary typed Response payload.
type QueryResult[T any] struct {
	Response     T      `json:"Response"`
	ID           string `json:"_id"`
	ResponseTime int64  `json:"ResponseTime"`
	ResponseCode int    `json:"ResponseCode"`
	Message      string `json:"Message"`
}

// RequestResult is the envelope for request operations.
type RequestResult[T any] struct {
	Response     T      `json:"Response"`
	ID           string `json:"_id"`
	ResponseTime int64  `json:"ResponseTime"`
	ResponseCode int    `json:"ResponseCode"`
	Message      string `json:"Message"`
}

// MutationResult is the envelope for Create/Update/Delete (and their Bulk
// variants). Result holds the affected entity IDs.
type MutationResult struct {
	Result       []string `json:"Result"`
	ResponseTime int64    `json:"ResponseTime"`
	ResponseCode int      `json:"ResponseCode"`
	Message      string   `json:"Message"`
}

// SingleMutationResult is the trimmed envelope used by single-row mutations.
type SingleMutationResult struct {
	Result       string `json:"Result"`
	ResponseTime int64  `json:"ResponseTime"`
}

// FindByIdResult wraps a single optional item.
type FindByIdResult[T any] struct {
	Item         *T    `json:"Item,omitempty"`
	ResponseTime int64 `json:"ResponseTime"`
}

// GraphQL is the wire payload for an HTTP POST against the Dataceen GraphQL
// endpoint.
type GraphQL struct {
	OperationName string `json:"operationName"`
	Query         string `json:"query"`
}
