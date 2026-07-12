import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/cupertino.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:go_router/go_router.dart';

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
          if (driver == WorkflowDriverKind.unsupported) return false;
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
    return ClipRRect(
      borderRadius: BorderRadius.circular(8),
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
      navigationBar: CupertinoNavigationBar(
        middle: Text(driver.label, style: GizText.title),
        border: null,
        transitionBetweenRoutes: false,
      ),
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
      padding: const EdgeInsets.only(bottom: 24),
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

        return (driver == WorkflowDriverKind.chatroom
                ? _ChatroomWorkspaceListTile(
                    workspace: workspace,
                    metadata: metadata,
                    onPressed: onPressed,
                  )
                : WorkspaceListTile(workspace: workspace, onPressed: onPressed))
            .animate(delay: (index * 45).ms)
            .fadeIn(duration: 280.ms)
            .slideY(begin: 0.05, end: 0, curve: Curves.easeOutCubic);
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
      leading: Container(
        width: 50,
        height: 50,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: isDirect ? const Color(0xFFD9F2EA) : const Color(0xFFDDE8FF),
          shape: isDirect ? BoxShape.circle : BoxShape.rectangle,
          borderRadius: isDirect ? null : BorderRadius.circular(8),
        ),
        child: Icon(
          isDirect ? CupertinoIcons.person_fill : CupertinoIcons.person_2_fill,
          color: isDirect ? const Color(0xFF17795B) : const Color(0xFF315E9D),
          size: 22,
        ),
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

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (_chat != null) return;
    final data = MobileDataScope.watch(context);
    if (data.connectionState == MobileConnectionState.connecting) return;
    final chat = data.createWorkspaceChat(widget.workspaceName);
    _chat = chat;
    chat.addListener(_handleChatChanged);
    unawaited(chat.start());
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
    _chat?.removeListener(_handleChatChanged);
    _chat?.dispose();
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
      navigationBar: CupertinoNavigationBar(
        backgroundColor: signal.chrome,
        brightness: signal.brightness,
        middle: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              chatroomMetadata?.title ?? workspace.title,
              style: GizText.title.copyWith(color: signal.text),
            ),
            Text(
              '${isDirectChat ? 'Direct chat' : workflow.driver.label}'
              '  /  ${_connectionLabel(chat?.state)}',
              style: GizText.label.copyWith(color: signal.muted, fontSize: 9),
            ),
          ],
        ),
        trailing: const GizSignalPulse(size: 24),
        border: null,
        transitionBetweenRoutes: false,
      ),
      child: SafeArea(
        top: false,
        child: Padding(
          padding: EdgeInsets.only(
            top:
                MediaQuery.paddingOf(context).top +
                kMinInteractiveDimensionCupertino,
          ),
          child: Column(
            children: [
              Expanded(
                child: Stack(
                  children: [
                    Positioned.fill(
                      child: _WorkspaceMessageList(
                        controller: _scrollController,
                        messages: messages,
                        state: chat?.state ?? WorkspaceChatState.loading,
                        signal: signal,
                        error: chat?.lastError,
                      ),
                    ),
                    Positioned(
                      top: 4,
                      left: 0,
                      right: 0,
                      child: IgnorePointer(
                        child: _AgentSignalStage(
                          imagePath: isDirectChat
                              ? null
                              : workflow.driver.imagePath,
                          state: chat?.state ?? WorkspaceChatState.loading,
                          recording: chat?.recording ?? false,
                          accent: accent,
                          signal: signal,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
              _PushToTalkControl(
                chat: chat,
                accent: signal.actionAccent,
                signal: signal,
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

Color _driverAccent(
  WorkflowDriverKind driver,
  Brightness brightness,
) => switch ((driver, brightness)) {
  (WorkflowDriverKind.astTranslate, _) => _workspaceVoiceAccent(brightness),
  (WorkflowDriverKind.doubaoRealtime, Brightness.light) => const Color(
    0xFFE66843,
  ),
  (WorkflowDriverKind.flowcraft, Brightness.light) => const Color(0xFF1687B5),
  (WorkflowDriverKind.chatroom, Brightness.light) => const Color(0xFFC68B11),
  (WorkflowDriverKind.doubaoRealtime, Brightness.dark) => const Color(
    0xFFFF8B6A,
  ),
  (WorkflowDriverKind.flowcraft, Brightness.dark) => const Color(0xFF70D8FF),
  (WorkflowDriverKind.chatroom, Brightness.dark) => const Color(0xFFFFD166),
  (WorkflowDriverKind.unsupported, _) => GizColors.accent,
};

Color _workspaceVoiceAccent(Brightness brightness) =>
    brightness == Brightness.dark
    ? const Color(0xFF8CFFB5)
    : const Color(0xFF2AAE72);

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
    canvas: Color(0xFFF1F5F1),
    chrome: Color(0xF2F1F5F1),
    panel: Color(0xFFFFFFFF),
    panelStrong: Color(0xFFE4EBE6),
    line: Color(0xFFCBD6CF),
    muted: Color(0xFF627169),
    text: Color(0xFF101713),
    onAccent: Color(0xFF07110C),
    actionAccent: GizColors.accent,
    brandAccent: Color(0xFF668700),
    outgoingFill: GizColors.ink,
    outgoingText: GizColors.surface,
  );

  static const dark = _SignalPalette(
    brightness: Brightness.dark,
    canvas: Color(0xFF080B0A),
    chrome: Color(0xED080B0A),
    panel: Color(0xFF121715),
    panelStrong: Color(0xFF19201D),
    line: Color(0xFF2A332F),
    muted: Color(0xFF8E9B95),
    text: Color(0xFFF4F8F5),
    onAccent: Color(0xFF07110C),
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

class _AgentSignalStage extends StatefulWidget {
  const _AgentSignalStage({
    required this.imagePath,
    required this.state,
    required this.recording,
    required this.accent,
    required this.signal,
  });

  final Color accent;
  final String? imagePath;
  final bool recording;
  final _SignalPalette signal;
  final WorkspaceChatState state;

  @override
  State<_AgentSignalStage> createState() => _AgentSignalStageState();
}

class _AgentSignalStageState extends State<_AgentSignalStage>
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
    return SizedBox(
      height: 104,
      width: double.infinity,
      child: AnimatedBuilder(
        animation: _controller,
        builder: (context, child) {
          final energy = widget.recording
              ? 0.78 + math.sin(_controller.value * math.pi * 10) * 0.18
              : active
              ? 0.42 + math.sin(_controller.value * math.pi * 2) * 0.08
              : 0.18;
          return Stack(
            alignment: Alignment.center,
            children: [
              Positioned.fill(
                child: CustomPaint(
                  painter: _SignalFieldPainter(
                    progress: _controller.value,
                    accent: widget.accent,
                    energy: energy,
                  ),
                ),
              ),
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
          );
        },
      ),
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
  });

  final ScrollController controller;
  final Object? error;
  final List<WorkspaceChatMessage> messages;
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
      return Center(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 36),
          child: Text(
            unavailable
                ? 'This conversation is unavailable right now.'
                : 'The channel is clear.\nHold the signal to speak.',
            textAlign: TextAlign.center,
            style: GizText.body.copyWith(color: signal.muted, height: 1.65),
          ),
        ),
      );
    }
    return ListView.separated(
      controller: controller,
      reverse: true,
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 14),
      itemCount: messages.length + (error == null ? 0 : 1),
      separatorBuilder: (_, _) => const SizedBox(height: 10),
      itemBuilder: (context, index) {
        if (index == messages.length) {
          return Text(
            'Live updates paused. Showing saved messages.',
            textAlign: TextAlign.center,
            style: GizText.label.copyWith(color: signal.muted),
          );
        }
        final message = messages[messages.length - 1 - index];
        return _WorkspaceSignalMessage(message: message, signal: signal);
      },
    );
  }
}

class _WorkspaceSignalMessage extends StatelessWidget {
  const _WorkspaceSignalMessage({required this.message, required this.signal});

  final WorkspaceChatMessage message;
  final _SignalPalette signal;

  @override
  Widget build(BuildContext context) {
    final incoming = message.incoming;
    final width = MediaQuery.sizeOf(context).width;
    return Align(
      alignment: incoming ? Alignment.centerLeft : Alignment.centerRight,
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: width * 0.82),
        child: Container(
          padding: const EdgeInsets.fromLTRB(14, 11, 14, 12),
          decoration: BoxDecoration(
            color: incoming ? signal.panel : signal.outgoingFill,
            borderRadius: BorderRadius.only(
              topLeft: const Radius.circular(14),
              topRight: const Radius.circular(14),
              bottomLeft: Radius.circular(incoming ? 4 : 14),
              bottomRight: Radius.circular(incoming ? 14 : 4),
            ),
            border: Border.all(
              color: incoming ? signal.line : signal.outgoingFill,
            ),
            boxShadow: signal.brightness == Brightness.light
                ? [
                    const BoxShadow(
                      color: Color(0x0F111916),
                      blurRadius: 14,
                      offset: Offset(0, 5),
                    ),
                  ]
                : null,
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(
                    incoming
                        ? CupertinoIcons.sparkles
                        : CupertinoIcons.waveform,
                    size: 13,
                    color: incoming ? signal.brandAccent : signal.outgoingText,
                  ),
                  const SizedBox(width: 7),
                  Text(
                    incoming ? 'AGENT TRANSMISSION' : 'YOUR VOICE',
                    style: GizText.label.copyWith(
                      color: incoming
                          ? signal.brandAccent
                          : signal.outgoingText.withValues(alpha: 0.68),
                      fontSize: 8,
                    ),
                  ),
                  if (!incoming) ...[
                    const SizedBox(width: 12),
                    _MiniWaveform(color: signal.outgoingText),
                  ],
                ],
              ),
              const SizedBox(height: 8),
              Text(
                message.text.isEmpty ? '...' : message.text,
                style: GizText.body.copyWith(
                  color: incoming ? signal.text : signal.outgoingText,
                  fontSize: incoming ? 15 : 14,
                  height: 1.5,
                  fontWeight: incoming ? FontWeight.w500 : FontWeight.w700,
                ),
              ),
              if (message.state == WorkspaceMessageState.streaming ||
                  message.state == WorkspaceMessageState.failed) ...[
                const SizedBox(height: 8),
                Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (message.state == WorkspaceMessageState.streaming)
                      SizedBox(
                        width: 10,
                        height: 10,
                        child: CupertinoActivityIndicator(
                          radius: 5,
                          color: incoming
                              ? signal.brandAccent
                              : signal.outgoingText,
                        ),
                      )
                    else
                      Icon(
                        CupertinoIcons.exclamationmark_circle_fill,
                        size: 11,
                        color: incoming
                            ? signal.brandAccent
                            : signal.outgoingText,
                      ),
                    const SizedBox(width: 5),
                    Text(
                      message.state == WorkspaceMessageState.failed
                          ? 'SIGNAL INTERRUPTED'
                          : 'STREAMING',
                      style: GizText.label.copyWith(
                        color: incoming
                            ? signal.muted
                            : signal.outgoingText.withValues(alpha: 0.62),
                        fontSize: 8,
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

class _MiniWaveform extends StatelessWidget {
  const _MiniWaveform({required this.color});

  final Color color;

  @override
  Widget build(BuildContext context) {
    const heights = [5.0, 10.0, 7.0, 13.0, 8.0, 11.0, 5.0];
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        for (final height in heights)
          Container(
            width: 2,
            height: height,
            margin: const EdgeInsets.only(left: 2),
            decoration: BoxDecoration(
              color: color.withValues(alpha: 0.58),
              borderRadius: BorderRadius.circular(2),
            ),
          ),
      ],
    );
  }
}

class _PushToTalkControl extends StatefulWidget {
  const _PushToTalkControl({
    required this.chat,
    required this.accent,
    required this.signal,
  });

  final WorkspaceChatController? chat;
  final Color accent;
  final _SignalPalette signal;

  @override
  State<_PushToTalkControl> createState() => _PushToTalkControlState();
}

class _PushToTalkControlState extends State<_PushToTalkControl>
    with SingleTickerProviderStateMixin {
  late final AnimationController _energy = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1500),
  )..repeat();

  @override
  void dispose() {
    _energy.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final controller = widget.chat;
    final enabled = controller?.canRecord ?? false;
    final recording = controller?.recording ?? false;
    final preparing = controller?.startingInput ?? false;
    final label = recording
        ? 'RELEASE TO TRANSMIT'
        : preparing
        ? 'OPENING MICROPHONE'
        : enabled
        ? 'HOLD TO SPEAK'
        : 'VOICE LINK UNAVAILABLE';
    return SizedBox(
      height: 132,
      width: double.infinity,
      child: AnimatedBuilder(
        animation: _energy,
        builder: (context, child) {
          return CustomPaint(
            painter: _VoiceDockPainter(
              progress: _energy.value,
              accent: widget.accent,
              active: recording,
              enabled: enabled,
              signal: widget.signal,
            ),
            child: child,
          );
        },
        child: Column(
          mainAxisAlignment: MainAxisAlignment.end,
          children: [
            Listener(
              onPointerDown: enabled
                  ? (_) => unawaited(controller!.startInput())
                  : null,
              onPointerUp: enabled
                  ? (_) => unawaited(controller!.finishInput())
                  : null,
              onPointerCancel: enabled
                  ? (_) => unawaited(
                      controller!.finishInput(error: 'recording canceled'),
                    )
                  : null,
              child: Semantics(
                button: true,
                enabled: enabled,
                label: label,
                child: AnimatedScale(
                  scale: recording ? 0.92 : 1,
                  duration: const Duration(milliseconds: 140),
                  curve: Curves.easeOutCubic,
                  child: AnimatedContainer(
                    duration: const Duration(milliseconds: 180),
                    width: 70,
                    height: 70,
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      color: enabled
                          ? widget.accent
                          : widget.signal.panelStrong,
                      border: Border.all(
                        color: enabled ? widget.accent : widget.signal.line,
                        width: 1.5,
                      ),
                      boxShadow: enabled
                          ? [
                              BoxShadow(
                                color: widget.accent.withValues(
                                  alpha: recording ? 0.48 : 0.2,
                                ),
                                blurRadius: recording ? 34 : 20,
                                spreadRadius: recording ? 8 : 2,
                              ),
                            ]
                          : null,
                    ),
                    child: Icon(
                      recording
                          ? CupertinoIcons.waveform
                          : CupertinoIcons.mic_fill,
                      size: 26,
                      color: enabled
                          ? widget.signal.onAccent
                          : widget.signal.muted,
                    ),
                  ),
                ),
              ),
            ),
            const SizedBox(height: 10),
            Text(
              label,
              style: GizText.label.copyWith(
                color: recording ? widget.accent : widget.signal.muted,
                fontSize: 9,
              ),
            ),
            const SizedBox(height: 9),
          ],
        ),
      ),
    );
  }
}

class _VoiceDockPainter extends CustomPainter {
  const _VoiceDockPainter({
    required this.progress,
    required this.accent,
    required this.active,
    required this.enabled,
    required this.signal,
  });

  final Color accent;
  final bool active;
  final bool enabled;
  final double progress;
  final _SignalPalette signal;

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height + 4);
    final radius = size.width * (active ? 0.62 : 0.5);
    final field = Paint()
      ..shader = RadialGradient(
        colors: [
          (enabled ? accent : signal.panelStrong).withValues(
            alpha: active ? 0.28 : 0.13,
          ),
          signal.canvas.withValues(alpha: 0),
        ],
      ).createShader(Rect.fromCircle(center: center, radius: radius));
    canvas.drawCircle(center, radius, field);

    for (var ring = 0; ring < 3; ring++) {
      final pulse = (progress + ring * 0.3) % 1;
      final ringRadius = 54 + pulse * (active ? 92 : 55);
      canvas.drawArc(
        Rect.fromCircle(center: center, radius: ringRadius),
        math.pi,
        math.pi,
        false,
        Paint()
          ..style = PaintingStyle.stroke
          ..strokeWidth = 1
          ..color = (enabled ? accent : signal.line).withValues(
            alpha: (1 - pulse) * (active ? 0.34 : 0.12),
          ),
      );
    }
  }

  @override
  bool shouldRepaint(_VoiceDockPainter oldDelegate) =>
      oldDelegate.progress != progress ||
      oldDelegate.active != active ||
      oldDelegate.enabled != enabled ||
      oldDelegate.accent != accent ||
      oldDelegate.signal != signal;
}

class ChatroomWorkspacePage extends StatelessWidget {
  const ChatroomWorkspacePage({super.key, required this.workspaceName});

  final String workspaceName;

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final workspace = data.workspace(workspaceName);
    final workflow = data.workflow(workspace.workflowName);
    final metadata = data.chatroomWorkspace(workspaceName);
    final kind = metadata?.kind ?? workspace.chatroomKind;
    if (kind == ChatroomWorkspaceKind.direct) {
      return WorkspaceChatPage(workspaceName: workspaceName);
    }
    return GroupChatPage(
      room: ChatroomCard(
        id: workspace.name,
        name: metadata?.title ?? workspace.title,
        subtitle: _chatroomSubtitle(metadata, workflow.title),
        memberCount: 0,
      ),
    );
  }
}

String _chatroomSubtitle(ChatroomWorkspaceMetadata? metadata, String fallback) {
  if (metadata == null) return fallback;
  if (metadata.kind == ChatroomWorkspaceKind.direct) return 'Direct chat';
  return metadata.description.trim().isEmpty
      ? 'Group chat'
      : metadata.description;
}

class GroupChatPage extends StatefulWidget {
  const GroupChatPage({super.key, required this.room});

  final ChatroomCard room;

  @override
  State<GroupChatPage> createState() => _GroupChatPageState();
}

class _GroupChatPageState extends State<GroupChatPage> {
  final _controller = TextEditingController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return CupertinoPageScaffold(
      navigationBar: CupertinoNavigationBar(
        middle: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(widget.room.name, style: GizText.title),
            Text(
              widget.room.memberCount > 0
                  ? '${widget.room.memberCount} members'
                  : widget.room.subtitle,
              style: GizText.label.copyWith(color: GizColors.secondaryInk),
            ),
          ],
        ),
        border: null,
        transitionBetweenRoutes: false,
      ),
      child: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(16, 20, 16, 18),
                children: const [
                  _ChatBubble(
                    text: 'Avery: The new workflow is live.',
                    incoming: true,
                    color: GizColors.blue,
                  ),
                  SizedBox(height: 10),
                  _ChatBubble(
                    text: 'I will test it from mobile.',
                    incoming: false,
                    color: GizColors.ink,
                  ),
                ],
              ),
            ),
            _Composer(controller: _controller),
          ],
        ),
      ),
    );
  }
}

