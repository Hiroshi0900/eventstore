package memory

import (
	"context"
	"errors"
	"testing"

	es "github.com/Hiroshi0900/eventstore"
)

func TestStore(t *testing.T) {
	ctx := context.Background()

	t.Run("New creates empty store", func(t *testing.T) {
		// given
		// no setup required

		// when
		sut := New()

		// then
		if sut == nil {
			t.Fatal("New() returned nil")
		}
		allEvents := sut.GetAllEvents()
		if len(allEvents) != 0 {
			t.Errorf("New store should have no events, got %d", len(allEvents))
		}
	})

	t.Run("GetLatestSnapshotByID returns nil for non-existent", func(t *testing.T) {
		// given
		sut := New()
		id := es.NewAggregateId("Test", "non-existent")

		// when
		snapshot, err := sut.GetLatestSnapshotByID(ctx, id)

		// then
		if err != nil {
			t.Fatalf("GetLatestSnapshotByID() error = %v", err)
		}
		if snapshot != nil {
			t.Error("GetLatestSnapshotByID() should return nil for non-existent")
		}
	})

	t.Run("GetEventsByIDSinceSeqNr returns empty for non-existent", func(t *testing.T) {
		// given
		sut := New()
		id := es.NewAggregateId("Test", "non-existent")

		// when
		events, err := sut.GetEventsByIDSinceSeqNr(ctx, id, 0)

		// then
		if err != nil {
			t.Fatalf("GetEventsByIDSinceSeqNr() error = %v", err)
		}
		if len(events) != 0 {
			t.Errorf("GetEventsByIDSinceSeqNr() should return empty, got %d events", len(events))
		}
	})

	t.Run("PersistEvent stores event", func(t *testing.T) {
		// given
		sut := New()
		id := es.NewAggregateId("Test", "1")
		event := es.NewEvent("e1", "Created", id, []byte("payload"), es.WithSeqNr(1))

		// when
		err := sut.PersistEvent(ctx, event, 0)

		// then
		if err != nil {
			t.Fatalf("PersistEvent() error = %v", err)
		}
		if sut.GetEventCount(id) != 1 {
			t.Errorf("GetEventCount() = %d, want %d", sut.GetEventCount(id), 1)
		}
		events, _ := sut.GetEventsByIDSinceSeqNr(ctx, id, 0)
		if len(events) != 1 {
			t.Errorf("GetEventsByIDSinceSeqNr() returned %d events, want 1", len(events))
		}
	})

	t.Run("PersistEvent enforces optimistic locking", func(t *testing.T) {
		// given
		sut := New()
		id := es.NewAggregateId("Test", "1")
		event1 := es.NewEvent("e1", "Created", id, nil, es.WithSeqNr(1))
		_ = sut.PersistEvent(ctx, event1, 0)
		event2 := es.NewEvent("e2", "Updated", id, nil, es.WithSeqNr(2))

		// when
		// expectedVersion=0 means "skip version check", so we use a wrong non-zero version
		err := sut.PersistEvent(ctx, event2, 999) // Wrong version (current is 1)

		// then
		if err == nil {
			t.Error("PersistEvent() should fail with wrong version")
		}
		if !errors.Is(err, es.ErrOptimisticLock) {
			t.Errorf("error should be ErrOptimisticLock, got %v", err)
		}
	})

	t.Run("PersistEvent with correct version succeeds", func(t *testing.T) {
		// given
		sut := New()
		id := es.NewAggregateId("Test", "1")
		event1 := es.NewEvent("e1", "Created", id, nil, es.WithSeqNr(1))
		event2 := es.NewEvent("e2", "Updated", id, nil, es.WithSeqNr(2))
		_ = sut.PersistEvent(ctx, event1, 0)

		// when
		err := sut.PersistEvent(ctx, event2, 1) // Correct version

		// then
		if err != nil {
			t.Errorf("PersistEvent() with correct version failed: %v", err)
		}
		if sut.GetEventCount(id) != 2 {
			t.Errorf("GetEventCount() = %d, want %d", sut.GetEventCount(id), 2)
		}
	})

	t.Run("GetEventsByIDSinceSeqNr filters by sequence number", func(t *testing.T) {
		// given
		sut := New()
		id := es.NewAggregateId("Test", "1")
		for i := 1; i <= 5; i++ {
			event := es.NewEvent("e"+string(rune('0'+i)), "Event", id, nil, es.WithSeqNr(uint64(i)))
			_ = sut.PersistEvent(ctx, event, uint64(i-1))
		}

		// when
		events, err := sut.GetEventsByIDSinceSeqNr(ctx, id, 3)

		// then
		if err != nil {
			t.Fatalf("GetEventsByIDSinceSeqNr() error = %v", err)
		}
		if len(events) != 2 {
			t.Errorf("GetEventsByIDSinceSeqNr(3) returned %d events, want 2", len(events))
		}
		if events[0].GetSeqNr() != 4 {
			t.Errorf("First event SeqNr = %d, want 4", events[0].GetSeqNr())
		}
		if events[1].GetSeqNr() != 5 {
			t.Errorf("Second event SeqNr = %d, want 5", events[1].GetSeqNr())
		}
	})

	t.Run("GetEventsByIDSinceSeqNr returns events in order", func(t *testing.T) {
		// given
		sut := New()
		id := es.NewAggregateId("Test", "1")
		// Add events in reverse order (to test sorting)
		for i := 5; i >= 1; i-- {
			event := es.NewEvent("e"+string(rune('0'+i)), "Event", id, nil, es.WithSeqNr(uint64(i)))
			sut.events[id.AsString()] = append(sut.events[id.AsString()], event)
		}

		// when
		events, _ := sut.GetEventsByIDSinceSeqNr(ctx, id, 0)

		// then
		for i := 0; i < len(events)-1; i++ {
			if events[i].GetSeqNr() >= events[i+1].GetSeqNr() {
				t.Errorf("Events not sorted: seqNr %d before %d",
					events[i].GetSeqNr(), events[i+1].GetSeqNr())
			}
		}
	})
}

