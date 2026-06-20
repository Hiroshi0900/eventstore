// Package postgres provides a PostgreSQL-backed EventStore implementation.
//
// DynamoDB adapter と同一の es.EventStore[A, C, E] interface を実装し、両者を
// 差し替え可能にする（移行時のデュアルライト用途）。DynamoDB 固有の
// TransactWriteItems + ConditionExpression による楽観ロックは、PostgreSQL の
// 単一トランザクション + UNIQUE 制約 + version 条件付き UPSERT に 1:1 対応する。
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	es "github.com/Hiroshi0900/eventstore"
)

// DB は pgx の最小 subset interface（pgxpool.Pool が満たす）。
// 楽観ロックのため Begin による明示トランザクションを必要とする。
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Config は PostgreSQL event store の設定。DynamoDB と異なり sharding / GSI は不要。
type Config struct {
	JournalTable  string
	SnapshotTable string
}

// DefaultConfig はデフォルト設定を返す。
func DefaultConfig() Config {
	return Config{
		JournalTable:  "event_journal",
		SnapshotTable: "event_snapshot",
	}
}

// store は PostgreSQL-backed EventStore[A, C, E] 実装。concrete 型は非公開で、
// 外部からは New() が返す es.EventStore[A, C, E] interface 経由でのみ操作する。
type store[A es.Aggregate[C, E], C es.Command, E es.Event] struct {
	db     DB
	config Config
	aggSer es.AggregateSerializer[A, C, E]
	evSer  es.EventSerializer[E]

	// 構築時に table 名から組み立てた SQL（毎回の fmt.Sprintf を避ける）。
	selectSnapshotSQL string
	selectEventsSQL   string
	insertEventSQL    string
	upsertSnapshotSQL string
}

func newStore[A es.Aggregate[C, E], C es.Command, E es.Event](
	db DB,
	config Config,
	aggSer es.AggregateSerializer[A, C, E],
	evSer es.EventSerializer[E],
) *store[A, C, E] {
	j := config.JournalTable
	s := config.SnapshotTable
	return &store[A, C, E]{
		db:     db,
		config: config,
		aggSer: aggSer,
		evSer:  evSer,
		selectSnapshotSQL: fmt.Sprintf(
			`SELECT seq_nr, version, payload, occurred_at FROM %s WHERE aggregate_id = $1`, s),
		selectEventsSQL: fmt.Sprintf(
			`SELECT event_id, seq_nr, type_name, payload, is_created, occurred_at,
			        COALESCE(traceparent, ''), COALESCE(tracestate, '')
			 FROM %s WHERE aggregate_id = $1 AND seq_nr > $2 ORDER BY seq_nr ASC`, j),
		insertEventSQL: fmt.Sprintf(
			`INSERT INTO %s
			   (aggregate_id, seq_nr, event_id, type_name, payload, is_created, occurred_at, traceparent, tracestate)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (aggregate_id, seq_nr) DO NOTHING`, j),
		// version 条件付き UPSERT。DynamoDB の
		//   attribute_not_exists(version) OR version = :expected
		// と等価:
		//   - 行が無ければ INSERT（expected 不問）
		//   - 行があり version = expected なら UPDATE
		//   - 行があり version != expected なら 0 行（楽観ロック失敗）
		upsertSnapshotSQL: fmt.Sprintf(
			`INSERT INTO %s (aggregate_id, seq_nr, version, payload, occurred_at)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (aggregate_id) DO UPDATE
			   SET seq_nr = EXCLUDED.seq_nr, version = EXCLUDED.version,
			       payload = EXCLUDED.payload, occurred_at = EXCLUDED.occurred_at
			 WHERE %s.version = $6`, s, s),
	}
}

// New は PostgreSQL-backed EventStore[A, C, E] を生成する。
// db には *pgxpool.Pool を渡す想定。
func New[A es.Aggregate[C, E], C es.Command, E es.Event](
	db DB,
	config Config,
	aggSer es.AggregateSerializer[A, C, E],
	evSer es.EventSerializer[E],
) es.EventStore[A, C, E] {
	return newStore(db, config, aggSer, evSer)
}

// NewWithSchema は EventStore[A, C, E] + SchemaManager を併せ持つオブジェクトを返す。
// テーブル管理が必要なテスト・初期化スクリプトで使う。
func NewWithSchema[A es.Aggregate[C, E], C es.Command, E es.Event](
	db DB,
	config Config,
	aggSer es.AggregateSerializer[A, C, E],
	evSer es.EventSerializer[E],
) interface {
	es.EventStore[A, C, E]
	SchemaManager
} {
	return newStore(db, config, aggSer, evSer)
}

