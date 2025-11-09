# Parley - Agencia Templates Rewrite

Parley is the functional rewrite of Agencia templates.  Every directive is an English phrase wrapped
in `{{ ... }}` so it parses left-to-right without backtracking.  The grammar is minimal, the runtime
remains pure-Go, and every Parley construct evaluates to a new value instead of mutating state.

## Sentence Rules

* Keywords are case-insensitive; the guide prints them uppercase so data tokens stand out.
* Names never include spaces or quotes.  `planner` becomes the tokens `planner` and `tasks`.
* Directives describe intent, not control flow.  No explicit loops—only declarative listings and maps.
* Results are immutable.  Chaining functions returns new values the same way functional languages do.

## Core Forms

The shapes in `docs/parley_forms.txt` map directly onto helper calls in the Go, Dart, and Python
runtime sketches.  The snippets below assume the interfaces described in
`docs/parley_go.md`, `docs/parley_dart.md`, and `docs/parley_python.md`.

### Input

- **Parley** `{{ INPUT }}`
  - Go
    ```go
    value, err := rt.resolver.Input("")
    ```
  - Dart
    ```dart
    final value = await runtime.resolver.input("");
    ```
  - Python
    ```python
    value = runtime.resolver.input("")
    ```

- **Parley** `{{ INPUT topic }}`
  - Go
    ```go
    value, err := rt.resolver.Input("topic")
    ```
  - Dart
    ```dart
    final value = await runtime.resolver.input("topic");
    ```
  - Python
    ```python
    value = runtime.resolver.input("topic")
    ```

### Calls

- **Parley** `{{ CALL greet }}`
  - Go
    ```go
    result, err := rt.evalCall("greet", CallOptions{})
    ```
  - Dart
    ```dart
    final result = await runtime.evalCall("greet", CallOptions());
    ```
  - Python
    ```python
    result = runtime.eval_call("greet", CallOptions())
    ```

- **Parley** `{{ CALL greet FROM planner }}`
  - Go
    ```go
    result, err := rt.evalCall("greet", CallOptions{From: "planner"})
    ```
  - Dart
    ```dart
    final result = await runtime.evalCall(
      "greet",
      CallOptions(from: "planner"),
    );
    ```
  - Python
    ```python
    result = runtime.eval_call("greet", CallOptions(from_label="planner"))
    ```

- **Parley** `{{ CALL summarize WITH THE report }}`
  - Go
    ```go
    result, err := rt.evalCall("summarize", CallOptions{
      With: rt.lookup("report"),
    })
    ```
  - Dart
    ```dart
    final result = await runtime.evalCall(
      "summarize",
      CallOptions(withValue: runtime.lookup("report")),
    );
    ```
  - Python
    ```python
    result = runtime.eval_call(
      "summarize",
      CallOptions(with_value=runtime.lookup("report")),
    )
    ```

- **Parley** `{{ CALL draft WITH ... END }}`
  - Go
    ```go
    result, err := rt.evalCall("draft", CallOptions{
      Block: blockBuilder(nodes),
    })
    ```
  - Dart
    ```dart
    final result = await runtime.evalCall(
      "draft",
      CallOptions(block: blockBuilder(nodes)),
    );
    ```
  - Python
    ```python
    result = runtime.eval_call(
      "draft",
      CallOptions(block=lambda rt: eval_block(rt, nodes)),
    )
    ```

- **Parley** `{{ CALL notify ON LIST updates }}`
  - Go
    ```go
    result, err := rt.evalCall("notify", CallOptions{
      OnList: rt.lookup("updates").List(),
    })
    ```
  - Dart
    ```dart
    final result = await runtime.evalCall(
      "notify",
      CallOptions(onList: runtime.lookup("updates").asList()),
    );
    ```
  - Python
    ```python
    result = runtime.eval_call(
      "notify",
      CallOptions(on_list=list(runtime.lookup("updates").as_list())),
    )
    ```

