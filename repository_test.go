package eventstore_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
		return counterAggregate{
			id:    c.id,
			count: c.count + e.By,
		}
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

// === replay witness contract fixture (test-only, isolated from counter fixture) ===

type witnessCounterEvent interface {
	es.Event
	isWitnessCounterEvent()
}

type witnessCounterIncrementedEvent struct {
	AggID counterID
	By    int
}

func (e witnessCounterIncrementedEvent) EventTypeName() string       { return "WitnessCounterIncremented" }
func (e witnessCounterIncrementedEvent) AggregateID() es.AggregateID { return e.AggID }
func (witnessCounterIncrementedEvent) isWitnessCounterEvent()        {}

// replayWitnessEvent is a test-only event used to prove replay consumed the
// returned GetEventsSince slice on the existing-aggregate path.
type replayWitnessEvent struct {
	AggID counterID
}

func (e replayWitnessEvent) EventTypeName() string       { return "ReplayWitness" }
func (e replayWitnessEvent) AggregateID() es.AggregateID { return e.AggID }
func (replayWitnessEvent) isWitnessCounterEvent()        {}

type witnessCounterCommand interface {
	es.Command
	isWitnessCounterCommand()
}

type witnessCounterIncrementCommand struct {
	By int
}

func (witnessCounterIncrementCommand) CommandTypeName() string  { return "WitnessCounterIncrement" }
func (witnessCounterIncrementCommand) isWitnessCounterCommand() {}

type witnessCounterAggregate struct {
	id             counterID
	count          int
	witnessApplied bool
}

func (a witnessCounterAggregate) AggregateID() es.AggregateID { return a.id }

func (a witnessCounterAggregate) ApplyCommand(cmd witnessCounterCommand) (witnessCounterEvent, error) {
	switch x := cmd.(type) {
	case witnessCounterIncrementCommand:
		return witnessCounterIncrementedEvent{AggID: a.id, By: x.By}, nil
	default:
		return nil, es.ErrUnknownCommand
	}
}

func (a witnessCounterAggregate) ApplyEvent(ev witnessCounterEvent) es.Aggregate[witnessCounterCommand, witnessCounterEvent] {
	switch e := ev.(type) {
	case witnessCounterIncrementedEvent:
		return witnessCounterAggregate{
			id:             a.id,
			count:          a.count + e.By,
			witnessApplied: a.witnessApplied,
		}
	case replayWitnessEvent:
		return witnessCounterAggregate{
			id:             a.id,
			count:          a.count,
			witnessApplied: true,
		}
	default:
		return a
	}
}

func blankWitnessCounter(id es.AggregateID) witnessCounterAggregate {
	if cid, ok := id.(counterID); ok {
		return witnessCounterAggregate{id: cid}
	}
	return witnessCounterAggregate{id: counterID{value: id.Value()}}
}

type replayWitnessingStore struct {
	es.EventStore[witnessCounterAggregate, witnessCounterCommand, witnessCounterEvent]
}

func newReplayWitnessingStore(
	base es.EventStore[witnessCounterAggregate, witnessCounterCommand, witnessCounterEvent],
) *replayWitnessingStore {
	return &replayWitnessingStore{EventStore: base}
}

func (s *replayWitnessingStore) GetEventsSince(
	ctx context.Context,
	id es.AggregateID,
	seqNr uint64,
) ([]es.StoredEvent[witnessCounterEvent], error) {
	events, err := s.EventStore.GetEventsSince(ctx, id, seqNr)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return events, nil
	}
	altered := append([]es.StoredEvent[witnessCounterEvent](nil), events...)
	return appendReplayWitness(id, altered), nil
}

