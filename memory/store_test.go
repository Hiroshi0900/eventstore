package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	es "github.com/Hiroshi0900/eventstore"
	"github.com/Hiroshi0900/eventstore/memory"
)

// memoryTestAggID is a typed AggregateID local to the memory tests.
type memoryTestAggID struct {
	typeName, value string
}

func (a memoryTestAggID) TypeName() string { return a.typeName }
func (a memoryTestAggID) Value() string    { return a.value }
func (a memoryTestAggID) AsString() string { return a.typeName + "-" + a.value }

// stubEvent / stubAggregate: 最小限の Event / Command / Aggregate 実装。
type stubCommand interface {
	es.Command
	isStubCommand()
}

type createStubCommand struct{}

func (createStubCommand) CommandTypeName() string { return "CreateStub" }
func (createStubCommand) isStubCommand()          {}

type stubEvent struct {
	aggID memoryTestAggID
	name  string
}

func (e stubEvent) EventTypeName() string       { return e.name }
func (e stubEvent) AggregateID() es.AggregateID { return e.aggID }

type stubAggregate struct {
	id memoryTestAggID
}

func (a stubAggregate) AggregateID() es.AggregateID { return a.id }
func (a stubAggregate) ApplyCommand(stubCommand) (stubEvent, error) {
	return stubEvent{}, es.ErrUnknownCommand
}
func (a stubAggregate) ApplyEvent(stubEvent) es.Aggregate[stubCommand, stubEvent] { return a }

func newStored(id memoryTestAggID, seqNr uint64, isCreated bool) es.StoredEvent[stubEvent] {
	return es.StoredEvent[stubEvent]{
		Event:      stubEvent{aggID: id, name: "Test"},
		EventID:    "ev-test",
		SeqNr:      seqNr,
		IsCreated:  isCreated,
		OccurredAt: time.Now().UTC(),
	}
}

func newStoredSnap(id memoryTestAggID, seqNr, version uint64) es.StoredSnapshot[stubAggregate] {
	return es.StoredSnapshot[stubAggregate]{
		Aggregate:  stubAggregate{id: id},
		SeqNr:      seqNr,
		Version:    version,
		OccurredAt: time.Now().UTC(),
	}
}

func TestStore_GetLatestSnapshot_empty(t *testing.T) {
	s := memory.New[stubAggregate, stubCommand, stubEvent]()
	id := memoryTestAggID{"Visit", "x"}

	_, found, err := s.GetLatestSnapshot(context.Background(), id)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if found {
		t.Errorf("found = true, want false")
	}
}

