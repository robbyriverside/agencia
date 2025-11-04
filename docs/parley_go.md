# Parley Implementation Notes (Go)

This document sketches a Go-friendly approach to evaluating Parley directives.  It does not attempt
to be a full interpreter—just the key types and control flow you would wire into Agencia’s runtime.

## Core Interfaces

```go
type Value interface {
	String() string
	List() []Value
	IsEmpty() bool
}

type Resolver interface {
	Input(label string) (Value, error)
	Fact(agent, label string) (Value, error)
	Call(agent string, opts CallOptions) (Value, error)
}

type CallOptions struct {
	From   string   // CALL <agent> FROM <label>
	With   Value    // CALL <agent> WITH <value>
	OnList []Value  // CALL <agent> ON LIST <value>
	Block  *Block   // CALL <agent> WITH ... END / ON LIST ... END
}
```

`Value` is intentionally minimal: Parley mostly pushes strings or lists through helpers, so the
interface focuses on those conversions.

## Evaluating Directives

Each directive expands into a small function.  The interpreter walks the rendered template,
dispatching on the directive keyword.

```go
func (rt *Runtime) evalCall(agent string, opts CallOptions) (Value, error) {
	if opts.Block != nil {
		opts.With = rt.evalBlock(opts.Block)
	}
	if len(opts.OnList) > 0 && opts.Block != nil {
		opts.OnList = rt.evalBlock(opts.Block).List()
	}
	return rt.resolver.Call(agent, opts)
}

func (rt *Runtime) evalIf(pred Predicate, t Branch, f Branch) Value {
	if pred(rt) {
		return t(rt)
	}
	return f(rt)
}

func (rt *Runtime) evalList(src Value, style ListStyle) Value {
	return formatList(src.List(), style)
}

func (rt *Runtime) evalUsing(label string, producer Branch) Value {
	val := producer(rt)
	rt.bindings[label] = val
	return val
}
```

## Predicate Helpers

```go
type Predicate func(*Runtime) bool

func eqFact(label string, want string) Predicate {
	return func(rt *Runtime) bool {
		v, _ := rt.lookupValue(label)
		return strings.EqualFold(v.String(), want)
	}
}
```

Predicates compose the same way Go templates do today: the runtime can expose helpers like `Is`,
`Has`, or `Empty`.

## Binding and References

`USING summary FROM CALL summarize` is handled by pushing the value into `rt.bindings`.  Later,
`THE summary` or `USE summary` resolves by checking the binding map before falling back to facts or
inputs.

```go
func (rt *Runtime) lookupValue(label string) (Value, bool) {
	if v, ok := rt.bindings[label]; ok {
		return v, true
	}
	if v, err := rt.resolver.Input(label); err == nil {
		return v, true
	}
	return rt.resolver.Fact("", label) // agent inferred upstream
}
```

## Integration

- The Go implementation reuses Agencia’s existing host objects (facts, inputs, agent registry).
- Directives expand to well-scoped helper calls; you can translate Parley source into Go code with a
  simple parser or by mapping forms directly to functions.
- Block forms (`... END`) compile to `Block` syntax trees that call back into `Runtime` when
  evaluated.

This outline balances declarative templates with Go’s explicit control: Parley continues to read
like English while the Go runtime stays close to Agencia’s existing template engine.
