import 'prototype_models.dart';

const recentWorkspaces = [
  WorkspaceCard(
    name: 'Morning check-in',
    workflowName: 'chatroom',
    lastActive: '12 min ago',
    chatroomKind: ChatroomWorkspaceKind.direct,
  ),
  WorkspaceCard(
    name: 'Mobile app plan',
    workflowName: 'chat',
    lastActive: 'Yesterday',
  ),
];

const workflowWorkspaces = [
  ...recentWorkspaces,
  WorkspaceCard(
    name: 'Builder crew room',
    workflowName: 'chatroom',
    lastActive: 'Today',
    chatroomKind: ChatroomWorkspaceKind.group,
  ),
  WorkspaceCard(
    name: 'Hands-free test',
    workflowName: 'doubao-realtime',
    lastActive: '2 days ago',
  ),
  WorkspaceCard(
    name: 'Parser pass',
    workflowName: 'translate-zh-en-auto',
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
