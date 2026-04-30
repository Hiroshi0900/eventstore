# Loaded Aggregate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `LoadedAggregate`-based advanced repository API that avoids replaying the same aggregate between repeated command-side saves while preserving the existing `Load` / `Save` behavior.

**Architecture:** Extend `Repository` with `LoadForCommand` and `SaveLoaded`, introduce a public `LoadedAggregate` wrapper with opaque revision fields and an `Aggregate()` accessor, and refactor repository persistence logic so the standard and advanced paths share one command-application and persistence flow. Prove the new path with repository-level tests that detect redundant `LoadStreamAfter` calls and preserve snapshot/optimistic-lock semantics.

**Tech Stack:** Go, generics, standard `testing` package, existing memory store and repository fixtures

---

### Task 1: Add failing tests for the loaded-aggregate flow

**Files:**
- Modify: `repository_test.go`

- [ ] **Step 1: Add a test store wrapper that counts replay reads**

Append the following test helper near the existing `replayWitnessingStore` fixture in `repository_test.go`:

```go
type countingStore struct {
    es.EventStore[counterAggregate, counterCommand, counterEvent]
    loadCalls int
}

func newCountingStore(
    base es.EventStore[counterAggregate, counterCommand, counterEvent],
) *countingStore {
    return &countingStore{EventStore: base}
}

func (s *countingStore) LoadStreamAfter(
    ctx context.Context,
    id es.AggregateID,
    seqNr uint64,
) ([]es.StoredEvent[counterEvent], error) {
    s.loadCalls++
    return s.EventStore.LoadStreamAfter(ctx, id, seqNr)
}
```

- [ ] **Step 2: Add a failing test for repeated `SaveLoaded` without replay**

Append this test to `repository_test.go` after the existing `Save`-path tests:

```go
func TestRepository_SaveLoaded_reusesLoadedContextWithoutReplay(t *testing.T) {
    cfg := es.DefaultConfig()
    cfg.SnapshotInterval = 100

    base := memory.New[counterAggregate, counterCommand, counterEvent]()
    store := newCountingStore(base)
    repo := es.NewRepository[counterAggregate, counterCommand, counterEvent](store, blankCounter, cfg)
    id := counterID{value: "loaded"}

    loaded, err := repo.LoadForCommand(context.Background(), id)
    if err != nil {
        t.Fatalf("LoadForCommand: %v", err)
    }

    afterFirst, err := repo.SaveLoaded(context.Background(), loaded, incrementCommand{By: 2})
    if err != nil {
        t.Fatalf("first SaveLoaded: %v", err)
    }
    if got := afterFirst.Aggregate().count; got != 2 {
        t.Fatalf("count after first SaveLoaded: got %d, want 2", got)
    }

    callsAfterFirst := store.loadCalls

    afterSecond, err := repo.SaveLoaded(context.Background(), afterFirst, incrementCommand{By: 3})
    if err != nil {
        t.Fatalf("second SaveLoaded: %v", err)
    }
    if got := afterSecond.Aggregate().count; got != 5 {
        t.Fatalf("count after second SaveLoaded: got %d, want 5", got)
    }
    if store.loadCalls != callsAfterFirst {
        t.Fatalf("LoadStreamAfter calls: got %d, want %d", store.loadCalls, callsAfterFirst)
    }
}
```

- [ ] **Step 3: Add a failing not-found test for `LoadForCommand`**

Append this test immediately after the previous one:

```go
func TestRepository_LoadForCommand_notFound(t *testing.T) {
    repo, _ := newCounterRepo(t, es.DefaultConfig())

    _, err := repo.LoadForCommand(context.Background(), counterID{value: "missing"})
    if err == nil {
        t.Fatal("expected error")
    }

    var nf *es.AggregateNotFoundError
    if !errors.As(err, &nf) {
        t.Fatalf("expected AggregateNotFoundError, got %T", err)
    }
}
```

- [ ] **Step 4: Run the targeted tests and confirm they fail**

Run:

```sh
go test ./... -run 'TestRepository_(SaveLoaded_reusesLoadedContextWithoutReplay|LoadForCommand_notFound)$'
```

Expected: compile failure because `Repository` does not yet expose `LoadForCommand`, `SaveLoaded`, or `LoadedAggregate`.

