package dataceen

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	tokens := &fakeTokenProvider{}
	cfg := Config{
		APIURL:  srv.URL,
		APIPath: "/graphql",
		Domain:  "Domain",
		Model:   "Model",
		Scope:   "Scope",
		APIScope: "api://test/.default",
		// MSAL fields are unused since we override the token provider:
		TenantID:     "t",
		ClientID:     "c",
		ClientSecret: "s",
	}
	c, err := NewClient(cfg,
		WithLogger(SilentLogger),
		WithHTTPClient(srv.Client()),
		WithTokenProvider(tokens),
		WithTransientRetryBase(0),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, body)
}

func TestExecuteQuery_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{
		  "data": {
		    "FindCustomer": {
		      "Items": [{"_id":"c1","CustomerId":"cst-1"}],
		      "Cursor": "next",
		      "ResponseCode": 0
		    }
		  }
		}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	type customer struct {
		ID         string `json:"_id"`
		CustomerID string `json:"CustomerId"`
	}
	var res FindResult[customer]

	if err := c.ExecuteQuery(context.Background(), GraphQL{
		OperationName: "FindCustomer",
		Query:         "q",
	}, &res); err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("Items=%d, want 1", len(res.Items))
	}
	if res.Items[0].ID != "c1" {
		t.Fatalf("Items[0].ID=%q, want c1", res.Items[0].ID)
	}
	if res.Cursor != "next" {
		t.Fatalf("Cursor=%q, want next", res.Cursor)
	}
}

func TestExecuteQuery_GraphQLErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"errors":[{"message":"Field not found: bogus"}]}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	err := c.ExecuteQuery(context.Background(), GraphQL{
		OperationName: "FindCustomer",
		Query:         "q",
	}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Field not found") {
		t.Fatalf("err = %v, want substring 'Field not found'", err)
	}
}

func TestExecuteQuery_ResponseCodeNonZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"data":{"FindCustomer":{"Items":[],"ResponseCode":42,"Message":"Forbidden field"}}}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	err := c.ExecuteQuery(context.Background(), GraphQL{
		OperationName: "FindCustomer",
		Query:         "q",
	}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var de *Error
	if !errors.As(err, &de) {
		t.Fatalf("err type=%T, want *Error", err)
	}
	if de.ResponseCode != 42 {
		t.Fatalf("ResponseCode=%d, want 42", de.ResponseCode)
	}
	if !strings.Contains(de.Message, "Forbidden field") {
		t.Fatalf("Message=%q, want contains 'Forbidden field'", de.Message)
	}
}

func TestExecuteQuery_MissingOperation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"data":{}}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	err := c.ExecuteQuery(context.Background(), GraphQL{
		OperationName: "FindCustomer",
		Query:         "q",
	}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "does not contain data.FindCustomer") {
		t.Fatalf("err = %v, want substring 'does not contain data.FindCustomer'", err)
	}
}

func TestExecuteMutation_NoRetryDefault(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	err := c.ExecuteMutation(context.Background(), GraphQL{
		OperationName: "CreateCustomer",
		Query:         "mutation { CreateCustomer { Result } }",
	}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("calls=%d, want 1 (mutations don't retry)", calls)
	}
}
