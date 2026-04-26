// Package keyresolver provides DynamoDB partition key and sort key resolution
// with optional FNV-1a hash-based sharding.
package keyresolver

import (
	"fmt"
	"hash/fnv"

	es "github.com/Hiroshi0900/eventstore/v2"
)

// KeyComponents holds the resolved key components for an event.
type KeyComponents struct {
	PartitionKey   string
	SortKey        string
	AggregateIDKey string
}

// Config holds the configuration for the key resolver.
type Config struct {
	// ShardCount is the number of shards for partition key distribution.
	// Default is 1 (no sharding).
	ShardCount uint64
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		ShardCount: 1,
	}
}

// Resolver implements DynamoDB key resolution using FNV-1a hash-based sharding.
type Resolver struct {
	config Config
}

// New creates a new Resolver with the given configuration.
func New(config Config) *Resolver {
	if config.ShardCount == 0 {
		config.ShardCount = 1
	}
	return &Resolver{config: config}
}

// NewDefault creates a new Resolver with default configuration.
func NewDefault() *Resolver {
	return New(DefaultConfig())
}

// ResolvePartitionKey generates the partition key for sharding.
func (r *Resolver) ResolvePartitionKey(id es.AggregateID) string {
	shardID := r.computeShardID(id.Value())
	return fmt.Sprintf("%s-%d", id.TypeName(), shardID)
}

// ResolveSortKeyForEvent generates the sort key for an event.
func (r *Resolver) ResolveSortKeyForEvent(id es.AggregateID, seqNr uint64) string {
	return fmt.Sprintf("%s-%s-%020d", id.TypeName(), id.Value(), seqNr)
}

// ResolveSortKeyForSnapshot generates the sort key for a snapshot.
func (r *Resolver) ResolveSortKeyForSnapshot(id es.AggregateID) string {
	// Snapshots use sequence number 0 to indicate "latest".
	return fmt.Sprintf("%s-%s-0", id.TypeName(), id.Value())
}

// ResolveAggregateIDKey generates the aggregate ID key for GSI queries.
func (r *Resolver) ResolveAggregateIDKey(id es.AggregateID) string {
	return id.AsString()
}

// ResolveEventKeys resolves all key components for an event.
func (r *Resolver) ResolveEventKeys(id es.AggregateID, seqNr uint64) KeyComponents {
	return KeyComponents{
		PartitionKey:   r.ResolvePartitionKey(id),
		SortKey:        r.ResolveSortKeyForEvent(id, seqNr),
		AggregateIDKey: r.ResolveAggregateIDKey(id),
	}
}

// ResolveSnapshotKeys resolves all key components for a snapshot.
func (r *Resolver) ResolveSnapshotKeys(id es.AggregateID) KeyComponents {
	return KeyComponents{
		PartitionKey:   r.ResolvePartitionKey(id),
		SortKey:        r.ResolveSortKeyForSnapshot(id),
		AggregateIDKey: r.ResolveAggregateIDKey(id),
	}
}

// GetShardCount returns the configured shard count.
func (r *Resolver) GetShardCount() uint64 {
	return r.config.ShardCount
}

// computeShardID computes the shard ID using FNV-1a hashing.
func (r *Resolver) computeShardID(value string) uint64 {
	if r.config.ShardCount <= 1 {
		return 0
	}
	h := fnv.New64a()
	if _, err := h.Write([]byte(value)); err != nil {
		// fnv.Write never returns an error, but handle safely
		return 0
	}
	return h.Sum64() % r.config.ShardCount
}
