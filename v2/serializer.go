package eventstore

import "encoding/json"

// Serializer は汎用シリアライザの抽象です。Event payload と Aggregate snapshot の両方を扱います。
type Serializer interface {
	SerializeEvent(payload any) ([]byte, error)
	DeserializeEvent(data []byte, target any) error
	SerializeAggregate(aggregate any) ([]byte, error)
	DeserializeAggregate(data []byte, target any) error
}

// JSONSerializer は JSON を使う Serializer 実装です。
type JSONSerializer struct{}

func NewJSONSerializer() *JSONSerializer { return &JSONSerializer{} }

func (s *JSONSerializer) SerializeEvent(payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, NewSerializationError("event", err)
	}
	return data, nil
}

func (s *JSONSerializer) DeserializeEvent(data []byte, target any) error {
	if err := json.Unmarshal(data, target); err != nil {
		return NewDeserializationError("event", err)
	}
	return nil
}

func (s *JSONSerializer) SerializeAggregate(aggregate any) ([]byte, error) {
	data, err := json.Marshal(aggregate)
	if err != nil {
		return nil, NewSerializationError("aggregate", err)
	}
	return data, nil
}

func (s *JSONSerializer) DeserializeAggregate(data []byte, target any) error {
	if err := json.Unmarshal(data, target); err != nil {
		return NewDeserializationError("aggregate", err)
	}
	return nil
}
