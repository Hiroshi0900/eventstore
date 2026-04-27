package eventsourcing_test

import (
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
)

func TestNewAggregateID(t *testing.T) {
	id := es.NewAggregateID("Visit", "01J0000000")

	if got := id.TypeName(); got != "Visit" {
		t.Errorf("TypeName(): got %q, want %q", got, "Visit")
	}
	if got := id.Value(); got != "01J0000000" {
		t.Errorf("Value(): got %q, want %q", got, "01J0000000")
	}
	if got := id.AsString(); got != "Visit-01J0000000" {
		t.Errorf("AsString(): got %q, want %q", got, "Visit-01J0000000")
	}
}
