# eventstore

A lightweight event sourcing library for Go, backed by DynamoDB.

## Installation

```sh
go get github.com/Hiroshi0900/eventstore
```

## Packages

| Package | Description |
|---|---|
| `github.com/Hiroshi0900/eventstore` | Core interfaces and types |
| `github.com/Hiroshi0900/eventstore/dynamodb` | DynamoDB-backed EventStore implementation |
| `github.com/Hiroshi0900/eventstore/memory` | In-memory EventStore implementation (for testing) |

## Quick Start

### 1. Define your aggregate

```go
import es "github.com/Hiroshi0900/eventstore"

type OrderID struct{ value string }
func (id *OrderID) GetTypeName() string { return "Order" }
func (id *OrderID) GetValue() string    { return id.value }
func (id *OrderID) AsString() string    { return "Order-" + id.value }

type Order struct {
    id      es.AggregateID
    seqNr   uint64
    version uint64
    status  string
}

func (o *Order) GetId() es.AggregateID        { return o.id }
func (o *Order) GetSeqNr() uint64              { return o.seqNr }
func (o *Order) GetVersion() uint64            { return o.version }
func (o *Order) WithVersion(v uint64) es.Aggregate { return &Order{id: o.id, seqNr: o.seqNr, version: v, status: o.status} }
func (o *Order) WithSeqNr(s uint64) es.Aggregate   { return &Order{id: o.id, seqNr: s, version: o.version, status: o.status} }
```

### 2. Implement AggregateFactory

```go
type OrderFactory struct{}

func (f *OrderFactory) Create(id es.AggregateID) *Order {
    return &Order{id: id}
}

func (f *OrderFactory) Apply(agg *Order, event es.Event) (*Order, error) {
    // apply event to aggregate and return updated aggregate
    return agg, nil
}

func (f *OrderFactory) Serialize(agg *Order) ([]byte, error) {
    return json.Marshal(agg)
}

func (f *OrderFactory) Deserialize(data []byte) (*Order, error) {
    var agg Order
    return &agg, json.Unmarshal(data, &agg)
}
```

### 3. Use the repository

```go
import (
    es "github.com/Hiroshi0900/eventstore"
    esdb "github.com/Hiroshi0900/eventstore/dynamodb"
)

store := esdb.New(dynamoClient, esdb.DefaultConfig())
repo := es.NewRepository[*Order](store, &OrderFactory{}, es.NewJSONSerializer(), es.DefaultEventStoreConfig())

// Load
order, err := repo.FindByID(ctx, &OrderID{value: "order-123"})

// Save
event := es.NewEvent("evt-1", "OrderCreated", order.GetId(), payload, es.WithSeqNr(1), es.WithIsCreated(true))
err = repo.Save(ctx, order, []es.Event{event})
```

### Testing with in-memory store

```go
import (
    es "github.com/Hiroshi0900/eventstore"
    esmem "github.com/Hiroshi0900/eventstore/memory"
)

store := esmem.New()
repo := es.NewRepository[*Order](store, &OrderFactory{}, es.NewJSONSerializer(), es.DefaultEventStoreConfig())
```

## License

MIT
