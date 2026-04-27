# v2 Domain Purity Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** main にマージ済みの v2 サブモジュール (`github.com/Hiroshi0900/eventstore/v2`) に対して、ドメイン純度を高める refactor を delta として適用する。`Aggregate` / `Event` / `Command` interface からストレージメタ情報 (SeqNr / Version / EventID / OccurredAt / IsCreated / Payload bytes) を完全排除し、ライブラリ側の `EventEnvelope` / `SnapshotEnvelope` に集約する。

**Architecture:** Repository を `Save(aggID, cmd)` に refactor、EventStore を Envelope ベース化、`Aggregate[E Event]` ジェネリクス導入、AggregateSerializer + EventSerializer 分離。`BaseAggregate` / `BaseEvent` / `internal/aggregateid` / `JSONEventSerializer` / `internal/envelope` を削除し、`v2/envelope.go` を新規公開。Protobuf serializer は新 API に追従して書き直す。

**Tech Stack:** Go 1.25, AWS SDK Go v2 (DynamoDB), OpenTelemetry, 標準 `testing` パッケージ, Protocol Buffers

**スコープ外:** terrat-go-app の v2 移行（別計画）

---

## ファイル構造

### 新規追加 (v2/)

| ファイル | 内容 |
|---|---|
| `v2/envelope.go` | `EventEnvelope` / `SnapshotEnvelope` 公開型、JSON marshal/unmarshal |
| `v2/eventid.go` | `generateEventID` 内部ヘルパ (crypto/rand + hex 32 chars) |

### 大規模修正 (v2/)

| ファイル | 変更内容 |
|---|---|
| `v2/types.go` | `Aggregate` / `Event` / `Command` interface のメタ情報排除、`Aggregate[E Event]` ジェネリック化 |
| `v2/aggregate.go` | **削除**（`BaseAggregate` 廃止） |
| `v2/event.go` | **削除**（`BaseEvent` / `EventOption` 廃止） |
| `v2/event_test.go` | **削除**（BaseEvent 廃止に伴う） |
| `v2/event_store.go` | `EventStore[T]` から generic 削除、Envelope ベースへ。`Config` を `SnapshotInterval` のみに |
| `v2/event_store_test.go` | Config 整理に追従 |
| `v2/repository.go` | `Repository[T,E]` ジェネリック強化、`Save(aggID, cmd)` API |
| `v2/repository_test.go` | 全テスト書き換え（counter aggregate を pure struct に） |
| `v2/serializer.go` | `AggregateSerializer[T,E]` + `EventSerializer[E]` 分離、`JSONEventSerializer` 削除 |
| `v2/serializer_test.go` | **削除**（JSONEventSerializer なし） |
| `v2/errors.go` | `ErrAggregateIDMismatch` 削除（Command が AggregateID 持たないので不要）。`ErrInvalidEvent` / `ErrEventStoreUnavailable` も未使用なら削除 |
| `v2/errors_test.go` | 削除した sentinel に対応 |
| `v2/memory/store.go` | Envelope ベースに改修、generic 削除 |
| `v2/memory/store_test.go` | 全テスト書き換え |
| `v2/dynamodb/store.go` | Envelope ベースに改修、`AggregateSerializer[T]` 引数削除（Repository 側で serialize） |
| `v2/dynamodb/store_test.go` | 全テスト書き換え |
| `v2/serialization/protoes/serializer.go` | `EventSerializer[E]` interface に追従して書き直し |
| `v2/serialization/protoes/serializer_test.go` | テスト追従 |
| `v2/serialization/protoes/event.proto` | Envelope wire format に再設計 |

### 削除

| ファイル | 削除理由 |
|---|---|
| `v2/internal/aggregateid/aggregateid.go` | 利用側が独自 ID 型を定義する想定。library で deserialize 時に必要なら envelope.go 内 helper として保持 |
| `v2/internal/envelope/envelope.go` | 公開 Envelope 型に置き換え |
| `v2/internal/envelope/envelope_test.go` | 同上 |

### 維持

| ファイル | 理由 |
|---|---|
| `v2/internal/keyresolver/` | DynamoDB 用 partition key 解決、変更不要 |
| `v2/go.mod` / `v2/go.sum` | 依存追加なし |

