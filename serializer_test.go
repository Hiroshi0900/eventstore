package eventsourcing

import (
	"errors"
	"testing"
)

func TestJSONSerializer(t *testing.T) {
	t.Run("SerializeEvent serializes struct to JSON", func(t *testing.T) {
		// given
		sut := NewJSONSerializer()
		payload := struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}{
			Name:  "test",
			Value: 42,
		}

		// when
		data, err := sut.SerializeEvent(payload)

		// then
		if err != nil {
			t.Fatalf("SerializeEvent() error = %v", err)
		}
		expected := `{"name":"test","value":42}`
		if string(data) != expected {
			t.Errorf("SerializeEvent() = %q, want %q", string(data), expected)
		}
	})

	t.Run("DeserializeEvent deserializes JSON to struct", func(t *testing.T) {
		// given
		sut := NewJSONSerializer()
		data := []byte(`{"name":"test","value":42}`)
		var target struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}

		// when
		err := sut.DeserializeEvent(data, &target)

		// then
		if err != nil {
			t.Fatalf("DeserializeEvent() error = %v", err)
		}
		if target.Name != "test" {
			t.Errorf("Name = %q, want %q", target.Name, "test")
		}
		if target.Value != 42 {
			t.Errorf("Value = %d, want %d", target.Value, 42)
		}
	})

	t.Run("SerializeAggregate serializes aggregate state", func(t *testing.T) {
		// given
		sut := NewJSONSerializer()
		aggregate := map[string]interface{}{
			"id":     "abc123",
			"status": "active",
		}

		// when
		data, err := sut.SerializeAggregate(aggregate)

		// then
		if err != nil {
			t.Fatalf("SerializeAggregate() error = %v", err)
		}
		if len(data) == 0 {
			t.Error("SerializeAggregate() returned empty data")
		}
	})

	t.Run("DeserializeAggregate deserializes aggregate state", func(t *testing.T) {
		// given
		sut := NewJSONSerializer()
		data := []byte(`{"id":"abc123","status":"active"}`)
		var target map[string]interface{}

		// when
		err := sut.DeserializeAggregate(data, &target)

		// then
		if err != nil {
			t.Fatalf("DeserializeAggregate() error = %v", err)
		}
		if target["id"] != "abc123" {
			t.Errorf("id = %v, want %q", target["id"], "abc123")
		}
	})

	t.Run("SerializeEvent returns error for invalid input", func(t *testing.T) {
		// given
		sut := NewJSONSerializer()
		ch := make(chan int) // Channels cannot be serialized to JSON

		// when
		_, err := sut.SerializeEvent(ch)

		// then
		if err == nil {
			t.Error("SerializeEvent() should return error for channel")
		}
		if !errors.Is(err, ErrSerializationFailed) {
			t.Errorf("error should be ErrSerializationFailed, got %v", err)
		}
	})

	t.Run("DeserializeEvent returns error for invalid JSON", func(t *testing.T) {
		// given
		sut := NewJSONSerializer()
		data := []byte(`{invalid json}`)
		var target struct{}

		// when
		err := sut.DeserializeEvent(data, &target)

		// then
		if err == nil {
			t.Error("DeserializeEvent() should return error for invalid JSON")
		}
		if !errors.Is(err, ErrDeserializationFailed) {
			t.Errorf("error should be ErrDeserializationFailed, got %v", err)
		}
	})
}
