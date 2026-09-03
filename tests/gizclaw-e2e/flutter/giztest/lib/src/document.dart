/// Giztest scenario documents, loaded and validated for the subset of the
/// `gizclaw.test/v1alpha1` contract this runner executes.
///
/// The Go runner owns the complete schema. This runner executes the `rpc`,
/// `client_rpc`, `http` and `output` step kinds and reports every other kind
/// as skipped instead of passing it silently.
library;

import 'dart:convert';
import 'dart:io';

import 'package:yaml/yaml.dart';

const documentVersion = 'gizclaw.test/v1alpha1';
const maxDocumentBytes = 4 << 20;
const defaultTaskTimeout = Duration(minutes: 5);

const supportedOperations = {
  'rpc',
  'client_rpc',
  'http',
  'output',
  'reconnect',
};

const allOperations = [
  'rpc',
  'rpc_stream',
  'client_rpc',
  'http',
  'speech',
  'peer_stream',
  'output',
  'review_op',
  'barrier',
  'reconnect',
  'workspace_relay',
];

const clientRpcMethods = {
  'client.info.get',
  'client.identifiers.get',
  'client.tool.invoke',
  'client.device.status.get',
  'client.device.volume.set',
  'client.device.sound.play',
  'client.device.reboot',
  'client.wifi.status.get',
  'client.wifi.saved.list',
  'client.wifi.saved.forget',
  'client.wifi.scan',
  'client.wifi.connect',
};

/// Methods this runner can install a provider for. `client.tool.invoke` needs
/// the tool-serving surface the Flutter device SDK exposes separately.
const supportedClientRpcMethods = {
  'client.info.get',
  'client.identifiers.get',
  'client.device.status.get',
  'client.device.volume.set',
  'client.device.sound.play',
  'client.device.reboot',
  'client.wifi.status.get',
  'client.wifi.saved.list',
  'client.wifi.saved.forget',
  'client.wifi.scan',
  'client.wifi.connect',
};

final _namePattern = RegExp(r'^[a-z0-9][a-z0-9._-]{0,127}$');
final _stepIdPattern = RegExp(r'^[A-Za-z_][A-Za-z0-9_-]{0,63}$');
final _variablePattern = RegExp(r'^[A-Za-z_][A-Za-z0-9_]{0,63}$');
final _clientPattern = RegExp(r'^[A-Za-z_][A-Za-z0-9_-]{0,63}$');
final _referencePattern = RegExp(r'\$\{([A-Za-z_][A-Za-z0-9_]*)\}');
final _durationPattern = RegExp(
  r'^(?:(\d+(?:\.\d+)?)h)?(?:(\d+(?:\.\d+)?)m)?'
  r'(?:(\d+(?:\.\d+)?)s)?(?:(\d+(?:\.\d+)?)ms)?$',
);

/// Raised when a document uses a step kind or variable type outside the subset
/// this runner executes.
class UnsupportedStepException implements Exception {
  const UnsupportedStepException(this.operation);

  final String operation;

  @override
  String toString() => 'unsupported step kind for this runner: $operation';
}

/// Raised when a document is malformed.
class DocumentException implements Exception {
  const DocumentException(this.path, this.message);

  final String path;
  final String message;

  @override
  String toString() => '$path: $message';
}

class VariableSpec {
  const VariableSpec({
    required this.direction,
    required this.type,
    this.value,
    this.env,
    this.generate,
    this.secret = false,
  });

  final String direction;
  final String type;
  final Object? value;
  final String? env;
  final String? generate;
  final bool secret;

  bool get isOutput => direction == 'output';
}

class ClientSpec {
  const ClientSpec({
    required this.identity,
    required this.connection,
    required this.accessPoint,
    this.registrationToken,
  });

  final String identity;
  final String connection;
  final String accessPoint;
  final String? registrationToken;
}

