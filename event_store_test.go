package eventsourcing

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestDefaultRepository(t *testing.T) {
	ctx := context.Background()

	t.Run("FindByID returns error for non-existent aggregate", func(t *testing.T) {
		// given
		store := newMockStore()
		factory := &testAggregateFactory{}
		serializer := NewJSONSerializer()
		config := DefaultEventStoreConfig()
		sut := NewRepository[*testAgg](store, factory, serializer, config)

		// when
		_, err := sut.FindByID(ctx, NewAggregateId("Test", "non-existent"))

		// then
		if err == nil {
			t.Error("FindByID() should return error for non-existent aggregate")
		}
		if !errors.Is(err, ErrAggregateNotFound) {
			t.Errorf("error should be ErrAggregateNotFound, got %v", err)
		}
	})

	t.Run("FindByID loads aggregate from events", func(t *testing.T) {
		// given
		store := newMockStore()
		factory := &testAggregateFactory{}
		serializer := NewJSONSerializer()
		config := DefaultEventStoreConfig()
		id := NewAggregateId("Test", "1")
		event1 := NewEvent("e1", "Created", id, []byte(`{"name":"initial"}`),
			WithSeqNr(1), WithIsCreated(true))
		event2 := NewEvent("e2", "Updated", id, []byte(`{"name":"updated"}`),
			WithSeqNr(2))
		store.events[id.AsString()] = []Event{event1, event2}
		sut := NewRepository[*testAgg](store, factory, serializer, config)

		// when
		agg, err := sut.FindByID(ctx, id)

		// then
		if err != nil {
			t.Fatalf("FindByID() error = %v", err)
		}
		if agg.Name != "updated" {
			t.Errorf("aggregate.Name = %q, want %q", agg.Name, "updated")
		}
	})

	t.Run("FindByID uses snapshot when available", func(t *testing.T) {
		// given
		store := newMockStore()
		factory := &testAggregateFactory{}
		serializer := NewJSONSerializer()
		config := DefaultEventStoreConfig()
		id := NewAggregateId("Test", "1")
		snapshotData, _ := json.Marshal(&testAgg{
			ID:      id,
			SeqNr:   5,
			Version: 1,
			Name:    "from-snapshot",
		})
		store.snapshots[id.AsString()] = &SnapshotData{
			Payload: snapshotData,
			SeqNr:   5,
			Version: 1,
		}
		event := NewEvent("e6", "Updated", id, []byte(`{"name":"after-snapshot"}`),
			WithSeqNr(6))
		store.events[id.AsString()] = []Event{event}
		store.eventSeqNrFilter = 5
		sut := NewRepository[*testAgg](store, factory, serializer, config)

		// when
		agg, err := sut.FindByID(ctx, id)

		// then
		if err != nil {
			t.Fatalf("FindByID() error = %v", err)
		}
		if agg.Name != "after-snapshot" {
			t.Errorf("aggregate.Name = %q, want %q", agg.Name, "after-snapshot")
		}
	})

	t.Run("Save persists events", func(t *testing.T) {
		// given
		store := newMockStore()
		factory := &testAggregateFactory{}
		serializer := NewJSONSerializer()
		config := DefaultEventStoreConfig()
		id := NewAggregateId("Test", "1")
		agg := &testAgg{ID: id, SeqNr: 1, Version: 0, Name: "test"}
		event := NewEvent("e1", "Created", id, []byte(`{"name":"test"}`), WithSeqNr(1))
		sut := NewRepository[*testAgg](store, factory, serializer, config)

		// when
		err := sut.Save(ctx, agg, []Event{event})

		// then
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		if len(store.persistedEvents) != 1 {
			t.Errorf("Expected 1 persisted event, got %d", len(store.persistedEvents))
		}
	})

	t.Run("Save with empty events does nothing", func(t *testing.T) {
		// given
		store := newMockStore()
		factory := &testAggregateFactory{}
		serializer := NewJSONSerializer()
		config := DefaultEventStoreConfig()
		id := NewAggregateId("Test", "1")
		agg := &testAgg{ID: id, SeqNr: 0, Version: 0}
		sut := NewRepository[*testAgg](store, factory, serializer, config)

		// when
		err := sut.Save(ctx, agg, []Event{})

		// then
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		if len(store.persistedEvents) != 0 {
			t.Error("Save() with empty events should not persist anything")
		}
	})

	t.Run("Save creates snapshot at interval", func(t *testing.T) {
		// given
		store := newMockStore()
		factory := &testAggregateFactory{}
		serializer := NewJSONSerializer()
		config := EventStoreConfig{SnapshotInterval: 5}
		id := NewAggregateId("Test", "1")
		agg := &testAgg{ID: id, SeqNr: 5, Version: 0, Name: "test"}
		event := NewEvent("e5", "Updated", id, nil, WithSeqNr(5))
		sut := NewRepository[*testAgg](store, factory, serializer, config)

		// when
		err := sut.Save(ctx, agg, []Event{event})

		// then
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		if !store.snapshotCreated {
			t.Error("Save() should create snapshot at interval")
		}
	})

	t.Run("Save does not create snapshot before interval", func(t *testing.T) {
		// given
		store := newMockStore()
		factory := &testAggregateFactory{}
		serializer := NewJSONSerializer()
		config := EventStoreConfig{SnapshotInterval: 5}
		id := NewAggregateId("Test", "1")
		agg := &testAgg{ID: id, SeqNr: 3, Version: 0, Name: "test"}
		event := NewEvent("e3", "Updated", id, nil, WithSeqNr(3))
		sut := NewRepository[*testAgg](store, factory, serializer, config)

		// when
		err := sut.Save(ctx, agg, []Event{event})

		// then
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		if store.snapshotCreated {
			t.Error("Save() should not create snapshot before interval")
		}
	})
}

