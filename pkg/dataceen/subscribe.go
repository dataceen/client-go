package dataceen

import (
	"context"
	"strconv"
	"sync/atomic"

	"github.com/dataceen/client-go/pkg/dataceen/subscriptions"
)

// loggerAdapter wires dataceen.Logger into subscriptions.Logger. The
// subscriptions package uses string log levels to avoid an import cycle.
type loggerAdapter struct{ inner Logger }

func (a loggerAdapter) Log(level, message string, err error) {
	a.inner.Log(LogLevel(level), message, err)
}

// CreateSubscription builds a Subscription handle pre-populated with the
// client's domain/model/scope. Callers register handlers on the returned
// handle, then pass it to Subscribe.
func (c *Client) CreateSubscription(req subscriptions.Request) *subscriptions.Subscription {
	if req.Domain == "" {
		req.Domain = c.cfg.Domain
	}
	if req.Model == "" {
		req.Model = c.cfg.Model
	}
	if req.Scope == "" {
		req.Scope = c.cfg.Scope
	}
	id := "sub-" + strconv.FormatInt(atomic.AddInt64(&c.subscriptionCounter, 1), 10)
	return subscriptions.NewSubscription(id, req)
}

// Subscribe runs the subscription until ctx is canceled or reconnects are
// exhausted. Returns nil for a clean shutdown (ctx canceled, server-closed
// stream); returns the underlying error otherwise.
func (c *Client) Subscribe(ctx context.Context, sub *subscriptions.Subscription) error {
	return subscriptions.Run(ctx, sub, subscriptions.RunOptions{
		SubscriptionURL:      c.cfg.SubscriptionURL,
		SubscriptionScope:    c.cfg.SubscriptionScope,
		Tokens:               c.opts.tokenProvider,
		Logger:               loggerAdapter{inner: c.opts.logger},
		SlowHandlerThreshold: c.opts.slowHandlerThreshold,
	})
}
