import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../l10n/locale_resolution.dart';

enum AppLanguagePreference {
  system('system'),
  english('en'),
  simplifiedChinese('zh-CN');

  const AppLanguagePreference(this.storageValue);

  final String storageValue;

  static AppLanguagePreference? fromStorage(String? value) {
    for (final preference in values) {
      if (preference.storageValue == value) return preference;
    }
    return null;
  }
}

abstract interface class AppLocaleStore {
  Future<String?> read();

  Future<void> write(String value);
}

class SharedPreferencesAppLocaleStore implements AppLocaleStore {
  SharedPreferencesAppLocaleStore({SharedPreferencesAsync? preferences})
    : _preferences = preferences ?? SharedPreferencesAsync();

  static const storageKey = 'gizclaw.app.language.v1';

  final SharedPreferencesAsync _preferences;

  @override
  Future<String?> read() => _preferences.getString(storageKey);

  @override
  Future<void> write(String value) => _preferences.setString(storageKey, value);
}

class AppLocaleController extends ChangeNotifier {
  AppLocaleController({
    AppLocaleStore? store,
    AppLanguagePreference preference = AppLanguagePreference.system,
    List<Locale>? platformLocales,
  }) : _store = store ?? _MemoryAppLocaleStore(),
       _preference = preference,
       _platformLocales = List.unmodifiable(
         platformLocales ?? WidgetsBinding.instance.platformDispatcher.locales,
       );

  static Future<AppLocaleController> load({
    AppLocaleStore? store,
    List<Locale>? platformLocales,
  }) async {
    final resolvedStore = store ?? SharedPreferencesAppLocaleStore();
    AppLanguagePreference preference = AppLanguagePreference.system;
    try {
      preference =
          AppLanguagePreference.fromStorage(await resolvedStore.read()) ??
          AppLanguagePreference.system;
    } catch (_) {
      preference = AppLanguagePreference.system;
    }
    return AppLocaleController(
      store: resolvedStore,
      preference: preference,
      platformLocales: platformLocales,
    );
  }

  final AppLocaleStore _store;
  AppLanguagePreference _preference;
  List<Locale> _platformLocales;
  Future<void> _writeTail = Future<void>.value();

  AppLanguagePreference get preference => _preference;

  Locale get effectiveLocale => switch (_preference) {
    AppLanguagePreference.system => resolveSystemLocale(_platformLocales),
    AppLanguagePreference.english => appEnglishLocale,
    AppLanguagePreference.simplifiedChinese => appSimplifiedChineseLocale,
  };

  void updatePlatformLocales(List<Locale>? locales) {
    final next = List<Locale>.unmodifiable(locales ?? const <Locale>[]);
    if (_localeListsEqual(_platformLocales, next)) return;
    final previous = effectiveLocale;
    _platformLocales = next;
    if (_preference == AppLanguagePreference.system &&
        previous != effectiveLocale) {
      notifyListeners();
    }
  }

  Future<void> setPreference(AppLanguagePreference preference) async {
    if (_preference == preference) return;
    _preference = preference;
    notifyListeners();

    final operation = _writeTail.then(
      (_) => _store.write(preference.storageValue),
    );
    _writeTail = operation.catchError((_) {});
    await operation;
  }
}

class _MemoryAppLocaleStore implements AppLocaleStore {
  String? _value;

  @override
  Future<String?> read() async => _value;

  @override
  Future<void> write(String value) async => _value = value;
}

class AppLocaleScope extends InheritedNotifier<AppLocaleController> {
  const AppLocaleScope({
    super.key,
    required AppLocaleController controller,
    required super.child,
  }) : super(notifier: controller);

  static AppLocaleController watch(BuildContext context) {
    final scope = context.dependOnInheritedWidgetOfExactType<AppLocaleScope>();
    assert(scope != null, 'AppLocaleScope is missing');
    return scope!.notifier!;
  }

  static AppLocaleController read(BuildContext context) {
    final element = context
        .getElementForInheritedWidgetOfExactType<AppLocaleScope>();
    final scope = element?.widget as AppLocaleScope?;
    assert(scope != null, 'AppLocaleScope is missing');
    return scope!.notifier!;
  }
}

bool _localeListsEqual(List<Locale> left, List<Locale> right) {
  if (left.length != right.length) return false;
  for (var index = 0; index < left.length; index += 1) {
    if (left[index] != right[index]) return false;
  }
  return true;
}