class Step {
  const Step({
    required this.id,
    required this.raw,
    this.client,
    this.timeout,
    this.saveAs,
    this.capture = const {},
    this.expect = const {},
    this.expectError,
  });

  final String id;
  final Map<String, Object?> raw;
  final String? client;
  final String? timeout;
  final String? saveAs;
  final Map<String, String> capture;
  final Map<String, Map<String, Object?>> expect;
  final Map<String, Object?>? expectError;

  Map<String, Object?>? get rpc => raw['rpc'] as Map<String, Object?>?;
  Map<String, Object?>? get clientRpc =>
      raw['client_rpc'] as Map<String, Object?>?;
  Map<String, Object?>? get http => raw['http'] as Map<String, Object?>?;
  Map<String, Object?>? get output => raw['output'] as Map<String, Object?>?;
  Map<String, Object?>? get reconnect =>
      raw['reconnect'] as Map<String, Object?>?;

  String get operation {
    for (final name in allOperations) {
      if (raw[name] != null) {
        return name;
      }
    }
    return '';
  }
}

class GiztestDocument {
  const GiztestDocument({
    required this.path,
    required this.version,
    required this.name,
    required this.repeat,
    required this.clients,
    required this.variables,
    required this.steps,
    required this.finalizers,
    this.timeout,
    this.redact = const [],
  });

  final String path;
  final String version;
  final String name;
  final int repeat;
  final String? timeout;
  final Map<String, ClientSpec> clients;
  final Map<String, VariableSpec> variables;
  final List<Step> steps;
  final List<Step> finalizers;
  final List<String> redact;

  Duration get taskTimeout => timeout == null || timeout!.isEmpty
      ? defaultTaskTimeout
      : parseDuration(timeout!);
}

/// Parses a Go duration string such as `30s`, `2m30s` or `250ms`.
Duration parseDuration(String value) {
  final match = _durationPattern.firstMatch(value.trim());
  if (match == null ||
      List.generate(
        4,
        (index) => match.group(index + 1),
      ).every((g) => g == null)) {
    throw FormatException('invalid duration: $value');
  }
  double part(int group) => double.parse(match.group(group) ?? '0');
  final micros =
      part(1) * 3600 * 1e6 + part(2) * 60 * 1e6 + part(3) * 1e6 + part(4) * 1e3;
  return Duration(microseconds: micros.round());
}

/// operationNeedsClient mirrors the Go runner: every kind except barrier,
/// output, review and workspace_relay runs against a declared client.
bool operationNeedsClient(String operation) {
  return operation != 'barrier' &&
      operation != 'output' &&
      operation != 'review_op' &&
      operation != 'workspace_relay';
}

/// Converts YAML nodes into plain Dart maps, lists and scalars.
Object? plainYaml(Object? node) {
  if (node is YamlMap) {
    return <String, Object?>{
      for (final entry in node.entries)
        entry.key.toString(): plainYaml(entry.value),
    };
  }
  if (node is YamlList) {
    return node.map(plainYaml).toList();
  }
  return node;
}

/// Finds every `${name}` reference in a step, matching the Go runner's
/// whole-step JSON scan.
List<String> collectReferences(Step step) {
  final found = <String>{};
  for (final match in _referencePattern.allMatches(jsonEncode(step.raw))) {
    found.add(match.group(1)!);
  }
  return found.toList()..sort();
}

Never _fail(String path, String message) =>
    throw DocumentException(path, message);

