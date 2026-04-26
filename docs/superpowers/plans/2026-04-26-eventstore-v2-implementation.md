# eventstore v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** v2 サブモジュール (`github.com/Hiroshi0900/eventstore/v2`) として、Command 抽象 + 集約に責務を寄せた API を新規実装し、in-memory / DynamoDB の永続化バックエンドを TDD で構築する。

**Architecture:** v1 のコードはそのまま `// Deprecated:` 化して残し、`v2/` ディレクトリ配下に独立した Go モジュールを作成。core types (Aggregate / Command / Event) → memory store → Repository[T] → dynamodb store の順に TDD で積み上げ、最終的に v1 を Deprecated 化して README に v2 への移行ガイドを追記する。

**Tech Stack:** Go 1.25, AWS SDK Go v2 (DynamoDB), OpenTelemetry, 標準 `testing` パッケージ（assertion ヘルパは標準のみ、testify は使わない）

**スコープ外:** terrat-go-app の v2 移行（別計画）

---

## ファイル構造

### 新規作成 (v2/)
```
v2/
├── go.mod                       # module github.com/Hiroshi0900/eventstore/v2
├── go.sum
├── aggregate.go                 # AggregateID, DefaultAggregateID, Aggregate interface
├── aggregate_test.go
├── command.go                   # Command interface
├── event.go                     # Event interface, DefaultEvent, EventOption
├── event_test.go
├── errors.go                    # Sentinel + typed errors
├── errors_test.go
├── serializer.go                # Serializer interface, JSONSerializer
├── serializer_test.go
├── event_store.go               # EventStore interface, Config, SnapshotData
├── event_store_test.go          # Config.ShouldSnapshot テスト
├── repository.go                # Repository[T], AggregateSerializer[T]
├── repository_test.go           # Repository を memory store で end-to-end テスト
├── memory/
│   ├── store.go
│   └── store_test.go
├── dynamodb/
│   ├── store.go
│   └── store_test.go
└── internal/
    ├── keyresolver/
    │   ├── key_resolver.go
    │   └── key_resolver_test.go
    └── envelope/
        ├── envelope.go
        └── envelope_test.go
```

### v1 への変更（互換維持、Deprecated コメントのみ）
- `types.go`, `event_store.go`, `errors.go`, `serializer.go`, `memory/store.go`, `dynamodb/store.go` の公開シンボルに `// Deprecated:` コメント追記

### ドキュメント
- `README.md` に v2 セクション追加

---

## Phase 1: v2 サブモジュール初期化

### Task 1: v2 ディレクトリと go.mod 作成

**Files:**
- Create: `v2/go.mod`

- [ ] **Step 1: v2 ディレクトリを作成**

```bash
mkdir -p /Users/sakemihiroshi/develop/terrat/eventstore/v2
```

- [ ] **Step 2: v2/go.mod を作成**

`v2/go.mod`:
```
module github.com/Hiroshi0900/eventstore/v2

go 1.25
```

- [ ] **Step 3: 動作確認**

Run: `cd v2 && go mod tidy && cd ..`
Expected: エラーなし、`v2/go.sum` は作成されないか空（依存なしのため）

- [ ] **Step 4: Commit**

```bash
git add v2/go.mod
git commit -m "chore(v2): initialize v2 submodule"
```

---

## Phase 2: Core types (TDD)

### Task 2: AggregateID interface と DefaultAggregateID

**Files:**
- Create: `v2/aggregate.go`
- Create: `v2/aggregate_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`v2/aggregate_test.go`:
```go
package eventstore

import "testing"

