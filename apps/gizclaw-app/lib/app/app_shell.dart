import 'package:flutter/cupertino.dart';
import 'package:go_router/go_router.dart';

import 'global_conversation_control.dart';

class AppShell extends StatelessWidget {
  const AppShell({
    super.key,
    required this.navigationShell,
    required this.location,
  });

  final Uri location;
  final StatefulNavigationShell navigationShell;

  @override
  Widget build(BuildContext context) {
    return CupertinoPageScaffold(
      child: GlobalConversationOverlay(
        location: location,
        navigationShell: navigationShell,
        child: navigationShell,
      ),
    );
  }
}
