package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	es "github.com/Hiroshi0900/eventstore"
	pg "github.com/Hiroshi0900/eventstore/postgres"
)

// === stub domain types (generic instantiation 用) ===

type testAggID struct{ value string }

func (a testAggID) TypeName() string { return "T" }
func (a testAggID) Value() string    { return a.value }
func (a testAggID) AsString() string { return "T-" + a.value }

type testEvent struct {
	aggID testAggID
	Name  string
}

func (e testEvent) EventTypeName() string       { return "TestEvent" }
func (e testEvent) AggregateID() es.AggregateID { return e.aggID }

type testCommand struct{}

func (testCommand) CommandTypeName() string { return "TestCommand" }

type testAggregate struct {
	id   testAggID
	Data string
}

func (a testAggregate) AggregateID() es.AggregateID { return a.id }
func (a testAggregate) ApplyCommand(testCommand) (testEvent, error) {
	return testEvent{aggID: a.id}, nil
}
func (a testAggregate) ApplyEvent(testEvent) es.Aggregate[testCommand, testEvent] {
	return a
}

// === JSON serializers ===

type aggSerializer struct{}

func (aggSerializer) Serialize(a testAggregate) ([]byte, error) {
	return json.Marshal(map[string]string{"id": a.id.value, "data": a.Data})
}
func (aggSerializer) Deserialize(b []byte) (testAggregate, error) {
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return testAggregate{}, err
	}
	return testAggregate{id: testAggID{value: m["id"]}, Data: m["data"]}, nil
}

type evSerializer struct{}

func (evSerializer) Serialize(e testEvent) ([]byte, error) {
	return json.Marshal(map[string]string{"id": e.aggID.value, "name": e.Name})
}
func (evSerializer) Deserialize(_ string, b []byte) (testEvent, error) {
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return testEvent{}, err
	}
	return testEvent{aggID: testAggID{value: m["id"]}, Name: m["name"]}, nil
}

// === test harness ===

type pgStore = interface {
	es.EventStore[testAggregate, testCommand, testEvent]
	pg.SchemaManager
}

