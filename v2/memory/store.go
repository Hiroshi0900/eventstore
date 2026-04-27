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

// PersistEvent appends a single event. expectedVersion==0 indicates "first
// write" and the operation fails with ErrDuplicateAggregate if events already
// exist for the aggregate. expectedVersion>0 has no effect (optimistic locking
// is performed only by PersistEventAndSnapshot).
// Duplicate (aggregateID, seqNr) entries are rejected.
func (s *Store) PersistEvent(_ context.Context, ev *es.EventEnvelope, expectedVersion uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := ev.AggregateID.AsString()

	if expectedVersion == 0 && len(s.events[key]) > 0 {
		return es.NewDuplicateAggregateError(key)
	}

	for _, existing := range s.events[key] {
		if existing.SeqNr == ev.SeqNr {
			return es.NewDuplicateAggregateError(key)
		}
	}

	s.events[key] = append(s.events[key], ev)
	return nil
}

// PersistEventAndSnapshot appends an event AND updates the snapshot atomically.
// The snapshot's Version field acts as the optimistic lock: the caller is
// expected to pass Version = currentSnapshotVersion + 1. The operation fails
// with ErrOptimisticLock if the stored snapshot's Version is not snap.Version - 1.
// (For initial writes there is no stored snapshot; snap.Version must be 1.)
func (s *Store) PersistEventAndSnapshot(
	_ context.Context,
	ev *es.EventEnvelope,
	snap *es.SnapshotEnvelope,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := ev.AggregateID.AsString()

	expected := snap.Version - 1
	current := uint64(0)
	if existing := s.snapshots[key]; existing != nil {
		current = existing.Version
	}
	if current != expected {
		return es.NewOptimisticLockError(key, expected, current)
	}

	for _, existing := range s.events[key] {
		if existing.SeqNr == ev.SeqNr {
			return es.NewDuplicateAggregateError(key)
		}
	}

	s.events[key] = append(s.events[key], ev)
	s.snapshots[key] = snap
	return nil
}
