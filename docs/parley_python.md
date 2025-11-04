# Parley Implementation Notes (Python)

Python excels at rapid DSL prototyping, so a Parley runtime can lean on dynamic typing and generators.
This outline shows how to wire the constructs described in `parley_forms.txt` into a Python service.

## Core Protocols

```python
from typing import Protocol, Iterable, Dict, Callable

class Value(Protocol):
    def as_string(self) -> str: ...
    def as_list(self) -> Iterable["Value"]: ...
    def is_empty(self) -> bool: ...

class Resolver(Protocol):
    def input(self, label: str) -> Value: ...
    def fact(self, agent: str, label: str) -> Value: ...
    def call(self, agent: str, options: "CallOptions") -> Value: ...

class CallOptions:
    def __init__(self, *, from_label=None, with_value=None, on_list=None, block=None):
        self.from_label = from_label
        self.with_value = with_value
        self.on_list = on_list or []
        self.block = block  # callable returning Value
```

`Value` can wrap native Python types; you can implement `StringValue`, `ListValue`, etc., or defer to
existing Agencia abstractions.

## Runtime Skeleton

```python
class Runtime:
    def __init__(self, resolver: Resolver):
        self.resolver = resolver
        self.bindings: Dict[str, Value] = {}

    def eval_call(self, agent: str, opts: CallOptions) -> Value:
        if opts.block:
            produced = opts.block(self)
            if opts.with_value is None:
                opts.with_value = produced
            if not opts.on_list:
                opts.on_list = list(produced.as_list())
            opts.block = None
        return self.resolver.call(agent, opts)

    def eval_using(self, label: str, producer: Callable[["Runtime"], Value]) -> Value:
        value = producer(self)
        self.bindings[label] = value
        return value

    def eval_if(self, predicate: Callable[["Runtime"], bool],
                on_true: Callable[["Runtime"], Value],
                on_false: Callable[["Runtime"], Value]) -> Value:
        return on_true(self) if predicate(self) else on_false(self)
```

Blocks (`... END`) compile to callables that receive the runtime.  Inline values (e.g., `CALL agent
WITH INPUT`) bypass the block conversion.

## Predicates

```python
def fact_equals(agent: str, label: str, expected: str) -> Callable[[Runtime], bool]:
    def _pred(rt: Runtime) -> bool:
        value = rt.resolver.fact(agent, label)
        return value.as_string().lower() == expected.lower()
    return _pred
```

Predicates reference helper verbs (`IS`, `HAS`, `EMPTY`) mapped to small functions.

## Formatting Helpers

```python
def render_list(values: Iterable[Value], style: str = "default") -> Value:
    if style == "BULLETS":
        text = "\n".join(f"- {v.as_string()}" for v in values)
    elif style == "SENTENCES":
        text = " ".join(f"{v.as_string()}." for v in values)
    else:
        text = "[" + " ".join(v.as_string() for v in values) + "]"
    return StringValue(text)
```

`LIST`, `CALL ... ON LIST`, and similar directives call into helpers like `render_list`.

## Lookup Rules

```python
def lookup(rt: Runtime, label: str) -> Value:
    if label in rt.bindings:
        return rt.bindings[label]
    try:
        return rt.resolver.input(label)
    except KeyError:
        agent, field = parse_fact_label(label)  # implementation detail
        return rt.resolver.fact(agent, field)
```

Bindings take precedence, then inputs, then facts. `parse_fact_label` can derive agent/field from
metadata stored during parsing.

## Glue Code

- Tokenize the template by splitting on `{{ ... }}` and map each directive to a handler function.
- Represent block bodies as nested arrays of nodes (strings or directives).
- Use Python generators or simple string builders to produce the final template output.

This approach keeps Parley lightweight while embracing Python’s expressiveness—perfect for iterating
on Agencia prototypes or supporting scripting environments.
