// Package envelope provides wire format types for persisting events and snapshots.
package envelope

import (
	es "github.com/Hiroshi0900/eventstore"
)

// EventEnvelope wraps an event with metadata for persistence.
// It stores the full event information including type information.
type EventEnvelope struct {
	Id          string `json:"id"`
	TypeName    string `json:"type_name"`
	AggregateId string `json:"aggregate_id"`
	SeqNr       uint64 `json:"seq_nr"`
	IsCreated   bool   `json:"is_created"`
	OccurredAt  uint64 `json:"occurred_at"`
	Payload     []byte `json:"payload"`
}

// FromEvent converts an Event to an EventEnvelope.
func FromEvent(e es.Event) *EventEnvelope {
	return &EventEnvelope{
		Id:          e.GetID(),
		TypeName:    e.GetTypeName(),
		AggregateId: e.GetAggregateId().AsString(),
		SeqNr:       e.GetSeqNr(),
		IsCreated:   e.IsCreated(),
		OccurredAt:  e.GetOccurredAt(),
		Payload:     e.GetPayload(),
	}
}

// SnapshotEnvelope wraps a snapshot with metadata for persistence.
type SnapshotEnvelope struct {
	AggregateId string `json:"aggregate_id"`
	TypeName    string `json:"type_name"`
	SeqNr       uint64 `json:"seq_nr"`
	Version     uint64 `json:"version"`
	Payload     []byte `json:"payload"`
}
