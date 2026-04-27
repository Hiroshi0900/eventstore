package eventsourcing

// Command represents an intent to mutate an aggregate.
// CommandTypeName is used for OTel span names, audit logs, and metrics.
// AggregateID is not part of Command because it is passed to Repository.Save explicitly.
type Command interface {
	CommandTypeName() string
}
