import 'package:flutter/material.dart';

void main() {
  runApp(const GizClawApp());
}

class GizClawApp extends StatelessWidget {
  const GizClawApp({super.key});

  @override
  Widget build(BuildContext context) {
    const seed = Color(0xFF1F7A68);
    return MaterialApp(
      title: 'GizClaw',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: seed,
          brightness: Brightness.light,
        ),
        scaffoldBackgroundColor: const Color(0xFFF7F7F2),
        useMaterial3: true,
      ),
      home: const HomeShell(),
    );
  }
}

class HomeShell extends StatefulWidget {
  const HomeShell({super.key});

  @override
  State<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends State<HomeShell> {
  int _selectedIndex = 0;

  @override
  Widget build(BuildContext context) {
    final pages = <Widget>[
      BrowsePage(onOpenWorkflow: _openWorkflow),
      const ChatsPage(),
      const FriendsPage(),
      const PetPage(),
      const MePage(),
    ];

    return Scaffold(
      body: SafeArea(child: pages[_selectedIndex]),
      bottomNavigationBar: NavigationBar(
        selectedIndex: _selectedIndex,
        onDestinationSelected: (value) {
          setState(() => _selectedIndex = value);
        },
        destinations: const [
          NavigationDestination(
            icon: Icon(Icons.explore_outlined),
            selectedIcon: Icon(Icons.explore),
            label: 'Browse',
          ),
          NavigationDestination(
            icon: Icon(Icons.forum_outlined),
            selectedIcon: Icon(Icons.forum),
            label: 'Chats',
          ),
          NavigationDestination(
            icon: Icon(Icons.people_outline),
            selectedIcon: Icon(Icons.people),
            label: 'Friends',
          ),
          NavigationDestination(
            icon: Icon(Icons.pets_outlined),
            selectedIcon: Icon(Icons.pets),
            label: 'Pet',
          ),
          NavigationDestination(
            icon: Icon(Icons.person_outline),
            selectedIcon: Icon(Icons.person),
            label: 'Me',
          ),
        ],
      ),
    );
  }

  void _openWorkflow(WorkflowCard workflow) {
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => WorkflowDetailPage(workflow: workflow),
      ),
    );
  }
}

class BrowsePage extends StatelessWidget {
  const BrowsePage({super.key, required this.onOpenWorkflow});

  final ValueChanged<WorkflowCard> onOpenWorkflow;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return CustomScrollView(
      slivers: [
        SliverPadding(
          padding: const EdgeInsets.fromLTRB(20, 8, 20, 12),
          sliver: SliverToBoxAdapter(
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    'GizClaw',
                    style: theme.textTheme.headlineMedium?.copyWith(
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ),
                FilledButton.tonalIcon(
                  onPressed: () {},
                  icon: const Icon(Icons.wifi_tethering),
                  label: const Text('Online'),
                ),
              ],
            ),
          ),
        ),
        SliverToBoxAdapter(
          child: SizedBox(
            height: 188,
            child: ListView.separated(
              padding: const EdgeInsets.symmetric(horizontal: 20),
              scrollDirection: Axis.horizontal,
              itemBuilder: (context, index) {
                final workflow = featuredWorkflows[index];
                return FeaturedWorkflowCard(
                  workflow: workflow,
                  onTap: () => onOpenWorkflow(workflow),
                );
              },
              separatorBuilder: (_, _) => const SizedBox(width: 14),
              itemCount: featuredWorkflows.length,
            ),
          ),
        ),
        SliverPadding(
          padding: const EdgeInsets.fromLTRB(20, 24, 20, 8),
          sliver: SliverToBoxAdapter(
            child: Text(
              'Continue',
              style: theme.textTheme.titleLarge?.copyWith(
                fontWeight: FontWeight.w800,
              ),
            ),
          ),
        ),
        SliverPadding(
          padding: const EdgeInsets.symmetric(horizontal: 20),
          sliver: SliverToBoxAdapter(
            child: WorkspaceStrip(workspaces: recentWorkspaces),
          ),
        ),
        SliverPadding(
          padding: const EdgeInsets.fromLTRB(20, 24, 20, 8),
          sliver: SliverToBoxAdapter(
            child: Text(
              'All Workflows',
              style: theme.textTheme.titleLarge?.copyWith(
                fontWeight: FontWeight.w800,
              ),
            ),
          ),
        ),
        SliverPadding(
          padding: const EdgeInsets.fromLTRB(20, 0, 20, 24),
          sliver: SliverList.separated(
            itemBuilder: (context, index) {
              final workflow = allWorkflows[index];
              return WorkflowListTile(
                workflow: workflow,
                onTap: () => onOpenWorkflow(workflow),
              );
            },
            separatorBuilder: (_, _) => const SizedBox(height: 10),
            itemCount: allWorkflows.length,
          ),
        ),
      ],
    );
  }
}

