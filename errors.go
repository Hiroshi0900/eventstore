package eventsourcing

import (
	"errors"
	"fmt"
)

// Sentinel errors for the eventsourcing library.
var (
	// ErrOptimisticLock is returned when a concurrent modification is detected.
	// It occurs when the version in the store does not match the expected version.
	//
	// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
	ErrOptimisticLock = errors.New("optimistic lock error: concurrent modification detected")

	// ErrAggregateNotFound is returned when an aggregate is not found in the store.
	//
	// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
	ErrAggregateNotFound = errors.New("aggregate not found")

	// ErrSerializationFailed is returned when serialization of an event or snapshot fails.
	//
	// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
	ErrSerializationFailed = errors.New("serialization failed")

	// ErrDeserializationFailed is returned when deserialization of an event or snapshot fails.
	//
	// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
	ErrDeserializationFailed = errors.New("deserialization failed")

	// ErrInvalidEvent is returned when an event is invalid or malformed.
	//
	// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
	ErrInvalidEvent = errors.New("invalid event")

	// ErrInvalidAggregate is returned when an aggregate is invalid or malformed.
	//
	// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
	ErrInvalidAggregate = errors.New("invalid aggregate")

	// ErrEventStoreUnavailable is returned when the event store is not available.
	//
	// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
	ErrEventStoreUnavailable = errors.New("event store unavailable")

	// ErrDuplicateAggregate is returned when an aggregate with the same ID already exists.
	// This is used to detect duplicate creation when using deterministic IDs.
	//
	// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
	ErrDuplicateAggregate = errors.New("aggregate already exists")
)

// OptimisticLockError provides detailed information about a lock conflict.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
type OptimisticLockError struct {
	AggregateId     string
	ExpectedVersion uint64
	ActualVersion   uint64
}

// Error returns the formatted error message.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *OptimisticLockError) Error() string {
	return fmt.Sprintf(
		"optimistic lock error: aggregate %s expected version %d but got %d",
		e.AggregateId, e.ExpectedVersion, e.ActualVersion,
	)
}

// Is reports whether the target error is ErrOptimisticLock.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *OptimisticLockError) Is(target error) bool {
	return target == ErrOptimisticLock
}

// NewOptimisticLockError creates a new OptimisticLockError.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func NewOptimisticLockError(aggregateId string, expected, actual uint64) *OptimisticLockError {
	return &OptimisticLockError{
		AggregateId:     aggregateId,
		ExpectedVersion: expected,
		ActualVersion:   actual,
	}
}

// AggregateNotFoundError provides information about which aggregate was not found.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
type AggregateNotFoundError struct {
	AggregateId string
	TypeName    string
}

// Error returns the formatted error message.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *AggregateNotFoundError) Error() string {
	return fmt.Sprintf("aggregate not found: type=%s, id=%s", e.TypeName, e.AggregateId)
}

// Is reports whether the target error is ErrAggregateNotFound.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *AggregateNotFoundError) Is(target error) bool {
	return target == ErrAggregateNotFound
}

// NewAggregateNotFoundError creates a new AggregateNotFoundError.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func NewAggregateNotFoundError(typeName, aggregateId string) *AggregateNotFoundError {
	return &AggregateNotFoundError{
		AggregateId: aggregateId,
		TypeName:    typeName,
	}
}

// SerializationError provides details about a serialization failure.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
type SerializationError struct {
	Operation string // "serialize" or "deserialize"
	Target    string // what was being serialized (e.g. "event", "snapshot")
	Cause     error
}

// Error returns the formatted error message.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *SerializationError) Error() string {
	return fmt.Sprintf("failed to %s %s: %v", e.Operation, e.Target, e.Cause)
}

// Unwrap returns the underlying cause of the serialization error.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *SerializationError) Unwrap() error {
	return e.Cause
}

// Is reports whether the target error matches the serialization or deserialization sentinel error.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *SerializationError) Is(target error) bool {
	if e.Operation == "serialize" {
		return target == ErrSerializationFailed
	}
	return target == ErrDeserializationFailed
}

// NewSerializationError creates a new SerializationError.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func NewSerializationError(target string, cause error) *SerializationError {
	return &SerializationError{
		Operation: "serialize",
		Target:    target,
		Cause:     cause,
	}
}

// NewDeserializationError creates a new deserialization error.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func NewDeserializationError(target string, cause error) *SerializationError {
	return &SerializationError{
		Operation: "deserialize",
		Target:    target,
		Cause:     cause,
	}
}

// DuplicateAggregateError provides details when an aggregate with the same ID already exists.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
type DuplicateAggregateError struct {
	AggregateId string
}

// Error returns the formatted error message.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *DuplicateAggregateError) Error() string {
	return fmt.Sprintf("aggregate already exists: id=%s", e.AggregateId)
}

// Is reports whether the target error is ErrDuplicateAggregate.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func (e *DuplicateAggregateError) Is(target error) bool {
	return target == ErrDuplicateAggregate
}

// NewDuplicateAggregateError creates a new DuplicateAggregateError.
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
func NewDuplicateAggregateError(aggregateId string) *DuplicateAggregateError {
	return &DuplicateAggregateError{AggregateId: aggregateId}
}
