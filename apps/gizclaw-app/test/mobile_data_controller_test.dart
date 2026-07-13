import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/data/mobile_data_controller.dart';
import 'package:gizclaw_app/prototype/prototype_models.dart';

void main() {
  test('creates typed defaults for a Doubao workspace', () {
    final parameters = newWorkspaceParametersForDriver(
      WorkflowDriverKind.doubaoRealtime,
    );
    final doubao = parameters.doubaoRealtimeWorkspaceParameters;
    expect(
      doubao.agentType,
      DoubaoRealtimeWorkspaceParametersAgentType
          .DOUBAO_REALTIME_WORKSPACE_PARAMETERS_AGENT_TYPE_DOUBAO_REALTIME,
    );
    expect(doubao.input, WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK);
  });

  test('creates the auto S2S profile for a translation workspace', () {
    final parameters = newWorkspaceParametersForDriver(
      WorkflowDriverKind.astTranslate,
    );
    final ast = parameters.asttranslateWorkspaceParameters;
    expect(ast.enableSourceLanguageDetect, isTrue);
    expect(ast.langPair, 'auto');
    expect(ast.mode, ASTTranslateMode.ASTTRANSLATE_MODE_S2S);
    expect(ast.hasTranslationModel(), isFalse);
  });

  test('repairs an empty parameter envelope for mode switching', () {
    final workspace = Workspace(
      name: 'translator',
      workflowName: 'volc-ast-translate',
      parameters: WorkspaceParameters(),
    );

    final repaired = workspaceWithDefaultInputParameters(
      workspace,
      WorkflowDriverKind.astTranslate,
    );

    expect(repaired, isNotNull);
    expect(
      repaired!.parameters.asttranslateWorkspaceParameters.input,
      WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
    );
    expect(
      repaired.parameters.asttranslateWorkspaceParameters.mode,
      ASTTranslateMode.ASTTRANSLATE_MODE_S2S,
    );
  });

  test('preserves existing typed workspace parameters', () {
    final workspace = Workspace(
      parameters: WorkspaceParameters(
        asttranslateWorkspaceParameters: ASTTranslateWorkspaceParameters(
          input: WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME,
          langPair: 'zh/en',
        ),
      ),
    );

    expect(
      workspaceWithDefaultInputParameters(
        workspace,
        WorkflowDriverKind.astTranslate,
      ),
      isNull,
    );
    expect(
      workspace.parameters.asttranslateWorkspaceParameters.input,
      WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME,
    );
    expect(
      workspace.parameters.asttranslateWorkspaceParameters.langPair,
      'zh/en',
    );
  });
}
