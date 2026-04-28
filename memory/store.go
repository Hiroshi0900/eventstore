// Package memory provides an in-memory EventStore implementation for testing.
package memory

import (
	"context"
	"sort"
	"sync"

	es "github.com/Hiroshi0900/eventstore"
)

// store is an in-memory EventStore implementation. concrete type は非公開で、
// 外部からは New() が返す es.EventStore[T, E] interface 経由でのみ操作する。
//
// in-memory なので (de)serialize は一切行わず、StoredEvent / StoredSnapshot を
// そのまま保持する。
type store[T es.Aggregate[E], E es.Event] struct {
	mu        sync.RWMutex
	events    map[string][]es.StoredEvent[E]   // aggregateID.AsString() -> ordered events
	snapshots map[string]es.StoredSnapshot[T]  // aggregateID.AsString() -> latest snapshot
	hasSnap   map[string]bool                  // T のゼロ値と区別するための存在フラグ
}

// New は in-memory EventStore[T, E] を生成する。
//
// memory store は (de)serialize を行わないので serializer は不要。
func New[T es.Aggregate[E], E es.Event]() es.EventStore[T, E] {
	return &store[T, E]{
		events:    make(map[string][]es.StoredEvent[E]),
		snapshots: make(map[string]es.StoredSnapshot[T]),
		hasSnap:   make(map[string]bool),
	}
}

// GetLatestSnapshot returns the latest snapshot or found=false.
func (s *store[T, E]) GetLatestSnapshot(_ context.Context, id es.AggregateID) (es.StoredSnapshot[T], bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := id.AsString()
	if !s.hasSnap[key] {
		var zero es.StoredSnapshot[T]
		return zero, false, nil
	}
	return s.snapshots[key], true, nil
}

// GetEventsSince returns events with SeqNr > seqNr, ordered by SeqNr.
func (s *store[T, E]) GetEventsSince(_ context.Context, id es.AggregateID, seqNr uint64) ([]es.StoredEvent[E], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	src := s.events[id.AsString()]
	var out []es.StoredEvent[E]
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
// is performed only by PersistEventAndSnapshot). Duplicate (aggregateID, seqNr)
// entries are rejected.
func (s *store[T, E]) PersistEvent(_ context.Context, ev es.StoredEvent[E], expectedVersion uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := ev.Event.AggregateID().AsString()

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
// 楽観ロック: 現行 snapshot version が snap.Version - 1 と一致しない場合 ErrOptimisticLock を返す。
// (initial write では現行 snapshot 未存在 ⇔ current=0, snap.Version=1 ⇔ expected=0 で match)
func (s *store[T, E]) PersistEventAndSnapshot(
	_ context.Context,
	ev es.StoredEvent[E],
	snap es.StoredSnapshot[T],
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := ev.Event.AggregateID().AsString()

	expected := snap.Version - 1
	current := uint64(0)
	if s.hasSnap[key] {
		current = s.snapshots[key].Version
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
	s.hasSnap[key] = true
	return nil
}
