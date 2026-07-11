import 'package:flutter/cupertino.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:go_router/go_router.dart';

import '../../data/mobile_data_controller.dart';
import '../../giz_ui/giz_ui.dart';
import '../../prototype/prototype_data.dart';
import '../../prototype/prototype_models.dart';

class BrowsePage extends StatelessWidget {
  const BrowsePage({super.key});

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final visibleWorkflows = data.workflows.take(3).toList();
    final recent = data.workspaces.take(4).toList();
    return CupertinoPageScaffold(
      child: SafeArea(
        bottom: false,
        child: CustomScrollView(
          key: const PageStorageKey('browse-scroll'),
          slivers: [
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 16),
              sliver: SliverToBoxAdapter(
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.end,
                  children: [
                    const Expanded(
                      child: Text('Play your\nworkflows', style: GizText.hero),
                    ),
                    _LiveBadge(state: data.connectionState),
                  ],
                ),
              ),
            ),
            SliverToBoxAdapter(
              child: SizedBox(
                height: 258,
                child: ListView.separated(
                  padding: const EdgeInsets.symmetric(horizontal: 20),
                  scrollDirection: Axis.horizontal,
                  itemCount: featuredCollections.length,
                  separatorBuilder: (_, _) => const SizedBox(width: 12),
                  itemBuilder: (context, index) {
                    final collection = featuredCollections[index];
                    return FeaturedCollectionCard(
                      collection: collection,
                      onPressed: () =>
                          context.push('/browse/collections/${collection.id}'),
                    );
                  },
                ),
              ),
            ),
            const SliverPadding(padding: EdgeInsets.only(top: 28)),
            const SliverToBoxAdapter(
              child: GizSectionHeader(title: 'Jump back in'),
            ),
            const SliverPadding(padding: EdgeInsets.only(top: 10)),
            SliverToBoxAdapter(child: _WorkspaceStrip(workspaces: recent)),
            const SliverPadding(padding: EdgeInsets.only(top: 28)),
            SliverToBoxAdapter(
              child: GizSectionHeader(
                title: 'All Workflows',
                actionLabel: 'View all',
                onAction: () => context.push('/browse/workflows'),
              ),
            ),
            const SliverPadding(padding: EdgeInsets.only(top: 4)),
            if (visibleWorkflows.isEmpty)
              SliverToBoxAdapter(child: _DataStatus(controller: data))
            else
              SliverList.builder(
                itemCount: visibleWorkflows.length,
                itemBuilder: (context, index) {
                  final workflow = visibleWorkflows[index];
                  return WorkflowListTile(workflow: workflow)
                      .animate(delay: (index * 45).ms)
                      .fadeIn(duration: 300.ms)
                      .slideY(begin: 0.06, end: 0, curve: Curves.easeOutCubic);
                },
              ),
            const SliverPadding(padding: EdgeInsets.only(bottom: 112)),
          ],
        ),
      ),
    );
  }
}

class _LiveBadge extends StatelessWidget {
  const _LiveBadge({required this.state});

  final MobileConnectionState state;

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.only(bottom: 3),
      padding: const EdgeInsets.fromLTRB(8, 5, 10, 5),
      decoration: BoxDecoration(
        color: GizColors.ink,
        borderRadius: BorderRadius.circular(99),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          GizSignalPulse(size: 17),
          SizedBox(width: 4),
          Text(
            state == MobileConnectionState.connected ? 'LIVE' : 'LOCAL',
            style: TextStyle(
              fontFamily: 'Manrope',
              color: GizColors.surface,
              fontSize: 10,
              fontWeight: FontWeight.w800,
              letterSpacing: 0,
            ),
          ),
        ],
      ),
    );
  }
}

class FeaturedCollectionCard extends StatelessWidget {
  const FeaturedCollectionCard({
    super.key,
    required this.collection,
    required this.onPressed,
  });

