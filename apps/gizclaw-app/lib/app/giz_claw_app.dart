import 'package:flutter/cupertino.dart';
import 'package:go_router/go_router.dart';

import '../data/mobile_data_controller.dart';
import '../giz_ui/giz_ui.dart';
import '../l10n/generated/app_localizations.dart';
import '../l10n/locale_resolution.dart';
import '../routing/app_router.dart';
import 'app_locale_controller.dart';

class GizClawApp extends StatefulWidget {
  const GizClawApp({super.key, this.dataController, this.localeController});

  final MobileDataController? dataController;
  final AppLocaleController? localeController;

  @override
  State<GizClawApp> createState() => _GizClawAppState();
}

class _GizClawAppState extends State<GizClawApp> with WidgetsBindingObserver {
  late final MobileDataController _data;
  late final AppLocaleController _locale;
  late final GoRouter _router;

  @override
  void initState() {
    super.initState();
    _data = widget.dataController ?? MobileDataController();
    _locale = widget.localeController ?? AppLocaleController();
    WidgetsBinding.instance.addObserver(this);
    _locale.addListener(_handleLocaleChanged);
    _data.setEffectiveLocale(_locale.effectiveLocale);
    _router = createAppRouter(dataController: _data);
    _data.start();
  }

  @override
  void didChangeLocales(List<Locale>? locales) {
    _locale.updatePlatformLocales(locales);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) _data.handleAppResumed();
  }

  void _handleLocaleChanged() {
    _data.setEffectiveLocale(_locale.effectiveLocale);
    if (mounted) setState(() {});
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _locale.removeListener(_handleLocaleChanged);
    _router.dispose();
    _data.dispose();
    _locale.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AppLocaleScope(
      controller: _locale,
      child: MobileDataScope(
        controller: _data,
        child: CupertinoApp.router(
          locale: _locale.effectiveLocale,
          supportedLocales: appSupportedLocales,
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          onGenerateTitle: (context) => AppLocalizations.of(context).appTitle,
          debugShowCheckedModeBanner: false,
          theme: gizCupertinoTheme,
          routerConfig: _router,
        ),
      ),
    );
  }
}
