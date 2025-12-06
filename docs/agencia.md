# Agencia: A Prompt-Centric Platform for Agentic Programming

## 0. Background

These are not autonomous-agents. The term agentic is widely used to reference autonomous agents that cooperate to perform some modular intelligence function. But I have no interest in AGI.

However, I agree with the notion of agentic modularity as a way of programming using LLMs. So this is about programming and I am a programmer doing a job. My objective is to use AI to provide new software functionality including technical tools like smart research assistant, but also a human friendly user interface. Instead of you figuring out how the software works, the software figures out what you need.

Concurrency is discussed early and often in agentic circles, but to the programmer it's just an optimization. We have plans for using concurrency, it's not the highest priority. We prefer to support a robust programming system for building real applications at the speed of a prototype.

## 1. Introduction

Agencia is a prompt-programming system. Prompts are AI context which are composed to ask an LLM the right question. Each agent is a function with inputs and outputs. The language is optimized to describe prompt composition. We use **Parley**, a simple template language, to build these prompts.

In Agencia, there is only one functional element: the agent. You can call other agents directly using `SEND` inside the template. This declarative style allows building agentic systems where agents behave as functions, maintaining clear input/output boundaries.

## 2. Agents

Here is a simple agent defined in YAML:

```yaml
agents:
  greet:
    description: Greet the user
    template: |
      Hello, {{ INPUT }}!
```

This agent simply repeats the input with a greeting. Parley instructions are written inside `{{ ... }}`.

### Inputs

The simplest thing an agent can do is repeat what the user said. We use `INPUT` for this.

```yaml
agents:
  echo:
    description: "Repeat what the user says"
    template: |
      You said: {{ INPUT }}
```

You can also define structured inputs to extract specific details:

```yaml
agents:
  greeter:
    description: "Greet a person by name"
    inputs:
      name:
        description: "The name of the user"
    template: |
      Hello, {{ INPUT name }}!
```

If the user says "My name is Sarah", the AI extracts "Sarah" and the agent replies "Hello, Sarah!".

## 3. Libraries and Listeners

Function agents (which call code) must be declared in code or imported from libraries. Libraries are collections of agents referenced using standard dot syntax: `library.agent`.

For example, to get the current time using the built-in `time` library:

```yaml
agents:
  clock:
    description: "Tell the time"
    template: |
      The current time is {{ SEND now IN time }}.
```

The `SEND` keyword is used to call another agent. You can pass inputs to the called agent as well:

```yaml
    template: |
       I will now search for cats: {{ SEND search IN web MESSAGE "cats" }}
```

## 4. Facts and Memory

Agencia Chat maintains ephemeral state including Facts. Facts are structured knowledge extracted from an agent's response.

To allow an agent to save information, define it in a `facts` section:

```yaml
agents:
  profiler:
    facts:
      username:
        description: "The user's name"
    template: |
      {{ INPUT username }}
```

Once a fact is saved, any agent can access it using `FACT`:

```yaml
agents:
  greeter:
    template: |
      Hello, {{ FACT username IN profiler }}.
```

This allows agents to share context and memory across a conversation.

## 5. Roles and Character Modeling

In Agencia, roles define the character the user is interacting with—distinct from the agents that execute the tasks. A role is a reusable persona that can be assigned to any number of agents, allowing for shared behavior, memory, and expressive consistency.

### What is a Role?

A Role in Agencia is a structured definition of an agent’s character. It includes:
*   **Name**: A human-readable identifier (e.g., “Lucinda”, “Dr. Patel”).
*   **Whoami**: A description of skills, history, and important facts.
*   **Personality**: A prompt template capturing voice, tone, and backstory.
*   **Performance**: A prompt template describing working style (concise, chatty, etc.).
*   **Facts**: Memory bank collected from responses.
*   **Inputs**: Memory bank collected from structured inputs.

### Roles vs Agents

Agents are technical execution units (tasks). A role is the identity that wraps those tasks. Multiple agents can share a single role, so the user perceives a continuous conversation with one character even as the backend logic switches agents.

### Example

Imagine a role named "Nurse Lucinda". Agents like `schedule_nurse`, `confirm_appointment`, and `triage_symptoms` can all adopt this role. The user talks to Lucinda, while different agents handle the logic.

## 6. Jobs

An agent can also declare a **job**, which is a list of agents to call in order. This effectively creates a workflow.

```yaml
agents:
  checkout_process:
    description: "Full checkout process"
    job:
      - check_availability
      - process_payment
      - confirm_order
    template: |
      Starting checkout...
```

Jobs can be asynchronous and notify the user upon completion.

## 7. Using Agencia

Agencia is open-source.
Code: https://github.com/robbyriverside/agencia
Service: https://fibberist.com/agencia

## 8. License

MIT

## 9. Contact

Rob Farrow
robbyriverside@gmail.com
