package eventstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/memory"
)

// --- テスト用ミニ集約: 単純なカウンタ ---

type counter struct {
	id      es.AggregateID
	value   int
	seqNr   uint64
	version uint64
}

func newCounter(id es.AggregateID) counter { return counter{id: id} }

func (c counter) AggregateID() es.AggregateID       { return c.id }
func (c counter) SeqNr() uint64                     { return c.seqNr }
func (c counter) Version() uint64                   { return c.version }
func (c counter) WithVersion(v uint64) es.Aggregate { c.version = v; return c }
func (c counter) WithSeqNr(s uint64) es.Aggregate   { c.seqNr = s; return c }

func (c counter) ApplyCommand(cmd es.Command) (es.Event, error) {
	switch cmd.(type) {
	case incrementCmd:
		return es.NewEvent("evt-"+cmd.AggregateID().AsString(), "Incremented", cmd.AggregateID(), nil,
			es.WithSeqNr(c.seqNr+1), es.WithIsCreated(c.seqNr == 0)), nil
	default:
		return nil, es.ErrUnknownCommand
	}
}

func (c counter) ApplyEvent(ev es.Event) es.Aggregate {
	switch ev.EventTypeName() {
	case "Incremented":
		c.value++
		c.seqNr = ev.SeqNr()
	}
	return c
}

type incrementCmd struct{ id es.AggregateID }

func (c incrementCmd) CommandTypeName() string     { return "Increment" }
func (c incrementCmd) AggregateID() es.AggregateID { return c.id }

// --- AggregateSerializer ---

type counterSnapshot struct {
	Value   int    `json:"value"`
	SeqNr   uint64 `json:"seq_nr"`
	Version uint64 `json:"version"`
	IDType  string `json:"id_type"`
	IDValue string `json:"id_value"`
}

type counterSerializer struct{}

func (counterSerializer) Serialize(c counter) ([]byte, error) {
	return json.Marshal(counterSnapshot{
		Value: c.value, SeqNr: c.seqNr, Version: c.version,
		IDType: c.id.TypeName(), IDValue: c.id.Value(),
	})
}

func (counterSerializer) Deserialize(data []byte) (counter, error) {
	var s counterSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return counter{}, err
	}
	return counter{
		id:      es.NewAggregateID(s.IDType, s.IDValue),
		value:   s.Value,
		seqNr:   s.SeqNr,
		version: s.Version,
	}, nil
}

// --- 共通ヘルパ ---

func newTestRepo() *es.Repository[counter] {
	return es.NewRepository[counter](
		memory.New(),
		func(id es.AggregateID) counter { return newCounter(id) },
		counterSerializer{},
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
	store := memory.New()
	r := es.NewRepository[counter](
		store,
		func(id es.AggregateID) counter { return newCounter(id) },
		counterSerializer{},
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

	snap, err := store.GetLatestSnapshotByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshotByID err: %v", err)
	}
	if snap == nil {
		t.Fatal("expected snapshot to be saved at seqNr=3")
	}
	if snap.SeqNr != 3 {
		t.Errorf("snap.SeqNr = %d, want 3", snap.SeqNr)
	}
	if snap.Version != 1 {
		t.Errorf("snap.Version = %d, want 1", snap.Version)
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
	store := memory.New()
	r := es.NewRepository[counter](
		store,
		func(id es.AggregateID) counter { return newCounter(id) },
		counterSerializer{},
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
// versions マップを「PersistEvent でインクリメント、PersistEventAndSnapshot で上書き」していた
// 以前のバグを catch するためのテスト。
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
