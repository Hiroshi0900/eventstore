// Package eventstore は Event Sourcing を実現するためのライブラリです (v2)。
package eventstore

import "time"

// 主要なドメインインターフェース。具象型は同 package 内の他ファイルにある。
type (
	// AggregateID は集約の識別子を表します。
	AggregateID interface {
		TypeName() string
		Value() string
		AsString() string
	}

	// Aggregate は Event Sourcing における集約ルートを表します。
	// 状態遷移時に異なる具象型を返せるよう、ApplyEvent の戻り値は interface です。
	Aggregate interface {
		AggregateID() AggregateID
		SeqNr() uint64
		Version() uint64

		ApplyCommand(Command) (Event, error)
		ApplyEvent(Event) Aggregate

		WithVersion(uint64) Aggregate
		WithSeqNr(uint64) Aggregate
	}

	// Command はドメインの意図を表します。Repository.Store でターゲット集約に適用されます。
	Command interface {
		CommandTypeName() string
		AggregateID() AggregateID
	}

	// Event は過去に発生した不変のドメインイベントを表します。
	Event interface {
		EventID() string
		EventTypeName() string
		AggregateID() AggregateID
		SeqNr() uint64
		IsCreated() bool
		OccurredAt() time.Time
		Payload() []byte
		WithSeqNr(uint64) Event
	}
)
