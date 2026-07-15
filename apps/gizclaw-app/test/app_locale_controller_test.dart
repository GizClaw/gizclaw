import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw_app/app/app_locale_controller.dart';
import 'package:gizclaw_app/l10n/locale_resolution.dart';

void main() {
  test('loads a persisted language preference', () async {
    final controller = await AppLocaleController.load(
      store: _MemoryLocaleStore('zh-CN'),
      platformLocales: const [Locale('en')],
    );
    addTearDown(controller.dispose);

    expect(controller.preference, AppLanguagePreference.simplifiedChinese);
    expect(controller.effectiveLocale, appSimplifiedChineseLocale);
  });

  test('system mode maps unsupported and traditional Chinese to English', () {
    expect(resolveSystemLocale(const [Locale('fr')]), appEnglishLocale);
    expect(
      resolveSystemLocale(const [Locale('zh')]),
      appSimplifiedChineseLocale,
    );
    expect(resolveSystemLocale(const [Locale('zh', 'TW')]), appEnglishLocale);
    expect(
      resolveSystemLocale(const [
        Locale.fromSubtags(languageCode: 'zh', scriptCode: 'Hant'),
      ]),
      appEnglishLocale,
    );
    expect(
      resolveSystemLocale(const [Locale('zh', 'CN')]),
      appSimplifiedChineseLocale,
    );
  });

  test('rapid changes persist in selection order', () async {
    final store = _DelayedLocaleStore();
    final controller = AppLocaleController(
      store: store,
      platformLocales: const [Locale('en')],
    );
    addTearDown(controller.dispose);

    final first = controller.setPreference(AppLanguagePreference.english);
    final second = controller.setPreference(
      AppLanguagePreference.simplifiedChinese,
    );
    await Future.wait([first, second]);

    expect(store.writes, ['en', 'zh-CN']);
    expect(controller.effectiveLocale, appSimplifiedChineseLocale);
  });
}

class _MemoryLocaleStore implements AppLocaleStore {
  _MemoryLocaleStore(this.value);

  String? value;

  @override
  Future<String?> read() async => value;

  @override
  Future<void> write(String value) async => this.value = value;
}

class _DelayedLocaleStore implements AppLocaleStore {
  final writes = <String>[];

  @override
  Future<String?> read() async => null;

  @override
  Future<void> write(String value) async {
    await Future<void>.delayed(const Duration(milliseconds: 1));
    writes.add(value);
  }
}
