package eventsourcing

import (
	"encoding/json"
	"time"
)

// EventEnvelope wraps a serialized domain event with all storage metadata.
// This is the wire format used between Repository and EventStore.
type EventEnvelope struct {
	EventID       string
	EventTypeName string
	AggregateID   AggregateID
	SeqNr         uint64
	IsCreated     bool
	OccurredAt    time.Time
	Payload       []byte
	TraceParent   string
	TraceState    string
}

// envelope JSON format. AggregateID is split into typeName/value.
type eventEnvelopeJSON struct {
	EventID        string    `json:"event_id"`
	EventTypeName  string    `json:"event_type_name"`
	AggregateType  string    `json:"aggregate_type"`
	AggregateValue string    `json:"aggregate_value"`
	SeqNr          uint64    `json:"seq_nr"`
	IsCreated      bool      `json:"is_created"`
	OccurredAt     time.Time `json:"occurred_at"`
	Payload        []byte    `json:"payload"`
	TraceParent    string    `json:"traceparent,omitempty"`
	TraceState     string    `json:"tracestate,omitempty"`
}

func (e EventEnvelope) MarshalJSON() ([]byte, error) {
	return json.Marshal(eventEnvelopeJSON{
		EventID:        e.EventID,
		EventTypeName:  e.EventTypeName,
		AggregateType:  e.AggregateID.TypeName(),
		AggregateValue: e.AggregateID.Value(),
		SeqNr:          e.SeqNr,
		IsCreated:      e.IsCreated,
		OccurredAt:     e.OccurredAt,
		Payload:        e.Payload,
		TraceParent:    e.TraceParent,
		TraceState:     e.TraceState,
	})
}

func (e *EventEnvelope) UnmarshalJSON(data []byte) error {
	var j eventEnvelopeJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	e.EventID = j.EventID
	e.EventTypeName = j.EventTypeName
	e.AggregateID = NewAggregateID(j.AggregateType, j.AggregateValue)
	e.SeqNr = j.SeqNr
	e.IsCreated = j.IsCreated
	e.OccurredAt = j.OccurredAt
	e.Payload = j.Payload
	e.TraceParent = j.TraceParent
	e.TraceState = j.TraceState
	return nil
}

// SnapshotEnvelope wraps a serialized aggregate state with metadata.
type SnapshotEnvelope struct {
	AggregateID AggregateID
	SeqNr       uint64
	Version     uint64
	Payload     []byte
	OccurredAt  time.Time
}

type snapshotEnvelopeJSON struct {
	AggregateType  string    `json:"aggregate_type"`
	AggregateValue string    `json:"aggregate_value"`
	SeqNr          uint64    `json:"seq_nr"`
	Version        uint64    `json:"version"`
	Payload        []byte    `json:"payload"`
	OccurredAt     time.Time `json:"occurred_at"`
}

func (s SnapshotEnvelope) MarshalJSON() ([]byte, error) {
	return json.Marshal(snapshotEnvelopeJSON{
		AggregateType:  s.AggregateID.TypeName(),
		AggregateValue: s.AggregateID.Value(),
		SeqNr:          s.SeqNr,
		Version:        s.Version,
		Payload:        s.Payload,
		OccurredAt:     s.OccurredAt,
	})
}

func (s *SnapshotEnvelope) UnmarshalJSON(data []byte) error {
	var j snapshotEnvelopeJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	s.AggregateID = NewAggregateID(j.AggregateType, j.AggregateValue)
	s.SeqNr = j.SeqNr
	s.Version = j.Version
	s.Payload = j.Payload
	s.OccurredAt = j.OccurredAt
	return nil
}