  final WorkflowCollection collection;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 328,
      child: GizPressable(
        onPressed: onPressed,
        borderRadius: BorderRadius.circular(12),
        scaleWhenPressed: 0.985,
        child: ClipRRect(
          borderRadius: BorderRadius.circular(12),
          child: Stack(
            fit: StackFit.expand,
            children: [
              CollectionArtworkHero(collection: collection),
              const DecoratedBox(
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.topCenter,
                    end: Alignment.bottomCenter,
                    colors: [
                      Color(0x0007100E),
                      Color(0x1807100E),
                      Color(0xE807100E),
                    ],
                    stops: [0, 0.42, 1],
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.all(18),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    GizTag(
                      label: collection.label,
                      backgroundColor: const Color(0xEFFFFFFF),
                      foregroundColor: GizColors.ink,
                    ),
                    const Spacer(),
                    Text(
                      collection.title,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: GizText.pageTitle.copyWith(
                        color: GizColors.surface,
                        fontSize: 27,
                      ),
                    ),
                    const SizedBox(height: 7),
                    Text(
                      collection.subtitle,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: GizText.body.copyWith(
                        color: const Color(0xD9FFFFFF),
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

class CollectionArtworkHero extends StatelessWidget {
  const CollectionArtworkHero({super.key, required this.collection});

  final WorkflowCollection collection;

  @override
  Widget build(BuildContext context) {
    const radius = BorderRadius.all(Radius.circular(12));
    return Hero(
      tag: 'collection-${collection.id}',
      transitionOnUserGestures: true,
      placeholderBuilder: (_, _, child) => child,
      flightShuttleBuilder:
          (flightContext, animation, direction, fromContext, toContext) {
            return ClipRRect(
              borderRadius: radius,
              child: Image.asset(collection.imagePath, fit: BoxFit.cover),
            );
          },
      child: ClipRRect(
        borderRadius: radius,
        child: Image.asset(collection.imagePath, fit: BoxFit.cover),
      ),
    );
  }
}

class _WorkspaceStrip extends StatelessWidget {
  const _WorkspaceStrip({required this.workspaces});

  final List<WorkspaceCard> workspaces;

  @override
  Widget build(BuildContext context) {
    if (workspaces.isEmpty) {
      return Padding(
        padding: const EdgeInsets.symmetric(horizontal: 20),
        child: Text(
          'Workspaces will appear after the server syncs.',
          style: GizText.body.copyWith(color: GizColors.secondaryInk),
        ),
      );
    }
    final data = MobileDataScope.watch(context);
    return SizedBox(
      height: 116,
      child: ListView.separated(
        padding: const EdgeInsets.symmetric(horizontal: 20),
        scrollDirection: Axis.horizontal,
        itemCount: workspaces.length,
        separatorBuilder: (_, _) => const SizedBox(width: 10),
        itemBuilder: (context, index) {
          final workspace = workspaces[index];
          final workflow = data.workflow(workspace.workflowName);
          return SizedBox(
            width: 248,
            child: GizPressable(
              onPressed: () => context.push(
                '/chats/workspaces/${Uri.encodeComponent(workspace.name)}',
              ),
              borderRadius: BorderRadius.circular(8),
              scaleWhenPressed: 0.985,
              child: DecoratedBox(
                decoration: BoxDecoration(
                  color: GizColors.surface,
                  borderRadius: BorderRadius.circular(8),
                  border: Border.all(color: GizColors.separator),
                ),
                child: Padding(
                  padding: const EdgeInsets.all(14),
                  child: Row(
                    children: [
                      Container(
                        width: 42,
                        height: 88,
                        alignment: Alignment.center,
                        decoration: BoxDecoration(
                          color: workflow.bannerColor,
                          borderRadius: BorderRadius.circular(6),
                        ),
                        child: Icon(
                          workflow.icon,
                          color: GizColors.surface,
                          size: 21,
                        ),
                      ),
                      const SizedBox(width: 13),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          mainAxisAlignment: MainAxisAlignment.center,
                          children: [
                            Text(
                              workspace.name,
                              maxLines: 2,
                              overflow: TextOverflow.ellipsis,
                              style: GizText.title,
                            ),
                            const SizedBox(height: 6),
                            Text(
                              workspace.lastActive,
                              style: GizText.label.copyWith(
                                color: GizColors.secondaryInk,
                              ),
                            ),
                          ],
                        ),
                      ),
                      const Icon(
                        CupertinoIcons.arrow_up_right,
                        size: 16,
                        color: GizColors.secondaryInk,
                      ),
                    ],
                  ),
                ),
              ),
            ),
          );
        },
      ),
    );
  }
}

class WorkflowListTile extends StatelessWidget {
  const WorkflowListTile({super.key, required this.workflow});

  final WorkflowCard workflow;

  @override
  Widget build(BuildContext context) {
    return GizPressable(
      onPressed: () => context.push('/browse/workflows/${workflow.name}'),
      child: Container(
        padding: const EdgeInsets.fromLTRB(20, 14, 16, 14),
        decoration: const BoxDecoration(
          border: Border(bottom: BorderSide(color: GizColors.separator)),
        ),
        child: Row(
          children: [
            SizedBox(
              width: 66,
              height: 66,
              child: WorkflowArtworkHero(workflow: workflow, compact: true),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    workflow.category.toUpperCase(),
                    style: GizText.label.copyWith(
                      color: workflow.bannerColor,
                      fontSize: 10,
                    ),
                  ),
                  const SizedBox(height: 4),
                  Text(workflow.title, style: GizText.title),
                  const SizedBox(height: 4),
                  Text(
                    workflow.subtitle,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: GizText.body.copyWith(color: GizColors.secondaryInk),
                  ),
                ],
              ),
            ),
            const SizedBox(width: 10),
            const Icon(
              CupertinoIcons.chevron_forward,
              size: 18,
              color: GizColors.secondaryInk,
            ),
          ],
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
    final radius = BorderRadius.circular(compact ? 10 : 14);
    final artwork = _WorkflowArtwork(workflow: workflow);
    return Hero(
      tag: 'workflow-${workflow.name}',
      transitionOnUserGestures: true,
      placeholderBuilder: (_, _, child) => child,
      flightShuttleBuilder:
          (flightContext, animation, direction, fromContext, toContext) {
            final pushing = direction == HeroFlightDirection.push;
            final begin = BorderRadius.circular(pushing ? 10 : 14);
            final end = BorderRadius.circular(pushing ? 14 : 10);
            return AnimatedBuilder(
              animation: animation,
              builder: (context, child) {
                return ClipRRect(
                  borderRadius: BorderRadius.lerp(
                    begin,
                    end,
                    Curves.easeInOut.transform(animation.value),
                  )!,
                  child: child,
                );
              },
              child: _WorkflowArtwork(workflow: workflow),
            );
          },
      child: ClipRRect(borderRadius: radius, child: artwork),
    );
  }
}

class _WorkflowArtwork extends StatelessWidget {
  const _WorkflowArtwork({required this.workflow});

  final WorkflowCard workflow;

  @override
  Widget build(BuildContext context) {
    if (workflow.imagePath != null) {
      return Image.asset(workflow.imagePath!, fit: BoxFit.cover);
    }
    return ColoredBox(
      color: workflow.bannerColor.withValues(alpha: 0.15),
      child: Center(
        child: Icon(workflow.icon, color: workflow.bannerColor, size: 26),
      ),
    );
  }
}

class AllWorkflowsPage extends StatelessWidget {
  const AllWorkflowsPage({super.key});

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    return CupertinoPageScaffold(
      navigationBar: const CupertinoNavigationBar(
        middle: Text('All Workflows'),
        border: null,
      ),
      child: SafeArea(
        child: ListView.builder(
          padding: const EdgeInsets.only(top: 8, bottom: 28),
          itemCount: data.workflows.length,
          itemBuilder: (context, index) {
            return WorkflowListTile(workflow: data.workflows[index]);
          },
        ),
      ),
    );
  }
}

class CollectionPage extends StatelessWidget {
  const CollectionPage({super.key, required this.collection});

  final WorkflowCollection collection;

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final workflows = data.workflows
        .where((workflow) => collection.workflowNames.contains(workflow.name))
        .toList();
    return CupertinoPageScaffold(
      navigationBar: const CupertinoNavigationBar(
        middle: Text('Collection'),
        border: null,
      ),
      child: CustomScrollView(
        slivers: [
          SliverSafeArea(
            bottom: false,
            sliver: SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 14, 20, 18),
              sliver: SliverToBoxAdapter(
                child: ClipRRect(
                  borderRadius: BorderRadius.circular(12),
                  child: AspectRatio(
                    aspectRatio: 4 / 3,
                    child: Stack(
                      fit: StackFit.expand,
                      children: [
                        CollectionArtworkHero(collection: collection),
                        const DecoratedBox(
                          decoration: BoxDecoration(
                            gradient: LinearGradient(
                              begin: Alignment.topCenter,
                              end: Alignment.bottomCenter,
                              colors: [Color(0x00000000), Color(0xDD07100E)],
                            ),
                          ),
                        ),
                        Positioned(
                          left: 20,
                          right: 20,
                          bottom: 20,
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                collection.title,
                                style: GizText.pageTitle.copyWith(
                                  color: GizColors.surface,
                                ),
                              ),
                              const SizedBox(height: 7),
                              Text(
                                collection.subtitle,
                                style: GizText.body.copyWith(
                                  color: const Color(0xD9FFFFFF),
                                ),
                              ),
                            ],
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ),
          ),
          const SliverToBoxAdapter(
            child: GizSectionHeader(title: 'In this collection'),
          ),
          SliverList.builder(
            itemCount: workflows.length,
            itemBuilder: (context, index) {
              return _CollectionWorkflowRow(workflow: workflows[index]);
            },
          ),
          const SliverPadding(padding: EdgeInsets.only(bottom: 30)),
        ],
      ),
    );
  }
}

class _CollectionWorkflowRow extends StatelessWidget {
  const _CollectionWorkflowRow({required this.workflow});

  final WorkflowCard workflow;

  @override
  Widget build(BuildContext context) {
    return GizListRow(
      leading: ClipRRect(
        borderRadius: BorderRadius.circular(8),
        child: SizedBox(
          width: 58,
          height: 58,
          child: _WorkflowArtwork(workflow: workflow),
        ),
      ),
      title: workflow.title,
      subtitle: '${workflow.category}  |  ${workflow.driverLabel}',
      onPressed: () => context.push('/browse/workflows/${workflow.name}'),
    );
  }
}

class WorkflowDetailPage extends StatelessWidget {
  const WorkflowDetailPage({super.key, required this.workflowName});

  final String workflowName;

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final workflow = data.workflow(workflowName);
    final workspaces = data.workspaces
        .where((workspace) => workspace.workflowName == workflow.name)
        .toList();
    return CupertinoPageScaffold(
      navigationBar: CupertinoNavigationBar(
        middle: Text(workflow.title),
        border: null,
      ),
      child: CustomScrollView(
        slivers: [
          SliverSafeArea(
            bottom: false,
            sliver: SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 14, 20, 20),
              sliver: SliverToBoxAdapter(
                child: AspectRatio(
                  aspectRatio: 1.5,
                  child: WorkflowArtworkHero(
                    workflow: workflow,
                    compact: false,
                  ),
                ),
              ),
            ),
          ),
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(20, 0, 20, 24),
            sliver: SliverToBoxAdapter(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      GizTag(
                        label: workflow.category,
                        backgroundColor: workflow.bannerColor,
                      ),
                      const SizedBox(width: 8),
                      GizTag(
                        label: workflow.driverLabel,
                        backgroundColor: GizColors.surface,
                        foregroundColor: GizColors.secondaryInk,
                      ),
                    ],
                  ),
                  const SizedBox(height: 14),
                  Text(workflow.title, style: GizText.pageTitle),
                  const SizedBox(height: 8),
                  Text(
                    workflow.subtitle,
                    style: GizText.body.copyWith(
                      color: GizColors.secondaryInk,
                      fontSize: 16,
                    ),
                  ),
                ],
              ),
            ),
          ),
          const SliverToBoxAdapter(
            child: GizSectionHeader(title: 'Your workspaces'),
          ),
          const SliverPadding(padding: EdgeInsets.only(top: 4)),
          if (workspaces.isEmpty)
            SliverPadding(
              padding: const EdgeInsets.all(20),
              sliver: SliverToBoxAdapter(
                child: Text(
                  'No workspace is available yet.',
                  style: GizText.body.copyWith(color: GizColors.secondaryInk),
                ),
              ),
            )
          else
            SliverList.builder(
              itemCount: workspaces.length,
              itemBuilder: (context, index) {
                return WorkspaceListTile(workspace: workspaces[index]);
              },
            ),
          const SliverPadding(padding: EdgeInsets.only(bottom: 32)),
        ],
      ),
    );
  }
}

