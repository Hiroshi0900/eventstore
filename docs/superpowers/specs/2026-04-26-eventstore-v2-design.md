# eventstore v2 設計仕様

- **作成日**: 2026-04-26
- **対象リポジトリ**: `github.com/Hiroshi0900/eventstore`
- **参照**: [yoshiyoshifujii/go-eventstore](https://github.com/yoshiyoshifujii/go-eventstore)
- **利用先**: `~/develop/terrat/terrat-go-app`（visit / memorial_setting / account ドメイン）

---

## 1. 目的

参照リポジトリ (yoshiyoshifujii/go-eventstore) のドメインモデル設計思想を取り込み、現状リポの本番志向の資産（DynamoDB 実装・スナップショット・楽観的ロック・OTel 統合）を維持したまま、API を v2 として再構築する。

### 取り込みたい設計思想

| 項目 | 内容 |
|---|---|
| Command 抽象 | `Command` を第一級 interface とし、`Repository.Store(ctx, cmd, agg)` でユースケース層を薄くする |
| 集約に責務集約 | `ApplyCommand` / `ApplyEvent` を集約自身のメソッドにする（Tell-Don't-Ask） |
| State Pattern サポート | `ApplyEvent(Event) Aggregate` で interface 戻りにし、状態遷移時に別の型を返せる |

### 維持したい既存資産

- ジェネリクス `Repository[T Aggregate]`
- DynamoDB 実装（シャーディング・トランザクション・OTel propagation）
- スナップショット戦略（`SnapshotInterval`）
- 楽観的ロック（version + ConditionExpression）
- エラー型ファミリー、Serializer 抽象、OpenTelemetry 統合

---

## 2. 背景

### 現状リポの課題

- **Command 概念がない**: 利用側 (terrat-go-app) では「状態別メソッド (`agg.Reschedule(...)`)」を直接呼んでおり、ユースケース層に手続き的なコードが残る
- **`AggregateFactory[T]` の責務分散**: `Apply` (イベント適用) が集約外 (Factory) にあり、ドメインロジックが集約に閉じない
- **State Pattern が利用側で実現済みだがライブラリ側のサポートがない**: `Visit` ドメインは既に `VisitScheduled`/`VisitCompleted`/`VisitRescheduled`/`VisitCanceled`/`VisitResting` の状態別型で構成されているが、ライブラリ側の `Aggregate` interface は単一型前提

### 参照リポの良い設計と限界

- **良い**: `Command` + `ApplyCommand`/`ApplyEvent` を集約に集約、状態別型サポート、`SeqNr` 値オブジェクト化、Empty() による nil-safe
- **限界**: 永続化・並行制御・スナップショット・OTel など本番要件が薄い（メモリ実装のみ）

### v2 のスタンス

両者のいいとこ取り。ドメインモデル化は参照リポ流に、永続化・並行制御は現状リポの資産を継承。

---

## 3. スコープ

### v2 で実施する

- v2 サブモジュール (`github.com/Hiroshi0900/eventstore/v2`) として新規構築
- `Aggregate` interface に `ApplyCommand` / `ApplyEvent` を追加
- `Command` interface を新規導入
- `AggregateFactory[T]` を廃止し、`createBlank` 関数 + `AggregateSerializer[T]` interface に分離
- メソッド名から `Get` プレフィックスを除去（参照リポ流）
- `Repository.Save([]Event)` を `Repository.Store(cmd, agg) (T, error)` に置換（1コマンド1イベント）
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
| 4 | State Pattern サポート | `ApplyEvent(Event) Aggregate` で interface 戻り |
| 5 | Factory の運命 | **廃止**。`createBlank` 関数 + `AggregateSerializer[T]` |
| 6 | `Empty()` パターン | **不採用** |
| 7 | `SeqNr` 値オブジェクト化 | 別計画（v2 では `uint64` のまま） |
| 8 | Snapshot 戦略 | 現状維持 (`SnapshotInterval`) |
| 9 | 楽観的ロック | 現状維持 (Version + ConditionExpression) |
| 10 | Event payload | `[]byte` 維持（永続化層との疎結合） |
| 11 | メソッド命名 | `Get` プレフィックス削除（`GetID()` → `AggregateID()` 等） |

---

## 5. 詳細設計

### 5.1 ディレクトリ構成

```
eventstore/
├── go.mod                           # v1（既存、Deprecated 化）
├── (既存ファイル群、API 変更なし、Deprecated コメント追加)
└── v2/
    ├── go.mod                       # module github.com/Hiroshi0900/eventstore/v2
    ├── aggregate.go                 # Aggregate interface, AggregateID interface
    ├── command.go                   # Command interface
    ├── event.go                     # Event interface, DefaultEvent, EventOption
    ├── repository.go                # Repository[T], AggregateSerializer[T]
    ├── event_store.go               # EventStore interface, Config, SnapshotData
    ├── errors.go                    # エラー型
    ├── serializer.go                # JSON Serializer
    ├── memory/
    │   └── store.go                 # in-memory 実装
    ├── dynamodb/
    │   └── store.go                 # DynamoDB 実装
    └── internal/
        ├── keyresolver/
        └── envelope/
```

### 5.2 Aggregate interface

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

type AggregateID interface {
    TypeName() string
    Value() string
    AsString() string  // "{TypeName}-{Value}"
}
```

#### 設計のポイント

- `ApplyCommand` の戻り値は **単一の `Event`**（1コマンド1イベント原則）
- `ApplyEvent` の戻り値は **`Aggregate` interface**：状態遷移時に別の具象型を返せる
  - 利用側で `Visit interface { eventstore.Aggregate; ... }` を定義し、`VisitScheduled`/`VisitCompleted` 等が実装する規約
- `WithVersion` / `WithSeqNr` は `Repository` 内部で集約に新しい version/seqNr をセットする際に使用

### 5.3 Command interface

```go
type Command interface {
    CommandTypeName() string
    AggregateID() AggregateID
}
```

#### 設計のポイント

- `AggregateID()` を持つことで `Repository.Store(cmd, agg)` 時にターゲット集約と整合性チェック可能
- Command は `Empty()` を持たない（不採用方針に従う）

### 5.4 Event interface

```go
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
```

#### 設計のポイント

- `Payload() []byte` は永続化層との疎結合のために維持
- 利用側は具象 Event 型 ↔ `[]byte` の変換を `event_serializer.go` で実装（現状パターン継続）
- `OccurredAt` は `time.Time` を返す（現状の `uint64` Unix milli から変更）

### 5.5 Repository[T] と AggregateSerializer[T]

```go
type AggregateSerializer[T Aggregate] interface {
    Serialize(T) ([]byte, error)
    Deserialize([]byte) (T, error)
}

type Repository[T Aggregate] struct {
    store       EventStore
    createBlank func(AggregateID) T
    serializer  AggregateSerializer[T]
    config      Config
}

func NewRepository[T Aggregate](
    store       EventStore,
    createBlank func(AggregateID) T,
    serializer  AggregateSerializer[T],
    config      Config,
) *Repository[T]

// 集約をロードする。スナップショットがあればそれを起点に、なければ空集約から
// イベントを replay して再構築する。
// 集約も永続化されたイベントもなければ ErrAggregateNotFound を返す。
func (r *Repository[T]) Load(ctx context.Context, id AggregateID) (T, error)

// コマンドを集約に適用してイベントを生成し、永続化して新しい集約状態を返す。
// 動作フロー:
//   1. cmd.AggregateID() == agg.AggregateID() を検証（不一致なら ErrAggregateIDMismatch）
//   2. ev, err := agg.ApplyCommand(cmd) でイベントを生成
//   3. nextSeqNr = agg.SeqNr() + 1 を ev に WithSeqNr で適用
//   4. nextAgg := agg.ApplyEvent(ev) で集約を新状態に遷移（型アサーション (T) は内部）
//   5. nextSeqNr % SnapshotInterval == 0 の場合:
//        - serializer.Serialize(nextAgg) で snapshot payload を作成
//        - store.PersistEventAndSnapshot(ctx, ev, nextAgg) を呼ぶ
//      それ以外:
//        - store.PersistEvent(ctx, ev, agg.Version()) を呼ぶ
//   6. 永続化成功後、Version() を更新した nextAgg を返す
func (r *Repository[T]) Store(ctx context.Context, cmd Command, agg T) (T, error)
```

#### 設計のポイント

- **Factory 廃止**: 「空集約生成」と「シリアライゼーション」を別の関心事として分離
- `createBlank` は単なる関数（interface を実装する手間がない）
- `AggregateSerializer[T]` は interface（実装差し替え：JSON / Protobuf / msgpack）
- `Store` の戻り値は **`ApplyEvent` 後の新集約**：利用側は再構築不要
- `Repository[T]` 内部で `agg.ApplyEvent(ev).(T)` の型アサーションを行う（型アサーションを利用側に露出させない）

### 5.6 EventStore interface

```go
type EventStore interface {
    GetLatestSnapshotByID(ctx context.Context, id AggregateID) (*SnapshotData, error)
    GetEventsByIDSinceSeqNr(ctx context.Context, id AggregateID, seqNr uint64) ([]Event, error)
    PersistEvent(ctx context.Context, event Event, version uint64) error
    PersistEventAndSnapshot(ctx context.Context, event Event, snapshot SnapshotData) error
}

type SnapshotData struct {
    Payload []byte  // serialized aggregate state
    SeqNr   uint64
    Version uint64
}

#### 楽観的ロックの方針

楽観的ロックは **`PersistEventAndSnapshot` の `snapshot.Version` 一本**で行う。`PersistEvent` は楽観的ロックを行わず、以下のチェックのみ：

- 既に同じ `(AggregateID, SeqNr)` のイベントが存在しないこと（DynamoDB では `attribute_not_exists(#pk)` の Condition、memory では既存イベントの SeqNr 衝突チェック）
- 初回イベント（`version == 0` で渡された場合）は、その集約のイベントが他に存在しないこと（`ErrDuplicateAggregate`）

`PersistEvent` の `version` 引数は **「初回作成かどうかの識別」専用**であり、それ以外の値での楽観的ロック判定には使わない。これにより v1 DynamoDB（`version` 引数を実質無視していた）と意味論を揃える。

#### Snapshot Interval 中の並行更新

`SnapshotInterval` の間は楽観的ロックが発生しないため、極めて短いウィンドウで並行更新が起きる可能性がある。これは **イベントの SeqNr 衝突チェックで防がれる**：同じ SeqNr のイベントを 2 つ書き込もうとすると、後続が `ErrDuplicateAggregate`（または同等のエラー）で失敗する。


```

#### 設計のポイント

- 基本シグネチャは v1 と同等（DynamoDB / Postgres 実装の変更を最小化）
- 引数の `Aggregate` interface は v2 のものに変わる（メソッド名変更）
- **Snapshot payload の作成責務は `Repository` 側**: `EventStore` 実装は受け取った `Aggregate` の状態を `SnapshotData.Payload` として書き込むのみ。Serialize は `Repository.Store` 内で `AggregateSerializer` 経由で行い、`SnapshotData{Payload, SeqNr, Version}` を組み立てて `PersistEventAndSnapshot` に渡す
- v1 の DynamoDB 実装で `marshalSnapshot` が payload 未設定だった点は、v2 で `EventStore` の引数を `(ctx, event, snapshotData SnapshotData)` 形式に整理し、Repository 側で全フィールドを埋めて渡す形に統一

### 5.7 Snapshot 戦略

v1 から **変更なし**。

```go
type Config struct {
    SnapshotInterval  uint64  // default: 5
    JournalTableName  string
    SnapshotTableName string
    ShardCount        uint64
    KeepSnapshotCount uint64
}
```

`Repository.Store` 内で `seqNr % SnapshotInterval == 0` のときに `PersistEventAndSnapshot` を選択。

### 5.8 楽観的ロック

v1 から方針を整理：

- 集約の `Version()` は「**スナップショット世代カウンタ**」として位置づける
- `PersistEventAndSnapshot` 時に snapshot table の ConditionExpression `version = :expected_version` で楽観的ロック
- `PersistEvent` 単体では楽観的ロックを行わない（イベント SeqNr 衝突チェックのみ）
- 失敗時は `OptimisticLockError`

> **v1 との差**：v1 memory store は `PersistEvent` 内でも `versions` を更新していたが、v2 では「Version = snapshot 世代」の意味論に統一し、`PersistEvent` では更新しない。

### 5.9 エラー型

v1 から **基本変更なし**。エラー型を v2 パッケージにコピー。

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
)
```

加えて、Command 関連の新規エラー：

```go
var (
    ErrUnknownCommand     = errors.New("unknown command type for current state")
    ErrAggregateIDMismatch = errors.New("command aggregate ID does not match")
)
```

---

## 6. 移行戦略

### 6.1 v1 の扱い

- v1 のすべての公開型・関数に `// Deprecated: use v2 instead` コメントを付与
- v1 の API 変更は **禁止**（破壊的変更を起こさない）
- v1 を使い続けるドメインは現状動作

### 6.2 利用側 (terrat-go-app) の段階移行

ドメイン単位で v2 に移行。**1ドメインずつ完結させる**ことで、移行中にビルドが壊れない状態を維持する。

#### 推奨順序（移行コストの小さい順）

1. **account ドメイン** - 状態別型を使っていない、コマンド種別が少ない
2. **memorial_setting ドメイン** - 単一型集約、コマンド種別が中程度
3. **visit ドメイン** - 状態別型 (5種類) でコマンド種別も多い、最大の移行対象

#### ドメイン移行手順

各ドメインで以下を実施：

1. `xxx_commands.go` を新規追加（Command 型を定義）
2. 状態別型に `ApplyCommand(Command) (Event, error)` `ApplyEvent(Event) Aggregate` を実装
3. 既存の状態別メソッド (`Reschedule`, `Complete` 等) を **`ApplyCommand` 内の switch 分岐に移植して削除**
   - 直接呼び出しを廃止することで、ドメインロジック呼び出し経路を Command に一本化
4. `aggregate_factory.go` を `serializer.go` に置き換え（`Create` / `Apply` 削除、`Serialize` / `Deserialize` のみ残し、`createBlank` 関数は別途定義）
5. usecase 層: `repo.Save(ctx, agg, []Event{ev})` → `repo.Store(ctx, cmd, agg)` に置換
6. `Aggregate` のメソッド名変更（`GetId()` → `AggregateID()` 等）に追従
7. import パスを `github.com/Hiroshi0900/eventstore` から `github.com/Hiroshi0900/eventstore/v2` に変更
8. Postgres 実装も v2 用に追従（`Aggregate` メソッド名変更、`PersistEventAndSnapshot` シグネチャ変更に対応）

#### `visit_command_executor.go` の特殊対応

現状、複数イベント発行（`ScheduledEvent` + 業務イベント）をしている箇所を **2回の `Store` 呼び出し** に分割する：

```go
// Before
events := []Event{scheduledEvent, businessEvent}
repo.Save(ctx, newState, events)

// After
visit, err = repo.Store(ctx, scheduleCmd, blankVisit)  // ScheduledEvent
if err != nil { ... }
visit, err = repo.Store(ctx, businessCmd, visit)        // 業務イベント
```

トランザクション性は元々保証されていない（v1 の `Save` も内部で1イベントずつ `PersistEvent` をループしている）ため、挙動は変わらない。

### 6.3 v1 削除のタイミング

- 全利用ドメインの v2 移行完了後
- v1 を別ブランチにアーカイブ（`v1.x` ブランチ等）し、main からは削除
- セマンティックバージョニング上、メジャーバージョンの破棄として扱う

---

## 7. 利用例（v2）

### 集約の定義

```go
package visit

import esv2 "github.com/Hiroshi0900/eventstore/v2"

// ドメイン interface
type Visit interface {
    esv2.Aggregate
    ID() VisitID
    IsActive() bool
}

// 状態別型
type VisitScheduled struct {
    id      VisitID
    seqNr   uint64
    version uint64
    // ...
}

func (v VisitScheduled) AggregateID() esv2.AggregateID { return v.id }
func (v VisitScheduled) SeqNr() uint64                 { return v.seqNr }
func (v VisitScheduled) Version() uint64               { return v.version }
func (v VisitScheduled) IsActive() bool                { return true }

func (v VisitScheduled) ApplyCommand(cmd esv2.Command) (esv2.Event, error) {
    switch c := cmd.(type) {
    case RescheduleCommand:
        return NewVisitRescheduledEvent(v.id, v.seqNr+1, c.NewScheduledAt), nil
    case CompleteCommand:
        return NewVisitCompletedEvent(v.id, v.seqNr+1, c.Note), nil
    case CancelCommand:
        return NewVisitCanceledEvent(v.id, v.seqNr+1, c.Reason), nil
    default:
        return nil, esv2.ErrUnknownCommand
    }
}

func (v VisitScheduled) ApplyEvent(ev esv2.Event) esv2.Aggregate {
    switch e := ev.(type) {
    case VisitCompletedEvent:
        return VisitCompleted{ /* ... */ }  // 別の型を返す
    case VisitCanceledEvent:
        return VisitCanceled{ /* ... */ }
    case VisitRescheduledEvent:
        return VisitRescheduled{ /* ... */ }
    }
    return v
}
```

### Repository 構築

```go
createBlankVisit := func(id esv2.AggregateID) Visit {
    return VisitScheduled{ id: id.(VisitID) }
}

type visitSerializer struct{}
func (visitSerializer) Serialize(v Visit) ([]byte, error)   { /* ... */ }
func (visitSerializer) Deserialize(data []byte) (Visit, error) { /* ... */ }

repo := esv2.NewRepository[Visit](
    store,
    createBlankVisit,
    visitSerializer{},
    esv2.DefaultConfig(),
)
```

### usecase 層

```go
visit, err := repo.Load(ctx, visitID)
if err != nil { return err }

cmd := RescheduleCommand{NewScheduledAt: t}
visit, err = repo.Store(ctx, cmd, visit)
if err != nil { return err }
```

---

## 8. オープン課題

- **`SeqNr` 値オブジェクト化**: 別計画として進める。v2 リリース後に検討
- **空集約生成パターンの命名**: `createBlank` 関数のシグネチャは `func(AggregateID) T` で確定だが、利用側の命名規約 (`NewBlankXxx` 等) はドキュメント化したい
- **Postgres 実装の責任分界**: 利用側にあるため、v2 用の更新コストは利用側が負う
- **テスト戦略**: v2 のテストは現状リポと同等のカバレッジを目指す（in-memory ベースのレポジトリテスト + DynamoDB integration test）
