# v2 Command Generic Design

## Summary

Issue #4 addresses an asymmetry in the v2 API: `Event` is constrained per domain through generics, while `Command` remains the untyped base interface. We will adopt **Option A** and make `Command` a first-class generic parameter throughout the v2 domain-facing API.

This is an intentional breaking change to the v2 API. The goal is to reject wrong-domain commands at compile time, align `ApplyCommand` with the existing typed-event model, and make the core abstractions internally consistent.

## Decision

Adopt the following API direction:

```go
type Aggregate[C Command, E Event] interface {
    AggregateID() AggregateID
    ApplyCommand(C) (E, error)
    ApplyEvent(E) Aggregate[C, E]
}

type Repository[T Aggregate[C, E], C Command, E Event] interface {
    Load(ctx context.Context, aggID AggregateID) (T, error)
    Save(ctx context.Context, aggID AggregateID, cmd C) (T, error)
}
```

`EventStore` and serializer-related constraints will also receive `C` where needed for type consistency:

```go
type EventStore[T Aggregate[C, E], C Command, E Event] interface { ... }
type AggregateSerializer[T Aggregate[C, E], C Command, E Event] interface { ... }
```

Although `EventStore` and serializers do not semantically use `Command`, carrying `C` through these constraints avoids a split model where the repository layer is typed differently from the persistence layer.

## Rejected Alternatives

### Option B: Generic `Repository` only

This improves the `Save` surface but leaves `Aggregate.ApplyCommand` untyped. That preserves the asymmetry at the core abstraction boundary and weakens the design story of v2.

### Option C: Consumer-side marker interfaces only

This is a valid migration tactic for downstream apps, but it does not resolve the library design issue. It keeps correctness dependent on user discipline rather than the exported type system.

## Scope

In scope:

- `v2/types.go`
- `v2/repository.go`
- `v2/event_store.go`
- `v2/serializer.go`
- `v2/memory`
- `v2/dynamodb`
- all affected v2 tests
- README examples for v2

Out of scope:

- v1 API changes
- downstream consumer migrations in other repositories
- compatibility shims to preserve the old v2 type signatures

## API Impact

The following public signatures will change:

- `Aggregate[E]` to `Aggregate[C, E]`
- `ApplyCommand(Command)` to `ApplyCommand(C)`
- `ApplyEvent(E) Aggregate[E]` to `ApplyEvent(E) Aggregate[C, E]`
- `Repository[T, E]` to `Repository[T, C, E]`
- `Save(..., cmd Command)` to `Save(..., cmd C)`
- `EventStore[T, E]` to `EventStore[T, C, E]`
- constructors such as `NewRepository`, `memory.New`, `dynamodb.New`, and `dynamodb.NewWithTables`

This is a compile-time breaking change for consumers. Expected downstream edits are:

- define domain-specific command interfaces where useful
- add the `C` type parameter to repository and store instantiations
- update aggregate `ApplyCommand` signatures

## Implementation Plan

1. Change the core interfaces in `v2/types.go`.
2. Propagate `C` through repository and event-store abstractions.
3. Update memory and DynamoDB store implementations and constructors.
4. Update serializer constraints and any helper types that reference `Aggregate`.
5. Rewrite tests to use typed command interfaces in the same style already used for typed event interfaces.
6. Update README examples to show `CounterCommand`-style domain command interfaces.
7. Run `go test ./...` at repository root and in `v2/`.

## Testing Strategy

- Existing v2 unit tests must pass after the refactor.
- Add or update tests to ensure repository construction and `Save` use typed command domains.
- Rely on compile-time coverage for the main safety improvement; no runtime branch is added for wrong-domain commands because such calls should no longer type-check.

## Risks

### Consumer churn

Every v2 consumer must update generic parameters and aggregate method signatures. This is expected and acceptable because the change is deliberate and localized.

### Over-propagating `C`

Passing `C` through `EventStore` and serializers adds verbosity where `Command` is not directly used. We accept this because it preserves a single coherent type model and keeps constraints mechanically straightforward.

### Documentation drift

README and examples are part of the public API. They must be updated in the same change to avoid leaving consumers with stale usage patterns.

## Rollout Notes

- Treat this as a breaking v2 API change.
- Keep the library refactor and downstream consumer updates in separate PRs.
- The library PR should clearly call out the signature changes and show one before/after usage example.
