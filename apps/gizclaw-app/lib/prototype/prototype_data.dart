import '../giz_ui/giz_ui.dart';
import 'prototype_models.dart';

const featuredWorkflows = [
  WorkflowCard(
    name: 'chatroom-daily',
    title: 'Daily Companion',
    subtitle: 'Voice and text sessions for everyday planning.',
    driverLabel: 'Chatroom',
    category: 'Featured',
    bannerColor: GizColors.accent,
    icon: GizIcons.waveform,
    driver: WorkflowDriverKind.chatroom,
    imagePath: 'assets/workflows/daily-companion.png',
  ),
  WorkflowCard(
    name: 'flowcraft-studio',
    title: 'Flowcraft Studio',
    subtitle: 'Build structured work from reusable workflows.',
    driverLabel: 'Flowcraft',
    category: 'Productivity',
    bannerColor: GizColors.blue,
    icon: GizIcons.rectangle_3_offgrid,
    driver: WorkflowDriverKind.flowcraft,
    imagePath: 'assets/workflows/flowcraft-studio.png',
  ),
  WorkflowCard(
    name: 'realtime-lab',
    title: 'Realtime Lab',
    subtitle: 'Low-latency audio agent sessions.',
    driverLabel: 'Doubao Realtime',
    category: 'Audio',
    bannerColor: GizColors.coral,
    icon: GizIcons.waveform_path,
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
    bannerColor: GizColors.lavender,
    icon: GizIcons.chevron_left_slash_chevron_right,
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

ChatroomCard chatroomById(String id) {
  return chatrooms.firstWhere(
    (room) => room.id == id,
    orElse: () => chatrooms.first,
  );
}
