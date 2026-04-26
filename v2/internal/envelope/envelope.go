// Package envelope provides wire format types for persisting events and snapshots.
package envelope

import (
	es "github.com/Hiroshi0900/eventstore/v2"
)

// EventEnvelope wraps an event with metadata for persistence.
// It stores the full event information including type information.
type EventEnvelope struct {
	ID          string `json:"id"`
	TypeName    string `json:"type_name"`
	AggregateID string `json:"aggregate_id"`
	SeqNr       uint64 `json:"seq_nr"`
	IsCreated   bool   `json:"is_created"`
	OccurredAt  int64  `json:"occurred_at"` // Unix milli (v2: time.Time → int64)
	Payload     []byte `json:"payload"`
}

// FromEvent converts an Event to an EventEnvelope.
func FromEvent(e es.Event) *EventEnvelope {
	return &EventEnvelope{
		ID:          e.EventID(),
		TypeName:    e.EventTypeName(),
		AggregateID: e.AggregateID().AsString(),
		SeqNr:       e.SeqNr(),
		IsCreated:   e.IsCreated(),
		OccurredAt:  e.OccurredAt().UnixMilli(),
		Payload:     e.Payload(),
	}
}

// SnapshotEnvelope wraps a snapshot with metadata for persistence.
type SnapshotEnvelope struct {
	AggregateID string `json:"aggregate_id"`
	TypeName    string `json:"type_name"`
	SeqNr       uint64 `json:"seq_nr"`
	Version     uint64 `json:"version"`
	Payload     []byte `json:"payload"`
}
