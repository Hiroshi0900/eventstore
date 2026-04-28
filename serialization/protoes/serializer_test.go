package protoes_test

import (
	"encoding/json"
	"errors"
	"testing"

	es "github.com/Hiroshi0900/eventstore"
	"github.com/Hiroshi0900/eventstore/serialization/protoes"
)

// テスト用 typed AggregateID。
type sampleAggID struct {
	value string
}

func (s sampleAggID) TypeName() string { return "Sample" }
func (s sampleAggID) Value() string    { return s.value }
func (s sampleAggID) AsString() string { return "Sample-" + s.value }

// テスト用 Event interface (E)。
type sampleEvent interface {
	es.Event
	isSample()
}

type sampleCreated struct {
	id   sampleAggID
	name string
}

func (e sampleCreated) EventTypeName() string       { return "SampleCreated" }
func (e sampleCreated) AggregateID() es.AggregateID { return e.id }
func (sampleCreated) isSample()                     {}

// 簡易な encode / decode 関数 (実際には proto を使う想定だが、テストは JSON で代替)。
type sampleCreatedWire struct {
	IDValue string `json:"id_value"`
	Name    string `json:"name"`
}

func encode(ev sampleEvent) ([]byte, error) {
	switch e := ev.(type) {
	case sampleCreated:
		return json.Marshal(sampleCreatedWire{IDValue: e.id.value, Name: e.name})
	}
	return nil, es.NewSerializationError("event", errors.New("unknown event type"))
}

func decode(typeName string, data []byte) (sampleEvent, error) {
	switch typeName {
	case "SampleCreated":
		var w sampleCreatedWire
		if err := json.Unmarshal(data, &w); err != nil {
			return nil, es.NewDeserializationError("event", err)
		}
		return sampleCreated{id: sampleAggID{value: w.IDValue}, name: w.Name}, nil
	}
	return nil, es.NewDeserializationError("event", errors.New("unknown event type: "+typeName))
}

func TestCodec_RoundTrip(t *testing.T) {
	c := protoes.New[sampleEvent](encode, decode)
	in := sampleCreated{id: sampleAggID{value: "abc"}, name: "hello"}

	data, err := c.Serialize(in)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}

	got, err := c.Deserialize(in.EventTypeName(), data)
	if err != nil {
		t.Fatalf("Deserialize: %v", err)
	}

	gc, ok := got.(sampleCreated)
	if !ok {
		t.Fatalf("got type %T, want sampleCreated", got)
	}
	if gc.id.value != in.id.value || gc.name != in.name {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", gc, in)
	}
}

func TestCodec_DeserializeError(t *testing.T) {
	c := protoes.New[sampleEvent](encode, decode)
	_, err := c.Deserialize("Unknown", []byte(`{}`))
	if err == nil {
		t.Fatal("want error for unknown type")
	}
	if !errors.Is(err, es.ErrDeserializationFailed) {
		t.Errorf("got %v, want ErrDeserializationFailed", err)
	}
}
