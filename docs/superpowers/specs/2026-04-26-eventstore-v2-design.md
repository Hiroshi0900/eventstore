# eventstore v2 設計仕様

- **作成日**: 2026-04-26
- **改訂日**: 2026-04-27（Domain Purity Revision）
- **対象リポジトリ**: `github.com/Hiroshi0900/eventstore`
- **参照**: [yoshiyoshifujii/go-eventstore](https://github.com/yoshiyoshifujii/go-eventstore)
- **利用先**: `~/develop/terrat/terrat-go-app`（visit / memorial_setting / account ドメイン）

---

## 1. 目的

参照リポジトリ (yoshiyoshifujii/go-eventstore) のドメインモデル設計思想を取り込み、現状リポの本番志向の資産（DynamoDB 実装・スナップショット・楽観的ロック・OTel 統合）を維持したまま、API を v2 として再構築する。

加えて **Domain Purity Revision** として、ドメイン側の型 (Aggregate / Event / Command) からデータストアのメタ情報（SeqNr / Version / EventID / OccurredAt / IsCreated 等）を完全排除する。利用側は「業務概念としての state と振る舞い」のみを実装し、永続化に関するメタデータはライブラリ側の Envelope が担う。

### 取り込みたい設計思想

| 項目 | 内容 |
|---|---|
| Command 抽象 | `Command` を第一級 interface とし、`Repository.Save(ctx, aggID, cmd)` でユースケース層を薄くする |
| 集約に責務集約 | `ApplyCommand` / `ApplyEvent` を集約自身のメソッドにする（Tell-Don't-Ask） |
| State Pattern サポート | `ApplyEvent(E) Aggregate[E]` で interface 戻りにし、状態遷移時に別の型を返せる |
| **ドメインの純粋性** | Aggregate / Event / Command はビジネス情報のみ。SeqNr / Version 等のメタは持たせない |

### 維持したい既存資産

- ジェネリクス `Repository[T Aggregate[E], E Event]`
- DynamoDB 実装（シャーディング・トランザクション・OTel propagation）
- スナップショット戦略（`SnapshotInterval`）
- 楽観的ロック（version + ConditionExpression）
- エラー型ファミリー、Serializer 抽象、OpenTelemetry 統合

---

## 2. 背景

### 現状リポ (v1) の課題

- **Command 概念がない**: 利用側 (terrat-go-app) では「状態別メソッド (`agg.Reschedule(...)`)」を直接呼んでおり、ユースケース層に手続き的なコードが残る
- **`AggregateFactory[T]` の責務分散**: `Apply` (イベント適用) が集約外 (Factory) にあり、ドメインロジックが集約に閉じない
- **State Pattern が利用側で実現済みだがライブラリ側のサポートがない**: `Visit` ドメインは既に `VisitScheduled`/`VisitCompleted`/`VisitRescheduled`/`VisitCanceled`/`VisitResting` の状態別型で構成されているが、ライブラリ側の `Aggregate` interface は単一型前提
- **メタ情報がドメインに混在**: `SeqNr` / `Version` / `EventID` / `OccurredAt` / `IsCreated` を Aggregate / Event interface が要求するため、利用側はこれらをフィールドとして持ち回らざるを得ない

### 参照リポの良い設計と限界

- **良い**: `Command` + `ApplyCommand`/`ApplyEvent` を集約に集約、状態別型サポート、`SeqNr` 値オブジェクト化、Empty() による nil-safe
- **限界**: 永続化・並行制御・スナップショット・OTel など本番要件が薄い（メモリ実装のみ）

### v2 のスタンス

両者のいいとこ取り。ドメインモデル化は参照リポ流に、永続化・並行制御は現状リポの資産を継承。さらに「ドメインからメタを完全排除」して純度を高める。

---

## 3. スコープ

### v2 で実施する

- v2 サブモジュール (`github.com/Hiroshi0900/eventstore/v2`) として新規構築
- `Aggregate[E Event]` interface に `ApplyCommand` / `ApplyEvent` を持たせる
- `Command` interface を新規導入（`CommandTypeName()` のみ）
- ドメイン側 `Event` interface からメタ情報を排除（`EventTypeName()` + `AggregateID()` のみ）
- ライブラリ側に `EventEnvelope` / `SnapshotEnvelope` を導入し、メタ情報をすべてここに持たせる
- `AggregateFactory[T]` を廃止し、`createBlank` 関数 + 2 つの Serializer interface に分離
- メソッド名から `Get` プレフィックスを除去（参照リポ流）
- `Repository.Save(ctx, aggID, cmd) (T, error)` に統一（load → ApplyCommand → ApplyEvent → 永続化を内部で一括）
- 補助 API として `Repository.Load(ctx, aggID) (T, error)` を提供（読み取り専用）
- v1 を `// Deprecated: ...` 化（API 変更なし、互換維持）
- in-memory / DynamoDB 実装を v2 用に再構築（API 変更に追従）

### v2 では実施しない

- **`SeqNr` 値オブジェクト化**: 別計画として温める。v2 では `uint64` のまま
- **`Empty()` パターン**: Go 慣習に合わないため不採用（`error` で nil-safe を達成）
- **スナップショット廃止**: 性能上の保険として現状維持
- **Postgres 実装**: 利用側 (terrat-go-app) で持っているため、v2 用に追従するのは利用側の責任
- **新しい永続化バックエンド**: 既存の DynamoDB と memory のみ

---

## 4. 決定事項サマリ

| # | 項目 | 決定 |
|---|---|---|
| 1 | 互換戦略 | v2 サブモジュール（`/v2/go.mod`） |
| 2 | 1コマンドあたりのイベント | **1イベント固定** |
| 3 | `Apply` 系メソッドの所在 | 集約自身が `ApplyCommand` / `ApplyEvent` を持つ |
| 4 | State Pattern サポート | `ApplyEvent(E) Aggregate[E]` で interface 戻り |
| 5 | Factory の運命 | **廃止**。`createBlank` 関数 + 2 つの Serializer interface |
| 6 | `Empty()` パターン | **不採用**（ただし `createBlank` で「初期化前状態」を表現） |
| 7 | `SeqNr` 値オブジェクト化 | 別計画（v2 では `uint64` のまま） |
| 8 | Snapshot 戦略 | 現状維持 (`SnapshotInterval`) |
| 9 | 楽観的ロック | snapshot version 一本（`PersistEventAndSnapshot` 時のみ） |
| 10 | ドメイン Event のメタ情報 | **完全排除**。`EventTypeName()` と `AggregateID()` のみ |
| 11 | ドメイン Aggregate のメタ情報 | **完全排除**。`AggregateID()` + `ApplyCommand` + `ApplyEvent` のみ |
| 12 | メタ情報の保持場所 | `EventEnvelope` / `SnapshotEnvelope`（library 側） |
| 13 | Repository API | `Save(ctx, aggID, cmd)` + `Load(ctx, aggID)` |
| 14 | Command interface | `CommandTypeName()` のみ（`AggregateID()` は持たない） |
| 15 | ジェネリクス | `Aggregate[E Event]` で Event 型を伝播 |
| 16 | メソッド命名 | `Get` プレフィックス削除（`GetID()` → `AggregateID()` 等） |

---

## 5. 詳細設計

### 5.1 ディレクトリ構成

```
eventstore/
├── go.mod                           # v1（既存、Deprecated 化）
├── (既存ファイル群、API 変更なし、Deprecated コメント追加)
└── v2/
    ├── go.mod                       # module github.com/Hiroshi0900/eventstore/v2
    ├── aggregate.go                 # Aggregate[E], AggregateID interface
    ├── event.go                     # Event interface
    ├── command.go                   # Command interface
    ├── envelope.go                  # EventEnvelope, SnapshotEnvelope
    ├── serializer.go                # AggregateSerializer[T,E], EventSerializer[E]
    ├── repository.go                # Repository[T,E], DefaultRepository
    ├── event_store.go               # EventStore interface, Config
    ├── errors.go                    # エラー型
    ├── memory/
    │   └── store.go                 # in-memory 実装
    ├── dynamodb/
    │   └── store.go                 # DynamoDB 実装
    └── internal/
        ├── keyresolver/
        └── envelope/                # wire-format struct（DynamoDB attribute 用）
```

### 5.2 ドメイン側 interface

#### 5.2.1 AggregateID

```go
type AggregateID interface {
    TypeName() string
    Value() string
    AsString() string  // "{TypeName}-{Value}"
}
```

`v1` の `DefaultAggregateID` 相当を `v2` でも提供する（メソッド名のみ変更）。

#### 5.2.2 Aggregate

```go
type Aggregate[E Event] interface {
    AggregateID() AggregateID
    ApplyCommand(Command) (E, error)
    ApplyEvent(E) Aggregate[E]
}
```

設計のポイント：

- **メタ情報を一切持たない**: `SeqNr()` / `Version()` / `WithSeqNr()` / `WithVersion()` を含めない
- `ApplyCommand` の戻り値は **単一の E**（1コマンド1イベント原則 + 型安全）
- `ApplyEvent` の戻り値は `Aggregate[E]`（interface）：状態遷移時に別の具象型を返せる（state pattern）
- ジェネリクス `[E Event]` により、Visit ドメインは `Aggregate[VisitEvent]`、Memorial ドメインは `Aggregate[MemorialEvent]` のように specialize できる

#### 5.2.3 Event

```go
type Event interface {
    EventTypeName() string
    AggregateID() AggregateID
}
```

設計のポイント：

- **メタ情報を一切持たない**: `SeqNr()` / `EventID()` / `IsCreated()` / `OccurredAt()` / `Payload()` / `WithSeqNr()` を含めない
- これらはすべて `EventEnvelope`（library 側）が保持する
- `EventTypeName()` は serializer の dispatch・OTel スパン名・監査用
- `AggregateID()` はイベントがどの集約に属するかを示すドメイン情報（メタではなく業務的な所属）
- 業務的に「いつ発生したか」が必要な場合は、ドメイン Event 自体のフィールドとして持たせる（例: `VisitScheduledEvent.scheduledAt`）。`OccurredAt`（永続化時刻）とは別概念

#### 5.2.4 Command

```go
type Command interface {
    CommandTypeName() string
}
```

設計のポイント：

- `AggregateID()` は持たない（`Repository.Save(ctx, aggID, cmd)` で `aggID` が独立に渡されるため）
- `CommandTypeName()` は OTel スパン名・監査・ロギング用

### 5.3 ドメイン側 Serializer

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

設計のポイント：

- **Factory の責務を Serializer に集約**: v1 の `AggregateFactory[T]` の `Serialize` / `Deserialize` を `AggregateSerializer[T, E]` に移し、`Create` は `createBlank func(AggregateID) T` に分離、`Apply` は集約自身のメソッドに移す
- `EventSerializer.Deserialize` は `typeName` を引数に取り、利用側が switch で具象型に dispatch する
- 制約 `T Aggregate[E], E Event` により、Repository / Serializer / Aggregate / Event の 4 者の整合性が compile error で保証される
- 利用側の serializer 実装 (`visitAggregateSerializer`, `visitEventSerializer`) は struct で持ち、`NewRepository` に渡す

### 5.4 ライブラリ側 Envelope

```go
type EventEnvelope struct {
    EventID       string
    EventTypeName string
    AggregateID   AggregateID
    SeqNr         uint64
    IsCreated     bool
    OccurredAt    time.Time
    Payload       []byte           // EventSerializer.Serialize の結果
    TraceParent   string           // OTel 伝搬
    TraceState    string
}

type SnapshotEnvelope struct {
    AggregateID AggregateID
    SeqNr       uint64
    Version     uint64
    Payload     []byte             // AggregateSerializer.Serialize の結果
    OccurredAt  time.Time
}
```

設計のポイント：

- **ドメインから剥がしたメタ情報をすべてここに集約**
- `EventEnvelope.Payload` には `EventSerializer.Serialize` で得たドメイン Event の bytes が入る
- `SnapshotEnvelope.Payload` には `AggregateSerializer.Serialize` で得たドメイン Aggregate の bytes が入る
- `Version` は **snapshot 世代カウンタ**（楽観的ロックの基準）。`EventEnvelope` には載らない
- OTel `traceparent` / `tracestate` は `EventEnvelope` のフィールドとして明示的に持たせる（v1 は context 経由で暗黙的に注入していたが、v2 は型レベルで可視化）

### 5.5 Repository

```go
type Repository[T Aggregate[E], E Event] interface {
    Load(ctx context.Context, aggID AggregateID) (T, error)
    Save(ctx context.Context, aggID AggregateID, cmd Command) (T, error)
}

type DefaultRepository[T Aggregate[E], E Event] struct {
    store         EventStore
    createBlank   func(AggregateID) T
    aggSerializer AggregateSerializer[T, E]
    evSerializer  EventSerializer[E]
    config        Config
}

func NewRepository[T Aggregate[E], E Event](
    store         EventStore,
    createBlank   func(AggregateID) T,
    aggSerializer AggregateSerializer[T, E],
    evSerializer  EventSerializer[E],
    config        Config,
) *DefaultRepository[T, E]
```

#### 5.5.1 Load の動作

```
1. store.GetLatestSnapshot(ctx, aggID) で SnapshotEnvelope を取得
2. snapshot があれば:
     agg = aggSerializer.Deserialize(snapshot.Payload)
     currentSeqNr = snapshot.SeqNr
     currentVersion = snapshot.Version
   なければ:
     agg = createBlank(aggID)
     currentSeqNr = 0
     currentVersion = 0
3. store.GetEventsSince(ctx, aggID, currentSeqNr) で []*EventEnvelope を取得
4. 各 envelope に対し:
     ev = evSerializer.Deserialize(envelope.EventTypeName, envelope.Payload)
     agg = agg.ApplyEvent(ev).(T)  // 型アサーションは library 内
5. snapshot も event も無ければ ErrAggregateNotFound を返す
6. agg を返す
```

#### 5.5.2 Save の動作

```
1. Load と同様に内部で agg / currentSeqNr / currentVersion を取得
   ただし「集約が存在しない」ケースでもエラーにせず、blank + (0, 0) で開始
   （creation コマンドを受け付けるため）
2. ev, err := agg.ApplyCommand(cmd)
3. nextAgg := agg.ApplyEvent(ev).(T)
4. nextSeqNr := currentSeqNr + 1
5. EventEnvelope を組み立て:
     Payload = evSerializer.Serialize(ev)
     EventID = ULID 等で生成
     EventTypeName = ev.EventTypeName()
     AggregateID = ev.AggregateID()
     SeqNr = nextSeqNr
     IsCreated = (currentSeqNr == 0)
     OccurredAt = time.Now()
     TraceParent / TraceState = OTel context から抽出
6. config.ShouldSnapshot(nextSeqNr) を判定:
     true → SnapshotEnvelope 組み立て:
              Payload = aggSerializer.Serialize(nextAgg)
              SeqNr = nextSeqNr
              Version = currentVersion + 1
              OccurredAt = time.Now()
            store.PersistEventAndSnapshot(ctx, evEnvelope, snapEnvelope)
     false → store.PersistEvent(ctx, evEnvelope, currentVersion)
7. nextAgg を返す
```

設計のポイント：

- **Load と Save の両方が内部で「集約のロード」を行う**: ユースケース層は aggID と cmd を渡すだけでよい
- **「集約が存在しない」ケースの挙動が Load と Save で異なる**:
    - Load: `ErrAggregateNotFound` を返す
    - Save: blank で開始（creation を許可）
- **型アサーション `(T)` は library 内に閉じ込める**: 利用側は型アサーションを書かない
- **`IsCreated` は library が機械的に判定**（`currentSeqNr == 0`）: ドメインに「これは作成イベント？」を判断させない

### 5.6 EventStore interface

```go
type EventStore interface {
    GetLatestSnapshot(ctx context.Context, id AggregateID) (*SnapshotEnvelope, error)
    GetEventsSince(ctx context.Context, id AggregateID, seqNr uint64) ([]*EventEnvelope, error)
    PersistEvent(ctx context.Context, ev *EventEnvelope, expectedVersion uint64) error
    PersistEventAndSnapshot(ctx context.Context, ev *EventEnvelope, snap *SnapshotEnvelope) error
}
```

設計のポイント：

- **ドメイン型を一切扱わない**: 引数・戻り値はすべて `*EventEnvelope` / `*SnapshotEnvelope` / `AggregateID`
- DynamoDB 実装と memory 実装の差は store.go 内に閉じ、ドメイン側の変更を要求しない
- `expectedVersion` は `PersistEvent` では「初回作成かどうかの識別」専用（`0` なら他にイベントが存在してはならない、それ以外は通常の追記）。snapshot 一本で楽観的ロックを行う方針

### 5.7 楽観的ロックの方針

楽観的ロックは **`PersistEventAndSnapshot` の `snapshot.Version` 一本**で行う。`PersistEvent` は楽観的ロックを行わず、以下のチェックのみ：

- 既に同じ `(AggregateID, SeqNr)` のイベントが存在しないこと
  - DynamoDB: `attribute_not_exists(#pk)` の Condition
  - memory: 既存イベントの SeqNr 衝突チェック
- 初回イベント（`expectedVersion == 0`）は、その集約のイベントが他に存在しないこと（`ErrDuplicateAggregate`）

#### Snapshot Interval 中の並行更新

`SnapshotInterval` の間（snapshot を作らないイベント）は楽観的ロックが発生しないため、極めて短いウィンドウで並行更新が起きる可能性がある。これは **イベントの SeqNr 衝突チェックで防がれる**：同じ SeqNr のイベントを 2 つ書き込もうとすると、後続が `ErrDuplicateAggregate` で失敗する。

### 5.8 Snapshot 戦略

v1 から **基本変更なし**。

```go
type Config struct {
    SnapshotInterval  uint64  // default: 5
    JournalTableName  string
    SnapshotTableName string
    ShardCount        uint64
    KeepSnapshotCount uint64
}

func (c Config) ShouldSnapshot(seqNr uint64) bool {
    if c.SnapshotInterval == 0 {
        return false
    }
    return seqNr%c.SnapshotInterval == 0
}
```

`Repository.Save` 内で `seqNr % SnapshotInterval == 0` のときに `PersistEventAndSnapshot` を選択。

### 5.9 createBlank と creation 処理

利用側は `createBlank(id AggregateID) T` で「初期化前状態を表す Aggregate」を提供する。この状態の `ApplyCommand` は creation 系コマンドのみを受け付け、それ以外は `ErrUnknownCommand` を返す。

```go
type EmptyVisit struct {
    id VisitID
}

func (e EmptyVisit) AggregateID() es.AggregateID { return e.id }

func (e EmptyVisit) ApplyCommand(cmd es.Command) (VisitEvent, error) {
    switch c := cmd.(type) {
    case ScheduleVisitCommand:
        return VisitScheduledEvent{aggID: e.id, scheduledAt: c.ScheduledAt}, nil
    default:
        return nil, es.ErrUnknownCommand
    }
}

func (e EmptyVisit) ApplyEvent(ev VisitEvent) es.Aggregate[VisitEvent] {
    switch v := ev.(type) {
    case VisitScheduledEvent:
        return VisitScheduled{id: e.id, scheduledAt: v.scheduledAt}
    }
    return e
}
```

設計のポイント：

- 参照リポの `Empty()` パターンを Go 流に再解釈：「Empty 状態」も他の状態と同等の Aggregate 実装として扱う
- `createBlank` は単なる関数（interface を作らない）
- creation 用コマンドの種類が複数あるなら、`EmptyVisit.ApplyCommand` の switch 内で分岐

### 5.10 OTel 統合

- v1 と同等：`Repository.Save` 内で span を作成（span 名は `cmd.CommandTypeName()` ベース）
- `EventEnvelope.TraceParent` / `TraceState` を context から抽出してセット
- DynamoDB 実装では PutItem の attribute としてシリアライズ
- memory 実装では追跡のみ（実際の伝搬はしない）

### 5.11 エラー型

v1 から基本継承 + 新規追加：

```go
var (
    ErrOptimisticLock        = errors.New("optimistic lock error")
    ErrAggregateNotFound     = errors.New("aggregate not found")
    ErrSerializationFailed   = errors.New("serialization failed")
    ErrDeserializationFailed = errors.New("deserialization failed")
    ErrInvalidEvent          = errors.New("invalid event")
    ErrInvalidAggregate      = errors.New("invalid aggregate")
    ErrEventStoreUnavailable = errors.New("event store unavailable")
    ErrDuplicateAggregate    = errors.New("aggregate already exists")
    ErrUnknownCommand        = errors.New("unknown command type for current state")
)
```

`ErrAggregateIDMismatch`（前 revision 案で検討していた）は、Command が `AggregateID()` を持たない方針のため不要。

---

## 6. 移行戦略

### 6.1 v1 の扱い

- v1 のすべての公開型・関数に `// Deprecated: use v2 instead` コメントを付与
- v1 の API 変更は **禁止**（破壊的変更を起こさない）
- v1 を使い続けるドメインは現状動作

### 6.2 利用側 (terrat-go-app) の段階移行

ドメイン単位で v2 に移行。**1 ドメインずつ完結させる**ことで、移行中にビルドが壊れない状態を維持する。

#### 推奨順序（移行コストの小さい順）

1. **account ドメイン** - 状態別型を使っていない、コマンド種別が少ない
2. **memorial_setting ドメイン** - 単一型集約、コマンド種別が中程度
3. **visit ドメイン** - 状態別型 (5 種類) でコマンド種別も多い、最大の移行対象

#### ドメイン移行手順

各ドメインで以下を実施：

1. `xxx_commands.go` を新規追加（Command 型を定義、`CommandTypeName()` を実装）
2. ドメインの Event interface (`VisitEvent` 等) を再定義し、`EventTypeName()` と `AggregateID()` のみを要求するように整理。既存の `SeqNr` / `EventID` / `OccurredAt` / `IsCreated` フィールドはドメインから削除
3. ドメインの Aggregate interface (`Visit` 等) を再定義し、`es.Aggregate[VisitEvent]` を embed。既存の `SeqNr` / `Version` フィールドはドメインから削除
4. 状態別型に `ApplyCommand(Command) (E, error)` `ApplyEvent(E) Aggregate[E]` を実装
5. 既存の状態別メソッド (`Reschedule`, `Complete` 等) を **`ApplyCommand` 内の switch 分岐に移植して削除**
6. `aggregate_factory.go` を `serializer.go` 群に置き換え：
    - `aggregate_serializer.go`: `AggregateSerializer[T, E]` 実装
    - `event_serializer.go`: `EventSerializer[E]` 実装
    - `createBlank` 関数を別途定義
7. usecase 層: `repo.Load(ctx, id) → 状態別メソッド呼び出し → repo.Save(ctx, agg, []Event{ev})` の流れを `repo.Save(ctx, id, cmd)` 1 行に置換
8. import パスを `github.com/Hiroshi0900/eventstore` から `github.com/Hiroshi0900/eventstore/v2` に変更
9. Postgres 実装も v2 用に追従（`EventEnvelope` / `SnapshotEnvelope` ベースのインターフェース変更に対応）

#### `visit_command_executor.go` の特殊対応

現状、複数イベント発行（`ScheduledEvent` + 業務イベント）をしている箇所を **2 回の `Save` 呼び出し** に分割する：

```go
// Before (v1)
events := []Event{scheduledEvent, businessEvent}
repo.Save(ctx, newState, events)

// After (v2)
visit, err := repo.Save(ctx, visitID, scheduleCmd)    // ScheduledEvent
if err != nil { ... }
visit, err = repo.Save(ctx, visitID, businessCmd)      // 業務イベント
```

トランザクション性は元々保証されていない（v1 の `Save` も内部で 1 イベントずつ `PersistEvent` をループしている）ため、挙動は変わらない。

### 6.3 v1 削除のタイミング

- 全利用ドメインの v2 移行完了後
- v1 を別ブランチにアーカイブ（`v1.x` ブランチ等）し、main からは削除
- セマンティックバージョニング上、メジャーバージョンの破棄として扱う

---

## 7. 利用例（v2）

### 7.1 ドメイン Event の定義

```go
package visit

import (
    "time"
    es "github.com/Hiroshi0900/eventstore/v2"
)

// Visit ドメインの Event interface
type VisitEvent interface {
    es.Event
    isVisitEvent()  // marker
}

// 具象 Event 型
type VisitScheduledEvent struct {
    aggID       VisitID
    scheduledAt time.Time
}

func (e VisitScheduledEvent) EventTypeName() string  { return "VisitScheduled" }
func (e VisitScheduledEvent) AggregateID() es.AggregateID { return e.aggID }
func (VisitScheduledEvent) isVisitEvent() {}

type VisitCompletedEvent struct {
    aggID VisitID
    note  string
}

func (e VisitCompletedEvent) EventTypeName() string  { return "VisitCompleted" }
func (e VisitCompletedEvent) AggregateID() es.AggregateID { return e.aggID }
func (VisitCompletedEvent) isVisitEvent() {}
```

### 7.2 ドメイン Command の定義

```go
type ScheduleVisitCommand struct {
    ScheduledAt time.Time
}
func (ScheduleVisitCommand) CommandTypeName() string { return "ScheduleVisit" }

type CompleteCommand struct {
    Note string
}
func (CompleteCommand) CommandTypeName() string { return "CompleteVisit" }
```

### 7.3 ドメイン Aggregate の定義

```go
// Visit ドメインの Aggregate interface
type Visit interface {
    es.Aggregate[VisitEvent]
    ID() VisitID
    IsActive() bool
}

// 「初期化前」状態（createBlank が返す）
type EmptyVisit struct {
    id VisitID
}

func (e EmptyVisit) AggregateID() es.AggregateID { return e.id }
func (e EmptyVisit) ID() VisitID                  { return e.id }
func (e EmptyVisit) IsActive() bool               { return false }

func (e EmptyVisit) ApplyCommand(cmd es.Command) (VisitEvent, error) {
    switch c := cmd.(type) {
    case ScheduleVisitCommand:
        return VisitScheduledEvent{aggID: e.id, scheduledAt: c.ScheduledAt}, nil
    default:
        return nil, es.ErrUnknownCommand
    }
}

func (e EmptyVisit) ApplyEvent(ev VisitEvent) es.Aggregate[VisitEvent] {
    if v, ok := ev.(VisitScheduledEvent); ok {
        return VisitScheduled{id: e.id, scheduledAt: v.scheduledAt}
    }
    return e
}

// 「予定済み」状態
type VisitScheduled struct {
    id          VisitID
    scheduledAt time.Time
}

func (v VisitScheduled) AggregateID() es.AggregateID { return v.id }
func (v VisitScheduled) ID() VisitID                  { return v.id }
func (v VisitScheduled) IsActive() bool               { return true }

func (v VisitScheduled) ApplyCommand(cmd es.Command) (VisitEvent, error) {
    switch c := cmd.(type) {
    case CompleteCommand:
        return VisitCompletedEvent{aggID: v.id, note: c.Note}, nil
    default:
        return nil, es.ErrUnknownCommand
    }
}

func (v VisitScheduled) ApplyEvent(ev VisitEvent) es.Aggregate[VisitEvent] {
    if e, ok := ev.(VisitCompletedEvent); ok {
        return VisitCompleted{id: v.id, note: e.note}
    }
    return v
}

// 「完了」状態
type VisitCompleted struct {
    id   VisitID
    note string
}

func (v VisitCompleted) AggregateID() es.AggregateID { return v.id }
func (v VisitCompleted) ID() VisitID                  { return v.id }
func (v VisitCompleted) IsActive() bool               { return false }
func (v VisitCompleted) ApplyCommand(cmd es.Command) (VisitEvent, error) {
    return nil, es.ErrUnknownCommand
}
func (v VisitCompleted) ApplyEvent(ev VisitEvent) es.Aggregate[VisitEvent] {
    return v
}
```

### 7.4 Serializer 実装

```go
type visitAggregateSerializer struct{}

func (visitAggregateSerializer) Serialize(v Visit) ([]byte, error) {
    // JSON 等で v の状態をエンコード（state 識別子 + フィールド）
    // ...
}

func (visitAggregateSerializer) Deserialize(data []byte) (Visit, error) {
    // state 識別子で具象型を分岐して返す
    // ...
}

type visitEventSerializer struct{}

func (visitEventSerializer) Serialize(ev VisitEvent) ([]byte, error) {
    // JSON 等で ev のフィールドをエンコード
    // ...
}

func (visitEventSerializer) Deserialize(typeName string, data []byte) (VisitEvent, error) {
    switch typeName {
    case "VisitScheduled":
        var e VisitScheduledEvent
        // ... unmarshal data into e
        return e, nil
    case "VisitCompleted":
        var e VisitCompletedEvent
        // ... unmarshal data into e
        return e, nil
    default:
        return nil, es.ErrDeserializationFailed
    }
}
```

### 7.5 Repository 構築

```go
repo := es.NewRepository[Visit, VisitEvent](
    store,
    func(id es.AggregateID) Visit { return EmptyVisit{id: id.(VisitID)} },
    visitAggregateSerializer{},
    visitEventSerializer{},
    es.DefaultConfig(),
)
```

### 7.6 usecase 層

```go
// 新規作成
visit, err := repo.Save(ctx, visitID, ScheduleVisitCommand{ScheduledAt: t})
if err != nil { return err }

// 既存集約を更新
visit, err = repo.Save(ctx, visitID, CompleteCommand{Note: "OK"})
if err != nil { return err }

// 状態を読み取りたいだけのとき
visit, err = repo.Load(ctx, visitID)
if err != nil { return err }
fmt.Println(visit.IsActive())
```

---

## 8. オープン課題

- **`SeqNr` 値オブジェクト化**: 別計画として進める。v2 リリース後に検討
- **Snapshot の Aggregate state 識別**: 状態別型を持つ Aggregate の `AggregateSerializer.Deserialize` は「どの状態か」を bytes 内に埋め込む必要がある。利用側の serializer 実装でタグ付けする規約を別途ドキュメント化したい
- **Postgres 実装の責任分界**: 利用側にあるため、v2 用の更新コストは利用側が負う
- **テスト戦略**: v2 のテストは現状リポと同等のカバレッジを目指す（in-memory ベースの Repository テスト + DynamoDB integration test）
- **ApplyEvent 内部の type assertion `(T)` の安全性**: `agg.ApplyEvent(ev)` が `Aggregate[E]` を返した後 library 内で `(T)` にキャストするが、利用側が `T` (= Visit interface) を満たさない値を返した場合に panic する。compile time に防げないため runtime check + 明確なエラーメッセージで対応
