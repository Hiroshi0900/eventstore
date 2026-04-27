# eventstore v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** v2 サブモジュール (`github.com/Hiroshi0900/eventstore/v2`) として、ドメインからストレージのメタ情報 (SeqNr / Version / EventID / OccurredAt / IsCreated / Payload bytes) を完全排除し、Envelope 駆動で永続化する API を新規実装する。

**Architecture:** v1 のコードはそのまま `// Deprecated:` 化して残し、`v2/` ディレクトリ配下に独立した Go モジュールを作成。core types (Aggregate[E] / Event / Command) → Envelope → Serializer interfaces → memory store → Repository[T,E] → keyresolver → DynamoDB store の順に TDD で積み上げる。

**Tech Stack:** Go 1.25, AWS SDK Go v2 (DynamoDB), OpenTelemetry, 標準 `testing` パッケージ（assertion ヘルパは標準のみ、testify は使わない）。EventID 生成は `crypto/rand` + hex 16 bytes（外部依存追加なし）。

**スコープ外:** terrat-go-app の v2 移行（別計画）

---

## ファイル構造

### 新規作成 (v2/)

```
v2/
├── go.mod                       # module github.com/Hiroshi0900/eventstore/v2
├── go.sum
├── aggregate.go                 # AggregateID interface, DefaultAggregateID, Aggregate[E] interface
├── aggregate_test.go
├── event.go                     # Event interface
├── command.go                   # Command interface
├── envelope.go                  # EventEnvelope, SnapshotEnvelope (public types)
├── envelope_test.go
├── serializer.go                # AggregateSerializer[T,E], EventSerializer[E] interfaces
├── event_store.go               # EventStore interface, Config, ShouldSnapshot
├── event_store_test.go
├── errors.go                    # Sentinel + typed errors
├── errors_test.go
├── eventid.go                   # generateEventID() ヘルパ (crypto/rand + hex)
├── eventid_test.go
├── repository.go                # Repository[T,E], DefaultRepository, NewRepository
├── repository_test.go           # counter テスト用集約 + end-to-end テスト
├── memory/
│   ├── store.go
│   └── store_test.go
├── dynamodb/
│   ├── store.go
│   └── store_test.go
└── internal/
    └── keyresolver/
        ├── key_resolver.go
        └── key_resolver_test.go
```

設計判断：
- `EventEnvelope` / `SnapshotEnvelope` は **公開型**かつ **wire-format**を兼ねる（v1 の `internal/envelope` は v2 では不要）
- `internal/envelope` は v1 にしか存在せず、v2 では top-level の `envelope.go` で代替
- `memory/store.go` は in-memory 実装。テスト用とライブラリ品質の両立を目指す
- ULID ではなく `crypto/rand` + hex の理由：外部依存追加を避けるため。32-char hex の一意性は実用上 ULID 同等

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

## Phase 2: Core domain interfaces (TDD)

### Task 2: AggregateID interface と DefaultAggregateID

**Files:**
- Create: `v2/aggregate.go`
- Create: `v2/aggregate_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`v2/aggregate_test.go`:
```go
package eventsourcing_test

import (
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
)

