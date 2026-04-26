package eventstore

import "context"

// Repository[T] は集約 T の高レベルロード/保存 API です。
type Repository[T Aggregate] struct {
	store       EventStore
	createBlank func(AggregateID) T
	serializer  AggregateSerializer[T]
	config      Config
}

// NewRepository は Repository[T] を生成します。
func NewRepository[T Aggregate](
	store EventStore,
	createBlank func(AggregateID) T,
	serializer AggregateSerializer[T],
	config Config,
) *Repository[T] {
	return &Repository[T]{
		store:       store,
		createBlank: createBlank,
		serializer:  serializer,
		config:      config,
	}
}

// Load は集約をロードします。スナップショットがあれば起点に、なければ空集約から
// イベントを replay。スナップショットもイベントもない場合は ErrAggregateNotFound。
func (r *Repository[T]) Load(ctx context.Context, id AggregateID) (T, error) {
	var zero T

	snap, err := r.store.GetLatestSnapshotByID(ctx, id)
	if err != nil {
		return zero, err
	}

	var agg T
	var seqNr uint64
	if snap != nil {
		restored, err := r.serializer.Deserialize(snap.Payload)
		if err != nil {
			return zero, err
		}
		agg = restored.WithVersion(snap.Version).(T)
		seqNr = snap.SeqNr
	} else {
		agg = r.createBlank(id)
		seqNr = 0
	}

	events, err := r.store.GetEventsByIDSinceSeqNr(ctx, id, seqNr)
	if err != nil {
		return zero, err
	}

	if snap == nil && len(events) == 0 {
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
		payload, err := r.serializer.Serialize(nextAgg)
		if err != nil {
			return zero, err
		}
		nextVersion := agg.Version() + 1
		snap := SnapshotData{Payload: payload, SeqNr: nextSeqNr, Version: nextVersion}
		if err := r.store.PersistEventAndSnapshot(ctx, ev, snap); err != nil {
			return zero, err
		}
		nextAgg = nextAgg.WithVersion(nextVersion).(T)
	} else {
		// PersistEvent への version 引数は「初回作成識別」のみ。
		// 楽観ロックは PersistEventAndSnapshot に集約。
		if err := r.store.PersistEvent(ctx, ev, agg.Version()); err != nil {
			return zero, err
		}
	}

	return nextAgg, nil
}
