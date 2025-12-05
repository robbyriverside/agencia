# Agencia Dart Conversion Plan

## Goal
Deliver a Dart/Flutter-friendly library (new repo: `dev/agencia_dart`) that can evaluate Parley
templates, call Agencia agents, and drop seamlessly into Flutter apps. The library should feel native
to Dart, expose async-friendly APIs, and ship with examples/tests that mirror the current Go
behavior.

## Assumptions
- `dev/agencia_dart` is a fresh repository dedicated to the Dart port.
- We reuse existing YAML specs and Parley templates produced in this repo.
- Flutter apps embed the library as a dependency (local path or pub package).
- Networking to Agencia/OpenAI happens through resolver hooks (no direct Go interop).

## Workstream 1 – Repository Bootstrap
1. Scaffold a Dart package (`agencia_parley`) inside `dev/agencia_dart` with `dart create -t package`.
2. Configure analysis (`analysis_options.yaml`), formatting, lint rules, and CI (GitHub Actions).
3. Add README covering installation, supported features, and roadmap.

## Workstream 2 – Parley Parser & Translator
1. Port the Parley tokenizer/translator from Go (`parley/translator.go`) into Dart:
   - Mirror directive parsing (`{{ ... }}`) and inline/block behaviors.
   - Implement ELSE IF support, LIST, LET, SEND, etc., guided by `docs/parley_forms.txt`.
2. Validate translation parity by snapshot-testing core examples (`docs/examples/*`).
3. Expose a `String translate(String parleySource)` API plus AST structs for future inspection.

## Workstream 3 – Runtime & Resolver Abstractions
1. Implement `ParleyValue`, `Runtime`, `Resolver`, and helpers described in `docs/parley_dart.md`.
2. Support asynchronous predicates, bindings, fact storage, list formatting, and observations.
3. Define a `SpecSession` object analogous to Go’s `Chat`:
   - Holds facts, observations, trace cards (JSON-friendly).
   - Provides `Future<String> run(String agent, String prompt)` for one-shot calls.
   - Offers `Stream<ChatEvent>` for interactive conversations.

## Workstream 4 – Flutter Integration Layer
1. Build a Flutter plugin (or plain package) that wraps `SpecSession` into a `ChangeNotifier` / Riverpod provider.
2. Create widgets:
   - `ParleyChatView` for interactive text I/O.
   - `ParleyResultCard` for rendering facts/observations.
3. Document configuration for mobile (env vars, secure storage for API keys).

## Workstream 5 – Tooling, Testing, and Examples
1. Add unit tests for translator edge cases, runtime behaviors, and resolver mocks (use `test` package).
2. Set up golden tests comparing Go vs Dart translations using shared fixtures exported from this repo.
3. Ship sample apps:
   - CLI demo (Dart console) that loads `examples/hello.yaml`.
   - Flutter demo that runs the helpline Parley example end-to-end.
4. Provide integration tests that stub Agencia responses to keep CI offline-friendly.

## Workstream 6 – Documentation & Distribution
1. Write API docs with `dart doc` and host via GitHub Pages.
2. Add migration guidance (`docs/parley_dart.md` → code) and troubleshooting to `README`.
3. Prepare for pub.dev release:
   - Complete `pubspec.yaml` metadata and license.
   - Add changelog and semantic versioning strategy.

## Next Steps
1. Confirm architectural decisions (e.g., whether translator compiles to Go templates or Dart runtime).
2. Stand up the new repo with Workstream 1 tasks.
3. Iterate on Workstreams 2–4 in parallel branches, merging once parity tests pass.
