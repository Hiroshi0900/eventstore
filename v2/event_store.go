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

// EventStore は永続化抽象。Repository ↔ EventStore のやり取りはすべて
// Envelope 経由で行われ、domain 型 (T Aggregate) は EventStore 層に露出しない。
//
// 実装側 (memory.New / dynamodb.New) は EventEnvelope と SnapshotEnvelope を
// そのまま保存・復元する。Aggregate / Event の (de)serialize は Repository が
// AggregateSerializer / EventSerializer 経由で行い、bytes が Envelope.Payload に格納される。
type EventStore interface {
	// GetLatestSnapshot は最新 snapshot envelope を返す。snapshot 未存在の場合は nil, nil を返す。
	GetLatestSnapshot(ctx context.Context, id AggregateID) (*SnapshotEnvelope, error)

	// GetEventsSince は seqNr より大きい seqNr の event envelope を昇順で返す。
	GetEventsSince(ctx context.Context, id AggregateID, seqNr uint64) ([]*EventEnvelope, error)

	// PersistEvent は event envelope 単独を保存する (snapshot interval 外のケース)。
	// expectedVersion は呼び出し側 context 用で、実装は楽観ロックには使わない。
	// 楽観ロックは PersistEventAndSnapshot に集約。
	// 同一 (AggregateID, SeqNr) の重複保存は ErrDuplicateAggregate で拒否すること。
	PersistEvent(ctx context.Context, ev *EventEnvelope, expectedVersion uint64) error

	// PersistEventAndSnapshot は event envelope と snapshot envelope をアトミックに保存する。
	// 楽観ロックは snap.Version を基準に、現行 snapshot version が snap.Version - 1 と一致しない
	// 場合に ErrOptimisticLock を返す。snap.Version == 1 の場合は初回作成扱い (現行 snapshot 未存在を要求)。
	PersistEventAndSnapshot(ctx context.Context, ev *EventEnvelope, snap *SnapshotEnvelope) error
}
