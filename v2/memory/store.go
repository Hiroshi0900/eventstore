// Package memory は in-memory な EventStore 実装を提供します（テスト用途）。
package memory

import (
	"context"
	"sort"
	"sync"

	es "github.com/Hiroshi0900/eventstore/v2"
)

// Store は in-memory な EventStore 実装です。
type Store struct {
	mu        sync.RWMutex
	events    map[string][]es.Event       // aggregateID -> events
	snapshots map[string]*es.SnapshotData // aggregateID -> latest snapshot
	versions  map[string]uint64           // aggregateID -> current version
}

// New は空の Store を生成します。
func New() *Store {
	return &Store{
		events:    map[string][]es.Event{},
		snapshots: map[string]*es.SnapshotData{},
		versions:  map[string]uint64{},
	}
}

// GetLatestSnapshotByID は集約の最新スナップショットを返します。存在しなければ nil。
func (s *Store) GetLatestSnapshotByID(_ context.Context, id es.AggregateID) (*es.SnapshotData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if snap, ok := s.snapshots[id.AsString()]; ok {
		return snap, nil
	}
	return nil, nil
}

// GetEventsByIDSinceSeqNr は seqNr より大きい seqNr のイベントを昇順で返します。
func (s *Store) GetEventsByIDSinceSeqNr(_ context.Context, id es.AggregateID, seqNr uint64) ([]es.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := s.events[id.AsString()]
	if len(all) == 0 {
		return nil, nil
	}
	out := make([]es.Event, 0, len(all))
	for _, ev := range all {
		if ev.SeqNr() > seqNr {
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SeqNr() < out[j].SeqNr() })
	return out, nil
}

// Clear はストア内のすべてのデータを消去します（テスト用途）。
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = map[string][]es.Event{}
	s.snapshots = map[string]*es.SnapshotData{}
	s.versions = map[string]uint64{}
}
