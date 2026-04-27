package eventsourcing

import "context"

// EventStore is the low-level persistence abstraction.
// All operations work on Envelopes; domain types are not exposed at this layer.
type EventStore interface {
	// GetLatestSnapshot returns the most recent snapshot for the aggregate,
	// or nil if no snapshot exists.
	GetLatestSnapshot(ctx context.Context, id AggregateID) (*SnapshotEnvelope, error)

	// GetEventsSince returns all events with SeqNr > seqNr, ordered by SeqNr.
	GetEventsSince(ctx context.Context, id AggregateID, seqNr uint64) ([]*EventEnvelope, error)

	// PersistEvent appends a single event without a snapshot.
	// expectedVersion is used only for first-write detection (== 0 means
	// "this aggregate must not exist yet"). It is NOT used as an optimistic
	// lock; that is performed by PersistEventAndSnapshot.
	// The store rejects events with duplicate (AggregateID, SeqNr) via
	// ErrDuplicateAggregate.
	PersistEvent(ctx context.Context, ev *EventEnvelope, expectedVersion uint64) error

	// PersistEventAndSnapshot atomically appends an event and updates the
	// snapshot. The snapshot's Version field is used for optimistic locking:
	// the operation fails with ErrOptimisticLock if the stored snapshot's
	// version != snap.Version - 1 (i.e. the version was incremented by
	// someone else in parallel).
	PersistEventAndSnapshot(ctx context.Context, ev *EventEnvelope, snap *SnapshotEnvelope) error
}

// Config holds configuration for the EventStore.
type Config struct {
	// SnapshotInterval determines the number of events between snapshots.
	// Default 5. A value of 0 disables snapshots entirely.
	SnapshotInterval uint64

	// JournalTableName is the table name for events (DynamoDB only).
	JournalTableName string

	// SnapshotTableName is the table name for snapshots (DynamoDB only).
	SnapshotTableName string

	// ShardCount controls FNV-1a-based partition key sharding (DynamoDB only).
	ShardCount uint64

	// KeepSnapshotCount determines how many old snapshots to retain.
	// 0 means all snapshots are retained.
	KeepSnapshotCount uint64
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		SnapshotInterval:  5,
		JournalTableName:  "journal",
		SnapshotTableName: "snapshot",
		ShardCount:        1,
		KeepSnapshotCount: 0,
	}
}

// ShouldSnapshot returns true when seqNr triggers a snapshot.
func (c Config) ShouldSnapshot(seqNr uint64) bool {
	if c.SnapshotInterval == 0 {
		return false
	}
	return seqNr%c.SnapshotInterval == 0
}
