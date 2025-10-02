package combine

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// Option configures Combine instance.
type Option func(*Combine)

// WithConcurrent enables concurrent execution of aggregate handlers.
func WithConcurrent() Option {
	return func(c *Combine) {
		c.concurrent = true
	}
}

// WithCtx sets the component-level context available to handlers.
func WithCtx(ctx map[string]any) Option {
	return func(c *Combine) {
		c.combineCtx = ctx
	}
}

// AggregateHandler consumes a slice of values and component ctx, and returns a map
// keyed by original item key to aggregated value.
// The key can be original struct pointer, or a chosen identity extracted via field value.
type AggregateHandler interface {
	Handle(values []any, combineCtx map[string]any) map[any]any
}

// HandleFunc is an adapter to allow the use of ordinary functions as AggregateHandler.
// Example: c.Register("foo", HandleFunc(func(values []any, ctx map[string]any) map[any]any { ... }))
type HandleFunc func(values []any, combineCtx map[string]any) map[any]any

// Handle calls f(values, combineCtx).
func (f HandleFunc) Handle(values []any, combineCtx map[string]any) map[any]any {
	return f(values, combineCtx)
}

// Combine is the component root.
type Combine struct {
	concurrent bool
	combineCtx map[string]any

	// registries
	aggregateHandlers map[string]AggregateHandler

	mu sync.RWMutex
}

