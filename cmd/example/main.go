package main

import (
	"fmt"
	"strings"

    "github.com/kyousukesan/combine-go/combine"
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
	c := combine.NewCombine(
		combine.WithConcurrent(),
		combine.WithCtx(map[string]any{"factor": 1.5, "env": "prod"}),
	)

	// Aggregate handler: uppercase -> returns map keyed by index
	c.Register("uppercase", func(values []any, ctx map[string]any) map[any]any {
		out := make(map[any]any, len(values))
		for idx, v := range values {
			out[idx] = strings.ToUpper(fmt.Sprint(v))
		}
		return out
	})

	// Aggregate handler: combineItem -> returns map keyed by index
	c.Register("combineItem", func(values []any, ctx map[string]any) map[any]any {
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
	})

	// Aggregate handler: avg_score -> returns map keyed by index
	c.Register("avg_score", func(values []any, ctx map[string]any) map[any]any {
		out := make(map[any]any, len(values))
		for idx := range values {
			if idx == 0 {
				out[idx] = float64(20)
			} else {
				out[idx] = float64(100)
			}
		}
		return out
	})

	items := []Item{
		{ID: 1, Name: "alice", Score: 90},
		{ID: 2, Name: "bob", Score: 80},
	}

	// must pass pointer slice to allow field setting by reflection for non-exported receivers
	// but here fields are exported, so []Item works; using []*Item ensures method calls on pointers
	ptrs := []*Item{&items[0], &items[1]}

	if err := c.Process(ptrs); err != nil {
		panic(err)
	}

	fmt.Printf("%+v\n", items)
}