- [ ] **Step 5: Commit the failing tests**

Run:

```sh
git add repository_test.go
git commit -m "test: cover loaded aggregate repository flow"
```

### Task 2: Implement `LoadedAggregate` and the new repository methods

**Files:**
- Modify: `repository.go`

- [ ] **Step 1: Add the public wrapper type and API surface**

Near the top of `repository.go`, before `Repository`, insert:

```go
type LoadedAggregate[A Aggregate[C, E], C Command, E Event] struct {
    aggregate A
    seqNr     uint64
    version   uint64
}

func (l LoadedAggregate[A, C, E]) Aggregate() A {
    return l.aggregate
}
```

Then extend `Repository` with:

```go
LoadForCommand(ctx context.Context, aggID AggregateID) (LoadedAggregate[A, C, E], error)
SaveLoaded(ctx context.Context, loaded LoadedAggregate[A, C, E], cmd C) (LoadedAggregate[A, C, E], error)
```

- [ ] **Step 2: Extract a helper that persists one command result from a known revision**

Replace the body duplication in `Save` with a helper in `repository.go`:

```go
func (r *defaultRepository[A, C, E]) saveKnownAggregate(
    ctx context.Context,
    agg A,
    currentSeqNr uint64,
    currentVersion uint64,
    cmd C,
) (LoadedAggregate[A, C, E], error) {
    var zero LoadedAggregate[A, C, E]

    ev, err := agg.ApplyCommand(cmd)
    if err != nil {
        return zero, err
    }

    next, ok := agg.ApplyEvent(ev).(A)
    if !ok {
        return zero, fmt.Errorf(
            "%w: ApplyEvent returned a value that does not implement A",
            ErrInvalidAggregate,
        )
    }

    nextSeqNr := currentSeqNr + 1
    now := time.Now().UTC()

    eventID, err := generateEventID()
    if err != nil {
        return zero, err
    }

    stored := StoredEvent[E]{
        Event:      ev,
        EventID:    eventID,
        SeqNr:      nextSeqNr,
        IsCreated:  currentSeqNr == 0,
        OccurredAt: now,
    }

    nextVersion := currentVersion
    if r.config.ShouldSnapshot(nextSeqNr) {
        nextVersion = currentVersion + 1
        snap := StoredSnapshot[A]{
            Aggregate:  next,
            SeqNr:      nextSeqNr,
            Version:    nextVersion,
            OccurredAt: now,
        }
        if err := r.store.PersistEventAndSnapshot(ctx, stored, snap); err != nil {
            return zero, err
        }
    } else {
        expectedVersion := currentVersion
        if currentSeqNr > 0 && expectedVersion == 0 {
            expectedVersion = currentSeqNr
        }
        if err := r.store.PersistEvent(ctx, stored, expectedVersion); err != nil {
            return zero, err
        }
    }

    return LoadedAggregate[A, C, E]{
        aggregate: next,
        seqNr:     nextSeqNr,
        version:   nextVersion,
    }, nil
}
```

- [ ] **Step 3: Implement `LoadForCommand`, `SaveLoaded`, and refit `Save`**

Update `repository.go` so:

```go
func (r *defaultRepository[A, C, E]) LoadForCommand(
    ctx context.Context,
    aggID AggregateID,
) (LoadedAggregate[A, C, E], error) {
    agg, seqNr, version, notFound, err := r.loadInternal(ctx, aggID)
    if err != nil {
        var zero LoadedAggregate[A, C, E]
        return zero, err
    }
    if notFound {
        var zero LoadedAggregate[A, C, E]
        return zero, NewAggregateNotFoundError(aggID.TypeName(), aggID.Value())
    }
    return LoadedAggregate[A, C, E]{
        aggregate: agg,
        seqNr:     seqNr,
        version:   version,
    }, nil
}

func (r *defaultRepository[A, C, E]) SaveLoaded(
    ctx context.Context,
    loaded LoadedAggregate[A, C, E],
    cmd C,
) (LoadedAggregate[A, C, E], error) {
    return r.saveKnownAggregate(ctx, loaded.aggregate, loaded.seqNr, loaded.version, cmd)
}
```

Refit `Save` to call the helper and return `loaded.Aggregate()`:

