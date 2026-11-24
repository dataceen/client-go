package dataceen

import (
	"context"
	"strings"

	"github.com/dataceen/client-go/pkg/dataceen/internal/personctx"
)

// PersonHeader is the request header that carries the signed-in person's token.
const PersonHeader = "Dataceen-Person"

// PersonMetadata is the gRPC metadata entry that carries the person's token on
// a subscription.
const PersonMetadata = "dataceen-person"

// WithPerson returns a context whose calls are made for a signed-in person:
// every GraphQL call given this context carries the person's token in the
// Dataceen-Person header, beside the application's own credentials, and the
// answer holds that person's rows.
//
// A site's server serves many people from one client, so the person rides on
// the request's context and not on the client:
//
//	ctx := dataceen.WithPerson(r.Context(), personToken)
//	orders, err := orderClient.Find(ctx, ...)
//
// The token is the one the sign-in address gave for the person and the scope.
// An empty token returns the context as it was: the application alone.
//
// A subscription run with this context is for that person too; see
// WithPersonSource for one that outlives a token.
func WithPerson(ctx context.Context, personToken string) context.Context {
	if strings.TrimSpace(personToken) == "" {
		return ctx
	}
	return personctx.With(ctx, func() string { return personToken })
}

// WithPersonSource is WithPerson with the token read each time it is needed:
// on every call, and every time a subscription's stream is opened. A
// subscription run with this context is sent the events of the person's own
// rows; its stream outlives a token and is opened again after a drop, so give
// it a function that returns the person's current token:
//
//	ctx := dataceen.WithPersonSource(ctx, session.PersonToken)
//	err := client.Subscribe(ctx, sub)
func WithPersonSource(ctx context.Context, source func() string) context.Context {
	return personctx.With(ctx, source)
}

// PersonFrom returns the person's token a context carries, or "".
func PersonFrom(ctx context.Context) string {
	return personctx.From(ctx)
}
