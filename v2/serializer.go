package eventstore

import (
	"encoding/json"
	"time"

	"github.com/Hiroshi0900/eventstore/v2/internal/aggregateid"
)

// EventSerializer は Event 全体（メタデータ + payload）の (de)serialization を担います。
type EventSerializer interface {
	Serialize(event Event) ([]byte, error)
	Deserialize(data []byte) (Event, error)
}

// JSONEventSerializer は JSON 形式の EventSerializer 実装です。
type JSONEventSerializer struct{}

// NewJSONEventSerializer は JSONEventSerializer を生成します。
func NewJSONEventSerializer() *JSONEventSerializer { return &JSONEventSerializer{} }

// jsonEventEnvelope は JSON wire format。AggregateID は TypeName/Value 別フィールドで保持。
type jsonEventEnvelope struct {
	EventID       string `json:"event_id"`
	EventTypeName string `json:"event_type_name"`
	IDTypeName    string `json:"id_type_name"`
	IDValue       string `json:"id_value"`
	SeqNr         uint64 `json:"seq_nr"`
	IsCreated     bool   `json:"is_created"`
	OccurredAtMs  int64  `json:"occurred_at_ms"`
	Payload       []byte `json:"payload"`
}

// Serialize は Event を JSON にエンコードします。
func (s *JSONEventSerializer) Serialize(event Event) ([]byte, error) {
	env := jsonEventEnvelope{
		EventID:       event.EventID(),
		EventTypeName: event.EventTypeName(),
		IDTypeName:    event.AggregateID().TypeName(),
		IDValue:       event.AggregateID().Value(),
		SeqNr:         event.SeqNr(),
		IsCreated:     event.IsCreated(),
		OccurredAtMs:  event.OccurredAt().UnixMilli(),
		Payload:       event.Payload(),
	}
	data, err := json.Marshal(env)
	if err != nil {
		return nil, NewSerializationError("event", err)
	}
	return data, nil
}

// Deserialize は JSON を Event にデコードします。
func (s *JSONEventSerializer) Deserialize(data []byte) (Event, error) {
	var env jsonEventEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, NewDeserializationError("event", err)
	}
	aggID := aggregateid.New(env.IDTypeName, env.IDValue)
	ev := NewEvent(
		env.EventID,
		env.EventTypeName,
		aggID,
		env.Payload,
		WithSeqNr(env.SeqNr),
		WithIsCreated(env.IsCreated),
		WithOccurredAt(time.UnixMilli(env.OccurredAtMs)),
	)
	return ev, nil
}

// コンパイル時 interface 適合確認。
var _ EventSerializer = (*JSONEventSerializer)(nil)