### ドキュメント

- `README.md` の v2 Quick Start を新 API に追従

---

## Phase 0: 準備

### Task 0: spec / plan のコミット

- [ ] **Step 1: spec / plan を main 起点 refactor として明記**

このファイル (`docs/superpowers/plans/2026-04-27-eventstore-v2-domain-purity-refactor.md`) と spec (`docs/superpowers/specs/2026-04-26-eventstore-v2-design.md`) を新ブランチにコミット。

```bash
git add docs/superpowers/
git commit -m "docs: add v2 domain purity refactor spec & plan"
```

---

## Phase 1: Envelope 公開化（土台）

### Task 1: `v2/envelope.go` を新規作成

**Files:**
- Create: `v2/envelope.go`

EventEnvelope / SnapshotEnvelope 公開型を導入。Repository ↔ EventStore の境界はこの型で会話する。

- [ ] **Step 1: 実装**

`v2/envelope.go`:
```go
package eventstore

import (
	"encoding/json"
	"time"
)

// EventEnvelope wraps a serialized domain event with all storage metadata.
// Repository ↔ EventStore のやり取りはこの型を経由。
type EventEnvelope struct {
	EventID       string
	EventTypeName string
	AggregateID   AggregateID
	SeqNr         uint64
	IsCreated     bool
	OccurredAt    time.Time
	Payload       []byte
	TraceParent   string
	TraceState    string
}

type eventEnvelopeJSON struct {
	EventID        string    `json:"event_id"`
	EventTypeName  string    `json:"event_type_name"`
	AggregateType  string    `json:"aggregate_type"`
	AggregateValue string    `json:"aggregate_value"`
	SeqNr          uint64    `json:"seq_nr"`
	IsCreated      bool      `json:"is_created"`
	OccurredAt     time.Time `json:"occurred_at"`
	Payload        []byte    `json:"payload"`
	TraceParent    string    `json:"traceparent,omitempty"`
	TraceState     string    `json:"tracestate,omitempty"`
}

func (e EventEnvelope) MarshalJSON() ([]byte, error) {
	return json.Marshal(eventEnvelopeJSON{
		EventID: e.EventID, EventTypeName: e.EventTypeName,
		AggregateType: e.AggregateID.TypeName(), AggregateValue: e.AggregateID.Value(),
		SeqNr: e.SeqNr, IsCreated: e.IsCreated,
		OccurredAt: e.OccurredAt, Payload: e.Payload,
		TraceParent: e.TraceParent, TraceState: e.TraceState,
	})
}

func (e *EventEnvelope) UnmarshalJSON(data []byte) error {
	var j eventEnvelopeJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	e.EventID, e.EventTypeName = j.EventID, j.EventTypeName
	e.AggregateID = simpleAggregateID{typeName: j.AggregateType, value: j.AggregateValue}
	e.SeqNr, e.IsCreated = j.SeqNr, j.IsCreated
	e.OccurredAt, e.Payload = j.OccurredAt, j.Payload
	e.TraceParent, e.TraceState = j.TraceParent, j.TraceState
	return nil
}

// SnapshotEnvelope wraps a serialized aggregate state with metadata.
type SnapshotEnvelope struct {
	AggregateID AggregateID
	SeqNr       uint64
	Version     uint64
	Payload     []byte
	OccurredAt  time.Time
}

type snapshotEnvelopeJSON struct {
	AggregateType  string    `json:"aggregate_type"`
	AggregateValue string    `json:"aggregate_value"`
	SeqNr          uint64    `json:"seq_nr"`
	Version        uint64    `json:"version"`
	Payload        []byte    `json:"payload"`
	OccurredAt     time.Time `json:"occurred_at"`
}

func (s SnapshotEnvelope) MarshalJSON() ([]byte, error) {
	return json.Marshal(snapshotEnvelopeJSON{
		AggregateType: s.AggregateID.TypeName(), AggregateValue: s.AggregateID.Value(),
		SeqNr: s.SeqNr, Version: s.Version, Payload: s.Payload, OccurredAt: s.OccurredAt,
	})
}

func (s *SnapshotEnvelope) UnmarshalJSON(data []byte) error {
	var j snapshotEnvelopeJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	s.AggregateID = simpleAggregateID{typeName: j.AggregateType, value: j.AggregateValue}
	s.SeqNr, s.Version = j.SeqNr, j.Version
	s.Payload, s.OccurredAt = j.Payload, j.OccurredAt
	return nil
}

// simpleAggregateID is an unexported AggregateID implementation used only by
// envelope deserialization (and dynamodb attribute unmarshal). User code must
// define its own typed AggregateID.
type simpleAggregateID struct {
	typeName string
	value    string
}

func (s simpleAggregateID) TypeName() string { return s.typeName }
func (s simpleAggregateID) Value() string    { return s.value }
func (s simpleAggregateID) AsString() string { return s.typeName + "-" + s.value }
```