class FeaturedWorkflowCard extends StatelessWidget {
  const FeaturedWorkflowCard({
    super.key,
    required this.workflow,
    required this.onTap,
  });

  final WorkflowCard workflow;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 300,
      child: Material(
        color: workflow.bannerColor,
        borderRadius: BorderRadius.circular(8),
        clipBehavior: Clip.antiAlias,
        child: InkWell(
          onTap: onTap,
          child: Stack(
            children: [
              Positioned(
                right: 18,
                top: 18,
                child: Icon(
                  workflow.icon,
                  size: 92,
                  color: Colors.white.withValues(alpha: 0.24),
                ),
              ),
              Padding(
                padding: const EdgeInsets.all(18),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Chip(
                      label: Text(workflow.driverLabel),
                      visualDensity: VisualDensity.compact,
                      backgroundColor: Colors.white.withValues(alpha: 0.86),
                      side: BorderSide.none,
                    ),
                    const Spacer(),
                    Text(
                      workflow.title,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.headlineSmall
                          ?.copyWith(
                            color: Colors.white,
                            fontWeight: FontWeight.w800,
                          ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      workflow.subtitle,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                        color: Colors.white.withValues(alpha: 0.88),
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class WorkspaceStrip extends StatelessWidget {
  const WorkspaceStrip({super.key, required this.workspaces});

  final List<WorkspaceCard> workspaces;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 96,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        itemBuilder: (context, index) {
          final workspace = workspaces[index];
          return SizedBox(
            width: 218,
            child: Card(
              margin: EdgeInsets.zero,
              elevation: 0,
              color: Colors.white,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(8),
              ),
              child: Padding(
                padding: const EdgeInsets.all(14),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      workspace.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.titleMedium?.copyWith(
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 6),
                    Text(
                      workspace.workflowName,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                    const Spacer(),
                    Text(
                      workspace.lastActive,
                      style: Theme.of(context).textTheme.labelMedium,
                    ),
                  ],
                ),
              ),
            ),
          );
        },
        separatorBuilder: (_, _) => const SizedBox(width: 10),
        itemCount: workspaces.length,
      ),
    );
  }
}

class WorkflowListTile extends StatelessWidget {
  const WorkflowListTile({
    super.key,
    required this.workflow,
    required this.onTap,
  });

  final WorkflowCard workflow;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: EdgeInsets.zero,
      elevation: 0,
      color: Colors.white,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      child: ListTile(
        onTap: onTap,
        leading: Container(
          width: 48,
          height: 48,
          decoration: BoxDecoration(
            color: workflow.bannerColor.withValues(alpha: 0.14),
            borderRadius: BorderRadius.circular(8),
          ),
          child: Icon(workflow.icon, color: workflow.bannerColor),
        ),
        title: Text(
          workflow.title,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: const TextStyle(fontWeight: FontWeight.w700),
        ),
        subtitle: Text(
          workflow.subtitle,
          maxLines: 2,
          overflow: TextOverflow.ellipsis,
        ),
        trailing: const Icon(Icons.chevron_right),
      ),
    );
  }
}

class WorkflowDetailPage extends StatelessWidget {
  const WorkflowDetailPage({super.key, required this.workflow});

  final WorkflowCard workflow;

