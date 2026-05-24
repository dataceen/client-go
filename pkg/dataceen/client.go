package dataceen

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Client is the low-level Dataceen client. It mirrors the HTTP/token surface
// of the C# DataceenClient. Typed per-entity methods (FindCustomer, etc.)
// are produced by the codegen in Phase 3.
type Client struct {
	cfg       Config
	opts      clientOptions
	transport *httpTransport

	// subscriptionCounter generates unique IDs for each Subscription this
	// Client creates. Atomic so concurrent CreateSubscription calls are safe.
	subscriptionCounter int64
}

// NewClient builds a Client. If WithTokenProvider isn't supplied, the default
// MSAL token provider is constructed from cfg's TenantID/ClientID/ClientSecret.
//
//	client, err := dataceen.NewClient(cfg, dataceen.WithLogger(myLogger))
func NewClient(cfg Config, opts ...Option) (*Client, error) {
	resolved := defaultOptions()
	for _, o := range opts {
		o(&resolved)
	}

	if resolved.tokenProvider == nil {
		tp, err := NewMSALTokenProvider(cfg)
		if err != nil {
			return nil, err
		}
		resolved.tokenProvider = tp
	}

	c := &Client{
		cfg:  cfg,
		opts: resolved,
	}
	c.transport = newHTTPTransport(cfg, resolved)
	return c, nil
}

// Config returns a copy of the configuration the client was built with.
// Useful for the subscription path (Phase 4) which needs Domain/Model/Scope.
func (c *Client) Config() Config { return c.cfg }

// ExecuteOptions tunes a single ExecuteQuery / ExecuteMutation call.
type ExecuteOptions struct {
	// RetryOnTransient overrides the default. Defaults: query=true,
	// mutation=false. Use the helpers WithRetry/WithoutRetry for clarity.
	RetryOnTransient *bool
}

// WithRetry forces transient retries on for this call.
func WithRetry() ExecuteOptions {
	v := true
	return ExecuteOptions{RetryOnTransient: &v}
}

// WithoutRetry forces transient retries off for this call.
func WithoutRetry() ExecuteOptions {
	v := false
	return ExecuteOptions{RetryOnTransient: &v}
}

// ExecuteQuery sends a GraphQL query and decodes data[operationName] into out.
// out must be a non-nil pointer. Retries on transient errors by default.
func (c *Client) ExecuteQuery(ctx context.Context, req GraphQL, out any, opts ...ExecuteOptions) error {
	retry := true
	for _, o := range opts {
		if o.RetryOnTransient != nil {
			retry = *o.RetryOnTransient
		}
	}
	body, err := c.transport.postGraphQL(ctx, req, retry)
	if err != nil {
		return err
	}
	return parseGraphQLResponse(body, req, out)
}

// ExecuteRaw runs a query that does not follow the typed Find/Search shape:
// it decodes the entire `data` object into out and skips the
// data[operationName] pickup and ResponseCode!=0 check. Use this for
// introspection (`{ __schema { ... } }`) or any query whose top-level field
// name doesn't match the operation name.
//
// Retries on transient errors by default — same as ExecuteQuery.
func (c *Client) ExecuteRaw(ctx context.Context, req GraphQL, out any, opts ...ExecuteOptions) error {
	retry := true
	for _, o := range opts {
		if o.RetryOnTransient != nil {
			retry = *o.RetryOnTransient
		}
	}
	body, err := c.transport.postGraphQL(ctx, req, retry)
	if err != nil {
		return err
	}
	return parseRawGraphQLResponse(body, req, out)
}

// ExecuteMutation sends a GraphQL mutation and decodes data[operationName]
// into out. Does NOT retry transient errors by default — mutations are not
// idempotent without server-side keys.
func (c *Client) ExecuteMutation(ctx context.Context, req GraphQL, out any, opts ...ExecuteOptions) error {
	retry := false
	for _, o := range opts {
		if o.RetryOnTransient != nil {
			retry = *o.RetryOnTransient
		}
	}
	body, err := c.transport.postGraphQL(ctx, req, retry)
	if err != nil {
		return err
	}
	return parseGraphQLResponse(body, req, out)
}

// parseGraphQLResponse:
//   1. JSON-parses the envelope
//   2. If errors[] present, returns *Error
//   3. Extracts data[operationName]
//   4. If payload has ResponseCode != 0, returns *Error
//   5. Decodes payload into out (if out != nil)
func parseGraphQLResponse(body []byte, req GraphQL, out any) error {
	var envelope struct {
		Data   map[string]json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		preview := string(body)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return newError(
			fmt.Sprintf("parse response as JSON: %v. Raw: %s", err, preview),
			-1, req.Query, err)
	}

	if len(envelope.Errors) > 0 {
		msgs := make([]string, 0, len(envelope.Errors))
		for _, e := range envelope.Errors {
			msgs = append(msgs, e.Message)
		}
		return newError("GraphQL errors: "+strings.Join(msgs, "; "), -1, req.Query, nil)
	}

	payload, ok := envelope.Data[req.OperationName]
	if !ok || len(payload) == 0 || string(payload) == "null" {
		preview := string(body)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return newError(
			fmt.Sprintf("response does not contain data.%s. Raw: %s", req.OperationName, preview),
			-1, req.Query, nil)
	}

	// Sniff for ResponseCode != 0 before decoding into the caller's type.
	var coded struct {
		ResponseCode *int   `json:"ResponseCode"`
		Message      string `json:"Message"`
	}
	_ = json.Unmarshal(payload, &coded) // ignore: payload may be a non-object
	if coded.ResponseCode != nil && *coded.ResponseCode != 0 {
		msg := coded.Message
		if msg == "" {
			msg = fmt.Sprintf("ResponseCode=%d", *coded.ResponseCode)
		}
		return newError(msg, *coded.ResponseCode, req.Query, nil)
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return newError("decode payload: "+err.Error(), -1, req.Query, err)
	}
	return nil
}

// parseRawGraphQLResponse decodes the entire `data` envelope into out.
// Unlike parseGraphQLResponse it does NOT pick by operationName and does NOT
// inspect ResponseCode — used for introspection and other "raw" queries.
func parseRawGraphQLResponse(body []byte, req GraphQL, out any) error {
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		preview := string(body)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return newError(
			fmt.Sprintf("parse response as JSON: %v. Raw: %s", err, preview),
			-1, req.Query, err)
	}
	if len(envelope.Errors) > 0 {
		msgs := make([]string, 0, len(envelope.Errors))
		for _, e := range envelope.Errors {
			msgs = append(msgs, e.Message)
		}
		return newError("GraphQL errors: "+strings.Join(msgs, "; "), -1, req.Query, nil)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		preview := string(body)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return newError("response has no data. Raw: "+preview, -1, req.Query, nil)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return newError("decode raw payload: "+err.Error(), -1, req.Query, err)
	}
	return nil
}