func TestDefaultAggregateID_AsString(t *testing.T) {
	id := NewAggregateID("Visit", "abc-123")

	if got, want := id.TypeName(), "Visit"; got != want {
		t.Errorf("TypeName() = %q, want %q", got, want)
	}
	if got, want := id.Value(), "abc-123"; got != want {
		t.Errorf("Value() = %q, want %q", got, want)
	}
	if got, want := id.AsString(), "Visit-abc-123"; got != want {
		t.Errorf("AsString() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestDefaultAggregateID_AsString -v`
Expected: コンパイルエラー（`NewAggregateID undefined` 等）

- [ ] **Step 3: 最小実装**

`v2/aggregate.go`:
```go
// Package eventstore は Event Sourcing を実現するためのライブラリです (v2)。
package eventstore

import "fmt"

// AggregateID は集約の識別子を表します。
type AggregateID interface {
	TypeName() string
	Value() string
	AsString() string
}

// DefaultAggregateID は AggregateID のデフォルト実装です。
type DefaultAggregateID struct {
	typeName string
	value    string
}

// NewAggregateID は DefaultAggregateID を生成します。
func NewAggregateID(typeName, value string) DefaultAggregateID {
	return DefaultAggregateID{typeName: typeName, value: value}
}

func (id DefaultAggregateID) TypeName() string { return id.typeName }
func (id DefaultAggregateID) Value() string    { return id.value }
func (id DefaultAggregateID) AsString() string {
	return fmt.Sprintf("%s-%s", id.typeName, id.value)
}

// Aggregate は Event Sourcing における集約ルートを表します。
// 状態遷移時に異なる具象型を返せるよう、ApplyEvent の戻り値は interface です。
type Aggregate interface {
	AggregateID() AggregateID
	SeqNr() uint64
	Version() uint64

	ApplyCommand(Command) (Event, error)
	ApplyEvent(Event) Aggregate

	WithVersion(uint64) Aggregate
	WithSeqNr(uint64) Aggregate
}
```

- [ ] **Step 4: テスト成功を確認**

Run: `cd v2 && go test -run TestDefaultAggregateID -v`
Expected: コンパイルエラー（Command, Event がまだない）→ Task 3, 4 で解消されるので **このタスクのコミットは Task 4 後**

- [ ] **Step 5: コミットは Task 4 完了後にまとめて行う（先送り）**

---

### Task 3: Event interface と DefaultEvent

**Files:**
- Create: `v2/event.go`
- Create: `v2/event_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`v2/event_test.go`:
```go
package eventstore

import (
	"testing"
	"time"
)

func TestNewEvent_Defaults(t *testing.T) {
	id := NewAggregateID("Visit", "v1")
	ev := NewEvent("evt-1", "VisitScheduled", id, []byte("payload"))

	if got, want := ev.EventID(), "evt-1"; got != want {
		t.Errorf("EventID() = %q, want %q", got, want)
	}
	if got, want := ev.EventTypeName(), "VisitScheduled"; got != want {
		t.Errorf("EventTypeName() = %q, want %q", got, want)
	}
	if got, want := ev.AggregateID().AsString(), "Visit-v1"; got != want {
		t.Errorf("AggregateID() = %q, want %q", got, want)
	}
	if got, want := ev.SeqNr(), uint64(1); got != want {
		t.Errorf("SeqNr() = %d, want %d", got, want)
	}
	if ev.IsCreated() {
		t.Error("IsCreated() = true, want false (default)")
	}
	if got, want := string(ev.Payload()), "payload"; got != want {
		t.Errorf("Payload() = %q, want %q", got, want)
	}
	if ev.OccurredAt().IsZero() {
		t.Error("OccurredAt() should not be zero by default")
	}
}

func TestNewEvent_WithOptions(t *testing.T) {
	id := NewAggregateID("Visit", "v1")
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := NewEvent("evt-1", "VisitScheduled", id, nil,
		WithSeqNr(42),
		WithIsCreated(true),
		WithOccurredAt(ts),
	)

	if got, want := ev.SeqNr(), uint64(42); got != want {
		t.Errorf("SeqNr() = %d, want %d", got, want)
	}
	if !ev.IsCreated() {
		t.Error("IsCreated() = false, want true")
	}
	if !ev.OccurredAt().Equal(ts) {
		t.Errorf("OccurredAt() = %v, want %v", ev.OccurredAt(), ts)
	}
}

func TestDefaultEvent_WithSeqNr(t *testing.T) {
	id := NewAggregateID("Visit", "v1")
	ev := NewEvent("evt-1", "VisitScheduled", id, nil, WithSeqNr(1))

	updated := ev.WithSeqNr(5)
	if got, want := updated.SeqNr(), uint64(5); got != want {
		t.Errorf("WithSeqNr(5).SeqNr() = %d, want %d", got, want)
	}
	if got, want := ev.SeqNr(), uint64(1); got != want {
		t.Errorf("original SeqNr() should be unchanged: got %d, want %d", got, want)
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestNewEvent -v`
Expected: コンパイルエラー（`NewEvent undefined` 等）

- [ ] **Step 3: 最小実装**

`v2/event.go`:
```go
package eventstore

import "time"

// Event は過去に発生した不変のドメインイベントを表します。
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

// EventOption は Event の関数オプションです。
type EventOption func(*DefaultEvent)

// WithSeqNr はシーケンス番号をセットします。
func WithSeqNr(seqNr uint64) EventOption {
	return func(e *DefaultEvent) { e.seqNr = seqNr }
}

// WithIsCreated は IsCreated フラグをセットします。
func WithIsCreated(isCreated bool) EventOption {
	return func(e *DefaultEvent) { e.isCreated = isCreated }
}

// WithOccurredAt は発生時刻をセットします。
func WithOccurredAt(t time.Time) EventOption {
	return func(e *DefaultEvent) { e.occurredAt = t }
}

// DefaultEvent は Event のデフォルト実装です。
type DefaultEvent struct {
	eventID     string
	typeName    string
	aggregateID AggregateID
	seqNr       uint64
	isCreated   bool
	occurredAt  time.Time
	payload     []byte
}

// NewEvent は DefaultEvent を生成します。
func NewEvent(
	eventID string,
	typeName string,
	aggregateID AggregateID,
	payload []byte,
	opts ...EventOption,
) DefaultEvent {
	e := DefaultEvent{
		eventID:     eventID,
		typeName:    typeName,
		aggregateID: aggregateID,
		seqNr:       1,
		isCreated:   false,
		occurredAt:  time.Now(),
		payload:     payload,
	}
	for _, opt := range opts {
		opt(&e)
	}
	return e
}

func (e DefaultEvent) EventID() string           { return e.eventID }
func (e DefaultEvent) EventTypeName() string     { return e.typeName }
func (e DefaultEvent) AggregateID() AggregateID  { return e.aggregateID }
func (e DefaultEvent) SeqNr() uint64             { return e.seqNr }
func (e DefaultEvent) IsCreated() bool           { return e.isCreated }
func (e DefaultEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e DefaultEvent) Payload() []byte           { return e.payload }

// WithSeqNr は新しい SeqNr を持つコピーを返します（不変性）。
func (e DefaultEvent) WithSeqNr(seqNr uint64) Event {
	e.seqNr = seqNr
	return e
}
```

- [ ] **Step 4: テスト成功を確認**

Run: `cd v2 && go test -run TestNewEvent -v && cd v2 && go test -run TestDefaultEvent -v`
Expected: コンパイルエラー（Command がまだない）→ Task 4 で解消

---

### Task 4: Command interface

**Files:**
- Create: `v2/command.go`

- [ ] **Step 1: Command interface を定義**

`v2/command.go`:
```go
package eventstore

// Command はドメインの意図を表します。Repository.Store でターゲット集約に適用されます。
type Command interface {
	CommandTypeName() string
	AggregateID() AggregateID
}
```

- [ ] **Step 2: コンパイルと既存テストの実行**

Run: `cd v2 && go build ./... && go test ./...`
Expected: コンパイル成功、Task 2 と Task 3 のテストが PASS

- [ ] **Step 3: Commit (Task 2-4 をまとめて)**

```bash
git add v2/aggregate.go v2/aggregate_test.go v2/event.go v2/event_test.go v2/command.go
git commit -m "feat(v2): add core types (AggregateID, Event, Command, Aggregate)"
```

---

### Task 5: Errors

**Files:**
- Create: `v2/errors.go`
- Create: `v2/errors_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`v2/errors_test.go`:
```go
package eventstore

import (
	"errors"
	"testing"
)

func TestOptimisticLockError_IsErrOptimisticLock(t *testing.T) {
	err := NewOptimisticLockError("Visit-v1", 5, 3)
	if !errors.Is(err, ErrOptimisticLock) {
		t.Errorf("expected errors.Is(err, ErrOptimisticLock) = true")
	}
}

func TestAggregateNotFoundError_IsErrAggregateNotFound(t *testing.T) {
	err := NewAggregateNotFoundError("Visit", "v1")
	if !errors.Is(err, ErrAggregateNotFound) {
		t.Errorf("expected errors.Is(err, ErrAggregateNotFound) = true")
	}
}

func TestSerializationError_IsErrSerializationFailed(t *testing.T) {
	cause := errors.New("boom")
	err := NewSerializationError("event", cause)
	if !errors.Is(err, ErrSerializationFailed) {
		t.Errorf("expected errors.Is(err, ErrSerializationFailed) = true")
	}
	if !errors.Is(err, cause) {
		t.Errorf("expected unwrap chain to include cause")
	}
}

func TestDeserializationError_IsErrDeserializationFailed(t *testing.T) {
	err := NewDeserializationError("snapshot", errors.New("bad"))
	if !errors.Is(err, ErrDeserializationFailed) {
		t.Errorf("expected errors.Is(err, ErrDeserializationFailed) = true")
	}
}

func TestDuplicateAggregateError_IsErrDuplicateAggregate(t *testing.T) {
	err := NewDuplicateAggregateError("Visit-v1")
	if !errors.Is(err, ErrDuplicateAggregate) {
		t.Errorf("expected errors.Is(err, ErrDuplicateAggregate) = true")
	}
}

func TestAggregateIDMismatchError_IsErrAggregateIDMismatch(t *testing.T) {
	err := NewAggregateIDMismatchError("Visit-v1", "Visit-v2")
	if !errors.Is(err, ErrAggregateIDMismatch) {
		t.Errorf("expected errors.Is(err, ErrAggregateIDMismatch) = true")
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestOptimisticLockError -v`
Expected: コンパイルエラー

- [ ] **Step 3: 最小実装**

`v2/errors.go`:
```go
package eventstore

import (
	"errors"
	"fmt"
)

// Sentinel errors.
var (
	ErrOptimisticLock        = errors.New("optimistic lock error: concurrent modification detected")
	ErrAggregateNotFound     = errors.New("aggregate not found")
	ErrSerializationFailed   = errors.New("serialization failed")
	ErrDeserializationFailed = errors.New("deserialization failed")
	ErrInvalidEvent          = errors.New("invalid event")
	ErrInvalidAggregate      = errors.New("invalid aggregate")
	ErrEventStoreUnavailable = errors.New("event store unavailable")
	ErrDuplicateAggregate    = errors.New("aggregate already exists")
	ErrUnknownCommand        = errors.New("unknown command type for current state")
	ErrAggregateIDMismatch   = errors.New("command aggregate ID does not match aggregate")
)

// OptimisticLockError は楽観ロック失敗の詳細を持ちます。
type OptimisticLockError struct {
	AggregateID     string
	ExpectedVersion uint64
	ActualVersion   uint64
}

func (e *OptimisticLockError) Error() string {
	return fmt.Sprintf("optimistic lock error: aggregate %s expected version %d but got %d",
		e.AggregateID, e.ExpectedVersion, e.ActualVersion)
}
func (e *OptimisticLockError) Is(target error) bool { return target == ErrOptimisticLock }

func NewOptimisticLockError(aggregateID string, expected, actual uint64) *OptimisticLockError {
	return &OptimisticLockError{AggregateID: aggregateID, ExpectedVersion: expected, ActualVersion: actual}
}

// AggregateNotFoundError は集約が存在しないことを示します。
type AggregateNotFoundError struct {
	TypeName    string
	AggregateID string
}

func (e *AggregateNotFoundError) Error() string {
	return fmt.Sprintf("aggregate not found: type=%s, id=%s", e.TypeName, e.AggregateID)
}
func (e *AggregateNotFoundError) Is(target error) bool { return target == ErrAggregateNotFound }

func NewAggregateNotFoundError(typeName, aggregateID string) *AggregateNotFoundError {
	return &AggregateNotFoundError{TypeName: typeName, AggregateID: aggregateID}
}

// SerializationError はシリアライゼーション/デシリアライゼーション失敗を表します。
type SerializationError struct {
	Operation string // "serialize" or "deserialize"
	Target    string // "event" / "aggregate" / "snapshot"
	Cause     error
}

func (e *SerializationError) Error() string {
	return fmt.Sprintf("failed to %s %s: %v", e.Operation, e.Target, e.Cause)
}
func (e *SerializationError) Unwrap() error { return e.Cause }
func (e *SerializationError) Is(target error) bool {
	if e.Operation == "serialize" {
		return target == ErrSerializationFailed
	}
	return target == ErrDeserializationFailed
}

func NewSerializationError(target string, cause error) *SerializationError {
	return &SerializationError{Operation: "serialize", Target: target, Cause: cause}
}

func NewDeserializationError(target string, cause error) *SerializationError {
	return &SerializationError{Operation: "deserialize", Target: target, Cause: cause}
}

// DuplicateAggregateError は同じ ID の集約が既に存在することを示します。
type DuplicateAggregateError struct {
	AggregateID string
}

func (e *DuplicateAggregateError) Error() string {
	return fmt.Sprintf("aggregate already exists: id=%s", e.AggregateID)
}
func (e *DuplicateAggregateError) Is(target error) bool { return target == ErrDuplicateAggregate }

func NewDuplicateAggregateError(aggregateID string) *DuplicateAggregateError {
	return &DuplicateAggregateError{AggregateID: aggregateID}
}

// AggregateIDMismatchError は Command の AggregateID が Aggregate と一致しないときに返されます。
type AggregateIDMismatchError struct {
	CommandAggregateID   string
	AggregateAggregateID string
}

func (e *AggregateIDMismatchError) Error() string {
	return fmt.Sprintf("command aggregate ID %q does not match aggregate %q",
		e.CommandAggregateID, e.AggregateAggregateID)
}
func (e *AggregateIDMismatchError) Is(target error) bool { return target == ErrAggregateIDMismatch }

func NewAggregateIDMismatchError(cmdID, aggID string) *AggregateIDMismatchError {
	return &AggregateIDMismatchError{CommandAggregateID: cmdID, AggregateAggregateID: aggID}
}
```

- [ ] **Step 4: テスト成功を確認**

Run: `cd v2 && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/errors.go v2/errors_test.go
git commit -m "feat(v2): add error types"
```

---

## Phase 3: Serializer

### Task 6: Serializer interface と JSONSerializer

**Files:**
- Create: `v2/serializer.go`
- Create: `v2/serializer_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`v2/serializer_test.go`:
```go
package eventstore

import (
	"errors"
	"testing"
)

type testPayload struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestJSONSerializer_RoundTrip(t *testing.T) {
	s := NewJSONSerializer()
	src := testPayload{Name: "alice", Age: 30}

	data, err := s.SerializeEvent(src)
	if err != nil {
		t.Fatalf("SerializeEvent err: %v", err)
	}

	var got testPayload
	if err := s.DeserializeEvent(data, &got); err != nil {
		t.Fatalf("DeserializeEvent err: %v", err)
	}
	if got != src {
		t.Errorf("round trip mismatch: got=%+v, want=%+v", got, src)
	}
}

func TestJSONSerializer_DeserializeError(t *testing.T) {
	s := NewJSONSerializer()
	var dst testPayload
	err := s.DeserializeEvent([]byte("{invalid"), &dst)
	if !errors.Is(err, ErrDeserializationFailed) {
		t.Errorf("expected ErrDeserializationFailed, got %v", err)
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestJSONSerializer -v`
Expected: コンパイルエラー

- [ ] **Step 3: 最小実装**

`v2/serializer.go`:
```go
package eventstore

import "encoding/json"

// Serializer は汎用シリアライザの抽象です。Event payload と Aggregate snapshot の両方を扱います。
type Serializer interface {
	SerializeEvent(payload any) ([]byte, error)
	DeserializeEvent(data []byte, target any) error
	SerializeAggregate(aggregate any) ([]byte, error)
	DeserializeAggregate(data []byte, target any) error
}

// JSONSerializer は JSON を使う Serializer 実装です。
type JSONSerializer struct{}

func NewJSONSerializer() *JSONSerializer { return &JSONSerializer{} }

func (s *JSONSerializer) SerializeEvent(payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, NewSerializationError("event", err)
	}
	return data, nil
}

func (s *JSONSerializer) DeserializeEvent(data []byte, target any) error {
	if err := json.Unmarshal(data, target); err != nil {
		return NewDeserializationError("event", err)
	}
	return nil
}

func (s *JSONSerializer) SerializeAggregate(aggregate any) ([]byte, error) {
	data, err := json.Marshal(aggregate)
	if err != nil {
		return nil, NewSerializationError("aggregate", err)
	}
	return data, nil
}

func (s *JSONSerializer) DeserializeAggregate(data []byte, target any) error {
	if err := json.Unmarshal(data, target); err != nil {
		return NewDeserializationError("aggregate", err)
	}
	return nil
}
```

- [ ] **Step 4: テスト成功を確認**

Run: `cd v2 && go test -run TestJSONSerializer -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/serializer.go v2/serializer_test.go
git commit -m "feat(v2): add Serializer and JSONSerializer"
```

---

## Phase 4: EventStore interface と関連型

### Task 7: EventStore interface, Config, SnapshotData, AggregateSerializer

**Files:**
- Create: `v2/event_store.go`
- Create: `v2/event_store_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`v2/event_store_test.go`:
```go
package eventstore

import "testing"

func TestDefaultConfig_Defaults(t *testing.T) {
	c := DefaultConfig()
	if got, want := c.SnapshotInterval, uint64(5); got != want {
		t.Errorf("SnapshotInterval = %d, want %d", got, want)
	}
}

func TestConfig_ShouldSnapshot(t *testing.T) {
	cases := []struct {
		interval uint64
		seqNr    uint64
		want     bool
	}{
		{interval: 5, seqNr: 0, want: true},  // 0 % 5 == 0
		{interval: 5, seqNr: 1, want: false},
		{interval: 5, seqNr: 5, want: true},
		{interval: 5, seqNr: 10, want: true},
		{interval: 0, seqNr: 5, want: false}, // disabled
		{interval: 1, seqNr: 1, want: true},
	}
	for _, tc := range cases {
		c := Config{SnapshotInterval: tc.interval}
		if got := c.ShouldSnapshot(tc.seqNr); got != tc.want {
			t.Errorf("ShouldSnapshot(interval=%d, seq=%d) = %v, want %v",
				tc.interval, tc.seqNr, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestDefaultConfig -v`
Expected: コンパイルエラー

- [ ] **Step 3: 最小実装**

`v2/event_store.go`:
```go
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
```

- [ ] **Step 4: テスト成功を確認**

Run: `cd v2 && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/event_store.go v2/event_store_test.go
git commit -m "feat(v2): add EventStore interface, Config, SnapshotData, AggregateSerializer"
```

---

## Phase 5: memory store (TDD)

### Task 8: memory.Store の skeleton

**Files:**
- Create: `v2/memory/store.go`
- Create: `v2/memory/store_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`v2/memory/store_test.go`:
```go
package memory

import (
	"context"
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
)

func TestNew_Empty(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}

	id := es.NewAggregateID("Visit", "v1")
	snap, err := s.GetLatestSnapshotByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshotByID err: %v", err)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot, got %+v", snap)
	}

	events, err := s.GetEventsByIDSinceSeqNr(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsByIDSinceSeqNr err: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty events, got %d", len(events))
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test ./memory/... -v`
Expected: コンパイルエラー

- [ ] **Step 3: 最小実装**

`v2/memory/store.go`:
```go
// Package memory は in-memory な EventStore 実装を提供します（テスト用途）。
package memory

import (
	"context"
	"sort"
	"sync"

	es "github.com/Hiroshi0900/eventstore/v2"
)

// Store は in-memory な EventStore 実装です。
type Store struct {
	mu        sync.RWMutex
	events    map[string][]es.Event       // aggregateID -> events
	snapshots map[string]*es.SnapshotData // aggregateID -> latest snapshot
}

// New は空の Store を生成します。
func New() *Store {
	return &Store{
		events:    map[string][]es.Event{},
		snapshots: map[string]*es.SnapshotData{},
	}
}

// GetLatestSnapshotByID は集約の最新スナップショットを返します。存在しなければ nil。
func (s *Store) GetLatestSnapshotByID(_ context.Context, id es.AggregateID) (*es.SnapshotData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if snap, ok := s.snapshots[id.AsString()]; ok {
		return snap, nil
	}
	return nil, nil
}

// GetEventsByIDSinceSeqNr は seqNr より大きい seqNr のイベントを昇順で返します。
func (s *Store) GetEventsByIDSinceSeqNr(_ context.Context, id es.AggregateID, seqNr uint64) ([]es.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := s.events[id.AsString()]
	if len(all) == 0 {
		return nil, nil
	}
	out := make([]es.Event, 0, len(all))
	for _, ev := range all {
		if ev.SeqNr() > seqNr {
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SeqNr() < out[j].SeqNr() })
	return out, nil
}

// Clear はストア内のすべてのデータを消去します（テスト用途）。
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = map[string][]es.Event{}
	s.snapshots = map[string]*es.SnapshotData{}
}
```

- [ ] **Step 4: テスト成功を確認**

Run: `cd v2 && go test ./memory/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/memory/
git commit -m "feat(v2/memory): add Store skeleton with read methods"
```

---

### Task 9: memory.Store.PersistEvent

**Files:**
- Modify: `v2/memory/store.go`
- Modify: `v2/memory/store_test.go`

- [ ] **Step 1: テスト追加**

`v2/memory/store_test.go` に追記:
```go
func TestStore_PersistEvent_FirstEvent(t *testing.T) {
	s := New()
	id := es.NewAggregateID("Visit", "v1")
	ev := es.NewEvent("evt-1", "VisitScheduled", id, []byte("p"),
		es.WithSeqNr(1), es.WithIsCreated(true))

	if err := s.PersistEvent(context.Background(), ev, 0); err != nil {
		t.Fatalf("PersistEvent err: %v", err)
	}

	got, err := s.GetEventsByIDSinceSeqNr(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsByIDSinceSeqNr err: %v", err)
	}
	if len(got) != 1 || got[0].EventID() != "evt-1" {
		t.Errorf("expected single event evt-1, got %+v", got)
	}
}

func TestStore_PersistEvent_DuplicateOnCreate(t *testing.T) {
	s := New()
	id := es.NewAggregateID("Visit", "v1")
	ev := es.NewEvent("evt-1", "VisitScheduled", id, nil, es.WithSeqNr(1))

	if err := s.PersistEvent(context.Background(), ev, 0); err != nil {
		t.Fatalf("first PersistEvent err: %v", err)
	}
	err := s.PersistEvent(context.Background(), ev, 0)
	if !errors.Is(err, es.ErrDuplicateAggregate) {
		t.Errorf("expected ErrDuplicateAggregate, got %v", err)
	}
}

func TestStore_PersistEvent_DuplicateSeqNr(t *testing.T) {
	s := New()
	id := es.NewAggregateID("Visit", "v1")
	ev1 := es.NewEvent("evt-1", "VisitScheduled", id, nil, es.WithSeqNr(1))
	ev2 := es.NewEvent("evt-2", "VisitCompleted", id, nil, es.WithSeqNr(1)) // 同じ SeqNr

	if err := s.PersistEvent(context.Background(), ev1, 0); err != nil {
		t.Fatalf("ev1 err: %v", err)
	}
	// 同じ SeqNr のイベントを書き込もうとすると ErrDuplicateAggregate
	err := s.PersistEvent(context.Background(), ev2, 1)
	if !errors.Is(err, es.ErrDuplicateAggregate) {
		t.Errorf("expected ErrDuplicateAggregate, got %v", err)
	}
}
```

`v2/memory/store_test.go` の import に `"errors"` を追加。

> **設計メモ**: `PersistEvent` の `version` 引数は **「初回作成識別」専用**として扱う（version==0 で既存ありなら Duplicate、それ以外は無視）。楽観的ロックは `PersistEventAndSnapshot` 経由の snapshot.Version でのみ行う。これは v1 DynamoDB の挙動と整合する。

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test ./memory/... -v`
Expected: コンパイルエラー（`PersistEvent undefined`）

- [ ] **Step 3: 実装**

`v2/memory/store.go` に追記:
```go
// PersistEvent はイベントを保存します。
// 初回イベント (version == 0) で既存があれば ErrDuplicateAggregate。
// 同じ SeqNr のイベントが既に存在する場合も ErrDuplicateAggregate。
// version 引数は初回識別以外には使用しない（v1 DynamoDB と意味論を揃えるため）。
// 楽観的ロックは PersistEventAndSnapshot の snapshot.Version で行う。
func (s *Store) PersistEvent(_ context.Context, event es.Event, version uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := event.AggregateID().AsString()

	// 初回作成の整合性チェック
	if version == 0 && len(s.events[key]) > 0 {
		return es.NewDuplicateAggregateError(key)
	}

	// 同 SeqNr 衝突チェック
	for _, existing := range s.events[key] {
		if existing.SeqNr() == event.SeqNr() {
			return es.NewDuplicateAggregateError(key)
		}
	}

	s.events[key] = append(s.events[key], event)
	return nil
}
```

> versions マップは PersistEvent では更新しない。snapshot.Version の管理は PersistEventAndSnapshot に集約。

- [ ] **Step 4: テスト成功を確認**

Run: `cd v2 && go test ./memory/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/memory/
git commit -m "feat(v2/memory): add PersistEvent with optimistic lock"
```

---

### Task 10: memory.Store.PersistEventAndSnapshot

**Files:**
- Modify: `v2/memory/store.go`
- Modify: `v2/memory/store_test.go`

- [ ] **Step 1: テスト追加**

`v2/memory/store_test.go` に追記:
```go
func TestStore_PersistEventAndSnapshot_First(t *testing.T) {
	s := New()
	id := es.NewAggregateID("Visit", "v1")
	ev := es.NewEvent("evt-1", "VisitScheduled", id, nil,
		es.WithSeqNr(1), es.WithIsCreated(true))
	snap := es.SnapshotData{Payload: []byte("snap-1"), SeqNr: 1, Version: 1}

	if err := s.PersistEventAndSnapshot(context.Background(), ev, snap); err != nil {
		t.Fatalf("PersistEventAndSnapshot err: %v", err)
	}

	got, err := s.GetLatestSnapshotByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshotByID err: %v", err)
	}
	if got == nil {
		t.Fatal("snapshot is nil")
	}
	if string(got.Payload) != "snap-1" || got.SeqNr != 1 || got.Version != 1 {
		t.Errorf("snapshot mismatch: %+v", got)
	}

	events, _ := s.GetEventsByIDSinceSeqNr(context.Background(), id, 0)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestStore_PersistEventAndSnapshot_OptimisticLock(t *testing.T) {
	s := New()
	id := es.NewAggregateID("Visit", "v1")
	ev1 := es.NewEvent("evt-1", "VisitScheduled", id, nil, es.WithSeqNr(1))
	snap1 := es.SnapshotData{Payload: []byte("v1"), SeqNr: 1, Version: 1}
	if err := s.PersistEventAndSnapshot(context.Background(), ev1, snap1); err != nil {
		t.Fatalf("first err: %v", err)
	}

	ev2 := es.NewEvent("evt-2", "VisitCompleted", id, nil, es.WithSeqNr(2))
	snap2 := es.SnapshotData{Payload: []byte("v2"), SeqNr: 2, Version: 99} // ズレた version
	err := s.PersistEventAndSnapshot(context.Background(), ev2, snap2)
	if !errors.Is(err, es.ErrOptimisticLock) {
		t.Errorf("expected ErrOptimisticLock, got %v", err)
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test ./memory/... -v`
Expected: コンパイルエラー（`PersistEventAndSnapshot undefined`）

- [ ] **Step 3: 実装**

`v2/memory/store.go` に追記:
```go
// PersistEventAndSnapshot はイベントとスナップショットをアトミックに保存します。
// 楽観的ロック: 既存 snapshot の Version + 1 が snapshot.Version と一致しない場合 ErrOptimisticLock。
// snapshot がない場合は snapshot.Version == 1 を期待。
func (s *Store) PersistEventAndSnapshot(_ context.Context, event es.Event, snapshot es.SnapshotData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := event.AggregateID().AsString()

	// 同 SeqNr 衝突チェック (PersistEvent と同じ整合性)
	for _, existing := range s.events[key] {
		if existing.SeqNr() == event.SeqNr() {
			return es.NewDuplicateAggregateError(key)
		}
	}

	// snapshot 世代ベースの楽観ロック
	var currentSnapVersion uint64
	if existing, ok := s.snapshots[key]; ok {
		currentSnapVersion = existing.Version
	}
	expected := currentSnapVersion + 1
	if snapshot.Version != expected {
		return es.NewOptimisticLockError(key, expected, snapshot.Version)
	}

	s.events[key] = append(s.events[key], event)
	s.snapshots[key] = &snapshot
	return nil
}
```

> Store struct から `versions` マップを削除する。snapshot version は `snapshots[key].Version` から取得する（情報の二重管理を避ける）。

- [ ] **Step 4: テスト成功を確認**

Run: `cd v2 && go test ./memory/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/memory/
git commit -m "feat(v2/memory): add PersistEventAndSnapshot"
```

---

## Phase 6: Repository[T] (TDD)

このフェーズではテスト用のミニ集約を導入し、`Repository[T]` の各シナリオを TDD で実装する。

### Task 11: テスト用集約 (counterAggregate) を `repository_test.go` に定義

**Files:**
- Create: `v2/repository_test.go`

- [ ] **Step 1: テスト用集約と Command/Event を定義（外部テストパッケージで循環参照を回避）**

`v2/repository_test.go`:
```go
package eventstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/memory"
)

// --- テスト用ミニ集約: 単純なカウンタ ---

type counter struct {
	id      es.AggregateID
	value   int
	seqNr   uint64
	version uint64
}

func newCounter(id es.AggregateID) counter { return counter{id: id} }

func (c counter) AggregateID() es.AggregateID         { return c.id }
func (c counter) SeqNr() uint64                       { return c.seqNr }
func (c counter) Version() uint64                     { return c.version }
func (c counter) WithVersion(v uint64) es.Aggregate   { c.version = v; return c }
func (c counter) WithSeqNr(s uint64) es.Aggregate     { c.seqNr = s; return c }

func (c counter) ApplyCommand(cmd es.Command) (es.Event, error) {
	switch cmd.(type) {
	case incrementCmd:
		return es.NewEvent("evt-"+cmd.AggregateID().AsString(), "Incremented", cmd.AggregateID(), nil,
			es.WithSeqNr(c.seqNr+1), es.WithIsCreated(c.seqNr == 0)), nil
	default:
		return nil, es.ErrUnknownCommand
	}
}

func (c counter) ApplyEvent(ev es.Event) es.Aggregate {
	switch ev.EventTypeName() {
	case "Incremented":
		c.value++
		c.seqNr = ev.SeqNr()
	}
	return c
}

type incrementCmd struct{ id es.AggregateID }

func (c incrementCmd) CommandTypeName() string    { return "Increment" }
func (c incrementCmd) AggregateID() es.AggregateID { return c.id }

// --- AggregateSerializer ---

type counterSnapshot struct {
	Value   int    `json:"value"`
	SeqNr   uint64 `json:"seq_nr"`
	Version uint64 `json:"version"`
	IDType  string `json:"id_type"`
	IDValue string `json:"id_value"`
}

type counterSerializer struct{}

func (counterSerializer) Serialize(c counter) ([]byte, error) {
	return json.Marshal(counterSnapshot{
		Value: c.value, SeqNr: c.seqNr, Version: c.version,
		IDType: c.id.TypeName(), IDValue: c.id.Value(),
	})
}

func (counterSerializer) Deserialize(data []byte) (counter, error) {
	var s counterSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return counter{}, err
	}
	return counter{
		id:      es.NewAggregateID(s.IDType, s.IDValue),
		value:   s.Value,
		seqNr:   s.SeqNr,
		version: s.Version,
	}, nil
}

// --- 共通ヘルパ ---

func newTestRepo() *es.Repository[counter] {
	return es.NewRepository[counter](
		memory.New(),
		func(id es.AggregateID) counter { return newCounter(id) },
		counterSerializer{},
		es.DefaultConfig(),
	)
}

// プレースホルダテスト（次タスクで本格テストを追加）
func TestRepository_Construct(t *testing.T) {
	r := newTestRepo()
	if r == nil {
		t.Fatal("nil repo")
	}
	_ = context.Background
	_ = errors.Is
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestRepository_Construct -v`
Expected: コンパイルエラー（`Repository undefined`、`NewRepository undefined`）

- [ ] **Step 3: Repository skeleton を作成**

`v2/repository.go`:
```go
package eventstore

// Repository[T] は集約 T の高レベルロード/保存 API です。
type Repository[T Aggregate] struct {
	store       EventStore
	createBlank func(AggregateID) T
	serializer  AggregateSerializer[T]
	config      Config
}

// NewRepository は Repository[T] を生成します。
func NewRepository[T Aggregate](
	store EventStore,
	createBlank func(AggregateID) T,
	serializer AggregateSerializer[T],
	config Config,
) *Repository[T] {
	return &Repository[T]{
		store:       store,
		createBlank: createBlank,
		serializer:  serializer,
		config:      config,
	}
}
```

- [ ] **Step 4: テスト成功を確認**

Run: `cd v2 && go test -run TestRepository_Construct -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/repository.go v2/repository_test.go
git commit -m "feat(v2): scaffold Repository[T] with test counter aggregate"
```

---

### Task 12: Repository.Load - 集約が存在しない場合 ErrAggregateNotFound

**Files:**
- Modify: `v2/repository.go`
- Modify: `v2/repository_test.go`

- [ ] **Step 1: テスト追加**

`v2/repository_test.go` の末尾に追記:
```go
func TestRepository_Load_NotFound(t *testing.T) {
	r := newTestRepo()
	id := es.NewAggregateID("Counter", "c1")

	_, err := r.Load(context.Background(), id)
	if !errors.Is(err, es.ErrAggregateNotFound) {
		t.Errorf("expected ErrAggregateNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestRepository_Load_NotFound -v`
Expected: コンパイルエラー（`Load undefined`）

- [ ] **Step 3: 実装**

`v2/repository.go` に追記:
```go
import "context"

// Load は集約をロードします。スナップショットがあれば起点に、なければ空集約から
// イベントを replay。スナップショットもイベントもない場合は ErrAggregateNotFound。
func (r *Repository[T]) Load(ctx context.Context, id AggregateID) (T, error) {
	var zero T

	snap, err := r.store.GetLatestSnapshotByID(ctx, id)
	if err != nil {
		return zero, err
	}

	var agg T
	var seqNr uint64
	if snap != nil {
		restored, err := r.serializer.Deserialize(snap.Payload)
		if err != nil {
			return zero, err
		}
		// version をスナップショットのものに復元
		agg = restored.WithVersion(snap.Version).(T)
		seqNr = snap.SeqNr
	} else {
		agg = r.createBlank(id)
		seqNr = 0
	}

	events, err := r.store.GetEventsByIDSinceSeqNr(ctx, id, seqNr)
	if err != nil {
		return zero, err
	}

	if snap == nil && len(events) == 0 {
		return zero, NewAggregateNotFoundError(id.TypeName(), id.Value())
	}

	for _, ev := range events {
		agg = agg.ApplyEvent(ev).(T)
	}
	return agg, nil
}
```

注: Go の import 文は通常先頭に固める。`v2/repository.go` は最終的にこのような形になる:
```go
package eventstore

import "context"

// (Repository struct と NewRepository、Load を順に配置)
```

- [ ] **Step 4: テスト成功を確認**

Run: `cd v2 && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/repository.go v2/repository_test.go
git commit -m "feat(v2): add Repository.Load with ErrAggregateNotFound"
```

---

### Task 13: Repository.Store - 単純な保存（スナップショットなし）

**Files:**
- Modify: `v2/repository.go`
- Modify: `v2/repository_test.go`

- [ ] **Step 1: テスト追加**

`v2/repository_test.go` の末尾に追記:
```go
func TestRepository_Store_FirstCommand(t *testing.T) {
	r := newTestRepo()
	id := es.NewAggregateID("Counter", "c1")
	c := newCounter(id)

	updated, err := r.Store(context.Background(), incrementCmd{id: id}, c)
	if err != nil {
		t.Fatalf("Store err: %v", err)
	}
	if got, want := updated.value, 1; got != want {
		t.Errorf("counter.value = %d, want %d", got, want)
	}
	if got, want := updated.SeqNr(), uint64(1); got != want {
		t.Errorf("counter.SeqNr = %d, want %d", got, want)
	}
}

func TestRepository_Store_MismatchedAggregateID(t *testing.T) {
	r := newTestRepo()
	idA := es.NewAggregateID("Counter", "a")
	idB := es.NewAggregateID("Counter", "b")
	c := newCounter(idA)

	_, err := r.Store(context.Background(), incrementCmd{id: idB}, c)
	if !errors.Is(err, es.ErrAggregateIDMismatch) {
		t.Errorf("expected ErrAggregateIDMismatch, got %v", err)
	}
}

func TestRepository_LoadAfterStore(t *testing.T) {
	r := newTestRepo()
	id := es.NewAggregateID("Counter", "c1")
	c := newCounter(id)

	// 3 回 Increment （SnapshotInterval=5 なのでスナップショットは取られない）
	for i := 0; i < 3; i++ {
		var err error
		c, err = r.Store(context.Background(), incrementCmd{id: id}, c)
		if err != nil {
			t.Fatalf("Store err at i=%d: %v", i, err)
		}
	}

	loaded, err := r.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load err: %v", err)
	}
	if got, want := loaded.value, 3; got != want {
		t.Errorf("loaded.value = %d, want %d", got, want)
	}
	if got, want := loaded.SeqNr(), uint64(3); got != want {
		t.Errorf("loaded.SeqNr = %d, want %d", got, want)
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestRepository_Store -v`
Expected: コンパイルエラー（`Store undefined`）

- [ ] **Step 3: 実装**

`v2/repository.go` に追記:
```go
// Store はコマンドを集約に適用してイベントを生成・永続化し、新しい集約を返します。
func (r *Repository[T]) Store(ctx context.Context, cmd Command, agg T) (T, error) {
	var zero T

	if cmd.AggregateID().AsString() != agg.AggregateID().AsString() {
		return zero, NewAggregateIDMismatchError(
			cmd.AggregateID().AsString(),
			agg.AggregateID().AsString(),
		)
	}

	ev, err := agg.ApplyCommand(cmd)
	if err != nil {
		return zero, err
	}

	nextSeqNr := agg.SeqNr() + 1
	ev = ev.WithSeqNr(nextSeqNr)
	nextAgg := agg.ApplyEvent(ev).(T)
	nextAgg = nextAgg.WithSeqNr(nextSeqNr).(T)

	if r.config.ShouldSnapshot(nextSeqNr) {
		payload, err := r.serializer.Serialize(nextAgg)
		if err != nil {
			return zero, err
		}
		nextVersion := agg.Version() + 1
		snap := SnapshotData{Payload: payload, SeqNr: nextSeqNr, Version: nextVersion}
		if err := r.store.PersistEventAndSnapshot(ctx, ev, snap); err != nil {
			return zero, err
		}
		nextAgg = nextAgg.WithVersion(nextVersion).(T)
	} else {
		if err := r.store.PersistEvent(ctx, ev, agg.Version()); err != nil {
			return zero, err
		}
	}

	return nextAgg, nil
}
```

- [ ] **Step 4: テスト成功を確認**

Run: `cd v2 && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/repository.go v2/repository_test.go
git commit -m "feat(v2): add Repository.Store (no-snapshot path)"
```

---

### Task 14: Repository.Store - スナップショット間隔到達時

**Files:**
- Modify: `v2/repository_test.go`

- [ ] **Step 1: テスト追加**

`v2/repository_test.go` の末尾に追記:
```go
func TestRepository_Store_TakesSnapshot(t *testing.T) {
	store := memory.New()
	r := es.NewRepository[counter](
		store,
		func(id es.AggregateID) counter { return newCounter(id) },
		counterSerializer{},
		es.Config{SnapshotInterval: 3},
	)
	id := es.NewAggregateID("Counter", "c1")
	c := newCounter(id)

	for i := 0; i < 3; i++ {
		var err error
		c, err = r.Store(context.Background(), incrementCmd{id: id}, c)
		if err != nil {
			t.Fatalf("Store err at i=%d: %v", i, err)
		}
	}

	// SeqNr=3 でスナップショットが取られているはず
	snap, err := store.GetLatestSnapshotByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshotByID err: %v", err)
	}
	if snap == nil {
		t.Fatal("expected snapshot to be saved at seqNr=3")
	}
	if snap.SeqNr != 3 {
		t.Errorf("snap.SeqNr = %d, want 3", snap.SeqNr)
	}
	if snap.Version != 1 {
		t.Errorf("snap.Version = %d, want 1", snap.Version)
	}

	// Load して状態が正しく復元されること
	loaded, err := r.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load err: %v", err)
	}
	if loaded.value != 3 {
		t.Errorf("loaded.value = %d, want 3", loaded.value)
	}
}
```

- [ ] **Step 2: テスト実行**

Run: `cd v2 && go test -run TestRepository_Store_TakesSnapshot -v`
Expected: PASS（既に Task 13 の実装でカバー済み）

- [ ] **Step 3: Commit**

```bash
git add v2/repository_test.go
git commit -m "test(v2): add Repository snapshot interval test"
```

---

### Task 15: Repository.Store - 楽観ロック失敗

**Files:**
- Modify: `v2/repository_test.go`

- [ ] **Step 1: テスト追加**

`v2/repository_test.go` の末尾に追記:
```go
func TestRepository_Store_OptimisticLockOnSnapshot(t *testing.T) {
	store := memory.New()
	r := es.NewRepository[counter](
		store,
		func(id es.AggregateID) counter { return newCounter(id) },
		counterSerializer{},
		es.Config{SnapshotInterval: 1}, // 毎回スナップショット
	)
	id := es.NewAggregateID("Counter", "c1")
	c := newCounter(id)

	// 1 回目は成功
	c, err := r.Store(context.Background(), incrementCmd{id: id}, c)
	if err != nil {
		t.Fatalf("first Store err: %v", err)
	}

	// stale: version=1 のローカルコピー
	stale := c
	// 別経路でもう一度進めて store の version を 2 にする
	c, err = r.Store(context.Background(), incrementCmd{id: id}, c)
	if err != nil {
		t.Fatalf("second Store err: %v", err)
	}
	// stale から保存しようとすると、store の current=2 に対して expected=2 を要求することになり失敗
	_, err = r.Store(context.Background(), incrementCmd{id: id}, stale)
	if !errors.Is(err, es.ErrOptimisticLock) {
		t.Errorf("expected ErrOptimisticLock, got %v", err)
	}
}
```

- [ ] **Step 2: テスト実行**

Run: `cd v2 && go test -run TestRepository_Store_OptimisticLockOnSnapshot -v`
Expected: PASS（Task 13 の実装でカバー済み）

- [ ] **Step 3: Commit**

```bash
git add v2/repository_test.go
git commit -m "test(v2): add Repository optimistic lock test"
```

---

## Phase 7: internal packages 移植

### Task 16: internal/keyresolver を v1 から移植

**Files:**
- Create: `v2/internal/keyresolver/key_resolver.go`
- Create: `v2/internal/keyresolver/key_resolver_test.go`

- [ ] **Step 1: v1 のファイルを v2 にコピー**

```bash
cp /Users/sakemihiroshi/develop/terrat/eventstore/internal/keyresolver/key_resolver.go \
   /Users/sakemihiroshi/develop/terrat/eventstore/v2/internal/keyresolver/key_resolver.go

cp /Users/sakemihiroshi/develop/terrat/eventstore/internal/keyresolver/key_resolver_test.go \
   /Users/sakemihiroshi/develop/terrat/eventstore/v2/internal/keyresolver/key_resolver_test.go
```

- [ ] **Step 2: import パスを v2 用に書き換え**

`v2/internal/keyresolver/key_resolver.go`:
- `es "github.com/Hiroshi0900/eventstore"` → `es "github.com/Hiroshi0900/eventstore/v2"`

`v2/internal/keyresolver/key_resolver_test.go`:
- 同様に書き換え

加えて、v1 の `id.GetTypeName()` / `id.GetValue()` / `id.AsString()` 呼び出しを v2 のメソッド名 `id.TypeName()` / `id.Value()` / `id.AsString()` に変更。

具体的には `v2/internal/keyresolver/key_resolver.go` の以下の行を変更:
```go
// Before (v1):
return fmt.Sprintf("%s-%d", id.GetTypeName(), shardID)
return fmt.Sprintf("%s-%s-%020d", id.GetTypeName(), id.GetValue(), seqNr)
return fmt.Sprintf("%s-%s-0", id.GetTypeName(), id.GetValue())

// After (v2):
return fmt.Sprintf("%s-%d", id.TypeName(), shardID)
return fmt.Sprintf("%s-%s-%020d", id.TypeName(), id.Value(), seqNr)
return fmt.Sprintf("%s-%s-0", id.TypeName(), id.Value())

// computeShardID は同じく id.Value() を使うように
```

`v2/internal/keyresolver/key_resolver_test.go` 内のテストでも `NewAggregateId` → `NewAggregateID` に変更（v2 では大文字 ID）。

- [ ] **Step 3: テスト実行**

Run: `cd v2 && go test ./internal/keyresolver/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add v2/internal/keyresolver/
git commit -m "feat(v2): port internal/keyresolver"
```

---

### Task 17: internal/envelope を v1 から移植

**Files:**
- Create: `v2/internal/envelope/envelope.go`
- Create: `v2/internal/envelope/envelope_test.go`

- [ ] **Step 1: v1 のファイルをコピー**

```bash
cp /Users/sakemihiroshi/develop/terrat/eventstore/internal/envelope/envelope.go \
   /Users/sakemihiroshi/develop/terrat/eventstore/v2/internal/envelope/envelope.go

cp /Users/sakemihiroshi/develop/terrat/eventstore/internal/envelope/envelope_test.go \
   /Users/sakemihiroshi/develop/terrat/eventstore/v2/internal/envelope/envelope_test.go
```

- [ ] **Step 2: import パスとメソッド名を v2 用に書き換え**

`v2/internal/envelope/envelope.go`:
```go
package envelope

import (
	es "github.com/Hiroshi0900/eventstore/v2"
)

type EventEnvelope struct {
	ID          string `json:"id"`
	TypeName    string `json:"type_name"`
	AggregateID string `json:"aggregate_id"`
	SeqNr       uint64 `json:"seq_nr"`
	IsCreated   bool   `json:"is_created"`
	OccurredAt  int64  `json:"occurred_at"` // Unix milli
	Payload     []byte `json:"payload"`
}

func FromEvent(e es.Event) *EventEnvelope {
	return &EventEnvelope{
		ID:          e.EventID(),
		TypeName:    e.EventTypeName(),
		AggregateID: e.AggregateID().AsString(),
		SeqNr:       e.SeqNr(),
		IsCreated:   e.IsCreated(),
		OccurredAt:  e.OccurredAt().UnixMilli(),
		Payload:     e.Payload(),
	}
}

type SnapshotEnvelope struct {
	AggregateID string `json:"aggregate_id"`
	TypeName    string `json:"type_name"`
	SeqNr       uint64 `json:"seq_nr"`
	Version     uint64 `json:"version"`
	Payload     []byte `json:"payload"`
}
```

`v2/internal/envelope/envelope_test.go` も上記 API 変更に追従。

- [ ] **Step 3: テスト実行**

Run: `cd v2 && go test ./internal/envelope/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add v2/internal/envelope/
git commit -m "feat(v2): port internal/envelope"
```

---

## Phase 8: dynamodb store 移植 + シグネチャ変更対応

### Task 18: dynamodb.Store の skeleton と依存追加

**Files:**
- Modify: `v2/go.mod`
- Create: `v2/dynamodb/store.go` (skeleton 部分のみ)

- [ ] **Step 1: 依存追加**

```bash
cd v2 && go get github.com/aws/aws-sdk-go-v2 \
  github.com/aws/aws-sdk-go-v2/service/dynamodb \
  go.opentelemetry.io/otel
```

- [ ] **Step 2: v1 から store.go をコピー**

```bash
cp /Users/sakemihiroshi/develop/terrat/eventstore/dynamodb/store.go \
   /Users/sakemihiroshi/develop/terrat/eventstore/v2/dynamodb/store.go
```

- [ ] **Step 3: v2 用に修正**

`v2/dynamodb/store.go` のヘッダ部:
```go
// Package dynamodb provides a DynamoDB-backed EventStore implementation (v2).
package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/internal/keyresolver"
)
```

メソッド名変更を全体に適用:
- `id.GetTypeName()` → `id.TypeName()`
- `id.GetValue()` → `id.Value()`
- `event.GetID()` → `event.EventID()`
- `event.GetTypeName()` → `event.EventTypeName()`
- `event.GetAggregateId()` → `event.AggregateID()`
- `event.GetSeqNr()` → `event.SeqNr()`
- `event.GetPayload()` → `event.Payload()`
- `event.GetOccurredAt()` → `event.OccurredAt().UnixMilli()` （`uint64` → `time.Time` 化に対応）
- `aggregate.GetVersion()` → `aggregate.Version()`
- `es.NewEvent(...)` の `WithOccurredAtUnixMilli(occurredAt)` を `WithOccurredAt(time.UnixMilli(int64(occurredAt)))` に置換

- [ ] **Step 4: ビルド確認**

Run: `cd v2 && go build ./dynamodb/...`
Expected: コンパイル成功（`PersistEventAndSnapshot` の signature mismatch エラーが出る → 次タスクで修正）

NOTE: 一旦コンパイルエラーが残ってもこのタスクではここまでとし、Task 19 で signature 変更を完成させる。

- [ ] **Step 5: ビルド失敗が PersistEventAndSnapshot シグネチャ問題に絞られていることを確認した上で commit**

```bash
git add v2/go.mod v2/go.sum v2/dynamodb/store.go
git commit -m "feat(v2/dynamodb): port store skeleton, method names updated"
```

---

### Task 19: dynamodb.Store.PersistEventAndSnapshot シグネチャ変更

**Files:**
- Modify: `v2/dynamodb/store.go`

- [ ] **Step 1: シグネチャを `(ctx, event, snapshot SnapshotData)` に変更**

`v2/dynamodb/store.go` 内の `PersistEventAndSnapshot` メソッドを以下に置換:
```go
// PersistEventAndSnapshot atomically persists an event and a snapshot.
// snapshot.Payload は呼び出し側 (Repository) が事前に作成済みであることを期待する。
func (s *Store) PersistEventAndSnapshot(ctx context.Context, event es.Event, snapshot es.SnapshotData) error {
	eventKeys := s.keyResolver.ResolveEventKeys(event.AggregateID(), event.SeqNr())
	snapshotKeys := s.keyResolver.ResolveSnapshotKeys(event.AggregateID())

	eventItem, err := s.marshalEvent(ctx, event, eventKeys)
	if err != nil {
		return err
	}

	snapshotItem := s.marshalSnapshot(snapshot, snapshotKeys)

	expectedVersion := snapshot.Version - 1 // 期待する現状 version

	transactItems := []types.TransactWriteItem{
		{
			Put: &types.Put{
				TableName:           aws.String(s.config.JournalTableName),
				Item:                eventItem,
				ConditionExpression: aws.String("attribute_not_exists(#pk)"),
				ExpressionAttributeNames: map[string]string{"#pk": colPKey},
			},
		},
	}

	if expectedVersion == 0 {
		// 初回: snapshot 行が存在しないことを condition に
		transactItems = append(transactItems, types.TransactWriteItem{
			Put: &types.Put{
				TableName:           aws.String(s.config.SnapshotTableName),
				Item:                snapshotItem,
				ConditionExpression: aws.String("attribute_not_exists(#pk)"),
				ExpressionAttributeNames: map[string]string{"#pk": colPKey},
			},
		})
	} else {
		transactItems = append(transactItems, types.TransactWriteItem{
			Put: &types.Put{
				TableName:           aws.String(s.config.SnapshotTableName),
				Item:                snapshotItem,
				ConditionExpression: aws.String("#version = :expected_version"),
				ExpressionAttributeNames: map[string]string{"#version": colVersion},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":expected_version": &types.AttributeValueMemberN{Value: strconv.FormatUint(expectedVersion, 10)},
				},
			},
		})
	}

	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transactItems,
	})
	if err != nil {
		if isTransactionCanceledDueToCondition(err) {
			return es.NewOptimisticLockError(
				event.AggregateID().AsString(),
				expectedVersion,
				0, // 実際の version は不明
			)
		}
		return fmt.Errorf("failed to persist event and snapshot: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: marshalSnapshot を SnapshotData ベースに変更**

`v2/dynamodb/store.go` 内の `marshalSnapshot` メソッドを以下に置換:
```go
// marshalSnapshot marshals SnapshotData into a DynamoDB item.
func (s *Store) marshalSnapshot(snapshot es.SnapshotData, keys keyresolver.KeyComponents) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		colPKey:    &types.AttributeValueMemberS{Value: keys.PartitionKey},
		colSKey:    &types.AttributeValueMemberS{Value: keys.SortKey},
		colAid:     &types.AttributeValueMemberS{Value: keys.AggregateIDKey},
		colSeqNr:   &types.AttributeValueMemberN{Value: strconv.FormatUint(snapshot.SeqNr, 10)},
		colVersion: &types.AttributeValueMemberN{Value: strconv.FormatUint(snapshot.Version, 10)},
		colPayload: &types.AttributeValueMemberB{Value: snapshot.Payload},
	}
}
```

- [ ] **Step 3: ビルド確認**

Run: `cd v2 && go build ./...`
Expected: コンパイル成功

- [ ] **Step 4: Commit**

```bash
git add v2/dynamodb/store.go
git commit -m "feat(v2/dynamodb): change PersistEventAndSnapshot signature to use SnapshotData"
```

---

### Task 20: dynamodb.Store の最低限のテスト（モック使用）

**Files:**
- Create: `v2/dynamodb/store_test.go`

- [ ] **Step 1: フェイク Client を作って構築のみテスト**

`v2/dynamodb/store_test.go`:
```go
package dynamodb

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type fakeClient struct {
	getItem            func(ctx context.Context, in *dynamodb.GetItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	query              func(ctx context.Context, in *dynamodb.QueryInput, opts ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	putItem            func(ctx context.Context, in *dynamodb.PutItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	transactWriteItems func(ctx context.Context, in *dynamodb.TransactWriteItemsInput, opts ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	createTable        func(ctx context.Context, in *dynamodb.CreateTableInput, opts ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error)
	describeTable      func(ctx context.Context, in *dynamodb.DescribeTableInput, opts ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
}

func (f *fakeClient) GetItem(ctx context.Context, in *dynamodb.GetItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if f.getItem != nil { return f.getItem(ctx, in, opts...) }
	return &dynamodb.GetItemOutput{}, nil
}
func (f *fakeClient) Query(ctx context.Context, in *dynamodb.QueryInput, opts ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if f.query != nil { return f.query(ctx, in, opts...) }
	return &dynamodb.QueryOutput{}, nil
}
func (f *fakeClient) PutItem(ctx context.Context, in *dynamodb.PutItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if f.putItem != nil { return f.putItem(ctx, in, opts...) }
	return &dynamodb.PutItemOutput{}, nil
}
func (f *fakeClient) TransactWriteItems(ctx context.Context, in *dynamodb.TransactWriteItemsInput, opts ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	if f.transactWriteItems != nil { return f.transactWriteItems(ctx, in, opts...) }
	return &dynamodb.TransactWriteItemsOutput{}, nil
}
func (f *fakeClient) CreateTable(ctx context.Context, in *dynamodb.CreateTableInput, opts ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error) {
	if f.createTable != nil { return f.createTable(ctx, in, opts...) }
	return &dynamodb.CreateTableOutput{}, nil
}
func (f *fakeClient) DescribeTable(ctx context.Context, in *dynamodb.DescribeTableInput, opts ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	if f.describeTable != nil { return f.describeTable(ctx, in, opts...) }
	return &dynamodb.DescribeTableOutput{}, nil
}

func TestNew_Construct(t *testing.T) {
	s := New(&fakeClient{}, DefaultConfig())
	if s == nil {
		t.Fatal("nil store")
	}
}
```

- [ ] **Step 2: テスト実行**

Run: `cd v2 && go test ./dynamodb/... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add v2/dynamodb/store_test.go
git commit -m "test(v2/dynamodb): add construction smoke test with fake client"
```

NOTE: 完全な DynamoDB integration test は別途 dynamodb-local を使った integration スイートで対応。本計画では skeleton + 単体ロジックの確認に留める。

---

## Phase 9: v1 を Deprecated 化

### Task 21: v1 公開シンボルに Deprecated コメント追加

**Files:**
- Modify: `types.go`
- Modify: `event_store.go`
- Modify: `errors.go`
- Modify: `serializer.go`
- Modify: `memory/store.go`
- Modify: `dynamodb/store.go`

- [ ] **Step 1: types.go の各公開型/関数に Deprecated コメント追記**

`types.go` の各公開シンボル（`AggregateID`, `DefaultAggregateID`, `NewAggregateId`, `Event`, `EventOption`, `WithSeqNr`, `WithIsCreated`, `WithOccurredAt`, `WithOccurredAtUnixMilli`, `DefaultEvent`, `NewEvent`, `Aggregate`, `AggregateResult`, `SnapshotData`）の doc コメント末尾に下記を追加。例：

```go
// AggregateID represents the unique identifier of an aggregate.
// ...
//
// Deprecated: Use github.com/Hiroshi0900/eventstore/v2 instead.
type AggregateID interface {
	...
}
```

同様に他の公開シンボルにも追記。

- [ ] **Step 2: 同じ作業を残りのファイルにも適用**

`event_store.go`, `errors.go`, `serializer.go`, `memory/store.go`, `dynamodb/store.go` の公開シンボルに同じコメント。

- [ ] **Step 3: ビルド・テスト実行**

Run: `go build ./... && go test ./...`
Expected: PASS（v1 のテストはそのまま動作）

- [ ] **Step 4: Commit**

```bash
git add types.go event_store.go errors.go serializer.go memory/store.go dynamodb/store.go
git commit -m "deprecate(v1): mark all public symbols as Deprecated, point to v2"
```

---

## Phase 10: ドキュメント

### Task 22: README に v2 セクション追加

**Files:**
- Modify: `README.md`

- [ ] **Step 1: README に v2 セクション追記**

`README.md` の末尾（License の前）に以下のセクションを追加:

```markdown
## v2

A redesigned API is available at `github.com/Hiroshi0900/eventstore/v2`. v2 introduces:

- `Command` abstraction (`Repository.Store(ctx, cmd, agg)`)
- `ApplyCommand` / `ApplyEvent` on the `Aggregate` interface
- State Pattern support: `ApplyEvent(Event) Aggregate` returns interface, allowing transitions to a different concrete type
- Removal of `AggregateFactory[T]` in favor of `createBlank` function + `AggregateSerializer[T]` interface
- Method names without `Get` prefix (`AggregateID()`, `EventID()`, `SeqNr()`, etc.)

v1 (this package root) remains available but is deprecated.

### Quick start (v2)

```go
import (
    es "github.com/Hiroshi0900/eventstore/v2"
    esmem "github.com/Hiroshi0900/eventstore/v2/memory"
)

// Define your domain interface and state types
type Visit interface {
    es.Aggregate
    IsActive() bool
}

type VisitScheduled struct { /* ... */ }

func (v VisitScheduled) AggregateID() es.AggregateID { /* ... */ }
func (v VisitScheduled) ApplyCommand(cmd es.Command) (es.Event, error) { /* ... */ }
func (v VisitScheduled) ApplyEvent(ev es.Event) es.Aggregate { /* ... */ }
// ...

// Construct repository
store := esmem.New()
repo := es.NewRepository[Visit](
    store,
    func(id es.AggregateID) Visit { return VisitScheduled{ /* ... */ } },
    visitSerializer{},
    es.DefaultConfig(),
)

// Use it
visit, err := repo.Load(ctx, visitID)
visit, err = repo.Store(ctx, RescheduleCommand{...}, visit)
```

See `docs/superpowers/specs/2026-04-26-eventstore-v2-design.md` for the full design.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add v2 section to README"
```

---

## 完了チェックリスト

すべてのタスクが完了した時点で以下を確認:

- [ ] `cd v2 && go vet ./...` がエラーなし
- [ ] `cd v2 && go test ./...` がすべて PASS
- [ ] `go vet ./...` (v1) がエラーなし
- [ ] `go test ./...` (v1) がすべて PASS
- [ ] v1 の公開シンボル全てに `// Deprecated:` コメントあり (`grep -r "// Deprecated:" --include="*.go" .` で確認)
- [ ] README.md に v2 セクションが存在
- [ ] git log で各 Phase ごとに段階的にコミットされている

---

## 注意事項

- **v1 のコードは変更しない**（Deprecated コメント以外）。v1 のテストは既存のまま動作する必要がある
- **v2 の internal package は v2 module の中だけで使う**。v1 の `internal/` を v2 から import してはいけない（モジュール境界を超えるため Go ビルダがエラーを出す）
- **DynamoDB の integration test は本計画外**。dynamodb-local を使う E2E テストは別途追加する想定
- **terrat-go-app の v2 移行は本計画外**。本計画完了後に別計画として着手する
