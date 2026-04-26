// Package eventstore は Event Sourcing を実現するためのライブラリです (v2)。
package eventstore

import "fmt"

// AggregateID は集約の識別子を表します。
type AggregateID interface {
	TypeName() string
	Value() string
	AsString() string
}

// DefaultAggregateID は AggregateID のデフォルト実装です。
type DefaultAggregateID struct {
	typeName string
	value    string
}

// NewAggregateID は DefaultAggregateID を生成します。
func NewAggregateID(typeName, value string) DefaultAggregateID {
	return DefaultAggregateID{typeName: typeName, value: value}
}

func (id DefaultAggregateID) TypeName() string { return id.typeName }
func (id DefaultAggregateID) Value() string    { return id.value }
func (id DefaultAggregateID) AsString() string {
	return fmt.Sprintf("%s-%s", id.typeName, id.value)
}

// Aggregate は Event Sourcing における集約ルートを表します。
// 状態遷移時に異なる具象型を返せるよう、ApplyEvent の戻り値は interface です。
type Aggregate interface {
	AggregateID() AggregateID
	SeqNr() uint64
	Version() uint64

	ApplyCommand(Command) (Event, error)
	ApplyEvent(Event) Aggregate

	WithVersion(uint64) Aggregate
	WithSeqNr(uint64) Aggregate
}
