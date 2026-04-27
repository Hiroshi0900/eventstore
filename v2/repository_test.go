package eventsourcing_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/memory"
)

// === counter ドメイン (テスト用) ===

type counterEvent interface {
	es.Event
	isCounterEvent()
}

type incrementedEvent struct {
	AggID es.AggregateID
	By    int
}

func (e incrementedEvent) EventTypeName() string       { return "Incremented" }
func (e incrementedEvent) AggregateID() es.AggregateID { return e.AggID }
func (incrementedEvent) isCounterEvent()               {}

type incrementCommand struct{ By int }

func (incrementCommand) CommandTypeName() string { return "Increment" }

type counterAggregate struct {
	id    es.AggregateID
	count int
}

func (c counterAggregate) AggregateID() es.AggregateID { return c.id }

func (c counterAggregate) ApplyCommand(cmd es.Command) (counterEvent, error) {
	switch x := cmd.(type) {
	case incrementCommand:
		return incrementedEvent{AggID: c.id, By: x.By}, nil
	default:
		return nil, es.ErrUnknownCommand
	}
}

func (c counterAggregate) ApplyEvent(ev counterEvent) es.Aggregate[counterEvent] {
	if e, ok := ev.(incrementedEvent); ok {
		return counterAggregate{id: c.id, count: c.count + e.By}
	}
	return c
}

// === Serializers ===

type counterAggregateState struct {
	TypeName string `json:"type_name"`
	Value    string `json:"value"`
	Count    int    `json:"count"`
}

type counterAggregateSerializer struct{}

func (counterAggregateSerializer) Serialize(c counterAggregate) ([]byte, error) {
	return json.Marshal(counterAggregateState{
		TypeName: c.id.TypeName(),
		Value:    c.id.Value(),
		Count:    c.count,
	})
}

func (counterAggregateSerializer) Deserialize(data []byte) (counterAggregate, error) {
	var s counterAggregateState
	if err := json.Unmarshal(data, &s); err != nil {
		return counterAggregate{}, es.NewDeserializationError("aggregate", err)
	}
	return counterAggregate{id: es.NewAggregateID(s.TypeName, s.Value), count: s.Count}, nil
}

type incrementedEventWire struct {
	AggregateType  string `json:"aggregate_type"`
	AggregateValue string `json:"aggregate_value"`
	By             int    `json:"by"`
}

type counterEventSerializer struct{}

func (counterEventSerializer) Serialize(ev counterEvent) ([]byte, error) {
	switch e := ev.(type) {
	case incrementedEvent:
		return json.Marshal(incrementedEventWire{
			AggregateType:  e.AggID.TypeName(),
			AggregateValue: e.AggID.Value(),
			By:             e.By,
		})
	default:
		return nil, es.NewSerializationError("event", errors.New("unknown event type"))
	}
}

func (counterEventSerializer) Deserialize(typeName string, data []byte) (counterEvent, error) {
	switch typeName {
	case "Incremented":
		var w incrementedEventWire
		if err := json.Unmarshal(data, &w); err != nil {
			return nil, es.NewDeserializationError("event", err)
		}
		return incrementedEvent{
			AggID: es.NewAggregateID(w.AggregateType, w.AggregateValue),
			By:    w.By,
		}, nil
	default:
		return nil, es.NewDeserializationError("event", errors.New("unknown event type: "+typeName))
	}
}

// blankCounter is the createBlank function passed to NewRepository.
func blankCounter(id es.AggregateID) counterAggregate {
	return counterAggregate{id: id, count: 0}
}

// helper: build a Repository with a fresh memory store.
func newCounterRepo(t *testing.T, cfg es.Config) (*es.DefaultRepository[counterAggregate, counterEvent], *memory.Store) {
	t.Helper()
	store := memory.New()
	repo := es.NewRepository[counterAggregate, counterEvent](
		store,
		blankCounter,
		counterAggregateSerializer{},
		counterEventSerializer{},
		cfg,
	)
	return repo, store
}

// useless test to confirm the test fixtures compile.
func TestCounterFixturesCompile(t *testing.T) {
	repo, _ := newCounterRepo(t, es.DefaultConfig())
	_ = repo
}

func TestRepository_Load_notFound(t *testing.T) {
	repo, _ := newCounterRepo(t, es.DefaultConfig())
	id := es.NewAggregateID("Counter", "missing")

	_, err := repo.Load(context.Background(), id)
	if err == nil {
		t.Fatalf("Load: want error, got nil")
	}
	if !errors.Is(err, es.ErrAggregateNotFound) {
		t.Errorf("Load: want ErrAggregateNotFound, got %v", err)
	}
}
