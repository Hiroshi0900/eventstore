# Loaded Aggregate Design

## Summary

Issue #10 adds a performance-oriented command-side API without changing the semantics of the existing repository API.

The library will keep `Repository.Load()` and `Repository.Save()` as the simple, correctness-first standard path. In parallel, it will add an explicit advanced path for repeated command handling against the same aggregate, using an opaque `LoadedAggregate` handle so application code does not need to manage raw persistence metadata such as sequence numbers or optimistic-lock versions.

## Decision

Adopt the following design direction:

```go
type Repository[A Aggregate[C, E], C Command, E Event] interface {
    Load(ctx context.Context, aggID AggregateID) (A, error)
    Save(ctx context.Context, aggID AggregateID, cmd C) (A, error)
}

type CommandRepository[A Aggregate[C, E], C Command, E Event] interface {
    Repository[A, C, E]

    LoadForCommand(ctx context.Context, aggID AggregateID) (LoadedAggregate[A, C, E], error)
    SaveLoaded(ctx context.Context, loaded LoadedAggregate[A, C, E], cmd C) (LoadedAggregate[A, C, E], error)
}
```

`LoadedAggregate` is a public wrapper type used only for this advanced path. It exposes the current aggregate state through an accessor, but keeps revision and replay metadata opaque.

`NewRepository(...)` remains the backward-compatible standard entry point. `NewCommandRepository(...)` returns the same underlying implementation behind a wider interface for callers that explicitly opt into the advanced path.

## Problem Statement

Issue #8 intentionally strengthened aggregate reconstruction correctness by making `Repository` depend on a stronger `LoadStreamAfter` contract and a strongly consistent DynamoDB replay path.

That is the right default, but it creates extra cost on command-side flows that repeatedly execute commands against the same aggregate in one request:

1. `Save()` reconstructs aggregate state
2. one command is applied and persisted
3. the next command handling step reconstructs the aggregate again

This is acceptable for a simple standard API, but it is unnecessarily expensive when the caller already holds a fresh aggregate context from the previous command step.

## Goals

- Keep `Load()` and `Save()` as the standard correctness-first repository API.
- Avoid repeated aggregate reconstruction when the caller already has a fresh loaded context.
- Keep sequence/version metadata opaque to normal consumers.
- Make the API backend-agnostic even though the motivating cost is especially visible on DynamoDB.
- Preserve a clean path for infrastructure adapters to wrap this API in a higher-level session abstraction.

## Non-Goals

- Changing the behavior or guarantees of existing `Load()` / `Save()`.
- Exposing raw `SeqNr`, `Version`, or expected-version fields to application code.
- Designing application-layer session abstractions in this library.
- Solving DynamoDB replay cost purely through public API changes instead of future schema work.

## Proposed API Shape

The repository gains a second command-side flow:

```go
type LoadedAggregate[A Aggregate[C, E], C Command, E Event] struct {
    // unexported fields
}

func (l LoadedAggregate[A, C, E]) Aggregate() A
```

The semantics are:

- `LoadForCommand` reconstructs the aggregate using the same correctness rules as `Load`
- `SaveLoaded` applies one command to an existing loaded handle and persists the result using the stored opaque revision context
- `SaveLoaded` returns the next `LoadedAggregate`, ready for another command without a fresh replay roundtrip
- `LoadedAggregate` can only be created for value-semantic aggregate shapes; pointer aggregates and aggregates that recursively contain pointer / map / slice / func / chan / interface / unsafe-pointer fields are rejected with `ErrInvalidAggregate`

This keeps the return type focused on "continue the command session" rather than "return persistence metadata or emitted events."

## Repository Behavior

The existing standard path remains unchanged:

- `Load` returns the aggregate only
- `Save` reconstructs as needed, applies one command, persists one event, and returns the next aggregate state

The new advanced path behaves similarly, but uses the loaded handle as its reconstruction anchor:

- `LoadForCommand` reuses the current `loadInternal` reconstruction logic and captures the resulting aggregate plus opaque persistence metadata
- `SaveLoaded` uses the loaded handle instead of replaying from storage again
- after persistence, `SaveLoaded` returns a refreshed loaded handle that contains the new aggregate state and updated opaque metadata

