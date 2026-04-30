package eventstore_test

import (
	"context"
	"errors"
	"testing"

	es "github.com/Hiroshi0900/eventstore"
	"github.com/Hiroshi0900/eventstore/memory"
)

// === counter ドメイン (テスト用、pure struct + typed AggregateID) ===

// counterID is a typed AggregateID。library から default 実装は提供されないので
// テスト側で typed な値型として定義する。
type counterID struct {
	value string
}

func (c counterID) TypeName() string { return "Counter" }
func (c counterID) Value() string    { return c.value }
func (c counterID) AsString() string { return "Counter-" + c.value }

// counterEvent: domain Event interface (E = counterEvent in Repository[A,C,E])。
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

type counterCommand interface {
	es.Command
	isCounterCommand()
}

type incrementCommand struct {
	By int
}

func (incrementCommand) CommandTypeName() string { return "Increment" }
func (incrementCommand) isCounterCommand()       {}

// counterAggregate: pure struct, no embedded boilerplate, no SeqNr/Version。
// 全メタは library の StoredEvent / StoredSnapshot に集約。
type counterAggregate struct {
	id    counterID
	count int
}

func (c counterAggregate) AggregateID() es.AggregateID { return c.id }

func (c counterAggregate) ApplyCommand(cmd counterCommand) (counterEvent, error) {
	switch x := cmd.(type) {
	case incrementCommand:
		return incrementedEvent{AggID: c.id, By: x.By}, nil
	default:
		return nil, es.ErrUnknownCommand
	}
}

func (c counterAggregate) ApplyEvent(ev counterEvent) es.Aggregate[counterCommand, counterEvent] {
	if e, ok := ev.(incrementedEvent); ok {
		return counterAggregate{id: c.id, count: c.count + e.By}
	}
	return c
}

// blankCounter is the createBlank function passed to NewRepository.
func blankCounter(id es.AggregateID) counterAggregate {
	if cid, ok := id.(counterID); ok {
		return counterAggregate{id: cid}
	}
	return counterAggregate{id: counterID{value: id.Value()}}
}

// helper: build a Repository with a fresh memory store.
// memory store は (de)serialize しないので serializer 不要。
func newCounterRepo(t *testing.T, cfg es.Config) (es.Repository[counterAggregate, counterCommand, counterEvent], es.EventStore[counterAggregate, counterCommand, counterEvent]) {
	t.Helper()
	store := memory.New[counterAggregate, counterCommand, counterEvent]()
	repo := es.NewRepository[counterAggregate, counterCommand, counterEvent](store, blankCounter, cfg)
	return repo, store
}

// loadStreamMutatingStore alters non-empty reconstruction reads so the caller's
// aggregate state changes only if the returned stream is actually applied.
type loadStreamMutatingStore[A es.Aggregate[C, E], C es.Command, E es.Event] struct {
	es.EventStore[A, C, E]
}

func newLoadStreamMutatingStore[A es.Aggregate[C, E], C es.Command, E es.Event](
	base es.EventStore[A, C, E],
) *loadStreamMutatingStore[A, C, E] {
	return &loadStreamMutatingStore[A, C, E]{
		EventStore: base,
	}
}

func (s *loadStreamMutatingStore[A, C, E]) PersistEvent(
	ctx context.Context,
	ev es.StoredEvent[E],
	expectedVersion uint64,
) error {
	return s.EventStore.PersistEvent(ctx, ev, expectedVersion)
}

func (s *loadStreamMutatingStore[A, C, E]) LoadStreamAfter(
	ctx context.Context,
	id es.AggregateID,
	seqNr uint64,
) ([]es.StoredEvent[E], error) {
	events, err := s.EventStore.LoadStreamAfter(ctx, id, seqNr)
	if err != nil {
		return nil, err
	}
	if len(events) > 0 {
		altered := append([]es.StoredEvent[E](nil), events...)
		altered = append(altered, events[len(events)-1])
		return altered, nil
	}
	return events, nil
}

func TestRepository_Save_backToBackUsesStoreReconstructionContract(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100

	base := memory.New[counterAggregate, counterCommand, counterEvent]()
	store := newLoadStreamMutatingStore[counterAggregate, counterCommand, counterEvent](base)
	repo := es.NewRepository[counterAggregate, counterCommand, counterEvent](store, blankCounter, cfg)
	id := counterID{value: "contract"}

	if _, err := repo.Save(context.Background(), id, incrementCommand{By: 1}); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	got, err := repo.Save(context.Background(), id, incrementCommand{By: 1})
	if err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if got.count != 3 {
		t.Fatalf("count: got %d, want 3", got.count)
	}
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

	stored, err := store.LoadStreamAfter(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("LoadStreamAfter: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("len: got %d, want 1", len(stored))
	}
	if stored[0].SeqNr != 1 || !stored[0].IsCreated {
		t.Errorf("metadata mismatch: %+v", stored[0])
	}
	if stored[0].EventID == "" {
		t.Errorf("EventID empty")
	}
	if stored[0].Event.EventTypeName() != "Incremented" {
		t.Errorf("EventTypeName: got %q", stored[0].Event.EventTypeName())
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

	stored, _ := store.LoadStreamAfter(context.Background(), id, 0)
	if len(stored) != 2 || stored[1].IsCreated || stored[1].SeqNr != 2 {
		t.Errorf("metadata mismatch: %+v", stored[1])
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

	snap, found, err := store.GetLatestSnapshot(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshot: %v", err)
	}
	if !found {
		t.Fatal("snapshot not found, want present")
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
type lockFailingStore[A es.Aggregate[C, E], C es.Command, E es.Event] struct {
	es.EventStore[A, C, E]
}

func (s *lockFailingStore[A, C, E]) PersistEventAndSnapshot(
	_ context.Context,
	ev es.StoredEvent[E],
	_ es.StoredSnapshot[A],
) error {
	return es.NewOptimisticLockError(ev.Event.AggregateID().AsString(), 1, 99)
}

func TestRepository_Save_propagatesOptimisticLockError(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 1 // 必ず snapshot を取らせる
	store := &lockFailingStore[counterAggregate, counterCommand, counterEvent]{
		EventStore: memory.New[counterAggregate, counterCommand, counterEvent](),
	}
	repo := es.NewRepository[counterAggregate, counterCommand, counterEvent](store, blankCounter, cfg)

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
func (unknownCmd) isCounterCommand()       {}

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
