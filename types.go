// Package eventsourcing provides a lightweight library for event sourcing.
// It uses DynamoDB as the event store and supports event-sourced aggregates.
package eventsourcing

import (
	"fmt"
	"time"
)

// AggregateID represents the unique identifier of an aggregate.
// It combines a type name (e.g. "MemorialSetting") with a unique value (e.g. a ULID).
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
type AggregateID interface {
	// GetTypeName returns the type name of the aggregate (e.g. "MemorialSetting").
	GetTypeName() string
	// GetValue returns the unique value of the aggregate ID.
	GetValue() string
	// AsString returns the full string representation: "{TypeName}-{Value}".
	AsString() string
}

// DefaultAggregateID is the default implementation of AggregateID.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
type DefaultAggregateID struct {
	typeName string
	value    string
}

// NewAggregateId creates a new DefaultAggregateID.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func NewAggregateId(typeName, value string) *DefaultAggregateID {
	return &DefaultAggregateID{
		typeName: typeName,
		value:    value,
	}
}

// GetTypeName returns the type name of the aggregate.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (id *DefaultAggregateID) GetTypeName() string {
	return id.typeName
}

// GetValue returns the unique value of the aggregate ID.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (id *DefaultAggregateID) GetValue() string {
	return id.value
}

// AsString returns the full string representation of the aggregate ID.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (id *DefaultAggregateID) AsString() string {
	return fmt.Sprintf("%s-%s", id.typeName, id.value)
}

// Event represents a domain event that has occurred.
// Events are immutable and represent facts about what happened in the past.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
type Event interface {
	// GetID returns the unique identifier of this event.
	GetID() string
	// GetTypeName returns the type name of this event (e.g. "SettingCreated").
	GetTypeName() string
	// GetAggregateId returns the aggregate this event belongs to.
	GetAggregateId() AggregateID
	// GetSeqNr returns the sequence number of this event within the aggregate.
	GetSeqNr() uint64
	// IsCreated returns whether this is a creation event (the first event).
	IsCreated() bool
	// GetOccurredAt returns the timestamp when this event occurred (Unix milliseconds).
	GetOccurredAt() uint64
	// GetPayload returns the event payload as a byte slice.
	GetPayload() []byte
}

// EventOption is a functional option for configuring an event.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
type EventOption func(*DefaultEvent)

// WithSeqNr sets the sequence number of the event.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func WithSeqNr(seqNr uint64) EventOption {
	return func(e *DefaultEvent) {
		e.seqNr = seqNr
	}
}

// WithIsCreated sets whether this is a creation event.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func WithIsCreated(isCreated bool) EventOption {
	return func(e *DefaultEvent) {
		e.isCreated = isCreated
	}
}

// WithOccurredAt sets the occurred-at timestamp.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func WithOccurredAt(t time.Time) EventOption {
	return func(e *DefaultEvent) {
		// #nosec G115 -- UnixMilli assumes time is after 1970
		e.occurredAt = uint64(t.UnixMilli())
	}
}

// WithOccurredAtUnixMilli sets the occurred-at timestamp as Unix milliseconds.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func WithOccurredAtUnixMilli(unixMilli uint64) EventOption {
	return func(e *DefaultEvent) {
		e.occurredAt = unixMilli
	}
}

// DefaultEvent is the default implementation of Event.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
type DefaultEvent struct {
	id          string
	typeName    string
	aggregateId AggregateID
	seqNr       uint64
	isCreated   bool
	occurredAt  uint64
	payload     []byte
}

// NewEvent creates a new DefaultEvent.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func NewEvent(
	id string,
	typeName string,
	aggregateId AggregateID,
	payload []byte,
	opts ...EventOption,
) *DefaultEvent {
	e := &DefaultEvent{
		id:          id,
		typeName:    typeName,
		aggregateId: aggregateId,
		seqNr:       1, // default; should be set explicitly
		isCreated:   false,
		// #nosec G115 -- UnixMilli assumes time is after 1970
		occurredAt: uint64(time.Now().UnixMilli()),
		payload:    payload,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// GetID returns the unique identifier of this event.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *DefaultEvent) GetID() string {
	return e.id
}

// GetTypeName returns the type name of this event.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *DefaultEvent) GetTypeName() string {
	return e.typeName
}

// GetAggregateId returns the aggregate this event belongs to.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *DefaultEvent) GetAggregateId() AggregateID {
	return e.aggregateId
}

// GetSeqNr returns the sequence number of this event within the aggregate.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *DefaultEvent) GetSeqNr() uint64 {
	return e.seqNr
}

// IsCreated returns whether this is a creation event.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *DefaultEvent) IsCreated() bool {
	return e.isCreated
}

// GetOccurredAt returns the timestamp when this event occurred.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *DefaultEvent) GetOccurredAt() uint64 {
	return e.occurredAt
}

// GetPayload returns the event payload as a byte slice.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *DefaultEvent) GetPayload() []byte {
	return e.payload
}

// Aggregate represents an event-sourced aggregate root.
// It holds version information for optimistic locking.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
type Aggregate interface {
	// GetId returns the unique identifier of the aggregate.
	GetId() AggregateID
	// GetSeqNr returns the current sequence number (event count).
	GetSeqNr() uint64
	// GetVersion returns the version for optimistic locking.
	GetVersion() uint64
	// WithVersion returns a new aggregate with the given version.
	WithVersion(version uint64) Aggregate
	// WithSeqNr returns a new aggregate with the given sequence number.
	WithSeqNr(seqNr uint64) Aggregate
}

// AggregateResult represents the result of loading an aggregate from the store.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
type AggregateResult struct {
	// Aggregate is the loaded aggregate, or nil if not found.
	Aggregate Aggregate
	// SeqNr is the sequence number from the snapshot.
	SeqNr uint64
	// Version is the version from the snapshot.
	Version uint64
}

// SnapshotData holds serialized snapshot data.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
type SnapshotData struct {
	Payload []byte
	SeqNr   uint64
	Version uint64
}
