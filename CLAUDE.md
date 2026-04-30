# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
# Build
go build ./...

# Lint / vet
go vet ./...

# Run all tests
go test ./...

# Run tests for a single package
go test github.com/Hiroshi0900/eventstore
go test github.com/Hiroshi0900/eventstore/memory
go test github.com/Hiroshi0900/eventstore/dynamodb
go test github.com/Hiroshi0900/eventstore/internal/keyresolver

# Run a single test
go test -run TestName ./...
```

## Architecture

This is a Go event sourcing library (`module github.com/Hiroshi0900/eventstore`, package `eventsourcing`).

### Layer model

```
Consumer code
    │
    ▼
Repository[T Aggregate]          ← high-level: load/save aggregates
    │   uses
    ▼
EventStore (interface)           ← low-level: persist/fetch raw events & snapshots
    │
    ├── dynamodb.Store            ← DynamoDB-backed implementation (production)
    └── memory.Store              ← in-memory implementation (tests)
```

### Core interfaces (root package)

| Type | Role |
|---|---|
| `AggregateID` | Identity: `GetTypeName() + GetValue()` → `AsString()` = `"TypeName-value"` |
| `Aggregate` | Root entity: carries `Version` (optimistic lock) and `SeqNr` (event count) |
| `Event` | Immutable domain fact: carries `SeqNr`, `IsCreated`, `Payload []byte` |
| `EventStore` | Storage abstraction: latest snapshot + aggregate reconstruction stream, then persist event/snapshot |
| `Repository[T]` | Generic repo: `FindByID` (snapshot + replay) and `Save` (conditional snapshot) |
| `AggregateFactory[T]` | Type-specific: `Create`, `Apply`, `Serialize`, `Deserialize` |
| `Serializer` | JSON encode/decode for both events and aggregates |

### Snapshot strategy

`EventStoreConfig.SnapshotInterval` (default 5) controls when snapshots are taken. On `Save`, every event whose `seqNr % SnapshotInterval == 0` triggers `PersistEventAndSnapshot`; all others use `PersistEvent`. On `FindByID`, the latest snapshot is loaded first, then only events since that `SeqNr` are replayed.

### DynamoDB table design (`dynamodb/store.go`)

Two tables: **journal** (events) and **snapshot** (latest per aggregate).

- **Partition key** (`pkey`): `"{TypeName}-{shardID}"` — shard ID is `FNV-1a(value) % ShardCount` (see `internal/keyresolver`)
- **Sort key** (`skey`): `"{TypeName}-{value}-{seqNr:020d}"` for events, `"{TypeName}-{value}-0"` for snapshots
- `LoadStreamAfter` is the reconstruction path and must stay off the `aid` GSI; it uses a strongly consistent primary-table read so `Repository.Save()` can immediately reconstruct the aggregate after a write.
- Current key schema still means reconstruction is correctness-first rather than perfectly optimal: the implementation reads the relevant shard partitions and filters by aggregate ID to rebuild one stream safely.
- **GSI** (`aid` + `seq_nr`): retained for non-reconstruction query patterns, not for aggregate replay
- Atomic event + snapshot write uses `TransactWriteItems`
- Optimistic locking: snapshot `Put` has a `version = :expected_version` condition
- OTel `traceparent`/`tracestate` are injected into each event item from context

### Internal packages

- `internal/keyresolver` — DynamoDB key generation and FNV-1a sharding; not intended for external use
- `internal/envelope` — wire-format structs (`EventEnvelope`, `SnapshotEnvelope`) for marshalling

### Implementing a new aggregate

1. Implement `AggregateID` (or use `NewAggregateId(typeName, value)`)
2. Implement `Aggregate` (`GetId`, `GetSeqNr`, `GetVersion`, `WithVersion`, `WithSeqNr`)
3. Implement `AggregateFactory[T]` (`Create`, `Apply`, `Serialize`, `Deserialize`)
4. Instantiate: `es.NewRepository[*YourType](store, factory, es.NewJSONSerializer(), es.DefaultEventStoreConfig())`
5. For tests use `memory.New()` as the store; for production use `dynamodb.New(client, dynamodb.DefaultConfig())`
