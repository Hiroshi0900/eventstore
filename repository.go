package eventstore

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

// LoadedAggregate は command 側で再利用可能な aggregate と opaque な永続化文脈を持つ。
type LoadedAggregate[A Aggregate[C, E], C Command, E Event] struct {
	aggregate A
	seqNr     uint64
	version   uint64
	owner     *defaultRepository[A, C, E]
}

// Aggregate は現在の aggregate 状態を返す。
func (l LoadedAggregate[A, C, E]) Aggregate() A {
	return l.aggregate
}

// Repository は集約 A の高レベル Load / Save API。
//
// 利用側は domain ごとに `Repository[Visit, VisitCommand, VisitEvent]` のように instantiate する。
// 実装は library 内部の defaultRepository[A,C,E] が担う (NewRepository 経由で取得)。
type Repository[A Aggregate[C, E], C Command, E Event] interface {
	// Load は集約をロードする。snapshot があれば起点に、なければ blank 集約から
	// イベントを replay。snapshot もイベントもなければ ErrAggregateNotFound。
	Load(ctx context.Context, aggID AggregateID) (A, error)

	// Save は aggID に対応する集約をロード (なければ blank) し、cmd を ApplyCommand
	// で適用してイベントを生成、ApplyEvent で次状態に遷移後、永続化して新状態を返す。
	// SnapshotInterval に達した seqNr では snapshot も同時に書く。
	Save(ctx context.Context, aggID AggregateID, cmd C) (A, error)

	// LoadForCommand は command 側の連続 Save 向けに aggregate と永続化文脈を返す。
	LoadForCommand(ctx context.Context, aggID AggregateID) (LoadedAggregate[A, C, E], error)

	// SaveLoaded は事前にロード済みの aggregate に command を適用して次の loaded 状態を返す。
	SaveLoaded(ctx context.Context, loaded LoadedAggregate[A, C, E], cmd C) (LoadedAggregate[A, C, E], error)
}

// defaultRepository は Repository interface の library 内部実装 (factory-sealed)。
//
// (de)serialize は EventStore[A,C,E] 実装側 (memory / dynamodb 等) の責務。
// 本 Repository は domain 型のみを扱い、wire format には触れない。
type defaultRepository[A Aggregate[C, E], C Command, E Event] struct {
	store       EventStore[A, C, E]
	createBlank func(AggregateID) A
	config      Config
}

// NewRepository は Repository[A, C, E] を生成する。
//
// createBlank は集約が未存在の状態 (Save の初回呼び出し) で使う初期 aggregate を返す関数。
// 利用側で typed AggregateID と pure struct を組み合わせて定義する。
//
// (de)serialize の責務は store 側にある (dynamodb 等のコンストラクタで Serializer を受け取る)。
func NewRepository[A Aggregate[C, E], C Command, E Event](
	store EventStore[A, C, E],
	createBlank func(AggregateID) A,
	config Config,
) Repository[A, C, E] {
	return &defaultRepository[A, C, E]{
		store:       store,
		createBlank: createBlank,
		config:      config,
	}
}

// loadInternal は snapshot + events から現状態を復元し、現 SeqNr / Version を返す。
// notFound==true は snapshot もイベントも見つからなかった (集約未存在) を意味する。
func (r *defaultRepository[A, C, E]) loadInternal(
	ctx context.Context,
	id AggregateID,
) (agg A, seqNr, version uint64, notFound bool, err error) {
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

	events, err := r.store.LoadStreamAfter(ctx, id, seqNr)
	if err != nil {
		return agg, 0, 0, false, err
	}

	if !found && len(events) == 0 {
		return agg, 0, 0, true, nil
	}

	for _, stored := range events {
		next, ok := agg.ApplyEvent(stored.Event).(A)
		if !ok {
			return agg, 0, 0, false, fmt.Errorf(
				"%w: ApplyEvent returned a value that does not implement A",
				ErrInvalidAggregate,
			)
		}
		agg = next
		seqNr = stored.SeqNr
	}
	return agg, seqNr, version, false, nil
}

// Load は集約をロードする。集約未存在の場合は ErrAggregateNotFound を返す。
func (r *defaultRepository[A, C, E]) Load(ctx context.Context, aggID AggregateID) (A, error) {
	agg, _, _, notFound, err := r.loadInternal(ctx, aggID)
	if err != nil {
		var zero A
		return zero, err
	}
	if notFound {
		var zero A
		return zero, NewAggregateNotFoundError(aggID.TypeName(), aggID.Value())
	}
	return agg, nil
}

// LoadForCommand は集約と次の SaveLoaded に必要な永続化文脈をロードする。
func (r *defaultRepository[A, C, E]) LoadForCommand(
	ctx context.Context,
	aggID AggregateID,
) (LoadedAggregate[A, C, E], error) {
	agg, seqNr, version, notFound, err := r.loadInternal(ctx, aggID)
	if err != nil {
		var zero LoadedAggregate[A, C, E]
		return zero, err
	}
	if notFound {
		var zero LoadedAggregate[A, C, E]
		return zero, NewAggregateNotFoundError(aggID.TypeName(), aggID.Value())
	}
	return r.newLoadedAggregate("LoadForCommand", agg, seqNr, version)
}

func (r *defaultRepository[A, C, E]) invalidLoadedAggregateError() error {
	return fmt.Errorf(
		"%w: SaveLoaded requires a loaded aggregate handle created by this repository",
		ErrInvalidAggregate,
	)
}

type savedAggregateState[A any] struct {
	aggregate A
	seqNr     uint64
	version   uint64
}

var timeTimeType = reflect.TypeOf(time.Time{})

