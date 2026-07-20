import 'package:flutter/cupertino.dart';
import 'package:gizclaw/gizclaw.dart';

import '../giz_ui/giz_ui.dart';
import '../l10n/generated/app_localizations.dart';
import '../prototype/prototype_models.dart';

enum AppWorkflowGroup { doubao, translate, raids, internal }

class FlowcraftModelRequirements {
  const FlowcraftModelRequirements({
    required this.generateModel,
    required this.extractModel,
    required this.embeddingModel,
  });

  final bool generateModel;
  final bool extractModel;
  final bool embeddingModel;
}

class AppWorkflowDefinition {
  const AppWorkflowDefinition({
    required this.alias,
    required this.driver,
    required this.group,
    required this.icon,
    required this.bannerColor,
    this.selectable = true,
    this.flowcraftRequirements,
    this.languagePair,
  }) : assert(
         driver != WorkflowDriverKind.flowcraft ||
             flowcraftRequirements != null,
       ),
       assert(
         driver != WorkflowDriverKind.astTranslate || languagePair != null,
       );

  final String alias;
  final Color bannerColor;
  final WorkflowDriverKind driver;
  final FlowcraftModelRequirements? flowcraftRequirements;
  final AppWorkflowGroup group;
  final IconData icon;
  final String? languagePair;
  final bool selectable;

  WorkflowCard card(AppLocalizations l10n, {String? name}) {
    final (title, subtitle) = switch (alias) {
      'doubao-realtime' => (
        l10n.workflowDoubaoRealtimeTitle,
        l10n.workflowDoubaoRealtimeSubtitle,
      ),
      'translate-zh-en-auto' => (
        l10n.workflowTranslateZhEnTitle,
        l10n.workflowTranslationSubtitle,
      ),
      'translate-zh-ja' => (
        l10n.workflowTranslateZhJaTitle,
        l10n.workflowTranslationSubtitle,
      ),
      'translate-zh-ko' => (
        l10n.workflowTranslateZhKoTitle,
        l10n.workflowTranslationSubtitle,
      ),
      'translate-zh-es' => (
        l10n.workflowTranslateZhEsTitle,
        l10n.workflowTranslationSubtitle,
      ),
      'chat' => (l10n.workflowChatTitle, l10n.workflowChatSubtitle),
      'journey' => (l10n.workflowJourneyTitle, l10n.workflowJourneySubtitle),
      'murder-mystery' => (
        l10n.workflowMurderMysteryTitle,
        l10n.workflowMurderMysterySubtitle,
      ),
      'chatroom' => (l10n.workflowChatroomTitle, l10n.workflowChatroomSubtitle),
      _ => (alias, ''),
    };
    return WorkflowCard(
      name: name ?? alias,
      title: title,
      subtitle: subtitle,
      driverLabel: driver.label,
      category: group.name,
      bannerColor: bannerColor,
      icon: icon,
      driver: driver,
    );
  }

  WorkspaceParameters workspaceParameters({
    String? generateModel,
    String? extractModel,
    String? embeddingModel,
  }) => switch (driver) {
    WorkflowDriverKind.flowcraft => WorkspaceParameters(
      flowcraftWorkspaceParameters: FlowcraftWorkspaceParameters(
        agentType: FlowcraftWorkspaceParametersAgentType
            .FLOWCRAFT_WORKSPACE_PARAMETERS_AGENT_TYPE_FLOWCRAFT,
        input: WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
        generateModel: flowcraftRequirements!.generateModel
            ? _nonEmpty(generateModel)
            : null,
        extractModel: flowcraftRequirements!.extractModel
            ? _nonEmpty(extractModel)
            : null,
        embeddingModel: flowcraftRequirements!.embeddingModel
            ? _nonEmpty(embeddingModel)
            : null,
      ),
    ),
    WorkflowDriverKind.doubaoRealtime => WorkspaceParameters(
      doubaoRealtimeWorkspaceParameters: DoubaoRealtimeWorkspaceParameters(
        agentType: DoubaoRealtimeWorkspaceParametersAgentType
            .DOUBAO_REALTIME_WORKSPACE_PARAMETERS_AGENT_TYPE_DOUBAO_REALTIME,
        input: WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
      ),
    ),
    WorkflowDriverKind.astTranslate => WorkspaceParameters(
      asttranslateWorkspaceParameters: ASTTranslateWorkspaceParameters(
        agentType: ASTTranslateWorkspaceParametersAgentType
            .ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_AST_TRANSLATE,
        enableSourceLanguageDetect: languagePair == 'auto',
        input: WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
        langPair: languagePair,
        mode: ASTTranslateMode.ASTTRANSLATE_MODE_S2S,
      ),
    ),
    _ => throw UnsupportedError('Creating $alias workspaces is not supported'),
  };
}

