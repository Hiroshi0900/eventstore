// Package memory は in-memory な EventStore 実装を提供します（テスト用途）。
package memory

import (
	"context"
	"sort"
	"sync"

	es "github.com/Hiroshi0900/eventstore/v2"
)

// Store[T] は in-memory な EventStore[T] 実装です。
type Store[T es.Aggregate] struct {
	mu        sync.RWMutex
	events    map[string][]es.Event // aggregateID -> events
	snapshots map[string]T          // aggregateID -> latest aggregate snapshot
	hasSnap   map[string]bool       // T のゼロ値と区別するための存在フラグ
}

// New[T] は空の Store[T] を生成します。
func New[T es.Aggregate]() *Store[T] {
	return &Store[T]{
		events:    map[string][]es.Event{},
		snapshots: map[string]T{},
		hasSnap:   map[string]bool{},
	}
}

// GetLatestSnapshotByID は集約の最新スナップショットを返します。存在しなければ found=false。
func (s *Store[T]) GetLatestSnapshotByID(_ context.Context, id es.AggregateID) (T, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var zero T
	key := id.AsString()
	if !s.hasSnap[key] {
		return zero, false, nil
	}
	return s.snapshots[key], true, nil
}

// GetEventsByIDSinceSeqNr は seqNr より大きい seqNr のイベントを昇順で返します。
func (s *Store[T]) GetEventsByIDSinceSeqNr(_ context.Context, id es.AggregateID, seqNr uint64) ([]es.Event, error) {
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
// 楽観的ロックは PersistEventAndSnapshot の aggregate.Version() で一本化。
func (s *Store[T]) PersistEvent(_ context.Context, event es.Event, _ uint64) error {
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
// 楽観的ロック: aggregate.Version() - 1 が現行 snapshot Version と一致しなければ ErrOptimisticLock。
// aggregate.Version() == 1 のとき初回作成扱い (既存 snapshot があれば衝突)。
// 同じ SeqNr のイベントが既に存在する場合も ErrOptimisticLock (並行更新による競合)。
func (s *Store[T]) PersistEventAndSnapshot(_ context.Context, event es.Event, aggregate T) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := event.AggregateID().AsString()

	// 楽観ロック: aggregate.Version() を新バージョン、期待 = それ - 1。
	newVersion := aggregate.Version()
	var expected uint64
	if newVersion >= 1 {
		expected = newVersion - 1
	}

	var currentSnapVersion uint64
	if s.hasSnap[key] {
		currentSnapVersion = s.snapshots[key].Version()
	}
	if currentSnapVersion != expected {
		return es.NewOptimisticLockError(key, expected, currentSnapVersion)
	}

	// 同 SeqNr 衝突は並行更新による競合 → ErrOptimisticLock
	for _, existing := range s.events[key] {
		if existing.SeqNr() == event.SeqNr() {
			return es.NewOptimisticLockError(key, expected, currentSnapVersion)
		}
	}

	s.events[key] = append(s.events[key], event)
	s.snapshots[key] = aggregate
	s.hasSnap[key] = true
	return nil
}

// Clear はストア内のすべてのデータを消去します（テスト用途）。
func (s *Store[T]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = map[string][]es.Event{}
	s.snapshots = map[string]T{}
	s.hasSnap = map[string]bool{}
}
