package eventstore

import "context"

// Config は Repository / EventStore の共通設定を保持する。
//
// DynamoDB 専用設定 (テーブル名・GSI 名・shard 数等) は v2/dynamodb.Config 側に
// 集約されており、本 Config には Repository が直接使う SnapshotInterval のみが含まれる。
type Config struct {
	// SnapshotInterval はスナップショットを取る seqNr の間隔。0 を指定すると
	// スナップショットは取られない。
	SnapshotInterval uint64
}

// DefaultConfig はデフォルト設定を返す (SnapshotInterval=5)。
func DefaultConfig() Config {
	return Config{SnapshotInterval: 5}
}

// ShouldSnapshot は seqNr においてスナップショットを取るべきかを返す。
func (c Config) ShouldSnapshot(seqNr uint64) bool {
	if c.SnapshotInterval == 0 {
		return false
	}
	return seqNr%c.SnapshotInterval == 0
}

// EventStore は永続化抽象。Repository ↔ EventStore のやり取りは domain 型
// (A Aggregate[C, E], C Command, E Event) で行われ、(de)serialize は EventStore 実装の責務。
//
// memory store は in-memory で型をそのまま保持するので serializer を必要としない。
// dynamodb store はコンストラクタで AggregateSerializer / EventSerializer を受け取り、
// 内部で (de)serialize する。
type EventStore[A Aggregate[C, E], C Command, E Event] interface {
	// GetLatestSnapshot は最新 snapshot を返す。snapshot 未存在の場合は found=false を返す。
	GetLatestSnapshot(ctx context.Context, id AggregateID) (snap StoredSnapshot[A], found bool, err error)

	// GetEventsSince は seqNr より大きい seqNr の events を昇順で返す。
	GetEventsSince(ctx context.Context, id AggregateID, seqNr uint64) ([]StoredEvent[E], error)

	// PersistEvent は event 単独を保存する (snapshot interval 外のケース)。
	// expectedVersion は呼び出し側 context 用で、実装は楽観ロックには使わない。
	// 楽観ロックは PersistEventAndSnapshot に集約。
	// 同一 (AggregateID, SeqNr) の重複保存は ErrDuplicateAggregate で拒否すること。
	PersistEvent(ctx context.Context, ev StoredEvent[E], expectedVersion uint64) error

	// PersistEventAndSnapshot は event と snapshot をアトミックに保存する。
	// 楽観ロックは snap.Version を基準に、現行 snapshot version が snap.Version - 1 と一致しない
	// 場合に ErrOptimisticLock を返す。snap.Version == 1 の場合は初回作成扱い (現行 snapshot 未存在を要求)。
	PersistEventAndSnapshot(ctx context.Context, ev StoredEvent[E], snap StoredSnapshot[A]) error
}