/// Enforces the four-line user story header the Go runner requires before any
/// YAML content.
void validateUserStory(String path, String text) {
  const prefixes = ['User Story:', 'As a ', 'I want ', 'So that '];
  final lines = <String>[];
  for (final raw in const LineSplitter().convert(text)) {
    final line = raw.trim();
    if (line.isEmpty) {
      continue;
    }
    if (!line.startsWith('#')) {
      break;
    }
    lines.add(line.replaceFirst(RegExp(r'^#+'), '').trim());
    if (lines.length == prefixes.length) {
      break;
    }
  }
  if (lines.length < prefixes.length) {
    _fail(path, 'user story header requires four comment lines');
  }
  for (var index = 0; index < prefixes.length; index++) {
    final prefix = prefixes[index];
    if (!lines[index].startsWith(prefix)) {
      _fail(path, 'user story line ${index + 1} must start with $prefix');
    }
    if (index > 0 && lines[index].substring(prefix.length).trim().isEmpty) {
      _fail(path, 'user story line ${index + 1} is empty');
    }
  }
}

Step _readStep(String path, Map<String, Object?> raw, int index, bool isFinal) {
  var id = raw['id'] as String? ?? '';
  if (id.isEmpty && isFinal) {
    id = 'finally_${index + 1}';
  }
  final expect = <String, Map<String, Object?>>{};
  for (final entry
      in (raw['expect'] as Map<String, Object?>? ?? const {}).entries) {
    expect[entry.key] = (entry.value as Map).cast<String, Object?>();
  }
  return Step(
    capture: (raw['capture'] as Map<String, Object?>? ?? const {}).map(
      (key, value) => MapEntry(key, value as String),
    ),
    client: raw['client'] as String?,
    expect: expect,
    expectError: raw['expect_error'] as Map<String, Object?>?,
    id: id,
    raw: raw,
    saveAs: raw['save_as'] as String?,
    timeout: raw['timeout'] as String?,
  );
}

void _validateVariables(String path, Map<String, VariableSpec> variables) {
  for (final entry in variables.entries) {
    final name = entry.key;
    final spec = entry.value;
    if (!_variablePattern.hasMatch(name)) {
      _fail(path, 'invalid variable name: $name');
    }
    if (spec.direction != 'input' && spec.direction != 'output') {
      _fail(path, 'variable $name has an invalid direction');
    }
    final sources = [
      spec.value,
      spec.env,
      spec.generate,
    ].where((source) => source != null).length;
    if (spec.direction == 'input' && sources != 1) {
      _fail(
        path,
        'input variable $name requires exactly one of value, env or generate',
      );
    }
    if (spec.isOutput && sources != 0) {
      _fail(path, 'output variable $name must not declare a source');
    }
    if (!const {
      'string',
      'integer',
      'number',
      'boolean',
      'object',
    }.contains(spec.type)) {
      // audio and binary variables belong to the speech and stream steps this
      // runner does not execute.
      throw UnsupportedStepException('variable:${spec.type}');
    }
  }
}

void _validateStep(
  String path,
  Step step,
  Map<String, ClientSpec> clients,
  Map<String, bool> produced,
  Map<String, VariableSpec> variables,
  Set<String> ids,
) {
  if (!_stepIdPattern.hasMatch(step.id)) {
    _fail(path, 'invalid step id: ${step.id}');
  }
  if (!ids.add(step.id)) {
    _fail(path, 'duplicate step id: ${step.id}');
  }
  final operation = step.operation;
  if (operation.isEmpty) {
    _fail(path, 'step ${step.id} declares no operation');
  }
  if (!supportedOperations.contains(operation)) {
    throw UnsupportedStepException(operation);
  }
  if (operationNeedsClient(operation) &&
      (step.client == null || !clients.containsKey(step.client))) {
    _fail(path, 'step ${step.id} names an unknown client');
  }
  if (step.timeout != null && parseDuration(step.timeout!) <= Duration.zero) {
    _fail(path, 'step ${step.id} timeout must be positive');
  }
  final clientRpc = step.clientRpc;
  if (clientRpc != null) {
    final method = clientRpc['method'] as String? ?? '';
    if (!clientRpcMethods.contains(method)) {
      _fail(path, 'step ${step.id} uses unknown client RPC $method');
    }
    if (!supportedClientRpcMethods.contains(method)) {
      throw UnsupportedStepException('client_rpc:$method');
    }
    final calls = clientRpc['expect_calls'] as int?;
    if (calls != null && (calls < 1 || calls > 1024)) {
      _fail(path, 'step ${step.id} expect_calls must be 1..1024');
    }
  }
  final http = step.http;
  if (http != null && !(http['path'] as String? ?? '').startsWith('/')) {
    _fail(path, 'step ${step.id} http path must be absolute');
  }

  for (final name in collectReferences(step)) {
    if (produced[name] != true) {
      _fail(path, 'step ${step.id} references unavailable variable $name');
    }
  }

  final targets = <String>[
    if (step.saveAs != null) step.saveAs!,
    ...step.capture.keys,
  ];
  for (final target in targets) {
    final spec = variables[target];
    if (spec == null) {
      _fail(path, 'step ${step.id} writes unknown variable $target');
    }
    if (!spec.isOutput) {
      _fail(path, 'step ${step.id} writes non-output variable $target');
    }
    if (produced[target] == true) {
      _fail(path, 'variable $target has more than one producer');
    }
    produced[target] = true;
  }
}

