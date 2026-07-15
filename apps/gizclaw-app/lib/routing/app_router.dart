import 'package:flutter/cupertino.dart';
import 'package:go_router/go_router.dart';

import '../app/app_shell.dart';
import '../app/global_conversation_control.dart';
import '../data/mobile_data_controller.dart';
import '../features/active/active_workspace_page.dart';
import '../features/chats/chat_pages.dart';
import '../features/identity/server_pages.dart';
import '../features/pet/pet_page.dart';
import '../features/social/social_pages.dart';
import '../giz_ui/giz_ui.dart';
import '../prototype/prototype_models.dart';

GoRouter createAppRouter({required MobileDataController dataController}) {
  final rootNavigatorKey = GlobalKey<NavigatorState>();
  return GoRouter(
    navigatorKey: rootNavigatorKey,
    initialLocation: dataController.hasActiveServer ? '/active' : '/identity',
    refreshListenable: dataController,
    redirect: (context, state) {
      if (!dataController.hasActiveServer &&
          !state.uri.path.startsWith('/identity')) {
        return '/identity';
      }
      return null;
    },
    routes: [
      GoRoute(path: '/', redirect: (_, _) => '/active'),
      StatefulShellRoute.indexedStack(
        builder: (context, state, navigationShell) {
          return AppShell(
            navigationShell: navigationShell,
            location: state.uri,
          );
        },
        branches: [
          StatefulShellBranch(
            initialLocation: '/active',
            routes: [
              GoRoute(
                path: '/active',
                pageBuilder: (context, state) =>
                    _page(state, const ActiveWorkspacePage()),
              ),
            ],
          ),
          StatefulShellBranch(
            initialLocation: '/raids',
            routes: [
              GoRoute(
                path: '/raids',
                redirect: (context, state) => state.uri.path == '/raids'
                    ? '/raids/drivers/flowcraft'
                    : null,
                pageBuilder: (context, state) =>
                    _page(state, const ChatsPage()),
                routes: [
                  GoRoute(
                    path: 'drivers/:driver',
                    pageBuilder: (context, state) => _page(
                      state,
                      DriverWorkspacesPage(
                        driver: WorkflowDriverKind.fromRouteKey(
                          state.pathParameters['driver']!,
                        ),
                      ),
                    ),
                    routes: [
                      GoRoute(
                        path: ':workspaceName',
                        parentNavigatorKey: rootNavigatorKey,
                        pageBuilder: (context, state) {
                          final workspaceName =
                              state.pathParameters['workspaceName']!;
                          final driver = WorkflowDriverKind.fromRouteKey(
                            state.pathParameters['driver']!,
                          );
                          return _page(
                            state,
                            GlobalConversationOverlay(
                              location: state.uri,
                              child: driver == WorkflowDriverKind.chatroom
                                  ? ChatroomWorkspacePage(
                                      workspaceName: workspaceName,
                                    )
                                  : WorkspaceChatPage(
                                      workspaceName: workspaceName,
                                    ),
                            ),
                          );
                        },
                      ),
                    ],
                  ),
                ],
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/friends',
                pageBuilder: (context, state) =>
                    _page(state, const FriendsPage()),
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/groups',
                pageBuilder: (context, state) =>
                    _page(state, const GroupsPage()),
                routes: [
                  GoRoute(
                    path: ':workspaceName',
                    parentNavigatorKey: rootNavigatorKey,
                    pageBuilder: (context, state) {
                      final workspaceName =
                          state.pathParameters['workspaceName']!;
                      return _page(
                        state,
                        GlobalConversationOverlay(
                          location: state.uri,
                          child: ChatroomWorkspacePage(
                            workspaceName: workspaceName,
                          ),
                        ),
                      );
                    },
                  ),
                ],
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/pets',
                pageBuilder: (context, state) => _page(state, const PetPage()),
                routes: [
                  GoRoute(
                    path: ':petId',
                    parentNavigatorKey: rootNavigatorKey,
                    pageBuilder: (context, state) => _page(
                      state,
                      GlobalConversationOverlay(
                        location: state.uri,
                        child: PetDetailPage(
                          petId: state.pathParameters['petId']!,
                        ),
                      ),
                    ),
                  ),
                ],
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/identity',
                pageBuilder: (context, state) => _page(state, const MePage()),
                routes: [
                  GoRoute(
                    path: 'servers',
                    parentNavigatorKey: rootNavigatorKey,
                    pageBuilder: (context, state) =>
                        _page(state, const ServerListPage()),
                    routes: [
                      GoRoute(
                        path: 'new',
                        parentNavigatorKey: rootNavigatorKey,
                        pageBuilder: (context, state) =>
                            _page(state, const AddServerPage()),
                      ),
                      GoRoute(
                        path: 'scan',
                        parentNavigatorKey: rootNavigatorKey,
                        pageBuilder: (context, state) =>
                            _page(state, const ScanServerQrPage()),
                      ),
                    ],
                  ),
                ],
              ),
            ],
          ),
        ],
      ),
    ],
    errorPageBuilder: (context, state) => _page(
      state,
      CupertinoPageScaffold(
        navigationBar: const CupertinoNavigationBar(
          middle: Text('Not found'),
          border: null,
        ),
        child: Center(
          child: Text(
            state.error?.toString() ?? 'This page is unavailable.',
            style: GizText.body,
          ),
        ),
      ),
    ),
  );
}

CupertinoPage<void> _page(GoRouterState state, Widget child) {
  return CupertinoPage<void>(key: state.pageKey, child: child);
}
