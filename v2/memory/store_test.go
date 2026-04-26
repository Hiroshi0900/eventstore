package memory_test

import (
	"context"
	"errors"
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/memory"
)

// visitID は Visit ドメインの typed AggregateID 実装 (テスト用)。
type visitID string

func (v visitID) TypeName() string { return "Visit" }
func (v visitID) Value() string    { return string(v) }
func (v visitID) AsString() string { return "Visit-" + string(v) }

// stubAgg は memory.Store のテスト用最小 Aggregate 実装。
type stubAgg struct {
	id      es.AggregateID
	seqNr   uint64
	version uint64
}

func newStub(id es.AggregateID) stubAgg                  { return stubAgg{id: id} }
func (a stubAgg) AggregateID() es.AggregateID            { return a.id }
func (a stubAgg) SeqNr() uint64                          { return a.seqNr }
func (a stubAgg) Version() uint64                        { return a.version }
func (a stubAgg) WithSeqNr(s uint64) es.Aggregate        { a.seqNr = s; return a }
func (a stubAgg) WithVersion(v uint64) es.Aggregate      { a.version = v; return a }
func (a stubAgg) ApplyCommand(es.Command) (es.Event, error) {
	return nil, es.ErrUnknownCommand
}
func (a stubAgg) ApplyEvent(es.Event) es.Aggregate { return a }

func TestNew_Empty(t *testing.T) {
	s := memory.New[stubAgg]()
	if s == nil {
		t.Fatal("New() returned nil")
	}

	id := visitID("v1")
	_, found, err := s.GetLatestSnapshotByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshotByID err: %v", err)
	}
	if found {
		t.Errorf("expected found=false on empty store")
	}

	events, err := s.GetEventsByIDSinceSeqNr(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsByIDSinceSeqNr err: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty events, got %d", len(events))
	}
}

func TestStore_PersistEvent_FirstEvent(t *testing.T) {
	s := memory.New[stubAgg]()
	id := visitID("v1")
	ev := es.NewEvent("evt-1", "VisitScheduled", id, []byte("p"),
		es.WithSeqNr(1), es.WithIsCreated(true))

	if err := s.PersistEvent(context.Background(), ev, 0); err != nil {
		t.Fatalf("PersistEvent err: %v", err)
	}

	got, err := s.GetEventsByIDSinceSeqNr(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsByIDSinceSeqNr err: %v", err)
	}
	if len(got) != 1 || got[0].EventID() != "evt-1" {
		t.Errorf("expected single event evt-1, got %+v", got)
	}
}

func TestStore_PersistEvent_DuplicateOnCreate(t *testing.T) {
	s := memory.New[stubAgg]()
	id := visitID("v1")
	ev := es.NewEvent("evt-1", "VisitScheduled", id, nil, es.WithSeqNr(1))

	if err := s.PersistEvent(context.Background(), ev, 0); err != nil {
		t.Fatalf("first PersistEvent err: %v", err)
	}
	err := s.PersistEvent(context.Background(), ev, 0)
	if !errors.Is(err, es.ErrDuplicateAggregate) {
		t.Errorf("expected ErrDuplicateAggregate, got %v", err)
	}
}

func TestStore_PersistEvent_DuplicateSeqNr(t *testing.T) {
	s := memory.New[stubAgg]()
	id := visitID("v1")
	ev1 := es.NewEvent("evt-1", "VisitScheduled", id, nil, es.WithSeqNr(1))
	ev2 := es.NewEvent("evt-2", "VisitCompleted", id, nil, es.WithSeqNr(1)) // 同じ SeqNr

	if err := s.PersistEvent(context.Background(), ev1, 0); err != nil {
		t.Fatalf("ev1 err: %v", err)
	}
	err := s.PersistEvent(context.Background(), ev2, 1)
	if !errors.Is(err, es.ErrDuplicateAggregate) {
		t.Errorf("expected ErrDuplicateAggregate, got %v", err)
	}
}

func TestStore_PersistEventAndSnapshot_First(t *testing.T) {
	s := memory.New[stubAgg]()
	id := visitID("v1")
	ev := es.NewEvent("evt-1", "VisitScheduled", id, nil,
		es.WithSeqNr(1), es.WithIsCreated(true))
	agg := stubAgg{id: id, seqNr: 1, version: 1}

	if err := s.PersistEventAndSnapshot(context.Background(), ev, agg); err != nil {
		t.Fatalf("PersistEventAndSnapshot err: %v", err)
	}

	got, found, err := s.GetLatestSnapshotByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshotByID err: %v", err)
	}
	if !found {
		t.Fatal("snapshot not found")
	}
	if got.SeqNr() != 1 || got.Version() != 1 {
		t.Errorf("snapshot mismatch: seqNr=%d version=%d", got.SeqNr(), got.Version())
	}

	events, _ := s.GetEventsByIDSinceSeqNr(context.Background(), id, 0)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestStore_PersistEventAndSnapshot_OptimisticLock(t *testing.T) {
	s := memory.New[stubAgg]()
	id := visitID("v1")
	ev1 := es.NewEvent("evt-1", "VisitScheduled", id, nil, es.WithSeqNr(1))
	agg1 := stubAgg{id: id, seqNr: 1, version: 1}
	if err := s.PersistEventAndSnapshot(context.Background(), ev1, agg1); err != nil {
		t.Fatalf("first err: %v", err)
	}

	// version=99 を期待される 2 ではなく与えると楽観ロック競合
	ev2 := es.NewEvent("evt-2", "VisitCompleted", id, nil, es.WithSeqNr(2))
	agg2 := stubAgg{id: id, seqNr: 2, version: 99}
	err := s.PersistEventAndSnapshot(context.Background(), ev2, agg2)
	if !errors.Is(err, es.ErrOptimisticLock) {
		t.Errorf("expected ErrOptimisticLock, got %v", err)
	}
}
