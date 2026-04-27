package eventsourcing

// AggregateSerializer encodes/decodes a domain aggregate to/from bytes.
// T is the aggregate type (typically a domain interface like Visit).
// E is the aggregate-specific Event type. Both are constrained so the
// serializer cannot be paired with mismatched aggregate/event types.
type AggregateSerializer[T Aggregate[E], E Event] interface {
	Serialize(T) ([]byte, error)
	Deserialize([]byte) (T, error)
}

// EventSerializer encodes/decodes a domain event to/from bytes.
// On Deserialize, typeName (== Event.EventTypeName()) is used to dispatch
// to the correct concrete event type.
type EventSerializer[E Event] interface {
	Serialize(E) ([]byte, error)
	Deserialize(typeName string, data []byte) (E, error)
}
