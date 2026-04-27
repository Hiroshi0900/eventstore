package eventsourcing

// Event represents a domain fact. Events carry only domain information;
// all storage metadata (SeqNr, EventID, OccurredAt, IsCreated, Payload bytes)
// is held by EventEnvelope at the library layer.
type Event interface {
	// EventTypeName returns the type discriminator used by EventSerializer
	// for dispatch on deserialization.
	EventTypeName() string

	// AggregateID returns the aggregate this event belongs to.
	AggregateID() AggregateID
}
