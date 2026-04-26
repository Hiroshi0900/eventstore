package eventstore

import "time"

// EventOption は Event の関数オプションです。
type EventOption func(*BaseEvent)

// WithSeqNr はシーケンス番号をセットします。
func WithSeqNr(seqNr uint64) EventOption {
	return func(e *BaseEvent) { e.seqNr = seqNr }
}

// WithIsCreated は IsCreated フラグをセットします。
func WithIsCreated(isCreated bool) EventOption {
	return func(e *BaseEvent) { e.isCreated = isCreated }
}

// WithOccurredAt は発生時刻をセットします。
func WithOccurredAt(t time.Time) EventOption {
	return func(e *BaseEvent) { e.occurredAt = t }
}

// BaseEvent は Event 共通フィールドを保持する concrete 型かつ埋め込み用 struct です。
//
// シンプルな event payload (バイト列) ベースのイベントは BaseEvent を直接使えます:
//
//	ev := es.NewEvent("evt-1", "OrderCreated", aggID, payload, es.WithIsCreated(true))
//
// 一方、ドメイン固有のフィールドを持つ typed event を作る場合は埋め込みできます:
//
//	type OrderCreated struct {
//	    es.BaseEvent
//	    Total int
//	}
//
//	// Event interface の WithSeqNr は戻り値が Event なので埋め込み元で override する必要あり
//	func (e OrderCreated) WithSeqNr(seqNr uint64) es.Event {
//	    e.BaseEvent = e.BaseEvent.WithSeqNrBase(seqNr)
//	    return e
//	}
//
// Payload() の中身を typed event 自身でシリアライズしたい場合は、Payload() を override します
// （その場合 BaseEvent は内部 payload を空にしておく）。
type BaseEvent struct {
	eventID     string
	typeName    string
	aggregateID AggregateID
	seqNr       uint64
	isCreated   bool
	occurredAt  time.Time
	payload     []byte
}

// NewEvent は BaseEvent を生成します（payload ベースのシンプルなイベント用）。
func NewEvent(
	eventID string,
	typeName string,
	aggregateID AggregateID,
	payload []byte,
	opts ...EventOption,
) BaseEvent {
	e := BaseEvent{
		eventID:     eventID,
		typeName:    typeName,
		aggregateID: aggregateID,
		seqNr:       1,
		isCreated:   false,
		occurredAt:  time.Now(),
		payload:     payload,
	}
	for _, opt := range opts {
		opt(&e)
	}
	return e
}

// NewBaseEvent は typed event 用に BaseEvent を生成します（payload は空）。
// 埋め込んだ event 型が Payload() を override する想定です。
func NewBaseEvent(eventID, typeName string, aggregateID AggregateID, opts ...EventOption) BaseEvent {
	return NewEvent(eventID, typeName, aggregateID, nil, opts...)
}

func (e BaseEvent) EventID() string          { return e.eventID }
func (e BaseEvent) EventTypeName() string    { return e.typeName }
func (e BaseEvent) AggregateID() AggregateID { return e.aggregateID }
func (e BaseEvent) SeqNr() uint64            { return e.seqNr }
func (e BaseEvent) IsCreated() bool          { return e.isCreated }
func (e BaseEvent) OccurredAt() time.Time    { return e.occurredAt }
func (e BaseEvent) Payload() []byte          { return e.payload }

// WithSeqNr は新しい SeqNr を持つコピーを返します（不変性）。
// 直接使う場合は Event interface に適合します。typed event に埋め込んだ場合は
// embed 元で WithSeqNr を override し、本メソッドの代わりに WithSeqNrBase を使ってください。
func (e BaseEvent) WithSeqNr(seqNr uint64) Event {
	e.seqNr = seqNr
	return e
}

// WithSeqNrBase は埋め込み用に BaseEvent 自身の型を返します。
// typed event の WithSeqNr override から呼び出すために使います。
func (e BaseEvent) WithSeqNrBase(seqNr uint64) BaseEvent {
	e.seqNr = seqNr
	return e
}
