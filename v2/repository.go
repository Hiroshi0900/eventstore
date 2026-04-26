package eventstore

import "context"

// Repository[T] は集約 T の高レベルロード/保存 API です。
// (de)serialize は EventStore[T] 実装側の責務。Repository は Aggregate を直接やり取りします。
type Repository[T Aggregate] struct {
	store       EventStore[T]
	createBlank func(AggregateID) T
	config      Config
}

// NewRepository は Repository[T] を生成します。
func NewRepository[T Aggregate](
	store EventStore[T],
	createBlank func(AggregateID) T,
	config Config,
) *Repository[T] {
	return &Repository[T]{
		store:       store,
		createBlank: createBlank,
		config:      config,
	}
}

// Load は集約をロードします。スナップショットがあれば起点に、なければ空集約から
// イベントを replay。スナップショットもイベントもない場合は ErrAggregateNotFound。
func (r *Repository[T]) Load(ctx context.Context, id AggregateID) (T, error) {
	var zero T

	snap, found, err := r.store.GetLatestSnapshotByID(ctx, id)
	if err != nil {
		return zero, err
	}

	var agg T
	var seqNr uint64
	if found {
		agg = snap
		seqNr = snap.SeqNr()
	} else {
		agg = r.createBlank(id)
		seqNr = 0
	}

	events, err := r.store.GetEventsByIDSinceSeqNr(ctx, id, seqNr)
	if err != nil {
		return zero, err
	}

	if !found && len(events) == 0 {
		return zero, NewAggregateNotFoundError(id.TypeName(), id.Value())
	}

	for _, ev := range events {
		agg = agg.ApplyEvent(ev).(T)
	}
	return agg, nil
}

// Store はコマンドを集約に適用してイベントを生成・永続化し、新しい集約を返します。
func (r *Repository[T]) Store(ctx context.Context, cmd Command, agg T) (T, error) {
	var zero T

	if cmd.AggregateID().AsString() != agg.AggregateID().AsString() {
		return zero, NewAggregateIDMismatchError(
			cmd.AggregateID().AsString(),
			agg.AggregateID().AsString(),
		)
	}

	ev, err := agg.ApplyCommand(cmd)
	if err != nil {
		return zero, err
	}

	nextSeqNr := agg.SeqNr() + 1
	ev = ev.WithSeqNr(nextSeqNr)
	nextAgg := agg.ApplyEvent(ev).(T)
	nextAgg = nextAgg.WithSeqNr(nextSeqNr).(T)

	if r.config.ShouldSnapshot(nextSeqNr) {
		nextVersion := agg.Version() + 1
		nextAgg = nextAgg.WithVersion(nextVersion).(T)
		if err := r.store.PersistEventAndSnapshot(ctx, ev, nextAgg); err != nil {
			return zero, err
		}
	} else {
		// PersistEvent への version 引数は実装側で無視可。
		// 楽観ロックは PersistEventAndSnapshot に集約。
		if err := r.store.PersistEvent(ctx, ev, agg.Version()); err != nil {
			return zero, err
		}
	}

	return nextAgg, nil
}
