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
import '../../prototype/prototype_models.dart';
import '../browse/browse_pages.dart';

class ChatsPage extends StatelessWidget {
  const ChatsPage({super.key});

  @override
  Widget build(BuildContext context) {
    return CupertinoPageScaffold(
      child: SafeArea(
        bottom: false,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Padding(
              padding: EdgeInsets.fromLTRB(20, 12, 20, 16),
              child: Text('Chats', style: GizText.pageTitle),
            ),
            const Expanded(child: _ChatTypeMenu()),
          ],
        ),
      ),
    );
  }
}

class _ChatTypeMenu extends StatelessWidget {
  const _ChatTypeMenu();

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final drivers = WorkflowDriverKind.values
        .where((driver) {
          if (driver == WorkflowDriverKind.unsupported ||
              driver == WorkflowDriverKind.chatroom) {
            return false;
          }
          return data.workflows.any((workflow) => workflow.driver == driver);
        })
        .toList(growable: false);
    if (drivers.isEmpty) {
      return Center(
        child: Text(
          'No chat workspaces yet.',
          style: GizText.body.copyWith(color: GizColors.secondaryInk),
        ),
      );
    }
    return ListView.builder(
      key: const PageStorageKey('chat-types'),
      padding: const EdgeInsets.only(bottom: 112),
      itemCount: drivers.length,
      itemBuilder: (context, index) {
        final driver = drivers[index];
        final count = data.workspaces.where((workspace) {
          return data.workflow(workspace.workflowName).driver == driver;
        }).length;
        return GizListRow(
              leading: _ChatTypeIcon(driver: driver),
              title: driver.label,
              subtitle: '$count workspaces',
              onPressed: () =>
                  context.push('/chats/drivers/${driver.routeKey}'),
            )
            .animate(delay: (index * 45).ms)
            .fadeIn(duration: 280.ms)
            .slideY(begin: 0.05, end: 0, curve: Curves.easeOutCubic);
      },
    );
  }
}

class _ChatTypeIcon extends StatelessWidget {
  const _ChatTypeIcon({required this.driver});

  final WorkflowDriverKind driver;

  @override
  Widget build(BuildContext context) {
    final imagePath = driver.imagePath;
    return ClipRSuperellipse(
      borderRadius: GizCorners.icon(50),
      child: Container(
        width: 50,
        height: 50,
        alignment: Alignment.center,
        color: const Color(0xFFE9ECE9),
        child: imagePath == null
            ? const Icon(
                CupertinoIcons.question_circle_fill,
                color: GizColors.secondaryInk,
              )
            : Image.asset(
                imagePath,
                width: 50,
                height: 50,
                fit: BoxFit.cover,
                filterQuality: FilterQuality.high,
              ),
      ),
    );
  }
}

class DriverWorkspacesPage extends StatelessWidget {
  const DriverWorkspacesPage({super.key, required this.driver});

  final WorkflowDriverKind driver;

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final workspaces = data.workspaces
        .where((workspace) {
          return data.workflow(workspace.workflowName).driver == driver;
        })
        .toList(growable: false);
    return CupertinoPageScaffold(
      child: SafeArea(
        child: _DriverWorkspaceList(
          driver: driver,
          workspaces: workspaces,
          chatroomMetadata: data.chatroomWorkspaces,
        ),
      ),
    );
  }
}

class _DriverWorkspaceList extends StatelessWidget {
  const _DriverWorkspaceList({
    required this.driver,
    required this.workspaces,
    required this.chatroomMetadata,
  });

  final List<ChatroomWorkspaceMetadata> chatroomMetadata;
  final WorkflowDriverKind driver;
  final List<WorkspaceCard> workspaces;

