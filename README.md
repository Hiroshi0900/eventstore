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

## v2

A new major version is available as a separate sub-module at
`github.com/Hiroshi0900/eventstore/v2`. v2 strips storage metadata
(`SeqNr`, `Version`, `EventID`, `OccurredAt`, `IsCreated`, `Payload` bytes)
out of domain types so that `Aggregate`, `Event`, and `Command` only carry
business information. The library wraps them in `EventEnvelope` and
`SnapshotEnvelope` at the storage boundary.

### Quick start (v2)

```go
import (
    es "github.com/Hiroshi0900/eventstore/v2"
    "github.com/Hiroshi0900/eventstore/v2/memory"
)

// 1. ドメイン側で Aggregate / Event / Command と Serializer を実装
//    (詳細は docs/superpowers/specs/2026-04-26-eventstore-v2-design.md §7 参照)

// 2. Repository を構築
store := memory.New()
repo := es.NewRepository[Visit, VisitEvent](
    store,
    func(id es.AggregateID) Visit { return EmptyVisit{id: id.(VisitID)} },
    visitAggregateSerializer{},
    visitEventSerializer{},
    es.DefaultConfig(),
)

// 3. ユースケース層は aggID と Command を渡すだけ
visit, err := repo.Save(ctx, visitID, ScheduleVisitCommand{ScheduledAt: t})
```

### v1 との主な違い

- ドメイン側の `Aggregate` / `Event` / `Command` がメタ情報を持たない
- ライブラリ側の `EventEnvelope` / `SnapshotEnvelope` がメタ情報を持つ
- `Repository.Save(aggID, cmd)` で load → ApplyCommand → ApplyEvent → 永続化を一括
- `AggregateFactory[T]` を廃止、`AggregateSerializer[T,E]` + `EventSerializer[E]` + `createBlank` 関数に分離
- 状態別型 (state pattern) を `ApplyEvent(E) Aggregate[E]` で正式サポート

詳細仕様: `docs/superpowers/specs/2026-04-26-eventstore-v2-design.md`

v1 (this README) は引き続き利用可能ですが、すべての公開シンボルが
`// Deprecated:` でマークされています。新規コードは v2 を推奨します。

## License

MIT
