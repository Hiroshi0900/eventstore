# eventstore

A lightweight event sourcing library for Go, backed by DynamoDB.

> **v2 is now available and recommended.** v1 (`github.com/Hiroshi0900/eventstore`)
> is **deprecated** but remains functional for an interim period. New projects
> should use v2 (`github.com/Hiroshi0900/eventstore/v2`).

## Versions

| Version | Import path | Status |
|---|---|---|
| v2 | `github.com/Hiroshi0900/eventstore/v2` | **Recommended** — Command abstraction, `ApplyCommand`/`ApplyEvent`, State Pattern support |
| v1 | `github.com/Hiroshi0900/eventstore` | **Deprecated** — kept for backward compatibility |

## v2 — Quick Start

### Installation

```sh
go get github.com/Hiroshi0900/eventstore/v2
```

### Packages

| Package | Description |
|---|---|
| `github.com/Hiroshi0900/eventstore/v2` | Core interfaces and types |
| `github.com/Hiroshi0900/eventstore/v2/dynamodb` | DynamoDB-backed `EventStore` implementation |
| `github.com/Hiroshi0900/eventstore/v2/memory` | In-memory `EventStore` implementation (for testing) |

### Key design points

- **Pure domain interfaces** — `Aggregate[E Event]`, `Event`, `Command` carry only business information. Storage metadata (`SeqNr`, `Version`, `EventID`, `OccurredAt`, `IsCreated`, `Payload bytes`) lives in the library-side `EventEnvelope` / `SnapshotEnvelope`.
- **Generic `Aggregate[E Event]`** — the aggregate type and its event type are linked at the type system level; `ApplyCommand` returns the concrete `E`.
- **State Pattern friendly** — `ApplyEvent(E) Aggregate[E]` lets different concrete state types represent different aggregate states (e.g. `VisitScheduled` → `VisitCompleted`).
- **No boilerplate types** — the library does **not** ship a `BaseAggregate` / `BaseEvent` / `DefaultAggregateID`. User domains define their own typed `AggregateID`s and event/aggregate structs.
- **Repository.Save(aggID, cmd)** — load → `ApplyCommand` → `ApplyEvent` → persist is a single Repository call. There is no separate "construct event then save" step.
- **Serialization owned by EventStore implementations** — `Repository[T,E]` works with domain types only; `(de)serialize` is the responsibility of the `EventStore[T,E]` implementation. memory store needs no serializer; dynamodb store takes `AggregateSerializer[T,E]` + `EventSerializer[E]` at construction.

### Define a typed AggregateID, Command, Event, Aggregate (no boilerplate)

```go
import (
    es "github.com/Hiroshi0900/eventstore/v2"
)

// --- typed AggregateID ---

type CounterID struct{ value string }

func NewCounterID(v string) CounterID { return CounterID{value: v} }

func (id CounterID) TypeName() string { return "Counter" }
func (id CounterID) Value() string    { return id.value }
func (id CounterID) AsString() string { return "Counter-" + id.value }

// --- Command (CommandTypeName のみ。AggregateID は Repository.Save の引数で渡す) ---

type IncrementCmd struct{ By int }

func (IncrementCmd) CommandTypeName() string { return "Increment" }

// --- Domain Event interface + concrete event ---

type CounterEvent interface {
    es.Event
    isCounterEvent()
}

type IncrementedEvent struct {
    AggID CounterID
    By    int
}

func (e IncrementedEvent) EventTypeName() string       { return "Incremented" }
func (e IncrementedEvent) AggregateID() es.AggregateID { return e.AggID }
func (IncrementedEvent) isCounterEvent()               {}

// --- Aggregate (pure struct, no embed, no SeqNr/Version) ---

type Counter struct {
    id    CounterID
    count int
}

func NewBlankCounter(id es.AggregateID) Counter {
    if cid, ok := id.(CounterID); ok {
        return Counter{id: cid}
    }
    return Counter{id: CounterID{value: id.Value()}}
}

func (c Counter) AggregateID() es.AggregateID { return c.id }

func (c Counter) ApplyCommand(cmd es.Command) (CounterEvent, error) {
    switch x := cmd.(type) {
    case IncrementCmd:
        return IncrementedEvent{AggID: c.id, By: x.By}, nil
    }
    return nil, es.ErrUnknownCommand
}

func (c Counter) ApplyEvent(ev CounterEvent) es.Aggregate[CounterEvent] {
    if e, ok := ev.(IncrementedEvent); ok {
        return Counter{id: c.id, count: c.count + e.By}
    }
    return c
}
```

