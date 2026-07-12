import 'package:flutter/cupertino.dart';

class WorkflowCollection {
  const WorkflowCollection({
    required this.id,
    required this.title,
    required this.subtitle,
    required this.label,
    required this.imagePath,
    required this.workflowNames,
  });

  final String id;
  final String title;
  final String subtitle;
  final String label;
  final String imagePath;
  final List<String> workflowNames;
}

enum WorkflowDriverKind {
  flowcraft('flowcraft', 'Flowcraft', 'assets/drivers/flowcraft.png'),
  doubaoRealtime(
    'doubao-realtime',
    'Doubao Realtime',
    'assets/drivers/doubao-realtime.png',
  ),
  astTranslate(
    'ast-translate',
    'AST Translate',
    'assets/drivers/ast-translate.png',
  ),
  chatroom('chatroom', 'Chatroom', 'assets/drivers/chatroom.png'),
  unsupported('unsupported', 'Unavailable', null);

  const WorkflowDriverKind(this.routeKey, this.label, this.imagePath);

  final String routeKey;
  final String label;
  final String? imagePath;

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
  final String? imagePath;

  factory WorkflowCard.fromServer({
    required String name,
    required String description,
    required String driver,
  }) {
    final normalized = driver.toLowerCase();
    if (normalized.contains('flowcraft')) {
      return WorkflowCard(
        name: name,
        title: _displayName(name),
        subtitle: description,
        driverLabel: 'Flowcraft',
        category: 'Productivity',
        bannerColor: const Color(0xFF416986),
        icon: CupertinoIcons.rectangle_3_offgrid,
        driver: WorkflowDriverKind.flowcraft,
      );
    }
    if (normalized.contains('doubao')) {
      return WorkflowCard(
        name: name,
        title: _displayName(name),
        subtitle: description,
        driverLabel: 'Doubao Realtime',
        category: 'Audio',
        bannerColor: const Color(0xFF9A5A36),
        icon: CupertinoIcons.waveform_path,
        driver: WorkflowDriverKind.doubaoRealtime,
      );
    }
    if (normalized.contains('ast')) {
      return WorkflowCard(
        name: name,
        title: _displayName(name),
        subtitle: description,
        driverLabel: 'AST Translate',
        category: 'Code',
        bannerColor: const Color(0xFF75517D),
        icon: CupertinoIcons.chevron_left_slash_chevron_right,
        driver: WorkflowDriverKind.astTranslate,
      );
    }
    if (normalized.contains('chatroom')) {
      return WorkflowCard(
        name: name,
        title: _displayName(name),
        subtitle: description,
        driverLabel: 'Chatroom',
        category: 'Conversation',
        bannerColor: const Color(0xFF1F7A68),
        icon: CupertinoIcons.waveform,
        driver: WorkflowDriverKind.chatroom,
      );
    }
    return WorkflowCard(
      name: name,
      title: _displayName(name),
      subtitle: description,
      driverLabel: 'Unavailable',
      category: 'Other',
      bannerColor: const Color(0xFF69736F),
      icon: CupertinoIcons.question_circle,
      driver: WorkflowDriverKind.unsupported,
    );
  }

  factory WorkflowCard.unknown(String name) => WorkflowCard.fromServer(
    name: name,
    description: 'Workflow data is not available yet.',
    driver: '',
  );
}

String _displayName(String value) {
  return value
      .split(RegExp('[-_]'))
      .where((part) => part.isNotEmpty)
      .map((part) => '${part[0].toUpperCase()}${part.substring(1)}')
      .join(' ');
}

class WorkspaceCard {
  const WorkspaceCard({
    required this.name,
    required this.workflowName,
    required this.lastActive,
    this.displayName,
    this.chatroomKind,
  });

  final ChatroomWorkspaceKind? chatroomKind;
  final String? displayName;
  final String name;
  final String workflowName;
  final String lastActive;

  String get title {
    final value = displayName?.trim();
    return value == null || value.isEmpty ? name : value;
  }
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
  });

  final String description;
  final ChatroomWorkspaceKind kind;
  final String title;
  final String workspaceName;
}

class FriendCard {
  const FriendCard({
    required this.name,
    required this.status,
    required this.online,
  });

  final String name;
  final String status;
  final bool online;
}