func TestStorePersistEventAndSnapshot(t *testing.T) {
	ctx := context.Background()

	t.Run("PersistEventAndSnapshot stores both", func(t *testing.T) {
		// given
		sut := New()
		id := es.NewAggregateId("Test", "1")
		event := es.NewEvent("e1", "Created", id, []byte("event-data"), es.WithSeqNr(1))
		aggregate := &testAggregate{id: id, seqNr: 1, version: 0}

		// when
		err := sut.PersistEventAndSnapshot(ctx, event, aggregate)

		// then
		if err != nil {
			t.Fatalf("PersistEventAndSnapshot() error = %v", err)
		}
		events, _ := sut.GetEventsByIDSinceSeqNr(ctx, id, 0)
		if len(events) != 1 {
			t.Errorf("Expected 1 event, got %d", len(events))
		}
		snapshot, _ := sut.GetLatestSnapshotByID(ctx, id)
		if snapshot == nil {
			t.Fatal("Snapshot should be stored")
		}
		if snapshot.SeqNr != 1 {
			t.Errorf("Snapshot SeqNr = %d, want 1", snapshot.SeqNr)
		}
		if snapshot.Version != 1 {
			t.Errorf("Snapshot Version = %d, want 1", snapshot.Version)
		}
	})

	t.Run("PersistEventAndSnapshot enforces optimistic locking", func(t *testing.T) {
		// given
		sut := New()
		id := es.NewAggregateId("Test", "1")
		event1 := es.NewEvent("e1", "Created", id, nil, es.WithSeqNr(1))
		agg1 := &testAggregate{id: id, seqNr: 1, version: 0}
		_ = sut.PersistEventAndSnapshot(ctx, event1, agg1)
		event2 := es.NewEvent("e2", "Updated", id, nil, es.WithSeqNr(2))
		// version=0 means "skip version check", so we use a wrong non-zero version
		agg2 := &testAggregate{id: id, seqNr: 2, version: 999} // Wrong version (current is 1)

		// when
		err := sut.PersistEventAndSnapshot(ctx, event2, agg2)

		// then
		if err == nil {
			t.Error("PersistEventAndSnapshot() should fail with wrong version")
		}
		if !errors.Is(err, es.ErrOptimisticLock) {
			t.Errorf("error should be ErrOptimisticLock, got %v", err)
		}
	})
}