// newTestStore は EVENTSTORE_TEST_POSTGRES_URL が無ければ skip する。
// テーブルを作成し、毎回 TRUNCATE して独立性を確保する。
func newTestStore(t *testing.T) (pgStore, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("EVENTSTORE_TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("EVENTSTORE_TEST_POSTGRES_URL not set; skipping postgres integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	store := pg.NewWithSchema[testAggregate, testCommand, testEvent](
		pool, pg.DefaultConfig(), aggSerializer{}, evSerializer{},
	)
	if err := store.CreateTables(ctx); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE event_journal, event_snapshot"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return store, pool
}

func storedEvent(id string, seqNr uint64, name string) es.StoredEvent[testEvent] {
	return es.StoredEvent[testEvent]{
		Event:      testEvent{aggID: testAggID{value: id}, Name: name},
		EventID:    "ev-" + name,
		SeqNr:      seqNr,
		IsCreated:  seqNr == 1,
		OccurredAt: time.Now().UTC().Truncate(time.Millisecond),
	}
}

func storedSnapshot(id string, seqNr, version uint64, data string) es.StoredSnapshot[testAggregate] {
	return es.StoredSnapshot[testAggregate]{
		Aggregate:  testAggregate{id: testAggID{value: id}, Data: data},
		SeqNr:      seqNr,
		Version:    version,
		OccurredAt: time.Now().UTC().Truncate(time.Millisecond),
	}
}

// === tests ===

func TestPersistEventAndSnapshot_thenGetLatestSnapshot(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	ev := storedEvent("a1", 1, "created")
	snap := storedSnapshot("a1", 1, 1, "hello")
	if err := store.PersistEventAndSnapshot(ctx, ev, snap); err != nil {
		t.Fatalf("persist: %v", err)
	}

	got, found, err := store.GetLatestSnapshot(ctx, testAggID{value: "a1"})
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if !found {
		t.Fatal("expected snapshot found")
	}
	if got.Aggregate.Data != "hello" || got.SeqNr != 1 || got.Version != 1 {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
}

func TestGetLatestSnapshot_none(t *testing.T) {
	store, _ := newTestStore(t)
	_, found, err := store.GetLatestSnapshot(context.Background(), testAggID{value: "missing"})
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if found {
		t.Fatal("expected not found")
	}
}

func TestPersistEvent_thenGetEventsSince(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	for i := uint64(1); i <= 3; i++ {
		if err := store.PersistEvent(ctx, storedEvent("a1", i, "e"+string(rune('0'+i))), i); err != nil {
			t.Fatalf("persist event %d: %v", i, err)
		}
	}

	evs, err := store.GetEventsSince(ctx, testAggID{value: "a1"}, 1)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("expected 2 events (seq>1), got %d", len(evs))
	}
	if evs[0].SeqNr != 2 || evs[1].SeqNr != 3 {
		t.Fatalf("expected ascending seq 2,3 got %d,%d", evs[0].SeqNr, evs[1].SeqNr)
	}
	if evs[0].Event.Name == "" || evs[0].EventID == "" {
		t.Fatalf("event payload/metadata lost: %+v", evs[0])
	}
}

func TestGetEventsSince_sentinelReturnsNil(t *testing.T) {
	store, _ := newTestStore(t)
	evs, err := store.GetEventsSince(context.Background(), testAggID{value: "a1"}, ^uint64(0))
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if evs != nil {
		t.Fatalf("expected nil for sentinel seqNr, got %v", evs)
	}
}

func TestPersistEvent_duplicateRejected(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.PersistEvent(ctx, storedEvent("a1", 1, "e1"), 1); err != nil {
		t.Fatalf("first persist: %v", err)
	}
	err := store.PersistEvent(ctx, storedEvent("a1", 1, "dup"), 1)
	if !errors.Is(err, es.ErrDuplicateAggregate) {
		t.Fatalf("expected ErrDuplicateAggregate, got %v", err)
	}
}

func TestPersistEventAndSnapshot_initialTwice_optimisticLock(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.PersistEventAndSnapshot(ctx, storedEvent("a1", 1, "c"), storedSnapshot("a1", 1, 1, "v1")); err != nil {
		t.Fatalf("first persist: %v", err)
	}
	// version=1 (初回作成扱い) を再実行 → 既存 snapshot ありで衝突。
	err := store.PersistEventAndSnapshot(ctx, storedEvent("a1", 2, "c2"), storedSnapshot("a1", 2, 1, "v1again"))
	if !errors.Is(err, es.ErrOptimisticLock) {
		t.Fatalf("expected ErrOptimisticLock, got %v", err)
	}
}

func TestPersistEventAndSnapshot_versionMismatch_optimisticLock(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.PersistEventAndSnapshot(ctx, storedEvent("a1", 1, "c"), storedSnapshot("a1", 1, 1, "v1")); err != nil {
		t.Fatalf("first persist: %v", err)
	}
	// 現行 version=1。expected=version-1=2 になる version=3 を投げる → mismatch。
	err := store.PersistEventAndSnapshot(ctx, storedEvent("a1", 2, "c2"), storedSnapshot("a1", 2, 3, "v3"))
	if !errors.Is(err, es.ErrOptimisticLock) {
		t.Fatalf("expected ErrOptimisticLock, got %v", err)
	}
}

func TestPersistEventAndSnapshot_concurrentSameVersion_exactlyOneWins(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	// 初期 version=1 を作成。
	if err := store.PersistEventAndSnapshot(ctx, storedEvent("a1", 1, "c"), storedSnapshot("a1", 1, 1, "v1")); err != nil {
		t.Fatalf("init: %v", err)
	}

	// 2 goroutine が同じ expected(=1) で version=2 を競合更新する。
	const n = 2
	results := make(chan error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func(i int) {
			<-start
			results <- store.PersistEventAndSnapshot(
				ctx,
				storedEvent("a1", 2, "u"),
				storedSnapshot("a1", 2, 2, "v2"),
			)
		}(i)
	}
	close(start)

	var success, lockErr int
	for i := 0; i < n; i++ {
		err := <-results
		switch {
		case err == nil:
			success++
		case errors.Is(err, es.ErrOptimisticLock):
			lockErr++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if success != 1 || lockErr != 1 {
		t.Fatalf("expected exactly 1 success and 1 optimistic-lock, got success=%d lock=%d", success, lockErr)
	}
}

func TestPersistEventAndSnapshot_sequentialVersions(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.PersistEventAndSnapshot(ctx, storedEvent("a1", 1, "c"), storedSnapshot("a1", 1, 1, "v1")); err != nil {
		t.Fatalf("v1: %v", err)
	}
	// version=2 は expected=1 と現行一致 → 成功。
	if err := store.PersistEventAndSnapshot(ctx, storedEvent("a1", 2, "u"), storedSnapshot("a1", 2, 2, "v2")); err != nil {
		t.Fatalf("v2: %v", err)
	}
	got, found, err := store.GetLatestSnapshot(ctx, testAggID{value: "a1"})
	if err != nil || !found {
		t.Fatalf("get: err=%v found=%v", err, found)
	}
	if got.Version != 2 || got.Aggregate.Data != "v2" {
		t.Fatalf("expected version2/v2, got %+v", got)
	}
}
