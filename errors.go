package eventstore

import (
	"errors"
	"fmt"
)

// Sentinel errors.
var (
	ErrOptimisticLock        = errors.New("optimistic lock error: concurrent modification detected")
	ErrAggregateNotFound     = errors.New("aggregate not found")
	ErrSerializationFailed   = errors.New("serialization failed")
	ErrDeserializationFailed = errors.New("deserialization failed")
	ErrInvalidEvent          = errors.New("invalid event")
	ErrInvalidAggregate      = errors.New("invalid aggregate")
	ErrEventStoreUnavailable = errors.New("event store unavailable")
	ErrDuplicateAggregate    = errors.New("aggregate already exists")
	ErrUnknownCommand        = errors.New("unknown command type for current state")
)

// OptimisticLockError は楽観ロック失敗の詳細を持ちます。
type OptimisticLockError struct {
	AggregateID     string
	ExpectedVersion uint64
	ActualVersion   uint64
}

func (e *OptimisticLockError) Error() string {
	return fmt.Sprintf("optimistic lock error: aggregate %s expected version %d but got %d",
		e.AggregateID, e.ExpectedVersion, e.ActualVersion)
}
func (e *OptimisticLockError) Is(target error) bool { return target == ErrOptimisticLock }

func NewOptimisticLockError(aggregateID string, expected, actual uint64) *OptimisticLockError {
	return &OptimisticLockError{AggregateID: aggregateID, ExpectedVersion: expected, ActualVersion: actual}
}

// AggregateNotFoundError は集約が存在しないことを示します。
type AggregateNotFoundError struct {
	TypeName    string
	AggregateID string
}

func (e *AggregateNotFoundError) Error() string {
	return fmt.Sprintf("aggregate not found: type=%s, id=%s", e.TypeName, e.AggregateID)
}
func (e *AggregateNotFoundError) Is(target error) bool { return target == ErrAggregateNotFound }

func NewAggregateNotFoundError(typeName, aggregateID string) *AggregateNotFoundError {
	return &AggregateNotFoundError{TypeName: typeName, AggregateID: aggregateID}
}

// SerializationError はシリアライゼーション/デシリアライゼーション失敗を表します。
type SerializationError struct {
	Operation string // "serialize" or "deserialize"
	Target    string // "event" / "aggregate" / "snapshot"
	Cause     error
}

func (e *SerializationError) Error() string {
	return fmt.Sprintf("failed to %s %s: %v", e.Operation, e.Target, e.Cause)
}
func (e *SerializationError) Unwrap() error { return e.Cause }
func (e *SerializationError) Is(target error) bool {
	if e.Operation == "serialize" {
		return target == ErrSerializationFailed
	}
	return target == ErrDeserializationFailed
}

func NewSerializationError(target string, cause error) *SerializationError {
	return &SerializationError{Operation: "serialize", Target: target, Cause: cause}
}

func NewDeserializationError(target string, cause error) *SerializationError {
	return &SerializationError{Operation: "deserialize", Target: target, Cause: cause}
}

// DuplicateAggregateError は同じ ID の集約が既に存在することを示します。
type DuplicateAggregateError struct {
	AggregateID string
}

func (e *DuplicateAggregateError) Error() string {
	return fmt.Sprintf("aggregate already exists: id=%s", e.AggregateID)
}
func (e *DuplicateAggregateError) Is(target error) bool { return target == ErrDuplicateAggregate }

func NewDuplicateAggregateError(aggregateID string) *DuplicateAggregateError {
	return &DuplicateAggregateError{AggregateID: aggregateID}
}