func TestStoreUtilities(t *testing.T) {
	ctx := context.Background()

	t.Run("Clear removes all data", func(t *testing.T) {
		// given
		sut := New()
		id := es.NewAggregateId("Test", "1")
		event := es.NewEvent("e1", "Created", id, nil, es.WithSeqNr(1))
		aggregate := &testAggregate{id: id, seqNr: 1, version: 0}
		_ = sut.PersistEventAndSnapshot(ctx, event, aggregate)

		// when
		sut.Clear()

		// then
		events, _ := sut.GetEventsByIDSinceSeqNr(ctx, id, 0)
		if len(events) != 0 {
			t.Error("Clear() should remove all events")
		}
		snapshot, _ := sut.GetLatestSnapshotByID(ctx, id)
		if snapshot != nil {
			t.Error("Clear() should remove all snapshots")
		}
	})

	t.Run("SetSnapshot allows direct snapshot setting", func(t *testing.T) {
		// given
		sut := New()
		id := es.NewAggregateId("Test", "1")
		snapshot := &es.SnapshotData{
			Payload: []byte("test-payload"),
			SeqNr:   10,
			Version: 5,
		}

		// when
		sut.SetSnapshot(id, snapshot)

		// then
		retrieved, _ := sut.GetLatestSnapshotByID(ctx, id)
		if retrieved == nil {
			t.Fatal("SetSnapshot() should store snapshot")
		}
		if retrieved.SeqNr != 10 {
			t.Errorf("SeqNr = %d, want 10", retrieved.SeqNr)
		}
		if retrieved.Version != 5 {
			t.Errorf("Version = %d, want 5", retrieved.Version)
		}
	})

	t.Run("GetAllEvents returns copy of events", func(t *testing.T) {
		// given
		sut := New()
		id := es.NewAggregateId("Test", "1")
		event := es.NewEvent("e1", "Created", id, nil, es.WithSeqNr(1))
		_ = sut.PersistEvent(ctx, event, 0)

		// when
		allEvents := sut.GetAllEvents()
		allEvents[id.AsString()] = nil // Modify the returned map

		// then
		if sut.GetEventCount(id) != 1 {
			t.Error("GetAllEvents() should return a copy, not the original")
		}
	})
}

// --- Mutant-killing tests ---

// TestGetEventsByIDSinceSeqNr_ExactBoundary kills the CONDITIONALS_BOUNDARY mutant on L63
// (> vs >=): event with seqNr equal to the filter value must NOT be returned.
func TestGetEventsByIDSinceSeqNr_ExactBoundary(t *testing.T) {
	ctx := context.Background()

	t.Run("event with seqNr equal to filter is NOT returned", func(t *testing.T) {
		// given
		// events seqNr 1-6
		sut := New()
		id := es.NewAggregateId("Test", "boundary")
		for i := 1; i <= 6; i++ {
			event := es.NewEvent("e"+string(rune('0'+i)), "Event", id, nil, es.WithSeqNr(uint64(i)))
			_ = sut.PersistEvent(ctx, event, uint64(i-1))
		}

		// when
		// filter with seqNr=5 (strictly greater than 5 expected)
		events, err := sut.GetEventsByIDSinceSeqNr(ctx, id, 5)

		// then
		// only seqNr=6 is returned, seqNr=5 is excluded
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("GetEventsByIDSinceSeqNr(5) returned %d events, want 1", len(events))
		}
		if events[0].GetSeqNr() != 6 {
			t.Errorf("returned event SeqNr = %d, want 6", events[0].GetSeqNr())
		}
	})

	t.Run("event with seqNr one less than filter IS returned", func(t *testing.T) {
		// given
		// events seqNr 1-5
		sut := New()
		id := es.NewAggregateId("Test", "boundary2")
		for i := 1; i <= 5; i++ {
			event := es.NewEvent("e"+string(rune('0'+i)), "Event", id, nil, es.WithSeqNr(uint64(i)))
			_ = sut.PersistEvent(ctx, event, uint64(i-1))
		}

		// when
		// filter with seqNr=4
		events, err := sut.GetEventsByIDSinceSeqNr(ctx, id, 4)

		// then
		// seqNr=5 IS returned
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("GetEventsByIDSinceSeqNr(4) returned %d events, want 1", len(events))
		}
		if events[0].GetSeqNr() != 5 {
			t.Errorf("returned event SeqNr = %d, want 5", events[0].GetSeqNr())
		}
	})
}