func appendReplayWitness(
	id es.AggregateID,
	events []es.StoredEvent[witnessCounterEvent],
) []es.StoredEvent[witnessCounterEvent] {
	witnessID, ok := id.(counterID)
	if !ok {
		witnessID = counterID{value: id.Value()}
	}
	last := events[len(events)-1]
	witnessOccurredAt := last.OccurredAt
	if witnessOccurredAt.IsZero() {
		witnessOccurredAt = time.Unix(0, int64(last.SeqNr)).UTC()
	}
	return append(events, es.StoredEvent[witnessCounterEvent]{
		Event:      replayWitnessEvent{AggID: witnessID},
		EventID:    "replay-witness-" + witnessID.Value(),
		SeqNr:      last.SeqNr + 1,
		OccurredAt: witnessOccurredAt.Add(time.Nanosecond),
	})
}

type countingStore struct {
	es.EventStore[counterAggregate, counterCommand, counterEvent]
	loadCalls int
}

func newCountingStore(
	base es.EventStore[counterAggregate, counterCommand, counterEvent],
) *countingStore {
	return &countingStore{EventStore: base}
}

func (s *countingStore) GetEventsSince(
	ctx context.Context,
	id es.AggregateID,
	seqNr uint64,
) ([]es.StoredEvent[counterEvent], error) {
	s.loadCalls++
	return s.EventStore.GetEventsSince(ctx, id, seqNr)
}

