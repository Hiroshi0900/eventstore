package eventsourcing

import (
	"context"
	"fmt"
	"time"
)

// Repository provides a high-level API over EventStore.
// T is the aggregate type and E is its associated Event type.
type Repository[T Aggregate[E], E Event] interface {
	// Load returns the current state of the aggregate by replaying events
	// from the latest snapshot. Returns ErrAggregateNotFound if no snapshot
	// and no events exist.
	Load(ctx context.Context, id AggregateID) (T, error)

	// Save loads the current state, applies the command to produce a single
	// event, persists the event (and snapshot when SnapshotInterval is hit),
	// and returns the next state. If the aggregate does not yet exist,
	// the createBlank function is used to construct an initial state.
	Save(ctx context.Context, id AggregateID, cmd Command) (T, error)
}

// DefaultRepository is the default Repository implementation.
type DefaultRepository[T Aggregate[E], E Event] struct {
	store         EventStore
	createBlank   func(AggregateID) T
	aggSerializer AggregateSerializer[T, E]
	evSerializer  EventSerializer[E]
	config        Config
}

// NewRepository creates a new DefaultRepository.
func NewRepository[T Aggregate[E], E Event](
	store EventStore,
	createBlank func(AggregateID) T,
	aggSerializer AggregateSerializer[T, E],
	evSerializer EventSerializer[E],
	config Config,
) *DefaultRepository[T, E] {
	return &DefaultRepository[T, E]{
		store:         store,
		createBlank:   createBlank,
		aggSerializer: aggSerializer,
		evSerializer:  evSerializer,
		config:        config,
	}
}

// loadInternal returns the current aggregate state, plus the seqNr and version
// of the latest snapshot (or 0,0 if no snapshot exists).
// notFound==true indicates that neither snapshot nor events exist.
func (r *DefaultRepository[T, E]) loadInternal(
	ctx context.Context,
	id AggregateID,
) (agg T, seqNr, version uint64, notFound bool, err error) {
	snap, err := r.store.GetLatestSnapshot(ctx, id)
	if err != nil {
		return agg, 0, 0, false, err
	}

	if snap != nil {
		agg, err = r.aggSerializer.Deserialize(snap.Payload)
		if err != nil {
			return agg, 0, 0, false, err
		}
		seqNr = snap.SeqNr
		version = snap.Version
	} else {
		agg = r.createBlank(id)
		seqNr = 0
		version = 0
	}

	events, err := r.store.GetEventsSince(ctx, id, seqNr)
	if err != nil {
		return agg, 0, 0, false, err
	}

	if snap == nil && len(events) == 0 {
		return agg, 0, 0, true, nil
	}

	for _, env := range events {
		ev, err := r.evSerializer.Deserialize(env.EventTypeName, env.Payload)
		if err != nil {
			return agg, 0, 0, false, err
		}
		next, ok := agg.ApplyEvent(ev).(T)
		if !ok {
			return agg, 0, 0, false, fmt.Errorf(
				"%w: ApplyEvent returned a value that does not implement T",
				ErrInvalidAggregate,
			)
		}
		agg = next
		seqNr = env.SeqNr
	}
	return agg, seqNr, version, false, nil
}

// Load returns the aggregate, or ErrAggregateNotFound if it doesn't exist.
func (r *DefaultRepository[T, E]) Load(ctx context.Context, id AggregateID) (T, error) {
	agg, _, _, notFound, err := r.loadInternal(ctx, id)
	if err != nil {
		var zero T
		return zero, err
	}
	if notFound {
		var zero T
		return zero, NewAggregateNotFoundError(id.TypeName(), id.Value())
	}
	return agg, nil
}

// Save loads the aggregate (or starts blank), applies the command, persists
// the resulting event, optionally creating a snapshot.
func (r *DefaultRepository[T, E]) Save(
	ctx context.Context,
	id AggregateID,
	cmd Command,
) (T, error) {
	var zero T

	agg, currentSeqNr, currentVersion, _, err := r.loadInternal(ctx, id)
	if err != nil {
		return zero, err
	}

	ev, err := agg.ApplyCommand(cmd)
	if err != nil {
		return zero, err
	}

	next, ok := agg.ApplyEvent(ev).(T)
	if !ok {
		return zero, fmt.Errorf(
			"%w: ApplyEvent returned a value that does not implement T",
			ErrInvalidAggregate,
		)
	}

	nextSeqNr := currentSeqNr + 1

	payload, err := r.evSerializer.Serialize(ev)
	if err != nil {
		return zero, err
	}

	eventID, err := generateEventID()
	if err != nil {
		return zero, err
	}

	envelope := &EventEnvelope{
		EventID:       eventID,
		EventTypeName: ev.EventTypeName(),
		AggregateID:   ev.AggregateID(),
		SeqNr:         nextSeqNr,
		IsCreated:     currentSeqNr == 0,
		OccurredAt:    time.Now().UTC(),
		Payload:       payload,
		// TraceParent / TraceState are filled by DynamoDB store via context.
	}

	if r.config.ShouldSnapshot(nextSeqNr) {
		snapPayload, err := r.aggSerializer.Serialize(next)
		if err != nil {
			return zero, err
		}
		snap := &SnapshotEnvelope{
			AggregateID: ev.AggregateID(),
			SeqNr:       nextSeqNr,
			Version:     currentVersion + 1,
			Payload:     snapPayload,
			OccurredAt:  envelope.OccurredAt,
		}
		if err := r.store.PersistEventAndSnapshot(ctx, envelope, snap); err != nil {
			return zero, err
		}
	} else {
		// expectedVersion is used by EventStore for first-write detection
		// (== 0 means "this aggregate must not exist yet"). Once any event has
		// been persisted (currentSeqNr > 0) we must pass a non-zero value to
		// bypass that check, even when no snapshot has been taken yet so
		// currentVersion is still 0. The value itself is otherwise unused —
		// optimistic locking is enforced by PersistEventAndSnapshot.
		expectedVersion := currentVersion
		if currentSeqNr > 0 && expectedVersion == 0 {
			expectedVersion = currentSeqNr
		}
		if err := r.store.PersistEvent(ctx, envelope, expectedVersion); err != nil {
			return zero, err
		}
	}

	return next, nil
}
