# Parley Implementation Notes (Dart)

This sketch shows how a Dart runtime (e.g., for Flutter or server-side Dart) could interpret Parley
directives.  The intent mirrors the Go outline, keeping the language declarative while the runtime
stays explicit.

## Core Types

```dart
abstract class ParleyValue {
  String asString();
  List<ParleyValue> asList();
  bool get isEmpty;
}

abstract class Resolver {
  Future<ParleyValue> input(String label);
  Future<ParleyValue> fact(String agent, String label);
  Future<ParleyValue> call(String agent, CallOptions options);
}

class CallOptions {
  final String? from;
  final ParleyValue? withValue;
  final List<ParleyValue>? onList;
  final Block? block;
  CallOptions({this.from, this.withValue, this.onList, this.block});
}
```

`ParleyValue` abstracts string/list conversion; it can wrap native types or JSON-friendly maps.

## Evaluator Skeleton

```dart
class Runtime {
  final Resolver resolver;
  final Map<String, ParleyValue> bindings = {};

  Runtime(this.resolver);

  Future<ParleyValue> evalCall(String agent, CallOptions options) async {
    if (options.block != null) {
      final value = await evalBlock(options.block!);
      options = CallOptions(
        from: options.from,
        withValue: options.withValue ?? value,
        onList: options.onList ?? value.asList(),
        block: null,
      );
    }
    return resolver.call(agent, options);
  }

  Future<ParleyValue> evalUsing(String label, Future<ParleyValue> Function() producer) async {
    final value = await producer();
    bindings[label] = value;
    return value;
  }

  Future<ParleyValue> evalIf(
      Future<bool> Function() predicate,
      Future<ParleyValue> Function() onTrue,
      Future<ParleyValue> Function() onFalse) async {
    return (await predicate()) ? await onTrue() : await onFalse();
  }
}
```

Blocks (`... END`) compile to `Block` objects that capture the child nodes; `evalBlock` walks those
nodes in order.

## Predicates and Helpers

Predicates return `Future<bool>` so they can depend on asynchronous lookups:

```dart
Future<bool> factEquals(Runtime runtime, String agent, String field, String expect) async {
  final value = await runtime.resolver.fact(agent, field);
  return value.asString().toLowerCase() == expect.toLowerCase();
}
```

Helpers like `LIST <value> OF BULLETS` become small formatter functions:

```dart
ParleyValue renderList(List<ParleyValue> items, {ListStyle style = ListStyle.defaulted}) {
  switch (style) {
    case ListStyle.bullets:
      return TextValue(items.map((v) => '- ${v.asString()}').join('\n'));
    case ListStyle.sentences:
      return TextValue(items.map((v) => '${v.asString()}.').join(' '));
    default:
      return TextValue('[${items.map((v) => v.asString()).join(' ')}]');
  }
}
```

## Binding Lookup

`THE summary` or `USE summary` first check `bindings`, then fall back to inputs/facts:

```dart
Future<ParleyValue?> lookup(Runtime runtime, String label) async {
  if (runtime.bindings.containsKey(label)) return runtime.bindings[label];
  try {
    return await runtime.resolver.input(label);
  } catch (_) {}
  // Agent can be derived by naming convention or explicit metadata.
  return await runtime.resolver.fact('', label);
}
```

## Integration Guidelines

- Keep directive parsers lightweight: tokenize by braces, then match against the forms listed in
  `parley_forms.txt`.
- Wrap `ParleyValue` around Dart primitives (`TextValue`, `ListValue`, etc.) so conversions are safe.
- Expose formatting helpers (`LIST`, `CALL ... ON LIST`, etc.) as extension methods on `Runtime`.

This structure slots into a Dart service or Flutter widget tree, keeping Parley’s declarative
surface while remaining idiomatic for async-first environments.
