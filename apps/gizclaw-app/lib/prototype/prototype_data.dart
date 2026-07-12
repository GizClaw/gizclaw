import 'package:flutter/cupertino.dart';

import 'prototype_models.dart';

const featuredCollections = [
  WorkflowCollection(
    id: 'everyday-companions',
    title: 'Everyday companions',
    subtitle: 'Agents made for daily rituals, planning, and conversation.',
    label: 'Curated collection',
    imagePath: 'assets/workflows/daily-companion.png',
    workflowNames: ['chatroom-daily', 'realtime-lab'],
  ),
  WorkflowCollection(
    id: 'build-something',
    title: 'Build something',
    subtitle: 'Structured workflows for turning ideas into working systems.',
    label: 'Editor pick',
    imagePath: 'assets/workflows/flowcraft-studio.png',
    workflowNames: ['flowcraft-studio', 'ast-translate'],
  ),
  WorkflowCollection(
    id: 'realtime-playground',
    title: 'Realtime playground',
    subtitle: 'Low-latency voice experiments and live agent sessions.',
    label: 'New this week',
    imagePath: 'assets/workflows/realtime-lab.png',
    workflowNames: ['realtime-lab', 'chatroom-daily'],
  ),
];

const featuredWorkflows = [
  WorkflowCard(
    name: 'chatroom-daily',
    title: 'Daily Companion',
    subtitle: 'Voice and text sessions for everyday planning.',
    driverLabel: 'Chatroom',
    category: 'Featured',
    bannerColor: Color(0xFF1F7A68),
    icon: CupertinoIcons.waveform,
    driver: WorkflowDriverKind.chatroom,
    imagePath: 'assets/workflows/daily-companion.png',
  ),
  WorkflowCard(
    name: 'flowcraft-studio',
    title: 'Flowcraft Studio',
    subtitle: 'Build structured work from reusable workflows.',
    driverLabel: 'Flowcraft',
    category: 'Productivity',
    bannerColor: Color(0xFF416986),
    icon: CupertinoIcons.rectangle_3_offgrid,
    driver: WorkflowDriverKind.flowcraft,
    imagePath: 'assets/workflows/flowcraft-studio.png',
  ),
  WorkflowCard(
    name: 'realtime-lab',
    title: 'Realtime Lab',
    subtitle: 'Low-latency audio agent sessions.',
    driverLabel: 'Doubao Realtime',
    category: 'Audio',
    bannerColor: Color(0xFF9A5A36),
    icon: CupertinoIcons.waveform_path,
    driver: WorkflowDriverKind.doubaoRealtime,
    imagePath: 'assets/workflows/realtime-lab.png',
  ),
];

const allWorkflows = [
  ...featuredWorkflows,
  WorkflowCard(
    name: 'ast-translate',
    title: 'AST Translate',
    subtitle: 'Translate code with workspace history and context.',
    driverLabel: 'AST',
    category: 'Code',
    bannerColor: Color(0xFF75517D),
    icon: CupertinoIcons.chevron_left_slash_chevron_right,
    driver: WorkflowDriverKind.astTranslate,
  ),
];

const recentWorkspaces = [
  WorkspaceCard(
    name: 'Morning check-in',
    workflowName: 'chatroom-daily',
    lastActive: '12 min ago',
    chatroomKind: ChatroomWorkspaceKind.direct,
  ),
  WorkspaceCard(
    name: 'Mobile app plan',
    workflowName: 'flowcraft-studio',
    lastActive: 'Yesterday',
  ),
];

const workflowWorkspaces = [
  ...recentWorkspaces,
  WorkspaceCard(
    name: 'Builder crew room',
    workflowName: 'chatroom-daily',
    lastActive: 'Today',
    chatroomKind: ChatroomWorkspaceKind.group,
  ),
  WorkspaceCard(
    name: 'Hands-free test',
    workflowName: 'realtime-lab',
    lastActive: '2 days ago',
  ),
  WorkspaceCard(
    name: 'Parser pass',
    workflowName: 'ast-translate',
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

WorkflowCard workflowByName(String name) {
  return allWorkflows.firstWhere(
    (workflow) => workflow.name == name,
    orElse: () => allWorkflows.first,
  );
}

WorkspaceCard workspaceByName(String name) {
  return workflowWorkspaces.firstWhere(
    (workspace) => workspace.name == name,
    orElse: () => workflowWorkspaces.first,
  );
}

WorkflowCollection collectionById(String id) {
  return featuredCollections.firstWhere(
    (collection) => collection.id == id,
    orElse: () => featuredCollections.first,
  );
}

ChatroomCard chatroomById(String id) {
  return chatrooms.firstWhere(
    (room) => room.id == id,
    orElse: () => chatrooms.first,
  );
}
