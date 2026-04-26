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

- **`Command` abstraction** — domain intent is a first-class type passed to `Repository.Store`
- **`ApplyCommand` / `ApplyEvent` on the aggregate** — the aggregate decides which event to emit and how to evolve its state
- **State Pattern friendly** — `ApplyEvent` returns `Aggregate` (interface), so different concrete types can represent different aggregate states
- **`AggregateFactory[T]` is gone** — replaced by `createBlank func(AggregateID) T` + `AggregateSerializer[T]`
- **No `Get` prefix on accessors** — `id.TypeName()`, `event.EventID()`, `aggregate.Version()`, etc.
- **`OccurredAt` is `time.Time`** (stored as Unix milliseconds on the wire)

### Define a Command, an Event handler, and an Aggregate

```go
import (
    es "github.com/Hiroshi0900/eventstore/v2"
)

// --- Command ---

type IncrementCmd struct{ ID es.AggregateID }

func (c IncrementCmd) CommandTypeName() string     { return "Increment" }
func (c IncrementCmd) AggregateID() es.AggregateID { return c.ID }

// --- Aggregate ---

type Counter struct {
    id      es.AggregateID
    value   int
    seqNr   uint64
    version uint64
}

func NewCounter(id es.AggregateID) Counter { return Counter{id: id} }

func (c Counter) AggregateID() es.AggregateID       { return c.id }
func (c Counter) SeqNr() uint64                     { return c.seqNr }
func (c Counter) Version() uint64                   { return c.version }
func (c Counter) WithVersion(v uint64) es.Aggregate { c.version = v; return c }
func (c Counter) WithSeqNr(s uint64) es.Aggregate   { c.seqNr = s; return c }

func (c Counter) ApplyCommand(cmd es.Command) (es.Event, error) {
    switch cmd.(type) {
    case IncrementCmd:
        return es.NewEvent(
            "evt-"+cmd.AggregateID().AsString(),
            "Incremented",
            cmd.AggregateID(),
            nil,
            es.WithIsCreated(c.seqNr == 0),
        ), nil
    default:
        return nil, es.ErrUnknownCommand
    }
}

func (c Counter) ApplyEvent(ev es.Event) es.Aggregate {
    if ev.EventTypeName() == "Incremented" {
        c.value++
        c.seqNr = ev.SeqNr()
    }
    return c
}
```

### Snapshot serialization

```go
type counterSerializer struct{}

func (counterSerializer) Serialize(c Counter) ([]byte, error) {
    return json.Marshal(struct {
        Value   int    `json:"value"`
        SeqNr   uint64 `json:"seq_nr"`
        Version uint64 `json:"version"`
        IDType  string `json:"id_type"`
        IDValue string `json:"id_value"`
    }{c.value, c.seqNr, c.version, c.id.TypeName(), c.id.Value()})
}

func (counterSerializer) Deserialize(data []byte) (Counter, error) { /* ... */ }
```

### Use a Repository

```go
import (
    es "github.com/Hiroshi0900/eventstore/v2"
    esmem "github.com/Hiroshi0900/eventstore/v2/memory"
)

store := esmem.New()
repo := es.NewRepository[Counter](
    store,
    NewCounter,
    counterSerializer{},
    es.DefaultConfig(),
)

id := es.NewAggregateID("Counter", "c1")
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

store := esdb.New(dynamoClient, esdb.DefaultConfig())
repo := es.NewRepository[Counter](store, NewCounter, counterSerializer{}, es.DefaultConfig())
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
