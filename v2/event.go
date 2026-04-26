package eventstore

import "time"

// Event は過去に発生した不変のドメインイベントを表します。
type Event interface {
	EventID() string
	EventTypeName() string
	AggregateID() AggregateID
	SeqNr() uint64
	IsCreated() bool
	OccurredAt() time.Time
	Payload() []byte
	WithSeqNr(uint64) Event
}

// EventOption は Event の関数オプションです。
type EventOption func(*DefaultEvent)

// WithSeqNr はシーケンス番号をセットします。
func WithSeqNr(seqNr uint64) EventOption {
	return func(e *DefaultEvent) { e.seqNr = seqNr }
}

// WithIsCreated は IsCreated フラグをセットします。
func WithIsCreated(isCreated bool) EventOption {
	return func(e *DefaultEvent) { e.isCreated = isCreated }
}

// WithOccurredAt は発生時刻をセットします。
func WithOccurredAt(t time.Time) EventOption {
	return func(e *DefaultEvent) { e.occurredAt = t }
}

// DefaultEvent は Event のデフォルト実装です。
type DefaultEvent struct {
	eventID     string
	typeName    string
	aggregateID AggregateID
	seqNr       uint64
	isCreated   bool
	occurredAt  time.Time
	payload     []byte
}

// NewEvent は DefaultEvent を生成します。
func NewEvent(
	eventID string,
	typeName string,
	aggregateID AggregateID,
	payload []byte,
	opts ...EventOption,
) DefaultEvent {
	e := DefaultEvent{
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

func (e DefaultEvent) EventID() string          { return e.eventID }
func (e DefaultEvent) EventTypeName() string    { return e.typeName }
func (e DefaultEvent) AggregateID() AggregateID { return e.aggregateID }
func (e DefaultEvent) SeqNr() uint64            { return e.seqNr }
func (e DefaultEvent) IsCreated() bool          { return e.isCreated }
func (e DefaultEvent) OccurredAt() time.Time    { return e.occurredAt }
func (e DefaultEvent) Payload() []byte          { return e.payload }

// WithSeqNr は新しい SeqNr を持つコピーを返します（不変性）。
func (e DefaultEvent) WithSeqNr(seqNr uint64) Event {
	e.seqNr = seqNr
	return e
}