// TestPersistEvent_VersionZeroSkipsCheck kills the CONDITIONALS_BOUNDARY mutant on L78
// (expectedVersion > 0): version=0 must skip the optimistic lock check.
func TestPersistEvent_VersionZeroSkipsCheck(t *testing.T) {
	ctx := context.Background()

	t.Run("version=0 skips optimistic lock check on initial insert", func(t *testing.T) {
		// given
		// empty store (currentVersion=0)
		sut := New()
		id := es.NewAggregateId("Test", "newaggregate")
		event := es.NewEvent("e1", "Created", id, nil, es.WithSeqNr(1))

		// when
		// persist with expectedVersion=0 (should skip check even though currentVersion=0)
		err := sut.PersistEvent(ctx, event, 0)

		// then
		// no error
		if err != nil {
			t.Errorf("PersistEvent() with version=0 should skip check, got error: %v", err)
		}
	})

	t.Run("version=1 enforces optimistic lock check", func(t *testing.T) {
		// given
		// empty store (currentVersion=0), but expectedVersion=1 (mismatch)
		sut := New()
		id := es.NewAggregateId("Test", "lockcheck")
		event := es.NewEvent("e1", "Created", id, nil, es.WithSeqNr(1))

		// when
		// persist with expectedVersion=1 but store has currentVersion=0
		err := sut.PersistEvent(ctx, event, 1)

		// then
		// optimistic lock error
		if err == nil {
			t.Error("PersistEvent() with version=1 vs currentVersion=0 should fail")
		}
		if !errors.Is(err, es.ErrOptimisticLock) {
			t.Errorf("error should be ErrOptimisticLock, got %v", err)
		}
	})
}

// TestPersistEvent_VersionIncrement kills the ARITHMETIC_BASE mutant on L86
// (currentVersion + 1 vs - 1): version must be incremented by exactly 1.
func TestPersistEvent_VersionIncrement(t *testing.T) {
	ctx := context.Background()

	t.Run("version increments by exactly 1 after each PersistEvent", func(t *testing.T) {
		// given
		sut := New()
		id := es.NewAggregateId("Test", "incr")
		event1 := es.NewEvent("e1", "Created", id, nil, es.WithSeqNr(1))
		event2 := es.NewEvent("e2", "Updated", id, nil, es.WithSeqNr(2))
		event3 := es.NewEvent("e3", "Updated", id, nil, es.WithSeqNr(3))
		event4 := es.NewEvent("e4", "Updated", id, nil, es.WithSeqNr(4))

		// when
		// First event: version 0 → 1
		if err := sut.PersistEvent(ctx, event1, 0); err != nil {
			t.Fatalf("PersistEvent(1) error: %v", err)
		}
		// Second event requires expectedVersion=1 (not 0 or 2)
		if err := sut.PersistEvent(ctx, event2, 1); err != nil {
			t.Fatalf("PersistEvent(2) with expectedVersion=1 should succeed: %v", err)
		}
		// Third event requires expectedVersion=2 (not 1 or 3)
		if err := sut.PersistEvent(ctx, event3, 2); err != nil {
			t.Fatalf("PersistEvent(3) with expectedVersion=2 should succeed: %v", err)
		}

		// then
		// Wrong version (skipped by 1) must fail
		if err := sut.PersistEvent(ctx, event4, 4); err == nil {
			t.Error("PersistEvent() with version skipped by 1 should fail")
		}
	})
}

