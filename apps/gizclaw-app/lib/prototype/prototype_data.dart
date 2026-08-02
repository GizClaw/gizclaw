import 'package:gizclaw/gizclaw.dart';

import 'prototype_models.dart';

final demoWorkflows = <Workflow>[
  Workflow(
    name: 'doubao-realtime',
    collection: 'assistants',
    driver: WorkflowDriver.WORKFLOW_DRIVER_DOUBAO_REALTIME,
    i18n: {
      'en': ResourceI18nText(
        displayName: 'Doubao',
        description: 'Realtime assistant',
      ),
      'zh-CN': ResourceI18nText(displayName: '豆包', description: '实时智能助手'),
    }.entries,
  ),
  Workflow(
    name: 'translate-zh-en-auto',
    collection: 'translates',
    driver: WorkflowDriver.WORKFLOW_DRIVER_AST_TRANSLATE,
    i18n: {
      'en': ResourceI18nText(displayName: 'Chinese / English'),
      'zh-CN': ResourceI18nText(displayName: '中英翻译'),
    }.entries,
  ),
  Workflow(
    name: 'journey',
    collection: 'raids',
    driver: WorkflowDriver.WORKFLOW_DRIVER_FLOWCRAFT,
    i18n: {
      'en': ResourceI18nText(displayName: 'Journey'),
      'zh-CN': ResourceI18nText(displayName: '赛博佩特'),
    }.entries,
  ),
];

const recentWorkspaces = [
  WorkspaceCard(
    name: 'Morning check-in',
    workflowName: 'chatroom',
    collection: 'assistants',
    lastActive: '12 min ago',
    chatroomKind: ChatroomWorkspaceKind.direct,
  ),
  WorkspaceCard(
    name: 'Mobile app plan',
    workflowName: 'journey',
    collection: 'raids',
    lastActive: 'Yesterday',
  ),
];

const workflowWorkspaces = [
  ...recentWorkspaces,
  WorkspaceCard(
    name: 'Builder crew room',
    workflowName: 'chatroom',
    collection: 'assistants',
    lastActive: 'Today',
    chatroomKind: ChatroomWorkspaceKind.group,
  ),
  WorkspaceCard(
    name: 'Hands-free test',
    workflowName: 'doubao-realtime',
    collection: 'assistants',
    lastActive: '2 days ago',
  ),
  WorkspaceCard(
    name: 'Parser pass',
    workflowName: 'translate-zh-en-auto',
    collection: 'translates',
    lastActive: 'Last week',
  ),
];

const chatrooms = [
  ChatroomCard(
    id: 'home-room',
    name: 'Home Room',
    subtitle: '3 recent voice messages',
    memberCount: 4,
  ),
  ChatroomCard(
    id: 'builder-crew',
    name: 'Builder Crew',
    subtitle: 'Last active today',
    memberCount: 7,
  ),
  ChatroomCard(
    id: 'game-night',
    name: 'Game Night',
    subtitle: 'Invite token available',
    memberCount: 5,
  ),
];

const chatroomWorkspaceMetadata = [
  ChatroomWorkspaceMetadata(
    workspaceName: 'Morning check-in',
    title: 'Avery',
    kind: ChatroomWorkspaceKind.direct,
  ),
  ChatroomWorkspaceMetadata(
    workspaceName: 'Builder crew room',
    title: 'Builder Crew',
    description: 'Shipping the mobile client',
    kind: ChatroomWorkspaceKind.group,
  ),
];

WorkspaceCard workspaceByName(String name) {
  return workflowWorkspaces.firstWhere(
    (workspace) => workspace.name == name,
    orElse: () => workflowWorkspaces.first,
  );
}

ChatroomCard chatroomById(String id) {
  return chatrooms.firstWhere(
    (room) => room.id == id,
    orElse: () => chatrooms.first,
  );
}
