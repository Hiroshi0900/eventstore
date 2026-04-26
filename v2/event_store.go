package eventstore

import "context"

// Config は EventStore / Repository の設定を保持します。
type Config struct {
	// SnapshotInterval はスナップショットを取る seqNr の間隔です。
	// 0 を指定するとスナップショットは取られません。
	SnapshotInterval uint64

	// JournalTableName / SnapshotTableName は DynamoDB 実装用のテーブル名です。
	JournalTableName  string
	SnapshotTableName string

	// ShardCount は DynamoDB 実装の partition key sharding 数です。
	ShardCount uint64

	// KeepSnapshotCount は DynamoDB 実装で保持する古いスナップショット数。0 は全保持。
	KeepSnapshotCount uint64
}

// DefaultConfig はデフォルト設定を返します。
func DefaultConfig() Config {
	return Config{
		SnapshotInterval:  5,
		JournalTableName:  "journal",
		SnapshotTableName: "snapshot",
		ShardCount:        1,
		KeepSnapshotCount: 0,
	}
}

// ShouldSnapshot は seqNr においてスナップショットを取るべきかを返します。
func (c Config) ShouldSnapshot(seqNr uint64) bool {
	if c.SnapshotInterval == 0 {
		return false
	}
	return seqNr%c.SnapshotInterval == 0
}

// EventStore[T] は集約 T の永続化抽象です。
//
// snapshot は Aggregate (T) を直接やり取りします。バイト列 payload との
// (de)serialize は実装が責務として持ちます (memory は in-memory のため不要、
// dynamodb は AggregateSerializer[T] をコンストラクタで受け取る)。
type EventStore[T Aggregate] interface {
	// GetLatestSnapshotByID は最新スナップショットを返します。
	// 第 2 戻り値 found=false なら snapshot 未存在 (エラーではない)。
	GetLatestSnapshotByID(ctx context.Context, id AggregateID) (T, bool, error)

	// GetEventsByIDSinceSeqNr は seqNr より大きい seqNr のイベントを昇順で返します。
	GetEventsByIDSinceSeqNr(ctx context.Context, id AggregateID, seqNr uint64) ([]Event, error)

	// PersistEvent はイベント単独を保存します (snapshot interval 外のケース)。
	// version 引数は呼び出し側の context 共有用で実装は無視して構いません。
	// 楽観ロックは PersistEventAndSnapshot に集約。
	PersistEvent(ctx context.Context, event Event, version uint64) error

	// PersistEventAndSnapshot はイベントと snapshot をアトミックに保存します。
	// 楽観ロックは aggregate.Version() を起点とし、期待 version は aggregate.Version() - 1。
	// aggregate.Version() == 1 の場合は初回作成扱い。
	PersistEventAndSnapshot(ctx context.Context, event Event, aggregate T) error
}

// AggregateSerializer は集約 T のスナップショット用シリアライザです。
// dynamodb など永続化実装で snapshot payload を組み立てる際に使います。
type AggregateSerializer[T Aggregate] interface {
	Serialize(T) ([]byte, error)
	Deserialize([]byte) (T, error)
}
