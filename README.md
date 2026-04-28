# eventstore

Go 向けの event sourcing ライブラリです。公開モジュールは
`github.com/Hiroshi0900/eventstore` です。

## Installation

```sh
go get github.com/Hiroshi0900/eventstore
```

## Packages

| Package | Description |
|---|---|
| `github.com/Hiroshi0900/eventstore` | Core interfaces and types |
| `github.com/Hiroshi0900/eventstore/dynamodb` | DynamoDB-backed `EventStore` implementation |
| `github.com/Hiroshi0900/eventstore/memory` | In-memory `EventStore` implementation |

## Development

実装本体は repo root にあります。`go test ./...` を root からそのまま実行できます。

## Overview

- `Aggregate[E Event]`, `Event`, `Command` はドメイン情報だけを持つ
- 保存用メタデータは `EventEnvelope` / `SnapshotEnvelope` に集約
- `Repository.Save(aggID, cmd)` で load → apply → persist を一括実行
- serializer は `EventStore` 実装側で扱う
- `ApplyEvent(E) Aggregate[E]` により State Pattern に対応

詳細な使い方と API は root 配下のコードとテストを参照してください。

## License

MIT