```go
loaded, err := r.saveKnownAggregate(ctx, agg, currentSeqNr, currentVersion, cmd)
if err != nil {
    return zero, err
}
return loaded.Aggregate(), nil
```

- [ ] **Step 4: Run the repository-focused tests**

Run:

```sh
go test ./... -run 'TestRepository_(SaveLoaded_reusesLoadedContextWithoutReplay|LoadForCommand_notFound|Save_)'
```

Expected: PASS for the new loaded-aggregate tests and existing save-path tests.

- [ ] **Step 5: Commit the repository implementation**

Run:

```sh
git add repository.go
git commit -m "feat: add loaded aggregate repository API"
```

### Task 3: Cover snapshot behavior and document usage

**Files:**
- Modify: `repository_test.go`
- Modify: `README.md`

- [ ] **Step 1: Add a snapshot-path regression test for `SaveLoaded`**

Append this test to `repository_test.go`:

```go
func TestRepository_SaveLoaded_updatesSnapshotVersionAcrossBoundary(t *testing.T) {
    cfg := es.DefaultConfig()
    cfg.SnapshotInterval = 2

    repo, store := newCounterRepo(t, cfg)
    id := counterID{value: "snapshot"}

    loaded, err := repo.LoadForCommand(context.Background(), id)
    if err == nil {
        t.Fatal("expected not found on empty aggregate")
    }

    if _, err := repo.Save(context.Background(), id, incrementCommand{By: 1}); err != nil {
        t.Fatalf("seed Save: %v", err)
    }

    loaded, err = repo.LoadForCommand(context.Background(), id)
    if err != nil {
        t.Fatalf("LoadForCommand after seed: %v", err)
    }

    loaded, err = repo.SaveLoaded(context.Background(), loaded, incrementCommand{By: 1})
    if err != nil {
        t.Fatalf("SaveLoaded at snapshot boundary: %v", err)
    }
    if got := loaded.Aggregate().count; got != 2 {
        t.Fatalf("count after snapshot boundary: got %d, want 2", got)
    }

    snap, found, err := store.GetLatestSnapshot(context.Background(), id)
    if err != nil {
        t.Fatalf("GetLatestSnapshot: %v", err)
    }
    if !found {
        t.Fatal("expected snapshot")
    }
    if snap.Version != 1 {
        t.Fatalf("snapshot version: got %d, want 1", snap.Version)
    }
    if snap.SeqNr != 2 {
        t.Fatalf("snapshot seqNr: got %d, want 2", snap.SeqNr)
    }
}
```

- [ ] **Step 2: Document the new advanced API in `README.md`**

Add one overview bullet and one short usage example like this:

```md
- `Repository.LoadForCommand` / `Repository.SaveLoaded` は、同一 aggregate に複数 command を続けて適用する時の最適化経路
```

```go
loaded, err := repo.LoadForCommand(ctx, counterID)
if err != nil { /* ... */ }

loaded, err = repo.SaveLoaded(ctx, loaded, IncrementCmd{By: 1})
if err != nil { /* ... */ }

current := loaded.Aggregate()
_ = current
```

Keep the existing `Load` / `Save` path documented as the standard correctness-first API.

- [ ] **Step 3: Run the full test suite**

Run:

```sh
go test ./...
```

Expected: PASS for all packages.

- [ ] **Step 4: Commit tests and docs**

Run:

```sh
git add repository_test.go README.md
git commit -m "docs: describe loaded aggregate command flow"
```

### Task 4: Final review and branch summary

**Files:**
- Modify: none

- [ ] **Step 1: Inspect the final diff**

Run:

```sh
git diff --stat main..
git diff -- main..HEAD repository.go repository_test.go README.md docs/superpowers/specs/2026-05-01-loaded-aggregate-design.md docs/superpowers/plans/2026-05-01-loaded-aggregate.md
```

Expected: only loaded-aggregate API, tests, README, spec, and plan changes are present.

- [ ] **Step 2: Re-run focused verification commands**

Run:

```sh
go test ./... -run 'TestRepository_(SaveLoaded_|LoadForCommand_)'
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Summarize the branch state**

Record the final branch state in your handoff:

```sh
git log --oneline --decorate -n 5
git status --short --branch
```

Expected: clean working tree on `codex/issue-10-loaded-aggregate`.
