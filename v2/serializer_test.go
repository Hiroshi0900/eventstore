package eventstore

import (
	"errors"
	"testing"
)

type testPayload struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestJSONSerializer_RoundTrip(t *testing.T) {
	s := NewJSONSerializer()
	src := testPayload{Name: "alice", Age: 30}

	data, err := s.SerializeEvent(src)
	if err != nil {
		t.Fatalf("SerializeEvent err: %v", err)
	}

	var got testPayload
	if err := s.DeserializeEvent(data, &got); err != nil {
		t.Fatalf("DeserializeEvent err: %v", err)
	}
	if got != src {
		t.Errorf("round trip mismatch: got=%+v, want=%+v", got, src)
	}
}

func TestJSONSerializer_DeserializeError(t *testing.T) {
	s := NewJSONSerializer()
	var dst testPayload
	err := s.DeserializeEvent([]byte("{invalid"), &dst)
	if !errors.Is(err, ErrDeserializationFailed) {
		t.Errorf("expected ErrDeserializationFailed, got %v", err)
	}
}
