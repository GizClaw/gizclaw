import 'package:flutter/material.dart';
import 'package:flutter_animate/flutter_animate.dart';

void main() {
  runApp(const GizClawApp());
}

class GizClawApp extends StatelessWidget {
  const GizClawApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'GizClaw',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF00A98F),
          brightness: Brightness.light,
          surface: const Color(0xFFF4F5F1),
        ),
        scaffoldBackgroundColor: const Color(0xFFF4F5F1),
        fontFamily: 'Manrope',
        textTheme: const TextTheme(
          displaySmall: TextStyle(
            fontSize: 38,
            height: 1.02,
            fontWeight: FontWeight.w800,
            letterSpacing: 0,
          ),
          headlineMedium: TextStyle(
            fontSize: 29,
            height: 1.08,
            fontWeight: FontWeight.w800,
            letterSpacing: 0,
          ),
          titleLarge: TextStyle(
            fontSize: 21,
            height: 1.2,
            fontWeight: FontWeight.w800,
            letterSpacing: 0,
          ),
          titleMedium: TextStyle(
            fontSize: 16,
            height: 1.3,
            fontWeight: FontWeight.w700,
            letterSpacing: 0,
          ),
          bodyLarge: TextStyle(fontSize: 16, height: 1.45, letterSpacing: 0),
          bodyMedium: TextStyle(fontSize: 14, height: 1.45, letterSpacing: 0),
          labelMedium: TextStyle(
            fontSize: 12,
            height: 1.2,
            fontWeight: FontWeight.w700,
            letterSpacing: 0,
          ),
        ),
        cardTheme: const CardThemeData(
          elevation: 0,
          margin: EdgeInsets.zero,
          color: Colors.white,
        ),
        pageTransitionsTheme: const PageTransitionsTheme(
          builders: {
            TargetPlatform.iOS: CupertinoPageTransitionsBuilder(),
            TargetPlatform.android: FadeForwardsPageTransitionsBuilder(),
          },
        ),
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
      extendBody: true,
      body: SafeArea(
        bottom: false,
        child: AnimatedSwitcher(
          duration: 240.ms,
          switchInCurve: Curves.easeOutCubic,
          switchOutCurve: Curves.easeInCubic,
          transitionBuilder: (child, animation) => FadeTransition(
            opacity: animation,
            child: SlideTransition(
              position: Tween<Offset>(
                begin: const Offset(0, 0.015),
                end: Offset.zero,
              ).animate(animation),
              child: child,
            ),
          ),
          child: KeyedSubtree(
            key: ValueKey(_selectedIndex),
            child: pages[_selectedIndex],
          ),
        ),
      ),
      bottomNavigationBar: GizDock(
        selectedIndex: _selectedIndex,
        onSelected: (value) => setState(() => _selectedIndex = value),
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

class GizDock extends StatelessWidget {
  const GizDock({
    super.key,
    required this.selectedIndex,
    required this.onSelected,
  });

  final int selectedIndex;
  final ValueChanged<int> onSelected;

  static const _items = [
    (Icons.explore_outlined, Icons.explore, 'Browse'),
    (Icons.forum_outlined, Icons.forum, 'Chats'),
    (Icons.people_outline, Icons.people, 'Friends'),
    (Icons.pets_outlined, Icons.pets, 'Pet'),
    (Icons.person_outline, Icons.person, 'Me'),
  ];

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      minimum: const EdgeInsets.fromLTRB(12, 0, 12, 10),
      child: Container(
        height: 70,
        padding: const EdgeInsets.symmetric(horizontal: 6),
        decoration: BoxDecoration(
          color: const Color(0xFF111916).withValues(alpha: 0.96),
          borderRadius: BorderRadius.circular(24),
          boxShadow: const [
            BoxShadow(
              color: Color(0x26000000),
              blurRadius: 30,
              offset: Offset(0, 12),
            ),
          ],
        ),
        child: Row(
          children: List.generate(_items.length, (index) {
            final item = _items[index];
            final selected = selectedIndex == index;
            return Expanded(
              child: Semantics(
                selected: selected,
                button: true,
                label: item.$3,
                child: InkResponse(
                  radius: 30,
                  onTap: () => onSelected(index),
                  child: Column(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      SizedBox(
                        width: 34,
                        height: 34,
                        child: Stack(
                          alignment: Alignment.center,
                          children: [
                            AnimatedScale(
                              duration: 260.ms,
                              curve: Curves.easeOutBack,
                              scale: selected ? 1 : 0,
                              child: Container(
                                width: 34,
                                height: 34,
                                decoration: const BoxDecoration(
                                  color: Color(0xFFB9F82E),
                                  shape: BoxShape.circle,
                                ),
                              ),
                            ),
                            AnimatedSwitcher(
                              duration: 180.ms,
                              child: Icon(
                                selected ? item.$2 : item.$1,
                                key: ValueKey(selected),
                                size: 21,
                                color: selected
                                    ? const Color(0xFF111916)
                                    : Colors.white70,
                              ),
                            ),
                          ],
                        ),
                      ),
                      const SizedBox(height: 2),
                      AnimatedDefaultTextStyle(
                        duration: 180.ms,
                        style: TextStyle(
                          color: selected
                              ? const Color(0xFFB9F82E)
                              : Colors.white54,
                          fontSize: 10,
                          fontWeight: selected
                              ? FontWeight.w700
                              : FontWeight.w500,
                        ),
                        child: Text(item.$3),
                      ),
                    ],
                  ),
                ),
              ),
            );
          }),
        ),
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
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 16),
          sliver: SliverToBoxAdapter(
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    'Play your\nworkflows',
                    style: theme.textTheme.displaySmall,
                  ),
                ),
                const SignalBadge(label: 'LIVE'),
              ],
            ),
          ),
        ),
        SliverToBoxAdapter(
          child: SizedBox(
            height: 260,
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
              separatorBuilder: (_, _) => const SizedBox(width: 12),
              itemCount: featuredWorkflows.length,
            ),
          ),
        ),
        SliverPadding(
          padding: const EdgeInsets.fromLTRB(20, 28, 20, 10),
          sliver: SliverToBoxAdapter(
            child: Text(
              'Jump back in',
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
          padding: const EdgeInsets.fromLTRB(20, 28, 20, 10),
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
          padding: const EdgeInsets.fromLTRB(20, 0, 20, 110),
          sliver: SliverList.separated(
            itemBuilder: (context, index) {
              final workflow = allWorkflows[index];
              return WorkflowListTile(
                    workflow: workflow,
                    onTap: () => onOpenWorkflow(workflow),
                  )
                  .animate(delay: (index * 45).ms)
                  .fadeIn(duration: 320.ms)
                  .slideY(begin: 0.08, end: 0, curve: Curves.easeOutCubic);
            },
            separatorBuilder: (_, _) => const SizedBox.shrink(),
            itemCount: allWorkflows.length,
          ),
        ),
      ],
    );
  }
}

