import 'package:flutter/cupertino.dart';
import 'package:go_router/go_router.dart';

import '../app/app_shell.dart';
import '../app/global_conversation_control.dart';
import '../features/browse/browse_pages.dart';
import '../features/chats/chat_pages.dart';
import '../features/pet/pet_page.dart';
import '../features/social/social_pages.dart';
import '../giz_ui/giz_ui.dart';
import '../prototype/prototype_data.dart';
import '../prototype/prototype_models.dart';

GoRouter createAppRouter() {
  final rootNavigatorKey = GlobalKey<NavigatorState>();
  return GoRouter(
    navigatorKey: rootNavigatorKey,
    initialLocation: '/active',
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
                    _page(state, const ActiveChatPage()),
              ),
              GoRoute(
                path: '/browse',
                pageBuilder: (context, state) =>
                    _page(state, const BrowsePage()),
                routes: [
                  GoRoute(
                    path: 'collections/:collectionId',
                    pageBuilder: (context, state) => _page(
                      state,
                      CollectionPage(
                        collection: collectionById(
                          state.pathParameters['collectionId']!,
                        ),
                      ),
                    ),
                  ),
                  GoRoute(
                    path: 'workflows',
                    pageBuilder: (context, state) =>
                        _page(state, const AllWorkflowsPage()),
                    routes: [
                      GoRoute(
                        path: ':workflowName',
                        pageBuilder: (context, state) => _page(
                          state,
                          WorkflowDetailPage(
                            workflowName: state.pathParameters['workflowName']!,
                          ),
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ],
          ),
          StatefulShellBranch(
            initialLocation: '/raids',
            routes: [
              GoRoute(
                path: '/raids',
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
