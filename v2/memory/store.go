// Package memory provides an in-memory EventStore implementation for testing.
package memory

import (
	"context"
	"sort"
	"sync"

	es "github.com/Hiroshi0900/eventstore/v2"
)

// Store is an in-memory EventStore implementation.
type Store struct {
	mu        sync.RWMutex
	events    map[string][]*es.EventEnvelope  // aggregateID.AsString() -> ordered events
	snapshots map[string]*es.SnapshotEnvelope // aggregateID.AsString() -> latest snapshot
}

// New creates a new in-memory store.
func New() *Store {
	return &Store{
		events:    make(map[string][]*es.EventEnvelope),
		snapshots: make(map[string]*es.SnapshotEnvelope),
	}
}

// GetLatestSnapshot returns the latest snapshot or nil.
func (s *Store) GetLatestSnapshot(_ context.Context, id es.AggregateID) (*es.SnapshotEnvelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshots[id.AsString()], nil
}

// GetEventsSince returns events with SeqNr > seqNr, ordered by SeqNr.
func (s *Store) GetEventsSince(_ context.Context, id es.AggregateID, seqNr uint64) ([]*es.EventEnvelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	src := s.events[id.AsString()]
	var out []*es.EventEnvelope
	for _, ev := range src {
		if ev.SeqNr > seqNr {
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SeqNr < out[j].SeqNr })
	return out, nil
}

// PersistEvent and PersistEventAndSnapshot are implemented in subsequent tasks.
func (s *Store) PersistEvent(_ context.Context, _ *es.EventEnvelope, _ uint64) error {
	panic("not implemented")
}
func (s *Store) PersistEventAndSnapshot(_ context.Context, _ *es.EventEnvelope, _ *es.SnapshotEnvelope) error {
	panic("not implemented")
}
