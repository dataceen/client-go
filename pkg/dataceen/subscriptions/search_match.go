package subscriptions

import (
	"encoding/json"
	"fmt"
)

// SearchMatchTopic is the topic of a saved search's match events
// (docs/search-distance-sort-plan.md section 13.8).
const SearchMatchTopic = "SearchMatch"

// SearchMatchEvent is a saved search's new matches: the records' ids (up to
// 100) and how many matched since the last run, among the changes indexed
// after From and up to To (ISO 8601, UTC).
type SearchMatchEvent struct {
	SavedSearchID string   `json:"savedSearchId"`
	Name          string   `json:"name"`
	Subject       string   `json:"subject"`
	Scope         string   `json:"scope"`
	Search        string   `json:"search"`
	IDs           []string `json:"ids"`
	Total         int64    `json:"total"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	Position      string   `json:"-"`
}

// ParseSearchMatch reads a match event's payload as the server writes it.
func ParseSearchMatch(complete string) (*SearchMatchEvent, error) {
	m := &SearchMatchEvent{}
	if complete == "" {
		return m, nil
	}
	if err := json.Unmarshal([]byte(complete), m); err != nil {
		return nil, fmt.Errorf("parse a SearchMatch event: %w", err)
	}
	return m, nil
}

// OnSearchMatch receives your saved searches' matches and asks for their
// topic: only your application's matches, on this subscription's scope, ever
// reach it. Register it before Subscribe.
func (s *Subscription) OnSearchMatch(handler func(m *SearchMatchEvent) error) {
	s.mu.Lock()
	present := false
	for _, t := range s.Request.Topics {
		if t == SearchMatchTopic {
			present = true
		}
	}
	if !present {
		s.Request.Topics = append(s.Request.Topics, SearchMatchTopic)
	}
	s.mu.Unlock()
	s.setHandler(SearchMatchTopic, func(low *LowLevelEvent) error {
		// Connection notices reach every topic handler without a payload.
		if low.Complete == "" {
			return nil
		}
		m, err := ParseSearchMatch(low.Complete)
		if err != nil {
			return err
		}
		m.Position = low.Position
		return handler(m)
	})
}
