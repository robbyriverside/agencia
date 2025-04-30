# Function Calling Test Plan
TestCallOpenAI_ContinuationMissingTool
TestCallOpenAI_AgentTemplateFails

---

## ✅ Existing Test

| Test | Description |
|:---|:---|
| `TestCallOpenAI_FunctionCalling` | Ensures that the AI can correctly call a single tool (`greet`) in response to a simple prompt. |

---

## 🧠 Additional Tests to Add

| Priority | Test Name | Description |
|:---|:---|:---|
| 🟢 Must | `TestCallOpenAI_ToolNotFound` | Simulate AI calling a nonexistent function (e.g., `unicorn_magic`). Ensure the system errors cleanly without crashing. |
| 🟢 Must | `TestCallOpenAI_BadArguments` | Simulate AI sending malformed JSON arguments to a tool. Ensure the system catches and handles the error. |
| 🟡 Should | `TestCallOpenAI_MultipleToolCalls` | Simulate AI calling multiple tools in one assistant reply. Ensure all tool calls are processed correctly. |
| 🟡 Should | `TestCallOpenAI_EmptyToolOutput` | Ensure that if a tool returns an empty string output, the system continues cleanly. |
| 🟠 Nice | `TestCallOpenAI_RecursiveToolCalling` | (Optional) If tool output triggers another tool call, ensure either correct continuation or intentional limitation (single-hop). |

---

## ✨ Bonus Ideas

- **Timeouts**: Simulate an OpenAI timeout after a tool call. Ensure graceful failure.
- **Max Depth Protection**: Protect against infinite recursive tool-call loops.
- **Dry Run Mode**: Add a mode where function calling is simulated locally without hitting OpenAI.

---

# 🎯 Goal

These tests will ensure that Agencia's AI-driven function-calling system is **reliable**, **graceful under failure**, and **ready for production-level usage**.