func invalidLoadedAggregateTypeReason(typ reflect.Type, path string) string {
	if typ == timeTimeType {
		return ""
	}

	switch typ.Kind() {
	case reflect.Pointer:
		return fmt.Sprintf("%s uses pointer type %s", path, typ)
	case reflect.Map:
		return fmt.Sprintf("%s uses map type %s", path, typ)
	case reflect.Slice:
		return fmt.Sprintf("%s uses slice type %s", path, typ)
	case reflect.Func:
		return fmt.Sprintf("%s uses func type %s", path, typ)
	case reflect.Chan:
		return fmt.Sprintf("%s uses channel type %s", path, typ)
	case reflect.Interface:
		return fmt.Sprintf("%s uses interface type %s", path, typ)
	case reflect.UnsafePointer:
		return fmt.Sprintf("%s uses unsafe pointer type %s", path, typ)
	case reflect.Array:
		return invalidLoadedAggregateTypeReason(typ.Elem(), path+"[]")
	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if reason := invalidLoadedAggregateTypeReason(field.Type, path+"."+field.Name); reason != "" {
				return reason
			}
		}
	}
	return ""
}

func (r *defaultRepository[A, C, E]) unsupportedLoadedAggregateError(
	operation string,
	agg A,
) error {
	typ := reflect.TypeOf(agg)
	if typ == nil {
		return fmt.Errorf(
			"%w: %s requires value-semantic aggregates; aggregate type is nil",
			ErrInvalidAggregate,
			operation,
		)
	}

	return fmt.Errorf(
		"%w: %s requires value-semantic aggregates; %s",
		ErrInvalidAggregate,
		operation,
		invalidLoadedAggregateTypeReason(typ, typ.String()),
	)
}

func (r *defaultRepository[A, C, E]) validateLoadedAggregate(
	operation string,
	agg A,
) error {
	typ := reflect.TypeOf(agg)
	if typ == nil {
		return r.unsupportedLoadedAggregateError(operation, agg)
	}
	if reason := invalidLoadedAggregateTypeReason(typ, typ.String()); reason != "" {
		return fmt.Errorf(
			"%w: %s requires value-semantic aggregates; %s",
			ErrInvalidAggregate,
			operation,
			reason,
		)
	}
	return nil
}

func (r *defaultRepository[A, C, E]) newLoadedAggregate(
	operation string,
	agg A,
	seqNr uint64,
	version uint64,
) (LoadedAggregate[A, C, E], error) {
	var zero LoadedAggregate[A, C, E]

	if err := r.validateLoadedAggregate(operation, agg); err != nil {
		return zero, err
	}

	return LoadedAggregate[A, C, E]{
		aggregate: agg,
		seqNr:     seqNr,
		version:   version,
		owner:     r,
	}, nil
}

func (r *defaultRepository[A, C, E]) persistKnownAggregate(
	ctx context.Context,
	agg A,
	currentSeqNr uint64,
	currentVersion uint64,
	cmd C,
	validateNext func(A) error,
) (savedAggregateState[A], error) {
	var zero savedAggregateState[A]

	ev, err := agg.ApplyCommand(cmd)
	if err != nil {
		return zero, err
	}

	next, ok := agg.ApplyEvent(ev).(A)
	if !ok {
		return zero, fmt.Errorf(
			"%w: ApplyEvent returned a value that does not implement A",
			ErrInvalidAggregate,
		)
	}
	if validateNext != nil {
		if err := validateNext(next); err != nil {
			return zero, err
		}
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

	nextVersion := currentVersion
	if r.config.ShouldSnapshot(nextSeqNr) {
		nextVersion = currentVersion + 1
		snap := StoredSnapshot[A]{
			Aggregate:  next,
			SeqNr:      nextSeqNr,
			Version:    nextVersion,
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

	return savedAggregateState[A]{
		aggregate: next,
		seqNr:     nextSeqNr,
		version:   nextVersion,
	}, nil
}

func (r *defaultRepository[A, C, E]) saveKnownAggregate(
	ctx context.Context,
	agg A,
	currentSeqNr uint64,
	currentVersion uint64,
	cmd C,
) (LoadedAggregate[A, C, E], error) {
	next, err := r.persistKnownAggregate(
		ctx,
		agg,
		currentSeqNr,
		currentVersion,
		cmd,
		func(next A) error {
			return r.validateLoadedAggregate("SaveLoaded", next)
		},
	)
	if err != nil {
		var zero LoadedAggregate[A, C, E]
		return zero, err
	}
	return r.newLoadedAggregate("SaveLoaded", next.aggregate, next.seqNr, next.version)
}

// Save は load → ApplyCommand → ApplyEvent → 永続化 を一括で行う。
// 集約未存在でも blank で開始するため、creation 系コマンドの初回適用にも使える。
func (r *defaultRepository[A, C, E]) Save(
	ctx context.Context,
	aggID AggregateID,
	cmd C,
) (A, error) {
	var zero A

	agg, currentSeqNr, currentVersion, _, err := r.loadInternal(ctx, aggID)
	if err != nil {
		return zero, err
	}

	next, err := r.persistKnownAggregate(ctx, agg, currentSeqNr, currentVersion, cmd, nil)
	if err != nil {
		return zero, err
	}
	return next.aggregate, nil
}

// SaveLoaded はロード済み aggregate に command を適用し、次の loaded 状態を返す。
func (r *defaultRepository[A, C, E]) SaveLoaded(
	ctx context.Context,
	loaded LoadedAggregate[A, C, E],
	cmd C,
) (LoadedAggregate[A, C, E], error) {
	if loaded.owner == nil || loaded.owner != r {
		var zero LoadedAggregate[A, C, E]
		return zero, r.invalidLoadedAggregateError()
	}
	return r.saveKnownAggregate(ctx, loaded.aggregate, loaded.seqNr, loaded.version, cmd)
}
