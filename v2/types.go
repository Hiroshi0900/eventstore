// Package eventstore は Event Sourcing を実現するためのライブラリです (v2)。
//
// ドメイン側の Aggregate / Event / Command interface はビジネス情報のみを
// 持ち、SeqNr / Version / EventID / OccurredAt / IsCreated 等の永続化メタ
// 情報は library 側の StoredEvent / StoredSnapshot に集約される。これにより
// 利用側は「業務概念としての state と振る舞い」のみを実装すればよい。
package eventstore

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// === Domain interfaces ===

// 主要なドメインインターフェース。具象型はすべて利用側で定義する想定で、
// library は default 実装 (DefaultAggregateID, BaseAggregate 等) を提供しない。
type (
	// AggregateID は集約の識別子を表す。利用側は domain ごとに typed な
	// 実装を定義する (例: type VisitID struct{...} で TypeName/Value/AsString を実装)。
	AggregateID interface {
		TypeName() string
		Value() string
		AsString() string
	}

	// Aggregate は Event Sourcing における集約ルート。
	// C は集約専用の Command 型、E は集約専用の Event 型（いずれも利用側で定義する
	// domain interface / union を想定）。
	//
	// メタ情報 (SeqNr / Version / 永続化メタ) を一切持たず、ApplyCommand /
	// ApplyEvent で表現される業務状態遷移のみを定義する。
	//
	// ApplyEvent の戻り値が Aggregate[C, E] (interface) なので、状態遷移時に
	// 異なる具象型を返す state pattern が可能 (例: VisitScheduled → VisitCompleted)。
	Aggregate[C Command, E Event] interface {
		AggregateID() AggregateID
		ApplyCommand(C) (E, error)
		ApplyEvent(E) Aggregate[C, E]
	}

	// Command はドメインの意図を表す。
	// AggregateID は持たない (Repository.Save が aggID を別引数で受け取るため冗長)。
	Command interface {
		CommandTypeName() string
	}

	// Event は過去に発生した不変のドメインイベント。
	// EventTypeName は EventSerializer の dispatch / OTel span name / 監査用。
	// 永続化メタ (EventID / SeqNr / IsCreated / OccurredAt) は StoredEvent に集約される。
	Event interface {
		EventTypeName() string
		AggregateID() AggregateID
	}
)

// === Stored* (library-side metadata + domain values) ===

// StoredEvent は domain Event に library-side のメタ情報を付与した型。
// Repository ↔ EventStore 間でやり取りされる。
//
// EventStore 実装は domain 型を直接扱い、必要なら自身で (de)serialize する。
// Repository 層には serialize の概念が漏れない設計。
type StoredEvent[E Event] struct {
	Event       E
	EventID     string
	SeqNr       uint64
	IsCreated   bool
	OccurredAt  time.Time
	TraceParent string
	TraceState  string
}

// StoredSnapshot は domain Aggregate に library-side のメタ情報を付与した型。
// Repository ↔ EventStore 間でやり取りされる。
type StoredSnapshot[T any] struct {
	Aggregate  T
	SeqNr      uint64
	Version    uint64
	OccurredAt time.Time
}

// === Internal helpers ===

// generateEventID は 16 random bytes から 32-char hex string を生成する。
// StoredEvent.EventID の発行に Repository から呼ばれる。実用上 ULID/UUIDv4
// と同等の一意性を持つ。
func generateEventID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