### Use a Repository (in-memory)

memory store は (de)serialize を行わないので serializer は不要。

```go
import (
    es "github.com/Hiroshi0900/eventstore/v2"
    esmem "github.com/Hiroshi0900/eventstore/v2/memory"
)

store := esmem.New[Counter, CounterEvent]() // returns es.EventStore[Counter, CounterEvent]
repo := es.NewRepository[Counter, CounterEvent](store, NewBlankCounter, es.DefaultConfig())

id := NewCounterID("c1")

// Save loads (or creates blank) → ApplyCommand → ApplyEvent → persist.
c, err := repo.Save(ctx, id, IncrementCmd{By: 1})

// Load replays from the latest snapshot + subsequent events.
loaded, err := repo.Load(ctx, id)
```

### Implement domain serializers (DynamoDB のみ必要)

DynamoDB store は永続化前後で domain ↔ bytes の変換を行うため、利用側が
`AggregateSerializer[T,E]` と `EventSerializer[E]` を実装して store に渡す。

```go
// JSON wire format for snapshots and events.
type counterSnapshot struct {
    Value string `json:"value"`
    Count int    `json:"count"`
}

type CounterAggregateSerializer struct{}

func (CounterAggregateSerializer) Serialize(c Counter) ([]byte, error) {
    return json.Marshal(counterSnapshot{Value: c.id.value, Count: c.count})
}
func (CounterAggregateSerializer) Deserialize(data []byte) (Counter, error) {
    var s counterSnapshot
    if err := json.Unmarshal(data, &s); err != nil {
        return Counter{}, es.NewDeserializationError("aggregate", err)
    }
    return Counter{id: CounterID{value: s.Value}, count: s.Count}, nil
}

type incrementedWire struct {
    IDValue string `json:"id_value"`
    By      int    `json:"by"`
}

type CounterEventSerializer struct{}

func (CounterEventSerializer) Serialize(ev CounterEvent) ([]byte, error) {
    if e, ok := ev.(IncrementedEvent); ok {
        return json.Marshal(incrementedWire{IDValue: e.AggID.value, By: e.By})
    }
    return nil, es.NewSerializationError("event", errors.New("unknown event type"))
}
func (CounterEventSerializer) Deserialize(typeName string, data []byte) (CounterEvent, error) {
    if typeName == "Incremented" {
        var w incrementedWire
        if err := json.Unmarshal(data, &w); err != nil {
            return nil, es.NewDeserializationError("event", err)
        }
        return IncrementedEvent{AggID: CounterID{value: w.IDValue}, By: w.By}, nil
    }
    return nil, es.NewDeserializationError("event", errors.New("unknown event type: "+typeName))
}
```

### Production: DynamoDB

```go
import (
    es "github.com/Hiroshi0900/eventstore/v2"
    esdb "github.com/Hiroshi0900/eventstore/v2/dynamodb"
)

store := esdb.New[Counter, CounterEvent](
    dynamoClient,
    esdb.DefaultConfig(),
    CounterAggregateSerializer{},
    CounterEventSerializer{},
)
repo := es.NewRepository[Counter, CounterEvent](store, NewBlankCounter, es.DefaultConfig())
```

The DynamoDB store maintains two tables (`journal`, `snapshot`), uses
`TransactWriteItems` for atomic event + snapshot writes, supports partition-key
sharding, and injects OpenTelemetry `traceparent` / `tracestate` into each
event item.

---

## v1 — Legacy (deprecated)

> v1 will continue to work but will not receive new features. Prefer v2 above.

```sh
go get github.com/Hiroshi0900/eventstore
```

```go
import es "github.com/Hiroshi0900/eventstore"

store := esmem.New() // or esdb.New(...)
repo := es.NewRepository[*Order](store, &OrderFactory{}, es.NewJSONSerializer(), es.DefaultEventStoreConfig())

order, err := repo.FindByID(ctx, &OrderID{value: "order-123"})

event := es.NewEvent("evt-1", "OrderCreated", order.GetId(), payload,
    es.WithSeqNr(1), es.WithIsCreated(true))
err = repo.Save(ctx, order, []es.Event{event})
```

## License

MIT
