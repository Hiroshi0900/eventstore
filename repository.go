package eventstore

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

// LoadedAggregate は aggregate の現在状態と次の Save に必要な永続化文脈を保持する。
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

// Repository は集約 A の高レベル API。
//
// 利用側は domain ごとに `Repository[Visit, VisitCommand, VisitEvent]` のように instantiate する。
// 実装は library 内部の defaultRepository[A,C,E] が担う (NewRepository 経由で取得)。
type Repository[A Aggregate[C, E], C Command, E Event] interface {
	// NewAggregate は新規集約用の blank LoadedAggregate を返す。DB アクセスは行わない。
	// 最初の Save 時に重複作成を検出して ErrDuplicateAggregate を返す。
	NewAggregate(ctx context.Context, aggID AggregateID) (LoadedAggregate[A, C, E], error)

	// Load は既存集約をロードする。snapshot があれば起点に、なければ blank 集約から
	// イベントを replay。snapshot もイベントもなければ ErrAggregateNotFound。
	Load(ctx context.Context, aggID AggregateID) (LoadedAggregate[A, C, E], error)

	// Save は LoadedAggregate に command を適用して次の LoadedAggregate を返す。
	// SnapshotInterval に達した seqNr では snapshot も同時に書く。
	Save(ctx context.Context, loaded LoadedAggregate[A, C, E], cmd C) (LoadedAggregate[A, C, E], error)
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
// createBlank は NewAggregate / Load の初回呼び出し時に使う初期 aggregate を返す関数。
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

	events, err := r.store.GetEventsSince(ctx, id, seqNr)
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

// NewAggregate は新規集約用の blank LoadedAggregate を返す。DB アクセスは行わない。
func (r *defaultRepository[A, C, E]) NewAggregate(
	_ context.Context,
	aggID AggregateID,
) (LoadedAggregate[A, C, E], error) {
	return r.newLoadedAggregate("NewAggregate", r.createBlank(aggID), 0, 0)
}

// Load は既存集約をロードする。集約未存在の場合は ErrAggregateNotFound を返す。
func (r *defaultRepository[A, C, E]) Load(ctx context.Context, aggID AggregateID) (LoadedAggregate[A, C, E], error) {
	agg, seqNr, version, notFound, err := r.loadInternal(ctx, aggID)
	if err != nil {
		var zero LoadedAggregate[A, C, E]
		return zero, err
	}
	if notFound {
		var zero LoadedAggregate[A, C, E]
		return zero, NewAggregateNotFoundError(aggID.TypeName(), aggID.Value())
	}
	return r.newLoadedAggregate("Load", agg, seqNr, version)
}

func (r *defaultRepository[A, C, E]) invalidLoadedAggregateError() error {
	return fmt.Errorf(
		"%w: Save requires a loaded aggregate handle created by this repository",
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
		// Optional fields represented as pointers to basic/named types (e.g. *string, *int,
		// *MyStringType) are copy-safe and common in Go domain models.
		// Reject only pointers whose element type would itself fail the check.
		elemKind := typ.Elem().Kind()
		switch elemKind {
		case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func,
			reflect.Chan, reflect.Interface, reflect.UnsafePointer:
			return fmt.Sprintf("%s uses pointer type %s", path, typ)
		case reflect.Struct:
			return invalidLoadedAggregateTypeReason(typ.Elem(), path)
		}
		// pointer to basic or named-basic kind: allowed
		return ""
	case reflect.Map:
		return fmt.Sprintf("%s uses map type %s", path, typ)
	case reflect.Slice:
		// Allow slices whose element type is itself copy-safe (e.g. []string, []uint8,
		// []MyValueType). Reject slices whose elements contain reference types
		// (e.g. []*T, []map[K]V, [][]T) since those would be unsafe to copy.
		if reason := invalidLoadedAggregateTypeReason(typ.Elem(), path+"[]"); reason != "" {
			return fmt.Sprintf("%s uses slice type %s", path, typ)
		}
		return ""
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

// Save は LoadedAggregate に command を適用し、次の LoadedAggregate を返す。
func (r *defaultRepository[A, C, E]) Save(
	ctx context.Context,
	loaded LoadedAggregate[A, C, E],
	cmd C,
) (LoadedAggregate[A, C, E], error) {
	if loaded.owner == nil || loaded.owner != r {
		var zero LoadedAggregate[A, C, E]
		return zero, r.invalidLoadedAggregateError()
	}

	next, err := r.persistKnownAggregate(
		ctx,
		loaded.aggregate,
		loaded.seqNr,
		loaded.version,
		cmd,
		func(next A) error {
			return r.validateLoadedAggregate("Save", next)
		},
	)
	if err != nil {
		var zero LoadedAggregate[A, C, E]
		return zero, err
	}
	return r.newLoadedAggregate("Save", next.aggregate, next.seqNr, next.version)
}
