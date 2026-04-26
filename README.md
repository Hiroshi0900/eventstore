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

### Key changes from v1

- **Generic `EventStore[T]`** — type-safe Store/Repository pair; snapshots round-trip as `Aggregate` values, not byte payloads
- **`Command` abstraction** — domain intent is a first-class type passed to `Repository.Store`
- **`ApplyCommand` / `ApplyEvent` on the aggregate** — the aggregate decides which event to emit and how to evolve its state
- **State Pattern friendly** — `ApplyEvent` returns `Aggregate` (interface), so different concrete types can represent different aggregate states
- **`BaseAggregate` / `BaseEvent`** — embeddable types that supply common boilerplate (AggregateID/SeqNr/Version accessors, etc.)
- **No `Get` prefix on accessors** — `id.TypeName()`, `event.EventID()`, `aggregate.Version()`, etc.
- **`OccurredAt` is `time.Time`** (stored as Unix milliseconds on the wire)
- **`EventSerializer`** — JSON and Protobuf implementations are provided for serializing whole `Event` values

### Define a Command and an Aggregate (with `BaseAggregate`)

```go
import (
    es "github.com/Hiroshi0900/eventstore/v2"
)

// --- AggregateID (typed per domain) ---

type CounterID string

func (id CounterID) TypeName() string { return "Counter" }
func (id CounterID) Value() string    { return string(id) }
func (id CounterID) AsString() string { return "Counter-" + string(id) }

// --- Command ---

type IncrementCmd struct{ ID es.AggregateID }

func (c IncrementCmd) CommandTypeName() string     { return "Increment" }
func (c IncrementCmd) AggregateID() es.AggregateID { return c.ID }

// --- Aggregate (embeds BaseAggregate to skip ID/SeqNr/Version boilerplate) ---

type Counter struct {
    es.BaseAggregate
    value int
}

func NewCounter(id es.AggregateID) Counter {
    return Counter{BaseAggregate: es.NewBaseAggregate(id, 0, 0)}
}

// Aggregate interface's WithSeqNr / WithVersion return Aggregate, so override.
func (c Counter) WithSeqNr(s uint64) es.Aggregate {
    c.BaseAggregate = c.BaseAggregate.WithSeqNr(s)
    return c
}
func (c Counter) WithVersion(v uint64) es.Aggregate {
    c.BaseAggregate = c.BaseAggregate.WithVersion(v)
    return c
}

func (c Counter) ApplyCommand(cmd es.Command) (es.Event, error) {
    switch cmd.(type) {
    case IncrementCmd:
        return es.NewEvent(
            "evt-"+cmd.AggregateID().AsString(),
            "Incremented",
            cmd.AggregateID(),
            nil,
            es.WithIsCreated(c.SeqNr() == 0),
        ), nil
    default:
        return nil, es.ErrUnknownCommand
    }
}

func (c Counter) ApplyEvent(ev es.Event) es.Aggregate {
    if ev.EventTypeName() == "Incremented" {
        c.value++
        c.BaseAggregate = c.BaseAggregate.WithSeqNr(ev.SeqNr())
    }
    return c
}
```

### Use a Repository (in-memory)

```go
import (
    es "github.com/Hiroshi0900/eventstore/v2"
    esmem "github.com/Hiroshi0900/eventstore/v2/memory"
)

// memory.Store keeps Aggregate values in-memory; no serializer needed.
store := esmem.New[Counter]()
repo := es.NewRepository[Counter](store, NewCounter, es.DefaultConfig())

id := CounterID("c1")
c := NewCounter(id)

// Apply a command and persist the resulting event.
c, err := repo.Store(ctx, IncrementCmd{ID: id}, c)

// Load replays from the latest snapshot + subsequent events.
loaded, err := repo.Load(ctx, id)
```

### Production: DynamoDB

```go
import (
    es "github.com/Hiroshi0900/eventstore/v2"
    esdb "github.com/Hiroshi0900/eventstore/v2/dynamodb"
)

// dynamodb.Store needs an AggregateSerializer[T] to (de)serialize the snapshot
// payload. JSON/Protobuf serializers can be reused, or roll your own.
type counterSerializer struct{}
func (counterSerializer) Serialize(c Counter) ([]byte, error)   { /* json/proto */ }
func (counterSerializer) Deserialize([]byte) (Counter, error)   { /* json/proto */ }

store := esdb.New[Counter](dynamoClient, esdb.DefaultConfig(), counterSerializer{})
repo := es.NewRepository[Counter](store, NewCounter, es.DefaultConfig())
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
