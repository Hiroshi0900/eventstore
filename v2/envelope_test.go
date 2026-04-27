package eventsourcing_test

import (
	"encoding/json"
	"testing"
	"time"

	es "github.com/Hiroshi0900/eventstore/v2"
)

func TestEventEnvelope_jsonRoundtrip(t *testing.T) {
	occurred := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	in := es.EventEnvelope{
		EventID:       "ev-1",
		EventTypeName: "VisitScheduled",
		AggregateID:   es.NewAggregateID("Visit", "x"),
		SeqNr:         1,
		IsCreated:     true,
		OccurredAt:    occurred,
		Payload:       []byte(`{"k":"v"}`),
		TraceParent:   "00-trace-span-01",
		TraceState:    "vendor=value",
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var out es.EventEnvelope
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if out.EventID != in.EventID || out.EventTypeName != in.EventTypeName ||
		out.AggregateID.AsString() != in.AggregateID.AsString() ||
		out.SeqNr != in.SeqNr || out.IsCreated != in.IsCreated ||
		!out.OccurredAt.Equal(in.OccurredAt) ||
		string(out.Payload) != string(in.Payload) ||
		out.TraceParent != in.TraceParent || out.TraceState != in.TraceState {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", out, in)
	}
}

func TestSnapshotEnvelope_jsonRoundtrip(t *testing.T) {
	occurred := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	in := es.SnapshotEnvelope{
		AggregateID: es.NewAggregateID("Visit", "x"),
		SeqNr:       5,
		Version:     1,
		Payload:     []byte(`{"state":"scheduled"}`),
		OccurredAt:  occurred,
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var out es.SnapshotEnvelope
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if out.AggregateID.AsString() != in.AggregateID.AsString() ||
		out.SeqNr != in.SeqNr || out.Version != in.Version ||
		string(out.Payload) != string(in.Payload) ||
		!out.OccurredAt.Equal(in.OccurredAt) {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", out, in)
	}
}
