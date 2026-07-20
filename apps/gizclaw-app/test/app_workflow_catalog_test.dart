import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/prototype/prototype_models.dart';
import 'package:gizclaw_app/workflows/app_workflow_catalog.dart';

void main() {
  test('keeps the App collection architecture fixed', () {
    expect(appWorkflowCollections.map((collection) => collection.id), [
      'assistants',
      'translates',
      'raids',
      'story-teller',
      'role-play',
    ]);
    expect(
      appWorkflowCollection('raids').displayName(const Locale('zh')),
      '大冒险',
    );
  });

  test('projects a dynamic workflow with runtime i18n', () {
    final workflow = Workflow(
      alias: 'journey',
      collection: 'raids',
      driver: WorkflowDriver.WORKFLOW_DRIVER_FLOWCRAFT,
      i18n: {
        'en': AliasI18nText(
          displayName: 'Journey',
          description: 'A dynamic adventure',
        ),
        'zh-CN': AliasI18nText(displayName: '赛博佩特', description: '动态大冒险'),
      }.entries,
    );

    final english = appWorkflowCard(workflow, const Locale('en'));
    final chinese = appWorkflowCard(workflow, const Locale('zh', 'CN'));

    expect(english.name, 'journey');
    expect(english.collection, 'raids');
    expect(english.title, 'Journey');
    expect(chinese.title, '赛博佩特');
    expect(chinese.subtitle, '动态大冒险');
    expect(chinese.driver, WorkflowDriverKind.flowcraft);
  });

  test('projects AST workspace language metadata independently of alias', () {
    final card = appWorkflowCard(
      Workflow(
        alias: 'japanese',
        collection: 'translates',
        driver: WorkflowDriver.WORKFLOW_DRIVER_AST_TRANSLATE,
        workspaceLangPair: 'zh/ja',
        i18n: {'en': AliasI18nText(displayName: 'Japanese')}.entries,
      ),
      const Locale('en'),
    );

    expect(card.workspaceLangPair, 'zh/ja');
    expect(card.driver, WorkflowDriverKind.astTranslate);
  });

  test('falls back to English and marks unknown drivers unavailable', () {
    final card = appWorkflowCard(
      Workflow(
        alias: 'future-workflow',
        collection: 'assistants',
        i18n: {'en': AliasI18nText(displayName: 'Future Workflow')}.entries,
      ),
      const Locale('fr'),
    );

    expect(card.title, 'Future Workflow');
    expect(card.driver, WorkflowDriverKind.unsupported);
  });
}
