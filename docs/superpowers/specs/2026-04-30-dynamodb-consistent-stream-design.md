# DynamoDB Consistent Stream Design

## Summary

Issue #8 exposes a contract mismatch in the library design. `Repository.Save()` assumes that a successful write is immediately visible to the next aggregate reconstruction, but the DynamoDB implementation currently reads events through a GSI, which is eventually consistent on real AWS.

We will preserve the current library goal: application code should work only with domain `Aggregate`, `Command`, and `Event` types, while persistence metadata and backend consistency details remain hidden inside the library.

The design direction is to strengthen the `EventStore` read contract around aggregate reconstruction and implement that contract correctly in `dynamodb.Store`, rather than introducing application-visible sequence/version handling APIs.

## Decision

Adopt the following design:

- Keep `Repository.Save(ctx, aggID, cmd)` as the primary public write API.
- Preserve the rule that application code does not manage `SeqNr`, `Version`, `StoredEvent`, or backend-specific consistency workarounds.
- Replace the weak "event list query" model with a stronger aggregate-stream reconstruction contract in `EventStore`.
- Rename the reconstruction read API from `GetEventsSince` to `LoadStreamAfter` to make its purpose explicit.
- Implement `LoadStreamAfter` in the DynamoDB backend using a primary-table, strongly consistent read path rather than the `aid` GSI.

This is an intentional breaking change in the library internals and exported store abstraction. The break is acceptable because the current contract is not strong enough to uphold the repository semantics that the library promises.

## Problem Statement

The current layering says:

- `Repository` restores aggregate state from snapshot + events
- `EventStore` provides those events
- application code stays unaware of persistence metadata

However, the actual `EventStore` contract is too weak. `GetEventsSince` looks like a generic query API, but `Repository` uses it as a correctness-critical reconstruction primitive.

That gap becomes observable on DynamoDB:

1. `Save` appends event `seqNr=1`
2. a second `Save` immediately calls `loadInternal`
3. `GetEventsSince` reads via GSI
4. the GSI has not propagated yet
5. the repository reconstructs from blank state and returns a false domain error

This is not just a DynamoDB bug. It is a design problem: the repository depends on a stronger read guarantee than the store abstraction currently states.

## Goals

- Keep the application-facing API domain-pure.
- Guarantee that aggregate reconstruction does not miss recently committed events.
- Make repository semantics backend-independent.
- Keep backend-specific consistency handling inside store implementations.
- Make the store contract communicate reconstruction intent clearly.

## Non-Goals

- Adding application-facing APIs that expose sequence numbers or expected versions.
- Introducing a repository session or unit-of-work abstraction in this change.
- Reworking the entire DynamoDB table schema in this issue.
- Optimizing for maximum write throughput at the expense of repository correctness.

## Proposed API Shape

The read side of `EventStore` should express reconstruction semantics directly:

```go
type EventStore[A Aggregate[C, E], C Command, E Event] interface {
    GetLatestSnapshot(ctx context.Context, id AggregateID) (StoredSnapshot[A], bool, error)
    LoadStreamAfter(ctx context.Context, id AggregateID, seqNr uint64) ([]StoredEvent[E], error)
    PersistEvent(ctx context.Context, ev StoredEvent[E], expectedVersion uint64) error
    PersistEventAndSnapshot(ctx context.Context, ev StoredEvent[E], snap StoredSnapshot[A]) error
}
```

`LoadStreamAfter` means:

- read the event stream for exactly one aggregate
- return all events with `SeqNr > afterSeqNr`
- preserve event order
- do not omit committed events needed for reconstruction

The key point is not the method name itself but the stronger contract. The chosen name makes it harder to mistake the method for a best-effort query.

## Repository Behavior

`Repository.loadInternal()` will continue to:

1. load the latest snapshot if present
2. call `LoadStreamAfter(snapshotSeqNr)`
3. replay returned events in order

`Repository.Save()` remains unchanged in public shape and meaning:

- load aggregate state
- apply one command
- persist one resulting event
- optionally persist a snapshot

The library continues to hide persistence metadata from consumers.

## DynamoDB Implementation Strategy

`dynamodb.Store.LoadStreamAfter` should not use the `aid` GSI.

Instead, it should read from the primary journal table using a strongly consistent query path that can reconstruct one aggregate stream correctly. The exact key expression can be finalized in implementation, but the design intent is:

- target the aggregate's resolved shard partition
- restrict to the aggregate's sort-key prefix
- read with `ConsistentRead: true`
- return results in ascending sequence order

This makes the DynamoDB backend honor the repository contract directly, rather than relying on fallback heuristics after an eventually consistent miss.

The existing GSI can remain for other read patterns if needed, but it should no longer be on the critical path for aggregate reconstruction.

## Performance Position

This design intentionally prioritizes correctness of `Repository.Save()` over the lowest possible DynamoDB read cost.

That trade-off is acceptable because:

- aggregate reconstruction is correctness-critical
- the library promises a simple, domain-pure API
- pushing retries or sequence tracking to applications would violate that goal

This does not claim the primary-table strong read is always the most efficient possible design. It claims it is the correct default for the library's standard repository path.

If future performance work is needed, it should be introduced as an internal optimization or as an additional advanced API that still does not force persistence metadata into normal application flows.

## Rejected Alternatives

### Keep `GetEventsSince` and only patch DynamoDB internally

This would likely fix the immediate bug, but it leaves the abstraction misleading. The repository would still depend on a stronger guarantee than the interface communicates.

### Expose `SaveFrom`, `expectedVersion`, or `SeqNr`-driven flows to applications

These APIs can improve performance for advanced use cases, but they move persistence metadata into the application boundary. That conflicts with the current design goal and should not be the primary fix.

### Add a repository-local cache or session model

This might avoid some reloads, but it adds lifecycle and concurrency complexity to the high-level API. It is not necessary to resolve the core contract problem.

## Scope

In scope:

- `event_store.go` interface changes
- `repository.go` reconstruction path update
- `dynamodb/store.go` read-path redesign for aggregate reconstruction
- `memory/store.go` method rename and contract alignment
- tests covering immediate back-to-back saves with a simulated stale read backend
- README and package docs where the store contract is described

Out of scope:

- DynamoDB schema migration beyond what is necessary to support the new read path
- application-level batching APIs
- performance benchmarking beyond regression-level confidence

## Testing Strategy

- Add a deterministic test backend that simulates stale post-write reads on the old `GetEventsSince` style path.
- Verify that back-to-back `Save()` calls succeed through the repository with the new store contract.
- Keep DynamoDB-local tests for structural behavior, but do not rely on DynamoDB Local to reproduce the original issue.
- Run the full `go test ./...` suite after the refactor.

## Risks

### DynamoDB key-path limitations

The current journal key design is optimized around sharded partitions, not purely around per-aggregate stream reads. The implementation may need careful query shaping to keep the strong read path practical.

### Breaking store implementations

Any external custom `EventStore` implementation must adopt the stronger read contract and renamed method. This is acceptable given the correctness bug and chosen direction.

### Naming-only fix temptation

Renaming without documenting the stronger semantics would not solve the real problem. The contract text is part of the fix.

## Rollout Notes

- Treat this as a breaking library change.
- Update comments and README text at the same time as code.
- Call out explicitly in release notes that aggregate reconstruction now requires a stronger store contract and that DynamoDB reconstruction no longer relies on the journal GSI.
