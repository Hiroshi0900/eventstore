package memory

import (
	"context"
	"errors"
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

func TestStore_PersistEvent_FirstEvent(t *testing.T) {
	s := New()
	id := es.NewAggregateID("Visit", "v1")
	ev := es.NewEvent("evt-1", "VisitScheduled", id, []byte("p"),
		es.WithSeqNr(1), es.WithIsCreated(true))

	if err := s.PersistEvent(context.Background(), ev, 0); err != nil {
		t.Fatalf("PersistEvent err: %v", err)
	}

	got, err := s.GetEventsByIDSinceSeqNr(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsByIDSinceSeqNr err: %v", err)
	}
	if len(got) != 1 || got[0].EventID() != "evt-1" {
		t.Errorf("expected single event evt-1, got %+v", got)
	}
}

func TestStore_PersistEvent_DuplicateOnCreate(t *testing.T) {
	s := New()
	id := es.NewAggregateID("Visit", "v1")
	ev := es.NewEvent("evt-1", "VisitScheduled", id, nil, es.WithSeqNr(1))

	if err := s.PersistEvent(context.Background(), ev, 0); err != nil {
		t.Fatalf("first PersistEvent err: %v", err)
	}
	err := s.PersistEvent(context.Background(), ev, 0)
	if !errors.Is(err, es.ErrDuplicateAggregate) {
		t.Errorf("expected ErrDuplicateAggregate, got %v", err)
	}
}

func TestStore_PersistEvent_OptimisticLock(t *testing.T) {
	s := New()
	id := es.NewAggregateID("Visit", "v1")
	ev1 := es.NewEvent("evt-1", "VisitScheduled", id, nil, es.WithSeqNr(1))
	ev2 := es.NewEvent("evt-2", "VisitCompleted", id, nil, es.WithSeqNr(2))

	if err := s.PersistEvent(context.Background(), ev1, 0); err != nil {
		t.Fatalf("ev1 err: %v", err)
	}
	err := s.PersistEvent(context.Background(), ev2, 99)
	if !errors.Is(err, es.ErrOptimisticLock) {
		t.Errorf("expected ErrOptimisticLock, got %v", err)
	}
}

func TestStore_PersistEventAndSnapshot_First(t *testing.T) {
	s := New()
	id := es.NewAggregateID("Visit", "v1")
	ev := es.NewEvent("evt-1", "VisitScheduled", id, nil,
		es.WithSeqNr(1), es.WithIsCreated(true))
	snap := es.SnapshotData{Payload: []byte("snap-1"), SeqNr: 1, Version: 1}

	if err := s.PersistEventAndSnapshot(context.Background(), ev, snap); err != nil {
		t.Fatalf("PersistEventAndSnapshot err: %v", err)
	}

	got, err := s.GetLatestSnapshotByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshotByID err: %v", err)
	}
	if got == nil {
		t.Fatal("snapshot is nil")
	}
	if string(got.Payload) != "snap-1" || got.SeqNr != 1 || got.Version != 1 {
		t.Errorf("snapshot mismatch: %+v", got)
	}

	events, _ := s.GetEventsByIDSinceSeqNr(context.Background(), id, 0)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestStore_PersistEventAndSnapshot_OptimisticLock(t *testing.T) {
	s := New()
	id := es.NewAggregateID("Visit", "v1")
	ev1 := es.NewEvent("evt-1", "VisitScheduled", id, nil, es.WithSeqNr(1))
	snap1 := es.SnapshotData{Payload: []byte("v1"), SeqNr: 1, Version: 1}
	if err := s.PersistEventAndSnapshot(context.Background(), ev1, snap1); err != nil {
		t.Fatalf("first err: %v", err)
	}

	ev2 := es.NewEvent("evt-2", "VisitCompleted", id, nil, es.WithSeqNr(2))
	snap2 := es.SnapshotData{Payload: []byte("v2"), SeqNr: 2, Version: 99}
	err := s.PersistEventAndSnapshot(context.Background(), ev2, snap2)
	if !errors.Is(err, es.ErrOptimisticLock) {
		t.Errorf("expected ErrOptimisticLock, got %v", err)
	}
}
