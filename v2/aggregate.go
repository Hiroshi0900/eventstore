package eventstore

// BaseAggregate は Aggregate 共通フィールド (AggregateID / SeqNr / Version) を
// 提供する埋め込み用 struct です。これを embed することで、ユーザは AggregateID() /
// SeqNr() / Version() の boilerplate を省略できます。
//
// 使い方:
//
//	type Counter struct {
//	    es.BaseAggregate
//	    value int
//	}
//
//	func NewCounter(id es.AggregateID) Counter {
//	    return Counter{BaseAggregate: es.NewBaseAggregate(id, 0, 0)}
//	}
//
//	// Aggregate interface の WithSeqNr / WithVersion は戻り値が Aggregate なので
//	// embed 元で override する必要があります。
//	func (c Counter) WithSeqNr(s uint64) es.Aggregate {
//	    c.BaseAggregate = c.BaseAggregate.WithSeqNr(s)
//	    return c
//	}
//	func (c Counter) WithVersion(v uint64) es.Aggregate {
//	    c.BaseAggregate = c.BaseAggregate.WithVersion(v)
//	    return c
//	}
//
//	func (c Counter) ApplyCommand(cmd es.Command) (es.Event, error) { ... }
//	func (c Counter) ApplyEvent(ev es.Event) es.Aggregate { ... }
//
// AggregateID 自体はドメイン側で typed な実装を定義してください
// (例: type CounterID struct{ value string } で AggregateID interface を満たす)。
type BaseAggregate struct {
	aggregateID AggregateID
	seqNr       uint64
	version     uint64
}

// NewBaseAggregate は BaseAggregate を生成します。
// version は楽観ロック用、初期作成時は 0 を指定します。
func NewBaseAggregate(aggregateID AggregateID, seqNr, version uint64) BaseAggregate {
	return BaseAggregate{aggregateID: aggregateID, seqNr: seqNr, version: version}
}

// AggregateID は集約 ID を返します（埋め込みで自動的に Aggregate interface を満たします）。
func (a BaseAggregate) AggregateID() AggregateID { return a.aggregateID }

// SeqNr は seqNr を返します。
func (a BaseAggregate) SeqNr() uint64 { return a.seqNr }

// Version は楽観ロック用の version を返します。
func (a BaseAggregate) Version() uint64 { return a.version }

// WithSeqNr は新しい seqNr を持つ BaseAggregate のコピーを返します。
// 埋め込み元の WithSeqNr override から呼び出してください。
func (a BaseAggregate) WithSeqNr(seqNr uint64) BaseAggregate {
	a.seqNr = seqNr
	return a
}

// WithVersion は新しい version を持つ BaseAggregate のコピーを返します。
// 埋め込み元の WithVersion override から呼び出してください。
func (a BaseAggregate) WithVersion(version uint64) BaseAggregate {
	a.version = version
	return a
}
