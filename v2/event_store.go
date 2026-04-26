package eventstore

import "context"

// SnapshotData はスナップショットの永続化データを表します。
type SnapshotData struct {
	Payload []byte // serialized aggregate state
	SeqNr   uint64
	Version uint64
}

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

// EventStore はイベント・スナップショットの永続化抽象です。
type EventStore interface {
	GetLatestSnapshotByID(ctx context.Context, id AggregateID) (*SnapshotData, error)
	GetEventsByIDSinceSeqNr(ctx context.Context, id AggregateID, seqNr uint64) ([]Event, error)
	PersistEvent(ctx context.Context, event Event, version uint64) error
	PersistEventAndSnapshot(ctx context.Context, event Event, snapshot SnapshotData) error
}

// AggregateSerializer は集約 T のスナップショット用シリアライザです。
type AggregateSerializer[T Aggregate] interface {
	Serialize(T) ([]byte, error)
	Deserialize([]byte) (T, error)
}