void validateDocument(GiztestDocument document) {
  final path = document.path;
  if (document.version != documentVersion) {
    _fail(path, 'unsupported version: ${document.version}');
  }
  if (!_namePattern.hasMatch(document.name)) {
    _fail(path, 'invalid document name: ${document.name}');
  }
  if (document.repeat < 1 || document.repeat > 1000) {
    _fail(path, 'repeat must be 1..1000');
  }
  if (document.taskTimeout <= Duration.zero) {
    _fail(path, 'task timeout must be positive');
  }
  if (document.clients.isEmpty || document.clients.length > 32) {
    _fail(path, 'a document declares 1..32 clients');
  }
  for (final entry in document.clients.entries) {
    if (!_clientPattern.hasMatch(entry.key)) {
      _fail(path, 'invalid client name: ${entry.key}');
    }
    if (entry.value.identity != 'ephemeral' ||
        entry.value.connection != 'webrtc') {
      _fail(path, 'client ${entry.key} must be an ephemeral webrtc client');
    }
    if (entry.value.accessPoint.isEmpty) {
      _fail(path, 'client ${entry.key} requires an access_point');
    }
  }
  _validateVariables(path, document.variables);

  final produced = <String, bool>{
    for (final entry in document.variables.entries)
      entry.key: !entry.value.isOutput,
  };
  if (document.steps.isEmpty || document.steps.length > 512) {
    _fail(path, 'a document declares 1..512 steps');
  }
  final ids = <String>{};
  for (final step in document.steps) {
    _validateStep(
      path,
      step,
      document.clients,
      produced,
      document.variables,
      ids,
    );
  }
  for (final step in document.finalizers) {
    if (step.clientRpc != null) {
      _fail(path, 'finally step ${step.id} cannot install a client RPC');
    }
    _validateStep(
      path,
      step,
      document.clients,
      produced,
      document.variables,
      ids,
    );
  }
  for (final name in document.redact) {
    if (!document.variables.containsKey(name)) {
      _fail(path, 'report.redact names unknown variable $name');
    }
  }
}

