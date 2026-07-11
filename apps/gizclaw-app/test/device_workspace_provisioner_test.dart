import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/data/device_workspace_provisioner.dart';

void main() {
  const publicKey = 'AbCdEfGhJkMnPqRsTuVwXyZ';
  final expectedName = mobileAstWorkspaceName(publicKey);

  test('derives a stable device workspace name', () {
    expect(expectedName, 'mobile-ast-abcdefghjkmn');
    expect(mobileAstWorkspaceName(publicKey), expectedName);
    expect(() => mobileAstWorkspaceName(''), throwsArgumentError);
  });

  test('reuses an existing device workspace', () async {
    var getCalls = 0;
    var createCalls = 0;
    final provisioner = DeviceWorkspaceProvisioner(
      getWorkspace: (_) async {
        getCalls++;
        throw StateError('unexpected get');
      },
      createWorkspace: (_) async {
        createCalls++;
        throw StateError('unexpected create');
      },
    );

    expect(
      await provisioner.ensureMobileAstWorkspace(
        publicKey,
        existingWorkflowName: mobileAstWorkflowName,
      ),
      isFalse,
    );
    expect(getCalls, 0);
    expect(createCalls, 0);
  });

  test('creates an embedded push-to-talk AST workspace when absent', () async {
    Workspace? created;
    final provisioner = DeviceWorkspaceProvisioner(
      getWorkspace: (_) async => throw StateError('unexpected get'),
      createWorkspace: (workspace) async {
        created = workspace;
        return workspace;
      },
    );

    expect(await provisioner.ensureMobileAstWorkspace(publicKey), isTrue);
    expect(created?.name, expectedName);
    expect(created?.workflowName, mobileAstWorkflowName);
    final ast = created!.parameters.asttranslateWorkspaceParameters;
    expect(
      ast.agentType,
      ASTTranslateWorkspaceParametersAgentType
          .ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_AST_TRANSLATE,
    );
    expect(ast.input, WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK);
    expect(ast.langPair, 'auto');
    expect(ast.translationModel, mobileAstWorkflowName);
  });

  test('recovers when another ensure creates the workspace first', () async {
    final provisioner = DeviceWorkspaceProvisioner(
      getWorkspace: (_) async =>
          Workspace(name: expectedName, workflowName: mobileAstWorkflowName),
      createWorkspace: (_) async => throw RpcError(409, 'already exists'),
    );

    expect(await provisioner.ensureMobileAstWorkspace(publicKey), isTrue);
  });

  test('rejects a device workspace bound to another workflow', () async {
    final provisioner = DeviceWorkspaceProvisioner(
      getWorkspace: (_) async => throw StateError('unexpected get'),
      createWorkspace: (_) async => throw StateError('unexpected create'),
    );

    expect(
      provisioner.ensureMobileAstWorkspace(
        publicKey,
        existingWorkflowName: 'other-workflow',
      ),
      throwsStateError,
    );
  });
}