- **Parley** `{{ CALL notify ON LIST ... END }}`
  - Go
    ```go
    result, err := rt.evalCall("notify", CallOptions{
      Block:  blockBuilder(nodes),
      OnList: rt.evalBlock(nodes).List(),
    })
    ```
  - Dart
    ```dart
    final result = await runtime.evalCall(
      "notify",
      CallOptions(
        block: blockBuilder(nodes),
        onList: (await runtime.evalBlock(blockBuilder(nodes))).asList(),
      ),
    );
    ```
  - Python
    ```python
    result = runtime.eval_call(
      "notify",
      CallOptions(
        block=lambda rt: eval_block(rt, nodes),
        on_list=list(eval_block(runtime, nodes).as_list()),
      ),
    )
    ```

### Facts

- **Parley** `{{ FACT name FROM concierge }}`
  - Go
    ```go
    value, err := rt.resolver.Fact("concierge", "name")
    ```
  - Dart
    ```dart
    final value = await runtime.resolver.fact("concierge", "name");
    ```
  - Python
    ```python
    value = runtime.resolver.fact("concierge", "name")
    ```

- **Parley** `{{ FACT agenda }}`
  - Go
    ```go
    value, ok := rt.lookupValue("agenda")
    ```
  - Dart
    ```dart
    final value = await runtime.lookup("agenda");
    ```
  - Python
    ```python
    value = runtime.lookup("agenda")
    ```

### Conditionals

- **Parley** `{{ IF language FROM concierge IS French THEN greeting FROM concierge ELSE INPUT END }}`
  - Go
    ```go
    result := rt.evalIf(
      eqFact("concierge", "language", "French"),
      func(rt *Runtime) Value { return mustFact(rt, "concierge", "greeting") },
      func(rt *Runtime) Value { return mustInput(rt, "") },
    )
    ```
  - Dart
    ```dart
    final result = await runtime.evalIf(
      () => factEquals(runtime, "concierge", "language", "French"),
      () async => await runtime.resolver.fact("concierge", "greeting"),
      () async => await runtime.resolver.input(""),
    );
    ```
  - Python
    ```python
    result = runtime.eval_if(
      fact_equals("concierge", "language", "French"),
      lambda rt: rt.resolver.fact("concierge", "greeting"),
      lambda rt: rt.resolver.input(""),
    )
    ```

- **Parley** `{{ IF tone FROM concierge IS Formal THEN ... END }}`
  - Go
    ```go
    result := rt.evalIf(
      eqFact("concierge", "tone", "Formal"),
      func(rt *Runtime) Value { return rt.evalBlock(formalNodes) },
      func(rt *Runtime) Value { return EmptyValue() },
    )
    ```
  - Dart
    ```dart
    final result = await runtime.evalIf(
      () => factEquals(runtime, "concierge", "tone", "Formal"),
      () => runtime.evalBlock(formalNodes),
      () => Future.value(emptyValue()),
    );
    ```
  - Python
    ```python
    result = runtime.eval_if(
      fact_equals("concierge", "tone", "Formal"),
      lambda rt: eval_block(rt, formal_nodes),
      lambda rt: empty_value(),
    )
    ```

- **Parley** `{{ ELSE ... END }}`
  - Go
    ```go
    elseBranch := Branch(func(rt *Runtime) Value {
      return rt.evalBlock(elseNodes)
    })
    ```
  - Dart
    ```dart
    final elseBranch = () => runtime.evalBlock(elseNodes);
    ```
  - Python
    ```python
    else_branch = lambda rt: eval_block(rt, else_nodes)
    ```

### Lists

- **Parley** `{{ LIST agenda }}`
  - Go
    ```go
    rendered := rt.evalList(rt.lookup("agenda"), ListDefault)
    ```
  - Dart
    ```dart
    final rendered = renderList((await runtime.lookup("agenda")).asList());
    ```
  - Python
    ```python
    rendered = render_list(runtime.lookup("agenda").as_list())
    ```

- **Parley** `{{ LIST agenda OF BULLETS }}`
  - Go
    ```go
    rendered := rt.evalList(rt.lookup("agenda"), ListBullets)
    ```
  - Dart
    ```dart
    final rendered = renderList(
      (await runtime.lookup("agenda")).asList(),
      style: ListStyle.bullets,
    );
    ```
  - Python
    ```python
    rendered = render_list(runtime.lookup("agenda").as_list(), style="BULLETS")
    ```