## Why `LoadedAggregate`

`LoadedAggregate` is not a globally standard term, but it matches the intended abstraction in this library:

- it clearly communicates that the value represents a currently loaded aggregate plus internal repository context
- it avoids exposing storage-specific terms such as `VersionToken` or `ExpectedVersion`
- it is a better fit for the library boundary than a vague name like `AdvancedRepository`

Application code is still expected to wrap this type inside an infrastructure-facing repository or command session abstraction if it wants to keep eventstore-specific concepts out of use-case code.

## Why a Separate Interface

The advanced path must not change the shape of the existing `Repository` interface because that would be a source-breaking change for current users.

`CommandRepository` keeps the standard path small and backward compatible while still allowing the same `defaultRepository` implementation to expose the optimization for callers that explicitly opt in.

The important boundary remains that raw persistence metadata stays opaque. Splitting the interface simply makes that opt-in explicit at the public API level.

## Backend Position

This API is not DynamoDB-specific, but the motivation is concrete on DynamoDB.

Today, the correctness-first reconstruction path uses a strongly consistent primary-table replay that:

- queries by shard key and lower sort-key bound
- filters by aggregate ID after the read
- may require multiple pages when the shard contains many other aggregates

That increases:

- DynamoDB read cost
- request latency
- internal paging and replay work

The advanced loaded-aggregate path provides a backend-agnostic way to avoid that repeated replay cost in command-side flows, while preserving the default correctness-first API.

## Rejected Alternatives

### Expose raw version or sequence numbers

This would help performance, but it pushes storage concerns into application code and weakens the library's design boundary.

### Replace `Load` / `Save` with handle-based APIs

This would make the simple standard path harder to use and would overfit the public API to optimization-oriented flows.

### Put the advanced methods directly on `Repository`

Rejected because it breaks backward compatibility for existing implementations and mocks that satisfy `Repository`.

### Return the emitted domain event from `SaveLoaded`

This broadens the purpose of the API. The main requirement is to continue a loaded command flow, so returning the next handle is enough for the first version.

## Scope

In scope:

- add `LoadedAggregate`
- add `CommandRepository` plus `NewCommandRepository`
- keep `Repository` limited to `Load` and `Save`
- update repository implementation to track and refresh opaque revision context
- validate that the loaded-handle API only accepts value-semantic aggregate shapes
- add tests for repeated command handling without a second replay roundtrip
- document intended usage in README and package comments

Out of scope:

- application-facing `CommandSession` helpers
- schema redesign for DynamoDB replay efficiency
- query-side or event-inspection APIs

## Testing Strategy

- add repository-level tests proving that `SaveLoaded` can apply multiple commands without requiring an extra `LoadStreamAfter` call between saves
- keep coverage for the standard `Load` / `Save` path unchanged
- validate that optimistic locking and snapshot behavior still work through the loaded path
- run `go test ./...`

## Risks

### Opaque wrapper leakage

If `LoadedAggregate` exposes too much internal detail, the library will reintroduce persistence concerns at the public boundary. The implementation should keep all revision metadata unexported.

### Mutable aliasing through aggregate shape

If `LoadedAggregate` is created for pointer-heavy or reference-heavy aggregate shapes, callers can mutate shared state behind the repository's back. The advanced API therefore rejects non-value-semantic aggregate shapes at runtime.

### Behavioral drift between standard and advanced paths

If `Save` and `SaveLoaded` diverge in command application or snapshot logic, the API will become confusing. The implementation should reuse the same internal persist logic where possible.

### Future naming pressure

If real usage reveals that `LoadedAggregate` is confusing in application-level code, that should be solved in adapters and examples before expanding the public API surface further.

## Rollout Notes

- treat this as an additive public API change
- keep `Load` / `Save` documented as the standard path
- describe `CommandRepository.LoadForCommand` / `CommandRepository.SaveLoaded` as an explicit optimization path for repeated command handling
- document the value-semantic restriction anywhere the advanced flow is shown
