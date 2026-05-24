// Package subscriptions implements the Dataceen subscription stream client.
// It opens a gRPC stream to the subscription service, manages baseload
// semantics, dispatches typed and low-level handlers, and reconnects on
// transient errors.
//
// Lifetime pattern (mirrors C# Subscription and the TS port):
//
//	sub := dataceen.CreateSubscription(req)
//	subscriptions.On(sub, "Customer", func(evt *subscriptions.Event[Customer]) error { ... })
//	sub.OnRaw(func(evt *subscriptions.LowLevelEvent) error { ... })
//	err := dataceen.Subscribe(ctx, sub)  // blocks until ctx cancel or fatal error
package subscriptions

import "encoding/json"

// StartMode mirrors the proto enum. Wire format: SCREAMING_SNAKE strings —
// but on the gRPC layer we use the typed proto enum from pkg/dataceenevent.
type StartMode string

const (
	StartPositionBeginning StartMode = "POSITION_BEGINNING"
	StartPositionEnd       StartMode = "POSITION_END"
	StartPositionExact     StartMode = "POSITION_EXACT"
	StartPositionTime      StartMode = "POSITION_TIME"
)

// MessageType matches the proto enum. KEEPALIVE messages are filtered before
// reaching handlers.
type MessageType string

const (
	MessageNormal    MessageType = "NORMAL"
	MessageKeepalive MessageType = "KEEPALIVE"
)

// EventType is the lifecycle / synthetic event type.
//
// Real events on the wire: SUBSCRIPTION_EVENT, BASELOAD_EVENT, BASELOAD_STEP_1,
// BASELOAD_STEP_1_ALL, BASELOAD_END, POSITION_AT_START_PASSED.
//
// Synthetic events injected by the manager: CONNECTION_LOST,
// CONNECTION_RECONNECTED.
type EventType string

const (
	EventSubscription          EventType = "SUBSCRIPTION_EVENT"
	EventBaseload              EventType = "BASELOAD_EVENT"
	EventBaseloadStep1         EventType = "BASELOAD_STEP_1"
	EventBaseloadStep1All      EventType = "BASELOAD_STEP_1_ALL"
	EventBaseloadEnd           EventType = "BASELOAD_END"
	EventPositionAtStartPassed EventType = "POSITION_AT_START_PASSED"
	EventConnectionLost        EventType = "CONNECTION_LOST"
	EventConnectionReconnected EventType = "CONNECTION_RECONNECTED"
)

// OperationType mirrors the proto enum.
type OperationType string

const (
	OpCreate OperationType = "CREATE"
	OpUpdate OperationType = "UPDATE"
	OpDelete OperationType = "DELETE"
)

// ObjectType mirrors the proto enum.
type ObjectType string

const (
	ObjectNode         ObjectType = "NODE"
	ObjectRelationship ObjectType = "RELATIONSHIP"
)

// Request opens a subscription. Mirrors the proto SubscriptionRequest with
// camelCased Go names; the gRPC layer translates field names back to the
// lowercase wire format.
type Request struct {
	// Topics — the entity types to receive events for. Required.
	Topics []string
	// BaseloadTopics — entity types to baseload before live events begin.
	// Optional; if empty no baseload is performed for any topic.
	BaseloadTopics []string
	// IDs — restrict the subscription to specific entity IDs. Optional.
	IDs []string
	// StartMode — where in the stream to start. Defaults to POSITION_BEGINNING.
	StartMode StartMode
	// Position — the wire-position string from a prior subscription. Used with
	// StartMode=POSITION_EXACT to resume.
	Position string
	// IncludeDelta — server includes the JSON delta of changes per event.
	IncludeDelta bool
	// IncludeComplete — server includes the full entity JSON per event.
	IncludeComplete bool
	// BaseloadContinueObjectType — used when continuing a baseload from a
	// prior position. Optional.
	BaseloadContinueObjectType string
	// PositionBeforeBaseload — server-side replay anchor. Optional.
	PositionBeforeBaseload string

	// Domain/Model/Scope — populated by Client.CreateSubscription from the
	// client config; callers can override per-subscription if needed.
	Domain string
	Model  string
	Scope  string
}

// LowLevelEvent is a flattened event with strongly-typed enum fields and
// the wire-format `complete` payload as a JSON string. Use this when you want
// to handle multiple topics with one handler or when typed unmarshaling
// would be awkward.
type LowLevelEvent struct {
	Position      string
	EventDateTime string // ISO 8601 UTC, "" for synthetic events
	OperationType OperationType
	ClientID      string
	// Domain identifies the source domain of the event. Added in proto
	// schema update 2026-05; older servers may leave it empty.
	Domain        string
	Topic         string
	ID            string
	FromID        string
	ToID          string
	Delta         string // JSON string per decisions/porting-guide
	Complete      string // JSON string; empty for synthetic / delta-only events
	EventType     EventType
	ObjectType    ObjectType
	Model         string
}

// Event is the typed counterpart of LowLevelEvent. Complete is parsed into T
// when the server included it, or zero when absent (synthetic events have
// no payload). Inspect EventType to disambiguate.
type Event[T any] struct {
	Position      string
	EventDateTime string
	OperationType OperationType
	ClientID      string
	// Domain identifies the source domain of the event. Added in proto
	// schema update 2026-05; older servers may leave it empty.
	Domain        string
	Topic         string
	ID            string
	FromID        string
	ToID          string
	Delta         string
	// Complete carries the parsed entity. Use ComplettePresent to tell apart
	// a zero-valued entity from "no payload".
	Complete         T
	CompletePresent bool
	EventType        EventType
	ObjectType       ObjectType
	Model            string
}

// RawHandler is invoked for every (non-KEEPALIVE) event the stream produces,
// regardless of topic.
type RawHandler func(evt *LowLevelEvent) error

// internalHandler is the per-topic handler shape the manager dispatches to.
// Typed handlers wrap their unmarshal step before delegating here.
type internalHandler func(evt *LowLevelEvent) error

// unmarshalComplete is a small helper used by typed handlers: parses the JSON
// payload into out. Returns ok=false (not an error) if Complete is empty.
func unmarshalComplete(complete string, out any) (bool, error) {
	if complete == "" {
		return false, nil
	}
	if err := json.Unmarshal([]byte(complete), out); err != nil {
		return false, err
	}
	return true, nil
}
