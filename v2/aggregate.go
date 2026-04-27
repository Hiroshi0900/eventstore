// Package eventsourcing v2 provides a lightweight library for event sourcing
// with domain types decoupled from storage metadata.
package eventsourcing

import "fmt"

// AggregateID represents the unique identifier of an aggregate.
type AggregateID interface {
	TypeName() string
	Value() string
	AsString() string
}

// DefaultAggregateID is the default implementation of AggregateID.
type DefaultAggregateID struct {
	typeName string
	value    string
}

// NewAggregateID creates a new DefaultAggregateID.
func NewAggregateID(typeName, value string) DefaultAggregateID {
	return DefaultAggregateID{typeName: typeName, value: value}
}

func (id DefaultAggregateID) TypeName() string { return id.typeName }
func (id DefaultAggregateID) Value() string    { return id.value }
func (id DefaultAggregateID) AsString() string {
	return fmt.Sprintf("%s-%s", id.typeName, id.value)
}

// Aggregate is the root entity of an event-sourced state machine.
// E is the aggregate-specific Event type (e.g. VisitEvent).
//
// Aggregate carries no storage metadata: SeqNr, Version, EventID, OccurredAt
// are managed by Repository / EventEnvelope at the library layer.
type Aggregate[E Event] interface {
	// AggregateID returns the unique identifier of this aggregate.
	AggregateID() AggregateID

	// ApplyCommand validates a command against the current state and
	// produces a single domain Event. Returns ErrUnknownCommand if the
	// command is not applicable to the current state.
	ApplyCommand(Command) (E, error)

	// ApplyEvent applies a domain event and returns the next aggregate state.
	// The returned value may be a different concrete type (state pattern).
	ApplyEvent(E) Aggregate[E]
}
