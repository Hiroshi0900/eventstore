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
}

// New は空の Store を生成します。
func New() *Store {
	return &Store{
		events:    map[string][]es.Event{},
		snapshots: map[string]*es.SnapshotData{},
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

// PersistEvent はイベントを保存します。
// 同じ SeqNr のイベントが既に存在する場合は ErrDuplicateAggregate（v1 DynamoDB の
// `attribute_not_exists(#pk)` 条件と同等）。これにより新規集約の二重作成も SeqNr=1 衝突として検出される。
//
// version 引数は EventStore interface 互換のため受け取るが使用しない。
// 楽観的ロックは PersistEventAndSnapshot の snapshot.Version で一本化。
func (s *Store) PersistEvent(_ context.Context, event es.Event, _ uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := event.AggregateID().AsString()

	// 同 SeqNr 衝突チェック (初回作成の二重実行もこれで検出)
	for _, existing := range s.events[key] {
		if existing.SeqNr() == event.SeqNr() {
			return es.NewDuplicateAggregateError(key)
		}
	}

	s.events[key] = append(s.events[key], event)
	return nil
}

// PersistEventAndSnapshot はイベントとスナップショットをアトミックに保存します。
// 楽観的ロック: 既存 snapshot の Version + 1 が snapshot.Version と一致しない場合 ErrOptimisticLock。
// snapshot がない場合は snapshot.Version == 1 を期待。
// 同じ SeqNr のイベントが既に存在する場合も ErrOptimisticLock (並行更新による競合)。
// これは v1 DynamoDB の TransactWriteItems が ConditionalCheckFailed を一律 ErrOptimisticLock として扱うのと整合。
func (s *Store) PersistEventAndSnapshot(_ context.Context, event es.Event, snapshot es.SnapshotData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := event.AggregateID().AsString()

	// snapshot 世代ベースの楽観ロック (先に判定)
	var currentSnapVersion uint64
	if existing, ok := s.snapshots[key]; ok {
		currentSnapVersion = existing.Version
	}
	expected := currentSnapVersion + 1
	if snapshot.Version != expected {
		return es.NewOptimisticLockError(key, expected, snapshot.Version)
	}

	// 同 SeqNr 衝突は並行更新による競合 → ErrOptimisticLock
	for _, existing := range s.events[key] {
		if existing.SeqNr() == event.SeqNr() {
			return es.NewOptimisticLockError(key, expected, snapshot.Version)
		}
	}

	s.events[key] = append(s.events[key], event)
	s.snapshots[key] = &snapshot
	return nil
}

// Clear はストア内のすべてのデータを消去します（テスト用途）。
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = map[string][]es.Event{}
	s.snapshots = map[string]*es.SnapshotData{}
}
