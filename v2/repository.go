package eventstore

import (
	"context"
	"fmt"
	"time"
)

// Repository は集約 T の高レベル Load / Save API。
//
// 利用側は domain ごとに `Repository[Visit, VisitCommand, VisitEvent]` のように instantiate する。
// 実装は library 内部の defaultRepository[T,C,E] が担う (NewRepository 経由で取得)。
type Repository[T Aggregate[C, E], C Command, E Event] interface {
	// Load は集約をロードする。snapshot があれば起点に、なければ blank 集約から
	// イベントを replay。snapshot もイベントもなければ ErrAggregateNotFound。
	Load(ctx context.Context, aggID AggregateID) (T, error)

	// Save は aggID に対応する集約をロード (なければ blank) し、cmd を ApplyCommand
	// で適用してイベントを生成、ApplyEvent で次状態に遷移後、永続化して新状態を返す。
	// SnapshotInterval に達した seqNr では snapshot も同時に書く。
	Save(ctx context.Context, aggID AggregateID, cmd C) (T, error)
}

// defaultRepository は Repository interface の library 内部実装 (factory-sealed)。
//
// (de)serialize は EventStore[T,C,E] 実装側 (memory / dynamodb 等) の責務。
// 本 Repository は domain 型のみを扱い、wire format には触れない。
type defaultRepository[T Aggregate[C, E], C Command, E Event] struct {
	store       EventStore[T, C, E]
	createBlank func(AggregateID) T
	config      Config
}

// NewRepository は Repository[T, C, E] を生成する。
//
// createBlank は集約が未存在の状態 (Save の初回呼び出し) で使う初期 aggregate を返す関数。
// 利用側で typed AggregateID と pure struct を組み合わせて定義する。
//
// (de)serialize の責務は store 側にある (dynamodb 等のコンストラクタで Serializer を受け取る)。
func NewRepository[T Aggregate[C, E], C Command, E Event](
	store EventStore[T, C, E],
	createBlank func(AggregateID) T,
	config Config,
) Repository[T, C, E] {
	return &defaultRepository[T, C, E]{
		store:       store,
		createBlank: createBlank,
		config:      config,
	}
}

// loadInternal は snapshot + events から現状態を復元し、現 SeqNr / Version を返す。
// notFound==true は snapshot もイベントも見つからなかった (集約未存在) を意味する。
func (r *defaultRepository[T, C, E]) loadInternal(
	ctx context.Context,
	id AggregateID,
) (agg T, seqNr, version uint64, notFound bool, err error) {
	snap, found, err := r.store.GetLatestSnapshot(ctx, id)
	if err != nil {
		return agg, 0, 0, false, err
	}

	if found {
		agg = snap.Aggregate
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

	if !found && len(events) == 0 {
		return agg, 0, 0, true, nil
	}

	for _, stored := range events {
		next, ok := agg.ApplyEvent(stored.Event).(T)
		if !ok {
			return agg, 0, 0, false, fmt.Errorf(
				"%w: ApplyEvent returned a value that does not implement T",
				ErrInvalidAggregate,
			)
		}
		agg = next
		seqNr = stored.SeqNr
	}
	return agg, seqNr, version, false, nil
}

// Load は集約をロードする。集約未存在の場合は ErrAggregateNotFound を返す。
func (r *defaultRepository[T, C, E]) Load(ctx context.Context, aggID AggregateID) (T, error) {
	agg, _, _, notFound, err := r.loadInternal(ctx, aggID)
	if err != nil {
		var zero T
		return zero, err
	}
	if notFound {
		var zero T
		return zero, NewAggregateNotFoundError(aggID.TypeName(), aggID.Value())
	}
	return agg, nil
}

// Save は load → ApplyCommand → ApplyEvent → 永続化 を一括で行う。
// 集約未存在でも blank で開始するため、creation 系コマンドの初回適用にも使える。
func (r *defaultRepository[T, C, E]) Save(
	ctx context.Context,
	aggID AggregateID,
	cmd C,
) (T, error) {
	var zero T

	agg, currentSeqNr, currentVersion, _, err := r.loadInternal(ctx, aggID)
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
	now := time.Now().UTC()

	eventID, err := generateEventID()
	if err != nil {
		return zero, err
	}

	stored := StoredEvent[E]{
		Event:      ev,
		EventID:    eventID,
		SeqNr:      nextSeqNr,
		IsCreated:  currentSeqNr == 0,
		OccurredAt: now,
		// TraceParent / TraceState は dynamodb 側で context から注入する。
	}

	if r.config.ShouldSnapshot(nextSeqNr) {
		snap := StoredSnapshot[T]{
			Aggregate:  next,
			SeqNr:      nextSeqNr,
			Version:    currentVersion + 1,
			OccurredAt: now,
		}
		if err := r.store.PersistEventAndSnapshot(ctx, stored, snap); err != nil {
			return zero, err
		}
	} else {
		// expectedVersion は EventStore で first-write 検出用 (== 0 で
		// "この aggregate は新規であるべき" の意味)。currentSeqNr > 0 で
		// snapshot 未取得 (currentVersion == 0) の場合、PersistEvent に 0 を
		// 渡すと first-write 扱いで ErrDuplicateAggregate になる。それを避け
		// るため non-zero (currentSeqNr) を渡す。値そのものは楽観ロックには
		// 使われないので意味は問わない。
		expectedVersion := currentVersion
		if currentSeqNr > 0 && expectedVersion == 0 {
			expectedVersion = currentSeqNr
		}
		if err := r.store.PersistEvent(ctx, stored, expectedVersion); err != nil {
			return zero, err
		}
	}

	return next, nil
}