// TestRepository_Load_appliesGetEventsSinceResultDuringReplay verifies that
// Load replays all events returned by GetEventsSince onto the aggregate.
func TestRepository_Load_appliesGetEventsSinceResultDuringReplay(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100

	base := memory.New[witnessCounterAggregate, witnessCounterCommand, witnessCounterEvent]()
	store := newReplayWitnessingStore(base)
	repo := es.NewRepository[witnessCounterAggregate, witnessCounterCommand, witnessCounterEvent](store, blankWitnessCounter, cfg)
	id := counterID{value: "contract"}

	seed, err := repo.NewAggregate(context.Background(), id)
	if err != nil {
		t.Fatalf("NewAggregate: %v", err)
	}
	if _, err := repo.Save(context.Background(), seed, witnessCounterIncrementCommand{By: 1}); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	// Load triggers GetEventsSince; witnessing store appends a witness event.
	loaded, err := repo.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := repo.Save(context.Background(), loaded, witnessCounterIncrementCommand{By: 1})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got.Aggregate().count != 2 {
		t.Fatalf("count: got %d, want 2", got.Aggregate().count)
	}
	if !got.Aggregate().witnessApplied {
		t.Fatal("expected replay witness event to be applied during Load")
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

func TestRepository_NewAggregate_thenSave_createsFirstEvent(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100 // snapshot 回避
	repo, store := newCounterRepo(t, cfg)
	id := counterID{value: "1"}

	blank, err := repo.NewAggregate(context.Background(), id)
	if err != nil {
		t.Fatalf("NewAggregate: %v", err)
	}
	got, err := repo.Save(context.Background(), blank, incrementCommand{By: 3})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got.Aggregate().count != 3 {
		t.Errorf("count: got %d, want 3", got.Aggregate().count)
	}

	stored, err := store.GetEventsSince(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsSince: %v", err)
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

func TestRepository_ChainedSave_producesSubsequentEvent(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100
	repo, store := newCounterRepo(t, cfg)
	id := counterID{value: "2"}

	loaded, err := repo.NewAggregate(context.Background(), id)
	if err != nil {
		t.Fatalf("NewAggregate: %v", err)
	}
	loaded, err = repo.Save(context.Background(), loaded, incrementCommand{By: 1})
	if err != nil {
		t.Fatalf("first Save: %v", err)
	}
	got, err := repo.Save(context.Background(), loaded, incrementCommand{By: 4})
	if err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if got.Aggregate().count != 5 {
		t.Errorf("count: got %d, want 5", got.Aggregate().count)
	}

	stored, _ := store.GetEventsSince(context.Background(), id, 0)
	if len(stored) != 2 || stored[1].IsCreated || stored[1].SeqNr != 2 {
		t.Errorf("metadata mismatch: %+v", stored[1])
	}
}

// TestRepository_Save_reusesLoadedContextWithoutReplay verifies that consecutive
// Save calls on the same LoadedAggregate do not re-read from the store.
func TestRepository_Save_reusesLoadedContextWithoutReplay(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100

	base := memory.New[counterAggregate, counterCommand, counterEvent]()
	store := newCountingStore(base)
	repo := es.NewRepository[counterAggregate, counterCommand, counterEvent](store, blankCounter, cfg)
	id := counterID{value: "loaded"}

	seed, err := repo.NewAggregate(context.Background(), id)
	if err != nil {
		t.Fatalf("NewAggregate: %v", err)
	}
	if _, err := repo.Save(context.Background(), seed, incrementCommand{By: 1}); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	loaded, err := repo.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	callsAfterLoad := store.loadCalls

	afterFirst, err := repo.Save(context.Background(), loaded, incrementCommand{By: 2})
	if err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if got := afterFirst.Aggregate().count; got != 3 {
		t.Fatalf("count after first Save: got %d, want 3", got)
	}
	if store.loadCalls != callsAfterLoad {
		t.Fatalf("GetEventsSince calls after first Save: got %d, want %d", store.loadCalls, callsAfterLoad)
	}

	afterSecond, err := repo.Save(context.Background(), afterFirst, incrementCommand{By: 3})
	if err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if got := afterSecond.Aggregate().count; got != 6 {
		t.Fatalf("count after second Save: got %d, want 6", got)
	}
	if store.loadCalls != callsAfterLoad {
		t.Fatalf("GetEventsSince calls after second Save: got %d, want %d", store.loadCalls, callsAfterLoad)
	}
}

func TestRepository_Save_rejectsZeroValueLoadedAggregate(t *testing.T) {
	repo, _ := newCounterRepo(t, es.DefaultConfig())

	_, err := repo.Save(context.Background(), es.LoadedAggregate[counterAggregate, counterCommand, counterEvent]{}, incrementCommand{By: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, es.ErrInvalidAggregate) {
		t.Fatalf("expected ErrInvalidAggregate, got %v", err)
	}
	if got := err.Error(); got != "invalid aggregate: Save requires a loaded aggregate handle created by this repository" {
		t.Fatalf("error message = %q", got)
	}
}

func TestRepository_Save_rejectsHandleFromDifferentRepository(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100

	repo1, _ := newCounterRepo(t, cfg)
	repo2, _ := newCounterRepo(t, cfg)
	id := counterID{value: "foreign"}

	seed, err := repo1.NewAggregate(context.Background(), id)
	if err != nil {
		t.Fatalf("NewAggregate: %v", err)
	}
	if _, err := repo1.Save(context.Background(), seed, incrementCommand{By: 1}); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	loaded, err := repo1.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	_, err = repo2.Save(context.Background(), loaded, incrementCommand{By: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, es.ErrInvalidAggregate) {
		t.Fatalf("expected ErrInvalidAggregate, got %v", err)
	}
	if got := err.Error(); got != "invalid aggregate: Save requires a loaded aggregate handle created by this repository" {
		t.Fatalf("error message = %q", got)
	}
}

func TestRepository_Save_updatesSnapshotVersionAcrossBoundary(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 2

	repo, store := newCounterRepo(t, cfg)
	id := counterID{value: "snapshot"}

	// seqNr=1 (no snapshot)
	loaded, err := repo.NewAggregate(context.Background(), id)
	if err != nil {
		t.Fatalf("NewAggregate: %v", err)
	}
	loaded, err = repo.Save(context.Background(), loaded, incrementCommand{By: 1})
	if err != nil {
		t.Fatalf("first Save: %v", err)
	}

	// seqNr=2 → snapshot taken (version=1)
	loaded, err = repo.Save(context.Background(), loaded, incrementCommand{By: 1})
	if err != nil {
		t.Fatalf("Save at snapshot boundary: %v", err)
	}
	if got := loaded.Aggregate().count; got != 2 {
		t.Fatalf("count after snapshot boundary: got %d, want 2", got)
	}

	snap, found, err := store.GetLatestSnapshot(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshot: %v", err)
	}
	if !found {
		t.Fatal("expected snapshot")
	}
	if snap.Version != 1 {
		t.Fatalf("snapshot version: got %d, want 1", snap.Version)
	}
	if snap.SeqNr != 2 {
		t.Fatalf("snapshot seqNr: got %d, want 2", snap.SeqNr)
	}

	// seqNr=3 (no snapshot)
	loaded, err = repo.Save(context.Background(), loaded, incrementCommand{By: 1})
	if err != nil {
		t.Fatalf("Save seqNr=3: %v", err)
	}
	if got := loaded.Aggregate().count; got != 3 {
		t.Fatalf("count at seqNr=3: got %d, want 3", got)
	}

	// seqNr=4 → snapshot taken (version=2)
	loaded, err = repo.Save(context.Background(), loaded, incrementCommand{By: 1})
	if err != nil {
		t.Fatalf("Save at second snapshot boundary: %v", err)
	}
	if got := loaded.Aggregate().count; got != 4 {
		t.Fatalf("count at seqNr=4: got %d, want 4", got)
	}

	snap, found, err = store.GetLatestSnapshot(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshot after second boundary: %v", err)
	}
	if !found {
		t.Fatal("expected snapshot after second boundary")
	}
	if snap.Version != 2 {
		t.Fatalf("snapshot version after second boundary: got %d, want 2", snap.Version)
	}
	if snap.SeqNr != 4 {
		t.Fatalf("snapshot seqNr after second boundary: got %d, want 4", snap.SeqNr)
	}

	got, err := repo.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load after Save sequence: %v", err)
	}
	if got.Aggregate().count != 4 {
		t.Fatalf("loaded count after Save sequence: got %d, want 4", got.Aggregate().count)
	}
}

type transitionCounterEvent interface {
	es.Event
	isTransitionCounterEvent()
}

type transitionCounterIncrementedEvent struct {
	AggID      counterID
	By         int
	MakeUnsafe bool
}

func (e transitionCounterIncrementedEvent) EventTypeName() string {
	return "TransitionCounterIncremented"
}
func (e transitionCounterIncrementedEvent) AggregateID() es.AggregateID {
	return e.AggID
}
func (transitionCounterIncrementedEvent) isTransitionCounterEvent() {}

type transitionCounterCommand interface {
	es.Command
	isTransitionCounterCommand()
}

type transitionIncrementCommand struct {
	By int
}

func (transitionIncrementCommand) CommandTypeName() string     { return "TransitionIncrement" }
func (transitionIncrementCommand) isTransitionCounterCommand() {}

type transitionMakeUnsafeCommand struct {
	By int
}

func (transitionMakeUnsafeCommand) CommandTypeName() string     { return "TransitionMakeUnsafe" }
func (transitionMakeUnsafeCommand) isTransitionCounterCommand() {}

type transitionCounterAggregate interface {
	es.Aggregate[transitionCounterCommand, transitionCounterEvent]
	isTransitionCounterAggregate()
}

type safeTransitionCounterAggregate struct {
	id    counterID
	count int
}

func (a safeTransitionCounterAggregate) AggregateID() es.AggregateID { return a.id }

func (a safeTransitionCounterAggregate) ApplyCommand(
	cmd transitionCounterCommand,
) (transitionCounterEvent, error) {
	switch x := cmd.(type) {
	case transitionIncrementCommand:
		return transitionCounterIncrementedEvent{AggID: a.id, By: x.By}, nil
	case transitionMakeUnsafeCommand:
		return transitionCounterIncrementedEvent{AggID: a.id, By: x.By, MakeUnsafe: true}, nil
	default:
		return nil, es.ErrUnknownCommand
	}
}

func (a safeTransitionCounterAggregate) ApplyEvent(
	ev transitionCounterEvent,
) es.Aggregate[transitionCounterCommand, transitionCounterEvent] {
	e, ok := ev.(transitionCounterIncrementedEvent)
	if !ok {
		return a
	}
	if e.MakeUnsafe {
		return unsafeTransitionCounterAggregate{
			id:      a.id,
			history: []int{a.count, a.count + e.By},
		}
	}
	return safeTransitionCounterAggregate{
		id:    a.id,
		count: a.count + e.By,
	}
}

func (safeTransitionCounterAggregate) isTransitionCounterAggregate() {}

type unsafeTransitionCounterAggregate struct {
	id      counterID
	history []int
}

func (a unsafeTransitionCounterAggregate) AggregateID() es.AggregateID { return a.id }

func (a unsafeTransitionCounterAggregate) ApplyCommand(
	cmd transitionCounterCommand,
) (transitionCounterEvent, error) {
	switch x := cmd.(type) {
	case transitionIncrementCommand:
		return transitionCounterIncrementedEvent{AggID: a.id, By: x.By}, nil
	case transitionMakeUnsafeCommand:
		return transitionCounterIncrementedEvent{AggID: a.id, By: x.By, MakeUnsafe: true}, nil
	default:
		return nil, es.ErrUnknownCommand
	}
}

func (a unsafeTransitionCounterAggregate) ApplyEvent(
	ev transitionCounterEvent,
) es.Aggregate[transitionCounterCommand, transitionCounterEvent] {
	e, ok := ev.(transitionCounterIncrementedEvent)
	if !ok {
		return a
	}

	history := append([]int(nil), a.history...)
	history = append(history, e.By)
	return unsafeTransitionCounterAggregate{id: a.id, history: history}
}

func (unsafeTransitionCounterAggregate) isTransitionCounterAggregate() {}

func blankTransitionCounter(id es.AggregateID) transitionCounterAggregate {
	if cid, ok := id.(counterID); ok {
		return safeTransitionCounterAggregate{id: cid}
	}
	return safeTransitionCounterAggregate{id: counterID{value: id.Value()}}
}

func TestRepository_Save_rejectsInvalidNextAggregateBeforePersist(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100

	store := memory.New[transitionCounterAggregate, transitionCounterCommand, transitionCounterEvent]()
	repo := es.NewRepository[transitionCounterAggregate, transitionCounterCommand, transitionCounterEvent](store, blankTransitionCounter, cfg)
	id := counterID{value: "transition"}

	loaded, err := repo.NewAggregate(context.Background(), id)
	if err != nil {
		t.Fatalf("NewAggregate: %v", err)
	}
	loaded, err = repo.Save(context.Background(), loaded, transitionIncrementCommand{By: 1})
	if err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	_, err = repo.Save(context.Background(), loaded, transitionMakeUnsafeCommand{By: 2})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, es.ErrInvalidAggregate) {
		t.Fatalf("expected ErrInvalidAggregate, got %v", err)
	}

	stored, err := store.GetEventsSince(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsSince: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored events after failed Save: got %d, want 1", len(stored))
	}

	got, err := repo.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load after failed Save: %v", err)
	}

	safe, ok := got.Aggregate().(safeTransitionCounterAggregate)
	if !ok {
		t.Fatalf("aggregate type after failed Save: got %T, want safeTransitionCounterAggregate", got.Aggregate())
	}
	if safe.count != 1 {
		t.Fatalf("aggregate count after failed Save: got %d, want 1", safe.count)
	}
}

type unsafeCounterEvent interface {
	es.Event
	isUnsafeCounterEvent()
}

type unsafeCounterIncrementedEvent struct {
	AggID counterID
	By    int
}

func (e unsafeCounterIncrementedEvent) EventTypeName() string       { return "UnsafeCounterIncremented" }
func (e unsafeCounterIncrementedEvent) AggregateID() es.AggregateID { return e.AggID }
func (unsafeCounterIncrementedEvent) isUnsafeCounterEvent()         {}

type unsafeCounterCommand interface {
	es.Command
	isUnsafeCounterCommand()
}

type unsafeIncrementCommand struct {
	By int
}

func (unsafeIncrementCommand) CommandTypeName() string { return "UnsafeIncrement" }
func (unsafeIncrementCommand) isUnsafeCounterCommand() {}

type unsafeCounterAggregate struct {
	id      counterID
	history []int
}

func (a unsafeCounterAggregate) AggregateID() es.AggregateID { return a.id }

func (a unsafeCounterAggregate) ApplyCommand(cmd unsafeCounterCommand) (unsafeCounterEvent, error) {
	switch x := cmd.(type) {
	case unsafeIncrementCommand:
		return unsafeCounterIncrementedEvent{AggID: a.id, By: x.By}, nil
	default:
		return nil, es.ErrUnknownCommand
	}
}

func (a unsafeCounterAggregate) ApplyEvent(ev unsafeCounterEvent) es.Aggregate[unsafeCounterCommand, unsafeCounterEvent] {
	if e, ok := ev.(unsafeCounterIncrementedEvent); ok {
		history := append([]int(nil), a.history...)
		history = append(history, e.By)
		return unsafeCounterAggregate{id: a.id, history: history}
	}
	return a
}

func blankUnsafeCounter(id es.AggregateID) unsafeCounterAggregate {
	if cid, ok := id.(counterID); ok {
		return unsafeCounterAggregate{id: cid}
	}
	return unsafeCounterAggregate{id: counterID{value: id.Value()}}
}

// TestRepository_NewAggregate_rejectsReferenceSemanticAggregate verifies that
// NewAggregate rejects aggregate types with reference semantics (slices, maps, etc.).
func TestRepository_NewAggregate_rejectsReferenceSemanticAggregate(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100

	store := memory.New[unsafeCounterAggregate, unsafeCounterCommand, unsafeCounterEvent]()
	repo := es.NewRepository[unsafeCounterAggregate, unsafeCounterCommand, unsafeCounterEvent](store, blankUnsafeCounter, cfg)
	id := counterID{value: "unsafe"}

	_, err := repo.NewAggregate(context.Background(), id)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, es.ErrInvalidAggregate) {
		t.Fatalf("expected ErrInvalidAggregate, got %v", err)
	}
	if !strings.Contains(err.Error(), "value-semantic") {
		t.Fatalf("expected value-semantic message, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "slice") {
		t.Fatalf("expected slice detail in message, got %q", err.Error())
	}
}

type timedCounterEvent interface {
	es.Event
	isTimedCounterEvent()
}

type timedCounterStampedEvent struct {
	AggID counterID
	At    time.Time
}

func (e timedCounterStampedEvent) EventTypeName() string       { return "TimedCounterStamped" }
func (e timedCounterStampedEvent) AggregateID() es.AggregateID { return e.AggID }
func (timedCounterStampedEvent) isTimedCounterEvent()          {}

type timedCounterCommand interface {
	es.Command
	isTimedCounterCommand()
}

type stampTimedCounterCommand struct {
	At time.Time
}

func (stampTimedCounterCommand) CommandTypeName() string { return "StampTimedCounter" }
func (stampTimedCounterCommand) isTimedCounterCommand()  {}

type timedCounterAggregate struct {
	id        counterID
	updatedAt time.Time
	updates   int
}

func (a timedCounterAggregate) AggregateID() es.AggregateID { return a.id }

func (a timedCounterAggregate) ApplyCommand(cmd timedCounterCommand) (timedCounterEvent, error) {
	switch x := cmd.(type) {
	case stampTimedCounterCommand:
		return timedCounterStampedEvent{AggID: a.id, At: x.At}, nil
	default:
		return nil, es.ErrUnknownCommand
	}
}

func (a timedCounterAggregate) ApplyEvent(ev timedCounterEvent) es.Aggregate[timedCounterCommand, timedCounterEvent] {
	e, ok := ev.(timedCounterStampedEvent)
	if !ok {
		return a
	}
	return timedCounterAggregate{
		id:        a.id,
		updatedAt: e.At,
		updates:   a.updates + 1,
	}
}

func blankTimedCounter(id es.AggregateID) timedCounterAggregate {
	if cid, ok := id.(counterID); ok {
		return timedCounterAggregate{id: cid}
	}
	return timedCounterAggregate{id: counterID{value: id.Value()}}
}

// TestRepository_NewAggregate_allowsTimeTimeAggregate verifies that aggregates
// containing time.Time fields are accepted (time.Time is whitelisted).
func TestRepository_NewAggregate_allowsTimeTimeAggregate(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100

	store := memory.New[timedCounterAggregate, timedCounterCommand, timedCounterEvent]()
	repo := es.NewRepository[timedCounterAggregate, timedCounterCommand, timedCounterEvent](store, blankTimedCounter, cfg)
	id := counterID{value: "timed"}
	firstAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	secondAt := firstAt.Add(2 * time.Hour)

	loaded, err := repo.NewAggregate(context.Background(), id)
	if err != nil {
		t.Fatalf("NewAggregate: %v", err)
	}
	loaded, err = repo.Save(context.Background(), loaded, stampTimedCounterCommand{At: firstAt})
	if err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if got := loaded.Aggregate(); !got.updatedAt.Equal(firstAt) {
		t.Fatalf("updatedAt after first Save: got %v, want %v", got.updatedAt, firstAt)
	}

	next, err := repo.Save(context.Background(), loaded, stampTimedCounterCommand{At: secondAt})
	if err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if got := next.Aggregate(); !got.updatedAt.Equal(secondAt) || got.updates != 2 {
		t.Fatalf("next aggregate: got updatedAt=%v updates=%d, want updatedAt=%v updates=2", got.updatedAt, got.updates, secondAt)
	}
}

func TestRepository_LoadAfterSaves_replaysCorrectly(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100
	repo, _ := newCounterRepo(t, cfg)
	id := counterID{value: "3"}

	loaded, err := repo.NewAggregate(context.Background(), id)
	if err != nil {
		t.Fatalf("NewAggregate: %v", err)
	}
	loaded, err = repo.Save(context.Background(), loaded, incrementCommand{By: 7})
	if err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	loaded, err = repo.Save(context.Background(), loaded, incrementCommand{By: 3})
	if err != nil {
		t.Fatalf("Save 2: %v", err)
	}

	got, err := repo.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Aggregate().count != 10 {
		t.Errorf("count: got %d, want 10", got.Aggregate().count)
	}
}

func TestRepository_Save_triggersSnapshot(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 3
	repo, store := newCounterRepo(t, cfg)
	id := counterID{value: "snap"}

	loaded, err := repo.NewAggregate(context.Background(), id)
	if err != nil {
		t.Fatalf("NewAggregate: %v", err)
	}
	for i := 0; i < 3; i++ {
		loaded, err = repo.Save(context.Background(), loaded, incrementCommand{By: 1})
		if err != nil {
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

	loaded, err := repo.NewAggregate(context.Background(), id)
	if err != nil {
		t.Fatalf("NewAggregate: %v", err)
	}
	for i := 0; i < 4; i++ {
		loaded, err = repo.Save(context.Background(), loaded, incrementCommand{By: 2})
		if err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	got, err := repo.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Aggregate().count != 8 {
		t.Errorf("count: got %d, want 8", got.Aggregate().count)
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

	loaded, err := repo.NewAggregate(context.Background(), counterID{value: "lock"})
	if err != nil {
		t.Fatalf("NewAggregate: %v", err)
	}
	_, err = repo.Save(context.Background(), loaded, incrementCommand{By: 1})
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
	loaded, err := repo.NewAggregate(context.Background(), counterID{value: "err"})
	if err != nil {
		t.Fatalf("NewAggregate: %v", err)
	}
	_, err = repo.Save(context.Background(), loaded, unknownCmd{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, es.ErrUnknownCommand) {
		t.Errorf("got %v, want ErrUnknownCommand", err)
	}
}
