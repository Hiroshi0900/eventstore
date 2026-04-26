package eventstore

import "fmt"

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
