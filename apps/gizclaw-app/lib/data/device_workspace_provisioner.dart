import 'package:gizclaw/gizclaw.dart';

const mobileAstWorkflowName = 'volc-ast-translate';

typedef WorkspaceGetter = Future<Workspace> Function(String name);
typedef WorkspaceCreator = Future<Workspace> Function(Workspace workspace);

class DeviceWorkspaceProvisioner {
  DeviceWorkspaceProvisioner({
    required WorkspaceGetter getWorkspace,
    required WorkspaceCreator createWorkspace,
  }) : _getWorkspace = getWorkspace,
       _createWorkspace = createWorkspace;

  factory DeviceWorkspaceProvisioner.forClient(GizClawClient client) {
    return DeviceWorkspaceProvisioner(
      getWorkspace: (name) async => (await client.getWorkspace(name)).value,
      createWorkspace: (workspace) async =>
          (await client.createWorkspace(workspace)).value,
    );
  }

  final WorkspaceGetter _getWorkspace;
  final WorkspaceCreator _createWorkspace;

  /// Returns true when the workspace was absent from the previous snapshot.
  Future<bool> ensureMobileAstWorkspace(
    String clientPublicKey, {
    String? existingWorkflowName,
  }) async {
    final name = mobileAstWorkspaceName(clientPublicKey);
    if (existingWorkflowName != null) {
      _validateWorkflow(existingWorkflowName, name);
      return false;
    }

    final workspace = mobileAstWorkspace(name);
    try {
      _validate(await _createWorkspace(workspace), name);
    } on RpcError catch (error) {
      if (error.code != 409) rethrow;
      _validate(await _getWorkspace(name), name);
    }
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
    name: name,
    workflowName: mobileAstWorkflowName,
    parameters: WorkspaceParameters(
      asttranslateWorkspaceParameters: ASTTranslateWorkspaceParameters(
        agentType: ASTTranslateWorkspaceParametersAgentType
            .ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_AST_TRANSLATE,
        input: WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
        langPair: 'auto',
        translationModel: mobileAstWorkflowName,
      ),
    ),
  );
}

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
