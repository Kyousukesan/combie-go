## Combine コンポーネント

Go のリフレクションとプラグ可能な集約関数で構成された汎用データ処理コンポーネントです。

- **入力**: `[]struct` または `[]*struct`
- **処理**: フィールドの `tag` に基づき登録済みの集約関数を呼び出し
- **コンテキスト**: 集約関数はコンポーネントレベルのコンテキスト `combineCtx` にアクセス可能
- **並行**: 複数の集約関数を並行実行可能（`WithConcurrent`）
- **出力**: 構造体フィールドへ書き込み、または出力メソッド（`fn:Method`）を呼び出し

---

### インストール

```bash
go get github.com/kyousukesan/combie-go
```

> Go 版本建议：1.22+

---

### クイックスタート

```go
package main

import (
    "fmt"
    "strings"

    "github.com/kyousukesan/combie-go/combine"
)

type Item struct {
    ID       int      `combine:"combineItem,Items"`
    Name     string   `combine:"uppercase,Name"`
    Score    int      `combine:"avg_score,fn:SetAvg"`
    Items    []string
    AvgScore float64
}

// 出力メソッド（tag の fn:SetAvg 用）
func (i *Item) SetAvg(v any) {
    switch x := v.(type) {
    case float64:
        i.AvgScore = x
    case int:
        i.AvgScore = float64(x)
    }
}

func main() {
    c := combine.NewCombine(
        combine.WithConcurrent(),
        combine.WithCtx(map[string]any{"factor": 1.5, "env": "prod"}),
    )

// 集約処理：大文字化（インデックスをキーにした結果を返す）
    c.Register("uppercase", combine.HandleFunc(func(values []any, ctx map[string]any) map[any]any {
        out := make(map[any]any, len(values))
        for idx, v := range values {
            out[idx] = strings.ToUpper(fmt.Sprint(v))
        }
        return out
    }))

    // 集約処理：ID に基づいて Items を構築（サンプルデータ）
    c.Register("combineItem", combine.HandleFunc(func(values []any, ctx map[string]any) map[any]any {
        out := make(map[any]any, len(values))
        for idx, v := range values {
            if id, _ := v.(int); id == 1 {
                out[idx] = []string{"bbb", "aaa", "ccc"}
            } else {
                out[idx] = []string{"sss"}
            }
        }
        return out
    }))

    // 集約処理：平均スコア計算（サンプルとして固定値を返す）
    c.Register("avg_score", combine.HandleFunc(func(values []any, ctx map[string]any) map[any]any {
        out := make(map[any]any, len(values))
        for idx := range values {
            if idx == 0 { out[idx] = float64(20) } else { out[idx] = float64(100) }
        }
        return out
    }))

    items := []any{
        &Item{ID: 1, Name: "alice", Score: 90},
        &Item{ID: 2, Name: "bob", Score: 80},
    }
    // Process は []any を受け取り、各要素は struct もしくは *struct を想定します
    if err := c.Process(items); err != nil { panic(err) }

    fmt.Printf("%+v\n", items)
}
```

出力例：

```text
[{ID:1 Name:ALICE Score:90 Items:[bbb aaa ccc] AvgScore:20} {ID:2 Name:BOB Score:80 Items:[sss] AvgScore:100}]
```

---

### Tag ルール

フィールドは `combine` タグで集約関数と出力先を指定します：

```text
combine:"函数名,输出目标"
```

- **関数名**: 登録済みの集約関数名
- **出力先**:
  - 省略またはフィールド名：結果をそのフィールドに書き込み
  - `fn:Method`：構造体のメソッドを出力として呼び出し。メソッドのシグネチャは `func (t *T) Method(v any)` など互換形

例：

```go
type Item struct {
    ID       int     `combine:"id_handler,ID"`
    Name     string  `combine:"uppercase,Name"`
    Score    int     `combine:"avg_score,fn:SetAvg"`
    AvgScore float64
}
```

---

### API（現行バージョン）

```go
// 创建组件
func NewCombine(opts ...Option) *Combine

// オプション
func WithConcurrent() Option                 // 集約関数の並行実行を有効化
func WithCtx(ctx map[string]any) Option      // コンポーネントコンテキスト combineCtx を設定

// 集約関数の登録（集約ハンドラのみをサポート）
type AggregateHandler func(values []any, combineCtx map[string]any) map[any]any
func (c *Combine) Register(name string, fn AggregateHandler)
func (c *Combine) RegisterAggregate(name string, fn AggregateHandler) // Register 的别名

// 実行
func (c *Combine) Process(items interface{}) error
```

#### ハンドラのシグネチャ

- 集約処理：`func([]any, map[string]any) map[any]any`

> 集約結果の `map` のキーは、結果を書き戻す対象を特定するために使われます：
> - インデックス（`0..n-1`）をキーにすることを推奨。インデックス順に書き戻します
> - もしくは元のフィールド値をキーにすることも可能（可能な限りマッチを試みます）

---

### 並行

- `WithConcurrent()` を有効にすると、異なる複数の集約関数が並行実行されます
- 同一集約関数内の並行処理と同期は、ユーザーコード側で制御してください

---

### エラーと境界

- 未登録の関数名はエラー：`handler <name> not registered`
- `fn:Method` が見つからない、または引数非互換の場合はエラー
- 受け入れはスライスのみ：`[]T` または `[]*T`。要素は struct もしくは *struct
- フィールドへの書き戻しで型不一致の場合はエラー（一般的な変換可能な代入は試みます）

---

### サンプルとテストの実行

```bash
# 构建与运行示例
go run ./cmd/example

# 运行测试
go test ./...
```

---

### 設計の要点

- Tag 駆動で処理ロジックを構成し、登録機構により“ビジネス非侵襲”な拡張を実現
- 集約処理のみをサポート：バッチ入力を収集して一括実行
- `fn:Method` により複雑なオブジェクト出力ロジックを表現可能
- 並行実行オプションで多集約シナリオの性能を向上


