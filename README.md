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

> 推奨 Go バージョン: 1.22+

---

### クイックスタート

```go
package main

import (
    "fmt"
    "strings"

    combine "github.com/kyousukesan/combie-go"
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
    // CtxBuilder で複数の k-v を組み立てて一括適用
    b := combine.NewCtxBuilder().
        Set("factor", 1.5).
        Set("env", "prod").
        Set("region", "ap-northeast-1").
        Set("trace", true)

    c := combine.NewCombine(
        combine.WithConcurrent(),
        combine.WithCtxBuilder(b),
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

---

### API（現行バージョン）

```go
// 生成
func NewCombine(opts ...Option) *Combine

// オプション
func WithConcurrent() Option                 // 集約関数の並行実行を有効化
func WithCtx(ctx map[string]any) Option      // コンポーネントコンテキスト combineCtx を設定

// 集約関数の登録（集約ハンドラのみをサポート）
type AggregateHandler interface { Handle(values []any, combineCtx map[string]any) map[any]any }
type HandleFunc func(values []any, combineCtx map[string]any) map[any]any // adapter
func (c *Combine) Register(name string, fn AggregateHandler)
func (c *Combine) RegisterAggregate(name string, fn AggregateHandler)

// 実行
func (c *Combine) Process(items []any) error
```

---

### サンプルとテストの実行

```bash
go run ./example
go test ./...
```
