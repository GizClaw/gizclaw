import 'package:flutter/cupertino.dart';
import 'package:go_router/go_router.dart';

import '../app/app_shell.dart';
import '../features/browse/browse_pages.dart';
import '../features/chats/chat_pages.dart';
import '../features/social/social_pages.dart';
import '../giz_ui/giz_ui.dart';
import '../prototype/prototype_data.dart';

GoRouter createAppRouter() {
  return GoRouter(
    initialLocation: '/browse',
    routes: [
      GoRoute(path: '/', redirect: (_, _) => '/browse'),
      StatefulShellRoute.indexedStack(
        builder: (context, state, navigationShell) {
          return AppShell(navigationShell: navigationShell);
        },
        branches: [
          StatefulShellBranch(
            routes: [
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
            initialLocation: '/chats/workspaces',
            routes: [
              GoRoute(
                path: '/chats/:mode',
                pageBuilder: (context, state) {
                  final mode = state.pathParameters['mode'] == 'groups'
                      ? ChatListMode.groups
                      : ChatListMode.workspaces;
                  return _page(state, ChatsPage(mode: mode));
                },
                routes: [
                  GoRoute(
                    path: ':conversationId',
                    pageBuilder: (context, state) {
                      if (state.pathParameters['mode'] == 'groups') {
                        return _page(
                          state,
                          GroupChatPage(
                            room: chatroomById(
                              state.pathParameters['conversationId']!,
                            ),
                          ),
                        );
                      }
                      return _page(
                        state,
                        WorkspaceChatPage(
                          workspaceName:
                              state.pathParameters['conversationId']!,
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
                path: '/friends',
                pageBuilder: (context, state) =>
                    _page(state, const FriendsPage()),
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/pet',
                pageBuilder: (context, state) => _page(state, const PetPage()),
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/me',
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
