package eventstore

import "testing"

func TestAggregateID_AsString(t *testing.T) {
	id := NewAggregateID("Visit", "abc-123")

	if got, want := id.TypeName(), "Visit"; got != want {
		t.Errorf("TypeName() = %q, want %q", got, want)
	}
	if got, want := id.Value(), "abc-123"; got != want {
		t.Errorf("Value() = %q, want %q", got, want)
	}
	if got, want := id.AsString(), "Visit-abc-123"; got != want {
		t.Errorf("AsString() = %q, want %q", got, want)
	}
}
