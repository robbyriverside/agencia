# Agencia Documentation TODO

## Added Features (docs/agencia.md)
- Agents: prompt, template, and alias types using `.Get` and `.Input` to compose prompts declaratively.
- Structured inputs with `.Input "name"`/`.Inputs` for extracting arguments before template execution.
- Function agent libraries referenced with dot notation such as `time.current` for reusable function agents.
- Chat facts persisted per session, accessible through `.Fact`, and reused via the chat ID.
- Jobs that run asynchronous agent pipelines, expose `.JobID`, and support local fact scope within the run.
- Roles and character modeling defining personas, additive memories, and performance prompts shared across agents.

## Missing Features (docs/agencia.md)
- Listener agents still lack a walkthrough covering declaration, discovery, and invocation patterns.
- Observations capture/storage flow is only named; the documentation omits how they are summarized or recalled.
- Spec and error-reporting guidance (e.g., alerting users to spec validation issues) is absent.
- Chat storage mechanics and retrieval semantics go unstated beyond mentioning persistence by chat ID.
- Input look-ahead or clarifier behavior for anticipating required inputs is not described.
- OpenAI client lifecycle expectations (connection reuse, close/cleanup) remain undocumented.
