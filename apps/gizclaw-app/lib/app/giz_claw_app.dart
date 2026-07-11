import 'package:flutter/cupertino.dart';
import 'package:go_router/go_router.dart';

import '../giz_ui/giz_ui.dart';
import '../routing/app_router.dart';

class GizClawApp extends StatefulWidget {
  const GizClawApp({super.key});

  @override
  State<GizClawApp> createState() => _GizClawAppState();
}

class _GizClawAppState extends State<GizClawApp> {
  late final GoRouter _router = createAppRouter();

  @override
  void dispose() {
    _router.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return CupertinoApp.router(
      title: 'GizClaw',
      debugShowCheckedModeBanner: false,
      theme: gizCupertinoTheme,
      routerConfig: _router,
    );
  }
}
