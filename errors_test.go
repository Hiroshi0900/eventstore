package eventsourcing

import (
	"errors"
	"fmt"
	"testing"
)

func TestOptimisticLockError(t *testing.T) {
	t.Run("Error message contains details", func(t *testing.T) {
		// given
		aggregateID := "MemorialSetting-abc123"
		expectedVersion := uint64(5)
		actualVersion := uint64(6)

		// when
		err := NewOptimisticLockError(aggregateID, expectedVersion, actualVersion)
		msg := err.Error()

		// then
		if msg == "" {
			t.Error("Error() should not be empty")
		}
		if !contains(msg, "MemorialSetting-abc123") {
			t.Errorf("Error() = %q, should contain aggregate ID", msg)
		}
		if !contains(msg, "5") || !contains(msg, "6") {
			t.Errorf("Error() = %q, should contain version numbers", msg)
		}
	})

	t.Run("Is matches ErrOptimisticLock", func(t *testing.T) {
		// given
		err := NewOptimisticLockError("Test-1", 1, 2)

		// when
		actual := errors.Is(err, ErrOptimisticLock)

		// then
		if !actual {
			t.Error("errors.Is(err, ErrOptimisticLock) should be true")
		}
	})

	t.Run("Is does not match other errors", func(t *testing.T) {
		// given
		err := NewOptimisticLockError("Test-1", 1, 2)

		// when
		actual := errors.Is(err, ErrAggregateNotFound)

		// then
		if actual {
			t.Error("errors.Is(err, ErrAggregateNotFound) should be false")
		}
	})
}

func TestAggregateNotFoundError(t *testing.T) {
	t.Run("Error message contains details", func(t *testing.T) {
		// given
		typeName := "MemorialSetting"
		id := "abc123"

		// when
		err := NewAggregateNotFoundError(typeName, id)
		msg := err.Error()

		// then
		if !contains(msg, "MemorialSetting") {
			t.Errorf("Error() = %q, should contain type name", msg)
		}
		if !contains(msg, "abc123") {
			t.Errorf("Error() = %q, should contain aggregate ID", msg)
		}
	})

	t.Run("Is matches ErrAggregateNotFound", func(t *testing.T) {
		// given
		err := NewAggregateNotFoundError("Test", "1")

		// when
		actual := errors.Is(err, ErrAggregateNotFound)

		// then
		if !actual {
			t.Error("errors.Is(err, ErrAggregateNotFound) should be true")
		}
	})

	t.Run("Is does not match other errors", func(t *testing.T) {
		// given
		err := NewAggregateNotFoundError("Test", "1")

		// when
		actual := errors.Is(err, ErrOptimisticLock)

		// then
		if actual {
			t.Error("errors.Is(err, ErrOptimisticLock) should be false")
		}
	})
}

func TestSerializationError(t *testing.T) {
	t.Run("NewSerializationError creates serialize error", func(t *testing.T) {
		// given
		cause := fmt.Errorf("json: unsupported type")
		target := "event"

		// when
		err := NewSerializationError(target, cause)
		msg := err.Error()

		// then
		if !contains(msg, "event") {
			t.Errorf("Error() = %q, should contain target", msg)
		}
		if !contains(msg, "serialize") {
			t.Errorf("Error() = %q, should contain operation", msg)
		}
	})

	t.Run("NewDeserializationError creates deserialize error", func(t *testing.T) {
		// given
		cause := fmt.Errorf("json: invalid character")
		target := "snapshot"

		// when
		err := NewDeserializationError(target, cause)
		msg := err.Error()

		// then
		if !contains(msg, "snapshot") {
			t.Errorf("Error() = %q, should contain target", msg)
		}
		if !contains(msg, "deserialize") {
			t.Errorf("Error() = %q, should contain operation", msg)
		}
	})

	t.Run("Serialize error Is matches ErrSerializationFailed", func(t *testing.T) {
		// given
		err := NewSerializationError("event", fmt.Errorf("cause"))

		// when
		matchesSerialization := errors.Is(err, ErrSerializationFailed)
		matchesDeserialization := errors.Is(err, ErrDeserializationFailed)

		// then
		if !matchesSerialization {
			t.Error("errors.Is(err, ErrSerializationFailed) should be true")
		}
		if matchesDeserialization {
			t.Error("errors.Is(err, ErrDeserializationFailed) should be false")
		}
	})

	t.Run("Deserialize error Is matches ErrDeserializationFailed", func(t *testing.T) {
		// given
		err := NewDeserializationError("event", fmt.Errorf("cause"))

		// when
		matchesDeserialization := errors.Is(err, ErrDeserializationFailed)
		matchesSerialization := errors.Is(err, ErrSerializationFailed)

		// then
		if !matchesDeserialization {
			t.Error("errors.Is(err, ErrDeserializationFailed) should be true")
		}
		if matchesSerialization {
			t.Error("errors.Is(err, ErrSerializationFailed) should be false")
		}
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		// given
		cause := fmt.Errorf("original error")
		err := NewSerializationError("event", cause)

		// when
		unwrapped := errors.Unwrap(err)

		// then
		if unwrapped != cause {
			t.Error("Unwrap() should return the original cause")
		}
	})

	t.Run("errors.Is works with wrapped cause", func(t *testing.T) {
		// given
		cause := fmt.Errorf("wrapped: %w", ErrInvalidEvent)
		err := NewSerializationError("event", cause)

		// when
		actual := errors.Is(err, ErrInvalidEvent)

		// then
		if !actual {
			t.Error("errors.Is(err, ErrInvalidEvent) should be true through unwrap")
		}
	})
}

func TestSentinelErrors(t *testing.T) {
	// given
	sentinels := []error{
		ErrOptimisticLock,
		ErrAggregateNotFound,
		ErrSerializationFailed,
		ErrDeserializationFailed,
		ErrInvalidEvent,
		ErrInvalidAggregate,
		ErrEventStoreUnavailable,
	}

	// when
	for i, e1 := range sentinels {
		for j, e2 := range sentinels {
			// then
			if i != j && errors.Is(e1, e2) {
				t.Errorf("Sentinel errors should be distinct: %v == %v", e1, e2)
			}
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
