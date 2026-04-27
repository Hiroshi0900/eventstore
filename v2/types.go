// Package eventstore は Event Sourcing を実現するためのライブラリです (v2)。
//
// ドメイン側の Aggregate / Event / Command interface はビジネス情報のみを
// 持ち、SeqNr / Version / EventID / OccurredAt / IsCreated / Payload bytes
// などの永続化メタ情報は library 側の EventEnvelope / SnapshotEnvelope に
// 集約される。これにより利用側は「業務概念としての state と振る舞い」のみ
// を実装すればよい。
package eventstore

// 主要なドメインインターフェース。具象型はすべて利用側で定義する想定で、
// library は default 実装 (DefaultAggregateID, BaseAggregate 等) を提供しない。
type (
	// AggregateID は集約の識別子を表す。利用側は domain ごとに typed な
	// 実装を定義する (例: type VisitID struct{...} で TypeName/Value/AsString を実装)。
	AggregateID interface {
		TypeName() string
		Value() string
		AsString() string
	}

	// Aggregate は Event Sourcing における集約ルート。
	// E は集約専用の Event 型（利用側で定義する domain Event interface）。
	//
	// メタ情報 (SeqNr / Version / 永続化メタ) を一切持たず、ApplyCommand /
	// ApplyEvent で表現される業務状態遷移のみを定義する。
	//
	// ApplyEvent の戻り値が Aggregate[E] (interface) なので、状態遷移時に
	// 異なる具象型を返す state pattern が可能 (例: VisitScheduled → VisitCompleted)。
	Aggregate[E Event] interface {
		AggregateID() AggregateID
		ApplyCommand(Command) (E, error)
		ApplyEvent(E) Aggregate[E]
	}

	// Command はドメインの意図を表す。
	// AggregateID は持たない (Repository.Save が aggID を別引数で受け取るため冗長)。
	Command interface {
		CommandTypeName() string
	}

	// Event は過去に発生した不変のドメインイベント。
	// EventTypeName は EventSerializer の dispatch / OTel span name / 監査用。
	// 永続化メタ (EventID / SeqNr / IsCreated / OccurredAt / Payload bytes)
	// は EventEnvelope に集約される。
	Event interface {
		EventTypeName() string
		AggregateID() AggregateID
	}
)
