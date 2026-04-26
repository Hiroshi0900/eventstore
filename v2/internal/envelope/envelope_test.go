package envelope

import (
	"testing"
	"time"

	es "github.com/Hiroshi0900/eventstore/v2"
)

func TestFromEvent(t *testing.T) {
	t.Run("FromEvent converts Event to EventEnvelope", func(t *testing.T) {
		// given
		aggregateID := es.NewAggregateID("MemorialSetting", "abc123")
		payload := []byte(`{"key":"value"}`)
		unixMilli := int64(1705315800000)
		event := es.NewEvent("event-1", "SettingCreated", aggregateID, payload,
			es.WithSeqNr(5),
			es.WithIsCreated(true),
			es.WithOccurredAt(time.UnixMilli(unixMilli)),
		)

		// when
		env := FromEvent(event)

		// then
		if env.ID != "event-1" {
			t.Errorf("ID = %q, want %q", env.ID, "event-1")
		}
		if env.TypeName != "SettingCreated" {
			t.Errorf("TypeName = %q, want %q", env.TypeName, "SettingCreated")
		}
		if env.AggregateID != "MemorialSetting-abc123" {
			t.Errorf("AggregateID = %q, want %q", env.AggregateID, "MemorialSetting-abc123")
		}
		if env.SeqNr != 5 {
			t.Errorf("SeqNr = %d, want %d", env.SeqNr, 5)
		}
		if env.IsCreated != true {
			t.Errorf("IsCreated = %v, want %v", env.IsCreated, true)
		}
		if env.OccurredAt != unixMilli {
			t.Errorf("OccurredAt = %d, want %d", env.OccurredAt, unixMilli)
		}
		if string(env.Payload) != `{"key":"value"}` {
			t.Errorf("Payload = %q, want %q", string(env.Payload), `{"key":"value"}`)
		}
	})
}