GiztestDocument parseDocument(String path, String text) {
  validateUserStory(path, text);
  final parsed = plainYaml(loadYaml(text));
  if (parsed is! Map<String, Object?>) {
    _fail(path, 'document is not a YAML mapping');
  }
  final clients = <String, ClientSpec>{};
  for (final entry
      in (parsed['clients'] as Map<String, Object?>? ?? const {}).entries) {
    final spec = (entry.value as Map).cast<String, Object?>();
    clients[entry.key] = ClientSpec(
      accessPoint: spec['access_point'] as String? ?? '',
      connection: spec['connection'] as String? ?? '',
      identity: spec['identity'] as String? ?? '',
      registrationToken: spec['registration_token'] as String?,
    );
  }
  final variables = <String, VariableSpec>{};
  for (final entry
      in (parsed['variables'] as Map<String, Object?>? ?? const {}).entries) {
    final spec = (entry.value as Map).cast<String, Object?>();
    variables[entry.key] = VariableSpec(
      direction: spec['direction'] as String? ?? '',
      env: spec['env'] as String?,
      generate: spec['generate'] as String?,
      secret: spec['secret'] as bool? ?? false,
      type: spec['type'] as String? ?? '',
      value: spec['value'],
    );
  }
  final steps = <Step>[];
  for (final (index, raw)
      in (parsed['steps'] as List<Object?>? ?? const []).indexed) {
    steps.add(
      _readStep(path, (raw as Map).cast<String, Object?>(), index, false),
    );
  }
  final finalizers = <Step>[];
  for (final (index, raw)
      in (parsed['finally'] as List<Object?>? ?? const []).indexed) {
    finalizers.add(
      _readStep(path, (raw as Map).cast<String, Object?>(), index, true),
    );
  }
  final report = parsed['report'] as Map<String, Object?>?;
  final document = GiztestDocument(
    clients: clients,
    finalizers: finalizers,
    name: parsed['name'] as String? ?? '',
    path: path,
    redact: (report?['redact'] as List<Object?>? ?? const [])
        .map((value) => value as String)
        .toList(),
    repeat: parsed['repeat'] as int? ?? 1,
    steps: steps,
    timeout: parsed['timeout'] as String?,
    variables: variables,
    version: parsed['version'] as String? ?? '',
  );
  validateDocument(document);
  return document;
}

Future<GiztestDocument> loadDocument(String path) async {
  final file = File(path);
  if (await file.length() > maxDocumentBytes) {
    _fail(path, 'document exceeds $maxDocumentBytes bytes');
  }
  return parseDocument(path, await file.readAsString());
}

/// Expands files and directories into sorted, deduplicated scenario paths.
/// Symbolic links are rejected, matching the Go runner.
Future<List<String>> discover(List<String> inputs) async {
  if (inputs.isEmpty) {
    throw const FormatException('at least one file or directory is required');
  }
  final found = <String>{};
  for (final input in inputs) {
    final absolute = File(input).absolute.path;
    final type = FileSystemEntity.typeSync(absolute, followLinks: false);
    if (type == FileSystemEntityType.link) {
      throw FormatException('symlink is not allowed: $absolute');
    }
    if (type == FileSystemEntityType.directory) {
      await for (final entity in Directory(absolute).list(recursive: true)) {
        if (entity is File && entity.path.endsWith('.giztest.yaml')) {
          found.add(entity.absolute.path);
        }
      }
      continue;
    }
    if (type != FileSystemEntityType.file) {
      throw FormatException('cannot read $absolute');
    }
    if (!absolute.endsWith('.giztest.yaml')) {
      throw FormatException('not a .giztest.yaml file: $absolute');
    }
    found.add(absolute);
  }
  if (found.isEmpty) {
    throw const FormatException('no .giztest.yaml files selected');
  }
  return found.toList()..sort();
}

class LoadResult {
  const LoadResult(this.documents, this.skipped);

  final List<GiztestDocument> documents;
  final Map<String, String> skipped;
}

Future<LoadResult> loadDocuments(List<String> paths) async {
  final documents = <GiztestDocument>[];
  final skipped = <String, String>{};
  final names = <String>{};
  for (final path in paths) {
    GiztestDocument document;
    try {
      document = await loadDocument(path);
    } on UnsupportedStepException catch (error) {
      skipped[path] = error.toString();
      continue;
    }
    if (!names.add(document.name)) {
      _fail(path, 'duplicate document name: ${document.name}');
    }
    documents.add(document);
  }
  documents.sort((left, right) => left.path.compareTo(right.path));
  return LoadResult(documents, skipped);
}
