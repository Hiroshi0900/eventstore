package eventsourcing

import (
	"errors"
	"fmt"
)

// Sentinel errors for the eventsourcing library.
var (
	// ErrOptimisticLock is returned when a concurrent modification is detected.
	// It occurs when the version in the store does not match the expected version.
	ErrOptimisticLock = errors.New("optimistic lock error: concurrent modification detected")

	// ErrAggregateNotFound is returned when an aggregate is not found in the store.
	ErrAggregateNotFound = errors.New("aggregate not found")

	// ErrSerializationFailed is returned when serialization of an event or snapshot fails.
	ErrSerializationFailed = errors.New("serialization failed")

	// ErrDeserializationFailed is returned when deserialization of an event or snapshot fails.
	ErrDeserializationFailed = errors.New("deserialization failed")

	// ErrInvalidEvent is returned when an event is invalid or malformed.
	ErrInvalidEvent = errors.New("invalid event")

	// ErrInvalidAggregate is returned when an aggregate is invalid or malformed.
	ErrInvalidAggregate = errors.New("invalid aggregate")

	// ErrEventStoreUnavailable is returned when the event store is not available.
	ErrEventStoreUnavailable = errors.New("event store unavailable")

	// ErrDuplicateAggregate is returned when an aggregate with the same ID already exists.
	// This is used to detect duplicate creation when using deterministic IDs.
	ErrDuplicateAggregate = errors.New("aggregate already exists")
)

// OptimisticLockError provides detailed information about a lock conflict.
type OptimisticLockError struct {
	AggregateId     string
	ExpectedVersion uint64
	ActualVersion   uint64
}

func (e *OptimisticLockError) Error() string {
	return fmt.Sprintf(
		"optimistic lock error: aggregate %s expected version %d but got %d",
		e.AggregateId, e.ExpectedVersion, e.ActualVersion,
	)
}

func (e *OptimisticLockError) Is(target error) bool {
	return target == ErrOptimisticLock
}

// NewOptimisticLockError creates a new OptimisticLockError.
func NewOptimisticLockError(aggregateId string, expected, actual uint64) *OptimisticLockError {
	return &OptimisticLockError{
		AggregateId:     aggregateId,
		ExpectedVersion: expected,
		ActualVersion:   actual,
	}
}

// AggregateNotFoundError provides information about which aggregate was not found.
type AggregateNotFoundError struct {
	AggregateId string
	TypeName    string
}

func (e *AggregateNotFoundError) Error() string {
	return fmt.Sprintf("aggregate not found: type=%s, id=%s", e.TypeName, e.AggregateId)
}

func (e *AggregateNotFoundError) Is(target error) bool {
	return target == ErrAggregateNotFound
}

// NewAggregateNotFoundError creates a new AggregateNotFoundError.
func NewAggregateNotFoundError(typeName, aggregateId string) *AggregateNotFoundError {
	return &AggregateNotFoundError{
		AggregateId: aggregateId,
		TypeName:    typeName,
	}
}

// SerializationError provides details about a serialization failure.
type SerializationError struct {
	Operation string // "serialize" or "deserialize"
	Target    string // what was being serialized (e.g. "event", "snapshot")
	Cause     error
}

func (e *SerializationError) Error() string {
	return fmt.Sprintf("failed to %s %s: %v", e.Operation, e.Target, e.Cause)
}

func (e *SerializationError) Unwrap() error {
	return e.Cause
}

func (e *SerializationError) Is(target error) bool {
	if e.Operation == "serialize" {
		return target == ErrSerializationFailed
	}
	return target == ErrDeserializationFailed
}

// NewSerializationError creates a new SerializationError.
func NewSerializationError(target string, cause error) *SerializationError {
	return &SerializationError{
		Operation: "serialize",
		Target:    target,
		Cause:     cause,
	}
}

// NewDeserializationError creates a new deserialization error.
func NewDeserializationError(target string, cause error) *SerializationError {
	return &SerializationError{
		Operation: "deserialize",
		Target:    target,
		Cause:     cause,
	}
}

// DuplicateAggregateError provides details when an aggregate with the same ID already exists.
type DuplicateAggregateError struct {
	AggregateId string
}

func (e *DuplicateAggregateError) Error() string {
	return fmt.Sprintf("aggregate already exists: id=%s", e.AggregateId)
}

func (e *DuplicateAggregateError) Is(target error) bool {
	return target == ErrDuplicateAggregate
}

// NewDuplicateAggregateError creates a new DuplicateAggregateError.
func NewDuplicateAggregateError(aggregateId string) *DuplicateAggregateError {
	return &DuplicateAggregateError{AggregateId: aggregateId}
}
