# eventstore

Go 向けの event sourcing ライブラリです。現在の公開モジュールは
`github.com/Hiroshi0900/eventstore/v2` のみです。

## Installation

```sh
go get github.com/Hiroshi0900/eventstore/v2
```

## Packages

| Package | Description |
|---|---|
| `github.com/Hiroshi0900/eventstore/v2` | Core interfaces and types |
| `github.com/Hiroshi0900/eventstore/v2/dynamodb` | DynamoDB-backed `EventStore` implementation |
| `github.com/Hiroshi0900/eventstore/v2/memory` | In-memory `EventStore` implementation |

## Development

実装本体は `v2/` 配下にあります。root には `go.work` を置いてあり、
リポジトリ root からでも `go test ./v2/...` のように実行できます。

## v2 Overview

- `Aggregate[E Event]`, `Event`, `Command` はドメイン情報だけを持つ
- 保存用メタデータは `EventEnvelope` / `SnapshotEnvelope` に集約
- `Repository.Save(aggID, cmd)` で load → apply → persist を一括実行
- serializer は `EventStore` 実装側で扱う
- `ApplyEvent(E) Aggregate[E]` により State Pattern に対応

詳細な使い方と API は `v2/` 配下のコードとテストを参照してください。

## License

MIT
