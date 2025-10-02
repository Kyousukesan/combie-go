package main

import (
	"fmt"
	"strings"

	combine "github.com/kyousukesan/combie-go"
)

type Item struct {
	ID       int    `combine:"combineItem,Items"`
	Name     string `combine:"uppercase,Name"`
	Score    int    `combine:"avg_score,fn:SetAvg"`
	Items    []string
	AvgScore float64
}

// Output method compatible with fn:SetAvg
func (i *Item) SetAvg(v any) {
	switch x := v.(type) {
	case float64:
		i.AvgScore = x
	case int:
		i.AvgScore = float64(x)
	case int64:
		i.AvgScore = float64(x)
	case string:
		// best-effort parse
		i.AvgScore = 0
		_ = x
	default:
		i.AvgScore = 0
	}
}

func main() {
	// Build ctx via CtxBuilder
	b := combine.NewCtxBuilder().
		Set("factor", 1.5).
		Set("env", "prod").
		Set("region", "ap-northeast-1").
		Set("trace", true)

	c := combine.NewCombine(
		combine.WithConcurrent(),
		combine.WithCtxBuilder(b),
	)

	// Aggregate handler: uppercase -> returns map keyed by index
	c.Register("uppercase", combine.HandleFunc(func(values []any, ctx map[string]any) map[any]any {
		out := make(map[any]any, len(values))
		for idx, v := range values {
			out[idx] = strings.ToUpper(fmt.Sprint(v))
		}
		return out
	}))

	// Aggregate handler: combineItem -> returns map keyed by index
	c.Register("combineItem", combine.HandleFunc(func(values []any, ctx map[string]any) map[any]any {
		out := make(map[any]any, len(values))
		// pretend DB query by ID in values; here fabricate
		for idx, v := range values {
			id := 0
			switch t := v.(type) {
			case int:
				id = t
			default:
				_ = t
			}
			if id == 1 {
				out[idx] = []string{"bbb", "aaa", "ccc"}
			} else {
				out[idx] = []string{"sss"}
			}
		}
		return out
	}))

	// Aggregate handler: avg_score -> register a standalone function via HandleFunc adapter
	c.Register("avg_score", combine.HandleFunc(AvgScoreAgg))

	items := []any{
		&Item{ID: 1, Name: "alice", Score: 90},
		&Item{ID: 2, Name: "bob", Score: 80},
	}

	if err := c.Process(items); err != nil {
		panic(err)
	}

	fmt.Printf("%+v\n", items)
}

// AvgScoreAgg is a reusable aggregate function to be registered via HandleFunc
func AvgScoreAgg(values []any, ctx map[string]any) map[any]any {
	out := make(map[any]any, len(values))
	for idx := range values {
		if idx == 0 {
			out[idx] = float64(20)
		} else {
			out[idx] = float64(100)
		}
	}
	return out
}
