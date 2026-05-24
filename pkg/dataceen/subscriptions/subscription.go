package subscriptions

import (
	"sync"
)

// Subscription is the long-lived handle: a request, a set of handlers, plus
// state the manager updates as events arrive (lastPosition, baseload tracking).
//
// Concurrency: state fields are guarded by the embedded mutex. Handlers may
// be registered before Subscribe is called; registering after the stream has
// started is allowed but new handlers won't see events that already passed.
type Subscription struct {
	ID      string
	Request Request

	mu sync.Mutex

	lastPosition  string
	isBaseLoading bool
	baseloadLabel string
	retryCount    int

	topicHandlers map[string]internalHandler
	rawHandlers   []RawHandler
}

// NewSubscription builds an empty subscription. Most callers use
// Client.CreateSubscription, which fills in domain/model/scope from config.
func NewSubscription(id string, req Request) *Subscription {
	return &Subscription{
		ID:            id,
		Request:       req,
		topicHandlers: map[string]internalHandler{},
	}
}

// OnRaw registers a low-level handler that fires for every (non-KEEPALIVE)
// event. Multiple raw handlers are allowed and run in registration order.
func (s *Subscription) OnRaw(h RawHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rawHandlers = append(s.rawHandlers, h)
}

// Off unregisters the handler for a topic.
func (s *Subscription) Off(topic string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.topicHandlers, topic)
}

// LastPosition returns the most recent event position the stream has reached.
// Useful for resuming after a clean shutdown.
func (s *Subscription) LastPosition() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastPosition
}

// IsBaseLoading reports whether the server is still emitting BASELOAD_EVENT
// items. False once BASELOAD_STEP_1 or BASELOAD_END is observed.
func (s *Subscription) IsBaseLoading() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isBaseLoading
}

// BaseloadLabel returns the topic name currently being baseloaded.
func (s *Subscription) BaseloadLabel() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.baseloadLabel
}

// snapshotHandlers returns copies of the handler maps so the manager can
// dispatch without holding the subscription lock while user code runs.
func (s *Subscription) snapshotHandlers() (map[string]internalHandler, []RawHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	topics := make(map[string]internalHandler, len(s.topicHandlers))
	for k, v := range s.topicHandlers {
		topics[k] = v
	}
	raws := append([]RawHandler{}, s.rawHandlers...)
	return topics, raws
}

func (s *Subscription) setHandler(topic string, h internalHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.topicHandlers[topic] = h
}

func (s *Subscription) updateState(eventType EventType, topic, position string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch eventType {
	case EventBaseloadEnd, EventBaseloadStep1, EventBaseloadStep1All:
		s.isBaseLoading = false
	case EventBaseload:
		s.isBaseLoading = true
		s.baseloadLabel = topic
		if position != "" {
			s.lastPosition = position
		}
	default:
		s.isBaseLoading = false
		if position != "" {
			s.lastPosition = position
		}
	}
}

func (s *Subscription) incRetry() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retryCount++
	return s.retryCount
}

func (s *Subscription) resetRetry() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	wasRetrying := s.retryCount > 0
	s.retryCount = 0
	return wasRetrying
}

// On registers a typed handler for the given topic. The Complete field of
// the event is JSON-unmarshaled into a T value before the handler runs;
// when the server sent no complete payload, CompletePresent is false and
// Complete is the zero value. Synthetic events (CONNECTION_LOST,
// CONNECTION_RECONNECTED) reach this handler with EventType set accordingly.
//
// Implemented as a free function because Go does not allow method type
// parameters.
func On[T any](sub *Subscription, topic string, handler func(evt *Event[T]) error) {
	wrapper := func(low *LowLevelEvent) error {
		var c T
		present := false
		if low.Complete != "" {
			ok, err := unmarshalComplete(low.Complete, &c)
			if err != nil {
				return err
			}
			present = ok
		}
		evt := &Event[T]{
			Position:        low.Position,
			EventDateTime:   low.EventDateTime,
			OperationType:   low.OperationType,
			ClientID:        low.ClientID,
			Domain:          low.Domain,
			Topic:           low.Topic,
			ID:              low.ID,
			FromID:          low.FromID,
			ToID:            low.ToID,
			Delta:           low.Delta,
			Complete:        c,
			CompletePresent: present,
			EventType:       low.EventType,
			ObjectType:      low.ObjectType,
			Model:           low.Model,
		}
		return handler(evt)
	}
	sub.setHandler(topic, wrapper)
}
