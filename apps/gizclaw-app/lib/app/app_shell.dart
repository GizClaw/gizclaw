import 'package:flutter/cupertino.dart';
import 'package:go_router/go_router.dart';

import '../giz_ui/giz_ui.dart';
import 'global_conversation_control.dart';

class AppShell extends StatelessWidget {
  const AppShell({super.key, required this.navigationShell});

  final StatefulNavigationShell navigationShell;

  static const _items = [
    BottomNavigationBarItem(
      icon: Icon(CupertinoIcons.compass),
      activeIcon: Icon(CupertinoIcons.compass_fill),
      label: 'Browse',
    ),
    BottomNavigationBarItem(
      icon: Icon(CupertinoIcons.chat_bubble_2),
      activeIcon: Icon(CupertinoIcons.chat_bubble_2_fill),
      label: 'Chats',
    ),
    BottomNavigationBarItem(
      icon: Icon(CupertinoIcons.person_2),
      activeIcon: Icon(CupertinoIcons.person_2_fill),
      label: 'Friends',
    ),
    BottomNavigationBarItem(
      icon: Icon(CupertinoIcons.sparkles),
      activeIcon: Icon(CupertinoIcons.sparkles),
      label: 'Pet',
    ),
    BottomNavigationBarItem(
      icon: Icon(CupertinoIcons.person_crop_circle),
      activeIcon: Icon(CupertinoIcons.person_crop_circle_fill),
      label: 'Me',
    ),
  ];

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    return CupertinoPageScaffold(
      child: GlobalConversationOverlay(
        child: Column(
          children: [
            Expanded(child: navigationShell),
            GizGlassBar(
              child: CupertinoTabBar(
                currentIndex: navigationShell.currentIndex,
                items: _items,
                onTap: (index) {
                  navigationShell.goBranch(
                    index,
                    initialLocation: index == navigationShell.currentIndex,
                  );
                },
                activeColor: dark ? GizColors.accent : GizColors.ink,
                inactiveColor: dark
                    ? const Color(0xA6FFFFFF)
                    : GizColors.secondaryInk,
                backgroundColor: const Color(0x00000000),
                border: null,
                iconSize: 23,
                height: 52,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