func TestEventStoreConfig(t *testing.T) {
	t.Run("DefaultEventStoreConfig returns expected values", func(t *testing.T) {
		// given
		// no setup required

		// when
		config := DefaultEventStoreConfig()

		// then
		if config.SnapshotInterval != 5 {
			t.Errorf("SnapshotInterval = %d, want 5", config.SnapshotInterval)
		}
		if config.JournalTableName != "journal" {
			t.Errorf("JournalTableName = %q, want %q", config.JournalTableName, "journal")
		}
		if config.SnapshotTableName != "snapshot" {
			t.Errorf("SnapshotTableName = %q, want %q", config.SnapshotTableName, "snapshot")
		}
	})

	t.Run("ShouldSnapshot returns true at interval", func(t *testing.T) {
		// given
		config := EventStoreConfig{SnapshotInterval: 5}
		testCases := []struct {
			seqNr  uint64
			expect bool
		}{
			{0, true},
			{1, false},
			{4, false},
			{5, true},
			{10, true},
			{11, false},
		}

		for _, tc := range testCases {
			// when
			actual := config.ShouldSnapshot(tc.seqNr)

			// then
			if actual != tc.expect {
				t.Errorf("ShouldSnapshot(%d) = %v, want %v", tc.seqNr, actual, tc.expect)
			}
		}
	})

	t.Run("ShouldSnapshot returns false when interval is 0", func(t *testing.T) {
		// given
		config := EventStoreConfig{SnapshotInterval: 0}

		for seqNr := uint64(0); seqNr < 10; seqNr++ {
			// when
			actual := config.ShouldSnapshot(seqNr)

			// then
			if actual {
				t.Errorf("ShouldSnapshot(%d) should be false when interval is 0", seqNr)
			}
		}
	})
}