func TestNewAggregateID(t *testing.T) {
	id := es.NewAggregateID("Visit", "01J0000000")

	if got := id.TypeName(); got != "Visit" {
		t.Errorf("TypeName(): got %q, want %q", got, "Visit")
	}
	if got := id.Value(); got != "01J0000000" {
		t.Errorf("Value(): got %q, want %q", got, "01J0000000")
	}
	if got := id.AsString(); got != "Visit-01J0000000" {
		t.Errorf("AsString(): got %q, want %q", got, "Visit-01J0000000")
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test ./...`
Expected: `undefined: es.NewAggregateID` などのコンパイルエラー

- [ ] **Step 3: 最小実装**

`v2/aggregate.go`:
```go
// Package eventsourcing v2 provides a lightweight library for event sourcing
// with domain types decoupled from storage metadata.
package eventsourcing

import "fmt"

// AggregateID represents the unique identifier of an aggregate.
type AggregateID interface {
	TypeName() string
	Value() string
	AsString() string
}

// DefaultAggregateID is the default implementation of AggregateID.
type DefaultAggregateID struct {
	typeName string
	value    string
}

// NewAggregateID creates a new DefaultAggregateID.
func NewAggregateID(typeName, value string) DefaultAggregateID {
	return DefaultAggregateID{typeName: typeName, value: value}
}

func (id DefaultAggregateID) TypeName() string { return id.typeName }
func (id DefaultAggregateID) Value() string    { return id.value }
func (id DefaultAggregateID) AsString() string {
	return fmt.Sprintf("%s-%s", id.typeName, id.value)
}
```

- [ ] **Step 4: テスト合格を確認**

Run: `cd v2 && go test -run TestNewAggregateID ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/aggregate.go v2/aggregate_test.go
git commit -m "feat(v2): add AggregateID interface and DefaultAggregateID"
```

---

### Task 3: Event interface

**Files:**
- Create: `v2/event.go`

- [ ] **Step 1: Event interface を定義**

`v2/event.go`:
```go
package eventsourcing

// Event represents a domain fact. Events carry only domain information;
// all storage metadata (SeqNr, EventID, OccurredAt, IsCreated, Payload bytes)
// is held by EventEnvelope at the library layer.
type Event interface {
	// EventTypeName returns the type discriminator used by EventSerializer
	// for dispatch on deserialization.
	EventTypeName() string

	// AggregateID returns the aggregate this event belongs to.
	AggregateID() AggregateID
}
```

- [ ] **Step 2: コンパイル確認**

Run: `cd v2 && go build ./...`
Expected: エラーなし

- [ ] **Step 3: Commit**

```bash
git add v2/event.go
git commit -m "feat(v2): add domain-pure Event interface"
```

---

### Task 4: Command interface

**Files:**
- Create: `v2/command.go`

- [ ] **Step 1: Command interface を定義**

`v2/command.go`:
```go
package eventsourcing

// Command represents an intent to mutate an aggregate.
// CommandTypeName is used for OTel span names, audit logs, and metrics.
// AggregateID is not part of Command because it is passed to Repository.Save explicitly.
type Command interface {
	CommandTypeName() string
}
```

- [ ] **Step 2: コンパイル確認**

Run: `cd v2 && go build ./...`
Expected: エラーなし

- [ ] **Step 3: Commit**

```bash
git add v2/command.go
git commit -m "feat(v2): add Command interface"
```

---

### Task 5: Aggregate[E] interface

**Files:**
- Modify: `v2/aggregate.go`
- Modify: `v2/aggregate_test.go`

- [ ] **Step 1: 失敗するテストを追加（mock aggregate でインタフェース契約を確認）**

`v2/aggregate_test.go` の末尾に追記：
```go
// mockEvent + mockAggregate は Aggregate[E] interface 契約を確認するためのテスト用 stub
type mockEvent struct {
	typeName string
	aggID    es.AggregateID
}

func (e mockEvent) EventTypeName() string       { return e.typeName }
func (e mockEvent) AggregateID() es.AggregateID { return e.aggID }

type mockCommand struct {
	name string
}

func (c mockCommand) CommandTypeName() string { return c.name }

type mockAggregate struct {
	id es.AggregateID
}

func (a mockAggregate) AggregateID() es.AggregateID { return a.id }

func (a mockAggregate) ApplyCommand(cmd es.Command) (mockEvent, error) {
	return mockEvent{typeName: cmd.CommandTypeName() + "Applied", aggID: a.id}, nil
}

func (a mockAggregate) ApplyEvent(_ mockEvent) es.Aggregate[mockEvent] {
	return a
}

func TestAggregateInterface_canBeImplementedWithStatePattern(t *testing.T) {
	id := es.NewAggregateID("Mock", "x")
	var agg es.Aggregate[mockEvent] = mockAggregate{id: id}

	ev, err := agg.ApplyCommand(mockCommand{name: "Do"})
	if err != nil {
		t.Fatalf("ApplyCommand() error = %v", err)
	}
	if got := ev.EventTypeName(); got != "DoApplied" {
		t.Errorf("event type: got %q, want %q", got, "DoApplied")
	}
	if got := ev.AggregateID().AsString(); got != "Mock-x" {
		t.Errorf("event aggID: got %q, want %q", got, "Mock-x")
	}

	next := agg.ApplyEvent(ev)
	if got := next.AggregateID().AsString(); got != "Mock-x" {
		t.Errorf("next aggID: got %q, want %q", got, "Mock-x")
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestAggregateInterface ./...`
Expected: `undefined: es.Aggregate` のコンパイルエラー

- [ ] **Step 3: Aggregate[E] interface を追加**

`v2/aggregate.go` の末尾に追記：
```go
// Aggregate is the root entity of an event-sourced state machine.
// E is the aggregate-specific Event type (e.g. VisitEvent).
//
// Aggregate carries no storage metadata: SeqNr, Version, EventID, OccurredAt
// are managed by Repository / EventEnvelope at the library layer.
type Aggregate[E Event] interface {
	// AggregateID returns the unique identifier of this aggregate.
	AggregateID() AggregateID

	// ApplyCommand validates a command against the current state and
	// produces a single domain Event. Returns ErrUnknownCommand if the
	// command is not applicable to the current state.
	ApplyCommand(Command) (E, error)

	// ApplyEvent applies a domain event and returns the next aggregate state.
	// The returned value may be a different concrete type (state pattern).
	ApplyEvent(E) Aggregate[E]
}
```

- [ ] **Step 4: テスト合格を確認**

Run: `cd v2 && go test -run TestAggregateInterface ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/aggregate.go v2/aggregate_test.go
git commit -m "feat(v2): add generic Aggregate[E] interface"
```

---

## Phase 3: Errors

### Task 6: Sentinel + typed errors

**Files:**
- Create: `v2/errors.go`
- Create: `v2/errors_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`v2/errors_test.go`:
```go
package eventsourcing_test

import (
	"errors"
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
)

func TestOptimisticLockError_IsErrOptimisticLock(t *testing.T) {
	err := es.NewOptimisticLockError("Visit-x", 3, 5)
	if !errors.Is(err, es.ErrOptimisticLock) {
		t.Errorf("errors.Is(err, ErrOptimisticLock) = false, want true")
	}
}

func TestAggregateNotFoundError_IsErrAggregateNotFound(t *testing.T) {
	err := es.NewAggregateNotFoundError("Visit", "x")
	if !errors.Is(err, es.ErrAggregateNotFound) {
		t.Errorf("errors.Is(err, ErrAggregateNotFound) = false, want true")
	}
}

func TestDuplicateAggregateError_IsErrDuplicateAggregate(t *testing.T) {
	err := es.NewDuplicateAggregateError("Visit-x")
	if !errors.Is(err, es.ErrDuplicateAggregate) {
		t.Errorf("errors.Is(err, ErrDuplicateAggregate) = false, want true")
	}
}

func TestSerializationError_IsErrSerializationFailed(t *testing.T) {
	err := es.NewSerializationError("event", errors.New("boom"))
	if !errors.Is(err, es.ErrSerializationFailed) {
		t.Errorf("errors.Is(err, ErrSerializationFailed) = false, want true")
	}
}

func TestDeserializationError_IsErrDeserializationFailed(t *testing.T) {
	err := es.NewDeserializationError("event", errors.New("boom"))
	if !errors.Is(err, es.ErrDeserializationFailed) {
		t.Errorf("errors.Is(err, ErrDeserializationFailed) = false, want true")
	}
}

func TestErrUnknownCommand_isSentinel(t *testing.T) {
	if es.ErrUnknownCommand == nil {
		t.Fatalf("ErrUnknownCommand should be non-nil sentinel")
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestOptimisticLockError ./...`
Expected: `undefined: es.ErrOptimisticLock` のコンパイルエラー

- [ ] **Step 3: 実装**

`v2/errors.go`:
```go
package eventsourcing

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
)

// OptimisticLockError provides detailed information about a lock conflict.
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

// AggregateNotFoundError provides information about which aggregate was not found.
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

// SerializationError provides details about a (de)serialization failure.
type SerializationError struct {
	Operation string // "serialize" or "deserialize"
	Target    string // "event", "aggregate"
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

// DuplicateAggregateError signals that an aggregate with the same ID already exists.
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
```

- [ ] **Step 4: テスト合格を確認**

Run: `cd v2 && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/errors.go v2/errors_test.go
git commit -m "feat(v2): add sentinel and typed errors"
```

---

## Phase 4: Envelopes

### Task 7: EventEnvelope と SnapshotEnvelope

**Files:**
- Create: `v2/envelope.go`
- Create: `v2/envelope_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`v2/envelope_test.go`:
```go
package eventsourcing_test

import (
	"encoding/json"
	"testing"
	"time"

	es "github.com/Hiroshi0900/eventstore/v2"
)

func TestEventEnvelope_jsonRoundtrip(t *testing.T) {
	occurred := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	in := es.EventEnvelope{
		EventID:       "ev-1",
		EventTypeName: "VisitScheduled",
		AggregateID:   es.NewAggregateID("Visit", "x"),
		SeqNr:         1,
		IsCreated:     true,
		OccurredAt:    occurred,
		Payload:       []byte(`{"k":"v"}`),
		TraceParent:   "00-trace-span-01",
		TraceState:    "vendor=value",
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var out es.EventEnvelope
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if out.EventID != in.EventID || out.EventTypeName != in.EventTypeName ||
		out.AggregateID.AsString() != in.AggregateID.AsString() ||
		out.SeqNr != in.SeqNr || out.IsCreated != in.IsCreated ||
		!out.OccurredAt.Equal(in.OccurredAt) ||
		string(out.Payload) != string(in.Payload) ||
		out.TraceParent != in.TraceParent || out.TraceState != in.TraceState {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", out, in)
	}
}

func TestSnapshotEnvelope_jsonRoundtrip(t *testing.T) {
	occurred := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	in := es.SnapshotEnvelope{
		AggregateID: es.NewAggregateID("Visit", "x"),
		SeqNr:       5,
		Version:     1,
		Payload:     []byte(`{"state":"scheduled"}`),
		OccurredAt:  occurred,
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var out es.SnapshotEnvelope
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if out.AggregateID.AsString() != in.AggregateID.AsString() ||
		out.SeqNr != in.SeqNr || out.Version != in.Version ||
		string(out.Payload) != string(in.Payload) ||
		!out.OccurredAt.Equal(in.OccurredAt) {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", out, in)
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestEventEnvelope ./...`
Expected: `undefined: es.EventEnvelope` のコンパイルエラー

- [ ] **Step 3: 実装**

`v2/envelope.go`:
```go
package eventsourcing

import (
	"encoding/json"
	"time"
)

// EventEnvelope wraps a serialized domain event with all storage metadata.
// This is the wire format used between Repository and EventStore.
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

// envelope JSON format. AggregateID is split into typeName/value.
type eventEnvelopeJSON struct {
	EventID         string    `json:"event_id"`
	EventTypeName   string    `json:"event_type_name"`
	AggregateType   string    `json:"aggregate_type"`
	AggregateValue  string    `json:"aggregate_value"`
	SeqNr           uint64    `json:"seq_nr"`
	IsCreated       bool      `json:"is_created"`
	OccurredAt      time.Time `json:"occurred_at"`
	Payload         []byte    `json:"payload"`
	TraceParent     string    `json:"traceparent,omitempty"`
	TraceState      string    `json:"tracestate,omitempty"`
}

func (e EventEnvelope) MarshalJSON() ([]byte, error) {
	return json.Marshal(eventEnvelopeJSON{
		EventID:        e.EventID,
		EventTypeName:  e.EventTypeName,
		AggregateType:  e.AggregateID.TypeName(),
		AggregateValue: e.AggregateID.Value(),
		SeqNr:          e.SeqNr,
		IsCreated:      e.IsCreated,
		OccurredAt:     e.OccurredAt,
		Payload:        e.Payload,
		TraceParent:    e.TraceParent,
		TraceState:     e.TraceState,
	})
}

func (e *EventEnvelope) UnmarshalJSON(data []byte) error {
	var j eventEnvelopeJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	e.EventID = j.EventID
	e.EventTypeName = j.EventTypeName
	e.AggregateID = NewAggregateID(j.AggregateType, j.AggregateValue)
	e.SeqNr = j.SeqNr
	e.IsCreated = j.IsCreated
	e.OccurredAt = j.OccurredAt
	e.Payload = j.Payload
	e.TraceParent = j.TraceParent
	e.TraceState = j.TraceState
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
		AggregateType:  s.AggregateID.TypeName(),
		AggregateValue: s.AggregateID.Value(),
		SeqNr:          s.SeqNr,
		Version:        s.Version,
		Payload:        s.Payload,
		OccurredAt:     s.OccurredAt,
	})
}

func (s *SnapshotEnvelope) UnmarshalJSON(data []byte) error {
	var j snapshotEnvelopeJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	s.AggregateID = NewAggregateID(j.AggregateType, j.AggregateValue)
	s.SeqNr = j.SeqNr
	s.Version = j.Version
	s.Payload = j.Payload
	s.OccurredAt = j.OccurredAt
	return nil
}
```

- [ ] **Step 4: テスト合格を確認**

Run: `cd v2 && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/envelope.go v2/envelope_test.go
git commit -m "feat(v2): add EventEnvelope and SnapshotEnvelope"
```

---

## Phase 5: Serializer interfaces

### Task 8: AggregateSerializer[T,E] と EventSerializer[E]

**Files:**
- Create: `v2/serializer.go`

- [ ] **Step 1: Serializer interface を定義**

`v2/serializer.go`:
```go
package eventsourcing

// AggregateSerializer encodes/decodes a domain aggregate to/from bytes.
// T is the aggregate type (typically a domain interface like Visit).
// E is the aggregate-specific Event type. Both are constrained so the
// serializer cannot be paired with mismatched aggregate/event types.
type AggregateSerializer[T Aggregate[E], E Event] interface {
	Serialize(T) ([]byte, error)
	Deserialize([]byte) (T, error)
}

// EventSerializer encodes/decodes a domain event to/from bytes.
// On Deserialize, typeName (== Event.EventTypeName()) is used to dispatch
// to the correct concrete event type.
type EventSerializer[E Event] interface {
	Serialize(E) ([]byte, error)
	Deserialize(typeName string, data []byte) (E, error)
}
```

- [ ] **Step 2: コンパイル確認**

Run: `cd v2 && go build ./...`
Expected: エラーなし

- [ ] **Step 3: Commit**

```bash
git add v2/serializer.go
git commit -m "feat(v2): add AggregateSerializer and EventSerializer interfaces"
```

---

## Phase 6: EventStore interface と Config

### Task 9: EventStore interface, Config, ShouldSnapshot

**Files:**
- Create: `v2/event_store.go`
- Create: `v2/event_store_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`v2/event_store_test.go`:
```go
package eventsourcing_test

import (
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
)

func TestDefaultConfig(t *testing.T) {
	c := es.DefaultConfig()
	if c.SnapshotInterval != 5 {
		t.Errorf("SnapshotInterval: got %d, want 5", c.SnapshotInterval)
	}
	if c.JournalTableName != "journal" {
		t.Errorf("JournalTableName: got %q, want %q", c.JournalTableName, "journal")
	}
	if c.SnapshotTableName != "snapshot" {
		t.Errorf("SnapshotTableName: got %q, want %q", c.SnapshotTableName, "snapshot")
	}
	if c.ShardCount != 1 {
		t.Errorf("ShardCount: got %d, want 1", c.ShardCount)
	}
}

func TestConfig_ShouldSnapshot(t *testing.T) {
	tests := []struct {
		name             string
		snapshotInterval uint64
		seqNr            uint64
		want             bool
	}{
		{"interval=0 disables snapshots", 0, 5, false},
		{"seqNr 1 with interval 5", 5, 1, false},
		{"seqNr 5 with interval 5", 5, 5, true},
		{"seqNr 6 with interval 5", 5, 6, false},
		{"seqNr 10 with interval 5", 5, 10, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := es.Config{SnapshotInterval: tt.snapshotInterval}
			if got := c.ShouldSnapshot(tt.seqNr); got != tt.want {
				t.Errorf("ShouldSnapshot(%d) with interval=%d: got %v, want %v",
					tt.seqNr, tt.snapshotInterval, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestDefaultConfig ./...`
Expected: `undefined: es.DefaultConfig` のコンパイルエラー

- [ ] **Step 3: 実装**

`v2/event_store.go`:
```go
package eventsourcing

import "context"

// EventStore is the low-level persistence abstraction.
// All operations work on Envelopes; domain types are not exposed at this layer.
type EventStore interface {
	// GetLatestSnapshot returns the most recent snapshot for the aggregate,
	// or nil if no snapshot exists.
	GetLatestSnapshot(ctx context.Context, id AggregateID) (*SnapshotEnvelope, error)

	// GetEventsSince returns all events with SeqNr > seqNr, ordered by SeqNr.
	GetEventsSince(ctx context.Context, id AggregateID, seqNr uint64) ([]*EventEnvelope, error)

	// PersistEvent appends a single event without a snapshot.
	// expectedVersion is used only for first-write detection (== 0 means
	// "this aggregate must not exist yet"). It is NOT used as an optimistic
	// lock; that is performed by PersistEventAndSnapshot.
	// The store rejects events with duplicate (AggregateID, SeqNr) via
	// ErrDuplicateAggregate.
	PersistEvent(ctx context.Context, ev *EventEnvelope, expectedVersion uint64) error

	// PersistEventAndSnapshot atomically appends an event and updates the
	// snapshot. The snapshot's Version field is used for optimistic locking:
	// the operation fails with ErrOptimisticLock if the stored snapshot's
	// version != snap.Version - 1 (i.e. the version was incremented by
	// someone else in parallel).
	PersistEventAndSnapshot(ctx context.Context, ev *EventEnvelope, snap *SnapshotEnvelope) error
}

// Config holds configuration for the EventStore.
type Config struct {
	// SnapshotInterval determines the number of events between snapshots.
	// Default 5. A value of 0 disables snapshots entirely.
	SnapshotInterval uint64

	// JournalTableName is the table name for events (DynamoDB only).
	JournalTableName string

	// SnapshotTableName is the table name for snapshots (DynamoDB only).
	SnapshotTableName string

	// ShardCount controls FNV-1a-based partition key sharding (DynamoDB only).
	ShardCount uint64

	// KeepSnapshotCount determines how many old snapshots to retain.
	// 0 means all snapshots are retained.
	KeepSnapshotCount uint64
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		SnapshotInterval:  5,
		JournalTableName:  "journal",
		SnapshotTableName: "snapshot",
		ShardCount:        1,
		KeepSnapshotCount: 0,
	}
}

// ShouldSnapshot returns true when seqNr triggers a snapshot.
func (c Config) ShouldSnapshot(seqNr uint64) bool {
	if c.SnapshotInterval == 0 {
		return false
	}
	return seqNr%c.SnapshotInterval == 0
}
```

- [ ] **Step 4: テスト合格を確認**

Run: `cd v2 && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/event_store.go v2/event_store_test.go
git commit -m "feat(v2): add EventStore interface, Config, ShouldSnapshot"
```

---

## Phase 7: EventID generator

### Task 10: generateEventID ヘルパ

**Files:**
- Create: `v2/eventid.go`
- Create: `v2/eventid_test.go`

`generateEventID` は Repository 内部で EventEnvelope.EventID を埋めるために使う。`crypto/rand` で 16 bytes 取って hex 化（32 文字）。外部依存追加なし。

- [ ] **Step 1: 失敗するテストを書く**

`v2/eventid_test.go`:
```go
package eventsourcing

import (
	"regexp"
	"testing"
)

func TestGenerateEventID_format(t *testing.T) {
	id, err := generateEventID()
	if err != nil {
		t.Fatalf("generateEventID() error = %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
		t.Errorf("id format unexpected: %q", id)
	}
}

func TestGenerateEventID_unique(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id, err := generateEventID()
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id at iteration %d: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestGenerateEventID ./...`
Expected: `undefined: generateEventID` のコンパイルエラー

- [ ] **Step 3: 実装**

`v2/eventid.go`:
```go
package eventsourcing

import (
	"crypto/rand"
	"encoding/hex"
)

// generateEventID produces a 32-char hex string from 16 random bytes.
// Equivalent uniqueness to UUIDv4 / ULID for practical purposes.
func generateEventID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
```

- [ ] **Step 4: テスト合格を確認**

Run: `cd v2 && go test -run TestGenerateEventID ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/eventid.go v2/eventid_test.go
git commit -m "feat(v2): add internal generateEventID helper"
```

---

## Phase 8: memory store (TDD)

### Task 11: memory.Store skeleton + GetLatestSnapshot + GetEventsSince

**Files:**
- Create: `v2/memory/store.go`
- Create: `v2/memory/store_test.go`

memory store は EventEnvelope ベースで動作する。集約ごとに events と snapshot を map で保持。

- [ ] **Step 1: 失敗するテストを書く**

`v2/memory/store_test.go`:
```go
package memory_test

import (
	"context"
	"testing"
	"time"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/memory"
)

func TestStore_GetLatestSnapshot_empty(t *testing.T) {
	store := memory.New()
	id := es.NewAggregateID("Visit", "x")

	snap, err := store.GetLatestSnapshot(context.Background(), id)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if snap != nil {
		t.Errorf("snap = %+v, want nil", snap)
	}
}

func TestStore_GetEventsSince_empty(t *testing.T) {
	store := memory.New()
	id := es.NewAggregateID("Visit", "x")

	events, err := store.GetEventsSince(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(events) != 0 {
		t.Errorf("events len = %d, want 0", len(events))
	}
}

func newEventEnvelope(t *testing.T, id es.AggregateID, seqNr uint64, isCreated bool) *es.EventEnvelope {
	t.Helper()
	return &es.EventEnvelope{
		EventID:       "ev-test",
		EventTypeName: "Test",
		AggregateID:   id,
		SeqNr:         seqNr,
		IsCreated:     isCreated,
		OccurredAt:    time.Now(),
		Payload:       []byte(`{}`),
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test ./memory/...`
Expected: `package memory not found` のコンパイルエラー

- [ ] **Step 3: skeleton 実装**

`v2/memory/store.go`:
```go
// Package memory provides an in-memory EventStore implementation for testing.
package memory

import (
	"context"
	"sort"
	"sync"

	es "github.com/Hiroshi0900/eventstore/v2"
)

// Store is an in-memory EventStore implementation.
type Store struct {
	mu        sync.RWMutex
	events    map[string][]*es.EventEnvelope    // aggregateID.AsString() -> ordered events
	snapshots map[string]*es.SnapshotEnvelope   // aggregateID.AsString() -> latest snapshot
}

// New creates a new in-memory store.
func New() *Store {
	return &Store{
		events:    make(map[string][]*es.EventEnvelope),
		snapshots: make(map[string]*es.SnapshotEnvelope),
	}
}

// GetLatestSnapshot returns the latest snapshot or nil.
func (s *Store) GetLatestSnapshot(_ context.Context, id es.AggregateID) (*es.SnapshotEnvelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshots[id.AsString()], nil
}

// GetEventsSince returns events with SeqNr > seqNr, ordered by SeqNr.
func (s *Store) GetEventsSince(_ context.Context, id es.AggregateID, seqNr uint64) ([]*es.EventEnvelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	src := s.events[id.AsString()]
	var out []*es.EventEnvelope
	for _, ev := range src {
		if ev.SeqNr > seqNr {
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SeqNr < out[j].SeqNr })
	return out, nil
}

// PersistEvent and PersistEventAndSnapshot are implemented in subsequent tasks.
func (s *Store) PersistEvent(_ context.Context, _ *es.EventEnvelope, _ uint64) error {
	panic("not implemented")
}
func (s *Store) PersistEventAndSnapshot(_ context.Context, _ *es.EventEnvelope, _ *es.SnapshotEnvelope) error {
	panic("not implemented")
}
```

- [ ] **Step 4: テスト合格を確認**

Run: `cd v2 && go test ./memory/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/memory/store.go v2/memory/store_test.go
git commit -m "feat(v2/memory): add Store skeleton with snapshot/event reads"
```

---

### Task 12: memory.Store.PersistEvent

**Files:**
- Modify: `v2/memory/store.go`
- Modify: `v2/memory/store_test.go`

- [ ] **Step 1: 失敗するテストを追加**

`v2/memory/store_test.go` の末尾に追記：
```go
func TestStore_PersistEvent_persistsAndQueryable(t *testing.T) {
	store := memory.New()
	id := es.NewAggregateID("Visit", "x")

	ev := newEventEnvelope(t, id, 1, true)
	if err := store.PersistEvent(context.Background(), ev, 0); err != nil {
		t.Fatalf("PersistEvent: %v", err)
	}

	got, err := store.GetEventsSince(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsSince: %v", err)
	}
	if len(got) != 1 || got[0].SeqNr != 1 {
		t.Errorf("got %+v, want 1 event with SeqNr=1", got)
	}
}

func TestStore_PersistEvent_duplicateSeqNr(t *testing.T) {
	store := memory.New()
	id := es.NewAggregateID("Visit", "x")

	if err := store.PersistEvent(context.Background(), newEventEnvelope(t, id, 1, true), 0); err != nil {
		t.Fatalf("first PersistEvent: %v", err)
	}
	err := store.PersistEvent(context.Background(), newEventEnvelope(t, id, 1, false), 0)
	if err == nil {
		t.Fatalf("second PersistEvent: want error, got nil")
	}
}

func TestStore_PersistEvent_initialEventOnExistingAggregate(t *testing.T) {
	store := memory.New()
	id := es.NewAggregateID("Visit", "x")

	if err := store.PersistEvent(context.Background(), newEventEnvelope(t, id, 1, true), 0); err != nil {
		t.Fatalf("first PersistEvent: %v", err)
	}
	// Second persist with expectedVersion=0 (i.e. "I think this aggregate is new")
	// must fail because the aggregate already has events.
	err := store.PersistEvent(context.Background(), newEventEnvelope(t, id, 2, true), 0)
	if err == nil {
		t.Fatalf("PersistEvent with expectedVersion=0 on existing aggregate: want error, got nil")
	}
	if !errorsIs(err, es.ErrDuplicateAggregate) {
		t.Errorf("expected ErrDuplicateAggregate, got %v", err)
	}
}

// errorsIs is a small helper to avoid importing errors in every test file.
func errorsIs(err, target error) bool {
	if err == nil {
		return target == nil
	}
	type isCheck interface{ Is(error) bool }
	if c, ok := err.(isCheck); ok && c.Is(target) {
		return true
	}
	return err == target
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestStore_PersistEvent ./memory/...`
Expected: panic（"not implemented"）

- [ ] **Step 3: PersistEvent を実装**

`v2/memory/store.go` の `PersistEvent` を置き換え：
```go
// PersistEvent appends a single event. expectedVersion==0 indicates "first
// write" and the operation fails with ErrDuplicateAggregate if events already
// exist for the aggregate. expectedVersion>0 has no effect (optimistic locking
// is performed only by PersistEventAndSnapshot).
// Duplicate (aggregateID, seqNr) entries are rejected.
func (s *Store) PersistEvent(_ context.Context, ev *es.EventEnvelope, expectedVersion uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := ev.AggregateID.AsString()

	if expectedVersion == 0 && len(s.events[key]) > 0 {
		return es.NewDuplicateAggregateError(key)
	}

	for _, existing := range s.events[key] {
		if existing.SeqNr == ev.SeqNr {
			return es.NewDuplicateAggregateError(key)
		}
	}

	s.events[key] = append(s.events[key], ev)
	return nil
}
```

- [ ] **Step 4: テスト合格を確認**

Run: `cd v2 && go test ./memory/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/memory/store.go v2/memory/store_test.go
git commit -m "feat(v2/memory): implement PersistEvent with duplicate detection"
```

---

### Task 13: memory.Store.PersistEventAndSnapshot

**Files:**
- Modify: `v2/memory/store.go`
- Modify: `v2/memory/store_test.go`

- [ ] **Step 1: 失敗するテストを追加**

`v2/memory/store_test.go` の末尾に追記：
```go
func newSnapshotEnvelope(id es.AggregateID, seqNr, version uint64) *es.SnapshotEnvelope {
	return &es.SnapshotEnvelope{
		AggregateID: id,
		SeqNr:       seqNr,
		Version:     version,
		Payload:     []byte(`{"state":"ok"}`),
		OccurredAt:  time.Now(),
	}
}

func TestStore_PersistEventAndSnapshot_initialWrite(t *testing.T) {
	store := memory.New()
	id := es.NewAggregateID("Visit", "x")

	ev := newEventEnvelope(t, id, 5, false)
	snap := newSnapshotEnvelope(id, 5, 1)

	if err := store.PersistEventAndSnapshot(context.Background(), ev, snap); err != nil {
		t.Fatalf("PersistEventAndSnapshot: %v", err)
	}

	got, err := store.GetLatestSnapshot(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshot: %v", err)
	}
	if got == nil || got.Version != 1 || got.SeqNr != 5 {
		t.Errorf("snapshot mismatch: got %+v", got)
	}
}

func TestStore_PersistEventAndSnapshot_optimisticLockFailure(t *testing.T) {
	store := memory.New()
	id := es.NewAggregateID("Visit", "x")

	if err := store.PersistEventAndSnapshot(
		context.Background(),
		newEventEnvelope(t, id, 5, false),
		newSnapshotEnvelope(id, 5, 1),
	); err != nil {
		t.Fatalf("first persist: %v", err)
	}
	// Concurrent caller still thinks the version is 1, computes Version=2,
	// but another writer already moved the version to 2 below.
	if err := store.PersistEventAndSnapshot(
		context.Background(),
		newEventEnvelope(t, id, 10, false),
		newSnapshotEnvelope(id, 10, 2),
	); err != nil {
		t.Fatalf("second persist: %v", err)
	}
	// Now a stale writer attempts to commit Version=2 again.
	err := store.PersistEventAndSnapshot(
		context.Background(),
		newEventEnvelope(t, id, 11, false),
		newSnapshotEnvelope(id, 11, 2),
	)
	if err == nil {
		t.Fatalf("want ErrOptimisticLock, got nil")
	}
	if !errorsIs(err, es.ErrOptimisticLock) {
		t.Errorf("got %v, want ErrOptimisticLock", err)
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestStore_PersistEventAndSnapshot ./memory/...`
Expected: panic（"not implemented"）

- [ ] **Step 3: PersistEventAndSnapshot を実装**

`v2/memory/store.go` の `PersistEventAndSnapshot` を置き換え：
```go
// PersistEventAndSnapshot appends an event AND updates the snapshot atomically.
// The snapshot's Version field acts as the optimistic lock: the caller is
// expected to pass Version = currentSnapshotVersion + 1. The operation fails
// with ErrOptimisticLock if the stored snapshot's Version is not snap.Version - 1.
// (For initial writes there is no stored snapshot; snap.Version must be 1.)
func (s *Store) PersistEventAndSnapshot(
	_ context.Context,
	ev *es.EventEnvelope,
	snap *es.SnapshotEnvelope,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := ev.AggregateID.AsString()

	expected := snap.Version - 1
	current := uint64(0)
	if existing := s.snapshots[key]; existing != nil {
		current = existing.Version
	}
	if current != expected {
		return es.NewOptimisticLockError(key, expected, current)
	}

	for _, existing := range s.events[key] {
		if existing.SeqNr == ev.SeqNr {
			return es.NewDuplicateAggregateError(key)
		}
	}

	s.events[key] = append(s.events[key], ev)
	s.snapshots[key] = snap
	return nil
}
```

- [ ] **Step 4: テスト合格を確認**

Run: `cd v2 && go test ./memory/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/memory/store.go v2/memory/store_test.go
git commit -m "feat(v2/memory): implement PersistEventAndSnapshot with optimistic lock"
```

---

## Phase 9: Repository[T,E] (TDD)

### Task 14: Repository テスト用の counter 集約とシリアライザを定義

**Files:**
- Create: `v2/repository_test.go`

repository テストでは domain-pure な集約を実装し、Repository.Save / Load を end-to-end で検証する。counter 集約：

- 状態は `count int` のみ
- Command: `IncrementCommand{By int}`
- Event: `IncrementedEvent{By int}`

- [ ] **Step 1: テスト用集約を定義**

`v2/repository_test.go`:
```go
package eventsourcing_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/memory"
)

// === counter ドメイン (テスト用) ===

type counterEvent interface {
	es.Event
	isCounterEvent()
}

type incrementedEvent struct {
	AggID es.AggregateID
	By    int
}

func (e incrementedEvent) EventTypeName() string       { return "Incremented" }
func (e incrementedEvent) AggregateID() es.AggregateID { return e.AggID }
func (incrementedEvent) isCounterEvent()               {}

type incrementCommand struct{ By int }

func (incrementCommand) CommandTypeName() string { return "Increment" }

type counterAggregate struct {
	id    es.AggregateID
	count int
}

func (c counterAggregate) AggregateID() es.AggregateID { return c.id }

func (c counterAggregate) ApplyCommand(cmd es.Command) (counterEvent, error) {
	switch x := cmd.(type) {
	case incrementCommand:
		return incrementedEvent{AggID: c.id, By: x.By}, nil
	default:
		return nil, es.ErrUnknownCommand
	}
}

func (c counterAggregate) ApplyEvent(ev counterEvent) es.Aggregate[counterEvent] {
	if e, ok := ev.(incrementedEvent); ok {
		return counterAggregate{id: c.id, count: c.count + e.By}
	}
	return c
}

// === Serializers ===

type counterAggregateState struct {
	TypeName string `json:"type_name"`
	Value    string `json:"value"`
	Count    int    `json:"count"`
}

type counterAggregateSerializer struct{}

func (counterAggregateSerializer) Serialize(c counterAggregate) ([]byte, error) {
	return json.Marshal(counterAggregateState{
		TypeName: c.id.TypeName(),
		Value:    c.id.Value(),
		Count:    c.count,
	})
}

func (counterAggregateSerializer) Deserialize(data []byte) (counterAggregate, error) {
	var s counterAggregateState
	if err := json.Unmarshal(data, &s); err != nil {
		return counterAggregate{}, es.NewDeserializationError("aggregate", err)
	}
	return counterAggregate{id: es.NewAggregateID(s.TypeName, s.Value), count: s.Count}, nil
}

type incrementedEventWire struct {
	AggregateType  string `json:"aggregate_type"`
	AggregateValue string `json:"aggregate_value"`
	By             int    `json:"by"`
}

type counterEventSerializer struct{}

func (counterEventSerializer) Serialize(ev counterEvent) ([]byte, error) {
	switch e := ev.(type) {
	case incrementedEvent:
		return json.Marshal(incrementedEventWire{
			AggregateType:  e.AggID.TypeName(),
			AggregateValue: e.AggID.Value(),
			By:             e.By,
		})
	default:
		return nil, es.NewSerializationError("event", errors.New("unknown event type"))
	}
}

func (counterEventSerializer) Deserialize(typeName string, data []byte) (counterEvent, error) {
	switch typeName {
	case "Incremented":
		var w incrementedEventWire
		if err := json.Unmarshal(data, &w); err != nil {
			return nil, es.NewDeserializationError("event", err)
		}
		return incrementedEvent{
			AggID: es.NewAggregateID(w.AggregateType, w.AggregateValue),
			By:    w.By,
		}, nil
	default:
		return nil, es.NewDeserializationError("event", errors.New("unknown event type: "+typeName))
	}
}

// blankCounter is the createBlank function passed to NewRepository.
func blankCounter(id es.AggregateID) counterAggregate {
	return counterAggregate{id: id, count: 0}
}

// helper: build a Repository with a fresh memory store.
func newCounterRepo(t *testing.T, cfg es.Config) (*es.DefaultRepository[counterAggregate, counterEvent], *memory.Store) {
	t.Helper()
	store := memory.New()
	repo := es.NewRepository[counterAggregate, counterEvent](
		store,
		blankCounter,
		counterAggregateSerializer{},
		counterEventSerializer{},
		cfg,
	)
	return repo, store
}

// useless test to confirm the test fixtures compile.
func TestCounterFixturesCompile(t *testing.T) {
	repo, _ := newCounterRepo(t, es.DefaultConfig())
	_ = repo
}
```

- [ ] **Step 2: テスト失敗を確認（Repository が未実装）**

Run: `cd v2 && go test ./...`
Expected: `undefined: es.DefaultRepository` 等のコンパイルエラー

(以下のタスクで実装する)

---

### Task 15: Repository.Load - 集約が存在しない時 ErrAggregateNotFound

**Files:**
- Create: `v2/repository.go`
- Modify: `v2/repository_test.go`

- [ ] **Step 1: 失敗するテストを追加**

`v2/repository_test.go` の末尾に追記：
```go
func TestRepository_Load_notFound(t *testing.T) {
	repo, _ := newCounterRepo(t, es.DefaultConfig())
	id := es.NewAggregateID("Counter", "missing")

	_, err := repo.Load(context.Background(), id)
	if err == nil {
		t.Fatalf("Load: want error, got nil")
	}
	if !errors.Is(err, es.ErrAggregateNotFound) {
		t.Errorf("Load: want ErrAggregateNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Repository skeleton + Load 実装**

`v2/repository.go`:
```go
package eventsourcing

import (
	"context"
	"fmt"
	"time"
)

// Repository provides a high-level API over EventStore.
// T is the aggregate type and E is its associated Event type.
type Repository[T Aggregate[E], E Event] interface {
	// Load returns the current state of the aggregate by replaying events
	// from the latest snapshot. Returns ErrAggregateNotFound if no snapshot
	// and no events exist.
	Load(ctx context.Context, id AggregateID) (T, error)

	// Save loads the current state, applies the command to produce a single
	// event, persists the event (and snapshot when SnapshotInterval is hit),
	// and returns the next state. If the aggregate does not yet exist,
	// the createBlank function is used to construct an initial state.
	Save(ctx context.Context, id AggregateID, cmd Command) (T, error)
}

// DefaultRepository is the default Repository implementation.
type DefaultRepository[T Aggregate[E], E Event] struct {
	store         EventStore
	createBlank   func(AggregateID) T
	aggSerializer AggregateSerializer[T, E]
	evSerializer  EventSerializer[E]
	config        Config
}

// NewRepository creates a new DefaultRepository.
func NewRepository[T Aggregate[E], E Event](
	store EventStore,
	createBlank func(AggregateID) T,
	aggSerializer AggregateSerializer[T, E],
	evSerializer EventSerializer[E],
	config Config,
) *DefaultRepository[T, E] {
	return &DefaultRepository[T, E]{
		store:         store,
		createBlank:   createBlank,
		aggSerializer: aggSerializer,
		evSerializer:  evSerializer,
		config:        config,
	}
}

// loadInternal returns the current aggregate state, plus the seqNr and version
// of the latest snapshot (or 0,0 if no snapshot exists).
// notFound==true indicates that neither snapshot nor events exist.
func (r *DefaultRepository[T, E]) loadInternal(
	ctx context.Context,
	id AggregateID,
) (agg T, seqNr, version uint64, notFound bool, err error) {
	snap, err := r.store.GetLatestSnapshot(ctx, id)
	if err != nil {
		return agg, 0, 0, false, err
	}

	if snap != nil {
		agg, err = r.aggSerializer.Deserialize(snap.Payload)
		if err != nil {
			return agg, 0, 0, false, err
		}
		seqNr = snap.SeqNr
		version = snap.Version
	} else {
		agg = r.createBlank(id)
		seqNr = 0
		version = 0
	}

	events, err := r.store.GetEventsSince(ctx, id, seqNr)
	if err != nil {
		return agg, 0, 0, false, err
	}

	if snap == nil && len(events) == 0 {
		return agg, 0, 0, true, nil
	}

	for _, env := range events {
		ev, err := r.evSerializer.Deserialize(env.EventTypeName, env.Payload)
		if err != nil {
			return agg, 0, 0, false, err
		}
		next, ok := agg.ApplyEvent(ev).(T)
		if !ok {
			return agg, 0, 0, false, fmt.Errorf(
				"%w: ApplyEvent returned a value that does not implement T",
				ErrInvalidAggregate,
			)
		}
		agg = next
		seqNr = env.SeqNr
	}
	return agg, seqNr, version, false, nil
}

// Load returns the aggregate, or ErrAggregateNotFound if it doesn't exist.
func (r *DefaultRepository[T, E]) Load(ctx context.Context, id AggregateID) (T, error) {
	agg, _, _, notFound, err := r.loadInternal(ctx, id)
	if err != nil {
		var zero T
		return zero, err
	}
	if notFound {
		var zero T
		return zero, NewAggregateNotFoundError(id.TypeName(), id.Value())
	}
	return agg, nil
}

// Save is implemented in the next task.
func (r *DefaultRepository[T, E]) Save(ctx context.Context, id AggregateID, cmd Command) (T, error) {
	var zero T
	_ = ctx
	_ = id
	_ = cmd
	return zero, fmt.Errorf("not implemented")
}
```

> **Note**: Repository[T,E] interface 充足は `repository_test.go` の `newCounterRepo` で実 instantiation しているため、コンパイル時に検証される。専用の `var _` アサーションは不要。

> **設計メモ**: 末尾の `var _` アサーションは Repository[T,E] interface を DefaultRepository[T,E] が満たすことを compile time に固定するための型チェックダミー。

- [ ] **Step 3: テスト合格を確認**

Run: `cd v2 && go test -run TestRepository_Load_notFound ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add v2/repository.go v2/repository_test.go
git commit -m "feat(v2): add Repository skeleton and Load with not-found handling"
```

---

### Task 16: Repository.Save - 単純な保存（snapshot 非到達）

**Files:**
- Modify: `v2/repository.go`
- Modify: `v2/repository_test.go`

- [ ] **Step 1: 失敗するテストを追加**

`v2/repository_test.go` の末尾に追記：
```go
func TestRepository_Save_firstEventCreatesAggregate(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100 // 大きく取って snapshot を回避
	repo, store := newCounterRepo(t, cfg)
	id := es.NewAggregateID("Counter", "1")

	got, err := repo.Save(context.Background(), id, incrementCommand{By: 3})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got.count != 3 {
		t.Errorf("count: got %d, want 3", got.count)
	}

	// 永続化された Event を直接確認
	envs, err := store.GetEventsSince(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsSince: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("event count: got %d, want 1", len(envs))
	}
	if envs[0].SeqNr != 1 {
		t.Errorf("SeqNr: got %d, want 1", envs[0].SeqNr)
	}
	if !envs[0].IsCreated {
		t.Errorf("IsCreated: got false, want true")
	}
	if envs[0].EventTypeName != "Incremented" {
		t.Errorf("EventTypeName: got %q, want %q", envs[0].EventTypeName, "Incremented")
	}
	if envs[0].EventID == "" {
		t.Errorf("EventID: empty")
	}
	if envs[0].OccurredAt.IsZero() {
		t.Errorf("OccurredAt: zero")
	}
}

func TestRepository_Save_subsequentEvent(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100
	repo, store := newCounterRepo(t, cfg)
	id := es.NewAggregateID("Counter", "2")

	if _, err := repo.Save(context.Background(), id, incrementCommand{By: 1}); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	got, err := repo.Save(context.Background(), id, incrementCommand{By: 4})
	if err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if got.count != 5 {
		t.Errorf("count: got %d, want 5", got.count)
	}

	envs, _ := store.GetEventsSince(context.Background(), id, 0)
	if len(envs) != 2 {
		t.Fatalf("event count: got %d, want 2", len(envs))
	}
	if envs[1].IsCreated {
		t.Errorf("second event IsCreated: got true, want false")
	}
	if envs[1].SeqNr != 2 {
		t.Errorf("second SeqNr: got %d, want 2", envs[1].SeqNr)
	}
}

func TestRepository_LoadAfterSave_replaysCorrectly(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 100
	repo, _ := newCounterRepo(t, cfg)
	id := es.NewAggregateID("Counter", "3")

	if _, err := repo.Save(context.Background(), id, incrementCommand{By: 7}); err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	if _, err := repo.Save(context.Background(), id, incrementCommand{By: 3}); err != nil {
		t.Fatalf("Save 2: %v", err)
	}

	got, err := repo.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.count != 10 {
		t.Errorf("count: got %d, want 10", got.count)
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestRepository_Save ./...`
Expected: 「not implemented」エラー

- [ ] **Step 3: Save を実装**

`v2/repository.go` の `Save` メソッドを次のように置き換え：
```go
// Save loads the aggregate (or starts blank), applies the command, persists
// the resulting event, optionally creating a snapshot.
func (r *DefaultRepository[T, E]) Save(
	ctx context.Context,
	id AggregateID,
	cmd Command,
) (T, error) {
	var zero T

	agg, currentSeqNr, currentVersion, _, err := r.loadInternal(ctx, id)
	if err != nil {
		return zero, err
	}

	ev, err := agg.ApplyCommand(cmd)
	if err != nil {
		return zero, err
	}

	next, ok := agg.ApplyEvent(ev).(T)
	if !ok {
		return zero, fmt.Errorf(
			"%w: ApplyEvent returned a value that does not implement T",
			ErrInvalidAggregate,
		)
	}

	nextSeqNr := currentSeqNr + 1

	payload, err := r.evSerializer.Serialize(ev)
	if err != nil {
		return zero, err
	}

	eventID, err := generateEventID()
	if err != nil {
		return zero, err
	}

	envelope := &EventEnvelope{
		EventID:       eventID,
		EventTypeName: ev.EventTypeName(),
		AggregateID:   ev.AggregateID(),
		SeqNr:         nextSeqNr,
		IsCreated:     currentSeqNr == 0,
		OccurredAt:    time.Now().UTC(),
		Payload:       payload,
		// TraceParent / TraceState are filled by DynamoDB store via context.
	}

	if r.config.ShouldSnapshot(nextSeqNr) {
		snapPayload, err := r.aggSerializer.Serialize(next)
		if err != nil {
			return zero, err
		}
		snap := &SnapshotEnvelope{
			AggregateID: ev.AggregateID(),
			SeqNr:       nextSeqNr,
			Version:     currentVersion + 1,
			Payload:     snapPayload,
			OccurredAt:  envelope.OccurredAt,
		}
		if err := r.store.PersistEventAndSnapshot(ctx, envelope, snap); err != nil {
			return zero, err
		}
	} else {
		if err := r.store.PersistEvent(ctx, envelope, currentVersion); err != nil {
			return zero, err
		}
	}

	return next, nil
}
```

- [ ] **Step 4: テスト合格を確認**

Run: `cd v2 && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/repository.go v2/repository_test.go
git commit -m "feat(v2): implement Repository.Save without snapshot trigger"
```

---

### Task 17: Repository.Save - snapshot 間隔到達時

**Files:**
- Modify: `v2/repository_test.go`

Save の実装は既に snapshot 分岐を含んでいる（前タスク）。本タスクでは snapshot がトリガされる動作を確認するテストのみを追加。

- [ ] **Step 1: テストを追加**

`v2/repository_test.go` の末尾に追記：
```go
func TestRepository_Save_triggersSnapshot(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 3
	repo, store := newCounterRepo(t, cfg)
	id := es.NewAggregateID("Counter", "snap")

	for i := 0; i < 3; i++ {
		if _, err := repo.Save(context.Background(), id, incrementCommand{By: 1}); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	snap, err := store.GetLatestSnapshot(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLatestSnapshot: %v", err)
	}
	if snap == nil {
		t.Fatalf("snapshot: nil, want present")
	}
	if snap.SeqNr != 3 {
		t.Errorf("snapshot SeqNr: got %d, want 3", snap.SeqNr)
	}
	if snap.Version != 1 {
		t.Errorf("snapshot Version: got %d, want 1", snap.Version)
	}
}

func TestRepository_LoadAfterSnapshot_usesSnapshot(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 2
	repo, _ := newCounterRepo(t, cfg)
	id := es.NewAggregateID("Counter", "snapload")

	for i := 0; i < 4; i++ {
		if _, err := repo.Save(context.Background(), id, incrementCommand{By: 2}); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	got, err := repo.Load(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.count != 8 {
		t.Errorf("count: got %d, want 8", got.count)
	}
}
```

- [ ] **Step 2: テスト合格を確認**

Run: `cd v2 && go test -run TestRepository_Save_triggersSnapshot ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add v2/repository_test.go
git commit -m "test(v2): cover Repository.Save snapshot trigger and load"
```

---

### Task 18: Repository.Save - 楽観ロック失敗

**Files:**
- Modify: `v2/repository_test.go`

- [ ] **Step 1: テストを追加**

`v2/repository_test.go` の末尾に追記：
```go
func TestRepository_Save_optimisticLockFailure(t *testing.T) {
	cfg := es.DefaultConfig()
	cfg.SnapshotInterval = 1 // 毎イベントで snapshot → 毎回楽観ロックが効く
	repo, store := newCounterRepo(t, cfg)
	id := es.NewAggregateID("Counter", "lock")

	if _, err := repo.Save(context.Background(), id, incrementCommand{By: 1}); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	// ストアを直接書き換えて並行更新を模擬：snapshot Version を 99 に上げる
	store.SetSnapshotForTest(id, 1, 99, []byte(`{"type_name":"Counter","value":"lock","count":1}`))

	_, err := repo.Save(context.Background(), id, incrementCommand{By: 1})
	if err == nil {
		t.Fatalf("Save: want optimistic lock error, got nil")
	}
	if !errors.Is(err, es.ErrOptimisticLock) {
		t.Errorf("Save: want ErrOptimisticLock, got %v", err)
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test -run TestRepository_Save_optimisticLockFailure ./...`
Expected: `undefined: store.SetSnapshotForTest` のコンパイルエラー

- [ ] **Step 3: memory.Store にテスト用ヘルパを追加**

`v2/memory/store.go` の末尾に追記：
```go
// SetSnapshotForTest directly overwrites a snapshot's version. Test helper
// only — used to simulate concurrent writers.
func (s *Store) SetSnapshotForTest(id es.AggregateID, seqNr, version uint64, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := id.AsString()
	s.snapshots[key] = &es.SnapshotEnvelope{
		AggregateID: id,
		SeqNr:       seqNr,
		Version:     version,
		Payload:     payload,
	}
}
```

- [ ] **Step 4: テスト合格を確認**

Run: `cd v2 && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/memory/store.go v2/repository_test.go
git commit -m "test(v2): cover Repository.Save optimistic lock failure"
```

---

### Task 19: Repository.Save - ApplyCommand エラー伝搬

**Files:**
- Modify: `v2/repository_test.go`

- [ ] **Step 1: テストを追加**

`v2/repository_test.go` の末尾に追記：
```go
type unknownCmd struct{}

func (unknownCmd) CommandTypeName() string { return "Unknown" }

func TestRepository_Save_propagatesApplyCommandError(t *testing.T) {
	repo, _ := newCounterRepo(t, es.DefaultConfig())
	id := es.NewAggregateID("Counter", "err")

	_, err := repo.Save(context.Background(), id, unknownCmd{})
	if err == nil {
		t.Fatalf("want ErrUnknownCommand, got nil")
	}
	if !errors.Is(err, es.ErrUnknownCommand) {
		t.Errorf("got %v, want ErrUnknownCommand", err)
	}
}
```

- [ ] **Step 2: テスト合格を確認**

Run: `cd v2 && go test -run TestRepository_Save_propagatesApplyCommandError ./...`
Expected: PASS（既存実装でエラーが伝搬する）

- [ ] **Step 3: Commit**

```bash
git add v2/repository_test.go
git commit -m "test(v2): cover Repository.Save command rejection"
```

---

## Phase 10: keyresolver の v2 移植

### Task 20: internal/keyresolver を v1 から移植

**Files:**
- Create: `v2/internal/keyresolver/key_resolver.go`
- Create: `v2/internal/keyresolver/key_resolver_test.go`

v1 の keyresolver は v1 の `AggregateID` interface（`GetTypeName()` / `GetValue()` メソッド）に依存しているので、v2 版に書き換える（`TypeName()` / `Value()` への変更）。

- [ ] **Step 1: ファイル雛形コピー**

```bash
mkdir -p /Users/sakemihiroshi/develop/terrat/eventstore/v2/internal/keyresolver
cp /Users/sakemihiroshi/develop/terrat/eventstore/internal/keyresolver/key_resolver.go \
   /Users/sakemihiroshi/develop/terrat/eventstore/v2/internal/keyresolver/key_resolver.go
cp /Users/sakemihiroshi/develop/terrat/eventstore/internal/keyresolver/key_resolver_test.go \
   /Users/sakemihiroshi/develop/terrat/eventstore/v2/internal/keyresolver/key_resolver_test.go
```

- [ ] **Step 2: import パスを v2 に書き換え**

`v2/internal/keyresolver/key_resolver.go` 冒頭：
```go
import (
	"fmt"
	"hash/fnv"

	es "github.com/Hiroshi0900/eventstore/v2"
)
```

- [ ] **Step 3: メソッド呼び出しを v2 に書き換え**

`key_resolver.go` 内の `id.GetTypeName()` → `id.TypeName()`、`id.GetValue()` → `id.Value()` に全て置換。

- [ ] **Step 4: テストファイルも同様に v2 化**

`v2/internal/keyresolver/key_resolver_test.go`：
- import パスを `github.com/Hiroshi0900/eventstore/v2` に変更
- AggregateID 構築を `es.NewAggregateID(...)` に変更
- `GetTypeName()` / `GetValue()` 呼び出しを `TypeName()` / `Value()` に変更

- [ ] **Step 5: テスト合格を確認**

Run: `cd v2 && go test ./internal/keyresolver/...`
Expected: PASS（v1 と同等の挙動）

- [ ] **Step 6: Commit**

```bash
git add v2/internal/keyresolver/
git commit -m "feat(v2/keyresolver): port internal keyresolver to v2 API"
```

---

## Phase 11: DynamoDB store

### Task 21: dynamodb.Store skeleton + 依存追加

**Files:**
- Modify: `v2/go.mod`
- Create: `v2/dynamodb/store.go`
- Create: `v2/dynamodb/store_test.go`

- [ ] **Step 1: go.mod に依存追加**

```bash
cd /Users/sakemihiroshi/develop/terrat/eventstore/v2
go get github.com/aws/aws-sdk-go-v2@v1.41.1
go get github.com/aws/aws-sdk-go-v2/service/dynamodb@v1.53.5
go get go.opentelemetry.io/otel@v1.40.0
go mod tidy
cd ..
```

- [ ] **Step 2: skeleton 実装**

`v2/dynamodb/store.go`:
```go
// Package dynamodb provides a DynamoDB-backed EventStore implementation for v2.
package dynamodb

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/internal/keyresolver"
)

// Client is the subset of dynamodb.Client used by Store. Defined as an
// interface to enable mocking.
type Client interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

// Config holds DynamoDB-specific settings.
type Config struct {
	JournalTableName  string
	SnapshotTableName string
	JournalGSIName    string
	SnapshotGSIName   string
	ShardCount        uint64
}

// DefaultConfig returns the default DynamoDB configuration.
func DefaultConfig() Config {
	return Config{
		JournalTableName:  "journal",
		SnapshotTableName: "snapshot",
		JournalGSIName:    "journal-aid-index",
		SnapshotGSIName:   "snapshot-aid-index",
		ShardCount:        1,
	}
}

// Store implements es.EventStore on top of DynamoDB.
type Store struct {
	client      Client
	keyResolver *keyresolver.Resolver
	config      Config
}

// New creates a new Store.
func New(client Client, config Config) *Store {
	return &Store{
		client:      client,
		keyResolver: keyresolver.New(keyresolver.Config{ShardCount: config.ShardCount}),
		config:      config,
	}
}

// DynamoDB attribute names.
const (
	colPKey        = "pkey"
	colSKey        = "skey"
	colAid         = "aid"
	colSeqNr       = "seq_nr"
	colPayload     = "payload"
	colOccurred    = "occurred_at"
	colTypeName    = "type_name"
	colEventID     = "event_id"
	colIsCreated   = "is_created"
	colVersion     = "version"
	colTraceparent = "traceparent"
	colTracestate  = "tracestate"
)

// Compile-time interface assertion.
var _ es.EventStore = (*Store)(nil)

// Method bodies are added in subsequent tasks.
func (s *Store) GetLatestSnapshot(_ context.Context, _ es.AggregateID) (*es.SnapshotEnvelope, error) {
	panic("not implemented")
}
func (s *Store) GetEventsSince(_ context.Context, _ es.AggregateID, _ uint64) ([]*es.EventEnvelope, error) {
	panic("not implemented")
}
func (s *Store) PersistEvent(_ context.Context, _ *es.EventEnvelope, _ uint64) error {
	panic("not implemented")
}
func (s *Store) PersistEventAndSnapshot(_ context.Context, _ *es.EventEnvelope, _ *es.SnapshotEnvelope) error {
	panic("not implemented")
}
```

- [ ] **Step 3: コンパイル確認**

Run: `cd v2 && go build ./...`
Expected: エラーなし

- [ ] **Step 4: Commit**

```bash
git add v2/go.mod v2/go.sum v2/dynamodb/store.go
git commit -m "feat(v2/dynamodb): add Store skeleton with deps"
```

---

### Task 22: dynamodb.Store.PersistEvent + GetEventsSince + GetLatestSnapshot

**Files:**
- Modify: `v2/dynamodb/store.go`
- Create: `v2/dynamodb/store_test.go`

DynamoDB の attribute marshalling は v1 を参考にしながら、新しい `EventEnvelope` / `SnapshotEnvelope` に合わせる。テストは fake Client を使う（実 DynamoDB は CI 外）。

- [ ] **Step 1: Fake client + PersistEvent テストを書く**

`v2/dynamodb/store_test.go`:
```go
package dynamodb_test

import (
	"context"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	es "github.com/Hiroshi0900/eventstore/v2"
	v2ddb "github.com/Hiroshi0900/eventstore/v2/dynamodb"
)

// fakeClient records the last received PutItem / TransactWriteItems call.
type fakeClient struct {
	lastPut   *ddb.PutItemInput
	lastTrans *ddb.TransactWriteItemsInput
	getResp   *ddb.GetItemOutput
	queryResp *ddb.QueryOutput
	putErr    error
	transErr  error
}

func (f *fakeClient) GetItem(_ context.Context, _ *ddb.GetItemInput, _ ...func(*ddb.Options)) (*ddb.GetItemOutput, error) {
	if f.getResp == nil {
		return &ddb.GetItemOutput{}, nil
	}
	return f.getResp, nil
}
func (f *fakeClient) Query(_ context.Context, _ *ddb.QueryInput, _ ...func(*ddb.Options)) (*ddb.QueryOutput, error) {
	if f.queryResp == nil {
		return &ddb.QueryOutput{}, nil
	}
	return f.queryResp, nil
}
func (f *fakeClient) PutItem(_ context.Context, in *ddb.PutItemInput, _ ...func(*ddb.Options)) (*ddb.PutItemOutput, error) {
	f.lastPut = in
	return &ddb.PutItemOutput{}, f.putErr
}
func (f *fakeClient) TransactWriteItems(_ context.Context, in *ddb.TransactWriteItemsInput, _ ...func(*ddb.Options)) (*ddb.TransactWriteItemsOutput, error) {
	f.lastTrans = in
	return &ddb.TransactWriteItemsOutput{}, f.transErr
}

func TestStore_PersistEvent_writesPutItem(t *testing.T) {
	c := &fakeClient{}
	store := v2ddb.New(c, v2ddb.DefaultConfig())
	id := es.NewAggregateID("Visit", "x")

	ev := &es.EventEnvelope{
		EventID:       "ev-1",
		EventTypeName: "Test",
		AggregateID:   id,
		SeqNr:         1,
		IsCreated:     true,
		OccurredAt:    time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC),
		Payload:       []byte(`{"k":"v"}`),
	}

	if err := store.PersistEvent(context.Background(), ev, 0); err != nil {
		t.Fatalf("PersistEvent: %v", err)
	}
	if c.lastPut == nil {
		t.Fatalf("PutItem not called")
	}
	if got := awssdk.ToString(c.lastPut.TableName); got != "journal" {
		t.Errorf("TableName: got %q, want %q", got, "journal")
	}
	if _, ok := c.lastPut.Item["pkey"].(*types.AttributeValueMemberS); !ok {
		t.Errorf("pkey attribute missing or wrong type")
	}
	if _, ok := c.lastPut.Item["skey"].(*types.AttributeValueMemberS); !ok {
		t.Errorf("skey attribute missing or wrong type")
	}
	if v, ok := c.lastPut.Item["seq_nr"].(*types.AttributeValueMemberN); !ok || v.Value != "1" {
		t.Errorf("seq_nr attribute mismatch: %+v", c.lastPut.Item["seq_nr"])
	}
}
```

- [ ] **Step 2: テスト失敗を確認**

Run: `cd v2 && go test ./dynamodb/...`
Expected: panic("not implemented")

- [ ] **Step 3: PersistEvent + Get* を実装**

`v2/dynamodb/store.go` のメソッド実装を追記（`panic` を置き換え）：
```go
import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/internal/keyresolver"
)
```

(import セクションは file 先頭に統合)

```go
// GetLatestSnapshot returns the latest snapshot or nil.
func (s *Store) GetLatestSnapshot(ctx context.Context, id es.AggregateID) (*es.SnapshotEnvelope, error) {
	keys := s.keyResolver.ResolveSnapshotKeys(id)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: awssdk.String(s.config.SnapshotTableName),
		Key: map[string]types.AttributeValue{
			colPKey: &types.AttributeValueMemberS{Value: keys.PartitionKey},
			colSKey: &types.AttributeValueMemberS{Value: keys.SortKey},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get snapshot: %w", err)
	}
	if out.Item == nil {
		return nil, nil
	}
	return s.unmarshalSnapshot(out.Item)
}

// GetEventsSince queries the journal GSI for events with SeqNr > seqNr.
func (s *Store) GetEventsSince(ctx context.Context, id es.AggregateID, seqNr uint64) ([]*es.EventEnvelope, error) {
	aidKey := s.keyResolver.ResolveAggregateIDKey(id)
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              awssdk.String(s.config.JournalTableName),
		IndexName:              awssdk.String(s.config.JournalGSIName),
		KeyConditionExpression: awssdk.String("#aid = :aid AND #seq_nr > :seq_nr"),
		ExpressionAttributeNames: map[string]string{
			"#aid":    colAid,
			"#seq_nr": colSeqNr,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":aid":    &types.AttributeValueMemberS{Value: aidKey},
			":seq_nr": &types.AttributeValueMemberN{Value: strconv.FormatUint(seqNr, 10)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	envs := make([]*es.EventEnvelope, 0, len(out.Items))
	for _, item := range out.Items {
		ev, err := s.unmarshalEvent(item)
		if err != nil {
			return nil, err
		}
		envs = append(envs, ev)
	}
	return envs, nil
}

// PersistEvent writes a single event without snapshot. Uses ConditionExpression
// to reject duplicate (pkey, skey).
func (s *Store) PersistEvent(ctx context.Context, ev *es.EventEnvelope, expectedVersion uint64) error {
	item := s.marshalEvent(ctx, ev)
	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           awssdk.String(s.config.JournalTableName),
		Item:                item,
		ConditionExpression: awssdk.String("attribute_not_exists(#pkey)"),
		ExpressionAttributeNames: map[string]string{
			"#pkey": colPKey,
		},
	})
	if err != nil {
		var ccfe *types.ConditionalCheckFailedException
		if errors.As(err, &ccfe) {
			return es.NewDuplicateAggregateError(ev.AggregateID.AsString())
		}
		return fmt.Errorf("put event: %w", err)
	}
	_ = expectedVersion // not used for non-snapshot writes
	return nil
}

// PersistEventAndSnapshot writes both atomically using TransactWriteItems.
// The snapshot put has a Version condition for optimistic locking.
func (s *Store) PersistEventAndSnapshot(ctx context.Context, ev *es.EventEnvelope, snap *es.SnapshotEnvelope) error {
	eventItem := s.marshalEvent(ctx, ev)
	snapshotItem := s.marshalSnapshot(snap)

	expected := snap.Version - 1
	condExpr := "attribute_not_exists(#version) OR #version = :expected"
	if expected == 0 {
		condExpr = "attribute_not_exists(#version)"
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:           awssdk.String(s.config.JournalTableName),
					Item:                eventItem,
					ConditionExpression: awssdk.String("attribute_not_exists(#pkey)"),
					ExpressionAttributeNames: map[string]string{
						"#pkey": colPKey,
					},
				},
			},
			{
				Put: &types.Put{
					TableName:           awssdk.String(s.config.SnapshotTableName),
					Item:                snapshotItem,
					ConditionExpression: awssdk.String(condExpr),
					ExpressionAttributeNames: map[string]string{
						"#version": colVersion,
					},
					ExpressionAttributeValues: condValues(expected),
				},
			},
		},
	})
	if err != nil {
		var tce *types.TransactionCanceledException
		if errors.As(err, &tce) {
			return es.NewOptimisticLockError(ev.AggregateID.AsString(), expected, 0)
		}
		return fmt.Errorf("transact write: %w", err)
	}
	return nil
}

func condValues(expected uint64) map[string]types.AttributeValue {
	if expected == 0 {
		return nil
	}
	return map[string]types.AttributeValue{
		":expected": &types.AttributeValueMemberN{Value: strconv.FormatUint(expected, 10)},
	}
}

// --- attribute marshal / unmarshal ---

func (s *Store) marshalEvent(ctx context.Context, ev *es.EventEnvelope) map[string]types.AttributeValue {
	keys := s.keyResolver.ResolveEventKeys(ev.AggregateID, ev.SeqNr)
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	item := map[string]types.AttributeValue{
		colPKey:      &types.AttributeValueMemberS{Value: keys.PartitionKey},
		colSKey:      &types.AttributeValueMemberS{Value: keys.SortKey},
		colAid:       &types.AttributeValueMemberS{Value: keys.AggregateIDKey},
		colSeqNr:     &types.AttributeValueMemberN{Value: strconv.FormatUint(ev.SeqNr, 10)},
		colPayload:   &types.AttributeValueMemberB{Value: ev.Payload},
		colOccurred:  &types.AttributeValueMemberN{Value: strconv.FormatInt(ev.OccurredAt.UnixMilli(), 10)},
		colTypeName:  &types.AttributeValueMemberS{Value: ev.EventTypeName},
		colEventID:   &types.AttributeValueMemberS{Value: ev.EventID},
		colIsCreated: &types.AttributeValueMemberBOOL{Value: ev.IsCreated},
	}
	if tp := carrier.Get("traceparent"); tp != "" {
		item[colTraceparent] = &types.AttributeValueMemberS{Value: tp}
	}
	if ts := carrier.Get("tracestate"); ts != "" {
		item[colTracestate] = &types.AttributeValueMemberS{Value: ts}
	}
	return item
}

func (s *Store) marshalSnapshot(snap *es.SnapshotEnvelope) map[string]types.AttributeValue {
	keys := s.keyResolver.ResolveSnapshotKeys(snap.AggregateID)
	return map[string]types.AttributeValue{
		colPKey:     &types.AttributeValueMemberS{Value: keys.PartitionKey},
		colSKey:     &types.AttributeValueMemberS{Value: keys.SortKey},
		colAid:      &types.AttributeValueMemberS{Value: keys.AggregateIDKey},
		colSeqNr:    &types.AttributeValueMemberN{Value: strconv.FormatUint(snap.SeqNr, 10)},
		colVersion:  &types.AttributeValueMemberN{Value: strconv.FormatUint(snap.Version, 10)},
		colPayload:  &types.AttributeValueMemberB{Value: snap.Payload},
		colOccurred: &types.AttributeValueMemberN{Value: strconv.FormatInt(snap.OccurredAt.UnixMilli(), 10)},
	}
}

func (s *Store) unmarshalEvent(item map[string]types.AttributeValue) (*es.EventEnvelope, error) {
	aidStr, _ := getS(item, colAid)
	id, err := parseAggregateIDKey(aidStr)
	if err != nil {
		return nil, err
	}
	seqNr, err := getN(item, colSeqNr)
	if err != nil {
		return nil, err
	}
	occurred, err := getN(item, colOccurred)
	if err != nil {
		return nil, err
	}
	payload, _ := getB(item, colPayload)
	typeName, _ := getS(item, colTypeName)
	eventID, _ := getS(item, colEventID)
	isCreated, _ := getBool(item, colIsCreated)
	traceparent, _ := getS(item, colTraceparent)
	tracestate, _ := getS(item, colTracestate)

	return &es.EventEnvelope{
		EventID:       eventID,
		EventTypeName: typeName,
		AggregateID:   id,
		SeqNr:         seqNr,
		IsCreated:     isCreated,
		// #nosec G115 -- UnixMilli timestamp fits within int64 for any realistic value
		OccurredAt:  time.UnixMilli(int64(occurred)).UTC(),
		Payload:     payload,
		TraceParent: traceparent,
		TraceState:  tracestate,
	}, nil
}

func (s *Store) unmarshalSnapshot(item map[string]types.AttributeValue) (*es.SnapshotEnvelope, error) {
	aidStr, _ := getS(item, colAid)
	id, err := parseAggregateIDKey(aidStr)
	if err != nil {
		return nil, err
	}
	seqNr, err := getN(item, colSeqNr)
	if err != nil {
		return nil, err
	}
	version, err := getN(item, colVersion)
	if err != nil {
		return nil, err
	}
	payload, _ := getB(item, colPayload)
	occurred, _ := getN(item, colOccurred)

	return &es.SnapshotEnvelope{
		AggregateID: id,
		SeqNr:       seqNr,
		Version:     version,
		Payload:     payload,
		// #nosec G115
		OccurredAt: time.UnixMilli(int64(occurred)).UTC(),
	}, nil
}

// --- low-level helpers ---

func getS(item map[string]types.AttributeValue, key string) (string, bool) {
	v, ok := item[key].(*types.AttributeValueMemberS)
	if !ok {
		return "", false
	}
	return v.Value, true
}
func getB(item map[string]types.AttributeValue, key string) ([]byte, bool) {
	v, ok := item[key].(*types.AttributeValueMemberB)
	if !ok {
		return nil, false
	}
	return v.Value, true
}
func getBool(item map[string]types.AttributeValue, key string) (bool, bool) {
	v, ok := item[key].(*types.AttributeValueMemberBOOL)
	if !ok {
		return false, false
	}
	return v.Value, true
}
func getN(item map[string]types.AttributeValue, key string) (uint64, error) {
	v, ok := item[key].(*types.AttributeValueMemberN)
	if !ok {
		return 0, fmt.Errorf("attribute %s missing or wrong type", key)
	}
	n, err := strconv.ParseUint(v.Value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("attribute %s parse: %w", key, err)
	}
	return n, nil
}

// parseAggregateIDKey reverses Resolver.ResolveAggregateIDKey == "{TypeName}-{Value}".
// We split on the first dash; aggregate IDs may legitimately contain dashes
// (e.g., ULIDs), so use the prefix only as type discriminator.
func parseAggregateIDKey(s string) (es.AggregateID, error) {
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			return es.NewAggregateID(s[:i], s[i+1:]), nil
		}
	}
	return nil, fmt.Errorf("invalid aggregate id key: %q", s)
}
```

- [ ] **Step 4: テスト合格を確認**

Run: `cd v2 && go test ./dynamodb/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add v2/dynamodb/store.go v2/dynamodb/store_test.go
git commit -m "feat(v2/dynamodb): implement PersistEvent + queries + (un)marshal helpers"
```

---

### Task 23: dynamodb.Store.PersistEventAndSnapshot のテスト追加

**Files:**
- Modify: `v2/dynamodb/store_test.go`

PersistEventAndSnapshot 本体は前タスクで実装済み。本タスクでは TransactWriteItems が期待通り組み立てられることを確認するテストのみ追加。

- [ ] **Step 1: テストを追加**

`v2/dynamodb/store_test.go` の末尾に追記：
```go
func TestStore_PersistEventAndSnapshot_initialWriteUsesAttributeNotExists(t *testing.T) {
	c := &fakeClient{}
	store := v2ddb.New(c, v2ddb.DefaultConfig())
	id := es.NewAggregateID("Visit", "x")
	ev := &es.EventEnvelope{
		EventID:       "ev-2",
		EventTypeName: "Test",
		AggregateID:   id,
		SeqNr:         5,
		IsCreated:     false,
		OccurredAt:    time.Now().UTC(),
		Payload:       []byte(`{}`),
	}
	snap := &es.SnapshotEnvelope{
		AggregateID: id,
		SeqNr:       5,
		Version:     1,
		Payload:     []byte(`{"state":"x"}`),
		OccurredAt:  time.Now().UTC(),
	}

	if err := store.PersistEventAndSnapshot(context.Background(), ev, snap); err != nil {
		t.Fatalf("PersistEventAndSnapshot: %v", err)
	}
	if c.lastTrans == nil {
		t.Fatalf("TransactWriteItems not called")
	}
	if len(c.lastTrans.TransactItems) != 2 {
		t.Errorf("TransactItems count: got %d, want 2", len(c.lastTrans.TransactItems))
	}
	snapPut := c.lastTrans.TransactItems[1].Put
	if snapPut == nil {
		t.Fatalf("snapshot put is nil")
	}
	if got := awssdk.ToString(snapPut.ConditionExpression); got != "attribute_not_exists(#version)" {
		t.Errorf("initial-write ConditionExpression: got %q, want %q",
			got, "attribute_not_exists(#version)")
	}
}

func TestStore_PersistEventAndSnapshot_subsequentWriteUsesVersionCondition(t *testing.T) {
	c := &fakeClient{}
	store := v2ddb.New(c, v2ddb.DefaultConfig())
	id := es.NewAggregateID("Visit", "x")
	ev := &es.EventEnvelope{
		EventID:     "ev-3",
		AggregateID: id,
		SeqNr:       10,
		OccurredAt:  time.Now().UTC(),
		Payload:     []byte(`{}`),
	}
	snap := &es.SnapshotEnvelope{
		AggregateID: id,
		SeqNr:       10,
		Version:     3, // expected = 2
		Payload:     []byte(`{}`),
		OccurredAt:  time.Now().UTC(),
	}

	if err := store.PersistEventAndSnapshot(context.Background(), ev, snap); err != nil {
		t.Fatalf("PersistEventAndSnapshot: %v", err)
	}
	snapPut := c.lastTrans.TransactItems[1].Put
	got := awssdk.ToString(snapPut.ConditionExpression)
	want := "attribute_not_exists(#version) OR #version = :expected"
	if got != want {
		t.Errorf("subsequent-write ConditionExpression: got %q, want %q", got, want)
	}
	expected, ok := snapPut.ExpressionAttributeValues[":expected"].(*types.AttributeValueMemberN)
	if !ok || expected.Value != "2" {
		t.Errorf(":expected value: got %+v, want N=2", snapPut.ExpressionAttributeValues[":expected"])
	}
}
```

- [ ] **Step 2: テスト合格を確認**

Run: `cd v2 && go test ./dynamodb/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add v2/dynamodb/store_test.go
git commit -m "test(v2/dynamodb): cover PersistEventAndSnapshot condition expressions"
```

---

### Task 24: dynamodb.Store.GetEventsSince の unmarshal を確認

**Files:**
- Modify: `v2/dynamodb/store_test.go`

- [ ] **Step 1: テストを追加**

`v2/dynamodb/store_test.go` の末尾に追記：
```go
func TestStore_GetEventsSince_unmarshalsCorrectly(t *testing.T) {
	c := &fakeClient{
		queryResp: &ddb.QueryOutput{
			Items: []map[string]types.AttributeValue{
				{
					"pkey":        &types.AttributeValueMemberS{Value: "Visit-0"},
					"skey":        &types.AttributeValueMemberS{Value: "Visit-x-00000000000000000001"},
					"aid":         &types.AttributeValueMemberS{Value: "Visit-x"},
					"seq_nr":      &types.AttributeValueMemberN{Value: "1"},
					"payload":     &types.AttributeValueMemberB{Value: []byte(`{"k":"v"}`)},
					"occurred_at": &types.AttributeValueMemberN{Value: "1700000000000"},
					"type_name":   &types.AttributeValueMemberS{Value: "VisitScheduled"},
					"event_id":    &types.AttributeValueMemberS{Value: "ev-1"},
					"is_created":  &types.AttributeValueMemberBOOL{Value: true},
				},
			},
		},
	}
	store := v2ddb.New(c, v2ddb.DefaultConfig())
	id := es.NewAggregateID("Visit", "x")

	envs, err := store.GetEventsSince(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsSince: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("len: got %d, want 1", len(envs))
	}
	got := envs[0]
	if got.SeqNr != 1 || got.EventID != "ev-1" || got.EventTypeName != "VisitScheduled" || !got.IsCreated {
		t.Errorf("envelope mismatch: %+v", got)
	}
	if got.AggregateID.AsString() != "Visit-x" {
		t.Errorf("AggregateID: got %q, want Visit-x", got.AggregateID.AsString())
	}
	if string(got.Payload) != `{"k":"v"}` {
		t.Errorf("Payload: got %q, want %q", got.Payload, `{"k":"v"}`)
	}
}
```

- [ ] **Step 2: テスト合格を確認**

Run: `cd v2 && go test ./dynamodb/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add v2/dynamodb/store_test.go
git commit -m "test(v2/dynamodb): cover GetEventsSince unmarshal"
```

---

## Phase 12: v1 Deprecation

### Task 25: v1 の公開シンボルに Deprecated コメント追加

**Files:**
- Modify: `types.go`, `event_store.go`, `errors.go`, `serializer.go`, `memory/store.go`, `dynamodb/store.go`

v1 は API 非破壊で残す。利用者を v2 に誘導するため Deprecated コメントを各公開シンボルに追加。

- [ ] **Step 1: types.go の各公開型に追加**

`types.go` の `AggregateID`, `DefaultAggregateID`, `Event`, `DefaultEvent`, `Aggregate`, `AggregateResult`, `SnapshotData`, `NewAggregateId`, `NewEvent`, `EventOption`, `WithSeqNr`, `WithIsCreated`, `WithOccurredAt`, `WithOccurredAtUnixMilli` の **doc コメント先頭** に：

```go
// Deprecated: use github.com/Hiroshi0900/eventstore/v2 instead.
```

例：
```go
// AggregateID represents the unique identifier of an aggregate.
// It combines a type name (e.g. "MemorialSetting") with a unique value (e.g. a ULID).
//
// Deprecated: use github.com/Hiroshi0900/eventstore/v2 instead.
type AggregateID interface {
```

- [ ] **Step 2: 同様に他のファイルにも追加**

- `event_store.go`: `EventStore`, `EventStoreConfig`, `DefaultEventStoreConfig`, `Repository`, `AggregateFactory`, `DefaultRepository`, `NewRepository`
- `errors.go`: 全 sentinel と error 構造体 + コンストラクタ
- `serializer.go`: `Serializer`, `JSONSerializer`, `NewJSONSerializer`
- `memory/store.go`: `Store`, `New`, 公開メソッド全部
- `dynamodb/store.go`: `Store`, `Client`, `Config`, `DefaultConfig`, `New`, 公開メソッド全部

- [ ] **Step 3: ビルド確認**

Run: `go build ./...`
Expected: エラーなし（コメントだけの変更）

- [ ] **Step 4: 既存テストが通ることを確認**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add types.go event_store.go errors.go serializer.go memory/store.go dynamodb/store.go
git commit -m "chore: deprecate v1 in favor of v2"
```

---

## Phase 13: Documentation

### Task 26: README に v2 セクション追加

**Files:**
- Modify: `README.md`

- [ ] **Step 1: README の現状を確認**

```bash
cat /Users/sakemihiroshi/develop/terrat/eventstore/README.md
```

- [ ] **Step 2: v2 セクションを追記**

`README.md` の末尾に追記：

```markdown
## v2

v2 は `github.com/Hiroshi0900/eventstore/v2` サブモジュールとして提供されます。
v1 のメタ情報がドメインに混在する課題を解消し、Aggregate / Event / Command を
ビジネス情報のみのピュアな型として定義できます。

### Quick start

```go
import (
    es "github.com/Hiroshi0900/eventstore/v2"
    "github.com/Hiroshi0900/eventstore/v2/memory"
)

// 1. ドメイン側で Aggregate / Event / Command と Serializer を実装
//    (詳細は docs/superpowers/specs/2026-04-26-eventstore-v2-design.md §7 参照)

// 2. Repository を構築
store := memory.New()
repo := es.NewRepository[Visit, VisitEvent](
    store,
    func(id es.AggregateID) Visit { return EmptyVisit{id: id.(VisitID)} },
    visitAggregateSerializer{},
    visitEventSerializer{},
    es.DefaultConfig(),
)

// 3. ユースケース層は aggID と Command を渡すだけ
visit, err := repo.Save(ctx, visitID, ScheduleVisitCommand{ScheduledAt: t})
```

### v1 との主な違い

- ドメイン側の Aggregate / Event / Command がメタ情報を持たない
- ライブラリ側の `EventEnvelope` / `SnapshotEnvelope` がメタ情報を持つ
- `Repository.Save(aggID, cmd)` で load → ApplyCommand → ApplyEvent → 永続化を一括
- `AggregateFactory[T]` を廃止、`AggregateSerializer[T,E]` + `EventSerializer[E]` + `createBlank` 関数に分離
- 状態別型 (state pattern) を `ApplyEvent(E) Aggregate[E]` で正式サポート

詳細仕様: `docs/superpowers/specs/2026-04-26-eventstore-v2-design.md`
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add v2 section to README"
```

---

## 完了チェックリスト

実装完了の確認：

- [ ] `cd v2 && go build ./...` がエラーなし
- [ ] `cd v2 && go test ./...` が全 PASS
- [ ] `cd v2 && go vet ./...` がエラーなし
- [ ] `go build ./...` (v1) がエラーなし
- [ ] `go test ./...` (v1) が PASS
- [ ] `go vet ./...` (v1) がエラーなし
- [ ] v1 の全公開シンボルに `// Deprecated:` コメントが付いている
- [ ] README に v2 セクションがある
- [ ] git log で各 Task が個別の commit になっている

---

## 注意事項

- **Go 1.25** のジェネリクスを前提（v1 と同じ）
- **assertion ライブラリ非使用**：標準 `testing` のみ。`testify` 等は導入しない
- **外部 ULID/UUID 依存なし**：`crypto/rand` + hex で代替
- **v1 と v2 を同じパッケージ名 `eventsourcing` で揃える**：import パスの違いだけで混乱させない
- **TDD 厳守**：テスト → 失敗確認 → 実装 → 合格確認 → コミット のサイクル
- **小さい commit**：1 タスク 1 commit、メッセージは Conventional Commits（`feat(v2):`, `test(v2):`, `chore:`, `docs:` 等）
- **terrat-go-app の v2 移行は別計画**：本計画は v2 ライブラリ単体の構築のみ
