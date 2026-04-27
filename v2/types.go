// Package eventstore は Event Sourcing を実現するためのライブラリです (v2)。
//
// ドメイン側の Aggregate / Event / Command interface はビジネス情報のみを
// 持ち、SeqNr / Version / EventID / OccurredAt / IsCreated / Payload bytes
// などの永続化メタ情報は library 側の EventEnvelope / SnapshotEnvelope に
// 集約される。これにより利用側は「業務概念としての state と振る舞い」のみ
// を実装すればよい。
package eventstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

// === Domain interfaces ===

// 主要なドメインインターフェース。具象型はすべて利用側で定義する想定で、
// library は default 実装 (DefaultAggregateID, BaseAggregate 等) を提供しない。
type (
	// AggregateID は集約の識別子を表す。利用側は domain ごとに typed な
	// 実装を定義する (例: type VisitID struct{...} で TypeName/Value/AsString を実装)。
	AggregateID interface {
		TypeName() string
		Value() string
		AsString() string
	}

	// Aggregate は Event Sourcing における集約ルート。
	// E は集約専用の Event 型（利用側で定義する domain Event interface）。
	//
	// メタ情報 (SeqNr / Version / 永続化メタ) を一切持たず、ApplyCommand /
	// ApplyEvent で表現される業務状態遷移のみを定義する。
	//
	// ApplyEvent の戻り値が Aggregate[E] (interface) なので、状態遷移時に
	// 異なる具象型を返す state pattern が可能 (例: VisitScheduled → VisitCompleted)。
	Aggregate[E Event] interface {
		AggregateID() AggregateID
		ApplyCommand(Command) (E, error)
		ApplyEvent(E) Aggregate[E]
	}

	// Command はドメインの意図を表す。
	// AggregateID は持たない (Repository.Save が aggID を別引数で受け取るため冗長)。
	Command interface {
		CommandTypeName() string
	}

	// Event は過去に発生した不変のドメインイベント。
	// EventTypeName は EventSerializer の dispatch / OTel span name / 監査用。
	// 永続化メタ (EventID / SeqNr / IsCreated / OccurredAt / Payload bytes)
	// は EventEnvelope に集約される。
	Event interface {
		EventTypeName() string
		AggregateID() AggregateID
	}
)

// === Envelopes (storage metadata carriers) ===

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

// === Envelope JSON wire format ===

// eventEnvelopeJSON は EventEnvelope の JSON wire format。AggregateID は interface
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

// snapshotEnvelopeJSON is the JSON wire format for SnapshotEnvelope.
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

// simpleAggregateID は envelope JSON deserialize で使われる library 内部の
// AggregateID 実装。利用側は独自 typed ID を定義する想定なので公開しない。
type simpleAggregateID struct {
	typeName string
	value    string
}

func (s simpleAggregateID) TypeName() string { return s.typeName }
func (s simpleAggregateID) Value() string    { return s.value }
func (s simpleAggregateID) AsString() string { return s.typeName + "-" + s.value }

// === Internal helpers ===

// generateEventID は 16 random bytes から 32-char hex string を生成する。
// EventEnvelope.EventID の発行に Repository から呼ばれる。実用上 ULID/UUIDv4
// と同等の一意性を持つ。
func generateEventID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