- **Parley** `{{ LIST agenda OF SENTENCES }}`
  - Go
    ```go
    rendered := rt.evalList(rt.lookup("agenda"), ListSentences)
    ```
  - Dart
    ```dart
    final rendered = renderList(
      (await runtime.lookup("agenda")).asList(),
      style: ListStyle.sentences,
    );
    ```
  - Python
    ```python
    rendered = render_list(runtime.lookup("agenda").as_list(), style="SENTENCES")
    ```

### Bindings

- **Parley** `{{ USING summary FROM CALL summarize }}`
  - Go
    ```go
    summary := rt.evalUsing("summary", func(rt *Runtime) Value {
      result, _ := rt.evalCall("summarize", CallOptions{})
      return result
    })
    ```
  - Dart
    ```dart
    final summary = await runtime.evalUsing(
      "summary",
      () => runtime.evalCall("summarize", CallOptions()),
    );
    ```
  - Python
    ```python
    summary = runtime.eval_using(
      "summary",
      lambda rt: rt.eval_call("summarize", CallOptions()),
    )
    ```

- **Parley** `{{ USING summary FROM ... END }}`
  - Go
    ```go
    summary := rt.evalUsing("summary", func(rt *Runtime) Value {
      return rt.evalBlock(nodes)
    })
    ```
  - Dart
    ```dart
    final summary = await runtime.evalUsing(
      "summary",
      () => runtime.evalBlock(nodes),
    );
    ```
  - Python
    ```python
    summary = runtime.eval_using("summary", lambda rt: eval_block(rt, nodes))
    ```

- **Parley** `{{ THE summary }}` / `{{ USE summary }}`
  - Go
    ```go
    value := rt.lookupBinding("summary")
    ```
  - Dart
    ```dart
    final value = runtime.lookup("summary");
    ```
  - Python
    ```python
    value = runtime.lookup("summary")
    ```

## Inputs

Use `INPUT` to read the current message or a specific structured input declared on the agent.

```parley
{{ INPUT }}
{{ INPUT topic }}
```

```gotemplate
{{ .Input }}
{{ .Input "topic" }}
```

Inputs can feed other expressions, act as arguments to `IF`, or become arguments to higher-order
functions.  Each directive resolves at render time without mutating chat state.

## Facts

Facts are remembered fields stored on the chat, exactly as described in `docs/agencia.md`.  The
lookup phrase is `FACT <field> FROM <agent>` and compiles to the existing chat fact helper.
When the directive already begins with another keyword (`IF`, `LIST`, `MAP`, etc.), you can omit the
word `FACT`—a bare name is treated as a fact reference.  Only standalone lookups (where the phrase
would otherwise start with a fact name) require the explicit `FACT` prefix.  To refresh or fetch
system data, call the appropriate agent first; once it returns, any declared facts (such as `legs
FROM itinerary`) are available to the template.

```parley
{{ FACT greeting FROM concierge }}
```

```gotemplate
{{ .Fact "concierge.greeting" }}
```

Facts carry their declared types, so the runtime can default them immutably.  When a fact might be
absent, pair it with an `IF` expression for predictable output.

## Conditionals as Expressions

Parley conditionals are expressions that return values.  The phrase is
`IF <predicate> THEN <value> ELSE <value>`, and both branches must evaluate.  Comparators such
as `IS`, `IS NOT`, `HAS`, and `EMPTY` map to equality, inequality, membership, and `len(...) == 0`.
When `THEN` (or `ELSE`) is left hanging—`THEN }}`—the block that follows becomes the value for that
branch, up to the matching `ELSE` or closing `END`.

Add intermediate checks with `ELSE IF <predicate> THEN <value>`.  Each `ELSE IF` runs only when all
previous predicates fail.  Inline expressions and block forms both support chained branches:

```parley
{{ IF status IN ticket IS Closed
     THEN FACT resolution_summary IN ticket
     ELSE IF priority IN ticket IS High
     THEN FACT escalation_note IN ticket
     ELSE FACT last_contact IN ticket
   END }}
```

