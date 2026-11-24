// Package personctx carries the signed-in person a call is made for on a
// context. It is its own package so that the client (GraphQL calls) and the
// subscriptions package (gRPC streams) read the same value.
package personctx

import (
	"context"
	"strings"
)

type key struct{}

// With returns a context that carries a source of the person's token. A nil
// source returns the context as it was.
func With(ctx context.Context, source func() string) context.Context {
	if source == nil {
		return ctx
	}
	return context.WithValue(ctx, key{}, source)
}

// From returns the person's token a context carries, asked from its source
// now, or "".
func From(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	source, _ := ctx.Value(key{}).(func() string)
	if source == nil {
		return ""
	}
	return strings.TrimSpace(source())
}
