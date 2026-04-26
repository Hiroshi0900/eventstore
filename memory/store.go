// Package memory provides an in-memory EventStore implementation for testing.
package memory

import (
	"context"
	"sort"
	"sync"

	es "github.com/Hiroshi0900/eventstore"
)

// Store is an in-memory EventStore implementation for testing.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
type Store struct {
	mu        sync.RWMutex
	events    map[string][]es.Event       // aggregateId -> events
	snapshots map[string]*es.SnapshotData // aggregateId -> snapshot
	versions  map[string]uint64           // aggregateId -> version
}

// New creates a new in-memory event store.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func New() *Store {
	return &Store{
		events:    make(map[string][]es.Event),
		snapshots: make(map[string]*es.SnapshotData),
		versions:  make(map[string]uint64),
	}
}

// GetLatestSnapshotByID retrieves the latest snapshot for the given aggregate ID.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (s *Store) GetLatestSnapshotByID(_ context.Context, id es.AggregateID) (*es.SnapshotData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := id.AsString()
	snapshot, exists := s.snapshots[key]
	if !exists {
		return nil, nil
	}
	return snapshot, nil
}

// GetEventsByIDSinceSeqNr retrieves all events since the given sequence number.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (s *Store) GetEventsByIDSinceSeqNr(_ context.Context, id es.AggregateID, seqNr uint64) ([]es.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := id.AsString()
	allEvents, exists := s.events[key]
	if !exists {
		return []es.Event{}, nil
	}

	// Filter events after the given sequence number
	var result []es.Event
	for _, event := range allEvents {
		if event.GetSeqNr() > seqNr {
			result = append(result, event)
		}
	}

	// Sort by sequence number
	sort.Slice(result, func(i, j int) bool {
		return result[i].GetSeqNr() < result[j].GetSeqNr()
	})

	return result, nil
}

// PersistEvent persists a single event without a snapshot.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (s *Store) PersistEvent(_ context.Context, event es.Event, expectedVersion uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := event.GetAggregateId().AsString()

	// Duplicate check on first creation
	if expectedVersion == 0 && len(s.events[key]) > 0 {
		return es.NewDuplicateAggregateError(key)
	}

	// Version check for optimistic locking
	currentVersion := s.versions[key]
	if expectedVersion > 0 && currentVersion != expectedVersion {
		return es.NewOptimisticLockError(key, expectedVersion, currentVersion)
	}

	// Persist the event
	s.events[key] = append(s.events[key], event)

	// Increment the version
	s.versions[key] = currentVersion + 1

	return nil
}

// PersistEventAndSnapshot atomically persists an event and a snapshot.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (s *Store) PersistEventAndSnapshot(_ context.Context, event es.Event, aggregate es.Aggregate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := event.GetAggregateId().AsString()

	// Version check for optimistic locking
	currentVersion := s.versions[key]
	expectedVersion := aggregate.GetVersion()
	if expectedVersion > 0 && currentVersion != expectedVersion {
		return es.NewOptimisticLockError(key, expectedVersion, currentVersion)
	}

	// Persist the event
	s.events[key] = append(s.events[key], event)

	// Update the snapshot.
	// Note: in the actual implementation the aggregate is serialized.
	// The in-memory store only stores metadata.
	s.snapshots[key] = &es.SnapshotData{
		Payload: nil, // payload is set at the repository layer
		SeqNr:   event.GetSeqNr(),
		Version: currentVersion + 1,
	}

	// Increment the version
	s.versions[key] = currentVersion + 1

	return nil
}

// Clear removes all data from the store. Useful for test cleanup.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = make(map[string][]es.Event)
	s.snapshots = make(map[string]*es.SnapshotData)
	s.versions = make(map[string]uint64)
}

// GetEventCount returns the total number of events for an aggregate.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (s *Store) GetEventCount(id es.AggregateID) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.events[id.AsString()])
}

// GetAllEvents returns all events in the store (for debugging/testing).
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (s *Store) GetAllEvents() map[string][]es.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]es.Event, len(s.events))
	for k, v := range s.events {
		events := make([]es.Event, len(v))
		copy(events, v)
		result[k] = events
	}
	return result
}

// SetSnapshot directly sets a snapshot (for testing).
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (s *Store) SetSnapshot(id es.AggregateID, snapshot *es.SnapshotData) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshots[id.AsString()] = snapshot
	s.versions[id.AsString()] = snapshot.Version
}
