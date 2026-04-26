package eventstore

import (
	"testing"
	"time"
)

func TestNewEvent_Defaults(t *testing.T) {
	id := NewAggregateID("Visit", "v1")
	ev := NewEvent("evt-1", "VisitScheduled", id, []byte("payload"))

	if got, want := ev.EventID(), "evt-1"; got != want {
		t.Errorf("EventID() = %q, want %q", got, want)
	}
	if got, want := ev.EventTypeName(), "VisitScheduled"; got != want {
		t.Errorf("EventTypeName() = %q, want %q", got, want)
	}
	if got, want := ev.AggregateID().AsString(), "Visit-v1"; got != want {
		t.Errorf("AggregateID() = %q, want %q", got, want)
	}
	if got, want := ev.SeqNr(), uint64(1); got != want {
		t.Errorf("SeqNr() = %d, want %d", got, want)
	}
	if ev.IsCreated() {
		t.Error("IsCreated() = true, want false (default)")
	}
	if got, want := string(ev.Payload()), "payload"; got != want {
		t.Errorf("Payload() = %q, want %q", got, want)
	}
	if ev.OccurredAt().IsZero() {
		t.Error("OccurredAt() should not be zero by default")
	}
}

func TestNewEvent_WithOptions(t *testing.T) {
	id := NewAggregateID("Visit", "v1")
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := NewEvent("evt-1", "VisitScheduled", id, nil,
		WithSeqNr(42),
		WithIsCreated(true),
		WithOccurredAt(ts),
	)

	if got, want := ev.SeqNr(), uint64(42); got != want {
		t.Errorf("SeqNr() = %d, want %d", got, want)
	}
	if !ev.IsCreated() {
		t.Error("IsCreated() = false, want true")
	}
	if !ev.OccurredAt().Equal(ts) {
		t.Errorf("OccurredAt() = %v, want %v", ev.OccurredAt(), ts)
	}
}

func TestDefaultEvent_WithSeqNr(t *testing.T) {
	id := NewAggregateID("Visit", "v1")
	ev := NewEvent("evt-1", "VisitScheduled", id, nil, WithSeqNr(1))

	updated := ev.WithSeqNr(5)
	if got, want := updated.SeqNr(), uint64(5); got != want {
		t.Errorf("WithSeqNr(5).SeqNr() = %d, want %d", got, want)
	}
	if got, want := ev.SeqNr(), uint64(1); got != want {
		t.Errorf("original SeqNr() should be unchanged: got %d, want %d", got, want)
	}
}