- [ ] **Step 2: テストを追加** — `v2/envelope_test.go` で JSON roundtrip を検証

`v2/envelope_test.go`:
```go
package eventstore_test

import (
	"encoding/json"
	"testing"
	"time"

	es "github.com/Hiroshi0900/eventstore/v2"
)

// envelope テストでは独自の AggregateID 型を定義（library が NewAggregateID を提供しない方針）。
type testAggID struct {
	typeName, value string
}

func (t testAggID) TypeName() string { return t.typeName }
func (t testAggID) Value() string    { return t.value }
func (t testAggID) AsString() string { return t.typeName + "-" + t.value }

func TestEventEnvelope_jsonRoundtrip(t *testing.T) {
	occurred := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	in := es.EventEnvelope{
		EventID:       "ev-1",
		EventTypeName: "VisitScheduled",
		AggregateID:   testAggID{"Visit", "x"},
		SeqNr:         1,
		IsCreated:     true,
		OccurredAt:    occurred,
		Payload:       []byte(`{"k":"v"}`),
		TraceParent:   "00-trace-span-01",
		TraceState:    "vendor=value",
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out es.EventEnvelope
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.AggregateID.AsString() != "Visit-x" || out.SeqNr != 1 || !out.OccurredAt.Equal(occurred) {
		t.Errorf("roundtrip mismatch: %+v", out)
	}
}

func TestSnapshotEnvelope_jsonRoundtrip(t *testing.T) {
	occurred := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	in := es.SnapshotEnvelope{
		AggregateID: testAggID{"Visit", "x"},
		SeqNr:       5,
		Version:     1,
		Payload:     []byte(`{"state":"x"}`),
		OccurredAt:  occurred,
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out es.SnapshotEnvelope
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.AggregateID.AsString() != "Visit-x" || out.Version != 1 {
		t.Errorf("roundtrip mismatch: %+v", out)
	}
}
```

- [ ] **Step 3: build / test**

```bash
cd v2 && go build ./... && go test ./...
```

注: `simpleAggregateID` は同じ package 内なので envelope_test.go から見えないが、roundtrip 後の `out.AggregateID` の AsString だけ検証すれば十分。

- [ ] **Step 4: Commit**

```bash
git add v2/envelope.go v2/envelope_test.go
git commit -m "feat(v2): add public EventEnvelope and SnapshotEnvelope types"
```

---

## Phase 2: ドメイン interface 改修

### Task 2: `v2/types.go` で interface のメタ排除 + ジェネリック化

**Files:**
- Modify: `v2/types.go`

main の現状：
```go
type Aggregate interface {
    AggregateID() AggregateID
    SeqNr() uint64
    Version() uint64
    ApplyCommand(Command) (Event, error)
    ApplyEvent(Event) Aggregate
    WithVersion(uint64) Aggregate
    WithSeqNr(uint64) Aggregate
}
type Event interface {
    EventID() string
    EventTypeName() string
    AggregateID() AggregateID
    SeqNr() uint64
    IsCreated() bool
    OccurredAt() time.Time
    Payload() []byte
    WithSeqNr(uint64) Event
}
type Command interface {
    CommandTypeName() string
    AggregateID() AggregateID
}
```

