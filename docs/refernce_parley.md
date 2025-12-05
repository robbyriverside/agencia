# Parley Lanmguage Reference

Prompts and templates in Agencia are just text.  What you write is what it is.
Sometimes the text you want to write depends on facts or inputs.  
Like listing all the things the user requested.  We call this dynamic content.

Dynamic content is provided by the template language Parley.  Parley allows 
inserting text from facts, or inputs, or even calling another agent.  Parley is
almost English.  It is a very specific subset of English, with sentences (aka statements)
that gather and render the values.  

Most of the template or prompt text is static, but parley exists between double curlys.

EG. {{ List Input value AS BULLETS }}

The descriptions in Parley are declarations.  When you use List in Parley, you are both
declaring that this result will be a list and also asking for a list to be extracted from
the value.  Extracting can mean to make a list of every line or a list from every paragraph and
from every bullet.  The bullet is the native form of a list.  If you ask the extract bullets
from a value, it will only extract the bullets, any other text will be ignored.

Parley is a very simple language with only 9 different statements each of which may have several
different ways to use it.  This reference contains the details of all the statements in the
language.

## SEND — call another agent

Use SEND when one agent needs output from another.

```
{{ SEND followup }}                         // shorthand for MESSAGE INPUT
{{ SEND summarize MESSAGE INPUT }}          // statement form with a value
{{ SEND summarize MESSAGE }}...{{ END }}    // block form; body becomes the message
{{ SEND summarize LIST tasks }}             // deliver a list value
{{ SEND summarize LIST }}...{{ END }}       // block list
{{ SEND summarize IN library }}             // call summarize inside the library
{{ SEND summarize IN library MESSAGE ... }} // mix libraries with statement or block bodies
```

Key ideas:

- **Shorthand** — `{{ SEND agent }}` and `{{ SEND agent IN library }}` default to `MESSAGE INPUT`, so whatever the user provided flows straight to the target agent.
- **Message vs. list** — `MESSAGE` sends a single value; `LIST` expects multiple items and calls `.CallOnList` so downstream agents can iterate cleanly.
- **Statement vs. block** — add a value directly in the statement (`MESSAGE INPUT`, `LIST USE tasks`) or open a block to write multi-line payloads. Close every block with `{{ END }}`.
- **Libraries** — prefix an agent with a library using `IN`: `{{ SEND summarize IN profile MESSAGE INPUT }}` resolves to `profile.summarize`. Use libraries to share helpers across domains.

Think of SEND as a function call: compose richer prompts by delegating sub-tasks to other agents and reuse their responses.

---

## Data Access — INPUT, FACT, LET, USE

These forms move data between Parley statements.

### INPUT

```
{{ INPUT }}          // entire user message  
{{ INPUT topic }}    // named destructured field
```

Use INPUT whenever you need what the user just said. Named variants let you bind structured inputs gathered upstream.

### FACT

```
{{ FACT owner }}                 // latest stored fact named "owner"
{{ FACT owner IN profile }}      // fact from another agent/library
{{ FACT escalation_note FROM ops }} // FROM still works for legacy templates
```

Agents persist facts after every run. Call them back with FACT to reference history, shared state, or other agents’ outputs.

### LET and USE

```
{{ LET summary BE SEND summarize MESSAGE INPUT }}
Summary: {{ USE summary }}

{{ LET bullets BE }}             // block form
- Item A
- Item B
{{ END }}
{{ SEND notify LIST USE bullets }}
```

- LET captures any Parley value once, even if producing it is expensive or order-dependent.
- USE replays the stored value exactly where you need it.
- Pair LET/USE to avoid duplicate SEND calls or to reformat data across multiple sections.

Together, INPUT, FACT, LET, and USE let you weave user data, stored knowledge, and intermediate computations into a single response.

---

## IF — branch on predicates

```
{{ IF status IN ticket IS Closed THEN FACT resolution_summary IN ticket ELSE FACT escalation_note IN ticket }}

{{ IF priority IN ticket IS High THEN }}
Escalate immediately.
{{ ELSE IF priority IN ticket IS Medium THEN }}
Monitor closely.
{{ ELSE }}
Note for tomorrow.
{{ END }}
```

- **Inline form** — `IF … THEN value ELSE value` fits when both branches are short. No `END` needed.
- **Block form** — `{{ IF … THEN }}…{{ ELSE }}…{{ END }}` shines for multi-line responses.
- **ELSE IF** — chain multiple predicates to keep logic linear and readable.
- **Predicates** — compare values with `IS`, `IS NOT`, check emptiness, or ask whether a list `HAS` something. Every predicate leverages the same `translatePredicate` rules documented in Parley.

Use IF to keep instructions accurate without rewriting entire blocks for each scenario.

---

## LIST — normalize list-shaped text

```
{{ LIST INPUT }}                       // read bullets, write bullets (default)
{{ LIST INPUT FROM LINES }}            // split by newline, then render bullets
{{ LIST notes FROM PARAGRAPHS AS LINES }}
{{ LIST tasks AS PARAGRAPHS }}         // write paragraphs even if source was bullets
```

Design goals:

- **Extraction** — `FROM BULLETS | LINES | PARAGRAPHS` describes how to read the input.
- **Presentation** — `AS BULLETS | LINES | PARAGRAPHS` controls how Parley writes the output.
- **Native form** — bullets are the canonical list format. When you ask for bullets, Parley ignores non-list text so your final output stays clean.

Use LIST whenever you need tidy enumerations, summaries of multiple items, or reusable bulleted sections shared across agents.

---

## Style Guide

- **Keep templates small.** Favor composing multiple SEND calls over one giant prompt; smaller agents are easier to test and reuse.
- **Name agents after their job.** `summarize_profile` reads better than `agent_3` and helps future authors understand dependencies.
- **Capture once, reuse often.** LET/USE prevents duplicate logic and keeps signal flow obvious.
- **Prefer lists to prose when summarizing many items.** LIST makes the structure explicit and spares downstream models from guesswork.
- **Whitespace is a feature.** Leave blank lines between major sections so humans (and future you) can scan the template quickly.

With these forms and habits, Parley stays readable while still giving you the power of structured templates.
