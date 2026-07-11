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

class WorkflowCard {
  const WorkflowCard({
    required this.name,
    required this.title,
    required this.subtitle,
    required this.driverLabel,
    required this.category,
    required this.bannerColor,
    required this.icon,
    this.imagePath,
  });

  final String name;
  final String title;
  final String subtitle;
  final String driverLabel;
  final String category;
  final Color bannerColor;
  final IconData icon;
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
      );
    }
    return WorkflowCard(
      name: name,
      title: _displayName(name),
      subtitle: description,
      driverLabel: 'Chatroom',
      category: 'Conversation',
      bannerColor: const Color(0xFF1F7A68),
      icon: CupertinoIcons.waveform,
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
  });

  final String name;
  final String workflowName;
  final String lastActive;
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
