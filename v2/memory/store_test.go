package memory

import (
	"context"
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
)

func TestNew_Empty(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}

	id := es.NewAggregateID("Visit", "v1")
	snap, err := s.GetLatestSnapshotByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshotByID err: %v", err)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot, got %+v", snap)
	}

	events, err := s.GetEventsByIDSinceSeqNr(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsByIDSinceSeqNr err: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty events, got %d", len(events))
	}
}
