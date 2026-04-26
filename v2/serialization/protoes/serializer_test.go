package protoes_test

import (
	"errors"
	"testing"
	"time"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/serialization/protoes"
)

func TestProtoEventSerializer_RoundTrip(t *testing.T) {
	s := protoes.New()
	id := es.NewAggregateID("Visit", "abc-123")
	ts := time.Date(2026, 4, 27, 12, 34, 56, 0, time.UTC)
	src := es.NewEvent(
		"evt-1",
		"VisitScheduled",
		id,
		[]byte(`{"foo":"bar"}`),
		es.WithSeqNr(7),
		es.WithIsCreated(true),
		es.WithOccurredAt(ts),
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
	if !got.OccurredAt().Equal(ts) {
		t.Errorf("OccurredAt = %v, want %v", got.OccurredAt(), ts)
	}
	if string(got.Payload()) != string(src.Payload()) {
		t.Errorf("Payload = %q, want %q", got.Payload(), src.Payload())
	}
}

func TestProtoEventSerializer_DeserializeError(t *testing.T) {
	s := protoes.New()
	// Protobuf の wire format として明らかに不正なバイト列。
	// tag=0 / wire type=7 はいずれも proto3 では invalid。
	_, err := s.Deserialize([]byte{0xff, 0xff, 0xff, 0xff, 0xff})
	if err == nil {
		t.Fatal("expected error for invalid protobuf data, got nil")
	}
	if !errors.Is(err, es.ErrDeserializationFailed) {
		t.Errorf("expected ErrDeserializationFailed, got %v", err)
	}
}
