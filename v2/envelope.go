package eventstore

import (
	"encoding/json"
	"time"
)

// EventEnvelope wraps a serialized domain event with all storage metadata.
// Repository ↔ EventStore のやり取りはこの型を経由する。
//
// ドメイン側の Event interface はビジネス情報のみを持ち、SeqNr / EventID /
// OccurredAt / IsCreated / Payload bytes / TraceParent / TraceState などの
// 永続化メタ情報は本 Envelope に集約される。
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

// eventEnvelopeJSON は EventEnvelope の wire format。AggregateID は interface
// なので json tag が直接付けられず、private struct を経由する。
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

// MarshalJSON encodes the envelope into the wire format.
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

// UnmarshalJSON decodes the wire format. AggregateID は library 内部実装の
// simpleAggregateID として復元される。利用側は interface 経由で扱う。
func (e *EventEnvelope) UnmarshalJSON(data []byte) error {
	var j eventEnvelopeJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	e.EventID = j.EventID
	e.EventTypeName = j.EventTypeName
	e.AggregateID = simpleAggregateID{typeName: j.AggregateType, value: j.AggregateValue}
	e.SeqNr = j.SeqNr
	e.IsCreated = j.IsCreated
	e.OccurredAt = j.OccurredAt
	e.Payload = j.Payload
	e.TraceParent = j.TraceParent
	e.TraceState = j.TraceState
	return nil
}

// SnapshotEnvelope wraps a serialized aggregate state with metadata.
// Repository が `AggregateSerializer[T,E]` で aggregate を bytes 化し、
// EventStore に渡す際の wire format。
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
	s.AggregateID = simpleAggregateID{typeName: j.AggregateType, value: j.AggregateValue}
	s.SeqNr = j.SeqNr
	s.Version = j.Version
	s.Payload = j.Payload
	s.OccurredAt = j.OccurredAt
	return nil
}

// simpleAggregateID は envelope deserialize や dynamodb attribute unmarshal で
// 使われる library 内部の AggregateID 実装。利用側は独自 typed ID を定義する
// 想定なので公開しない。
type simpleAggregateID struct {
	typeName string
	value    string
}

func (s simpleAggregateID) TypeName() string { return s.typeName }
func (s simpleAggregateID) Value() string    { return s.value }
func (s simpleAggregateID) AsString() string { return s.typeName + "-" + s.value }
