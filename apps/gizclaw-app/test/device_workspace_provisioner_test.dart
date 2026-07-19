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
    var putCalls = 0;
    final provisioner = DeviceWorkspaceProvisioner(
      getWorkspace: (_) async {
        getCalls++;
        throw StateError('unexpected get');
      },
      createWorkspace: (_) async {
        createCalls++;
        throw StateError('unexpected create');
      },
      putWorkspace: (_, _) async {
        putCalls++;
        throw StateError('unexpected put');
      },
    );

    expect(
      await provisioner.ensureMobileAstWorkspace(
        publicKey,
        existingWorkspace: mobileAstWorkspace(expectedName),
      ),
      isFalse,
    );
    expect(getCalls, 0);
    expect(createCalls, 0);
    expect(putCalls, 0);
  });

  test('preserves realtime mode on an existing device workspace', () async {
    var putCalls = 0;
    final realtime = mobileAstWorkspace(expectedName);
    realtime.parameters.asttranslateWorkspaceParameters.input =
        WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME;
    final provisioner = DeviceWorkspaceProvisioner(
      getWorkspace: (_) async => throw StateError('unexpected get'),
      createWorkspace: (_) async => throw StateError('unexpected create'),
      putWorkspace: (_, workspace) async {
        putCalls++;
        return workspace;
      },
    );

    expect(
      await provisioner.ensureMobileAstWorkspace(
        publicKey,
        existingWorkspace: realtime,
      ),
      isFalse,
    );
    expect(putCalls, 0);
  });

  test('creates an embedded push-to-talk AST workspace when absent', () async {
    Workspace? created;
    final provisioner = DeviceWorkspaceProvisioner(
      getWorkspace: (_) async => throw StateError('unexpected get'),
      createWorkspace: (workspace) async {
        created = workspace;
        return workspace;
      },
      putWorkspace: (_, _) async => throw StateError('unexpected put'),
    );

    expect(await provisioner.ensureMobileAstWorkspace(publicKey), isTrue);
    expect(created?.name, expectedName);
    expect(created?.workflowName, mobileAstWorkflowName);
    expect(created?.workflowSource, ResourceSource.RESOURCE_SOURCE_RUNTIME);
    final ast = created!.parameters.asttranslateWorkspaceParameters;
    expect(
      ast.agentType,
      ASTTranslateWorkspaceParametersAgentType
          .ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_AST_TRANSLATE,
    );
    expect(ast.input, WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK);
    expect(ast.enableSourceLanguageDetect, isTrue);
    expect(ast.langPair, mobileAstLanguagePair);
    expect(ast.mode, ASTTranslateMode.ASTTRANSLATE_MODE_S2S);
    expect(ast.translationModel, mobileAstTranslationModelName);
  });

  test(
    'updates an existing workspace to the fixed auto zh-en profile',
    () async {
      Workspace? updated;
      final provisioner = DeviceWorkspaceProvisioner(
        getWorkspace: (_) async => throw StateError('unexpected get'),
        createWorkspace: (_) async => throw StateError('unexpected create'),
        putWorkspace: (name, workspace) async {
          expect(name, expectedName);
          updated = workspace;
          return workspace;
        },
      );
      final stale = Workspace(
        name: expectedName,
        workflowName: mobileAstWorkflowName,
        parameters: WorkspaceParameters(
          asttranslateWorkspaceParameters: ASTTranslateWorkspaceParameters(
            input: WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME,
            langPair: 'zh/en',
          ),
        ),
      );

      expect(
        await provisioner.ensureMobileAstWorkspace(
          publicKey,
          existingWorkspace: stale,
        ),
        isTrue,
      );
      final ast = updated!.parameters.asttranslateWorkspaceParameters;
      expect(ast.langPair, mobileAstLanguagePair);
      expect(ast.enableSourceLanguageDetect, isTrue);
      expect(ast.mode, ASTTranslateMode.ASTTRANSLATE_MODE_S2S);
      expect(ast.input, WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME);
    },
  );

  test('recovers when another ensure creates the workspace first', () async {
    final provisioner = DeviceWorkspaceProvisioner(
      getWorkspace: (_) async =>
          Workspace(name: expectedName, workflowName: mobileAstWorkflowName),
      createWorkspace: (_) async => throw RpcError(409, 'already exists'),
      putWorkspace: (_, workspace) async => workspace,
    );

    expect(await provisioner.ensureMobileAstWorkspace(publicKey), isTrue);
  });

  test('rejects a device workspace bound to another workflow', () async {
    final provisioner = DeviceWorkspaceProvisioner(
      getWorkspace: (_) async => throw StateError('unexpected get'),
      createWorkspace: (_) async => throw StateError('unexpected create'),
      putWorkspace: (_, _) async => throw StateError('unexpected put'),
    );

    expect(
      provisioner.ensureMobileAstWorkspace(
        publicKey,
        existingWorkspace: Workspace(
          name: expectedName,
          workflowName: 'other-workflow',
        ),
      ),
      throwsStateError,
    );
  });
}