// GetLatestSnapshot は最新 snapshot を返す。未存在なら found=false。
func (s *store[A, C, E]) GetLatestSnapshot(ctx context.Context, id es.AggregateID) (es.StoredSnapshot[A], bool, error) {
	var zero es.StoredSnapshot[A]

	var (
		seqNr, version int64
		payload        []byte
		occurred       time.Time
	)
	err := s.db.QueryRow(ctx, s.selectSnapshotSQL, id.AsString()).
		Scan(&seqNr, &version, &payload, &occurred)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, fmt.Errorf("get snapshot: %w", err)
	}

	agg, err := s.aggSer.Deserialize(payload)
	if err != nil {
		return zero, false, err
	}
	return es.StoredSnapshot[A]{
		Aggregate: agg,
		// #nosec G115 -- seq_nr / version は非負で実用範囲
		SeqNr:      uint64(seqNr),
		Version:    uint64(version),
		OccurredAt: occurred.UTC(),
	}, true, nil
}

// GetEventsSince は seqNr より大きい seqNr の events を昇順で返す（強整合）。
func (s *store[A, C, E]) GetEventsSince(ctx context.Context, id es.AggregateID, seqNr uint64) ([]es.StoredEvent[E], error) {
	if seqNr == ^uint64(0) {
		return nil, nil
	}

	// #nosec G115 -- seqNr は実用範囲で int64 に収まる（sentinel は上で除外済み）
	rows, err := s.db.Query(ctx, s.selectEventsSQL, id.AsString(), int64(seqNr))
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	stored := make([]es.StoredEvent[E], 0)
	for rows.Next() {
		var (
			eventID, typeName       string
			seq                     int64
			payload                 []byte
			isCreated               bool
			occurred                time.Time
			traceparent, tracestate string
		)
		if err := rows.Scan(&eventID, &seq, &typeName, &payload, &isCreated, &occurred, &traceparent, &tracestate); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		ev, err := s.evSer.Deserialize(typeName, payload)
		if err != nil {
			return nil, err
		}
		stored = append(stored, es.StoredEvent[E]{
			Event:   ev,
			EventID: eventID,
			// #nosec G115 -- seq_nr は非負で実用範囲
			SeqNr:       uint64(seq),
			IsCreated:   isCreated,
			OccurredAt:  occurred.UTC(),
			TraceParent: traceparent,
			TraceState:  tracestate,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return stored, nil
}

// PersistEvent は event 単独を保存する。同一 (aggregate_id, seq_nr) は
// ErrDuplicateAggregate で拒否する。
func (s *store[A, C, E]) PersistEvent(ctx context.Context, ev es.StoredEvent[E], _ uint64) error {
	payload, err := s.evSer.Serialize(ev.Event)
	if err != nil {
		return err
	}
	tp, ts := traceContext(ctx)
	tag, err := s.db.Exec(ctx, s.insertEventSQL,
		ev.Event.AggregateID().AsString(),
		// #nosec G115 -- SeqNr は実用範囲
		int64(ev.SeqNr),
		ev.EventID,
		ev.Event.EventTypeName(),
		payload,
		ev.IsCreated,
		ev.OccurredAt.UTC(),
		tp, ts,
	)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return es.NewDuplicateAggregateError(ev.Event.AggregateID().AsString())
	}
	return nil
}

// PersistEventAndSnapshot は event と snapshot を 1 トランザクションでアトミックに保存する。
// 楽観ロックは snap.Version を基準に、event 重複 or snapshot version 不一致のいずれでも
// ErrOptimisticLock を返す（DynamoDB の TransactWriteItems と同一の意味論）。
func (s *store[A, C, E]) PersistEventAndSnapshot(ctx context.Context, ev es.StoredEvent[E], snap es.StoredSnapshot[A]) error {
	eventPayload, err := s.evSer.Serialize(ev.Event)
	if err != nil {
		return err
	}
	snapPayload, err := s.aggSer.Serialize(snap.Aggregate)
	if err != nil {
		return err
	}
	expected := snap.Version - 1
	aid := snap.Aggregate.AggregateID().AsString()
	tp, ts := traceContext(ctx)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // commit 後は no-op

	evTag, err := tx.Exec(ctx, s.insertEventSQL,
		ev.Event.AggregateID().AsString(),
		// #nosec G115 -- SeqNr は実用範囲
		int64(ev.SeqNr),
		ev.EventID,
		ev.Event.EventTypeName(),
		eventPayload,
		ev.IsCreated,
		ev.OccurredAt.UTC(),
		tp, ts,
	)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	if evTag.RowsAffected() == 0 {
		// 同一 (aggregate_id, seq_nr) が既存 = 並行更新。
		return es.NewOptimisticLockError(aid, expected, 0)
	}

	snapTag, err := tx.Exec(ctx, s.upsertSnapshotSQL,
		aid,
		// #nosec G115 -- SeqNr / Version は実用範囲
		int64(snap.SeqNr),
		int64(snap.Version),
		snapPayload,
		snap.OccurredAt.UTC(),
		int64(expected),
	)
	if err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}
	if snapTag.RowsAffected() == 0 {
		// 既存 snapshot の version != expected = 並行更新。
		return es.NewOptimisticLockError(aid, expected, 0)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// traceContext は OTel propagator から traceparent / tracestate を取り出す。
// 空の場合は nil を返し、列を NULL にする（DynamoDB の「属性を付けない」に対応）。
func traceContext(ctx context.Context) (traceparent, tracestate any) {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return nullIfEmpty(carrier.Get("traceparent")), nullIfEmpty(carrier.Get("tracestate"))
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
