package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/memory"
)

// memoryTestAggID is a typed AggregateID local to the memory tests.
type memoryTestAggID struct {
	typeName, value string
}

func (a memoryTestAggID) TypeName() string { return a.typeName }
func (a memoryTestAggID) Value() string    { return a.value }
func (a memoryTestAggID) AsString() string { return a.typeName + "-" + a.value }

func newEnv(id es.AggregateID, seqNr uint64, isCreated bool) *es.EventEnvelope {
	return &es.EventEnvelope{
		EventID:       "ev-test",
		EventTypeName: "Test",
		AggregateID:   id,
		SeqNr:         seqNr,
		IsCreated:     isCreated,
		OccurredAt:    time.Now().UTC(),
		Payload:       []byte(`{}`),
	}
}

func newSnap(id es.AggregateID, seqNr, version uint64) *es.SnapshotEnvelope {
	return &es.SnapshotEnvelope{
		AggregateID: id,
		SeqNr:       seqNr,
		Version:     version,
		Payload:     []byte(`{"state":"ok"}`),
		OccurredAt:  time.Now().UTC(),
	}
}

func TestStore_GetLatestSnapshot_empty(t *testing.T) {
	s := memory.New()
	id := memoryTestAggID{"Visit", "x"}

	snap, err := s.GetLatestSnapshot(context.Background(), id)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if snap != nil {
		t.Errorf("snap = %+v, want nil", snap)
	}
}

func TestStore_GetEventsSince_empty(t *testing.T) {
	s := memory.New()
	id := memoryTestAggID{"Visit", "x"}

	events, err := s.GetEventsSince(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(events) != 0 {
		t.Errorf("len = %d, want 0", len(events))
	}
}

func TestStore_PersistEvent_persistsAndQueryable(t *testing.T) {
	s := memory.New()
	id := memoryTestAggID{"Visit", "x"}

	if err := s.PersistEvent(context.Background(), newEnv(id, 1, true), 0); err != nil {
		t.Fatalf("PersistEvent: %v", err)
	}

	got, err := s.GetEventsSince(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsSince: %v", err)
	}
	if len(got) != 1 || got[0].SeqNr != 1 {
		t.Errorf("got %+v, want 1 event with SeqNr=1", got)
	}
}

func TestStore_PersistEvent_duplicateSeqNr(t *testing.T) {
	s := memory.New()
	id := memoryTestAggID{"Visit", "x"}

	if err := s.PersistEvent(context.Background(), newEnv(id, 1, true), 0); err != nil {
		t.Fatalf("first: %v", err)
	}
	err := s.PersistEvent(context.Background(), newEnv(id, 1, false), 0)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, es.ErrDuplicateAggregate) {
		t.Errorf("got %v, want ErrDuplicateAggregate", err)
	}
}

func TestStore_PersistEvent_initialOnExistingAggregate(t *testing.T) {
	s := memory.New()
	id := memoryTestAggID{"Visit", "x"}

	if err := s.PersistEvent(context.Background(), newEnv(id, 1, true), 0); err != nil {
		t.Fatalf("first: %v", err)
	}
	err := s.PersistEvent(context.Background(), newEnv(id, 2, true), 0)
	if !errors.Is(err, es.ErrDuplicateAggregate) {
		t.Errorf("got %v, want ErrDuplicateAggregate", err)
	}
}

func TestStore_PersistEventAndSnapshot_initialWrite(t *testing.T) {
	s := memory.New()
	id := memoryTestAggID{"Visit", "x"}

	ev := newEnv(id, 5, false)
	snap := newSnap(id, 5, 1)

	if err := s.PersistEventAndSnapshot(context.Background(), ev, snap); err != nil {
		t.Fatalf("PersistEventAndSnapshot: %v", err)
	}

	got, err := s.GetLatestSnapshot(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshot: %v", err)
	}
	if got == nil || got.Version != 1 || got.SeqNr != 5 {
		t.Errorf("snapshot mismatch: got %+v", got)
	}
}

func TestStore_PersistEventAndSnapshot_optimisticLockFailure(t *testing.T) {
	s := memory.New()
	id := memoryTestAggID{"Visit", "x"}

	if err := s.PersistEventAndSnapshot(
		context.Background(), newEnv(id, 5, false), newSnap(id, 5, 1)); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := s.PersistEventAndSnapshot(
		context.Background(), newEnv(id, 10, false), newSnap(id, 10, 2)); err != nil {
		t.Fatalf("second: %v", err)
	}
	// stale writer commits Version=2 again (current is 2, expected=1 → mismatch)
	err := s.PersistEventAndSnapshot(
		context.Background(), newEnv(id, 11, false), newSnap(id, 11, 2))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, es.ErrOptimisticLock) {
		t.Errorf("got %v, want ErrOptimisticLock", err)
	}
}
