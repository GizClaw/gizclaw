import 'package:flutter/cupertino.dart';
import 'package:go_router/go_router.dart';

import '../app/app_shell.dart';
import '../app/global_conversation_control.dart';
import '../data/mobile_data_controller.dart';
import '../features/active/active_workspace_page.dart';
import '../features/chats/chat_pages.dart';
import '../features/identity/server_pages.dart';
import '../features/onboarding/server_onboarding_page.dart';
import '../features/pet/pet_page.dart';
import '../features/social/social_pages.dart';
import '../giz_ui/giz_ui.dart';
import '../l10n/l10n.dart';
import '../workflows/app_workflow_catalog.dart';

GoRouter createAppRouter({required MobileDataController dataController}) {
  final rootNavigatorKey = GlobalKey<NavigatorState>();
  return GoRouter(
    navigatorKey: rootNavigatorKey,
    initialLocation: dataController.hasActiveServer ? '/active' : '/setup',
    refreshListenable: dataController,
    redirect: (context, state) {
      final path = state.uri.path;
      final inSetup = path == '/setup' || path.startsWith('/setup/');
      if (!dataController.hasActiveServer && !inSetup) {
        return '/setup';
      }
      if (dataController.hasActiveServer && inSetup) {
        return '/identity';
      }
      return null;
    },
    routes: [
      GoRoute(path: '/', redirect: (_, _) => '/active'),
      GoRoute(
        path: '/setup',
        pageBuilder: (context, state) =>
            _page(state, const ServerOnboardingPage()),
        routes: [
          GoRoute(
            path: 'servers',
            parentNavigatorKey: rootNavigatorKey,
            pageBuilder: (context, state) => _page(
              state,
              const ServerListPage(addServerRoute: '/setup/servers/new'),
            ),
            routes: [
              GoRoute(
                path: 'new',
                parentNavigatorKey: rootNavigatorKey,
                pageBuilder: (context, state) => _page(
                  state,
                  const AddServerPage(scanServerRoute: '/setup/servers/scan'),
                ),
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
      GoRoute(
        path: '/workspaces',
        redirect: (_, state) =>
            state.uri.path == '/workspaces' ? '/collections/assistants' : null,
        routes: [
          GoRoute(
            path: ':workspaceName',
            parentNavigatorKey: rootNavigatorKey,
            pageBuilder: (context, state) {
              final workspaceName = state.pathParameters['workspaceName']!;
              return _page(
                state,
                GlobalConversationOverlay(
                  location: state.uri,
                  child: ChatroomWorkspacePage(
                    workspaceName: workspaceName,
                    removedFallbackPath: '/friends',
                  ),
                ),
              );
            },
          ),
        ],
      ),
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
          for (final collection in appWorkflowCollections)
            StatefulShellBranch(
              initialLocation: '/collections/${collection.id}',
              routes: [
                GoRoute(
                  path: '/collections/${collection.id}',
                  pageBuilder: (context, state) => _page(
                    state,
                    CollectionWorkspacesPage(collection: collection.id),
                  ),
                  routes: [
                    GoRoute(
                      path: 'new',
                      parentNavigatorKey: rootNavigatorKey,
                      pageBuilder: (context, state) => _page(
                        state,
                        WorkflowPickerPage(collection: collection.id),
                      ),
                    ),
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
                            child: WorkspaceChatPage(
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
                            removedFallbackPath: '/groups',
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
                    path: 'scan',
                    parentNavigatorKey: rootNavigatorKey,
                    pageBuilder: (context, state) =>
                        _page(state, const ScanServerQrPage()),
                  ),
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
        navigationBar: CupertinoNavigationBar(
          middle: Text(context.l10n.uiText(key: 'notFound')),
          border: null,
        ),
        child: Center(
          child: Text(
            context.l10n.uiText(key: 'pageUnavailable'),
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
