package eventstore_test

import (
	"context"
	"errors"
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/memory"
)

// --- テスト用ミニ集約: 単純なカウンタ ---
// BaseAggregate を埋め込むパターンで、AggregateID/SeqNr/Version の boilerplate を省略。

type counter struct {
	es.BaseAggregate
	value int
}

func newCounter(id es.AggregateID) counter {
	return counter{BaseAggregate: es.NewBaseAggregate(id, 0, 0)}
}

// Aggregate interface の WithSeqNr / WithVersion は戻り値が Aggregate なので embed 元で override。
func (c counter) WithSeqNr(s uint64) es.Aggregate {
	c.BaseAggregate = c.BaseAggregate.WithSeqNr(s)
	return c
}
func (c counter) WithVersion(v uint64) es.Aggregate {
	c.BaseAggregate = c.BaseAggregate.WithVersion(v)
	return c
}

func (c counter) ApplyCommand(cmd es.Command) (es.Event, error) {
	switch cmd.(type) {
	case incrementCmd:
		return es.NewEvent("evt-"+cmd.AggregateID().AsString(), "Incremented", cmd.AggregateID(), nil,
			es.WithSeqNr(c.SeqNr()+1), es.WithIsCreated(c.SeqNr() == 0)), nil
	default:
		return nil, es.ErrUnknownCommand
	}
}

func (c counter) ApplyEvent(ev es.Event) es.Aggregate {
	switch ev.EventTypeName() {
	case "Incremented":
		c.value++
		c.BaseAggregate = c.BaseAggregate.WithSeqNr(ev.SeqNr())
	}
	return c
}

type incrementCmd struct{ id es.AggregateID }

func (c incrementCmd) CommandTypeName() string     { return "Increment" }
func (c incrementCmd) AggregateID() es.AggregateID { return c.id }

// --- 共通ヘルパ ---

func newTestRepo() *es.Repository[counter] {
	return es.NewRepository[counter](
		memory.New[counter](),
		func(id es.AggregateID) counter { return newCounter(id) },
		es.DefaultConfig(),
	)
}

func TestRepository_Construct(t *testing.T) {
	r := newTestRepo()
	if r == nil {
		t.Fatal("nil repo")
	}
	_ = context.Background
	_ = errors.Is
}

func TestRepository_Load_NotFound(t *testing.T) {
	r := newTestRepo()
	id := es.NewAggregateID("Counter", "c1")

	_, err := r.Load(context.Background(), id)
	if !errors.Is(err, es.ErrAggregateNotFound) {
		t.Errorf("expected ErrAggregateNotFound, got %v", err)
	}
}

func TestRepository_Store_FirstCommand(t *testing.T) {
	r := newTestRepo()
	id := es.NewAggregateID("Counter", "c1")
	c := newCounter(id)

	updated, err := r.Store(context.Background(), incrementCmd{id: id}, c)
	if err != nil {
		t.Fatalf("Store err: %v", err)
	}
	if got, want := updated.value, 1; got != want {
		t.Errorf("counter.value = %d, want %d", got, want)
	}
	if got, want := updated.SeqNr(), uint64(1); got != want {
		t.Errorf("counter.SeqNr = %d, want %d", got, want)
	}
}

func TestRepository_Store_MismatchedAggregateID(t *testing.T) {
	r := newTestRepo()
	idA := es.NewAggregateID("Counter", "a")
	idB := es.NewAggregateID("Counter", "b")
	c := newCounter(idA)

	_, err := r.Store(context.Background(), incrementCmd{id: idB}, c)
	if !errors.Is(err, es.ErrAggregateIDMismatch) {
		t.Errorf("expected ErrAggregateIDMismatch, got %v", err)
	}
}

