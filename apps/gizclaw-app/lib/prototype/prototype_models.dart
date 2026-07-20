import 'package:flutter/cupertino.dart';
import 'package:gizclaw/gizclaw.dart';

import '../giz_ui/giz_ui.dart';

enum WorkflowDriverKind {
  flowcraft('flowcraft', 'Flowcraft'),
  doubaoRealtime('doubao-realtime', 'Doubao Realtime'),
  astTranslate('ast-translate', 'AST Translate'),
  chatroom('chatroom', 'Chatroom'),
  unsupported('unsupported', 'Unavailable');

  const WorkflowDriverKind(this.routeKey, this.label);

  final String routeKey;
  final String label;

  static WorkflowDriverKind fromRouteKey(String value) {
    return values.firstWhere(
      (driver) => driver.routeKey == value,
      orElse: () => unsupported,
    );
  }
}

class WorkflowCard {
  const WorkflowCard({
    required this.name,
    required this.title,
    required this.subtitle,
    required this.driverLabel,
    required this.category,
    required this.bannerColor,
    required this.icon,
    required this.driver,
    this.source = ResourceSource.RESOURCE_SOURCE_RUNTIME,
    this.imagePath,
  });

  final String name;
  final String title;
  final String subtitle;
  final String driverLabel;
  final String category;
  final Color bannerColor;
  final IconData icon;
  final WorkflowDriverKind driver;
  final ResourceSource source;
  final String? imagePath;

  factory WorkflowCard.unknown(
    String name, {
    ResourceSource source = ResourceSource.RESOURCE_SOURCE_RUNTIME,
  }) => WorkflowCard(
    name: name,
    title: name,
    subtitle: 'Workflow is not supported by this app version.',
    driverLabel: 'Unavailable',
    category: 'Other',
    bannerColor: GizColors.secondaryInk,
    icon: GizIcons.question_circle,
    driver: WorkflowDriverKind.unsupported,
    source: source,
  );
}

class WorkspaceCard {
  const WorkspaceCard({
    required this.name,
    required this.workflowName,
    required this.lastActive,
    this.workflowSource = ResourceSource.RESOURCE_SOURCE_RUNTIME,
    this.chatroomKind,
  });

  final ChatroomWorkspaceKind? chatroomKind;
  final String name;
  final String workflowName;
  final ResourceSource workflowSource;
  final String lastActive;

  String get title => name;
}

class ChatroomCard {
  const ChatroomCard({
    required this.id,
    required this.name,
    required this.subtitle,
    required this.memberCount,
  });

  final String id;
  final String name;
  final String subtitle;
  final int memberCount;
}

enum ChatroomWorkspaceKind { direct, group }

class ChatroomWorkspaceMetadata {
  const ChatroomWorkspaceMetadata({
    required this.workspaceName,
    required this.title,
    required this.kind,
    this.description = '',
    this.resourceId = '',
    this.emoji = '',
  });

  final String description;
  final String emoji;
  final ChatroomWorkspaceKind kind;
  final String resourceId;
  final String title;
  final String workspaceName;
}
