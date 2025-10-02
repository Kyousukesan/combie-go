## Combine 组件

一个基于 Go 反射与可插拔聚合函数的通用数据处理组件。

- **输入**: `[]struct` 或 `[]*struct`
- **处理**: 根据字段 `tag` 调用已注册的聚合处理函数
- **上下文**: 聚合函数可访问组件级上下文 `combineCtx`
- **并行**: 多个聚合函数可选择并发执行（`WithConcurrent`）
- **输出**: 写入结构体字段或调用输出方法（`fn:Method`）

---

### 安装

```bash
go get github.com/kyousukesan/combine-go
```

> Go 版本建议：1.22+

---

### 快速开始

```go
package main

import (
    "fmt"
    "strings"

    "github.com/zhoujiwei/combine/combine"
)

type Item struct {
    ID       int      `combine:"combineItem,Items"`
    Name     string   `combine:"uppercase,Name"`
    Score    int      `combine:"avg_score,fn:SetAvg"`
    Items    []string
    AvgScore float64
}

// 输出方法（用于 tag 中的 fn:SetAvg）
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

    // 聚合处理：转大写（返回以索引为 key 的结果）
    c.Register("uppercase", func(values []any, ctx map[string]any) map[any]any {
        out := make(map[any]any, len(values))
        for idx, v := range values {
            out[idx] = strings.ToUpper(fmt.Sprint(v))
        }
        return out
    })

    // 聚合处理：根据 ID 组装 Items（示例数据）
    c.Register("combineItem", func(values []any, ctx map[string]any) map[any]any {
        out := make(map[any]any, len(values))
        for idx, v := range values {
            if id, _ := v.(int); id == 1 {
                out[idx] = []string{"bbb", "aaa", "ccc"}
            } else {
                out[idx] = []string{"sss"}
            }
        }
        return out
    })

    // 聚合处理：计算平均分（示例返回固定值）
    c.Register("avg_score", func(values []any, ctx map[string]any) map[any]any {
        out := make(map[any]any, len(values))
        for idx := range values {
            if idx == 0 { out[idx] = float64(20) } else { out[idx] = float64(100) }
        }
        return out
    })

    items := []Item{{ID: 1, Name: "alice", Score: 90}, {ID: 2, Name: "bob", Score: 80}}
    // 直接传递 []Item，调用 fn:Method 时框架会对元素取址
    if err := c.Process(items); err != nil { panic(err) }

    fmt.Printf("%+v\n", items)
}
```

输出示例：

```text
[{ID:1 Name:ALICE Score:90 Items:[bbb aaa ccc] AvgScore:20} {ID:2 Name:BOB Score:80 Items:[sss] AvgScore:100}]
```

---

### Tag 规则

字段通过 `combine` tag 指定聚合函数与输出目标：

```text
combine:"函数名,输出目标"
```

- **函数名**: 已注册的聚合函数名
- **输出目标**:
  - 省略或为字段名：将结果写入该字段
  - `fn:Method`：调用结构体的方法作为输出；方法签名为 `func (t *T) Method(v any)` 或兼容形态

示例：

```go
type Item struct {
    ID       int     `combine:"id_handler,ID"`
    Name     string  `combine:"uppercase,Name"`
    Score    int     `combine:"avg_score,fn:SetAvg"`
    AvgScore float64
}
```

---

### API（当前版本）

```go
// 创建组件
func NewCombine(opts ...Option) *Combine

// 选项
func WithConcurrent() Option                 // 开启聚合函数并发
func WithCtx(ctx map[string]any) Option      // 设置组件上下文 combineCtx

// 注册聚合函数（仅支持聚合处理接口）
type AggregateHandler func(values []any, combineCtx map[string]any) map[any]any
func (c *Combine) Register(name string, fn AggregateHandler)
func (c *Combine) RegisterAggregate(name string, fn AggregateHandler) // Register 的别名

// 执行处理
func (c *Combine) Process(items interface{}) error
```

#### 处理函数签名

- 聚合处理：`func([]any, map[string]any) map[any]any`

> 聚合返回的 `map` 的 key 用于定位结果写回对象：
> - 推荐使用“索引”为 key（即 `0..n-1`），组件会按索引回填
> - 或使用原始字段值作为 key（组件也会尝试匹配）

---

### 并发

- 开启 `WithConcurrent()` 后，多个不同的聚合函数会并发执行
- 同一个聚合函数内部由用户代码自行控制并发与同步

---

### 错误与边界

- 未注册的函数名会报错：`handler <name> not registered`
- `fn:Method` 未找到或参数不兼容会报错
- 仅支持切片入参：`[]T` 或 `[]*T`，元素必须是 struct 或 *struct
- 字段写回类型不兼容会报错（内部尝试常见类型的可转换赋值）

---

### 运行示例与测试

```bash
# 构建与运行示例
go run ./cmd/example

# 运行测试
go test ./...
```

---

### 设计要点回顾

- 通过 tag 驱动处理逻辑，结合注册机制实现“业务无侵入”扩展
- 仅支持聚合处理：收集批量输入后统一执行
- 支持通过 `fn:Method` 实现复杂的对象输出逻辑
- 可选并发以提升多聚合场景性能


