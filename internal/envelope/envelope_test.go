package envelope

import (
	"testing"

	es "github.com/Hiroshi0900/eventstore"
)

func TestFromEvent(t *testing.T) {
	t.Run("FromEvent converts Event to EventEnvelope", func(t *testing.T) {
		// given
		aggregateID := es.NewAggregateId("MemorialSetting", "abc123")
		payload := []byte(`{"key":"value"}`)
		event := es.NewEvent("event-1", "SettingCreated", aggregateID, payload,
			es.WithSeqNr(5),
			es.WithIsCreated(true),
			es.WithOccurredAtUnixMilli(1705315800000),
		)

		// when
		envelope := FromEvent(event)

		// then
		if envelope.Id != "event-1" {
			t.Errorf("Id = %q, want %q", envelope.Id, "event-1")
		}
		if envelope.TypeName != "SettingCreated" {
			t.Errorf("TypeName = %q, want %q", envelope.TypeName, "SettingCreated")
		}
		if envelope.AggregateId != "MemorialSetting-abc123" {
			t.Errorf("AggregateId = %q, want %q", envelope.AggregateId, "MemorialSetting-abc123")
		}
		if envelope.SeqNr != 5 {
			t.Errorf("SeqNr = %d, want %d", envelope.SeqNr, 5)
		}
		if envelope.IsCreated != true {
			t.Errorf("IsCreated = %v, want %v", envelope.IsCreated, true)
		}
		if envelope.OccurredAt != 1705315800000 {
			t.Errorf("OccurredAt = %d, want %d", envelope.OccurredAt, 1705315800000)
		}
		if string(envelope.Payload) != `{"key":"value"}` {
			t.Errorf("Payload = %q, want %q", string(envelope.Payload), `{"key":"value"}`)
		}
	})
}