class SignalBadge extends StatelessWidget {
  const SignalBadge({super.key, required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(9, 7, 11, 7),
      decoration: BoxDecoration(
        color: const Color(0xFF111916),
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          const SignalPulse(size: 16),
          const SizedBox(width: 5),
          Text(
            label,
            style: const TextStyle(
              color: Colors.white,
              fontSize: 11,
              fontWeight: FontWeight.w800,
            ),
          ),
        ],
      ),
    );
  }
}

class SignalPulse extends StatefulWidget {
  const SignalPulse({super.key, this.size = 32});

  final double size;

  @override
  State<SignalPulse> createState() => _SignalPulseState();
}

class _SignalPulseState extends State<SignalPulse>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: 1800.ms,
  )..repeat();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return SizedBox.square(
      dimension: widget.size,
      child: AnimatedBuilder(
        animation: _controller,
        builder: (context, _) => Stack(
          alignment: Alignment.center,
          children: [
            for (final offset in [0.0, 0.42])
              _PulseRing(
                progress: (_controller.value + offset) % 1,
                size: widget.size,
              ),
            Container(
              width: widget.size * 0.28,
              height: widget.size * 0.28,
              decoration: const BoxDecoration(
                color: Color(0xFFB9F82E),
                shape: BoxShape.circle,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _PulseRing extends StatelessWidget {
  const _PulseRing({required this.progress, required this.size});

  final double progress;
  final double size;

  @override
  Widget build(BuildContext context) {
    return Transform.scale(
      scale: 0.35 + progress * 0.65,
      child: Opacity(
        opacity: (1 - progress) * 0.7,
        child: Container(
          width: size,
          height: size,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            border: Border.all(color: const Color(0xFFB9F82E)),
          ),
        ),
      ),
    );
  }
}

class WorkflowArtworkHero extends StatelessWidget {
  const WorkflowArtworkHero({
    super.key,
    required this.workflow,
    required this.compact,
  });

  final WorkflowCard workflow;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final imagePath = workflow.imagePath!;
    final radius = BorderRadius.circular(compact ? 18 : 0);
    return Hero(
      tag: 'workflow-${workflow.name}',
      transitionOnUserGestures: true,
      placeholderBuilder: (_, _, child) => child,
      flightShuttleBuilder:
          (flightContext, animation, direction, fromContext, toContext) {
            final pushing = direction == HeroFlightDirection.push;
            final begin = BorderRadius.circular(pushing ? 18 : 0);
            final end = BorderRadius.circular(pushing ? 0 : 18);
            return AnimatedBuilder(
              animation: animation,
              builder: (context, child) => ClipRRect(
                borderRadius: BorderRadius.lerp(begin, end, animation.value)!,
                child: child,
              ),
              child: Material(
                color: Colors.transparent,
                child: Image.asset(imagePath, fit: BoxFit.cover),
              ),
            );
          },
      child: ClipRRect(
        borderRadius: radius,
        child: Image.asset(imagePath, fit: BoxFit.cover),
      ),
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
      width: 330,
      child: Material(
        color: const Color(0xFF111916),
        borderRadius: BorderRadius.circular(18),
        clipBehavior: Clip.antiAlias,
        child: InkWell(
          onTap: onTap,
          child: Stack(
            children: [
              if (workflow.imagePath != null)
                Positioned.fill(
                  child: WorkflowArtworkHero(workflow: workflow, compact: true),
                ),
              Positioned.fill(
                child: DecoratedBox(
                  decoration: BoxDecoration(
                    gradient: LinearGradient(
                      begin: Alignment.topCenter,
                      end: Alignment.bottomCenter,
                      colors: [
                        Colors.transparent,
                        const Color(0xFF07100E).withValues(alpha: 0.12),
                        const Color(0xFF07100E).withValues(alpha: 0.9),
                      ],
                      stops: const [0, 0.4, 1],
                    ),
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.all(18),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 10,
                        vertical: 6,
                      ),
                      decoration: BoxDecoration(
                        color: Colors.white.withValues(alpha: 0.88),
                        borderRadius: BorderRadius.circular(16),
                      ),
                      child: Text(
                        workflow.driverLabel.toUpperCase(),
                        style: const TextStyle(
                          fontSize: 10,
                          fontWeight: FontWeight.w900,
                        ),
                      ),
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
                    const SizedBox(height: 6),
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
      height: 112,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        itemBuilder: (context, index) {
          final workspace = workspaces[index];
          final colors = [const Color(0xFFDDF7EE), const Color(0xFFFFE5DD)];
          return SizedBox(
            width: 230,
            child: DecoratedBox(
              decoration: BoxDecoration(
                color: colors[index % colors.length],
                borderRadius: BorderRadius.circular(16),
              ),
              child: Padding(
                padding: const EdgeInsets.all(16),
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
                      style: Theme.of(context).textTheme.labelMedium?.copyWith(
                        color: const Color(0xFF59615E),
                      ),
                    ),
                    const Spacer(),
                    Row(
                      children: [
                        const SignalPulse(size: 16),
                        const SizedBox(width: 6),
                        Text(
                          workspace.lastActive,
                          style: Theme.of(context).textTheme.labelMedium,
                        ),
                      ],
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
    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 14),
          decoration: const BoxDecoration(
            border: Border(bottom: BorderSide(color: Color(0xFFD8DDD6))),
          ),
          child: Row(
            children: [
              ClipRRect(
                borderRadius: BorderRadius.circular(12),
                child: SizedBox(
                  width: 66,
                  height: 66,
                  child: workflow.imagePath != null
                      ? Image.asset(workflow.imagePath!, fit: BoxFit.cover)
                      : ColoredBox(
                          color: workflow.bannerColor.withValues(alpha: 0.14),
                          child: Icon(
                            workflow.icon,
                            color: workflow.bannerColor,
                          ),
                        ),
                ),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      workflow.category.toUpperCase(),
                      style: Theme.of(context).textTheme.labelMedium?.copyWith(
                        color: workflow.bannerColor,
                        fontSize: 10,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      workflow.title,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.titleMedium,
                    ),
                    const SizedBox(height: 4),
                    Text(
                      workflow.subtitle,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                        color: const Color(0xFF5F6865),
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 10),
              const Icon(Icons.arrow_forward, size: 20),
            ],
          ),
        ),
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
              background: Stack(
                fit: StackFit.expand,
                children: [
                  if (workflow.imagePath != null)
                    WorkflowArtworkHero(workflow: workflow, compact: false)
                  else
                    ColoredBox(color: workflow.bannerColor),
                  DecoratedBox(
                    decoration: BoxDecoration(
                      gradient: LinearGradient(
                        begin: Alignment.topCenter,
                        end: Alignment.bottomCenter,
                        colors: [
                          Colors.transparent,
                          const Color(0xFF07100E).withValues(alpha: 0.88),
                        ],
                      ),
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
                      style: Theme.of(context).textTheme.titleMedium?.copyWith(
                        color: Colors.white.withValues(alpha: 0.9),
                      ),
                    ),
                  ),
                ],
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
                  WorkflowMetaTag(label: workflow.driverLabel),
                  WorkflowMetaTag(label: '${workspaces.length} workspaces'),
                  WorkflowMetaTag(label: workflow.category),
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
              separatorBuilder: (_, _) => const SizedBox.shrink(),
              itemCount: workspaces.length,
            ),
          ),
        ],
      ),
      floatingActionButton: FloatingActionButton.extended(
        backgroundColor: const Color(0xFF111916),
        foregroundColor: Colors.white,
        shape: const StadiumBorder(),
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

class WorkflowMetaTag extends StatelessWidget {
  const WorkflowMetaTag({super.key, required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      decoration: BoxDecoration(
        color: const Color(0xFFE4E9E2),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Text(
        label,
        style: Theme.of(
          context,
        ).textTheme.labelMedium?.copyWith(color: const Color(0xFF3F4945)),
      ),
    );
  }
}

class WorkspaceListTile extends StatelessWidget {
  const WorkspaceListTile({super.key, required this.workspace});

  final WorkspaceCard workspace;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.transparent,
      child: InkWell(
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
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 14),
          decoration: const BoxDecoration(
            border: Border(bottom: BorderSide(color: Color(0xFFD8DDD6))),
          ),
          child: Row(
            children: [
              Container(
                width: 48,
                height: 48,
                decoration: BoxDecoration(
                  color: const Color(0xFF111916),
                  borderRadius: BorderRadius.circular(14),
                ),
                child: const Center(child: SignalPulse(size: 28)),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      workspace.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.titleMedium,
                    ),
                    const SizedBox(height: 5),
                    Text(
                      '${workspace.workflowName}  ·  ${workspace.lastActive}',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                        color: const Color(0xFF65706C),
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 10),
              const Icon(Icons.arrow_forward, size: 20),
            ],
          ),
        ),
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

class ChatsPage extends StatefulWidget {
  const ChatsPage({super.key});

  @override
  State<ChatsPage> createState() => _ChatsPageState();
}

class _ChatsPageState extends State<ChatsPage> {
  int _selected = 0;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 12),
          child: Row(
            children: [
              Expanded(
                child: Text(
                  'Chats',
                  style: Theme.of(context).textTheme.headlineMedium,
                ),
              ),
              const SignalBadge(label: 'WEBRTC'),
            ],
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 20),
          child: GizSegmentedControl(
            selectedIndex: _selected,
            onSelected: (value) => setState(() => _selected = value),
          ),
        ),
        Expanded(
          child: AnimatedSwitcher(
            duration: 260.ms,
            switchInCurve: Curves.easeOutCubic,
            transitionBuilder: (child, animation) => FadeTransition(
              opacity: animation,
              child: ScaleTransition(
                scale: Tween(begin: 0.985, end: 1.0).animate(animation),
                child: child,
              ),
            ),
            child: _selected == 0
                ? const WorkspaceChats(key: ValueKey('workspace'))
                : const GroupChats(key: ValueKey('groups')),
          ),
        ),
      ],
    );
  }
}

class GizSegmentedControl extends StatelessWidget {
  const GizSegmentedControl({
    super.key,
    required this.selectedIndex,
    required this.onSelected,
  });

  final int selectedIndex;
  final ValueChanged<int> onSelected;

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 48,
      padding: const EdgeInsets.all(4),
      decoration: BoxDecoration(
        color: const Color(0xFFE6E9E3),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Stack(
        children: [
          AnimatedAlign(
            duration: 300.ms,
            curve: Curves.easeOutBack,
            alignment: selectedIndex == 0
                ? Alignment.centerLeft
                : Alignment.centerRight,
            child: FractionallySizedBox(
              widthFactor: 0.5,
              child: Container(
                decoration: BoxDecoration(
                  color: const Color(0xFF111916),
                  borderRadius: BorderRadius.circular(12),
                  boxShadow: const [
                    BoxShadow(
                      color: Color(0x24000000),
                      blurRadius: 12,
                      offset: Offset(0, 4),
                    ),
                  ],
                ),
              ),
            ),
          ),
          Row(
            children: [
              _SegmentButton(
                label: 'Workspace',
                selected: selectedIndex == 0,
                onTap: () => onSelected(0),
              ),
              _SegmentButton(
                label: 'Group Chat',
                selected: selectedIndex == 1,
                onTap: () => onSelected(1),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

class _SegmentButton extends StatelessWidget {
  const _SegmentButton({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Expanded(
      child: InkWell(
        borderRadius: BorderRadius.circular(12),
        onTap: onTap,
        child: Center(
          child: AnimatedDefaultTextStyle(
            duration: 180.ms,
            style: TextStyle(
              color: selected ? Colors.white : const Color(0xFF48514E),
              fontWeight: FontWeight.w800,
              fontSize: 13,
            ),
            child: Text(label),
          ),
        ),
      ),
    );
  }
}

class WorkspaceChats extends StatelessWidget {
  const WorkspaceChats({super.key});

  @override
  Widget build(BuildContext context) {
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(20, 18, 20, 110),
      itemCount: workflowWorkspaces.length,
      separatorBuilder: (_, _) => const SizedBox.shrink(),
      itemBuilder: (context, index) =>
          WorkspaceListTile(workspace: workflowWorkspaces[index])
              .animate(delay: (index * 55).ms)
              .fadeIn(duration: 300.ms)
              .slideY(begin: 0.08, end: 0, curve: Curves.easeOutCubic),
    );
  }
}

class GroupChats extends StatelessWidget {
  const GroupChats({super.key});

  @override
  Widget build(BuildContext context) {
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(20, 18, 20, 110),
      itemCount: chatrooms.length,
      separatorBuilder: (_, _) => const SizedBox.shrink(),
      itemBuilder: (context, index) {
        final room = chatrooms[index];
        return Material(
          color: Colors.transparent,
          child: InkWell(
            onTap: () {},
            child: Container(
              padding: const EdgeInsets.symmetric(vertical: 15),
              decoration: const BoxDecoration(
                border: Border(bottom: BorderSide(color: Color(0xFFD8DDD6))),
              ),
              child: Row(
                children: [
                  Container(
                    width: 48,
                    height: 48,
                    alignment: Alignment.center,
                    decoration: BoxDecoration(
                      color: index.isEven
                          ? const Color(0xFFFFDDD2)
                          : const Color(0xFFD8EBFF),
                      borderRadius: BorderRadius.circular(14),
                    ),
                    child: Text(
                      '${index + 1}'.padLeft(2, '0'),
                      style: Theme.of(context).textTheme.labelMedium?.copyWith(
                        color: const Color(0xFF111916),
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                  ),
                  const SizedBox(width: 14),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          room.name,
                          style: Theme.of(context).textTheme.titleMedium,
                        ),
                        const SizedBox(height: 5),
                        Text(
                          room.subtitle,
                          style: Theme.of(context).textTheme.bodyMedium
                              ?.copyWith(color: const Color(0xFF65706C)),
                        ),
                      ],
                    ),
                  ),
                  const Icon(Icons.arrow_forward, size: 20),
                ],
              ),
            ),
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
    imagePath: 'assets/workflows/daily-companion.png',
  ),
  WorkflowCard(
    name: 'flowcraft-studio',
    title: 'Flowcraft Studio',
    subtitle: 'Build structured work from reusable workflows.',
    driverLabel: 'Flowcraft',
    category: 'Productivity',
    bannerColor: Color(0xFF4B6B8A),
    icon: Icons.account_tree_outlined,
    imagePath: 'assets/workflows/flowcraft-studio.png',
  ),
  WorkflowCard(
    name: 'realtime-lab',
    title: 'Realtime Lab',
    subtitle: 'Low-latency audio agent sessions.',
    driverLabel: 'Doubao Realtime',
    category: 'Audio',
    bannerColor: Color(0xFF9A5A36),
    icon: Icons.graphic_eq,
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