// TestPersistEventAndSnapshot_VersionZeroSkipsCheck kills the CONDITIONALS_BOUNDARY mutant on L101
// (expectedVersion > 0): version=0 in the aggregate must skip the optimistic lock check.
func TestPersistEventAndSnapshot_VersionZeroSkipsCheck(t *testing.T) {
	ctx := context.Background()

	t.Run("aggregate version=0 skips optimistic lock check", func(t *testing.T) {
		// given
		// empty store (currentVersion=0)
		sut := New()
		id := es.NewAggregateId("Test", "snapinitial")
		event := es.NewEvent("e1", "Created", id, nil, es.WithSeqNr(1))
		agg := &testAggregate{id: id, seqNr: 1, version: 0}

		// when
		// persist with aggregate.GetVersion()=0 (should skip check)
		err := sut.PersistEventAndSnapshot(ctx, event, agg)

		// then
		// no error
		if err != nil {
			t.Errorf("PersistEventAndSnapshot() with aggregate.version=0 should skip check, got: %v", err)
		}
	})

	t.Run("aggregate version=1 enforces optimistic lock on fresh store", func(t *testing.T) {
		// given
		// empty store (currentVersion=0), aggregate.version=1 (mismatch)
		sut := New()
		id := es.NewAggregateId("Test", "snaplock")
		event := es.NewEvent("e1", "Created", id, nil, es.WithSeqNr(1))
		agg := &testAggregate{id: id, seqNr: 1, version: 1}

		// when
		// persist with aggregate.version=1 but store currentVersion=0
		err := sut.PersistEventAndSnapshot(ctx, event, agg)

		// then
		// optimistic lock error
		if err == nil {
			t.Error("PersistEventAndSnapshot() with version=1 vs currentVersion=0 should fail")
		}
		if !errors.Is(err, es.ErrOptimisticLock) {
			t.Errorf("error should be ErrOptimisticLock, got %v", err)
		}
	})
}

// TestPersistEventAndSnapshot_VersionIncrement kills the ARITHMETIC_BASE mutant on L114/L118
// (currentVersion + 1): snapshot version and store version must both be currentVersion + 1.
func TestPersistEventAndSnapshot_VersionIncrement(t *testing.T) {
	ctx := context.Background()

	t.Run("snapshot version is currentVersion+1 after PersistEventAndSnapshot", func(t *testing.T) {
		// given
		sut := New()
		id := es.NewAggregateId("Test", "snapver")
		event := es.NewEvent("e1", "Created", id, nil, es.WithSeqNr(1))
		agg := &testAggregate{id: id, seqNr: 1, version: 0}

		// when
		if err := sut.PersistEventAndSnapshot(ctx, event, agg); err != nil {
			t.Fatalf("PersistEventAndSnapshot() error: %v", err)
		}

		// then
		snapshot, _ := sut.GetLatestSnapshotByID(ctx, id)
		if snapshot == nil {
			t.Fatal("snapshot should exist")
		}
		// currentVersion was 0, so snapshot.Version must be exactly 1
		if snapshot.Version != 1 {
			t.Errorf("snapshot.Version = %d, want 1 (currentVersion+1)", snapshot.Version)
		}

		// Second persist: currentVersion=1 → snapshot.Version must be 2
		event2 := es.NewEvent("e2", "Updated", id, nil, es.WithSeqNr(2))
		agg2 := &testAggregate{id: id, seqNr: 2, version: 1}
		if err := sut.PersistEventAndSnapshot(ctx, event2, agg2); err != nil {
			t.Fatalf("second PersistEventAndSnapshot() error: %v", err)
		}
		snapshot2, _ := sut.GetLatestSnapshotByID(ctx, id)
		if snapshot2 == nil {
			t.Fatal("snapshot should exist after second persist")
		}
		if snapshot2.Version != 2 {
			t.Errorf("snapshot.Version = %d, want 2 after second persist", snapshot2.Version)
		}
	})
}

// testAggregate is a simple aggregate implementation for testing
type testAggregate struct {
	id      es.AggregateID
	seqNr   uint64
	version uint64
}

func (a *testAggregate) GetId() es.AggregateID {
	return a.id
}

func (a *testAggregate) GetSeqNr() uint64 {
	return a.seqNr
}

func (a *testAggregate) GetVersion() uint64 {
	return a.version
}

func (a *testAggregate) WithVersion(version uint64) es.Aggregate {
	return &testAggregate{id: a.id, seqNr: a.seqNr, version: version}
}

func (a *testAggregate) WithSeqNr(seqNr uint64) es.Aggregate {
	return &testAggregate{id: a.id, seqNr: seqNr, version: a.version}
}
