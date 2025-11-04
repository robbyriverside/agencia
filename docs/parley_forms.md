# Parley Form Legend

These notes mirror the shorthand used in `docs/parley_forms.txt` so we can iterate on the surface
syntax without diving back into full grammar work.  Every placeholder in angle brackets represents a
class of words or phrases that can appear in the directive.

## Placeholders

- `<label>`  
  A human-readable name: one or more words joined by spaces or hyphens. Labels identify agents,
  facts, list declarations, or user-provided bindings. Examples: `planner`, `planner tasks`,
  `summary`.

- `<value>`  
  Any evaluable source: an `INPUT`, a `CALL`, a `FACT`, a `LIST`, a literal block (`... END`), or a
  label captured via `USING`. Values feed into other directives (for example `CALL <label> WITH <value>` or `USING
  <label> FROM <value>`).

- `<predicate>`  
  A comparison expressed in Parley English. Predicates usually read like `language FROM concierge IS
  French` or `tasks FROM planner HAS items`. They may reference `INPUT`, `FACT`, `CALL`, or labels
  combined with comparison terms such as `IS`, `IS NOT`, `HAS`, or `IS EMPTY`.

## Related forms

- `USING <label> FROM ... END` produces a label that can be retrieved via `THE <label>` or `USE
  <label>`, giving another way to create reusable values inside the template itself.
- `CALL <label> ON LIST ... END` consumes a `<value>` or block and may produce derived facts,
  depending on how the agent is defined.

```parley
{{ USING summary FROM CALL summarize }}
Summary:
{{ THE summary }}
```

`USING` evaluates the directive immediately and stores the result in a label (`summary`) that belongs
to the current template scope.  Unlike `REFS`, no external handle is required—the template itself
creates, names, and retrieves the value.

`USING` covers all internal binding needs. Any data returned by agents should be stored into facts,
which keeps the language surface small and leverages Agencia’s existing persistence model.

Keep this legend close while adjusting the forms: if a new placeholder appears in
`docs/parley_forms.txt`, add the matching description here so designers and implementers stay in
sync.
