import 'package:flutter/cupertino.dart';
import 'package:gizclaw/gizclaw.dart';

import '../giz_ui/giz_ui.dart';
import '../prototype/prototype_models.dart';

class AppWorkflowCollection {
  const AppWorkflowCollection({
    required this.id,
    required this.englishName,
    required this.chineseName,
    required this.icon,
    required this.bannerColor,
  });

  final Color bannerColor;
  final String chineseName;
  final String englishName;
  final IconData icon;
  final String id;

  String displayName(Locale locale) =>
      locale.languageCode == 'zh' ? chineseName : englishName;
}

const appWorkflowCollections = <AppWorkflowCollection>[
  AppWorkflowCollection(
    id: 'assistants',
    englishName: 'Assistants',
    chineseName: '智能助手',
    icon: GizIcons.waveform_path,
    bannerColor: GizColors.coral,
  ),
  AppWorkflowCollection(
    id: 'translates',
    englishName: 'Translates',
    chineseName: '翻译',
    icon: GizIcons.globe,
    bannerColor: GizColors.lavender,
  ),
  AppWorkflowCollection(
    id: 'raids',
    englishName: 'Raids',
    chineseName: '大冒险',
    icon: GizIcons.scope,
    bannerColor: GizColors.blue,
  ),
  AppWorkflowCollection(
    id: 'story-teller',
    englishName: 'Story Teller',
    chineseName: '故事大王',
    icon: GizIcons.wand_stars,
    bannerColor: GizColors.accent,
  ),
  AppWorkflowCollection(
    id: 'role-play',
    englishName: 'Role Play',
    chineseName: '角色扮演',
    icon: GizIcons.person_2,
    bannerColor: GizColors.secondaryInk,
  ),
];

AppWorkflowCollection appWorkflowCollection(String id) {
  return appWorkflowCollections.firstWhere(
    (collection) => collection.id == id,
    orElse: () => throw ArgumentError.value(id, 'id', 'unknown collection'),
  );
}

WorkflowCard appWorkflowCard(Workflow workflow, Locale locale) {
  final collection = appWorkflowCollection(workflow.collection);
  final text = _localizedAliasText(workflow.i18n, locale);
  final title = text?.displayName.trim();
  final subtitle = text?.description.trim();
  final driver = _workflowDriver(workflow.driver);
  return WorkflowCard(
    name: workflow.alias,
    title: title == null || title.isEmpty ? workflow.alias : title,
    subtitle: subtitle ?? '',
    driverLabel: driver.label,
    collection: workflow.collection,
    bannerColor: collection.bannerColor,
    icon: collection.icon,
    driver: driver,
    workspaceLangPair: workflow.hasWorkspaceLangPair()
        ? workflow.workspaceLangPair
        : null,
  );
}

AliasI18nText? _localizedAliasText(
  Map<String, AliasI18nText> translations,
  Locale locale,
) {
  final country = locale.countryCode?.trim();
  final exact = country == null || country.isEmpty
      ? locale.languageCode
      : '${locale.languageCode}-$country';
  return translations[exact] ??
      translations[locale.languageCode] ??
      translations['en'] ??
      (translations.isEmpty ? null : translations.values.first);
}

WorkflowDriverKind _workflowDriver(WorkflowDriver driver) => switch (driver) {
  WorkflowDriver.WORKFLOW_DRIVER_FLOWCRAFT => WorkflowDriverKind.flowcraft,
  WorkflowDriver.WORKFLOW_DRIVER_DOUBAO_REALTIME =>
    WorkflowDriverKind.doubaoRealtime,
  WorkflowDriver.WORKFLOW_DRIVER_AST_TRANSLATE =>
    WorkflowDriverKind.astTranslate,
  WorkflowDriver.WORKFLOW_DRIVER_CHATROOM => WorkflowDriverKind.chatroom,
  _ => WorkflowDriverKind.unsupported,
};
