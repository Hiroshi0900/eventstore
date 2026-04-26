// Package protoes は Protobuf による EventSerializer 実装を提供します。
package protoes

import (
	"time"

	"google.golang.org/protobuf/proto"

	es "github.com/Hiroshi0900/eventstore/v2"
)

// ProtoEventSerializer は EventEnvelope (proto3) を使う EventSerializer 実装です。
type ProtoEventSerializer struct{}

// New は ProtoEventSerializer を生成します。
func New() *ProtoEventSerializer { return &ProtoEventSerializer{} }

// Serialize は Event を Protobuf にエンコードします。
func (s *ProtoEventSerializer) Serialize(event es.Event) ([]byte, error) {
	env := &EventEnvelope{
		EventId:       event.EventID(),
		EventTypeName: event.EventTypeName(),
		IdTypeName:    event.AggregateID().TypeName(),
		IdValue:       event.AggregateID().Value(),
		SeqNr:         event.SeqNr(),
		IsCreated:     event.IsCreated(),
		OccurredAtMs:  event.OccurredAt().UnixMilli(),
		Payload:       event.Payload(),
	}
	data, err := proto.Marshal(env)
	if err != nil {
		return nil, es.NewSerializationError("event", err)
	}
	return data, nil
}

// Deserialize は Protobuf を Event にデコードします。
func (s *ProtoEventSerializer) Deserialize(data []byte) (es.Event, error) {
	var env EventEnvelope
	if err := proto.Unmarshal(data, &env); err != nil {
		return nil, es.NewDeserializationError("event", err)
	}
	aggID := es.NewAggregateID(env.GetIdTypeName(), env.GetIdValue())
	return es.NewEvent(
		env.GetEventId(),
		env.GetEventTypeName(),
		aggID,
		env.GetPayload(),
		es.WithSeqNr(env.GetSeqNr()),
		es.WithIsCreated(env.GetIsCreated()),
		es.WithOccurredAt(time.UnixMilli(env.GetOccurredAtMs())),
	), nil
}

// コンパイル時 interface 適合確認。
var _ es.EventSerializer = (*ProtoEventSerializer)(nil)
