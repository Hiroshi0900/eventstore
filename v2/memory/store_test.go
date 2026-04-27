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