// TestRepository_Save_SnapshotBoundary kills the CONDITIONALS_NEGATION mutants on L169/L174.
// ShouldSnapshot(seqNr) == true → PersistEventAndSnapshot must be called.
// ShouldSnapshot(seqNr) == false → PersistEvent must be called.
func TestRepository_Save_SnapshotBoundary(t *testing.T) {
	ctx := context.Background()

	t.Run("seqNr at interval boundary triggers PersistEventAndSnapshot", func(t *testing.T) {
		// given
		// interval=5, seqNr=5 → ShouldSnapshot returns true
		store := newMockStore()
		factory := &testAggregateFactory{}
		serializer := NewJSONSerializer()
		config := EventStoreConfig{SnapshotInterval: 5}
		id := NewAggregateId("Test", "snap-boundary")
		agg := &testAgg{ID: id, SeqNr: 5, Version: 0, Name: "test"}
		event := NewEvent("e5", "Event", id, nil, WithSeqNr(5))
		sut := NewRepository[*testAgg](store, factory, serializer, config)

		// when
		if err := sut.Save(ctx, agg, []Event{event}); err != nil {
			t.Fatalf("Save() error: %v", err)
		}

		// then
		// snapshot was created, NOT just a plain event persist
		if !store.snapshotCreated {
			t.Error("Save() with seqNr=5 and interval=5 must call PersistEventAndSnapshot")
		}
	})

	t.Run("seqNr just before interval does NOT trigger PersistEventAndSnapshot", func(t *testing.T) {
		// given
		// interval=5, seqNr=4 → ShouldSnapshot returns false
		store := newMockStore()
		factory := &testAggregateFactory{}
		serializer := NewJSONSerializer()
		config := EventStoreConfig{SnapshotInterval: 5}
		id := NewAggregateId("Test", "no-snap")
		agg := &testAgg{ID: id, SeqNr: 4, Version: 0, Name: "test"}
		event := NewEvent("e4", "Event", id, nil, WithSeqNr(4))
		sut := NewRepository[*testAgg](store, factory, serializer, config)

		// when
		if err := sut.Save(ctx, agg, []Event{event}); err != nil {
			t.Fatalf("Save() error: %v", err)
		}

		// then
		// no snapshot; plain PersistEvent only
		if store.snapshotCreated {
			t.Error("Save() with seqNr=4 and interval=5 must NOT call PersistEventAndSnapshot")
		}
		if len(store.persistedEvents) != 1 {
			t.Errorf("expected 1 persisted event via PersistEvent, got %d", len(store.persistedEvents))
		}
	})

	t.Run("seqNr=10 (second interval) triggers PersistEventAndSnapshot", func(t *testing.T) {
		// given
		// interval=5, seqNr=10 → ShouldSnapshot returns true
		store := newMockStore()
		factory := &testAggregateFactory{}
		serializer := NewJSONSerializer()
		config := EventStoreConfig{SnapshotInterval: 5}
		id := NewAggregateId("Test", "snap-second")
		agg := &testAgg{ID: id, SeqNr: 10, Version: 0, Name: "test"}
		event := NewEvent("e10", "Event", id, nil, WithSeqNr(10))
		sut := NewRepository[*testAgg](store, factory, serializer, config)

		// when
		if err := sut.Save(ctx, agg, []Event{event}); err != nil {
			t.Fatalf("Save() error: %v", err)
		}

		// then
		// snapshot was created
		if !store.snapshotCreated {
			t.Error("Save() with seqNr=10 and interval=5 must call PersistEventAndSnapshot")
		}
	})

	t.Run("seqNr=6 (just after interval) does NOT trigger PersistEventAndSnapshot", func(t *testing.T) {
		// given
		// interval=5, seqNr=6 → ShouldSnapshot returns false
		store := newMockStore()
		factory := &testAggregateFactory{}
		serializer := NewJSONSerializer()
		config := EventStoreConfig{SnapshotInterval: 5}
		id := NewAggregateId("Test", "no-snap-after")
		agg := &testAgg{ID: id, SeqNr: 6, Version: 0, Name: "test"}
		event := NewEvent("e6", "Event", id, nil, WithSeqNr(6))
		sut := NewRepository[*testAgg](store, factory, serializer, config)

		// when
		if err := sut.Save(ctx, agg, []Event{event}); err != nil {
			t.Fatalf("Save() error: %v", err)
		}

		// then
		// no snapshot
		if store.snapshotCreated {
			t.Error("Save() with seqNr=6 and interval=5 must NOT call PersistEventAndSnapshot")
		}
	})
}

