// Package protoes は EventSerializer[E] interface の Protobuf 風参照実装を提供する。
//
// v2 の新 API では Event interface はメタ情報を持たず (EventTypeName + AggregateID
// のみ)、ストレージメタ (SeqNr / EventID / OccurredAt 等) は library 側の
// EventEnvelope に集約される。したがって event payload の (de)serialize は
// 利用側のドメイン Event 型に固有の関数で行うのが最も柔軟である。
//
// 本パッケージは「利用側が proto.Marshal / proto.Unmarshal を呼ぶ encode /
// decode 関数を渡せば EventSerializer[E] を満たす thin adapter を作れる」
// 形のヘルパを提供する。proto 自体への直接依存は持たず、利用側が
// google.golang.org/protobuf を使ってドメイン Event ↔ proto message 変換を
// 実装する想定。
//
// なお過去の wire format (event.proto の EventEnvelope) は SeqNr / IsCreated
// 等 library 側に移ったメタを含んでおり obsolete になった。.proto ファイルは
// サンプル兼歴史的レコードとしてリポジトリに残す。
package protoes

import (
	es "github.com/Hiroshi0900/eventstore"
)

// Codec は利用側が定義した encode / decode 関数で構成される EventSerializer[E] 実装。
// proto に限らず任意の wire format で使える generic adapter。
type Codec[E es.Event] struct {
	encode func(E) ([]byte, error)
	decode func(typeName string, data []byte) (E, error)
}

// New は encode / decode 関数から Codec を生成する。
//
// 例 (proto 利用側コード):
//
//	encode := func(ev VisitEvent) ([]byte, error) {
//	    msg := visitEventToProto(ev)
//	    return proto.Marshal(msg)
//	}
//	decode := func(typeName string, data []byte) (VisitEvent, error) {
//	    var msg pb.VisitEvent
//	    if err := proto.Unmarshal(data, &msg); err != nil {
//	        return nil, es.NewDeserializationError("event", err)
//	    }
//	    return protoToVisitEvent(typeName, &msg)
//	}
//	codec := protoes.New[VisitEvent](encode, decode)
func New[E es.Event](
	encode func(E) ([]byte, error),
	decode func(typeName string, data []byte) (E, error),
) *Codec[E] {
	return &Codec[E]{encode: encode, decode: decode}
}

// Serialize は domain Event を bytes に encode する。
func (c *Codec[E]) Serialize(ev E) ([]byte, error) {
	return c.encode(ev)
}

// Deserialize は bytes を typeName で dispatch して domain Event に decode する。
func (c *Codec[E]) Deserialize(typeName string, data []byte) (E, error) {
	return c.decode(typeName, data)
}
