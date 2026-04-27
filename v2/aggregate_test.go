package eventsourcing_test

import (
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
)

func TestNewAggregateID(t *testing.T) {
	id := es.NewAggregateID("Visit", "01J0000000")

	if got := id.TypeName(); got != "Visit" {
		t.Errorf("TypeName(): got %q, want %q", got, "Visit")
	}
	if got := id.Value(); got != "01J0000000" {
		t.Errorf("Value(): got %q, want %q", got, "01J0000000")
	}
	if got := id.AsString(); got != "Visit-01J0000000" {
		t.Errorf("AsString(): got %q, want %q", got, "Visit-01J0000000")
	}
}

// mockEvent + mockAggregate は Aggregate[E] interface 契約を確認するためのテスト用 stub
type mockEvent struct {
	typeName string
	aggID    es.AggregateID
}

func (e mockEvent) EventTypeName() string       { return e.typeName }
func (e mockEvent) AggregateID() es.AggregateID { return e.aggID }

type mockCommand struct {
	name string
}

func (c mockCommand) CommandTypeName() string { return c.name }

type mockAggregate struct {
	id es.AggregateID
}

func (a mockAggregate) AggregateID() es.AggregateID { return a.id }

func (a mockAggregate) ApplyCommand(cmd es.Command) (mockEvent, error) {
	return mockEvent{typeName: cmd.CommandTypeName() + "Applied", aggID: a.id}, nil
}

func (a mockAggregate) ApplyEvent(_ mockEvent) es.Aggregate[mockEvent] {
	return a
}

func TestAggregateInterface_canBeImplementedWithStatePattern(t *testing.T) {
	id := es.NewAggregateID("Mock", "x")
	var agg es.Aggregate[mockEvent] = mockAggregate{id: id}

	ev, err := agg.ApplyCommand(mockCommand{name: "Do"})
	if err != nil {
		t.Fatalf("ApplyCommand() error = %v", err)
	}
	if got := ev.EventTypeName(); got != "DoApplied" {
		t.Errorf("event type: got %q, want %q", got, "DoApplied")
	}
	if got := ev.AggregateID().AsString(); got != "Mock-x" {
		t.Errorf("event aggID: got %q, want %q", got, "Mock-x")
	}

	next := agg.ApplyEvent(ev)
	if got := next.AggregateID().AsString(); got != "Mock-x" {
		t.Errorf("next aggID: got %q, want %q", got, "Mock-x")
	}
}
