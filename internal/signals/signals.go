// Package signals holds the outside state the scheduler reacts to: the last
// value seen on MQTT topics that inserts depend on, and when presence was
// last detected. The MQTT service, the admin API and the browser's touch
// listener write it; the scheduler reads it and wakes up on changes.
package signals

import (
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu       sync.Mutex
	values   map[string]string
	presence time.Time
	source   string
	changes  chan struct{}
}

func New() *Store {
	return &Store{values: map[string]string{}, changes: make(chan struct{}, 1)}
}

// Set records the latest value of topic.
func (s *Store) Set(topic, value string) {
	if s == nil {
		return
	}
	topic = strings.TrimSpace(topic)
	s.mu.Lock()
	old, seen := s.values[topic]
	s.values[topic] = value
	s.mu.Unlock()
	if !seen || old != value {
		s.notify()
	}
}

// Value returns the latest value of topic and whether it was seen at all.
func (s *Store) Value(topic string) (string, bool) {
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[strings.TrimSpace(topic)]
	return value, ok
}

// NotePresence records a presence detection from source ("mqtt", "api",
// "touch", ...).
func (s *Store) NotePresence(at time.Time, source string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	wasOld := s.presence.IsZero() || at.Sub(s.presence) > time.Minute
	s.presence = at
	s.source = source
	s.mu.Unlock()
	if wasOld {
		s.notify()
	}
}

// LastPresence returns the last detection and its source.
func (s *Store) LastPresence() (time.Time, string) {
	if s == nil {
		return time.Time{}, ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.presence, s.source
}

// Changes is signalled (coalesced) whenever a value or presence changes.
func (s *Store) Changes() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.changes
}

func (s *Store) notify() {
	select {
	case s.changes <- struct{}{}:
	default:
	}
}