```parley
{{ IF language FROM concierge IS French
     THEN salutation FROM concierge
     ELSE INPUT
   END }}
```

```gotemplate
{{ if eq (.Fact "concierge.language") "French" }}
  {{ .Fact "concierge.salutation" }}
{{ else }}
  {{ .Input }}
{{ end }}
```

Because the entire directive yields a value, you can choose between complete renderings:

```parley
{{ IF tone FROM concierge IS Formal
     THEN LIST concierge fact AS SENTENCES
     ELSE LIST concierge fact AS BULLETS
   END }}
```

The helper renders the selected branch while leaving the underlying facts unchanged.  Conditionals can
also render inline text blocks when you omit the immediate value:

```parley
{{ IF language FROM concierge IS French THEN }}
Bonjour!
{{ ELSE }}
Howdy!
{{ END }}
```

```gotemplate
{{ if eq (.Fact "concierge.language") "French" }}
Bonjour!
{{ else }}
Howdy!
{{ end }}
```

Insert `{{ ELSE IF <predicate> THEN }}` between the opening `IF` and the final `ELSE`/`END` when
you need more than two branches:

```parley
{{ IF status IN ticket IS Closed THEN }}
Archive the ticket.
{{ ELSE IF priority IN ticket IS High THEN }}
Escalate to the on-call engineer.
{{ ELSE }}
Continue monitoring until the next update.
{{ END }}
```

Or mix in previously bound values without a block.  Assuming `greeting` and `fallback` were captured
earlier with `USING`, you can write:

```parley
{{ IF language FROM concierge IS French
     THEN THE greeting
     ELSE THE fallback
   END }}
```

```gotemplate
{{ if eq (.Fact "concierge.language") "French" }}
  {{ $greeting }}
{{ else }}
  {{ $fallback }}
{{ end }}
```

## Declarative Listings

`LIST` renders the contents of a fact or input without exposing loops.  The first token after LIST
is the fact name (or `INPUT`).  Because the directive already begins with `LIST`, you can reference
the fact directly without repeating `FACT`.  Formatting is implied:

* default: whitespace-separated list in square brackets
* `AS BULLETS`: bullet list with `- `
* `AS SENTENCES`: list separated by periods

```parley
{{ LIST planner }}

{{ LIST planner AS BULLETS }}
```

```gotemplate
{{ $items := .Fact "planner" }}
[{{ range $index, $item := $items }}{{ if gt $index 0 }} {{ end }}{{ $item }}{{ end }}]

{{ range $item := .Fact "planner" }}
- {{ $item }}
{{ end }}
```

The helper returns a formatted string, leaving the original fact untouched.

## Higher-Order Composition

Parley treats collection helpers as pure functions.  `MAP`, `COLLECT`, and `FLATTEN` accept
directives and return new values, allowing pipeline-style composition.  The runtime can execute the
directives concurrently when the backing fact is marked safe for parallelism.

### MAP

`MAP` applies a directive to each element of a source list and concatenates the rendered results.
The directive runs with the element bound as the active fact.

```parley
{{ MAP planner
     LIST planner AS BULLETS
}}
```

```gotemplate
{{ range $group := .Fact "planner" }}
  {{ range $item := $group }}
  - {{ $item }}
  {{ end }}
{{ end }}
```

Each iteration is independent, so the runtime may schedule them concurrently while preserving order.

### COLLECT and FLATTEN

`COLLECT` gathers the outputs of a directive into a list of lists, mirroring functional `mapM`.  Add
`AND FLATTEN` to drop one layer of nesting, similar to Haskell’s `concat`.

```parley
{{ COLLECT planner
     LIST planner
}}

{{ COLLECT planner
     LIST planner
   AND FLATTEN
}}
```

```gotemplate
{{ $groups := collect (.Fact "planner") (tpl "LIST planner") }}
{{ $flat := flatten $groups }}

{{ range $group := $groups }}
[{{ range $index, $item := $group }}{{ if gt $index 0 }} {{ end }}{{ $item }}{{ end }}]
{{ end }}

{{ range $item := $flat }}
- {{ $item }}
{{ end }}
```

