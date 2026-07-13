import 'dart:async';
import 'dart:math' as math;
import 'dart:ui';

import 'package:flutter/cupertino.dart';
import 'package:flutter/services.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:go_router/go_router.dart';

import '../data/mobile_data_controller.dart';
import '../data/workspace_chat_controller.dart';
import '../giz_ui/giz_ui.dart';

class WorkspaceConversationDock extends StatefulWidget {
  const WorkspaceConversationDock({
    super.key,
    required this.workspaceName,
    this.compact = false,
  });

  final bool compact;
  final String workspaceName;

  @override
  State<WorkspaceConversationDock> createState() =>
      _WorkspaceConversationDockState();
}

class _WorkspaceConversationDockState extends State<WorkspaceConversationDock> {
  bool _activating = false;
  Object? _error;

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    if (data.activeWorkspaceName == widget.workspaceName) {
      return GlobalConversationControl(compact: widget.compact);
    }
    final activeName = data.activeWorkspaceName;
    final activeTitle = activeName == null
        ? 'No active workspace'
        : data.workspace(activeName).title;
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        CupertinoButton(
          padding: EdgeInsets.zero,
          onPressed: _activating ? null : _activate,
          child: AnimatedContainer(
            duration: const Duration(milliseconds: 180),
            width: 54,
            height: 54,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: dark ? const Color(0xE8242D29) : const Color(0xEE111916),
              border: Border.all(
                color: dark ? const Color(0x42FFFFFF) : const Color(0x1AFFFFFF),
              ),
              boxShadow: const [
                BoxShadow(
                  color: Color(0x29001812),
                  blurRadius: 18,
                  offset: Offset(0, 8),
                ),
              ],
            ),
            child: _activating
                ? const CupertinoActivityIndicator(color: CupertinoColors.white)
                : const Icon(
                    CupertinoIcons.bolt_fill,
                    color: CupertinoColors.white,
                    size: 20,
                  ),
          ),
        ),
        const SizedBox(height: 7),
        Text(
          'Make Active',
          style: GizText.label.copyWith(
            color: dark ? const Color(0xD9FFFFFF) : GizColors.secondaryInk,
            fontSize: 9,
          ),
        ),
        Text(
          'Current: $activeTitle',
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: GizText.label.copyWith(
            color: dark ? const Color(0x8FFFFFFF) : const Color(0x8A52605B),
            fontSize: 8,
          ),
        ),
        if (_error != null) ...[
          const SizedBox(height: 6),
          Text(
            _error.toString(),
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: GizText.label.copyWith(
              color: CupertinoColors.systemRed.resolveFrom(context),
            ),
          ),
        ],
      ],
    );
  }

  Future<void> _activate() async {
    setState(() {
      _activating = true;
      _error = null;
    });
    try {
      await MobileDataScope.watch(
        context,
      ).activateWorkspaceChat(widget.workspaceName);
    } catch (error) {
      _error = error;
    } finally {
      if (mounted) setState(() => _activating = false);
    }
  }
}

class GlobalConversationControl extends StatefulWidget {
  const GlobalConversationControl({super.key, this.compact = false});

  final bool compact;

  @override
  State<GlobalConversationControl> createState() =>
      _GlobalConversationControlState();
}

