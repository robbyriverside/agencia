# Parley Forms

Refreshed forms to align with label-driven updates.
Use `...` to denote a block body captured between the opening directive and `END`.

## Inputs

Retrieve input from the user.

- `INPUT`
- `INPUT <label>`

## Sending Messages

Send messages to agents.

### Simple Send
- `SEND <label>`  
  *Shorthand for `SEND <label> MESSAGE INPUT`*

### Single Message
- `SEND <label> MESSAGE <value>`  
  *Send a single message to an agent*
- `SEND <label> MESSAGE ... END`  
  *Send message in block*

### Multiple Messages
- `SEND <label> LIST <value>`  
  *Send multiple messages to an agent*
- `SEND <label> LIST ... END`  
  *Send multiple messages in the list block*

### Sending to Library Agents
- `SEND <label> IN <label>`  
  *Shorthand for `SEND <label> IN <label> MESSAGE INPUT`*
- `SEND <label> IN <label> MESSAGE <value>`  
  *Send a single message to an agent in a library*
- `SEND <label> IN <label> MESSAGE ... END`  
  *Send message in block*
- `SEND <label> IN <label> LIST <value>`  
  *Send multiple messages to an agent in a library*
- `SEND <label> IN <label> LIST ... END`  
  *Send multiple messages in the list block*

## Facts

Retrieve facts from the context.

- `FACT <label> IN <label>`
- `FACT <label>`

## Conditionals

Conditional logic within templates.

- `IF <predicate> THEN <value> ELSE <value>`  
  *Statement does not need `END`, like a block*
- `IF <predicate> THEN ... END`
- `ELSE IF <predicate> THEN ... END`
- `ELSE ... END`

## Lists

Handling list formatting and parsing.

### Input Formats
- `LIST <value>`  
  *Read bullets, write bullets*
- `LIST <value> FROM BULLETS`  
  *Explicit synonym for the default*
- `LIST <value> FROM LINES`  
  *Read lines (single newline), write bullets*
- `LIST <value> FROM PARAGRAPHS`  
  *Read paragraphs (blank line separated), write bullets*

### Output Formats
- `LIST <value> AS BULLETS`  
  *Write bullets (default)*
- `LIST <value> AS LINES`  
  *Read bullets, write single-line entries*
- `LIST <value> AS PARAGRAPHS`  
  *Read bullets, write paragraphs*

## Bindings

Variable assignment and usage.

- `LET <label> BE <value>`
- `LET <label> BE ... END`
- `USE <label>`
