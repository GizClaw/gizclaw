import 'package:flutter/widgets.dart';

const appEnglishLocale = Locale('en');
const appSimplifiedChineseLocale = Locale('zh', 'CN');

const appSupportedLocales = [appEnglishLocale, appSimplifiedChineseLocale];

Locale resolveSystemLocale(List<Locale> platformLocales) {
  final locale = platformLocales.firstOrNull;
  if (locale == null) return appEnglishLocale;
  final language = locale.languageCode.toLowerCase();
  if (language == 'en') return appEnglishLocale;
  if (language != 'zh') return appEnglishLocale;

  final script = locale.scriptCode?.toLowerCase();
  final country = locale.countryCode?.toLowerCase();
  if (script == 'hant' ||
      country == 'tw' ||
      country == 'hk' ||
      country == 'mo') {
    return appEnglishLocale;
  }
  if (script == 'hans' ||
      country == 'cn' ||
      country == 'sg' ||
      (script == null && country == null)) {
    return appSimplifiedChineseLocale;
  }
  return appEnglishLocale;
}