  @override
  Widget build(BuildContext context) {
    final workspaces = workflowWorkspaces
        .where((workspace) => workspace.workflowName == workflow.name)
        .toList();

    return Scaffold(
      body: CustomScrollView(
        slivers: [
          SliverAppBar.large(
            pinned: true,
            expandedHeight: 220,
            title: Text(workflow.title),
            backgroundColor: workflow.bannerColor,
            foregroundColor: Colors.white,
            flexibleSpace: FlexibleSpaceBar(
              background: ColoredBox(
                color: workflow.bannerColor,
                child: Stack(
                  children: [
                    Positioned(
                      right: 28,
                      bottom: 22,
                      child: Icon(
                        workflow.icon,
                        size: 132,
                        color: Colors.white.withValues(alpha: 0.22),
                      ),
                    ),
                    Positioned(
                      left: 20,
                      right: 110,
                      bottom: 24,
                      child: Text(
                        workflow.subtitle,
                        maxLines: 3,
                        overflow: TextOverflow.ellipsis,
                        style: Theme.of(context).textTheme.titleMedium
                            ?.copyWith(
                              color: Colors.white.withValues(alpha: 0.9),
                            ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(20, 18, 20, 8),
            sliver: SliverToBoxAdapter(
              child: Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  Chip(label: Text(workflow.driverLabel)),
                  Chip(label: Text('${workspaces.length} workspaces')),
                  Chip(label: Text(workflow.category)),
                ],
              ),
            ),
          ),
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
            sliver: SliverToBoxAdapter(
              child: Text(
                'Workspaces',
                style: Theme.of(
                  context,
                ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w800),
              ),
            ),
          ),
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(20, 0, 20, 88),
            sliver: SliverList.separated(
              itemBuilder: (context, index) {
                final workspace = workspaces[index];
                return WorkspaceListTile(workspace: workspace);
              },
              separatorBuilder: (_, _) => const SizedBox(height: 10),
              itemCount: workspaces.length,
            ),
          ),
        ],
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: () {
          Navigator.of(context).push(
            MaterialPageRoute<void>(
              builder: (_) => ChatPage(
                workflow: workflow,
                workspace: workspaces.isNotEmpty
                    ? workspaces.first
                    : WorkspaceCard(
                        name: 'New workspace',
                        workflowName: workflow.name,
                        lastActive: 'Now',
                      ),
              ),
            ),
          );
        },
        icon: const Icon(Icons.play_arrow),
        label: const Text('Open'),
      ),
    );
  }
}

class WorkspaceListTile extends StatelessWidget {
  const WorkspaceListTile({super.key, required this.workspace});

  final WorkspaceCard workspace;

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: EdgeInsets.zero,
      elevation: 0,
      color: Colors.white,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      child: ListTile(
        leading: const Icon(Icons.workspaces_outline),
        title: Text(
          workspace.name,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: const TextStyle(fontWeight: FontWeight.w700),
        ),
        subtitle: Text(workspace.lastActive),
        trailing: const Icon(Icons.chat_bubble_outline),
        onTap: () {
          final workflow = allWorkflows.firstWhere(
            (item) => item.name == workspace.workflowName,
            orElse: () => allWorkflows.first,
          );
          Navigator.of(context).push(
            MaterialPageRoute<void>(
              builder: (_) =>
                  ChatPage(workflow: workflow, workspace: workspace),
            ),
          );
        },
      ),
    );
  }
}

class ChatPage extends StatelessWidget {
  const ChatPage({super.key, required this.workflow, required this.workspace});

