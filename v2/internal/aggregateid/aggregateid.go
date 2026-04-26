// Package aggregateid は AggregateID interface の最小実装を提供します。
//
// このパッケージは v2 内部 (eventstore 本体および serializer 実装) 専用です。
// エンドユーザは独自の typed AggregateID 型 (例: OrderID, CounterID) を
// ドメイン側で定義してください。
package aggregateid

import "fmt"

// ID は eventstore.AggregateID interface を満たす最小実装です。
// シリアライザの Deserialize 路など、ドメイン型に依存せずに
// AggregateID を構築する必要がある箇所で使います。
type ID struct {
	typeName string
	value    string
}

// New は typeName / value を持つ ID を生成します。
func New(typeName, value string) ID {
	return ID{typeName: typeName, value: value}
}

// TypeName は集約タイプ名を返します。
func (i ID) TypeName() string { return i.typeName }

// Value は集約値を返します。
func (i ID) Value() string { return i.value }

// AsString は "TypeName-Value" 形式の文字列を返します。
func (i ID) AsString() string { return fmt.Sprintf("%s-%s", i.typeName, i.value) }