target：
```go
type AggregateID interface {
    TypeName() string
    Value() string
    AsString() string
}

type Aggregate[E Event] interface {
    AggregateID() AggregateID
    ApplyCommand(Command) (E, error)
    ApplyEvent(E) Aggregate[E]
}

type Event interface {
    EventTypeName() string
    AggregateID() AggregateID
}

type Command interface {
    CommandTypeName() string
}
```

- [ ] **Step 1: `v2/types.go` を target の内容に書き換え**
- [ ] **Step 2: `time` import 削除（不要に）**
- [ ] **Step 3: `cd v2 && go build ./...` で各ファイルの broken 箇所を洗い出し**

   この時点で大量の compile error が出る (BaseAggregate / BaseEvent / WithSeqNr 等への参照)。Phase 3 以降で順次解消するので、Phase 2 は types.go の修正のみで commit を切らずに次へ進む。

   または types.go 修正と aggregate.go / event.go 削除を同 commit にまとめても良い（後者推奨）。

### Task 3: `v2/aggregate.go` / `v2/event.go` / `v2/event_test.go` 削除

**Files:**
- Delete: `v2/aggregate.go`
- Delete: `v2/event.go`
- Delete: `v2/event_test.go`

`BaseAggregate` / `BaseEvent` / `EventOption` / `NewEvent` / `NewBaseEvent` / `NewBaseAggregate` / `WithSeqNr` (関数) / `WithIsCreated` (関数) / `WithOccurredAt` (関数) を全削除。

- [ ] **Step 1: ファイル削除**

```bash
rm v2/aggregate.go v2/event.go v2/event_test.go
```

- [ ] **Step 2: build 確認（types.go 改修と一緒に。まだ多数 broken）**

- [ ] **Step 3: Commit (types.go + 削除をまとめる)**

```bash
git add v2/types.go v2/aggregate.go v2/event.go v2/event_test.go
git commit -m "refactor(v2): purge metadata from domain interfaces; drop BaseAggregate/BaseEvent

Aggregate / Event / Command interface からストレージメタ情報
(SeqNr / Version / EventID / OccurredAt / IsCreated / Payload / WithSeqNr / WithVersion)
を完全排除。Aggregate[E Event] ジェネリック化で Event 型を伝播。
BaseAggregate / BaseEvent / EventOption など boilerplate types は削除。"
```

