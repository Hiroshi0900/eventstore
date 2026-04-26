package eventstore

import (
	"errors"
	"testing"
)

func TestOptimisticLockError_IsErrOptimisticLock(t *testing.T) {
	err := NewOptimisticLockError("Visit-v1", 5, 3)
	if !errors.Is(err, ErrOptimisticLock) {
		t.Errorf("expected errors.Is(err, ErrOptimisticLock) = true")
	}
}

func TestAggregateNotFoundError_IsErrAggregateNotFound(t *testing.T) {
	err := NewAggregateNotFoundError("Visit", "v1")
	if !errors.Is(err, ErrAggregateNotFound) {
		t.Errorf("expected errors.Is(err, ErrAggregateNotFound) = true")
	}
}

func TestSerializationError_IsErrSerializationFailed(t *testing.T) {
	cause := errors.New("boom")
	err := NewSerializationError("event", cause)
	if !errors.Is(err, ErrSerializationFailed) {
		t.Errorf("expected errors.Is(err, ErrSerializationFailed) = true")
	}
	if !errors.Is(err, cause) {
		t.Errorf("expected unwrap chain to include cause")
	}
}

func TestDeserializationError_IsErrDeserializationFailed(t *testing.T) {
	err := NewDeserializationError("snapshot", errors.New("bad"))
	if !errors.Is(err, ErrDeserializationFailed) {
		t.Errorf("expected errors.Is(err, ErrDeserializationFailed) = true")
	}
}

func TestDuplicateAggregateError_IsErrDuplicateAggregate(t *testing.T) {
	err := NewDuplicateAggregateError("Visit-v1")
	if !errors.Is(err, ErrDuplicateAggregate) {
		t.Errorf("expected errors.Is(err, ErrDuplicateAggregate) = true")
	}
}

func TestAggregateIDMismatchError_IsErrAggregateIDMismatch(t *testing.T) {
	err := NewAggregateIDMismatchError("Visit-v1", "Visit-v2")
	if !errors.Is(err, ErrAggregateIDMismatch) {
		t.Errorf("expected errors.Is(err, ErrAggregateIDMismatch) = true")
	}
}