`COLLECT` never mutates the source fact; it returns a fresh slice each time.  The optional 
`AND FLATTEN` shortcut mirrors `flatten (collect ...)` to keep templates concise.

Parley keeps the surface language brief while preserving all Agencia behaviors: structured inputs,
persisted facts, pure conditionals, and higher-order collection handling.  These sentences provide a
functional, immutable layer that compiles directly into the Go templates already shipping in
Agencia.

## Examples

The curated snippets below highlight every Parley language feature with a side-by-side Go-template
translation.  Refer back to the sections above for the semantics behind each helper.

* `CALL name` replaces `.Get "name"`.
* `USING label FROM CALL name` captures the result so it can be reused via `THE label` or `USE label`.
* `USING label FROM` (block form) captures the raw text between `USING` and `END` into `THE label` (or `USE label`).
* `CALL name WITH value` passes an explicit argument.
* `LOWER value` and `TRUNCATE value TO n` mirror common Go-template pipelines.

### Plain Input

```gotemplate
Hello, {{ .Input }}!
```

```parley
Hello, {{ INPUT }}!
```

### Named Input

```gotemplate
Checking out book: {{ .Input "title" }}
```

```parley
Checking out book: {{ INPUT title }}
```

### Agent Invocation

```gotemplate
{{ .Get "hello_user" }}
```

```parley
{{ CALL hello_user }}
```

### Binding and Reuse

```gotemplate
{{ $poem := .Get "write_poem" }}

Poem:
{{ $poem }}

Theme:
{{ .Get "extract_theme" $poem }}
```

```parley
{{ USING poem FROM CALL write_poem }}

Poem:
{{ THE poem }}

Theme:
{{ CALL extract_theme WITH THE poem }}
```

Both `THE poem` and `USE poem` dereference the captured value, so you can choose whichever reads
better in context.  For example:

```gotemplate
{{ $summary := .Get "summarize_input" }}
Summary:
{{ $summary }}
```

```parley
{{ USING summary FROM CALL summarize_input }}
Summary:
{{ USE summary }}
```

### Binding Raw Text

```gotemplate
{{ $stmt0 := `This text will
be bound to the variable 'stmt0'`
}}
```

```parley
{{ USING stmt0 FROM }}
This text will
be bound to the variable 'stmt0'
{{ END }}

You can refer back to the captured text using either `THE stmt0` or `USE stmt0`, whichever reads
more naturally alongside the surrounding prose.
```

### Facts and Conditionals

```gotemplate
{{ if eq (.Fact "concierge.language") "French" }}
  {{ .Fact "concierge.salutation" }}
{{ else }}
  {{ .Input }}
{{ end }}
```

```parley
{{ IF language FROM concierge IS French
     THEN salutation FROM concierge
     ELSE INPUT
   END }}
```

### Listing Collections

```gotemplate
{{ range $item := .Fact "planner" }}
- {{ $item }}
{{ end }}
```

```parley
{{ LIST planner AS BULLETS }}
```

### Mapping Collections

```gotemplate
{{ range $group := .Fact "planner" }}
  {{ range $item := $group }}
  - {{ $item }}
  {{ end }}
{{ end }}
```

```parley
{{ MAP planner
     LIST planner AS BULLETS
}}
```

### Collect and Flatten

```gotemplate
{{ $groups := collect (.Fact "planner") (tpl "LIST planner") }}
{{ $flat := flatten $groups }}

{{ range $item := $flat }}
- {{ $item }}
{{ end }}
```

```parley
{{ COLLECT planner
     LIST planner
   AND FLATTEN
}}
```

### Pipeline Helpers

```gotemplate
(Summary: {{ truncate .Input 30 }})
```

```parley
(Summary: {{ TRUNCATE INPUT TO 30 }})
```

```gotemplate
The central theme of the poem is {{ .Input | lower }}.
```

```parley
The central theme of the poem is {{ LOWER INPUT }}.
```

Other transformations follow the same pattern: replace `.Input` with `INPUT`, substitute `.Get "agent"`
with `CALL agent`, and express control flow with the declarative Parley sentences above.
