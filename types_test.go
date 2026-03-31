package eventsourcing

import (
	"testing"
	"time"
)

func TestDefaultAggregateID(t *testing.T) {
	t.Run("NewAggregateId creates valid ID", func(t *testing.T) {
		// given
		typeName := "MemorialSetting"
		value := "abc123"

		// when
		id := NewAggregateId(typeName, value)

		// then
		if id.GetTypeName() != "MemorialSetting" {
			t.Errorf("GetTypeName() = %q, want %q", id.GetTypeName(), "MemorialSetting")
		}
		if id.GetValue() != "abc123" {
			t.Errorf("GetValue() = %q, want %q", id.GetValue(), "abc123")
		}
		if id.AsString() != "MemorialSetting-abc123" {
			t.Errorf("AsString() = %q, want %q", id.AsString(), "MemorialSetting-abc123")
		}
	})

	t.Run("AsString format is TypeName-Value", func(t *testing.T) {
		// given
		testCases := []struct {
			typeName string
			value    string
			want     string
		}{
			{"User", "123", "User-123"},
			{"Order", "order-456", "Order-order-456"},
			{"MemorialSetting", "01H42K4ABWQ5V2XQEP3A48VE0Z", "MemorialSetting-01H42K4ABWQ5V2XQEP3A48VE0Z"},
		}

		for _, tc := range testCases {
			// when
			id := NewAggregateId(tc.typeName, tc.value)
			actual := id.AsString()

			// then
			if actual != tc.want {
				t.Errorf("NewAggregateId(%q, %q).AsString() = %q, want %q",
					tc.typeName, tc.value, actual, tc.want)
			}
		}
	})
}

func TestDefaultEvent(t *testing.T) {
	t.Run("NewEvent creates event with defaults", func(t *testing.T) {
		// given
		aggregateID := NewAggregateId("MemorialSetting", "abc123")
		payload := []byte(`{"key": "value"}`)

		// when
		event := NewEvent("event-1", "SettingCreated", aggregateID, payload)

		// then
		if event.GetID() != "event-1" {
			t.Errorf("GetID() = %q, want %q", event.GetID(), "event-1")
		}
		if event.GetTypeName() != "SettingCreated" {
			t.Errorf("GetTypeName() = %q, want %q", event.GetTypeName(), "SettingCreated")
		}
		if event.GetAggregateId().AsString() != "MemorialSetting-abc123" {
			t.Errorf("GetAggregateId().AsString() = %q, want %q",
				event.GetAggregateId().AsString(), "MemorialSetting-abc123")
		}
		if event.GetSeqNr() != 1 {
			t.Errorf("GetSeqNr() = %d, want %d", event.GetSeqNr(), 1)
		}
		if event.IsCreated() != false {
			t.Errorf("IsCreated() = %v, want %v", event.IsCreated(), false)
		}
		if string(event.GetPayload()) != `{"key": "value"}` {
			t.Errorf("GetPayload() = %q, want %q", string(event.GetPayload()), `{"key": "value"}`)
		}
		// OccurredAt should be set to current time (non-zero)
		if event.GetOccurredAt() == 0 {
			t.Error("GetOccurredAt() should not be zero")
		}
	})

	t.Run("WithSeqNr option sets sequence number", func(t *testing.T) {
		// given
		aggregateID := NewAggregateId("Test", "1")

		// when
		event := NewEvent("e1", "TestEvent", aggregateID, nil, WithSeqNr(42))

		// then
		if event.GetSeqNr() != 42 {
			t.Errorf("GetSeqNr() = %d, want %d", event.GetSeqNr(), 42)
		}
	})

	t.Run("WithIsCreated option sets created flag", func(t *testing.T) {
		// given
		aggregateID := NewAggregateId("Test", "1")

		// when
		event := NewEvent("e1", "TestEvent", aggregateID, nil, WithIsCreated(true))

		// then
		if event.IsCreated() != true {
			t.Errorf("IsCreated() = %v, want %v", event.IsCreated(), true)
		}
	})

	t.Run("WithOccurredAt option sets timestamp", func(t *testing.T) {
		// given
		aggregateID := NewAggregateId("Test", "1")
		fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

		// when
		event := NewEvent("e1", "TestEvent", aggregateID, nil, WithOccurredAt(fixedTime))

		// then
		expectedMilli := uint64(fixedTime.UnixMilli())
		if event.GetOccurredAt() != expectedMilli {
			t.Errorf("GetOccurredAt() = %d, want %d", event.GetOccurredAt(), expectedMilli)
		}
	})

	t.Run("WithOccurredAtUnixMilli option sets timestamp from milliseconds", func(t *testing.T) {
		// given
		aggregateID := NewAggregateId("Test", "1")
		unixMilli := uint64(1705315800000) // 2024-01-15 10:30:00 UTC

		// when
		event := NewEvent("e1", "TestEvent", aggregateID, nil, WithOccurredAtUnixMilli(unixMilli))

		// then
		if event.GetOccurredAt() != unixMilli {
			t.Errorf("GetOccurredAt() = %d, want %d", event.GetOccurredAt(), unixMilli)
		}
	})

	t.Run("Multiple options can be combined", func(t *testing.T) {
		// given
		aggregateID := NewAggregateId("Test", "1")

		// when
		event := NewEvent("e1", "Created", aggregateID, []byte("data"),
			WithSeqNr(1),
			WithIsCreated(true),
			WithOccurredAtUnixMilli(1000000),
		)

		// then
		if event.GetSeqNr() != 1 {
			t.Errorf("GetSeqNr() = %d, want %d", event.GetSeqNr(), 1)
		}
		if event.IsCreated() != true {
			t.Errorf("IsCreated() = %v, want %v", event.IsCreated(), true)
		}
		if event.GetOccurredAt() != 1000000 {
			t.Errorf("GetOccurredAt() = %d, want %d", event.GetOccurredAt(), 1000000)
		}
	})
}

func TestSnapshotData(t *testing.T) {
	t.Run("SnapshotData holds data correctly", func(t *testing.T) {
		// given
		payload := []byte(`{"state": "active"}`)
		seqNr := uint64(10)
		version := uint64(2)

		// when
		snapshot := &SnapshotData{
			Payload: payload,
			SeqNr:   seqNr,
			Version: version,
		}

		// then
		if string(snapshot.Payload) != `{"state": "active"}` {
			t.Errorf("Payload = %q, want %q", string(snapshot.Payload), `{"state": "active"}`)
		}
		if snapshot.SeqNr != 10 {
			t.Errorf("SeqNr = %d, want %d", snapshot.SeqNr, 10)
		}
		if snapshot.Version != 2 {
			t.Errorf("Version = %d, want %d", snapshot.Version, 2)
		}
	})
}
