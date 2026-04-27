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
