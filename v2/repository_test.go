package eventstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/memory"
)

// === counter ドメイン (テスト用、pure struct + typed AggregateID) ===

// counterID is a typed AggregateID. library から default 実装は提供されないので
// テスト側で typed な値型として定義する。
type counterID struct {
	value string
}

func (c counterID) TypeName() string { return "Counter" }
func (c counterID) Value() string    { return c.value }
func (c counterID) AsString() string { return "Counter-" + c.value }

// counterEvent: domain Event interface (E = counterEvent in Repository[T,E])。
type counterEvent interface {
	es.Event
	isCounterEvent()
}

type incrementedEvent struct {
	AggID counterID
	By    int
}

func (e incrementedEvent) EventTypeName() string       { return "Incremented" }
func (e incrementedEvent) AggregateID() es.AggregateID { return e.AggID }
func (incrementedEvent) isCounterEvent()               {}

type incrementCommand struct {
	By int
}

func (incrementCommand) CommandTypeName() string { return "Increment" }

// counterAggregate: pure struct, no embedded boilerplate, no SeqNr/Version。
// 全メタは library の Envelope に集約。
type counterAggregate struct {
	id    counterID
	count int
}

func (c counterAggregate) AggregateID() es.AggregateID { return c.id }

func (c counterAggregate) ApplyCommand(cmd es.Command) (counterEvent, error) {
	switch x := cmd.(type) {
	case incrementCommand:
		return incrementedEvent{AggID: c.id, By: x.By}, nil
	default:
		return nil, es.ErrUnknownCommand
	}
}

func (c counterAggregate) ApplyEvent(ev counterEvent) es.Aggregate[counterEvent] {
	if e, ok := ev.(incrementedEvent); ok {
		return counterAggregate{id: c.id, count: c.count + e.By}
	}
	return c
}

// === Serializers ===

type counterAggregateState struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type counterAggregateSerializer struct{}

func (counterAggregateSerializer) Serialize(c counterAggregate) ([]byte, error) {
	return json.Marshal(counterAggregateState{Value: c.id.value, Count: c.count})
}

func (counterAggregateSerializer) Deserialize(data []byte) (counterAggregate, error) {
	var s counterAggregateState
	if err := json.Unmarshal(data, &s); err != nil {
		return counterAggregate{}, es.NewDeserializationError("aggregate", err)
	}
	return counterAggregate{id: counterID{value: s.Value}, count: s.Count}, nil
}

type incrementedEventWire struct {
	IDValue string `json:"id_value"`
	By      int    `json:"by"`
}

type counterEventSerializer struct{}

func (counterEventSerializer) Serialize(ev counterEvent) ([]byte, error) {
	switch e := ev.(type) {
	case incrementedEvent:
		return json.Marshal(incrementedEventWire{IDValue: e.AggID.value, By: e.By})
	default:
		return nil, es.NewSerializationError("event", errors.New("unknown event type"))
	}
}

func (counterEventSerializer) Deserialize(typeName string, data []byte) (counterEvent, error) {
	switch typeName {
	case "Incremented":
		var w incrementedEventWire
		if err := json.Unmarshal(data, &w); err != nil {
			return nil, es.NewDeserializationError("event", err)
		}
		return incrementedEvent{AggID: counterID{value: w.IDValue}, By: w.By}, nil
	default:
		return nil, es.NewDeserializationError("event", errors.New("unknown event type: "+typeName))
	}
}

// blankCounter is the createBlank function passed to NewRepository.
func blankCounter(id es.AggregateID) counterAggregate {
	if cid, ok := id.(counterID); ok {
		return counterAggregate{id: cid, count: 0}
	}
	// AggregateID が typed counterID でなくても (envelope deserialize で
	// simpleAggregateID として復元される場合) 同じ TypeName/Value で counterID を再構築。
	return counterAggregate{id: counterID{value: id.Value()}, count: 0}
}

// helper: build a Repository with a fresh memory store.
func newCounterRepo(t *testing.T, cfg es.Config) (es.Repository[counterAggregate, counterEvent], es.EventStore) {
	t.Helper()
	store := memory.New()
	repo := es.NewRepository[counterAggregate, counterEvent](
		store,
		blankCounter,
		counterAggregateSerializer{},
		counterEventSerializer{},
		cfg,
	)
	return repo, store
}

func TestRepository_Construct(t *testing.T) {
	repo, _ := newCounterRepo(t, es.DefaultConfig())
	if repo == nil {
		t.Fatal("expected non-nil Repository")
	}
}