  @override
  Widget build(BuildContext context) {
    if (workspaces.isEmpty) {
      return Center(
        child: Text(
          'No ${driver.label} workspaces yet.',
          style: GizText.body.copyWith(color: GizColors.secondaryInk),
        ),
      );
    }
    return ListView.builder(
      key: PageStorageKey('driver-workspaces-${driver.routeKey}'),
      padding: const EdgeInsets.only(bottom: 124),
      itemCount: workspaces.length,
      itemBuilder: (context, index) {
        final workspace = workspaces[index];
        final metadata = driver == WorkflowDriverKind.chatroom
            ? _metadataForWorkspace(workspace.name)
            : null;
        void onPressed() {
          context.push(
            '/chats/drivers/${driver.routeKey}/'
            '${Uri.encodeComponent(workspace.name)}',
          );
        }

        final row = driver == WorkflowDriverKind.chatroom
            ? _ChatroomWorkspaceListTile(
                workspace: workspace,
                metadata: metadata,
                onPressed: onPressed,
              )
            : WorkspaceListTile(workspace: workspace, onPressed: onPressed);
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
                  CupertinoIcons.person_fill,
                  color: Color(0xFF17795B),
                  size: 22,
                ),
              ),
            )
          : const GizIconTile(
              icon: CupertinoIcons.person_2_fill,
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

class WorkspaceChatPage extends StatefulWidget {
  const WorkspaceChatPage({super.key, required this.workspaceName});

  final String workspaceName;

  @override
  State<WorkspaceChatPage> createState() => _WorkspaceChatPageState();
}

class _WorkspaceChatPageState extends State<WorkspaceChatPage> {
  final _scrollController = ScrollController();
  WorkspaceChatController? _chat;
  bool _ownsChat = false;
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
    );
    _bindChat(viewer, ownsChat: true, notify: true);
    await viewer.start(conversation: false);
    if (!mounted || request != _chatRequest) return;
    setState(() {});
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
    chat.addListener(_handleChatChanged);
    if (notify && mounted) setState(() {});
  }

  void _handleChatChanged() {
    if (!mounted) return;
    setState(() {});
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scrollController.hasClients) {
        _scrollController.animateTo(
          _scrollController.position.minScrollExtent,
          duration: const Duration(milliseconds: 220),
          curve: Curves.easeOutCubic,
        );
      }
    });
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
    final workflow = data.workflow(workspace.workflowName);
    final chatroomMetadata = data.chatroomWorkspace(widget.workspaceName);
    final chat = _chat;
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
          imagePath: isDirectChat ? null : workflow.driver.imagePath,
          state: chat?.state ?? WorkspaceChatState.loading,
          recording: chat?.recording ?? false,
          accent: accent,
          signal: signal,
          child: _WorkspaceMessageList(
            controller: _scrollController,
            messages: messages,
            state: chat?.state ?? WorkspaceChatState.loading,
            signal: signal,
            error: chat?.lastError,
            replayingHistoryId: chat?.replayingHistoryId,
            onReplay: chat?.replayHistory,
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
      (WorkflowDriverKind.astTranslate, _) => _workspaceVoiceAccent(brightness),
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
    brightness == Brightness.dark ? const Color(0xFF94D3C0) : GizColors.teal;

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
    brandAccent: GizColors.teal,
    outgoingFill: GizColors.teal,
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
    outgoingFill: GizColors.accent,
    outgoingText: GizColors.ink,
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
    required this.imagePath,
    required this.state,
    required this.recording,
    required this.accent,
    required this.signal,
    required this.child,
  });

  final Color accent;
  final Widget child;
  final String? imagePath;
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
                        imagePath: widget.imagePath,
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
    required this.imagePath,
    required this.accent,
    required this.energy,
    required this.signal,
  });

  final Color accent;
  final double energy;
  final String? imagePath;
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
                color: accent.withValues(alpha: 0.16 + energy * 0.18),
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
              child: imagePath == null
                  ? Icon(CupertinoIcons.waveform, color: accent, size: 24)
                  : ClipOval(child: Image.asset(imagePath!, fit: BoxFit.cover)),
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
              accent.withValues(alpha: 0.13 * energy),
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
          ..strokeWidth = line == 2 ? 1.2 : 0.7
          ..color = accent.withValues(alpha: 0.08 + energy * 0.08),
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
    final accent = incoming ? signal.brandAccent : const Color(0xFF5478D8);
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
                    incoming
                        ? CupertinoIcons.sparkles
                        : CupertinoIcons.waveform,
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
                      Icon(CupertinoIcons.play_fill, size: 10, color: accent),
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
                        CupertinoIcons.exclamationmark_circle_fill,
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
