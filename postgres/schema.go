package postgres

import (
	"context"
	"fmt"
)

// SchemaManager は journal / snapshot テーブルの作成・破棄を提供する。
// store の concrete 型を露出させずにテスト・初期化スクリプトから呼べるようにする抽象。
// （DynamoDB adapter の TableManager に対応）
type SchemaManager interface {
	CreateTables(ctx context.Context) error
	DropTables(ctx context.Context) error
}

// CreateTables は journal / snapshot テーブルを作成する（冪等: IF NOT EXISTS）。
//
//   - journal: PRIMARY KEY (aggregate_id, seq_nr) が DynamoDB の
//     attribute_not_exists(pkey) 条件（同一イベント重複拒否）に対応する。
//   - snapshot: aggregate_id を PRIMARY KEY とし「集約あたり最新1件」を保持する。
func (s *store[A, C, E]) CreateTables(ctx context.Context) error {
	journalDDL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
    aggregate_id text   NOT NULL,
    seq_nr       bigint NOT NULL,
    event_id     text   NOT NULL,
    type_name    text   NOT NULL,
    payload      bytea  NOT NULL,
    is_created   boolean NOT NULL,
    occurred_at  timestamptz NOT NULL,
    traceparent  text,
    tracestate   text,
    PRIMARY KEY (aggregate_id, seq_nr)
)`, s.config.JournalTable)

	snapshotDDL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
    aggregate_id text   PRIMARY KEY,
    seq_nr       bigint NOT NULL,
    version      bigint NOT NULL,
    payload      bytea  NOT NULL,
    occurred_at  timestamptz NOT NULL
)`, s.config.SnapshotTable)

	if _, err := s.db.Exec(ctx, journalDDL); err != nil {
		return fmt.Errorf("create journal table: %w", err)
	}
	if _, err := s.db.Exec(ctx, snapshotDDL); err != nil {
		return fmt.Errorf("create snapshot table: %w", err)
	}
	return nil
}

// DropTables は journal / snapshot テーブルを破棄する（テスト用）。
func (s *store[A, C, E]) DropTables(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", s.config.JournalTable)); err != nil {
		return fmt.Errorf("drop journal table: %w", err)
	}
	if _, err := s.db.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", s.config.SnapshotTable)); err != nil {
		return fmt.Errorf("drop snapshot table: %w", err)
	}
	return nil
}
