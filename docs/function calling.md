# Agencia Function Handling

_A Technical White Paper_

---

## 1. Introduction

Agencia is a flexible agent framework that enables dynamic, self-composing agent workflows through:

- OpenAI ToolCalls (function calling)
- Template-driven agent behaviors
- Structured fallback and error handling
- Adaptive recursion and user engagement strategies

This document describes how Agencia handles function (tool) invocation, fallback logic, dynamic agent chaining, and template expansion at runtime.

---

## 2. Core Concepts

| Concept | Description |
|:---|:---|
| **Agent** | A unit of behavior that can be invoked with input and returns output (and optionally an error). |
| **Function (Tool) Call** | OpenAI suggests invoking a tool. Agencia matches this to an agent and executes it. |
| **Template Expansion** | Agent outputs can contain Go templates to dynamically call other agents or prompt users. |
| **Recursive Handling** | Function calls can recursively trigger further agent/tool calls. |
| **Trace Logging** | Each agent execution is recorded for debugging recursive flows. |

---

## 3. Agent Result Structure

Each agent call returns an `AgentResult` containing:

- `Output`: The string result.
- `Error`: Optional error, signaling hard failure.
- `Ran`: Whether the agent was actually executed.

Failures are classified as:

| Type | Meaning |
|:---|:---|
| **Hard failure** | `error != nil`, aborts tool handling immediately. |
| **Soft failure** | Special result like `%no-result%`, allows graceful fallback. |

---

## 4. Tool Call Handling

When OpenAI responds with one or more ToolCalls:

1. Agencia matches each ToolCall's name to a registered agent.
2. Executes the agent with the provided arguments.
3. Inspects the result:
   - If output contains `{{ ... }}`, expands it as a Go template.
   - Otherwise, returns output as-is.

Tool results are sent back to OpenAI.  
If OpenAI responds with more ToolCalls, the system continues recursively.

Recursion depth is limited to **5 levels** to prevent infinite loops.

---

## 5. Template Context Features

Agents can embed dynamic behavior in their outputs using template directives:

| Method | Purpose |
|:---|:---|
| `.Get "agent-name"` | Dynamically call another agent and insert its output. |
| `.AskUser "question"` | Insert a user-facing question and optionally pause execution. |

Templates allow agents to:

- Chain together dynamically
- Request missing information from the user
- Self-compose richer results

---

## 6. Dynamic Agent Discovery

```text
{{ .Get "new-agent" }}
```

The system will:

1. Detect the `{{ .Get }}` directive.
2. Look up `new-agent` in the agent registry.
3. Execute the `new-agent` immediately.
4. Insert its output into the conversation seamlessly.

This allows agents to “recruit” helpers dynamically without needing all agents declared up front, enabling adaptive conversations.



---

## 7. Error Handling and Flow Control

Failure Mode	System Behavior
System (hard) error	Stops tool handling immediately.
%no-result% marker	Signals no useful result, but conversation may continue gracefully.
Natural fallback suggestion	GPT may adapt based on output text guidance.

All recursive calls are logged into a debug trace.
Recursion is safely capped after 5 levels to prevent runaway cycles.



---

## 8. Example: Weather Lookup with Almanac Fallback

User asks:

“What should I wear in Paris today?”

Conversation flow:
	1.	GPT calls weather_lookup.
	2.	weather_lookup returns:

Weather unknown. {{ .Get "almanac_weather" }}


	3.	Agencia parses the template.
	4.	Calls almanac_weather, retrieves seasonal climate advice.
	5.	GPT uses both results to craft a coherent answer.

✅ Seamless, intelligent fallback behavior.
✅ No need to pre-wire Almanac tool initially.



---

## 9. Future Extensions

Potential future enhancements:
- **.Options** directive: Present multiple user choices programmatically.
- Structured soft errors: Use more semantic signals instead of %no-result%.
- Deferred Execution: Allow templates to suggest agents without immediate invocation.
- User Input Workflow: Turn .AskUser prompts into real queued interactions.



---

## 10. Conclusion

Agencia’s function handling system enables:
- Natural, self-composing agent chains
- Embedded conversation guidance through templates
- Robust error control and traceability
- Dynamic adaptation of conversation paths at runtime

Through minimal but powerful constructs like .Get and .AskUser,
Agencia transforms simple tool calling into true intelligent orchestration.