  final WorkflowCard workflow;
  final WorkspaceCard workspace;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(workspace.name),
            Text(
              workflow.title,
              style: Theme.of(context).textTheme.labelMedium,
            ),
          ],
        ),
        actions: [
          IconButton(
            tooltip: 'Voice',
            onPressed: () {},
            icon: const Icon(Icons.mic_none),
          ),
        ],
      ),
      body: Column(
        children: [
          Expanded(
            child: ListView(
              padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
              children: const [
                ChatBubble(
                  text: 'Workspace loaded. What should we work on next?',
                  fromUser: false,
                ),
                ChatBubble(
                  text: 'Start from the last session and keep the same goal.',
                  fromUser: true,
                ),
                ChatBubble(
                  text: 'I found the recent context and opened the workflow.',
                  fromUser: false,
                ),
              ],
            ),
          ),
          SafeArea(
            top: false,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 12),
              child: Row(
                children: [
                  IconButton(
                    tooltip: 'Attach',
                    onPressed: () {},
                    icon: const Icon(Icons.add_circle_outline),
                  ),
                  Expanded(
                    child: TextField(
                      minLines: 1,
                      maxLines: 4,
                      decoration: InputDecoration(
                        hintText: 'Message',
                        filled: true,
                        fillColor: Colors.white,
                        border: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(8),
                          borderSide: BorderSide.none,
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(width: 8),
                  FilledButton(
                    onPressed: () {},
                    child: const Icon(Icons.arrow_upward),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class ChatBubble extends StatelessWidget {
  const ChatBubble({super.key, required this.text, required this.fromUser});

  final String text;
  final bool fromUser;

  @override
  Widget build(BuildContext context) {
    final color = fromUser
        ? Theme.of(context).colorScheme.primary
        : Theme.of(context).colorScheme.surfaceContainerHighest;
    final foreground = fromUser
        ? Theme.of(context).colorScheme.onPrimary
        : null;
    return Align(
      alignment: fromUser ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        constraints: const BoxConstraints(maxWidth: 300),
        margin: const EdgeInsets.only(bottom: 10),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        decoration: BoxDecoration(
          color: color,
          borderRadius: BorderRadius.circular(8),
        ),
        child: Text(text, style: TextStyle(color: foreground)),
      ),
    );
  }
}

class ChatsPage extends StatelessWidget {
  const ChatsPage({super.key});

  @override
  Widget build(BuildContext context) {
    return DefaultTabController(
      length: 2,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 8, 20, 4),
            child: Text(
              'Chats',
              style: Theme.of(
                context,
              ).textTheme.headlineMedium?.copyWith(fontWeight: FontWeight.w800),
            ),
          ),
          const TabBar(
            tabs: [
              Tab(text: 'Workspace'),
              Tab(text: 'Group Chat'),
            ],
          ),
          const Expanded(
            child: TabBarView(children: [WorkspaceChats(), GroupChats()]),
          ),
        ],
      ),
    );
  }
}

class WorkspaceChats extends StatelessWidget {
  const WorkspaceChats({super.key});

  @override
  Widget build(BuildContext context) {
    return ListView.separated(
      padding: const EdgeInsets.all(20),
      itemCount: workflowWorkspaces.length,
      separatorBuilder: (_, _) => const SizedBox(height: 10),
      itemBuilder: (context, index) =>
          WorkspaceListTile(workspace: workflowWorkspaces[index]),
    );
  }
}

class GroupChats extends StatelessWidget {
  const GroupChats({super.key});

  @override
  Widget build(BuildContext context) {
    return ListView.separated(
      padding: const EdgeInsets.all(20),
      itemCount: chatrooms.length,
      separatorBuilder: (_, _) => const SizedBox(height: 10),
      itemBuilder: (context, index) {
        final room = chatrooms[index];
        return Card(
          margin: EdgeInsets.zero,
          elevation: 0,
          color: Colors.white,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
          child: ListTile(
            leading: const Icon(Icons.groups_outlined),
            title: Text(
              room.name,
              style: const TextStyle(fontWeight: FontWeight.w700),
            ),
            subtitle: Text(room.subtitle),
            trailing: const Icon(Icons.chevron_right),
          ),
        );
      },
    );
  }
}

class FriendsPage extends StatelessWidget {
  const FriendsPage({super.key});

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(20),
      children: [
        Row(
          children: [
            Expanded(
              child: Text(
                'Friends',
                style: Theme.of(context).textTheme.headlineMedium?.copyWith(
                  fontWeight: FontWeight.w800,
                ),
              ),
            ),
            IconButton(
              tooltip: 'Add friend',
              onPressed: () {},
              icon: const Icon(Icons.person_add_alt_1_outlined),
            ),
          ],
        ),
        const SizedBox(height: 16),
        ...friends.map(
          (friend) => Card(
            elevation: 0,
            color: Colors.white,
            margin: const EdgeInsets.only(bottom: 10),
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(8),
            ),
            child: ListTile(
              leading: Stack(
                clipBehavior: Clip.none,
                children: [
                  CircleAvatar(child: Text(friend.name.substring(0, 1))),
                  Positioned(
                    right: -1,
                    bottom: -1,
                    child: Container(
                      width: 12,
                      height: 12,
                      decoration: BoxDecoration(
                        color: friend.online
                            ? const Color(0xFF2E9B65)
                            : const Color(0xFF9AA0A6),
                        shape: BoxShape.circle,
                        border: Border.all(color: Colors.white, width: 2),
                      ),
                    ),
                  ),
                ],
              ),
              title: Text(
                friend.name,
                style: const TextStyle(fontWeight: FontWeight.w700),
              ),
              subtitle: Text(friend.status),
              trailing: IconButton(
                tooltip: 'Message ${friend.name}',
                onPressed: () {},
                icon: const Icon(Icons.chat_bubble_outline),
              ),
            ),
          ),
        ),
      ],
    );
  }
}

