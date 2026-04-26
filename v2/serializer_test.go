package eventstore

import (
	"errors"
	"testing"
	"time"
)

func TestJSONEventSerializer_RoundTrip(t *testing.T) {
	s := NewJSONEventSerializer()
	id := NewAggregateID("Visit", "abc-123")
	ts := time.Date(2026, 4, 27, 12, 34, 56, 0, time.UTC)
	src := NewEvent(
		"evt-1",
		"VisitScheduled",
		id,
		[]byte(`{"foo":"bar"}`),
		WithSeqNr(7),
		WithIsCreated(true),
		WithOccurredAt(ts),
	)

	data, err := s.Serialize(src)
	if err != nil {
		t.Fatalf("Serialize err: %v", err)
	}

	got, err := s.Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize err: %v", err)
	}

	if got.EventID() != src.EventID() {
		t.Errorf("EventID = %q, want %q", got.EventID(), src.EventID())
	}
	if got.EventTypeName() != src.EventTypeName() {
		t.Errorf("EventTypeName = %q, want %q", got.EventTypeName(), src.EventTypeName())
	}
	if got.AggregateID().AsString() != src.AggregateID().AsString() {
		t.Errorf("AggregateID = %q, want %q", got.AggregateID().AsString(), src.AggregateID().AsString())
	}
	if got.SeqNr() != src.SeqNr() {
		t.Errorf("SeqNr = %d, want %d", got.SeqNr(), src.SeqNr())
	}
	if got.IsCreated() != src.IsCreated() {
		t.Errorf("IsCreated = %v, want %v", got.IsCreated(), src.IsCreated())
	}
	// OccurredAt は millisecond 精度で round-trip される。
	if !got.OccurredAt().Equal(ts) {
		t.Errorf("OccurredAt = %v, want %v", got.OccurredAt(), ts)
	}
	if string(got.Payload()) != string(src.Payload()) {
		t.Errorf("Payload = %q, want %q", got.Payload(), src.Payload())
	}
}

func TestJSONEventSerializer_DeserializeError(t *testing.T) {
	s := NewJSONEventSerializer()
	_, err := s.Deserialize([]byte("{invalid"))
	if !errors.Is(err, ErrDeserializationFailed) {
		t.Errorf("expected ErrDeserializationFailed, got %v", err)
	}
}