const appWorkflowDefinitions = <AppWorkflowDefinition>[
  AppWorkflowDefinition(
    alias: 'doubao-realtime',
    driver: WorkflowDriverKind.doubaoRealtime,
    group: AppWorkflowGroup.doubao,
    icon: GizIcons.waveform_path,
    bannerColor: GizColors.coral,
  ),
  AppWorkflowDefinition(
    alias: 'translate-zh-en-auto',
    driver: WorkflowDriverKind.astTranslate,
    group: AppWorkflowGroup.translate,
    icon: GizIcons.globe,
    bannerColor: GizColors.lavender,
    languagePair: 'auto',
  ),
  AppWorkflowDefinition(
    alias: 'translate-zh-ja',
    driver: WorkflowDriverKind.astTranslate,
    group: AppWorkflowGroup.translate,
    icon: GizIcons.globe,
    bannerColor: GizColors.lavender,
    languagePair: 'zh/ja',
  ),
  AppWorkflowDefinition(
    alias: 'translate-zh-ko',
    driver: WorkflowDriverKind.astTranslate,
    group: AppWorkflowGroup.translate,
    icon: GizIcons.globe,
    bannerColor: GizColors.lavender,
    languagePair: 'zh/ko',
  ),
  AppWorkflowDefinition(
    alias: 'translate-zh-es',
    driver: WorkflowDriverKind.astTranslate,
    group: AppWorkflowGroup.translate,
    icon: GizIcons.globe,
    bannerColor: GizColors.lavender,
    languagePair: 'zh/es',
  ),
  AppWorkflowDefinition(
    alias: 'chat',
    driver: WorkflowDriverKind.flowcraft,
    group: AppWorkflowGroup.raids,
    icon: GizIcons.rectangle_3_offgrid,
    bannerColor: GizColors.blue,
    flowcraftRequirements: FlowcraftModelRequirements(
      generateModel: true,
      extractModel: true,
      embeddingModel: false,
    ),
  ),
  AppWorkflowDefinition(
    alias: 'journey',
    driver: WorkflowDriverKind.flowcraft,
    group: AppWorkflowGroup.raids,
    icon: GizIcons.scope,
    bannerColor: GizColors.blue,
    flowcraftRequirements: FlowcraftModelRequirements(
      generateModel: true,
      extractModel: true,
      embeddingModel: false,
    ),
  ),
  AppWorkflowDefinition(
    alias: 'murder-mystery',
    driver: WorkflowDriverKind.flowcraft,
    group: AppWorkflowGroup.raids,
    icon: GizIcons.question_circle,
    bannerColor: GizColors.blue,
    flowcraftRequirements: FlowcraftModelRequirements(
      generateModel: true,
      extractModel: false,
      embeddingModel: false,
    ),
  ),
  AppWorkflowDefinition(
    alias: 'chatroom',
    driver: WorkflowDriverKind.chatroom,
    group: AppWorkflowGroup.internal,
    icon: GizIcons.waveform,
    bannerColor: GizColors.accent,
    selectable: false,
  ),
];

AppWorkflowDefinition? appWorkflowDefinition(String alias) {
  alias = _legacyWorkflowAliases[alias] ?? alias;
  for (final definition in appWorkflowDefinitions) {
    if (definition.alias == alias) return definition;
  }
  return null;
}

WorkflowCard? appWorkflowCard(String alias, Locale locale) {
  final definition = appWorkflowDefinition(alias);
  if (definition == null) return null;
  return definition.card(lookupAppLocalizations(locale), name: alias);
}

bool isLegacyAppWorkflowAlias(String alias) =>
    _legacyWorkflowAliases.containsKey(alias);

List<WorkflowCard> appWorkflowCards(Locale locale, {bool selectable = false}) {
  final l10n = lookupAppLocalizations(locale);
  return List.unmodifiable([
    for (final definition in appWorkflowDefinitions)
      if (!selectable || definition.selectable) definition.card(l10n),
  ]);
}

String? _nonEmpty(String? value) {
  final normalized = value?.trim() ?? '';
  return normalized.isEmpty ? null : normalized;
}

const _legacyWorkflowAliases = <String, String>{
  'ast-translate-zh-en-auto': 'translate-zh-en-auto',
  'ast-translate-zh-ja': 'translate-zh-ja',
  'ast-translate-zh-ko': 'translate-zh-ko',
  'ast-translate-zh-es': 'translate-zh-es',
};
