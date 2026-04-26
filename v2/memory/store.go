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

// PersistEvent はイベントを保存します。expectedVersion は楽観ロック用の現在 version。
// 初回イベント (expectedVersion == 0) で既存があれば ErrDuplicateAggregate。
// それ以外で version 不一致なら ErrOptimisticLock。
func (s *Store) PersistEvent(_ context.Context, event es.Event, expectedVersion uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := event.AggregateID().AsString()

	if expectedVersion == 0 && len(s.events[key]) > 0 {
		return es.NewDuplicateAggregateError(key)
	}
	current := s.versions[key]
	if expectedVersion > 0 && current != expectedVersion {
		return es.NewOptimisticLockError(key, expectedVersion, current)
	}

	s.events[key] = append(s.events[key], event)
	s.versions[key] = current + 1
	return nil
}

// PersistEventAndSnapshot はイベントとスナップショットをアトミックに保存します。
// snapshot.Version は「現在のスナップショット Version + 1」でなければなりません。
// 現在スナップショットがない場合は snapshot.Version == 1 を期待します。
func (s *Store) PersistEventAndSnapshot(_ context.Context, event es.Event, snapshot es.SnapshotData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := event.AggregateID().AsString()

	// 楽観ロック: 現在のスナップショット Version に基づくチェック
	var currentSnapVersion uint64
	if existing, ok := s.snapshots[key]; ok {
		currentSnapVersion = existing.Version
	}
	expectedVersion := currentSnapVersion + 1
	if snapshot.Version != expectedVersion {
		return es.NewOptimisticLockError(key, expectedVersion, snapshot.Version)
	}

	s.events[key] = append(s.events[key], event)
	s.snapshots[key] = &snapshot
	s.versions[key] = snapshot.Version
	return nil
}

// Clear はストア内のすべてのデータを消去します（テスト用途）。
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = map[string][]es.Event{}
	s.snapshots = map[string]*es.SnapshotData{}
	s.versions = map[string]uint64{}
}