func TestStore_LoadStreamAfter_empty(t *testing.T) {
	s := memory.New[stubAggregate, stubCommand, stubEvent]()
	id := memoryTestAggID{"Visit", "x"}

	events, err := s.LoadStreamAfter(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(events) != 0 {
		t.Errorf("len = %d, want 0", len(events))
	}
}

func TestStore_PersistEvent_persistsAndQueryable(t *testing.T) {
	s := memory.New[stubAggregate, stubCommand, stubEvent]()
	id := memoryTestAggID{"Visit", "x"}

	if err := s.PersistEvent(context.Background(), newStored(id, 1, true), 0); err != nil {
		t.Fatalf("PersistEvent: %v", err)
	}

	got, err := s.LoadStreamAfter(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("LoadStreamAfter: %v", err)
	}
	if len(got) != 1 || got[0].SeqNr != 1 {
		t.Errorf("got %+v, want 1 event with SeqNr=1", got)
	}
}

func TestStore_LoadStreamAfter_excludesBoundarySeqNr(t *testing.T) {
	s := memory.New[stubAggregate, stubCommand, stubEvent]()
	id := memoryTestAggID{"Visit", "boundary"}

	if err := s.PersistEvent(context.Background(), newStored(id, 1, true), 0); err != nil {
		t.Fatalf("PersistEvent 1: %v", err)
	}
	if err := s.PersistEvent(context.Background(), newStored(id, 2, false), 1); err != nil {
		t.Fatalf("PersistEvent 2: %v", err)
	}

	got, err := s.LoadStreamAfter(context.Background(), id, 1)
	if err != nil {
		t.Fatalf("LoadStreamAfter: %v", err)
	}
	if len(got) != 1 || got[0].SeqNr != 2 {
		t.Fatalf("got %+v, want only SeqNr=2", got)
	}
}

func TestStore_LoadStreamAfter_returnsEventsInAscendingOrder(t *testing.T) {
	s := memory.New[stubAggregate, stubCommand, stubEvent]()
	id := memoryTestAggID{"Visit", "ordering"}

	if err := s.PersistEvent(context.Background(), newStored(id, 1, true), 0); err != nil {
		t.Fatalf("PersistEvent 1: %v", err)
	}
	if err := s.PersistEvent(context.Background(), newStored(id, 2, false), 1); err != nil {
		t.Fatalf("PersistEvent 2: %v", err)
	}
	if err := s.PersistEvent(context.Background(), newStored(id, 3, false), 1); err != nil {
		t.Fatalf("PersistEvent 3: %v", err)
	}

	got, err := s.LoadStreamAfter(context.Background(), id, 1)
	if err != nil {
		t.Fatalf("LoadStreamAfter: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].SeqNr != 2 || got[1].SeqNr != 3 {
		t.Fatalf("seq order = [%d %d], want [2 3]", got[0].SeqNr, got[1].SeqNr)
	}
}

func TestStore_PersistEvent_duplicateSeqNr(t *testing.T) {
	s := memory.New[stubAggregate, stubCommand, stubEvent]()
	id := memoryTestAggID{"Visit", "x"}

	if err := s.PersistEvent(context.Background(), newStored(id, 1, true), 0); err != nil {
		t.Fatalf("first: %v", err)
	}
	err := s.PersistEvent(context.Background(), newStored(id, 1, false), 0)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, es.ErrDuplicateAggregate) {
		t.Errorf("got %v, want ErrDuplicateAggregate", err)
	}
}

func TestStore_PersistEvent_initialOnExistingAggregate(t *testing.T) {
	s := memory.New[stubAggregate, stubCommand, stubEvent]()
	id := memoryTestAggID{"Visit", "x"}

	if err := s.PersistEvent(context.Background(), newStored(id, 1, true), 0); err != nil {
		t.Fatalf("first: %v", err)
	}
	err := s.PersistEvent(context.Background(), newStored(id, 2, true), 0)
	if !errors.Is(err, es.ErrDuplicateAggregate) {
		t.Errorf("got %v, want ErrDuplicateAggregate", err)
	}
}

func TestStore_PersistEventAndSnapshot_initialWrite(t *testing.T) {
	s := memory.New[stubAggregate, stubCommand, stubEvent]()
	id := memoryTestAggID{"Visit", "x"}

	ev := newStored(id, 5, false)
	snap := newStoredSnap(id, 5, 1)

	if err := s.PersistEventAndSnapshot(context.Background(), ev, snap); err != nil {
		t.Fatalf("PersistEventAndSnapshot: %v", err)
	}

	got, found, err := s.GetLatestSnapshot(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshot: %v", err)
	}
	if !found || got.Version != 1 || got.SeqNr != 5 {
		t.Errorf("snapshot mismatch: got %+v found=%v", got, found)
	}
}

func TestStore_PersistEventAndSnapshot_optimisticLockFailure(t *testing.T) {
	s := memory.New[stubAggregate, stubCommand, stubEvent]()
	id := memoryTestAggID{"Visit", "x"}

	if err := s.PersistEventAndSnapshot(
		context.Background(), newStored(id, 5, false), newStoredSnap(id, 5, 1)); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := s.PersistEventAndSnapshot(
		context.Background(), newStored(id, 10, false), newStoredSnap(id, 10, 2)); err != nil {
		t.Fatalf("second: %v", err)
	}
	// stale writer commits Version=2 again (current is 2, expected=1 → mismatch)
	err := s.PersistEventAndSnapshot(
		context.Background(), newStored(id, 11, false), newStoredSnap(id, 11, 2))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, es.ErrOptimisticLock) {
		t.Errorf("got %v, want ErrOptimisticLock", err)
	}
}
