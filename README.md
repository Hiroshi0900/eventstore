# eventstore

Go 向けの event sourcing ライブラリです。公開モジュールは
`github.com/Hiroshi0900/eventstore` です。

## Installation

```sh
go get github.com/Hiroshi0900/eventstore
```

## Packages

| Package | Description |
|---|---|
| `github.com/Hiroshi0900/eventstore` | Core interfaces and types |
| `github.com/Hiroshi0900/eventstore/dynamodb` | DynamoDB-backed `EventStore` implementation |
| `github.com/Hiroshi0900/eventstore/memory` | In-memory `EventStore` implementation |

## Overview

- `Aggregate[C Command, E Event]`, `Event`, `Command` はドメイン情報だけを持つ
- 保存用メタデータは `StoredEvent` / `StoredSnapshot` に集約
- `Repository.Load` / `Repository.Save(aggID, cmd)` は、最新 snapshot とその後の確定済み event stream を使って集約を復元する
- `EventStore` 実装は、利用者が永続化の内部構造を意識しなくてもよいように、再構築に必要な stream を正しく返す責務を持つ
- serializer は `EventStore` 実装側で扱う
- `ApplyEvent(E) Aggregate[C, E]` により State Pattern に対応

## Quick Start

```go
import (
    es "github.com/Hiroshi0900/eventstore"
    esmem "github.com/Hiroshi0900/eventstore/memory"
)

type CounterID struct{ value string }

func (id CounterID) TypeName() string { return "Counter" }
func (id CounterID) Value() string    { return id.value }
func (id CounterID) AsString() string { return "Counter-" + id.value }

type CounterCommand interface {
    es.Command
    isCounterCommand()
}

type IncrementCmd struct{ By int }

func (IncrementCmd) CommandTypeName() string { return "Increment" }
func (IncrementCmd) isCounterCommand()       {}

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

type Counter struct {
    id    CounterID
    count int
}

func NewBlankCounter(id es.AggregateID) Counter {
    return Counter{id: CounterID{value: id.Value()}}
}

func (c Counter) AggregateID() es.AggregateID { return c.id }

func (c Counter) ApplyCommand(cmd CounterCommand) (CounterEvent, error) {
    switch x := cmd.(type) {
    case IncrementCmd:
        return IncrementedEvent{AggID: c.id, By: x.By}, nil
    }
    return nil, es.ErrUnknownCommand
}

func (c Counter) ApplyEvent(ev CounterEvent) es.Aggregate[CounterCommand, CounterEvent] {
    if e, ok := ev.(IncrementedEvent); ok {
        return Counter{id: c.id, count: c.count + e.By}
    }
    return c
}

store := esmem.New[Counter, CounterCommand, CounterEvent]()
repo := es.NewRepository[Counter, CounterCommand, CounterEvent](store, NewBlankCounter, es.DefaultConfig())
```

## Development

実装本体は repo root にあります。`go test ./...` を root からそのまま実行できます。

## License

MIT
