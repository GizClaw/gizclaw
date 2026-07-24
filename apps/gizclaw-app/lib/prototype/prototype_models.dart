import 'package:flutter/cupertino.dart';

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
    required this.collection,
    required this.bannerColor,
    required this.icon,
    required this.driver,
    this.imagePath,
    this.workspaceLangPair,
  });

  final String name;
  final String title;
  final String subtitle;
  final String driverLabel;
  final String collection;
  final Color bannerColor;
  final IconData icon;
  final WorkflowDriverKind driver;
  final String? imagePath;
  final String? workspaceLangPair;

  factory WorkflowCard.unknown(String name, {required String collection}) =>
      WorkflowCard(
        name: name,
        title: name,
        subtitle: 'Workflow is not supported by this app version.',
        driverLabel: 'Unavailable',
        collection: collection,
        bannerColor: GizColors.secondaryInk,
        icon: GizIcons.question_circle,
        driver: WorkflowDriverKind.unsupported,
      );
}

class WorkspaceCard {
  const WorkspaceCard({
    required this.name,
    required this.workflowAlias,
    required this.collection,
    required this.lastActive,
    this.chatroomKind,
  });

  final ChatroomWorkspaceKind? chatroomKind;
  final String collection;
  final String name;
  final String workflowAlias;
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
    this.peerPublicKey = '',
    this.isGroupOwner = false,
  });

  final String description;
  final String emoji;
  final bool isGroupOwner;
  final ChatroomWorkspaceKind kind;
  final String peerPublicKey;
  final String resourceId;
  final String title;
  final String workspaceName;
}