class _GlobalConversationControlState extends State<GlobalConversationControl>
    with SingleTickerProviderStateMixin {
  late final AnimationController _motion = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 2400),
  );

  @override
  void dispose() {
    _motion.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final chat = data.activeWorkspaceChat;
    final workspaceName = data.activeWorkspaceName;
    final workspace = workspaceName == null
        ? null
        : data.workspace(workspaceName);
    final mode = _effectiveMode(data.activeInputMode);
    final enabled = chat?.canRecord ?? false;
    final animate =
        (chat?.recording ?? false) || (chat?.outputLevel ?? 0) > 0.01;
    if (animate && !_motion.isAnimating) {
      _motion.repeat();
    } else if (!animate && _motion.isAnimating) {
      _motion.stop();
    }
    final title = workspace?.title ?? 'No active workspace';
    final status = _statusLabel(data, chat, mode);
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    final accent = dark ? const Color(0xFF8DFFD0) : const Color(0xFF087F68);

    final button = _VoiceOrb(
      animation: _motion,
      size: widget.compact ? 58 : 60,
      enabled: enabled,
      active: chat?.recording ?? false,
      realtime: mode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME,
      inputLevel: chat?.inputLevel ?? 0,
      outputLevel: chat?.outputLevel ?? 0,
      accent: accent,
      onTap: workspaceName == null
          ? null
          : () => _showConversationPopover(context, data),
      onLongPressStart: enabled
          ? () => _handleLongPressStart(chat!, mode)
          : null,
      onLongPressEnd:
          enabled && mode != WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME
          ? () => unawaited(chat!.finishInput())
          : null,
    );

    if (widget.compact) {
      return Semantics(
        label: '$title, $status',
        button: true,
        child: SizedBox.square(dimension: 74, child: button),
      );
    }

    return Semantics(
      label: '$title, $status',
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          button,
          const SizedBox(height: 8),
          Text(
            status,
            style: GizText.label.copyWith(
              color: dark ? const Color(0xCCFFFFFF) : GizColors.secondaryInk,
              fontSize: 10,
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _handleLongPressStart(
    WorkspaceChatController chat,
    WorkspaceInputMode mode,
  ) async {
    await HapticFeedback.mediumImpact();
    if (mode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME &&
        chat.recording) {
      await chat.finishInput();
      return;
    }
    await chat.startInput();
  }
}

class _VoiceOrb extends StatelessWidget {
  const _VoiceOrb({
    required this.animation,
    required this.size,
    required this.enabled,
    required this.active,
    required this.realtime,
    required this.inputLevel,
    required this.outputLevel,
    required this.accent,
    required this.onTap,
    required this.onLongPressStart,
    required this.onLongPressEnd,
  });

  final Animation<double> animation;
  final double size;
  final bool enabled;
  final bool active;
  final bool realtime;
  final double inputLevel;
  final double outputLevel;
  final Color accent;
  final VoidCallback? onTap;
  final VoidCallback? onLongPressStart;
  final VoidCallback? onLongPressEnd;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTap: onTap,
      onLongPressStart: onLongPressStart == null
          ? null
          : (_) => onLongPressStart!(),
      onLongPressEnd: onLongPressEnd == null ? null : (_) => onLongPressEnd!(),
      child: AnimatedBuilder(
        animation: animation,
        builder: (context, child) => CustomPaint(
          painter: _VoiceRingPainter(
            phase: animation.value,
            inputLevel: inputLevel,
            outputLevel: outputLevel,
            active: active,
            accent: accent,
          ),
          child: child,
        ),
        child: Center(
          child: AnimatedContainer(
            duration: const Duration(milliseconds: 180),
            width: size,
            height: size,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: enabled ? accent : const Color(0xFFB6BFBB),
              boxShadow: enabled
                  ? [
                      BoxShadow(
                        color: accent.withValues(alpha: active ? 0.4 : 0.2),
                        blurRadius: active ? 18 : 10,
                      ),
                    ]
                  : null,
            ),
            child: Icon(
              active
                  ? (realtime
                        ? CupertinoIcons.stop_fill
                        : CupertinoIcons.waveform)
                  : CupertinoIcons.mic_fill,
              color: CupertinoColors.white,
              size: active ? size * 0.42 : size * 0.4,
            ),
          ),
        ),
      ),
    );
  }
}

class _VoiceRingPainter extends CustomPainter {
  const _VoiceRingPainter({
    required this.phase,
    required this.inputLevel,
    required this.outputLevel,
    required this.active,
    required this.accent,
  });

  final double phase;
  final double inputLevel;
  final double outputLevel;
  final bool active;
  final Color accent;

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    final base = math.min(size.width, size.height) * 0.38;
    final breath = active ? (math.sin(phase * math.pi * 2) + 1) * 0.8 : 0.0;
    _ring(
      canvas,
      center,
      base + 4 + breath + inputLevel * 12,
      accent.withValues(alpha: 0.22 + inputLevel * 0.38),
      1.8 + inputLevel * 2.4,
    );
    _ring(
      canvas,
      center,
      base + 8 + outputLevel * 15,
      const Color(0xFF5F8CFF).withValues(alpha: 0.16 + outputLevel * 0.4),
      1.4 + outputLevel * 2.8,
    );
  }

  void _ring(
    Canvas canvas,
    Offset center,
    double radius,
    Color color,
    double width,
  ) {
    canvas.drawCircle(
      center,
      radius,
      Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = width
        ..color = color,
    );
  }

  @override
  bool shouldRepaint(_VoiceRingPainter oldDelegate) =>
      oldDelegate.phase != phase ||
      oldDelegate.inputLevel != inputLevel ||
      oldDelegate.outputLevel != outputLevel ||
      oldDelegate.active != active ||
      oldDelegate.accent != accent;
}

Future<void> _showConversationPopover(
  BuildContext context,
  MobileDataController data,
) async {
  await showCupertinoModalPopup<void>(
    context: context,
    barrierColor: const Color(0x24000000),
    builder: (context) => _ConversationPopover(data: data),
  );
}

class _ConversationPopover extends StatefulWidget {
  const _ConversationPopover({required this.data});

  final MobileDataController data;

  @override
  State<_ConversationPopover> createState() => _ConversationPopoverState();
}

