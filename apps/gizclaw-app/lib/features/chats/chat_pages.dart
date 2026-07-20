import 'dart:async';
import 'dart:math' as math;
import 'dart:ui';

import 'package:flutter/cupertino.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:go_router/go_router.dart';

import '../../app/global_conversation_control.dart';
import '../../data/mobile_data_controller.dart';
import '../../data/workspace_chat_controller.dart';
import '../../giz_ui/giz_ui.dart';
import '../../l10n/l10n.dart';
import '../../prototype/prototype_models.dart';
import '../../workflows/app_workflow_catalog.dart';

class CollectionWorkspacesPage extends StatelessWidget {
  const CollectionWorkspacesPage({super.key, required this.collection});

  final String collection;

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final definition = appWorkflowCollection(collection);
    final workspaces = data.workspaces
        .where((workspace) => workspace.collection == collection)
        .toList(growable: false);
    return CupertinoPageScaffold(
      child: SafeArea(
        bottom: false,
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 12, 12, 12),
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      definition.displayName(data.effectiveLocale),
                      style: GizText.pageTitle,
                    ),
                  ),
                  GizPageActionButton(
                    key: ValueKey('create-workspace-$collection'),
                    icon: GizIcons.add_circled_solid,
                    semanticLabel: context.l10n.newWorkspace,
                    onPressed: () =>
                        context.push('/collections/$collection/new'),
                  ),
                ],
              ),
            ),
            Expanded(
              child: _WorkspaceList(
                collection: collection,
                workspaces: workspaces,
                chatroomMetadata: data.chatroomWorkspaces,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class WorkflowPickerPage extends StatefulWidget {
  const WorkflowPickerPage({super.key, required this.collection});

  final String collection;

  @override
  State<WorkflowPickerPage> createState() => _WorkflowPickerPageState();
}

class _WorkflowPickerPageState extends State<WorkflowPickerPage> {
  String? _creatingAlias;
  Object? _error;

  Future<void> _select(WorkflowCard workflow) async {
    if (_creatingAlias != null ||
        workflow.driver == WorkflowDriverKind.unsupported ||
        workflow.driver == WorkflowDriverKind.chatroom) {
      return;
    }
    setState(() {
      _creatingAlias = workflow.name;
      _error = null;
    });
    try {
      final workspace = await MobileDataScope.watch(context).createWorkspace(
        collection: widget.collection,
        workflowAlias: workflow.name,
      );
      if (!mounted) return;
      context.go(
        '/collections/${widget.collection}/${Uri.encodeComponent(workspace.name)}',
      );
    } catch (error) {
      if (!mounted) return;
      setState(() {
        _creatingAlias = null;
        _error = error;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final collection = appWorkflowCollection(widget.collection);
    final workflows = data.workflowsForCollection(widget.collection);
    return CupertinoPageScaffold(
      navigationBar: CupertinoNavigationBar(
        middle: Text(collection.displayName(data.effectiveLocale)),
        border: null,
      ),
      child: SafeArea(
        child: Column(
          children: [
            if (_error case final error?)
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 0),
                child: Text(
                  _workspaceErrorMessage(error),
                  style: GizText.body.copyWith(
                    color: CupertinoColors.systemRed.resolveFrom(context),
                  ),
                ),
              ),
            Expanded(
              child: workflows.isEmpty
                  ? Center(
                      child: Text(
                        context.l10n.actionText(key: 'unavailable'),
                        style: GizText.body.copyWith(
                          color: GizColors.secondaryInk,
                        ),
                      ),
                    )
                  : ListView.separated(
                      key: ValueKey('workflow-picker-${widget.collection}'),
                      padding: const EdgeInsets.fromLTRB(16, 16, 16, 32),
                      itemCount: workflows.length,
                      separatorBuilder: (_, _) => const SizedBox(height: 8),
                      itemBuilder: (context, index) {
                        final workflow = workflows[index];
                        final busy = _creatingAlias == workflow.name;
                        final available =
                            workflow.driver != WorkflowDriverKind.unsupported &&
                            workflow.driver != WorkflowDriverKind.chatroom;
                        return GizListRow(
                          leading: _WorkflowIcon(workflow: workflow),
                          title: workflow.title,
                          subtitle: workflow.subtitle.isEmpty
                              ? workflow.driverLabel
                              : workflow.subtitle,
                          trailing: busy
                              ? const CupertinoActivityIndicator()
                              : const Icon(GizIcons.chevron_forward),
                          onPressed: available && _creatingAlias == null
                              ? () => _select(workflow)
                              : null,
                        );
                      },
                    ),
            ),
          ],
        ),
      ),
    );
  }
}

String _workspaceErrorMessage(Object error) {
  final text = error.toString();
  return text.startsWith('Bad state: ') ? text.substring(11) : text;
}

class _WorkspaceList extends StatelessWidget {
  const _WorkspaceList({
    required this.collection,
    required this.workspaces,
    required this.chatroomMetadata,
  });

  final List<ChatroomWorkspaceMetadata> chatroomMetadata;
  final String collection;
  final List<WorkspaceCard> workspaces;

  @override
  Widget build(BuildContext context) {
    if (workspaces.isEmpty) {
      return Center(
        child: Text(
          context.l10n.noWorkspaces,
          style: GizText.body.copyWith(color: GizColors.secondaryInk),
        ),
      );
    }
    return ListView.builder(
      key: PageStorageKey('workspaces-$collection'),
      padding: const EdgeInsets.only(bottom: 124),
      itemCount: workspaces.length,
      itemBuilder: (context, index) {
        final workspace = workspaces[index];
        final metadata = _metadataForWorkspace(workspace.name);
        void onPressed() {
          context.push(
            '/collections/$collection/${Uri.encodeComponent(workspace.name)}',
          );
        }

        final row = metadata != null || workspace.chatroomKind != null
            ? _ChatroomWorkspaceListTile(
                workspace: workspace,
                metadata: metadata,
                onPressed: onPressed,
              )
            : _WorkspaceListTile(workspace: workspace, onPressed: onPressed);
        if (index >= 8) return row;
        return row
            .animate(delay: (index * 32).ms)
            .fadeIn(duration: 220.ms)
            .slideY(begin: 0.035, end: 0, curve: Curves.easeOutCubic);
      },
    );
  }

  ChatroomWorkspaceMetadata? _metadataForWorkspace(String name) {
    for (final metadata in chatroomMetadata) {
      if (metadata.workspaceName == name) return metadata;
    }
    return null;
  }
}

class _ChatroomWorkspaceListTile extends StatelessWidget {
  const _ChatroomWorkspaceListTile({
    required this.workspace,
    required this.metadata,
    required this.onPressed,
  });

  final ChatroomWorkspaceMetadata? metadata;
  final VoidCallback onPressed;
  final WorkspaceCard workspace;

  @override
  Widget build(BuildContext context) {
    final kind = metadata?.kind ?? workspace.chatroomKind;
    final isDirect = kind == ChatroomWorkspaceKind.direct;
    final title = metadata?.title.trim();
    final description = metadata?.description.trim();
    final typeLabel = switch (kind) {
      ChatroomWorkspaceKind.direct => 'DIRECT CHAT',
      ChatroomWorkspaceKind.group => 'GROUP CHAT',
      null => 'CHATROOM',
    };
    return GizListRow(
      leading: isDirect
          ? GizSquircle(
              borderRadius: GizCorners.icon(50),
              child: Container(
                width: 50,
                height: 50,
                alignment: Alignment.center,
                color: const Color(0xFFD9F2EA),
                child: const Icon(
                  GizIcons.person_fill,
                  color: GizColors.ink,
                  size: 22,
                ),
              ),
            )
          : const GizIconTile(
              icon: GizIcons.person_2_fill,
              backgroundColor: Color(0xFFDDE8FF),
              foregroundColor: Color(0xFF315E9D),
              size: 50,
              iconSize: 22,
            ),
      title: title == null || title.isEmpty ? workspace.title : title,
      subtitle:
          '$typeLabel  |  '
          '${description == null || description.isEmpty ? workspace.lastActive : description}',
      onPressed: onPressed,
    );
  }
}

class _WorkspaceListTile extends StatelessWidget {
  const _WorkspaceListTile({required this.workspace, this.onPressed});

  final WorkspaceCard workspace;
  final VoidCallback? onPressed;

  @override
  Widget build(BuildContext context) {
    final workflow = MobileDataScope.watch(
      context,
    ).workflow(workspace.workflowAlias, collection: workspace.collection);
    final workflowLabel = workflow.driver == WorkflowDriverKind.unsupported
        ? context.l10n.actionText(key: 'unavailable')
        : workflow.title;
    return GizListRow(
      leading: _WorkflowIcon(workflow: workflow),
      title: workspace.title,
      subtitle: '$workflowLabel  |  ${workspace.lastActive}',
      onPressed:
          onPressed ??
          () => context.push(
            '/collections/${workspace.collection}/${Uri.encodeComponent(workspace.name)}',
          ),
    );
  }
}

class _WorkflowIcon extends StatelessWidget {
  const _WorkflowIcon({required this.workflow});

  final WorkflowCard workflow;

  @override
  Widget build(BuildContext context) {
    return GizIconTile(
      icon: workflow.icon,
      backgroundColor: workflow.bannerColor,
      foregroundColor: GizColors.surface,
      size: 50,
      iconSize: 22,
    );
  }
}

class WorkspaceChatPage extends StatefulWidget {
  const WorkspaceChatPage({super.key, required this.workspaceName});

  final String workspaceName;

  @override
  State<WorkspaceChatPage> createState() => _WorkspaceChatPageState();
}

typedef _WorkspaceMessageSnapshot = ({
  int count,
  String? lastId,
  String? lastText,
  WorkspaceMessageState? lastState,
});

typedef _WorkspaceChatViewSnapshot = ({
  WorkspaceChatState? state,
  bool recording,
  Object? error,
  String? replayingHistoryId,
  _WorkspaceMessageSnapshot messages,
});

_WorkspaceMessageSnapshot _workspaceMessageSnapshot(
  WorkspaceChatController? chat,
) {
  final messages = chat?.messages ?? const <WorkspaceChatMessage>[];
  final last = messages.isEmpty ? null : messages.last;
  return (
    count: messages.length,
    lastId: last?.id,
    lastText: last?.text,
    lastState: last?.state,
  );
}

_WorkspaceChatViewSnapshot _workspaceChatViewSnapshot(
  WorkspaceChatController? chat,
) => (
  state: chat?.state,
  recording: chat?.recording ?? false,
  error: chat?.lastError,
  replayingHistoryId: chat?.replayingHistoryId,
  messages: _workspaceMessageSnapshot(chat),
);

class _WorkspaceChatPageState extends State<WorkspaceChatPage> {
  static const _bottomFollowThreshold = 48.0;

  final _scrollController = ScrollController();
  WorkspaceChatController? _chat;
  _WorkspaceChatViewSnapshot? _chatViewSnapshot;
  bool _ownsChat = false;
  bool _autoScrollScheduled = false;
  bool _stickToBottom = true;
  bool _userScrolling = false;
  bool _newMessagesBelow = false;
  int _chatRequest = 0;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final data = MobileDataScope.watch(context);
    final active = data.activeWorkspaceChat;
    if (active?.workspaceName == widget.workspaceName) {
      _bindChat(active!, ownsChat: false, notify: false);
      return;
    }
    if (!_ownsChat || _chat?.workspaceName != widget.workspaceName) {
      unawaited(_loadHistoryViewer(data));
    }
  }

  Future<void> _loadHistoryViewer(MobileDataController data) async {
    final request = ++_chatRequest;
    final viewer = WorkspaceChatController(
      workspaceName: widget.workspaceName,
      repository: data.workspaceChatRepository,
      serverId: data.activeServerId,
      client: data.connection.client,
      onWorkspaceAccessError: data.reconcileWorkspaceFailure,
    );
    _bindChat(viewer, ownsChat: true, notify: true);
    await viewer.start(conversation: false);
    if (!mounted || request != _chatRequest) return;
    final previousMessages = _chatViewSnapshot?.messages;
    _chatViewSnapshot = _workspaceChatViewSnapshot(viewer);
    setState(() {});
    if (previousMessages != _chatViewSnapshot?.messages) {
      _scheduleMessageViewportUpdate();
    }
  }

  void _bindChat(
    WorkspaceChatController chat, {
    required bool ownsChat,
    required bool notify,
  }) {
    if (identical(chat, _chat)) return;
    _chat?.removeListener(_handleChatChanged);
    if (_ownsChat) _chat?.dispose();
    _chat = chat;
    _ownsChat = ownsChat;
    _chatViewSnapshot = _workspaceChatViewSnapshot(chat);
    _stickToBottom = true;
    _userScrolling = false;
    _newMessagesBelow = false;
    chat.addListener(_handleChatChanged);
    if (notify && mounted) setState(() {});
  }

  void _handleChatChanged() {
    if (!mounted) return;
    final next = _workspaceChatViewSnapshot(_chat);
    final messagesChanged = next.messages != _chatViewSnapshot?.messages;
    if (next == _chatViewSnapshot) return;
    _chatViewSnapshot = next;
    if (messagesChanged && !_stickToBottom) _newMessagesBelow = true;
    setState(() {});
    if (messagesChanged) _scheduleMessageViewportUpdate();
  }

  void _scheduleMessageViewportUpdate() {
    if (_autoScrollScheduled || _userScrolling) return;
    final preserve = !_stickToBottom && _scrollController.hasClients
        ? (
            pixels: _scrollController.position.pixels,
            maxScrollExtent: _scrollController.position.maxScrollExtent,
          )
        : null;
    _autoScrollScheduled = true;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _autoScrollScheduled = false;
      if (!mounted || _userScrolling || !_scrollController.hasClients) {
        return;
      }
      final position = _scrollController.position;
      if (_stickToBottom) {
        _scrollController.jumpTo(position.minScrollExtent);
        return;
      }
      if (preserve == null) return;
      final extentGrowth = position.maxScrollExtent - preserve.maxScrollExtent;
      final target = (preserve.pixels + extentGrowth).clamp(
        position.minScrollExtent,
        position.maxScrollExtent,
      );
      _scrollController.jumpTo(target);
    });
  }

  bool _handleScrollNotification(ScrollNotification notification) {
    if (notification.depth != 0) return false;
    final userDriven = switch (notification) {
      ScrollStartNotification(:final dragDetails) => dragDetails != null,
      ScrollUpdateNotification(:final dragDetails) => dragDetails != null,
      OverscrollNotification(:final dragDetails) => dragDetails != null,
      _ => false,
    };
    if (userDriven) {
      _userScrolling = true;
      _stickToBottom = _isNearBottom(notification.metrics);
    } else if (notification is ScrollEndNotification) {
      _userScrolling = false;
      _stickToBottom = _isNearBottom(notification.metrics);
    }
    if (_stickToBottom && _newMessagesBelow) {
      setState(() => _newMessagesBelow = false);
    }
    return false;
  }

  bool _isNearBottom(ScrollMetrics metrics) =>
      metrics.pixels <= metrics.minScrollExtent + _bottomFollowThreshold;

  void _showNewestMessages() {
    _stickToBottom = true;
    setState(() => _newMessagesBelow = false);
    if (!_scrollController.hasClients) return;
    unawaited(
      _scrollController.animateTo(
        _scrollController.position.minScrollExtent,
        duration: const Duration(milliseconds: 240),
        curve: Curves.easeOutCubic,
      ),
    );
  }

  @override
  void dispose() {
    _chatRequest += 1;
    _chat?.removeListener(_handleChatChanged);
    if (_ownsChat) _chat?.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final workspace = data.workspace(widget.workspaceName);
    final workflow = data.workflow(
      workspace.workflowAlias,
      collection: workspace.collection,
    );
    final chatroomMetadata = data.chatroomWorkspace(widget.workspaceName);
    final chat = _chat;
    final isActiveWorkspace = data.activeWorkspaceName == widget.workspaceName;
    final messages = chat?.messages ?? const <WorkspaceChatMessage>[];
    final signal = _SignalPalette.of(context);
    final isDirectChat = chatroomMetadata?.kind == ChatroomWorkspaceKind.direct;
    final accent = isDirectChat
        ? _workspaceVoiceAccent(signal.brightness)
        : _driverAccent(workflow.driver, signal.brightness);
    return CupertinoPageScaffold(
      backgroundColor: signal.canvas,
      child: SafeArea(
        bottom: false,
        child: _AgentSignalScene(
          resourceId: widget.workspaceName,
          state: chat?.state ?? WorkspaceChatState.loading,
          recording: chat?.recording ?? false,
          accent: accent,
          signal: signal,
          child: Stack(
            children: [
              Positioned.fill(
                child: NotificationListener<ScrollNotification>(
                  onNotification: _handleScrollNotification,
                  child: _WorkspaceMessageList(
                    controller: _scrollController,
                    messages: messages,
                    state: chat?.state ?? WorkspaceChatState.loading,
                    signal: signal,
                    error: chat?.lastError,
                    replayingHistoryId: chat?.replayingHistoryId,
                    onReplay: isActiveWorkspace ? chat?.replayHistory : null,
                  ),
                ),
              ),
              if (_newMessagesBelow)
                Positioned(
                  left: 24,
                  right: 24,
                  bottom:
                      GlobalConversationOverlay.bottomContentInset(context) + 4,
                  child: Center(
                    child: CupertinoButton.tinted(
                      key: const ValueKey('new-messages-below'),
                      sizeStyle: CupertinoButtonSize.small,
                      onPressed: _showNewestMessages,
                      child: Text(context.l10n.newMessagesBelow),
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

String _connectionLabel(WorkspaceChatState? state) => switch (state) {
  WorkspaceChatState.connected => 'LIVE',
  WorkspaceChatState.connecting || WorkspaceChatState.loading => 'LINKING',
  WorkspaceChatState.offline => 'OFFLINE',
  WorkspaceChatState.error => 'SIGNAL LOST',
  null => 'LINKING',
};

Color _driverAccent(WorkflowDriverKind driver, Brightness brightness) =>
    switch ((driver, brightness)) {
      (WorkflowDriverKind.astTranslate, Brightness.light) =>
        GizColors.primaryShadow,
      (WorkflowDriverKind.astTranslate, Brightness.dark) =>
        GizColors.primaryHighlight,
      (WorkflowDriverKind.doubaoRealtime, Brightness.light) => GizColors.coral,
      (WorkflowDriverKind.flowcraft, Brightness.light) => GizColors.blue,
      (WorkflowDriverKind.chatroom, Brightness.light) => GizColors.lavender,
      (WorkflowDriverKind.doubaoRealtime, Brightness.dark) => const Color(
        0xFFE9A08A,
      ),
      (WorkflowDriverKind.flowcraft, Brightness.dark) => const Color(
        0xFF9BB8C9,
      ),
      (WorkflowDriverKind.chatroom, Brightness.dark) => const Color(0xFFC4B6DB),
      (WorkflowDriverKind.unsupported, _) => GizColors.accent,
    };

Color _workspaceVoiceAccent(Brightness brightness) =>
    brightness == Brightness.dark ? const Color(0xFF94D3C0) : GizColors.accent;

class _SignalPalette {
  const _SignalPalette({
    required this.brightness,
    required this.canvas,
    required this.chrome,
    required this.panel,
    required this.panelStrong,
    required this.line,
    required this.muted,
    required this.text,
    required this.onAccent,
    required this.actionAccent,
    required this.brandAccent,
    required this.outgoingFill,
    required this.outgoingText,
  });

  static const light = _SignalPalette(
    brightness: Brightness.light,
    canvas: GizColors.canvas,
    chrome: Color(0xF2F5F6F2),
    panel: GizColors.surface,
    panelStrong: Color(0xFFE7EDE8),
    line: GizColors.separator,
    muted: GizColors.secondaryInk,
    text: GizColors.ink,
    onAccent: GizColors.ink,
    actionAccent: GizColors.accent,
    brandAccent: GizColors.accent,
    outgoingFill: GizColors.messageBlue,
    outgoingText: GizColors.surface,
  );

  static const dark = _SignalPalette(
    brightness: Brightness.dark,
    canvas: Color(0xFF0A100D),
    chrome: Color(0xED0A100D),
    panel: Color(0xFF13201B),
    panelStrong: Color(0xFF1B2A24),
    line: Color(0xFF304039),
    muted: Color(0xFF94A39C),
    text: Color(0xFFF3F7F4),
    onAccent: GizColors.ink,
    actionAccent: GizColors.accent,
    brandAccent: GizColors.accent,
    outgoingFill: GizColors.messageBlue,
    outgoingText: GizColors.surface,
  );

  final Color actionAccent;
  final Brightness brightness;
  final Color brandAccent;
  final Color canvas;
  final Color chrome;
  final Color line;
  final Color muted;
  final Color onAccent;
  final Color outgoingFill;
  final Color outgoingText;
  final Color panel;
  final Color panelStrong;
  final Color text;

  static _SignalPalette of(BuildContext context) =>
      MediaQuery.platformBrightnessOf(context) == Brightness.dark
      ? dark
      : light;
}

class _AgentSignalScene extends StatefulWidget {
  const _AgentSignalScene({
    required this.resourceId,
    required this.state,
    required this.recording,
    required this.accent,
    required this.signal,
    required this.child,
  });

  final Color accent;
  final Widget child;
  final String resourceId;
  final bool recording;
  final _SignalPalette signal;
  final WorkspaceChatState state;

  @override
  State<_AgentSignalScene> createState() => _AgentSignalSceneState();
}

class _AgentSignalSceneState extends State<_AgentSignalScene>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 3600),
  )..repeat();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final active = widget.state == WorkspaceChatState.connected;
    return AnimatedBuilder(
      animation: _controller,
      child: widget.child,
      builder: (context, child) {
        final energy = widget.recording
            ? 0.78 + math.sin(_controller.value * math.pi * 10) * 0.18
            : active
            ? 0.42 + math.sin(_controller.value * math.pi * 2) * 0.08
            : 0.18;
        return Stack(
          fit: StackFit.expand,
          children: [
            Positioned.fill(child: child!),
            Positioned(
              top: 4,
              left: 0,
              right: 0,
              height: 104,
              child: IgnorePointer(
                child: CustomPaint(
                  painter: _SignalFieldPainter(
                    progress: _controller.value,
                    accent: widget.accent,
                    energy: energy,
                  ),
                ),
              ),
            ),
            Positioned(
              top: 4,
              left: 0,
              right: 0,
              height: 104,
              child: IgnorePointer(
                child: Stack(
                  alignment: Alignment.center,
                  children: [
                    Transform.translate(
                      offset: Offset(
                        0,
                        math.sin(_controller.value * math.pi * 2) * 3,
                      ),
                      child: _AgentCore(
                        resourceId: widget.resourceId,
                        accent: widget.accent,
                        energy: energy,
                        signal: widget.signal,
                      ),
                    ),
                    Positioned(
                      bottom: 6,
                      child: DecoratedBox(
                        decoration: BoxDecoration(
                          color: widget.signal.panel.withValues(alpha: 0.82),
                          borderRadius: BorderRadius.circular(99),
                          border: Border.all(color: widget.signal.line),
                        ),
                        child: Padding(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 9,
                            vertical: 4,
                          ),
                          child: Text(
                            widget.recording
                                ? 'LISTENING'
                                : active
                                ? 'LIVE'
                                : _connectionLabel(widget.state),
                            style: GizText.label.copyWith(
                              color: widget.recording
                                  ? widget.accent
                                  : widget.signal.muted,
                              fontSize: 8,
                            ),
                          ),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ],
        );
      },
    );
  }
}

class _AgentCore extends StatelessWidget {
  const _AgentCore({
    required this.resourceId,
    required this.accent,
    required this.energy,
    required this.signal,
  });

  final Color accent;
  final double energy;
  final String resourceId;
  final _SignalPalette signal;

  @override
  Widget build(BuildContext context) {
    return SizedBox.square(
      dimension: 82,
      child: Stack(
        alignment: Alignment.center,
        children: [
          Container(
            width: 68 + energy * 8,
            height: 68 + energy * 8,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              border: Border.all(
                color: accent.withValues(alpha: 0.24 + energy * 0.2),
              ),
              boxShadow: [
                BoxShadow(
                  color: accent.withValues(alpha: 0.12 + energy * 0.14),
                  blurRadius: 24,
                  spreadRadius: 2,
                ),
              ],
            ),
          ),
          ClipOval(
            child: Container(
              width: 54,
              height: 54,
              padding: const EdgeInsets.all(3),
              decoration: BoxDecoration(
                color: signal.panelStrong,
                shape: BoxShape.circle,
                border: Border.all(color: accent.withValues(alpha: 0.46)),
              ),
              child: Center(
                key: ValueKey('resource-initial-$resourceId'),
                child: Text(
                  gizResourceInitial(resourceId),
                  style: GizText.sectionTitle.copyWith(color: accent),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _SignalFieldPainter extends CustomPainter {
  const _SignalFieldPainter({
    required this.progress,
    required this.accent,
    required this.energy,
  });

  final Color accent;
  final double energy;
  final double progress;

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height * 0.52);
    final glow = Paint()
      ..shader =
          RadialGradient(
            colors: [
              accent.withValues(alpha: 0.18 * energy),
              accent.withValues(alpha: 0),
            ],
          ).createShader(
            Rect.fromCircle(center: center, radius: size.width * 0.48),
          );
    canvas.drawCircle(center, size.width * 0.48, glow);

    for (var line = 0; line < 6; line++) {
      final path = Path();
      final baseline = size.height * (0.26 + line * 0.1);
      for (var x = 0.0; x <= size.width; x += 4) {
        final distance = (x - center.dx).abs() / center.dx;
        final focus = math.pow(math.max(0, 1 - distance), 2).toDouble();
        final phase = progress * math.pi * 2 + line * 0.72;
        final y =
            baseline + math.sin(x * 0.046 + phase) * (3 + 11 * focus * energy);
        if (x == 0) {
          path.moveTo(x, y);
        } else {
          path.lineTo(x, y);
        }
      }
      canvas.drawPath(
        path,
        Paint()
          ..style = PaintingStyle.stroke
          ..strokeWidth = line == 2 ? 1.35 : 0.8
          ..color = accent.withValues(alpha: 0.14 + energy * 0.1),
      );
    }
  }

  @override
  bool shouldRepaint(_SignalFieldPainter oldDelegate) =>
      oldDelegate.progress != progress ||
      oldDelegate.energy != energy ||
      oldDelegate.accent != accent;
}

class _WorkspaceMessageList extends StatelessWidget {
  const _WorkspaceMessageList({
    required this.controller,
    required this.messages,
    required this.state,
    required this.signal,
    required this.error,
    required this.replayingHistoryId,
    required this.onReplay,
  });

  final ScrollController controller;
  final Object? error;
  final List<WorkspaceChatMessage> messages;
  final ValueChanged<String>? onReplay;
  final String? replayingHistoryId;
  final _SignalPalette signal;
  final WorkspaceChatState state;

  @override
  Widget build(BuildContext context) {
    if (messages.isEmpty &&
        (state == WorkspaceChatState.loading ||
            state == WorkspaceChatState.connecting)) {
      return Center(child: CupertinoActivityIndicator(color: signal.muted));
    }
    if (messages.isEmpty) {
      final unavailable =
          state == WorkspaceChatState.error ||
          state == WorkspaceChatState.offline;
      final errorMessage = error == null ? null : _workspaceError(error!);
      return Center(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 36),
          child: Text(
            errorMessage ??
                (unavailable
                    ? 'This conversation is unavailable right now.'
                    : 'The channel is clear.\nHold the signal to speak.'),
            textAlign: TextAlign.center,
            style: GizText.body.copyWith(
              color: errorMessage == null
                  ? signal.muted
                  : CupertinoColors.systemRed.resolveFrom(context),
              height: 1.65,
            ),
          ),
        ),
      );
    }
    return ShaderMask(
      blendMode: BlendMode.dstIn,
      shaderCallback: (bounds) => const LinearGradient(
        begin: Alignment.topCenter,
        end: Alignment.bottomCenter,
        colors: [
          Color(0x00FFFFFF),
          Color(0x08FFFFFF),
          Color(0x45FFFFFF),
          Color(0xB8FFFFFF),
          Color(0xFFFFFFFF),
          Color(0xFFFFFFFF),
        ],
        stops: [0, 0.12, 0.3, 0.48, 0.64, 1],
      ).createShader(bounds),
      child: ListView.separated(
        key: const ValueKey('workspace-message-list'),
        controller: controller,
        reverse: true,
        padding: EdgeInsets.fromLTRB(
          22,
          16,
          22,
          GlobalConversationOverlay.bottomContentInset(context),
        ),
        itemCount: messages.length + (error == null ? 0 : 1),
        separatorBuilder: (_, _) => const SizedBox(height: 12),
        itemBuilder: (context, index) {
          if (index == messages.length) {
            return Text(
              'Live updates paused. Showing saved messages.',
              textAlign: TextAlign.center,
              style: GizText.label.copyWith(color: signal.muted),
            );
          }
          final message = messages[messages.length - 1 - index];
          return _WorkspaceSignalMessage(
            message: message,
            signal: signal,
            replaying: replayingHistoryId == message.id,
            onReplay: message.replayAvailable && onReplay != null
                ? () => onReplay!(message.id)
                : null,
          );
        },
      ),
    );
  }
}

String _workspaceError(Object error) {
  final text = error.toString();
  if (text.contains('ASR produced empty transcript')) {
    return "I couldn't hear that. Hold the mic and speak again.";
  }
  return text.startsWith('Bad state: ') ? text.substring(11) : text;
}

class _WorkspaceSignalMessage extends StatelessWidget {
  const _WorkspaceSignalMessage({
    required this.message,
    required this.signal,
    required this.replaying,
    required this.onReplay,
  });

  final WorkspaceChatMessage message;
  final VoidCallback? onReplay;
  final bool replaying;
  final _SignalPalette signal;

  @override
  Widget build(BuildContext context) {
    final incoming = message.incoming;
    final width = MediaQuery.sizeOf(context).width;
    final alignment = incoming
        ? CrossAxisAlignment.start
        : CrossAxisAlignment.end;
    final accent = incoming ? signal.brandAccent : GizColors.messageBlue;
    return Align(
      alignment: incoming ? Alignment.centerLeft : Alignment.centerRight,
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: width * 0.78),
        child: CupertinoButton(
          minimumSize: Size.zero,
          padding: EdgeInsets.zero,
          pressedOpacity: onReplay == null ? 1 : 0.58,
          onPressed: onReplay,
          child: Column(
            crossAxisAlignment: alignment,
            children: [
              Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(
                    incoming ? GizIcons.sparkles : GizIcons.waveform,
                    size: 12,
                    color: accent,
                  ),
                  const SizedBox(width: 7),
                  Text(
                    incoming ? 'AGENT' : 'YOU',
                    style: GizText.label.copyWith(color: accent, fontSize: 8),
                  ),
                  const SizedBox(width: 8),
                  Container(
                    width: 2,
                    height: 2,
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      color: signal.muted,
                    ),
                  ),
                  const SizedBox(width: 8),
                  Text(
                    _messageTime(message.createdAt),
                    style: GizText.label.copyWith(
                      color: signal.muted,
                      fontSize: 8,
                    ),
                  ),
                  if (message.replayAvailable) ...[
                    const SizedBox(width: 9),
                    if (replaying)
                      CupertinoActivityIndicator(radius: 5, color: accent)
                    else
                      Icon(GizIcons.play_fill, size: 10, color: accent),
                    const SizedBox(width: 4),
                    Text(
                      replaying ? 'OPENING' : 'REPLAY',
                      style: GizText.label.copyWith(
                        color: signal.muted,
                        fontSize: 7,
                      ),
                    ),
                  ],
                ],
              ),
              const SizedBox(height: 6),
              GizSquircle(
                borderRadius: GizCorners.compactCard,
                child: BackdropFilter(
                  filter: ImageFilter.blur(sigmaX: 9, sigmaY: 9),
                  child: DecoratedBox(
                    decoration: BoxDecoration(
                      color: incoming
                          ? signal.panel.withValues(alpha: 0.78)
                          : signal.outgoingFill.withValues(alpha: 0.9),
                      border: Border.all(
                        color: incoming
                            ? signal.line.withValues(alpha: 0.55)
                            : accent.withValues(alpha: 0.2),
                        width: 0.5,
                      ),
                    ),
                    child: Padding(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 12,
                        vertical: 9,
                      ),
                      child: Text(
                        message.text.isEmpty ? '...' : message.text,
                        style: GizText.body.copyWith(
                          color: incoming ? signal.text : signal.outgoingText,
                          fontSize: 13,
                          height: 1.4,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ),
                  ),
                ),
              ),
              if (message.state == WorkspaceMessageState.streaming ||
                  message.state == WorkspaceMessageState.failed) ...[
                const SizedBox(height: 6),
                Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (message.state == WorkspaceMessageState.streaming)
                      CupertinoActivityIndicator(radius: 5, color: accent)
                    else
                      Icon(
                        GizIcons.exclamationmark_circle_fill,
                        size: 11,
                        color: accent,
                      ),
                    const SizedBox(width: 5),
                    Text(
                      message.state == WorkspaceMessageState.failed
                          ? 'INTERRUPTED'
                          : 'STREAMING',
                      style: GizText.label.copyWith(
                        color: signal.muted,
                        fontSize: 7,
                      ),
                    ),
                  ],
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

String _messageTime(DateTime? value) {
  if (value == null) return 'NOW';
  final local = value.toLocal();
  final hour = local.hour.toString().padLeft(2, '0');
  final minute = local.minute.toString().padLeft(2, '0');
  return '$hour:$minute';
}

class ChatroomWorkspacePage extends StatelessWidget {
  const ChatroomWorkspacePage({super.key, required this.workspaceName});

  final String workspaceName;

  @override
  Widget build(BuildContext context) {
    return WorkspaceChatPage(workspaceName: workspaceName);
  }
}
