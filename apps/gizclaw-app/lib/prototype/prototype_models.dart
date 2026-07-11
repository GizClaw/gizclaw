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
