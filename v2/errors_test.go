package eventsourcing_test

import (
	"errors"
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
)

func TestOptimisticLockError_IsErrOptimisticLock(t *testing.T) {
	err := es.NewOptimisticLockError("Visit-x", 3, 5)
	if !errors.Is(err, es.ErrOptimisticLock) {
		t.Errorf("errors.Is(err, ErrOptimisticLock) = false, want true")
	}
}

func TestAggregateNotFoundError_IsErrAggregateNotFound(t *testing.T) {
	err := es.NewAggregateNotFoundError("Visit", "x")
	if !errors.Is(err, es.ErrAggregateNotFound) {
		t.Errorf("errors.Is(err, ErrAggregateNotFound) = false, want true")
	}
}

func TestDuplicateAggregateError_IsErrDuplicateAggregate(t *testing.T) {
	err := es.NewDuplicateAggregateError("Visit-x")
	if !errors.Is(err, es.ErrDuplicateAggregate) {
		t.Errorf("errors.Is(err, ErrDuplicateAggregate) = false, want true")
	}
}

func TestSerializationError_IsErrSerializationFailed(t *testing.T) {
	err := es.NewSerializationError("event", errors.New("boom"))
	if !errors.Is(err, es.ErrSerializationFailed) {
		t.Errorf("errors.Is(err, ErrSerializationFailed) = false, want true")
	}
}

func TestDeserializationError_IsErrDeserializationFailed(t *testing.T) {
	err := es.NewDeserializationError("event", errors.New("boom"))
	if !errors.Is(err, es.ErrDeserializationFailed) {
		t.Errorf("errors.Is(err, ErrDeserializationFailed) = false, want true")
	}
}

func TestErrUnknownCommand_isSentinel(t *testing.T) {
	if es.ErrUnknownCommand == nil {
		t.Fatalf("ErrUnknownCommand should be non-nil sentinel")
	}
}