// New creates a new Combine.
func New(opts ...Option) *Combine {
	c := &Combine{
		concurrent:        false,
		combineCtx:        make(map[string]any),
		aggregateHandlers: make(map[string]AggregateHandler),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewCombine is kept to match the design doc naming.
func NewCombine(opts ...Option) *Combine { return New(opts...) }

// Register registers an aggregate handler via the interface.
// Use HandleFunc to adapt plain functions.
func (c *Combine) Register(name string, fn AggregateHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if fn == nil {
		panic(fmt.Errorf("nil handler for %s", name))
	}
	c.aggregateHandlers[name] = fn
}

// RegisterAggregate explicitly registers an aggregate handler.
func (c *Combine) RegisterAggregate(name string, fn AggregateHandler) { c.Register(name, fn) }

// tagSpec parsed from struct tag: "funcName,outputTarget"
type tagSpec struct {
	funcName     string
	outputTarget string // field name or fn:Method
}

func parseTag(tag string) (tagSpec, bool) {
	if tag == "" {
		return tagSpec{}, false
	}
	parts := strings.Split(tag, ",")
	if len(parts) == 0 {
		return tagSpec{}, false
	}
	spec := tagSpec{funcName: strings.TrimSpace(parts[0])}
	if len(parts) > 1 {
		spec.outputTarget = strings.TrimSpace(parts[1])
	}
	if spec.funcName == "" {
		return tagSpec{}, false
	}
	return spec, true
}

// Process scans elements in items and applies handlers based on `combine` tag.
// items must be a slice of struct instances or pointers to structs, typed as []any.
func (c *Combine) Process(items []any) error {
	n := len(items)
	if n == 0 {
		return nil
	}

	// We will collect per-tag data for aggregate handlers.
	type fieldRef struct {
		itemValue reflect.Value // struct or pointer to struct
		fieldIdx  int
		fieldVal  any
		output    string // may be field name or fn:Method
	}

	// map funcName -> list of fieldRefs
	aggregates := map[string][]fieldRef{}

	// First pass: traverse all items and fields, collect aggregate inputs.
	for i := 0; i < n; i++ {
		elem := reflect.ValueOf(items[i])
		// support []*T and []T
		var structVal reflect.Value
		if elem.Kind() == reflect.Ptr {
			structVal = elem.Elem()
		} else {
			structVal = elem
		}
		if structVal.Kind() != reflect.Struct {
			return fmt.Errorf("slice element must be struct or *struct")
		}

		t := structVal.Type()
		for fi := 0; fi < t.NumField(); fi++ {
			field := t.Field(fi)
			tag := field.Tag.Get("combine")
			spec, ok := parseTag(tag)
			if !ok {
				continue
			}

			// read current value
			fv := structVal.Field(fi)
			var value any
			if fv.IsValid() {
				value = fv.Interface()
			}

			c.mu.RLock()
			_, isAgg := c.aggregateHandlers[spec.funcName]
			c.mu.RUnlock()

			if isAgg {
				aggregates[spec.funcName] = append(aggregates[spec.funcName], fieldRef{
					itemValue: elem,
					fieldIdx:  fi,
					fieldVal:  value,
					output:    spec.outputTarget,
				})
				continue
			}

			return fmt.Errorf("handler %s not registered", spec.funcName)
		}
	}

	// Aggregates: per funcName, gather values and invoke handler.
	type aggTask struct {
		name   string
		refs   []fieldRef
		values []any
	}

	tasks := make([]aggTask, 0, len(aggregates))
	for name, refs := range aggregates {
		vals := make([]any, 0, len(refs))
		for _, r := range refs {
			vals = append(vals, r.fieldVal)
		}
		tasks = append(tasks, aggTask{name: name, refs: refs, values: vals})
	}

	runTask := func(task aggTask) error {
		c.mu.RLock()
		handler := c.aggregateHandlers[task.name]
		ctx := c.combineCtx
		c.mu.RUnlock()
		if handler == nil {
			return fmt.Errorf("aggregate handler %s not found", task.name)
		}
		result := handler.Handle(task.values, ctx)

		// Write back results by matching order to refs. We assume handler keyed by original value or position.
		// Design doc leaves keying flexible; we will match by position index if numeric keys 0..n-1 are present,
		// otherwise try direct value match; else fall back to sequential mapping.
		for idx, ref := range task.refs {
			var out any
			// try index key
			if v, ok := result[idx]; ok {
				out = v
			} else if v, ok := result[ref.fieldVal]; ok {
				out = v
			} else {
				// sequential fallback
				// collect any remaining value from map (non-deterministic). To keep deterministic, use index fallback to nil.
				out = nil
			}

			if ref.output == "" || !strings.HasPrefix(ref.output, "fn:") {
				targetName := ""
				if ref.output == "" {
					// same field name
					targetName = refNameByIndex(ref.itemValue, ref.fieldIdx)
				} else {
					targetName = ref.output
				}
				if err := setField(derefIfPtr(ref.itemValue), targetName, out); err != nil {
					return err
				}
			} else {
				method := strings.TrimPrefix(ref.output, "fn:")
				if err := callOutputFunc(ref.itemValue, method, out); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if c.concurrent {
		var wg sync.WaitGroup
		var firstErr error
		var once sync.Once
		for _, task := range tasks {
			wg.Add(1)
			t := task
			go func() {
				defer wg.Done()
				if err := runTask(t); err != nil {
					once.Do(func() { firstErr = err })
				}
			}()
		}
		wg.Wait()
		return firstErr
	}

	for _, task := range tasks {
		if err := runTask(task); err != nil {
			return err
		}
	}
	return nil
}

func derefIfPtr(v reflect.Value) reflect.Value {
	if v.Kind() == reflect.Ptr {
		return v.Elem()
	}
	return v
}

func refNameByIndex(item reflect.Value, idx int) string {
	sv := derefIfPtr(item)
	return sv.Type().Field(idx).Name
}

func setField(structVal reflect.Value, fieldName string, value any) error {
	if structVal.Kind() != reflect.Struct {
		structVal = derefIfPtr(structVal)
		if structVal.Kind() != reflect.Struct {
			return fmt.Errorf("setField target must be struct")
		}
	}
	fv := structVal.FieldByName(fieldName)
	if !fv.IsValid() || !fv.CanSet() {
		return fmt.Errorf("cannot set field %s", fieldName)
	}
	val := reflect.ValueOf(value)
	if !val.IsValid() {
		// set zero value
		fv.Set(reflect.Zero(fv.Type()))
		return nil
	}
	// attempt conversion for common cases
	if val.Type().AssignableTo(fv.Type()) {
		fv.Set(val)
		return nil
	}
	if val.Type().ConvertibleTo(fv.Type()) {
		fv.Set(val.Convert(fv.Type()))
		return nil
	}
	return fmt.Errorf("cannot assign %s to field %s of type %s", val.Type(), fieldName, fv.Type())
}

func callOutputFunc(item reflect.Value, method string, arg any) error {
	// item must be a pointer to struct to call method with pointer receiver if needed
	var recv reflect.Value
	if item.Kind() == reflect.Ptr {
		recv = item
	} else {
		// take address if addressable
		if item.CanAddr() {
			recv = item.Addr()
		} else {
			return fmt.Errorf("output function requires addressable receiver")
		}
	}
	m := recv.MethodByName(method)
	if !m.IsValid() {
		// try function with signature func(*T, any)
		// not implemented; design doc suggests method on struct
		return fmt.Errorf("method %s not found", method)
	}
	mt := m.Type()
	if mt.NumIn() != 1 {
		return fmt.Errorf("method %s must accept 1 parameter", method)
	}
	in := reflect.ValueOf(arg)
	if !in.IsValid() {
		in = reflect.Zero(mt.In(0))
	} else if !in.Type().AssignableTo(mt.In(0)) {
		if in.Type().ConvertibleTo(mt.In(0)) {
			in = in.Convert(mt.In(0))
		} else {
			return fmt.Errorf("argument type %s not assignable to %s", in.Type(), mt.In(0))
		}
	}
	m.Call([]reflect.Value{in})
	return nil
}


