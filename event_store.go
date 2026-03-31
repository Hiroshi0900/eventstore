package eventsourcing

import (
	"context"
)

// EventStore provides operations for persisting and retrieving events and snapshots.
type EventStore interface {
	// GetLatestSnapshotByID retrieves the latest snapshot for the given aggregate ID.
	// Returns nil if no snapshot exists.
	GetLatestSnapshotByID(ctx context.Context, id AggregateID) (*SnapshotData, error)

	// GetEventsByIDSinceSeqNr retrieves all events since the given sequence number.
	// Events are returned in sequence number order.
	GetEventsByIDSinceSeqNr(ctx context.Context, id AggregateID, seqNr uint64) ([]Event, error)

	// PersistEvent persists a single event without a snapshot.
	// version is used for optimistic locking.
	PersistEvent(ctx context.Context, event Event, version uint64) error

	// PersistEventAndSnapshot atomically persists an event and a snapshot.
	// This operation uses optimistic locking based on the aggregate's version.
	PersistEventAndSnapshot(ctx context.Context, event Event, aggregate Aggregate) error
}

// EventStoreConfig holds the configuration for the event store.
type EventStoreConfig struct {
	// SnapshotInterval determines the number of events between snapshots.
	// Default is 5 (one snapshot every 5 events).
	SnapshotInterval uint64

	// JournalTableName is the DynamoDB table name for events.
	JournalTableName string

	// SnapshotTableName is the DynamoDB table name for snapshots.
	SnapshotTableName string

	// ShardCount is the number of shards for partition key distribution.
	ShardCount uint64

	// KeepSnapshotCount determines how many old snapshots to retain.
	// 0 means all snapshots are retained.
	KeepSnapshotCount uint64
}

// DefaultEventStoreConfig returns the default configuration.
func DefaultEventStoreConfig() EventStoreConfig {
	return EventStoreConfig{
		SnapshotInterval:  5,
		JournalTableName:  "journal",
		SnapshotTableName: "snapshot",
		ShardCount:        1,
		KeepSnapshotCount: 0,
	}
}

// ShouldSnapshot determines whether a snapshot should be created based on the sequence number.
func (c EventStoreConfig) ShouldSnapshot(seqNr uint64) bool {
	if c.SnapshotInterval == 0 {
		return false
	}
	return seqNr%c.SnapshotInterval == 0
}

// Repository provides a high-level abstraction over EventStore for aggregate operations.
type Repository[T Aggregate] interface {
	// FindByID loads an aggregate by ID.
	// Returns ErrAggregateNotFound if the aggregate does not exist.
	FindByID(ctx context.Context, id AggregateID) (T, error)

	// Save persists the aggregate and its uncommitted events.
	Save(ctx context.Context, aggregate T, events []Event) error
}

// AggregateFactory creates aggregates from events.
type AggregateFactory[T Aggregate] interface {
	// Create creates a new empty aggregate with the given ID.
	Create(id AggregateID) T

	// Apply applies an event to an aggregate and returns the updated aggregate.
	Apply(aggregate T, event Event) (T, error)

	// Serialize serializes an aggregate for snapshot storage.
	Serialize(aggregate T) ([]byte, error)

	// Deserialize deserializes a snapshot into an aggregate.
	Deserialize(data []byte) (T, error)
}

// DefaultRepository is a generic repository implementation.
type DefaultRepository[T Aggregate] struct {
	store      EventStore
	factory    AggregateFactory[T]
	serializer Serializer
	config     EventStoreConfig
}

// NewRepository creates a new DefaultRepository.
func NewRepository[T Aggregate](
	store EventStore,
	factory AggregateFactory[T],
	serializer Serializer,
	config EventStoreConfig,
) *DefaultRepository[T] {
	return &DefaultRepository[T]{
		store:      store,
		factory:    factory,
		serializer: serializer,
		config:     config,
	}
}

func (r *DefaultRepository[T]) FindByID(ctx context.Context, id AggregateID) (T, error) {
	var zero T

	// Try to load the snapshot
	snapshot, err := r.store.GetLatestSnapshotByID(ctx, id)
	if err != nil {
		return zero, err
	}

	var aggregate T
	var seqNr uint64

	if snapshot != nil {
		// Deserialize the snapshot
		agg, err := r.factory.Deserialize(snapshot.Payload)
		if err != nil {
			return zero, err
		}
		aggregate = agg.WithVersion(snapshot.Version).(T)
		seqNr = snapshot.SeqNr
	} else {
		// No snapshot, start from an empty aggregate
		aggregate = r.factory.Create(id)
		seqNr = 0
	}

	// Load events since the snapshot
	events, err := r.store.GetEventsByIDSinceSeqNr(ctx, id, seqNr)
	if err != nil {
		return zero, err
	}

	if len(events) == 0 && snapshot == nil {
		return zero, NewAggregateNotFoundError(id.GetTypeName(), id.GetValue())
	}

	// Apply events
	for _, event := range events {
		aggregate, err = r.factory.Apply(aggregate, event)
		if err != nil {
			return zero, err
		}
	}

	return aggregate, nil
}

func (r *DefaultRepository[T]) Save(ctx context.Context, aggregate T, events []Event) error {
	if len(events) == 0 {
		return nil
	}

	for _, event := range events {
		seqNr := event.GetSeqNr()

		if r.config.ShouldSnapshot(seqNr) {
			if err := r.store.PersistEventAndSnapshot(ctx, event, aggregate); err != nil {
				return err
			}
		} else {
			if err := r.store.PersistEvent(ctx, event, aggregate.GetVersion()); err != nil {
				return err
			}
		}
	}

	return nil
}