class PetPage extends StatelessWidget {
  const PetPage({super.key});

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(20),
      children: [
        Text(
          'Pet',
          style: Theme.of(
            context,
          ).textTheme.headlineMedium?.copyWith(fontWeight: FontWeight.w800),
        ),
        const SizedBox(height: 16),
        Card(
          elevation: 0,
          color: Colors.white,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
          child: Padding(
            padding: const EdgeInsets.all(18),
            child: Row(
              children: [
                Container(
                  width: 72,
                  height: 72,
                  decoration: BoxDecoration(
                    color: const Color(0xFFFFD166).withValues(alpha: 0.24),
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: const Icon(Icons.pets, size: 36),
                ),
                const SizedBox(width: 16),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'Miso',
                        style: Theme.of(context).textTheme.titleLarge?.copyWith(
                          fontWeight: FontWeight.w800,
                        ),
                      ),
                      const SizedBox(height: 6),
                      const Text('Level 7'),
                      const SizedBox(height: 10),
                      const LinearProgressIndicator(value: 0.62),
                    ],
                  ),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }
}

class MePage extends StatelessWidget {
  const MePage({super.key});

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(20),
      children: [
        Text(
          'Me',
          style: Theme.of(
            context,
          ).textTheme.headlineMedium?.copyWith(fontWeight: FontWeight.w800),
        ),
        const SizedBox(height: 16),
        const SettingsRow(
          icon: Icons.key_outlined,
          title: 'Identity',
          value: 'client-local',
        ),
        const SettingsRow(
          icon: Icons.dns_outlined,
          title: 'Server',
          value: '127.0.0.1:9820',
        ),
        const SettingsRow(
          icon: Icons.security_outlined,
          title: 'Connection',
          value: 'WebRTC',
        ),
      ],
    );
  }
}

class SettingsRow extends StatelessWidget {
  const SettingsRow({
    super.key,
    required this.icon,
    required this.title,
    required this.value,
  });

  final IconData icon;
  final String title;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Card(
      elevation: 0,
      color: Colors.white,
      margin: const EdgeInsets.only(bottom: 10),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      child: ListTile(
        leading: Icon(icon),
        title: Text(title),
        subtitle: Text(value),
        trailing: const Icon(Icons.chevron_right),
      ),
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
  });

  final String name;
  final String title;
  final String subtitle;
  final String driverLabel;
  final String category;
  final Color bannerColor;
  final IconData icon;
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
  const ChatroomCard({required this.name, required this.subtitle});

  final String name;
  final String subtitle;
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

const featuredWorkflows = [
  WorkflowCard(
    name: 'chatroom-daily',
    title: 'Daily Companion',
    subtitle: 'Voice and text sessions for everyday planning.',
    driverLabel: 'Chatroom',
    category: 'Featured',
    bannerColor: Color(0xFF1F7A68),
    icon: Icons.record_voice_over,
  ),
  WorkflowCard(
    name: 'flowcraft-studio',
    title: 'Flowcraft Studio',
    subtitle: 'Build structured work from reusable workflows.',
    driverLabel: 'Flowcraft',
    category: 'Productivity',
    bannerColor: Color(0xFF4B6B8A),
    icon: Icons.account_tree_outlined,
  ),
  WorkflowCard(
    name: 'realtime-lab',
    title: 'Realtime Lab',
    subtitle: 'Low-latency audio agent sessions.',
    driverLabel: 'Doubao Realtime',
    category: 'Audio',
    bannerColor: Color(0xFF9A5A36),
    icon: Icons.graphic_eq,
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
    bannerColor: Color(0xFF7A4D7D),
    icon: Icons.code,
  ),
];

const recentWorkspaces = [
  WorkspaceCard(
    name: 'Morning check-in',
    workflowName: 'chatroom-daily',
    lastActive: '12 min ago',
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
  ChatroomCard(name: 'Home Room', subtitle: '3 recent voice messages'),
  ChatroomCard(name: 'Builder Crew', subtitle: 'Last active today'),
  ChatroomCard(name: 'Game Night', subtitle: 'Invite token available'),
];

const friends = [
  FriendCard(name: 'Avery', status: 'Building in Flowcraft', online: true),
  FriendCard(name: 'Morgan', status: 'Online', online: true),
  FriendCard(name: 'Rin', status: 'Last active yesterday', online: false),
];
