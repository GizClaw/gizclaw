import 'package:gizclaw/gizclaw.dart';

const mobileAstWorkflowName = 'volc-ast-translate';
const mobileAstLanguagePair = 'auto';
const mobileAstDisplayName = 'Chinese-English Translator';

typedef WorkspaceGetter = Future<Workspace> Function(String name);
typedef WorkspaceCreator = Future<Workspace> Function(Workspace workspace);
typedef WorkspacePutter =
    Future<Workspace> Function(String name, Workspace workspace);

class DeviceWorkspaceProvisioner {
  DeviceWorkspaceProvisioner({
    required WorkspaceGetter getWorkspace,
    required WorkspaceCreator createWorkspace,
    required WorkspacePutter putWorkspace,
  }) : _getWorkspace = getWorkspace,
       _createWorkspace = createWorkspace,
       _putWorkspace = putWorkspace;

  factory DeviceWorkspaceProvisioner.forClient(GizClawClient client) {
    return DeviceWorkspaceProvisioner(
      getWorkspace: (name) async => (await client.getWorkspace(name)).value,
      createWorkspace: (workspace) async =>
          (await client.createWorkspace(workspace)).value,
      putWorkspace: (name, workspace) async =>
          (await client.putWorkspace(name, workspace)).value,
    );
  }

  final WorkspaceGetter _getWorkspace;
  final WorkspaceCreator _createWorkspace;
  final WorkspacePutter _putWorkspace;

  /// Returns true when the workspace snapshot needs to be refreshed.
  Future<bool> ensureMobileAstWorkspace(
    String clientPublicKey, {
    Workspace? existingWorkspace,
  }) async {
    final name = mobileAstWorkspaceName(clientPublicKey);
    if (existingWorkspace != null) {
      _validate(existingWorkspace, name);
      return _converge(existingWorkspace, name);
    }

    final workspace = mobileAstWorkspace(name);
    try {
      _validate(await _createWorkspace(workspace), name);
    } on RpcError catch (error) {
      if (error.code != 409) rethrow;
      final current = await _getWorkspace(name);
      _validate(current, name);
      await _converge(current, name);
    }
    return true;
  }

  Future<bool> _converge(Workspace current, String name) async {
    if (_hasMobileAstConfiguration(current)) return false;

    final input = _preservedInputMode(current);
    final updated = current.deepCopy()
      ..displayName = mobileAstDisplayName
      ..parameters = mobileAstParameters(input: input);
    _validate(await _putWorkspace(name, updated), name);
    return true;
  }
}

String mobileAstWorkspaceName(String clientPublicKey) {
  if (clientPublicKey.isEmpty) {
    throw ArgumentError.value(
      clientPublicKey,
      'clientPublicKey',
      'must not be empty',
    );
  }
  final suffix = clientPublicKey.length <= 12
      ? clientPublicKey
      : clientPublicKey.substring(0, 12);
  return 'mobile-ast-${suffix.toLowerCase()}';
}

Workspace mobileAstWorkspace(String name) {
  return Workspace(
    displayName: mobileAstDisplayName,
    name: name,
    workflowName: mobileAstWorkflowName,
    parameters: mobileAstParameters(),
  );
}

WorkspaceParameters mobileAstParameters({
  WorkspaceInputMode input =
      WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
}) {
  return WorkspaceParameters(
    asttranslateWorkspaceParameters: ASTTranslateWorkspaceParameters(
      agentType: ASTTranslateWorkspaceParametersAgentType
          .ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_AST_TRANSLATE,
      enableSourceLanguageDetect: true,
      input: input,
      langPair: mobileAstLanguagePair,
      mode: ASTTranslateMode.ASTTRANSLATE_MODE_S2S,
      translationModel: mobileAstWorkflowName,
    ),
  );
}

bool _hasMobileAstConfiguration(Workspace workspace) {
  if (!workspace.hasParameters() ||
      !workspace.parameters.hasAsttranslateWorkspaceParameters()) {
    return false;
  }
  final ast = workspace.parameters.asttranslateWorkspaceParameters;
  return workspace.displayName == mobileAstDisplayName &&
      ast.agentType ==
          ASTTranslateWorkspaceParametersAgentType
              .ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_AST_TRANSLATE &&
      ast.enableSourceLanguageDetect &&
      _isSupportedInputMode(ast.input) &&
      ast.langPair == mobileAstLanguagePair &&
      ast.mode == ASTTranslateMode.ASTTRANSLATE_MODE_S2S &&
      ast.translationModel == mobileAstWorkflowName;
}

WorkspaceInputMode _preservedInputMode(Workspace workspace) {
  if (workspace.hasParameters() &&
      workspace.parameters.hasAsttranslateWorkspaceParameters()) {
    final input = workspace.parameters.asttranslateWorkspaceParameters.input;
    if (_isSupportedInputMode(input)) return input;
  }
  return WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK;
}

bool _isSupportedInputMode(WorkspaceInputMode input) =>
    input == WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK ||
    input == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME;

void _validate(Workspace workspace, String expectedName) {
  if (workspace.name != expectedName) {
    throw StateError('Server returned an unexpected workspace name');
  }
  _validateWorkflow(workspace.workflowName, expectedName);
}

void _validateWorkflow(String workflowName, String expectedName) {
  if (workflowName != mobileAstWorkflowName) {
    throw StateError(
      'Workspace $expectedName does not use $mobileAstWorkflowName',
    );
  }
}