class WorkspaceListTile extends StatelessWidget {
  const WorkspaceListTile({super.key, required this.workspace});

  final WorkspaceCard workspace;

  @override
  Widget build(BuildContext context) {
    final workflow = MobileDataScope.watch(
      context,
    ).workflow(workspace.workflowName);
    return GizListRow(
      leading: Container(
        width: 50,
        height: 50,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: workflow.bannerColor,
          borderRadius: BorderRadius.circular(8),
        ),
        child: Icon(workflow.icon, color: GizColors.surface, size: 22),
      ),
      title: workspace.name,
      subtitle: '${workflow.title}  |  ${workspace.lastActive}',
      onPressed: () => context.push(
        '/chats/workspaces/${Uri.encodeComponent(workspace.name)}',
      ),
    );
  }
}

class _DataStatus extends StatelessWidget {
  const _DataStatus({required this.controller});

  final MobileDataController controller;

  @override
  Widget build(BuildContext context) {
    final String title;
    final String message;
    switch (controller.connectionState) {
      case MobileConnectionState.connecting:
        title = 'Connecting';
        message = 'Opening a secure WebRTC session.';
        break;
      case MobileConnectionState.unconfigured:
        title = 'No server connected';
        message = 'Add a development connection profile to load workflows.';
        break;
      case MobileConnectionState.offline:
        title = 'Server unavailable';
        message = 'Cached workflows stay available while GizClaw reconnects.';
        break;
      case MobileConnectionState.connected:
        title = 'No workflows';
        message = 'This server has no workflows visible to this client.';
        break;
    }
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 20, 20, 28),
      child: Row(
        children: [
          if (controller.connectionState == MobileConnectionState.connecting)
            const CupertinoActivityIndicator()
          else
            const Icon(CupertinoIcons.cloud, color: GizColors.secondaryInk),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(title, style: GizText.title),
                const SizedBox(height: 3),
                Text(
                  message,
                  style: GizText.body.copyWith(color: GizColors.secondaryInk),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
