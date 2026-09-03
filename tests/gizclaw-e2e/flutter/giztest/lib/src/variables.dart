/// Scenario variables: input resolution, `${name}` substitution and
/// single-write output capture, matching the Go runner's semantics.
library;

import 'dart:math';

import 'document.dart';

final _anchoredReference = RegExp(r'^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$');
final _embeddedReference = RegExp(r'\$\{([A-Za-z_][A-Za-z0-9_]*)\}');

/// Generates an input value. `uuid` produces a v4 UUID; `token` and `string`
/// both produce `g` followed by 32 lowercase hex characters, matching the Go
/// runner exactly.
String generateValue(String kind, {Random? random}) {
  final source = random ?? Random.secure();
  final bytes = List<int>.generate(16, (_) => source.nextInt(256));
  String hex(List<int> input) =>
      input.map((byte) => byte.toRadixString(16).padLeft(2, '0')).join();
  if (kind == 'uuid') {
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    final text = hex(bytes);
    return '${text.substring(0, 8)}-${text.substring(8, 12)}-'
        '${text.substring(12, 16)}-${text.substring(16, 20)}-'
        '${text.substring(20)}';
  }
  return 'g${hex(bytes)}';
}

class VariableEntry {
  const VariableEntry(this.data, this.spec);

  final Object? data;
  final VariableSpec spec;
}

class Variables {
  Variables(
    Map<String, VariableSpec> specs, {
    Map<String, String>? environment,
  }) {
    final env = environment ?? const {};
    for (final entry in specs.entries) {
      final name = entry.key;
      final spec = entry.value;
      if (spec.isOutput) {
        _entries[name] = VariableEntry(null, spec);
        continue;
      }
      Object? data;
      if (spec.env != null && spec.env!.isNotEmpty) {
        if (!env.containsKey(spec.env)) {
          throw StateError(
            'input variable $name requires environment ${spec.env}',
          );
        }
        data = env[spec.env];
      } else if (spec.generate != null && spec.generate!.isNotEmpty) {
        data = generateValue(spec.generate!);
      } else {
        data = spec.value;
      }
      _checkValueType(name, spec, data);
      _entries[name] = VariableEntry(data, spec);
    }
  }

  final _entries = <String, VariableEntry>{};

  static void _checkValueType(String name, VariableSpec spec, Object? data) {
    if (data == null) {
      if (spec.isOutput) {
        return;
      }
      throw StateError('variable $name is empty');
    }
    switch (spec.type) {
      case 'string':
        if (data is! String) throw StateError('variable $name want string');
      case 'boolean':
        if (data is! bool) throw StateError('variable $name want boolean');
      case 'integer':
        if (data is! int) throw StateError('variable $name want integer');
      case 'number':
        if (data is! num) throw StateError('variable $name want number');
      case 'object':
        if (data is! Map) throw StateError('variable $name want object');
      default:
        throw StateError('variable $name has unsupported type ${spec.type}');
    }
  }

  VariableEntry? operator [](String name) => _entries[name];

  /// Lists the secret string values to mask in reported errors, longest first
  /// so a longer secret is masked before one of its prefixes.
  List<String> redactions([List<String> requested = const []]) {
    final explicit = requested.toSet();
    final found = <String>[];
    for (final entry in _entries.entries) {
      if (!entry.value.spec.secret && !explicit.contains(entry.key)) {
        continue;
      }
      final data = entry.value.data;
      if (data is String && data.isNotEmpty) {
        found.add(data);
      }
    }
    found.sort((left, right) => right.length.compareTo(left.length));
    return found;
  }

  void assign(String name, Object? data) {
    final current = _entries[name];
    if (current == null) {
      throw StateError('unknown variable $name');
    }
    if (!current.spec.isOutput) {
      throw StateError('variable $name is not output');
    }
    if (current.data != null) {
      throw StateError('variable $name already assigned');
    }
    _checkValueType(name, current.spec, data);
    _entries[name] = VariableEntry(data, current.spec);
  }

  /// Substitutes `${name}` references. A string that is exactly one reference
  /// keeps the variable's runtime type; embedded references require string
  /// values and produce a string.
  Object? resolve(Object? input) {
    if (input is String) {
      final anchored = _anchoredReference.firstMatch(input);
      if (anchored != null) {
        final name = anchored.group(1)!;
        final entry = _entries[name];
        if (entry == null || entry.data == null) {
          throw StateError('variable $name unavailable');
        }
        return entry.data;
      }
      var result = input;
      for (final match in _embeddedReference.allMatches(input)) {
        final name = match.group(1)!;
        final entry = _entries[name];
        if (entry == null || entry.data == null) {
          throw StateError('variable $name unavailable');
        }
        final data = entry.data;
        if (data is! String) {
          throw StateError('embedded variable $name must be string');
        }
        result = result.replaceAll('\${$name}', data);
      }
      return result;
    }
    if (input is List) {
      return input.map(resolve).toList();
    }
    if (input is Map) {
      return <String, Object?>{
        for (final entry in input.entries)
          entry.key.toString(): resolve(entry.value),
      };
    }
    return input;
  }

  String resolveString(Object? input, String label) {
    final value = resolve(input);
    if (value is! String) {
      throw StateError('$label must resolve to string');
    }
    return value;
  }
}
