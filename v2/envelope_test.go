package eventstore_test

import (
	"encoding/json"
	"testing"
	"time"

	es "github.com/Hiroshi0900/eventstore/v2"
)

// envelopeTestAggID is a typed AggregateID defined in the test only —
// library does not provide a default implementation, so each test domain
// declares its own.
type envelopeTestAggID struct {
	typeName, value string
}

func (a envelopeTestAggID) TypeName() string { return a.typeName }
func (a envelopeTestAggID) Value() string    { return a.value }
func (a envelopeTestAggID) AsString() string { return a.typeName + "-" + a.value }

func TestEventEnvelope_jsonRoundtrip(t *testing.T) {
	occurred := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	in := es.EventEnvelope{
		EventID:       "ev-1",
		EventTypeName: "VisitScheduled",
		AggregateID:   envelopeTestAggID{"Visit", "x"},
		SeqNr:         1,
		IsCreated:     true,
		OccurredAt:    occurred,
		Payload:       []byte(`{"k":"v"}`),
		TraceParent:   "00-trace-span-01",
		TraceState:    "vendor=value",
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var out es.EventEnvelope
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if out.EventID != in.EventID {
		t.Errorf("EventID: got %q, want %q", out.EventID, in.EventID)
	}
	if out.EventTypeName != in.EventTypeName {
		t.Errorf("EventTypeName: got %q, want %q", out.EventTypeName, in.EventTypeName)
	}
	if out.AggregateID.AsString() != in.AggregateID.AsString() {
		t.Errorf("AggregateID: got %q, want %q", out.AggregateID.AsString(), in.AggregateID.AsString())
	}
	if out.SeqNr != in.SeqNr {
		t.Errorf("SeqNr: got %d, want %d", out.SeqNr, in.SeqNr)
	}
	if out.IsCreated != in.IsCreated {
		t.Errorf("IsCreated: got %v, want %v", out.IsCreated, in.IsCreated)
	}
	if !out.OccurredAt.Equal(in.OccurredAt) {
		t.Errorf("OccurredAt: got %v, want %v", out.OccurredAt, in.OccurredAt)
	}
	if string(out.Payload) != string(in.Payload) {
		t.Errorf("Payload: got %q, want %q", out.Payload, in.Payload)
	}
	if out.TraceParent != in.TraceParent {
		t.Errorf("TraceParent: got %q, want %q", out.TraceParent, in.TraceParent)
	}
	if out.TraceState != in.TraceState {
		t.Errorf("TraceState: got %q, want %q", out.TraceState, in.TraceState)
	}
}

func TestSnapshotEnvelope_jsonRoundtrip(t *testing.T) {
	occurred := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	in := es.SnapshotEnvelope{
		AggregateID: envelopeTestAggID{"Visit", "x"},
		SeqNr:       5,
		Version:     1,
		Payload:     []byte(`{"state":"scheduled"}`),
		OccurredAt:  occurred,
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var out es.SnapshotEnvelope
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if out.AggregateID.AsString() != in.AggregateID.AsString() {
		t.Errorf("AggregateID: got %q, want %q", out.AggregateID.AsString(), in.AggregateID.AsString())
	}
	if out.SeqNr != in.SeqNr {
		t.Errorf("SeqNr: got %d, want %d", out.SeqNr, in.SeqNr)
	}
	if out.Version != in.Version {
		t.Errorf("Version: got %d, want %d", out.Version, in.Version)
	}
	if string(out.Payload) != string(in.Payload) {
		t.Errorf("Payload: got %q, want %q", out.Payload, in.Payload)
	}
	if !out.OccurredAt.Equal(in.OccurredAt) {
		t.Errorf("OccurredAt: got %v, want %v", out.OccurredAt, in.OccurredAt)
	}
}