// Test types and mocks

type testAgg struct {
	ID      AggregateID
	SeqNr   uint64
	Version uint64
	Name    string
}

// testAggJSON is a JSON-serializable representation of testAgg.
type testAggJSON struct {
	TypeName string `json:"typeName"`
	Value    string `json:"value"`
	SeqNr    uint64 `json:"seqNr"`
	Version  uint64 `json:"version"`
	Name     string `json:"name"`
}

func (a *testAgg) MarshalJSON() ([]byte, error) {
	return json.Marshal(testAggJSON{
		TypeName: a.ID.GetTypeName(),
		Value:    a.ID.GetValue(),
		SeqNr:    a.SeqNr,
		Version:  a.Version,
		Name:     a.Name,
	})
}

func (a *testAgg) UnmarshalJSON(data []byte) error {
	var j testAggJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	a.ID = NewAggregateId(j.TypeName, j.Value)
	a.SeqNr = j.SeqNr
	a.Version = j.Version
	a.Name = j.Name
	return nil
}

func (a *testAgg) GetId() AggregateID { return a.ID }
func (a *testAgg) GetSeqNr() uint64   { return a.SeqNr }
func (a *testAgg) GetVersion() uint64 { return a.Version }
func (a *testAgg) WithVersion(v uint64) Aggregate {
	return &testAgg{ID: a.ID, SeqNr: a.SeqNr, Version: v, Name: a.Name}
}
func (a *testAgg) WithSeqNr(s uint64) Aggregate {
	return &testAgg{ID: a.ID, SeqNr: s, Version: a.Version, Name: a.Name}
}

type testAggregateFactory struct{}

func (f *testAggregateFactory) Create(id AggregateID) *testAgg {
	return &testAgg{ID: id}
}

func (f *testAggregateFactory) Apply(agg *testAgg, event Event) (*testAgg, error) {
	var payload struct {
		Name string `json:"name"`
	}
	if len(event.GetPayload()) > 0 {
		_ = json.Unmarshal(event.GetPayload(), &payload)
	}
	return &testAgg{
		ID:      agg.ID,
		SeqNr:   event.GetSeqNr(),
		Version: agg.Version,
		Name:    payload.Name,
	}, nil
}

func (f *testAggregateFactory) Serialize(agg *testAgg) ([]byte, error) {
	return json.Marshal(agg)
}

func (f *testAggregateFactory) Deserialize(data []byte) (*testAgg, error) {
	var agg testAgg
	if err := json.Unmarshal(data, &agg); err != nil {
		return nil, err
	}
	return &agg, nil
}

type mockStore struct {
	events           map[string][]Event
	snapshots        map[string]*SnapshotData
	persistedEvents  []Event
	snapshotCreated  bool
	eventSeqNrFilter uint64
}

func newMockStore() *mockStore {
	return &mockStore{
		events:    make(map[string][]Event),
		snapshots: make(map[string]*SnapshotData),
	}
}

func (s *mockStore) GetLatestSnapshotByID(_ context.Context, id AggregateID) (*SnapshotData, error) {
	return s.snapshots[id.AsString()], nil
}

func (s *mockStore) GetEventsByIDSinceSeqNr(_ context.Context, id AggregateID, seqNr uint64) ([]Event, error) {
	allEvents := s.events[id.AsString()]
	var result []Event
	for _, e := range allEvents {
		if e.GetSeqNr() > seqNr {
			result = append(result, e)
		}
	}
	return result, nil
}

func (s *mockStore) PersistEvent(_ context.Context, event Event, _ uint64) error {
	s.persistedEvents = append(s.persistedEvents, event)
	return nil
}

func (s *mockStore) PersistEventAndSnapshot(_ context.Context, event Event, _ Aggregate) error {
	s.persistedEvents = append(s.persistedEvents, event)
	s.snapshotCreated = true
	return nil
}