class _ChatBubble extends StatelessWidget {
  const _ChatBubble({
    required this.text,
    required this.incoming,
    required this.color,
  });

  final String text;
  final bool incoming;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Align(
      alignment: incoming ? Alignment.centerLeft : Alignment.centerRight,
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 290),
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: incoming ? GizColors.surface : color,
            borderRadius: BorderRadius.circular(8),
            border: incoming ? Border.all(color: GizColors.separator) : null,
          ),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 11),
            child: Text(
              text,
              style: GizText.body.copyWith(
                color: incoming ? GizColors.ink : GizColors.surface,
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _Composer extends StatelessWidget {
  const _Composer({required this.controller});

  final TextEditingController controller;

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: const BoxDecoration(
        color: Color(0xFAF4F5F1),
        border: Border(top: BorderSide(color: GizColors.separator)),
      ),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(12, 9, 10, 9),
        child: Row(
          children: [
            Expanded(
              child: CupertinoTextField(
                controller: controller,
                minLines: 1,
                maxLines: 4,
                placeholder: 'Message',
                padding: const EdgeInsets.symmetric(
                  horizontal: 14,
                  vertical: 11,
                ),
                style: GizText.body,
                textInputAction: TextInputAction.send,
                onSubmitted: (_) => controller.clear(),
                decoration: BoxDecoration(
                  color: GizColors.surface,
                  borderRadius: BorderRadius.circular(8),
                  border: Border.all(color: GizColors.separator),
                ),
              ),
            ),
            const SizedBox(width: 8),
            CupertinoButton(
              minimumSize: const Size.square(42),
              padding: EdgeInsets.zero,
              color: GizColors.ink,
              borderRadius: BorderRadius.circular(21),
              onPressed: controller.clear,
              child: const Icon(
                CupertinoIcons.arrow_up,
                size: 20,
                color: GizColors.surface,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
