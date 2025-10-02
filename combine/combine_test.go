package combine

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type testItem struct {
	ID       int    `combine:"combineItem,Items"`
	Name     string `combine:"uppercase,Name"`
	Score    int    `combine:"avg_score,fn:SetAvg"`
	Items    []string
	AvgScore float64
}

func (t *testItem) SetAvg(v any) {
	switch x := v.(type) {
	case float64:
		t.AvgScore = x
	case int:
		t.AvgScore = float64(x)
	default:
		t.AvgScore = 0
	}
}

func TestSingleAndAggregateHandlers(t *testing.T) {
	c := NewCombine(WithCtx(map[string]any{"env": "test"}))

	c.Register("uppercase", func(values []any, ctx map[string]any) map[any]any {
		out := make(map[any]any, len(values))
		for idx, v := range values {
			out[idx] = strings.ToUpper(fmt.Sprint(v))
		}
		return out
	})

	c.Register("combineItem", func(values []any, ctx map[string]any) map[any]any {
		out := make(map[any]any, len(values))
		for idx, v := range values {
			id, _ := v.(int)
			if id == 1 {
				out[idx] = []string{"bbb", "aaa", "ccc"}
			} else {
				out[idx] = []string{"sss"}
			}
		}
		return out
	})

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

	items := []*testItem{
		{ID: 1, Name: "alice", Score: 90},
		{ID: 2, Name: "bob", Score: 80},
	}

	if err := c.Process(items); err != nil {
		t.Fatalf("process error: %v", err)
	}

	if items[0].Name != "ALICE" || items[1].Name != "BOB" {
		t.Fatalf("uppercase failed: %+v", items)
	}

	if got := items[0].AvgScore; got != 20 {
		t.Fatalf("avg score[0]=%v want 20", got)
	}
	if got := items[1].AvgScore; got != 100 {
		t.Fatalf("avg score[1]=%v want 100", got)
	}

	want0 := []string{"aaa", "bbb", "ccc"}
	got0 := append([]string(nil), items[0].Items...)
	sort.Strings(got0)
	if !reflect.DeepEqual(got0, want0) {
		t.Fatalf("items[0].Items=%v want %v", items[0].Items, want0)
	}
	if !reflect.DeepEqual(items[1].Items, []string{"sss"}) {
		t.Fatalf("items[1].Items=%v want [sss]", items[1].Items)
	}
}

func TestFnOutputMethodMissing(t *testing.T) {
	type bad struct {
		Score int `combine:"avg_score,fn:Nope"`
	}

	c := NewCombine()
	c.Register("avg_score", func(values []any, ctx map[string]any) map[any]any {
		return map[any]any{0: 1}
	})

	items := []*bad{{Score: 1}}
	if err := c.Process(items); err == nil {
		t.Fatalf("expected error when calling missing method")
	}
}

func TestConcurrentAggregates(t *testing.T) {
	c := NewCombine(WithConcurrent())

	// two aggregates acting on different fields/tags
	type obj struct {
		A int `combine:"agg1,A"`
		B int `combine:"agg2,B"`
	}

	c.Register("agg1", func(values []any, ctx map[string]any) map[any]any {
		out := make(map[any]any, len(values))
		for i := range values {
			out[i] = 1
		}
		return out
	})
	c.Register("agg2", func(values []any, ctx map[string]any) map[any]any {
		out := make(map[any]any, len(values))
		for i := range values {
			out[i] = 2
		}
		return out
	})

	items := []*obj{{}, {}, {}}
	if err := c.Process(items); err != nil {
		t.Fatalf("process error: %v", err)
	}
	for i, it := range items {
		if it.A != 1 || it.B != 2 {
			t.Fatalf("concurrency write wrong at %d: %+v", i, it)
		}
	}
}

// reusable aggregate function for testing named registration reuse
func AggFunc1(values []any, ctx map[string]any) map[any]any {
	out := make(map[any]any, len(values))
	for i := range values {
		out[i] = 7
	}
	return out
}

func TestRegisterWithNamedFunc(t *testing.T) {
	c := NewCombine()

	type obj struct {
		B int `combine:"agg2,B"`
	}

	// register using named function to allow reuse
	c.Register("agg2", AggFunc1)

	items := []*obj{{}, {}, {}}
	if err := c.Process(items); err != nil {
		t.Fatalf("process error: %v", err)
	}
	for i, it := range items {
		if it.B != 7 {
			t.Fatalf("named func write wrong at %d: %+v", i, it)
		}
	}
}