class _ConversationPopoverState extends State<_ConversationPopover> {
  bool _switching = false;
  Object? _error;

  @override
  Widget build(BuildContext context) {
    final data = widget.data;
    final workspaceName = data.activeWorkspaceName;
    final workspace = workspaceName == null
        ? null
        : data.workspace(workspaceName);
    final chat = data.activeWorkspaceChat;
    final mode = _effectiveMode(data.activeInputMode);
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    return SafeArea(
      minimum: const EdgeInsets.fromLTRB(16, 16, 16, 18),
      child: Align(
        alignment: Alignment.bottomCenter,
        child: ClipRRect(
          borderRadius: BorderRadius.circular(24),
          child: BackdropFilter(
            filter: ImageFilter.blur(sigmaX: 26, sigmaY: 26),
            child: Container(
              width: math.min(MediaQuery.sizeOf(context).width - 32, 390),
              padding: const EdgeInsets.all(18),
              decoration: BoxDecoration(
                color: dark ? const Color(0xE8232B28) : const Color(0xEFF9FCFA),
                borderRadius: BorderRadius.circular(24),
                border: Border.all(
                  color: dark
                      ? const Color(0x30FFFFFF)
                      : const Color(0x22001913),
                ),
              ),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              workspace?.title ?? 'No active workspace',
                              style: GizText.sectionTitle.copyWith(
                                color: dark
                                    ? CupertinoColors.white
                                    : GizColors.ink,
                              ),
                            ),
                            const SizedBox(height: 4),
                            Text(
                              _statusLabel(data, chat, mode),
                              style: GizText.label.copyWith(
                                color: dark
                                    ? const Color(0xAFFFFFFF)
                                    : GizColors.secondaryInk,
                              ),
                            ),
                          ],
                        ),
                      ),
                      if (chat?.recording ?? false)
                        const GizSignalPulse(size: 28),
                    ],
                  ),
                  const SizedBox(height: 18),
                  CupertinoSlidingSegmentedControl<WorkspaceInputMode>(
                    groupValue: mode,
                    children: const {
                      WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK:
                          Padding(
                            padding: EdgeInsets.symmetric(horizontal: 8),
                            child: Text('Push to Talk'),
                          ),
                      WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME: Padding(
                        padding: EdgeInsets.symmetric(horizontal: 8),
                        child: Text('Realtime'),
                      ),
                    },
                    onValueChanged: (value) {
                      if (!_switching && value != null) {
                        unawaited(_setMode(value));
                      }
                    },
                  ),
                  if (_switching) ...[
                    const SizedBox(height: 12),
                    const Center(child: CupertinoActivityIndicator()),
                  ],
                  if (_error != null) ...[
                    const SizedBox(height: 12),
                    Text(
                      _error.toString(),
                      style: GizText.body.copyWith(
                        color: CupertinoColors.systemRed.resolveFrom(context),
                      ),
                    ),
                  ],
                  const SizedBox(height: 14),
                  SizedBox(
                    width: double.infinity,
                    child: CupertinoButton(
                      color: dark ? const Color(0xFF40514B) : GizColors.ink,
                      borderRadius: BorderRadius.circular(16),
                      onPressed: workspaceName == null
                          ? null
                          : () => _openWorkspace(workspaceName),
                      child: const Row(
                        mainAxisAlignment: MainAxisAlignment.center,
                        children: [
                          Text('Open workspace'),
                          SizedBox(width: 8),
                          Icon(CupertinoIcons.arrow_up_right, size: 17),
                        ],
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }

  Future<void> _setMode(WorkspaceInputMode mode) async {
    setState(() {
      _switching = true;
      _error = null;
    });
    try {
      await widget.data.setActiveInputMode(mode);
    } catch (error) {
      _error = error;
    } finally {
      if (mounted) setState(() => _switching = false);
    }
  }

  Future<void> _openWorkspace(String workspaceName) async {
    final route = await widget.data.routeForWorkspace(workspaceName);
    if (!mounted) return;
    final router = GoRouter.of(context);
    Navigator.pop(context);
    router.push(route);
  }
}

WorkspaceInputMode _effectiveMode(WorkspaceInputMode mode) =>
    mode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME
    ? mode
    : WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK;

String _statusLabel(
  MobileDataController data,
  WorkspaceChatController? chat,
  WorkspaceInputMode mode,
) {
  if (data.connectionState == MobileConnectionState.connecting) {
    return 'CONNECTING';
  }
  if (chat == null) return 'NO ACTIVE CONVERSATION';
  if (chat.recording) {
    return mode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME
        ? 'REALTIME LIVE'
        : 'LISTENING';
  }
  if (chat.outputLevel > 0.02) return 'SPEAKING';
  return mode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME
      ? 'REALTIME READY'
      : 'HOLD TO TALK';
}
