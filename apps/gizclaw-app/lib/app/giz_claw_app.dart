import 'package:flutter/cupertino.dart';
import 'package:go_router/go_router.dart';

import '../giz_ui/giz_ui.dart';
import '../data/mobile_data_controller.dart';
import '../routing/app_router.dart';

class GizClawApp extends StatefulWidget {
  const GizClawApp({super.key, this.dataController});

  final MobileDataController? dataController;

  @override
  State<GizClawApp> createState() => _GizClawAppState();
}

class _GizClawAppState extends State<GizClawApp> {
  late final GoRouter _router = createAppRouter();
  late final MobileDataController _data;

  @override
  void initState() {
    super.initState();
    _data = widget.dataController ?? MobileDataController();
    _data.start();
  }

  @override
  void dispose() {
    _router.dispose();
    _data.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MobileDataScope(
      controller: _data,
      child: CupertinoApp.router(
        title: 'GizClaw',
        debugShowCheckedModeBanner: false,
        theme: gizCupertinoTheme,
        routerConfig: _router,
      ),
    );
  }
}