func TestRepository_LoadAfterStore(t *testing.T) {
	r := newTestRepo()
	id := es.NewAggregateID("Counter", "c1")
	c := newCounter(id)

	for i := 0; i < 3; i++ {
		var err error
		c, err = r.Store(context.Background(), incrementCmd{id: id}, c)
		if err != nil {
			t.Fatalf("Store err at i=%d: %v", i, err)
		}
	}

	loaded, err := r.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load err: %v", err)
	}
	if got, want := loaded.value, 3; got != want {
		t.Errorf("loaded.value = %d, want %d", got, want)
	}
	if got, want := loaded.SeqNr(), uint64(3); got != want {
		t.Errorf("loaded.SeqNr = %d, want %d", got, want)
	}
}

func TestRepository_Store_TakesSnapshot(t *testing.T) {
	store := memory.New[counter]()
	r := es.NewRepository[counter](
		store,
		func(id es.AggregateID) counter { return newCounter(id) },
		es.Config{SnapshotInterval: 3},
	)
	id := es.NewAggregateID("Counter", "c1")
	c := newCounter(id)

	for i := 0; i < 3; i++ {
		var err error
		c, err = r.Store(context.Background(), incrementCmd{id: id}, c)
		if err != nil {
			t.Fatalf("Store err at i=%d: %v", i, err)
		}
	}

	snap, found, err := store.GetLatestSnapshotByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshotByID err: %v", err)
	}
	if !found {
		t.Fatal("expected snapshot to be saved at seqNr=3")
	}
	if snap.SeqNr() != 3 {
		t.Errorf("snap.SeqNr() = %d, want 3", snap.SeqNr())
	}
	if snap.Version() != 1 {
		t.Errorf("snap.Version() = %d, want 1", snap.Version())
	}

	loaded, err := r.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load err: %v", err)
	}
	if loaded.value != 3 {
		t.Errorf("loaded.value = %d, want 3", loaded.value)
	}
}

func TestRepository_Store_OptimisticLockOnSnapshot(t *testing.T) {
	store := memory.New[counter]()
	r := es.NewRepository[counter](
		store,
		func(id es.AggregateID) counter { return newCounter(id) },
		es.Config{SnapshotInterval: 1},
	)
	id := es.NewAggregateID("Counter", "c1")
	c := newCounter(id)

	c, err := r.Store(context.Background(), incrementCmd{id: id}, c)
	if err != nil {
		t.Fatalf("first Store err: %v", err)
	}

	stale := c
	c, err = r.Store(context.Background(), incrementCmd{id: id}, c)
	if err != nil {
		t.Fatalf("second Store err: %v", err)
	}
	_, err = r.Store(context.Background(), incrementCmd{id: id}, stale)
	if !errors.Is(err, es.ErrOptimisticLock) {
		t.Errorf("expected ErrOptimisticLock, got %v", err)
	}
}

// リグレッションテスト: SnapshotInterval=5 で 6 回以上 Store した時、
// snapshot 取得後の PersistEvent でバージョン管理が壊れないこと。
func TestRepository_Store_BeyondSnapshotInterval(t *testing.T) {
	r := newTestRepo() // SnapshotInterval=5 (DefaultConfig)
	id := es.NewAggregateID("Counter", "c1")
	c := newCounter(id)

	// 6 回 Store: 5 回目で snapshot、6 回目は通常の PersistEvent
	for i := 0; i < 6; i++ {
		var err error
		c, err = r.Store(context.Background(), incrementCmd{id: id}, c)
		if err != nil {
			t.Fatalf("Store err at i=%d: %v", i, err)
		}
	}

	if got, want := c.value, 6; got != want {
		t.Errorf("counter.value = %d, want %d", got, want)
	}
	if got, want := c.SeqNr(), uint64(6); got != want {
		t.Errorf("counter.SeqNr = %d, want %d", got, want)
	}

	loaded, err := r.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load err: %v", err)
	}
	if loaded.value != 6 {
		t.Errorf("loaded.value = %d, want 6", loaded.value)
	}
}
