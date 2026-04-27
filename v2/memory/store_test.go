package memory_test

import (
	"context"
	"testing"
	"time"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/memory"
)

func TestStore_GetLatestSnapshot_empty(t *testing.T) {
	store := memory.New()
	id := es.NewAggregateID("Visit", "x")

	snap, err := store.GetLatestSnapshot(context.Background(), id)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if snap != nil {
		t.Errorf("snap = %+v, want nil", snap)
	}
}

func TestStore_GetEventsSince_empty(t *testing.T) {
	store := memory.New()
	id := es.NewAggregateID("Visit", "x")

	events, err := store.GetEventsSince(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(events) != 0 {
		t.Errorf("events len = %d, want 0", len(events))
	}
}

func newEventEnvelope(t *testing.T, id es.AggregateID, seqNr uint64, isCreated bool) *es.EventEnvelope {
	t.Helper()
	return &es.EventEnvelope{
		EventID:       "ev-test",
		EventTypeName: "Test",
		AggregateID:   id,
		SeqNr:         seqNr,
		IsCreated:     isCreated,
		OccurredAt:    time.Now(),
		Payload:       []byte(`{}`),
	}
}

func TestStore_PersistEvent_persistsAndQueryable(t *testing.T) {
	store := memory.New()
	id := es.NewAggregateID("Visit", "x")

	ev := newEventEnvelope(t, id, 1, true)
	if err := store.PersistEvent(context.Background(), ev, 0); err != nil {
		t.Fatalf("PersistEvent: %v", err)
	}

	got, err := store.GetEventsSince(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsSince: %v", err)
	}
	if len(got) != 1 || got[0].SeqNr != 1 {
		t.Errorf("got %+v, want 1 event with SeqNr=1", got)
	}
}

func TestStore_PersistEvent_duplicateSeqNr(t *testing.T) {
	store := memory.New()
	id := es.NewAggregateID("Visit", "x")

	if err := store.PersistEvent(context.Background(), newEventEnvelope(t, id, 1, true), 0); err != nil {
		t.Fatalf("first PersistEvent: %v", err)
	}
	err := store.PersistEvent(context.Background(), newEventEnvelope(t, id, 1, false), 0)
	if err == nil {
		t.Fatalf("second PersistEvent: want error, got nil")
	}
}

func TestStore_PersistEvent_initialEventOnExistingAggregate(t *testing.T) {
	store := memory.New()
	id := es.NewAggregateID("Visit", "x")

	if err := store.PersistEvent(context.Background(), newEventEnvelope(t, id, 1, true), 0); err != nil {
		t.Fatalf("first PersistEvent: %v", err)
	}
	// Second persist with expectedVersion=0 (i.e. "I think this aggregate is new")
	// must fail because the aggregate already has events.
	err := store.PersistEvent(context.Background(), newEventEnvelope(t, id, 2, true), 0)
	if err == nil {
		t.Fatalf("PersistEvent with expectedVersion=0 on existing aggregate: want error, got nil")
	}
	if !errorsIs(err, es.ErrDuplicateAggregate) {
		t.Errorf("expected ErrDuplicateAggregate, got %v", err)
	}
}

// errorsIs is a small helper to avoid importing errors in every test file.
func errorsIs(err, target error) bool {
	if err == nil {
		return target == nil
	}
	type isCheck interface{ Is(error) bool }
	if c, ok := err.(isCheck); ok && c.Is(target) {
		return true
	}
	return err == target
}
