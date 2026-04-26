package eventstore

// Command はドメインの意図を表します。Repository.Store でターゲット集約に適用されます。
type Command interface {
	CommandTypeName() string
	AggregateID() AggregateID
}
