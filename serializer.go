package eventsourcing

import (
	"encoding/json"
)

// Serializer handles serialization and deserialization of events and aggregates.
type Serializer interface {
	// SerializeEvent serializes an event payload to bytes.
	SerializeEvent(payload any) ([]byte, error)
	// DeserializeEvent deserializes bytes into an event payload.
	DeserializeEvent(data []byte, target any) error
	// SerializeAggregate serializes an aggregate to bytes.
	SerializeAggregate(aggregate any) ([]byte, error)
	// DeserializeAggregate deserializes bytes into an aggregate.
	DeserializeAggregate(data []byte, target any) error
}

// JSONSerializer is an implementation of Serializer using JSON encoding.
type JSONSerializer struct{}

// NewJSONSerializer creates a new JSONSerializer.
func NewJSONSerializer() *JSONSerializer {
	return &JSONSerializer{}
}

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
