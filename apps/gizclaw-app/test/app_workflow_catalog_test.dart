import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/workflows/app_workflow_catalog.dart';

void main() {
  const aliases = [
    'doubao-realtime',
    'translate-zh-en-auto',
    'translate-zh-ja',
    'translate-zh-ko',
    'translate-zh-es',
    'chat',
    'journey',
    'murder-mystery',
    'chatroom',
  ];

  test('defines the exact RuntimeProfile/default workflow aliases', () {
    expect(
      appWorkflowDefinitions.map((definition) => definition.alias),
      aliases,
    );
    expect(
      appWorkflowDefinitions
          .where((definition) => definition.selectable)
          .map((definition) => definition.alias),
      aliases.take(8),
    );
    expect(appWorkflowDefinition('chatroom')?.selectable, isFalse);
    expect(appWorkflowDefinition('server-owned-workflow'), isNull);
  });

  test('builds App-owned localized cards in fixed order', () {
    final english = appWorkflowCards(const Locale('en'));
    final chinese = appWorkflowCards(const Locale('zh'));

    expect(english.map((card) => card.name), aliases);
    expect(chinese.map((card) => card.name), aliases);
    expect(
      english.every(
        (card) => card.source == ResourceSource.RESOURCE_SOURCE_RUNTIME,
      ),
      isTrue,
    );
    expect(english.first.title, 'Doubao Realtime');
    expect(chinese.first.title, isNot(english.first.title));
    expect(english.last.title, 'Chatroom');
    expect(chinese.last.title, '聊天室');
    expect(chinese.last.subtitle, '好友与群组对话');
  });

  test('builds typed parameters for every selectable workflow', () {
    for (final definition in appWorkflowDefinitions.where(
      (definition) => definition.selectable,
    )) {
      final parameters = definition.workspaceParameters(
        generateModel: 'generate',
        extractModel: 'extract',
        embeddingModel: 'embedding',
      );
      switch (definition.group) {
        case AppWorkflowGroup.doubao:
          expect(parameters.hasDoubaoRealtimeWorkspaceParameters(), isTrue);
        case AppWorkflowGroup.translate:
          expect(parameters.hasAsttranslateWorkspaceParameters(), isTrue);
          expect(
            parameters.asttranslateWorkspaceParameters.langPair,
            definition.languagePair,
          );
        case AppWorkflowGroup.raids:
          expect(parameters.hasFlowcraftWorkspaceParameters(), isTrue);
          final flowcraft = parameters.flowcraftWorkspaceParameters;
          final requirements = definition.flowcraftRequirements!;
          expect(flowcraft.hasGenerateModel(), requirements.generateModel);
          expect(flowcraft.hasExtractModel(), requirements.extractModel);
          expect(flowcraft.hasEmbeddingModel(), requirements.embeddingModel);
        case AppWorkflowGroup.internal:
          fail('Internal workflows must not be selectable');
      }
    }
  });

  test('keeps fixed Flowcraft requirements per alias', () {
    expect(
      appWorkflowDefinition('chat')?.flowcraftRequirements?.extractModel,
      isTrue,
    );
    expect(
      appWorkflowDefinition('journey')?.flowcraftRequirements?.extractModel,
      isTrue,
    );
    expect(
      appWorkflowDefinition(
        'murder-mystery',
      )?.flowcraftRequirements?.extractModel,
      isFalse,
    );
  });
}
