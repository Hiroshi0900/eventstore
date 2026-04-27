package eventstore

// AggregateSerializer は domain Aggregate (T) を bytes に (de)serialize する。
// snapshot 取得時に Repository から呼ばれ、結果が SnapshotEnvelope.Payload に格納される。
//
// 制約 [T Aggregate[C, E], C Command, E Event] により、Aggregate / Command / Event 型の
// ミスマッチを compile time に防ぐ。利用側は domain ごとに 1 つ実装する想定
// (例: visitAggregateSerializer)。
type AggregateSerializer[T Aggregate[C, E], C Command, E Event] interface {
	Serialize(T) ([]byte, error)
	Deserialize([]byte) (T, error)
}

// EventSerializer は domain Event を bytes に (de)serialize する。
// EventEnvelope.Payload に格納する bytes を生成し、Deserialize 時は typeName
// (== Event.EventTypeName()) で具象 Event 型に dispatch する。
//
// 利用側は domain ごとに 1 つ実装する想定 (例: visitEventSerializer)。
type EventSerializer[E Event] interface {
	Serialize(E) ([]byte, error)
	Deserialize(typeName string, data []byte) (E, error)
}
