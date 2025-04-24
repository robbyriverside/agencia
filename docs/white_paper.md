# Agencia: A Prompt-Centric Platform for Agentic Programming

## 1. Introduction

Agencia is a new kind of agentic platform—one built on the idea that **prompts are programs**. While other agentic frameworks treat prompts as hidden parameters behind functions or classes, Agencia inverts the model: the prompt is the agent. Everything else is secondary.

Agencia provides a minimal runtime where agents are defined using **Go templates**, return **Markdown** outputs, and communicate exclusively through **plain text**. The result is a fully expressive system with almost no infrastructure—just a single YAML file and a few dozen lines of Go code.

We believe that a return to simplicity, transparency, and composability is what makes truly powerful systems. Agencia proves that agentic software doesn’t require orchestration engines, DSLs, or custom compilers—only composition and clarity.

## 2. The Problem We Were Trying to Solve

Most modern agentic platforms—like CrewAI, AutoGen, LangGraph—are built with good intentions: to coordinate multiple LLMs into structured workflows. But as they grow more powerful, they also grow more opaque.

A common pattern is for agents to be declared in code, like this:

```python
agent = Agent(role="Researcher", goal="Find relevant facts", backstory="...")
```

…but the actual prompt that defines the agent’s behavior is buried deep inside a function or string literal. You can’t *see* what the agent will do by looking at the file. The prompt—the true definition of the agent—is hidden from view.

This is the problem we set out to solve:

> What would an agentic system look like if **the prompt was the primary declaration**?

Could you define the behavior of the agent *only* with its text prompt? Could the prompt be written in a way that is both executable and understandable? Could the system disappear behind the prompt instead of standing in front of it?

## 3. How Agencia Works: Building a Prompt-Centric Agent

Let’s walk through how agents are defined in Agencia.

Each agent is defined as a YAML entry, where the name of the agent maps directly to a Go template string. This template is the actual **prompt** that gets sent to the AI engine. The only language is templates and Markdown:

```yaml
agents:
  - greet_user: 
      template: |
        Hello {{ .Input }}, welcome to the system.

  - repeat_input: 
    template: |
      You said: > {{ .Input }}

  - echo_summary: 
      template: |
        {{ .Get "greet_user" }}

        {{ .Get "repeat_input" }}
```

A formatter is a special kind of agent that does not generate a prompt. It simply applies the template and returns the result.  

### Key Concepts

- **Prompt is the source of truth**. There are no separate functions or class wrappers.
- **`.Input`** refers to the user-provided text.
- **`.Get "agent_name"`** executes another agent and inserts its output.
- The output of any agent is just a **Markdown string**—which can contain prose, formatting, or embedded YAML blocks.

This format is immediately readable, composable, and explainable. No runtime logic is hidden. Prompts *are* programs.

### External Function Support

Agencia supports simple external functionality by allowing agents to reference a `function` instead of a prompt. These are plain text transformers—usually operating on Markdown—that can be plugged in to extend the system.

Here's an example of an external agent:

```yaml
agents:
  - strip_yaml:
      function: markdown.StripYamlBlock
```

This agent doesn't use a prompt—it instead calls the `markdown.StripYamlBlock` function, which might remove the first fenced `yaml` code block from the input. These functions follow the same contract as prompt agents:

```go
func(ctx context.Context, input string) (string, error)
```

This design means external logic is injected *only* where needed, and it remains just another agent in the system. The result is still Markdown, and the calling logic remains prompt-centric.

External agents are first-class citizens: they can be defined, invoked, and composed just like prompt agents. You use them the same way, with `.Get` calls and plain text inputs.

But they're never required.

You can build and run a complete Agencia system without using any external code at all. Prompt agents alone are powerful enough to build end-to-end workflows.

Here’s what this means in practice:

- **External agents are optional** – You never have to write code to build powerful systems.
- **Prompt agents can stand alone** – You can define full behavior using only Go templates and AI.
- **External functions are extensions** – Use them only when Markdown processing or deterministic logic is needed.

This design keeps the complexity modular. You start with prompts—and only bring in code when it serves the prompt.

## 4. Why It's So Simple

Agencia uses **Go templates** as its execution language and **Markdown** as its communication format. This lets us avoid building any custom DSLs, compilers, or interpreters.

The runtime itself is tiny—less than 200 lines of Go code:

- Load agents from YAML  
- Render templates dynamically with `.Input` and `.Get`  
- Call OpenAI or use a local mock response YAML file

There are no types to define. No schemas to enforce. No agent classes to subclass. If you can write a prompt, you can write an agent.

Because the system is built on universal formats (text, Markdown, YAML), it is trivially portable:

- You could rewrite the runtime in Python, Rust, JavaScript, or Bash.
- You could edit your agents in VS Code, a web form, or a notebook.
- You could explain the entire system to another developer in 60 seconds.

That’s the power of designing systems that remove restrictions instead of adding features.

Agencia doesn’t orchestrate agents. It lets them speak.