この commit 直後は v2/repository.go / v2/serializer.go / v2/event_store.go / v2/memory/* / v2/dynamodb/* / v2/serialization/protoes/* が broken 状態。後続 Phase で順次修復。

---

## Phase 3: errors / serializer / event_store 改修

### Task 4: `v2/errors.go` から `ErrAggregateIDMismatch` 削除

Command が `AggregateID()` を持たないので、Repository.Save 内で id mismatch を検出する必要がない。

**Files:**
- Modify: `v2/errors.go`
- Modify: `v2/errors_test.go`

- [ ] **Step 1: `errors.go` から `ErrAggregateIDMismatch` 変数、`AggregateIDMismatchError` struct、`NewAggregateIDMismatchError` 関数を削除**
- [ ] **Step 2: `errors_test.go` から該当テストを削除**
- [ ] **Step 3: `cd v2 && go test -run TestErrors ./...` で他 sentinels の挙動確認**
- [ ] **Step 4: Commit**

```bash
git add v2/errors.go v2/errors_test.go
git commit -m "refactor(v2/errors): drop ErrAggregateIDMismatch (Command no longer has AggregateID)"
```

### Task 5: `v2/serializer.go` を分離型 interface に改修

**Files:**
- Modify: `v2/serializer.go`
- Delete: `v2/serializer_test.go`

target：
```go
type AggregateSerializer[T Aggregate[E], E Event] interface {
    Serialize(T) ([]byte, error)
    Deserialize([]byte) (T, error)
}

type EventSerializer[E Event] interface {
    Serialize(E) ([]byte, error)
    Deserialize(typeName string, data []byte) (E, error)
}
```

JSONEventSerializer は Envelope 移行後は不要なので削除。

- [ ] **Step 1: serializer.go を target 内容に書き換え**
- [ ] **Step 2: serializer_test.go を削除（テスト対象消滅）**
- [ ] **Step 3: build 確認**
- [ ] **Step 4: Commit**

```bash
git add v2/serializer.go v2/serializer_test.go
git commit -m "refactor(v2): split AggregateSerializer[T,E] and EventSerializer[E]; drop JSONEventSerializer"
```

### Task 6: `v2/event_store.go` を Envelope ベースに改修

**Files:**
- Modify: `v2/event_store.go`
- Modify: `v2/event_store_test.go`

target：
```go
type EventStore interface {
    GetLatestSnapshot(ctx context.Context, id AggregateID) (*SnapshotEnvelope, error)
    GetEventsSince(ctx context.Context, id AggregateID, seqNr uint64) ([]*EventEnvelope, error)
    PersistEvent(ctx context.Context, ev *EventEnvelope, expectedVersion uint64) error
    PersistEventAndSnapshot(ctx context.Context, ev *EventEnvelope, snap *SnapshotEnvelope) error
}

type Config struct {
    SnapshotInterval uint64 // 0 disables snapshots
}

func DefaultConfig() Config { return Config{SnapshotInterval: 5} }
func (c Config) ShouldSnapshot(seqNr uint64) bool {
    if c.SnapshotInterval == 0 { return false }
    return seqNr % c.SnapshotInterval == 0
}
```

- [ ] **Step 1: event_store.go を target 内容に書き換え（generic 削除、Envelope ベース、Config から DynamoDB フィールド削除）**
- [ ] **Step 2: event_store_test.go の Config テストを修正**
- [ ] **Step 3: build はまだ通らない（memory/dynamodb が broken）**
- [ ] **Step 4: Commit**

```bash
git add v2/event_store.go v2/event_store_test.go
git commit -m "refactor(v2): make EventStore envelope-based; trim Config to SnapshotInterval"
```

### Task 7: `v2/eventid.go` 新規追加

**Files:**
- Create: `v2/eventid.go`

```go
package eventstore

import (
	"crypto/rand"
	"encoding/hex"
)

func generateEventID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
```

- [ ] **Step 1: 作成**
- [ ] **Step 2: `v2/eventid_test.go` で format / uniqueness テスト**

```go
package eventstore

import (
	"regexp"
	"testing"
)

func TestGenerateEventID_format(t *testing.T) {
	id, err := generateEventID()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
		t.Errorf("unexpected: %q", id)
	}
}
func TestGenerateEventID_unique(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 1000; i++ {
		id, _ := generateEventID()
		if _, dup := seen[id]; dup {
			t.Fatalf("dup: %s", id)
		}
		seen[id] = struct{}{}
	}
}
```

- [ ] **Step 3: Commit**

```bash
git add v2/eventid.go v2/eventid_test.go
git commit -m "feat(v2): add internal generateEventID helper"
```

---

## Phase 4: Memory store 改修

### Task 8: `v2/memory/store.go` を Envelope ベースに改修

**Files:**
- Modify: `v2/memory/store.go`
- Modify: `v2/memory/store_test.go`

generic 削除、`map[string][]*EventEnvelope` + `map[string]*SnapshotEnvelope`。楽観ロックは snapshot.Version で。

- [ ] **Step 1: store.go を Envelope ベースに書き換え**
- [ ] **Step 2: store_test.go を新 fixture (typed AggregateID 直接定義) に書き換え**
- [ ] **Step 3: `cd v2 && go test ./memory/...` 通過**
- [ ] **Step 4: Commit**

```bash
git add v2/memory/
git commit -m "refactor(v2/memory): convert to envelope-based EventStore; drop generic"
```

---

## Phase 5: DynamoDB store 改修

### Task 9: `v2/dynamodb/store.go` を Envelope ベースに改修

**Files:**
- Modify: `v2/dynamodb/store.go`
- Modify: `v2/dynamodb/store_test.go`

generic 削除、`AggregateSerializer[T]` 引数削除（Repository が serialize 済み payload を渡す）。OTel propagation は `EventEnvelope.TraceParent / TraceState` に bind。

- [ ] **Step 1: store.go を書き換え（attribute marshal/unmarshal は EventEnvelope/SnapshotEnvelope を直接処理）**
- [ ] **Step 2: store_test.go を Envelope を直接渡すスタイルに書き換え**
- [ ] **Step 3: `cd v2 && go test ./dynamodb/...` 通過**
- [ ] **Step 4: Commit**

```bash
git add v2/dynamodb/
git commit -m "refactor(v2/dynamodb): convert to envelope-based EventStore; drop generic & AggregateSerializer arg"
```

---

## Phase 6: Repository 改修

### Task 10: `v2/repository.go` を `Save(aggID, cmd)` に refactor

**Files:**
- Modify: `v2/repository.go`
- Modify: `v2/repository_test.go`

target：
```go
type Repository[T Aggregate[E], E Event] interface {
    Load(ctx context.Context, aggID AggregateID) (T, error)
    Save(ctx context.Context, aggID AggregateID, cmd Command) (T, error)
}

type defaultRepository[T Aggregate[E], E Event] struct {
    store         EventStore
    createBlank   func(AggregateID) T
    aggSerializer AggregateSerializer[T, E]
    evSerializer  EventSerializer[E]
    config        Config
}

func NewRepository[T Aggregate[E], E Event](
    store EventStore,
    createBlank func(AggregateID) T,
    aggSer AggregateSerializer[T, E],
    evSer EventSerializer[E],
    config Config,
) Repository[T, E] {
    return &defaultRepository[T, E]{store, createBlank, aggSer, evSer, config}
}
```

Save の動作：
1. internal load (snapshot + events replay)。集約が無ければ blank で開始（Save では NotFound にしない）
2. ApplyCommand → ev (E)
3. ApplyEvent(ev) → next (T) （type assertion 内部）
4. Envelope 構築：EventID 生成 / OccurredAt = now / IsCreated = (currentSeqNr == 0) / Payload = evSer.Serialize(ev)
5. ShouldSnapshot 判定で PersistEvent / PersistEventAndSnapshot 分岐
6. snapshot 取得時は AggregateSerializer で next を Serialize、SnapshotEnvelope を渡す

- [ ] **Step 1: repository.go を書き換え**
- [ ] **Step 2: repository_test.go を全面書き換え（counter aggregate を pure struct に、独自 typed counterID、独自 Serializer 実装）**

   テストフィクスチャの基本形：

```go
type counterID struct{ value string }
func (c counterID) TypeName() string { return "Counter" }
func (c counterID) Value() string    { return c.value }
func (c counterID) AsString() string { return "Counter-" + c.value }

type counterEvent interface {
    es.Event
    isCounterEvent()
}
type incrementedEvent struct {
    aggID counterID
    by    int
}
func (e incrementedEvent) EventTypeName() string         { return "Incremented" }
func (e incrementedEvent) AggregateID() es.AggregateID   { return e.aggID }
func (incrementedEvent) isCounterEvent()                 {}

type incrementCommand struct{ By int }
func (incrementCommand) CommandTypeName() string { return "Increment" }

type counterAggregate struct {
    id    counterID
    count int
}
func (c counterAggregate) AggregateID() es.AggregateID { return c.id }
func (c counterAggregate) ApplyCommand(cmd es.Command) (counterEvent, error) {
    switch x := cmd.(type) {
    case incrementCommand:
        return incrementedEvent{aggID: c.id, by: x.By}, nil
    }
    return nil, es.ErrUnknownCommand
}
func (c counterAggregate) ApplyEvent(ev counterEvent) es.Aggregate[counterEvent] {
    if e, ok := ev.(incrementedEvent); ok {
        return counterAggregate{id: c.id, count: c.count + e.by}
    }
    return c
}
```

   Serializer 実装、newCounterRepo helper、tests for Load_notFound / Save_first / Save_subsequent / Save_triggersSnapshot / Save_optimisticLockFailure (lockFailingStore wrapper) / Save_propagatesApplyCommandError を網羅。

- [ ] **Step 3: `cd v2 && go test ./...` 全 PASS**
- [ ] **Step 4: Commit**

```bash
git add v2/repository.go v2/repository_test.go
git commit -m "refactor(v2): rebuild Repository[T,E] with Save(aggID,cmd) API"
```

---

## Phase 7: Internal package 整理

### Task 11: `v2/internal/aggregateid/` 削除

**Files:**
- Delete: `v2/internal/aggregateid/aggregateid.go`

利用側が独自 ID 型を定義する想定。library 内 deserialize は `simpleAggregateID` (envelope.go 内) で対応。

- [ ] **Step 1: ファイル削除**

```bash
rm -r v2/internal/aggregateid
```

- [ ] **Step 2: `v2/internal/keyresolver/key_resolver_test.go` から `aggregateid` import 削除（独自テスト用 ID 型を定義）**
- [ ] **Step 3: `cd v2 && go test ./internal/...` 通過**
- [ ] **Step 4: Commit**

```bash
git add -A v2/internal/
git commit -m "refactor(v2/internal): drop aggregateid package; tests define own typed IDs"
```

### Task 12: `v2/internal/envelope/` 削除

**Files:**
- Delete: `v2/internal/envelope/envelope.go`
- Delete: `v2/internal/envelope/envelope_test.go`

公開 EventEnvelope / SnapshotEnvelope に置き換え済み。

- [ ] **Step 1: ファイル削除**

```bash
rm -r v2/internal/envelope
```

- [ ] **Step 2: 参照箇所がないこと確認（`grep -r 'internal/envelope' v2/`）**
- [ ] **Step 3: build 通過**
- [ ] **Step 4: Commit**

```bash
git add -A v2/internal/
git commit -m "refactor(v2/internal): drop internal/envelope (replaced by public EventEnvelope/SnapshotEnvelope)"
```

---

## Phase 8: Protobuf serializer 追従

### Task 13: `v2/serialization/protoes/` を新 API に追従

**Files:**
- Modify: `v2/serialization/protoes/event.proto`
- Regenerate: `v2/serialization/protoes/event.pb.go`
- Modify: `v2/serialization/protoes/serializer.go`
- Modify: `v2/serialization/protoes/serializer_test.go`

新 API では `EventSerializer[E]` interface に追従。proto は domain Event の Payload を抽象的に保持する形で残す。

- [ ] **Step 1: `event.proto` を Envelope wire format に再設計（library 側の Envelope serialization を proto 化するイメージ）**

   ただし domain Event の field 型は domain ごとに違うので、proto 側は generic な byte payload で保持しつつ、利用側が自分の Event 型と proto の対応を `EventSerializer[E]` 実装で行う形にする。最も簡単なのは「ProtoEventSerializer は library が提供せず、利用側が独自に実装する」という方針だが、user が「新 API に追従して書き直す」と判断したので、参考実装を提供する。

   実装案（serializer.go）：

```go
// ProtoEventSerializer is a reference implementation of EventSerializer[E].
// E は利用側が定義する domain Event interface。
// ユーザーは encode/decode 関数を渡して E ↔ []byte 変換を実装する。
type ProtoEventSerializer[E es.Event] struct {
    encode func(E) ([]byte, error)
    decode func(typeName string, data []byte) (E, error)
}

func New[E es.Event](
    encode func(E) ([]byte, error),
    decode func(typeName string, data []byte) (E, error),
) *ProtoEventSerializer[E] {
    return &ProtoEventSerializer[E]{encode: encode, decode: decode}
}

func (s *ProtoEventSerializer[E]) Serialize(ev E) ([]byte, error) {
    return s.encode(ev)
}

func (s *ProtoEventSerializer[E]) Deserialize(typeName string, data []byte) (E, error) {
    return s.decode(typeName, data)
}
```

   実質的には `EventSerializer[E]` interface を関数で具現化する thin adapter。proto 固有の wire format は利用側の encode/decode 関数で扱う。

   .proto ファイルは sample として残すが、library レベルでは generic なので proto 依存を最小化。

- [ ] **Step 2: `serializer_test.go` を sample encode/decode で動作確認**
- [ ] **Step 3: Commit**

```bash
git add v2/serialization/protoes/
git commit -m "refactor(v2/serialization/protoes): align ProtoEventSerializer with EventSerializer[E] interface"
```

---

## Phase 9: ドキュメント

### Task 14: README v2 Quick Start 書き換え

**Files:**
- Modify: `README.md`

main の README は `BaseAggregate` 埋め込みパターンの例を載せている。新 API に追従。

- [ ] **Step 1: README の v2 セクションを新 API（pure interface 実装、Repository.Save(aggID, cmd) など）の例に置換**

   key changes セクションも更新：
   - `BaseAggregate / BaseEvent` の言及を削除
   - "ドメイン純度" を強調（Aggregate / Event / Command がメタ持たない）
   - `Repository.Save(aggID, cmd)` API 説明
   - `AggregateSerializer[T,E]` + `EventSerializer[E]` 分離説明
   - Quick Start サンプルを `counterID` / pure struct counter 形式に

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: rewrite v2 quick start for domain-purity API"
```

---

## Phase 10: 完了確認 & PR

### Task 15: 全 build / test / vet 通過確認

- [ ] **Step 1: v2**

```bash
cd v2 && go build ./... && go test ./... && go vet ./...
```

- [ ] **Step 2: v1**

```bash
cd /Users/sakemihiroshi/develop/terrat/eventstore && go build ./... && go test ./... && go vet ./...
```

- [ ] **Step 3: 公開 API 表面の確認**

```bash
cd /Users/sakemihiroshi/develop/terrat/eventstore/v2
grep -rE '^(type|func|var|const) [A-Z]' --include='*.go' --exclude='*_test.go' | sort
```

期待される public 表面（`v2/` root）：
- types: `AggregateID`, `Aggregate[E]`, `Event`, `Command`
- envelope: `EventEnvelope`, `SnapshotEnvelope`
- serializer: `AggregateSerializer[T,E]`, `EventSerializer[E]`
- event_store: `EventStore`, `Config`, `DefaultConfig`
- errors: 9 sentinels (Mismatch 削除) + 4 typed errors + constructors
- repository: `Repository[T,E]`, `NewRepository`

サブパッケージ：
- `memory.New() es.EventStore`
- `dynamodb.Client`, `dynamodb.Config`, `dynamodb.DefaultConfig`, `dynamodb.New() es.EventStore`
- `serialization/protoes.ProtoEventSerializer[E]`, `serialization/protoes.New[E]`

`Default*`, `Base*`, `JSONEventSerializer`, `internal/aggregateid`, `internal/envelope`, `Err{InvalidEvent, EventStoreUnavailable, AggregateIDMismatch}` は消滅（Err{InvalidEvent, EventStoreUnavailable} の削除可否は実装中に確認）。

### Task 16: PR#2 close + 新 PR 起こし

- [ ] **Step 1: ブランチ push**

```bash
git push -u origin feat/v2-domain-purity-v2
```

- [ ] **Step 2: PR 作成**

```bash
gh pr create --title "refactor(v2): apply domain purity revision on top of main" --body "..."
```

- [ ] **Step 3: 旧 PR#2 を close**

```bash
gh pr close 2 --comment "Replaced by #N (proper main-based delta)"
```

---

## 完了チェックリスト

- [ ] `cd v2 && go build ./...` 全 package エラーなし
- [ ] `cd v2 && go test ./...` 全 PASS
- [ ] `cd v2 && go vet ./...` clean
- [ ] v1 (`go test ./...`) 影響なし
- [ ] 公開 API 表面が target に一致
- [ ] README v2 Quick Start が新 API 反映
- [ ] PR#2 closed
- [ ] 新 PR 作成 + spec/plan へのリンク

---

## 注意事項

- **大量の compile error が一時的に発生する**：Phase 2 で interface 改修すると Phase 3-6 が完了するまで build 通らない。各 Phase 単位で完結させて commit すること
- **ジェネリクス制約 `T Aggregate[E], E Event`**：利用側の型注釈が冗長になりがち。Repository / Serializer の constructor 呼び出し時は `NewRepository[Visit, VisitEvent](...)` のように明示
- **typed AggregateID パターン**：spec §7.1 の例（`type CounterID struct { ... }` で `TypeName/Value/AsString` 自前実装）に従う。library から `NewAggregateID` 等のヘルパは提供しない
- **OTel context**：DynamoDB store の attribute marshal で `propagation.Inject` して `EventEnvelope.TraceParent/TraceState` に格納
- **楽観ロック**: snapshot.Version 一本。PersistEvent では実施しない（SeqNr 衝突チェックのみ）