func TestRepository_Load_notFound(t *testing.T) {
	repo, _ := newCounterRepo(t, es.DefaultConfig())
	_, err := repo.Load(context.Background(), counterID{value: "missing"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, es.ErrAggregateNotFound) {
		t.Errorf("got %v, want ErrAggregateNotFound", err)
	}
}

func TestRepository_Save_firstEventCreatesAggregate(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100 // snapshot 回避
	repo, store := newCounterRepo(t, cfg)
	id := counterID{value: "1"}

	got, err := repo.Save(context.Background(), id, incrementCommand{By: 3})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got.count != 3 {
		t.Errorf("count: got %d, want 3", got.count)
	}

	envs, err := store.GetEventsSince(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsSince: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("len: got %d, want 1", len(envs))
	}
	if envs[0].SeqNr != 1 || !envs[0].IsCreated {
		t.Errorf("envelope mismatch: %+v", envs[0])
	}
	if envs[0].EventID == "" {
		t.Errorf("EventID empty")
	}
	if envs[0].EventTypeName != "Incremented" {
		t.Errorf("EventTypeName: got %q", envs[0].EventTypeName)
	}
}

func TestRepository_Save_subsequentEvent(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100
	repo, store := newCounterRepo(t, cfg)
	id := counterID{value: "2"}

	if _, err := repo.Save(context.Background(), id, incrementCommand{By: 1}); err != nil {
		t.Fatalf("first: %v", err)
	}
	got, err := repo.Save(context.Background(), id, incrementCommand{By: 4})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if got.count != 5 {
		t.Errorf("count: got %d, want 5", got.count)
	}

	envs, _ := store.GetEventsSince(context.Background(), id, 0)
	if len(envs) != 2 || envs[1].IsCreated || envs[1].SeqNr != 2 {
		t.Errorf("envelope[1] mismatch: %+v", envs[1])
	}
}

func TestRepository_LoadAfterSave_replaysCorrectly(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100
	repo, _ := newCounterRepo(t, cfg)
	id := counterID{value: "3"}

	if _, err := repo.Save(context.Background(), id, incrementCommand{By: 7}); err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	if _, err := repo.Save(context.Background(), id, incrementCommand{By: 3}); err != nil {
		t.Fatalf("Save 2: %v", err)
	}

	got, err := repo.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.count != 10 {
		t.Errorf("count: got %d, want 10", got.count)
	}
}

func TestRepository_Save_triggersSnapshot(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 3
	repo, store := newCounterRepo(t, cfg)
	id := counterID{value: "snap"}

	for i := 0; i < 3; i++ {
		if _, err := repo.Save(context.Background(), id, incrementCommand{By: 1}); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	snap, err := store.GetLatestSnapshot(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshot: %v", err)
	}
	if snap == nil {
		t.Fatal("snapshot nil, want present")
	}
	if snap.SeqNr != 3 || snap.Version != 1 {
		t.Errorf("snapshot: got %+v, want SeqNr=3 Version=1", snap)
	}
}

func TestRepository_LoadAfterSnapshot_usesSnapshot(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 2
	repo, _ := newCounterRepo(t, cfg)
	id := counterID{value: "snapload"}

	for i := 0; i < 4; i++ {
		if _, err := repo.Save(context.Background(), id, incrementCommand{By: 2}); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	got, err := repo.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.count != 8 {
		t.Errorf("count: got %d, want 8", got.count)
	}
}

// lockFailingStore wraps an EventStore and forces ErrOptimisticLock from
// PersistEventAndSnapshot. Used to verify Repository.Save propagates
// store-level optimistic lock errors transparently.
type lockFailingStore struct {
	es.EventStore
}

func (s *lockFailingStore) PersistEventAndSnapshot(
	_ context.Context,
	ev *es.EventEnvelope,
	_ *es.SnapshotEnvelope,
) error {
	return es.NewOptimisticLockError(ev.AggregateID.AsString(), 1, 99)
}

func TestRepository_Save_propagatesOptimisticLockError(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 1 // 必ず snapshot を取らせる
	store := &lockFailingStore{EventStore: memory.New()}
	repo := es.NewRepository[counterAggregate, counterEvent](
		store,
		blankCounter,
		counterAggregateSerializer{},
		counterEventSerializer{},
		cfg,
	)
	_, err := repo.Save(context.Background(), counterID{value: "lock"}, incrementCommand{By: 1})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, es.ErrOptimisticLock) {
		t.Errorf("got %v, want ErrOptimisticLock", err)
	}
}

type unknownCmd struct{}

func (unknownCmd) CommandTypeName() string { return "Unknown" }

func TestRepository_Save_propagatesApplyCommandError(t *testing.T) {
	repo, _ := newCounterRepo(t, es.DefaultConfig())
	_, err := repo.Save(context.Background(), counterID{value: "err"}, unknownCmd{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, es.ErrUnknownCommand) {
		t.Errorf("got %v, want ErrUnknownCommand", err)
	}
}
