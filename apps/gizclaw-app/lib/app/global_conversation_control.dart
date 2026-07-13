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
import '../prototype/prototype_data.dart';
import '../prototype/prototype_models.dart';

class GlobalConversationOverlay extends StatelessWidget {
  const GlobalConversationOverlay({
    super.key,
    required this.child,
    required this.location,
    this.navigationShell,
  });

  final Widget child;
  final Uri location;
  final StatefulNavigationShell? navigationShell;

  @override
  Widget build(BuildContext context) {
    return Stack(
      fit: StackFit.expand,
      children: [
        child,
        Positioned(
          left: 0,
          right: 0,
          bottom: 0,
          child: _GlobalBottomDock(
            location: location,
            navigationShell: navigationShell,
          ),
        ),
      ],
    );
  }
}

class _GlobalBottomDock extends StatelessWidget {
  const _GlobalBottomDock({required this.location, this.navigationShell});

  final Uri location;
  final StatefulNavigationShell? navigationShell;

  static const _rootPaths = {'/browse', '/chats', '/friends', '/pet', '/me'};

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    final shell = navigationShell;
    final showTabs = shell != null && _rootPaths.contains(location.path);
    return SafeArea(
      top: false,
      minimum: const EdgeInsets.fromLTRB(12, 10, 12, 8),
      child: Container(
        height: 76,
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(38),
          boxShadow: [
            const BoxShadow(
              color: Color(0x2E61D7FF),
              blurRadius: 22,
              spreadRadius: -3,
              offset: Offset(-18, 7),
            ),
            const BoxShadow(
              color: Color(0x266F75FF),
              blurRadius: 24,
              spreadRadius: -4,
              offset: Offset(-5, 9),
            ),
            const BoxShadow(
              color: Color(0x2EEA6BDB),
              blurRadius: 24,
              spreadRadius: -4,
              offset: Offset(13, 8),
            ),
            const BoxShadow(
              color: Color(0x26FF9D66),
              blurRadius: 20,
              spreadRadius: -5,
              offset: Offset(24, 5),
            ),
            BoxShadow(
              color: dark ? const Color(0x80000000) : const Color(0x1F001812),
              blurRadius: 24,
              offset: const Offset(0, 10),
            ),
          ],
        ),
        child: ClipRRect(
          borderRadius: BorderRadius.circular(38),
          child: BackdropFilter(
            filter: ImageFilter.blur(sigmaX: 28, sigmaY: 28),
            child: Container(
              decoration: BoxDecoration(
                color: dark ? const Color(0xD91B211F) : const Color(0xE8FAFCFB),
                borderRadius: BorderRadius.circular(38),
                border: Border.all(
                  color: dark
                      ? const Color(0x3DFFFFFF)
                      : const Color(0x26FFFFFF),
                ),
              ),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.center,
                children: [
                  Expanded(
                    child: AnimatedSwitcher(
                      duration: const Duration(milliseconds: 260),
                      switchInCurve: Curves.easeOutCubic,
                      switchOutCurve: Curves.easeInCubic,
                      transitionBuilder: (child, animation) => FadeTransition(
                        opacity: animation,
                        child: SlideTransition(
                          position: Tween<Offset>(
                            begin: const Offset(0, 0.08),
                            end: Offset.zero,
                          ).animate(animation),
                          child: child,
                        ),
                      ),
                      child: showTabs
                          ? _PrimaryDockNavigation(
                              key: const ValueKey('primary-dock'),
                              navigationShell: shell,
                            )
                          : _ContextDockNavigation(
                              key: ValueKey(location.path),
                              location: location,
                            ),
                    ),
                  ),
                  Container(
                    width: 1,
                    height: 34,
                    color: dark
                        ? const Color(0x2EFFFFFF)
                        : const Color(0x14001913),
                  ),
                  const GlobalConversationControl(compact: true),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _PrimaryDockNavigation extends StatelessWidget {
  const _PrimaryDockNavigation({super.key, required this.navigationShell});

  final StatefulNavigationShell navigationShell;

  static const _items = [
    (CupertinoIcons.compass, CupertinoIcons.compass_fill, 'Browse'),
    (CupertinoIcons.chat_bubble_2, CupertinoIcons.chat_bubble_2_fill, 'Chats'),
    (CupertinoIcons.person_2, CupertinoIcons.person_2_fill, 'Friends'),
    (CupertinoIcons.sparkles, CupertinoIcons.sparkles, 'Pet'),
    (
      CupertinoIcons.person_crop_circle,
      CupertinoIcons.person_crop_circle_fill,
      'Me',
    ),
  ];

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    return SizedBox(
      height: 62,
      child: Row(
        children: List.generate(_items.length, (index) {
          final item = _items[index];
          final selected = navigationShell.currentIndex == index;
          final foreground = selected
              ? (dark ? CupertinoColors.white : GizColors.ink)
              : (dark ? const Color(0x8FFFFFFF) : GizColors.secondaryInk);
          return Expanded(
            child: CupertinoButton(
              padding: EdgeInsets.zero,
              onPressed: () =>
                  navigationShell.goBranch(index, initialLocation: selected),
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Icon(
                    selected ? item.$2 : item.$1,
                    size: 20,
                    color: foreground,
                  ),
                  const SizedBox(height: 3),
                  Text(
                    item.$3,
                    maxLines: 1,
                    style: GizText.label.copyWith(
                      color: foreground,
                      fontSize: 8,
                    ),
                  ),
                ],
              ),
            ),
          );
        }),
      ),
    );
  }
}

class _ContextDockNavigation extends StatelessWidget {
  const _ContextDockNavigation({super.key, required this.location});

  final Uri location;

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    final info = _dockContext(location, data);
    return SizedBox(
      height: 62,
      child: Row(
        children: [
          CupertinoButton(
            padding: EdgeInsets.zero,
            minimumSize: const Size(48, 48),
            onPressed: () {
              if (GoRouter.of(context).canPop()) {
                context.pop();
              } else {
                context.go(info.fallbackRoute);
              }
            },
            child: Icon(
              CupertinoIcons.chevron_left,
              size: 20,
              color: dark ? CupertinoColors.white : GizColors.ink,
            ),
          ),
          Container(
            width: 1,
            height: 28,
            color: dark ? const Color(0x26FFFFFF) : const Color(0x12001913),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  info.title,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: GizText.title.copyWith(
                    color: dark ? CupertinoColors.white : GizColors.ink,
                  ),
                ),
                const SizedBox(height: 2),
                Row(
                  children: [
                    if (info.active) ...[
                      Container(
                        width: 6,
                        height: 6,
                        decoration: const BoxDecoration(
                          shape: BoxShape.circle,
                          color: Color(0xFF20A67A),
                        ),
                      ),
                      const SizedBox(width: 6),
                    ],
                    Expanded(
                      child: Text(
                        info.subtitle,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: GizText.label.copyWith(
                          color: dark
                              ? const Color(0x99FFFFFF)
                              : GizColors.secondaryInk,
                          fontSize: 8,
                        ),
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
          if (info.workspaceName != null) ...[
            const SizedBox(width: 8),
            _WorkspaceActivationPill(
              active: info.active,
              workspaceName: info.workspaceName!,
            ),
            const SizedBox(width: 10),
          ],
        ],
      ),
    );
  }
}

class _WorkspaceActivationPill extends StatefulWidget {
  const _WorkspaceActivationPill({
    required this.active,
    required this.workspaceName,
  });

  final bool active;
  final String workspaceName;

  @override
  State<_WorkspaceActivationPill> createState() =>
      _WorkspaceActivationPillState();
}

class _WorkspaceActivationPillState extends State<_WorkspaceActivationPill> {
  bool _activating = false;

  Future<void> _activate() async {
    if (widget.active || _activating) return;
    setState(() => _activating = true);
    HapticFeedback.selectionClick();
    try {
      await MobileDataScope.watch(
        context,
      ).activateWorkspaceChat(widget.workspaceName);
    } catch (error) {
      if (!mounted) return;
      await showCupertinoDialog<void>(
        context: context,
        builder: (context) => CupertinoAlertDialog(
          title: const Text('Unable to activate'),
          content: Text('$error'),
          actions: [
            CupertinoDialogAction(
              onPressed: () => Navigator.of(context).pop(),
              child: const Text('OK'),
            ),
          ],
        ),
      );
    } finally {
      if (mounted) setState(() => _activating = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    final active = widget.active;
    final foreground = active
        ? (dark ? const Color(0xFF8DFFD0) : const Color(0xFF087F68))
        : (dark ? const Color(0xFFE1E8E5) : const Color(0xFF40504A));
    final fill = active
        ? (dark ? const Color(0x2426D49B) : const Color(0x1920A67A))
        : (dark ? const Color(0x14FFFFFF) : const Color(0x0D001913));
    final border = active
        ? foreground.withValues(alpha: 0.28)
        : foreground.withValues(alpha: 0.12);

    return CupertinoButton(
      padding: EdgeInsets.zero,
      minimumSize: const Size(0, 36),
      pressedOpacity: active ? 1 : 0.62,
      onPressed: active ? null : _activate,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 260),
        curve: Curves.easeOutCubic,
        height: 34,
        padding: const EdgeInsets.symmetric(horizontal: 10),
        decoration: BoxDecoration(
          color: fill,
          borderRadius: BorderRadius.circular(17),
          border: Border.all(color: border),
          boxShadow: active
              ? [
                  BoxShadow(
                    color: foreground.withValues(alpha: 0.13),
                    blurRadius: 12,
                    spreadRadius: 1,
                  ),
                ]
              : null,
        ),
        child: AnimatedSwitcher(
          duration: const Duration(milliseconds: 180),
          child: _activating
              ? CupertinoActivityIndicator(
                  key: const ValueKey('activating'),
                  radius: 7,
                  color: foreground,
                )
              : Row(
                  key: ValueKey(active),
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Container(
                      width: 7,
                      height: 7,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: active ? foreground : const Color(0x00000000),
                        border: active
                            ? null
                            : Border.all(color: foreground, width: 1.3),
                        boxShadow: active
                            ? [
                                BoxShadow(
                                  color: foreground.withValues(alpha: 0.48),
                                  blurRadius: 7,
                                ),
                              ]
                            : null,
                      ),
                    ),
                    const SizedBox(width: 6),
                    Text(
                      active ? 'ACTIVE' : 'ACTIVATE',
                      style: GizText.label.copyWith(
                        color: foreground,
                        fontSize: 7,
                      ),
                    ),
                  ],
                ),
        ),
      ),
    );
  }
}

class _DockContext {
  const _DockContext({
    required this.title,
    required this.subtitle,
    required this.fallbackRoute,
    this.active = false,
    this.workspaceName,
  });

  final bool active;
  final String fallbackRoute;
  final String subtitle;
  final String title;
  final String? workspaceName;
}

_DockContext _dockContext(Uri location, MobileDataController data) {
  final segments = location.pathSegments
      .map(Uri.decodeComponent)
      .toList(growable: false);
  if (segments.length >= 4 && segments[0] == 'chats') {
    final driver = WorkflowDriverKind.fromRouteKey(segments[2]);
    final workspaceName = segments[3];
    final active = data.activeWorkspaceName == workspaceName;
    final chatroom = data.chatroomWorkspace(workspaceName);
    final contextLabel = chatroom == null
        ? driver.label
        : chatroom.kind == ChatroomWorkspaceKind.direct
        ? 'Direct chat'
        : 'Group chat';
    final mode =
        data.activeInputMode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME
        ? 'Realtime'
        : 'Push to Talk';
    return _DockContext(
      title: chatroom?.title ?? data.workspace(workspaceName).title,
      subtitle: active
          ? '$contextLabel  /  $mode'
          : '$contextLabel  /  Viewing',
      fallbackRoute: '/chats/drivers/${driver.routeKey}',
      active: active,
      workspaceName: workspaceName,
    );
  }
  if (segments.length >= 3 && segments[0] == 'chats') {
    final driver = WorkflowDriverKind.fromRouteKey(segments[2]);
    return _DockContext(
      title: driver.label,
      subtitle: 'Available workspaces',
      fallbackRoute: '/chats',
    );
  }
  if (segments.length >= 2 && segments[0] == 'pet') {
    final pet = data.petRouteContext(segments[1]);
    final workspaceName = pet?.workspaceName;
    final active =
        workspaceName != null && data.activeWorkspaceName == workspaceName;
    return _DockContext(
      title: pet?.title ?? 'Pet companion',
      subtitle: active ? 'Pet  /  Connected' : 'Pet  /  Viewing',
      fallbackRoute: '/pet',
      active: active,
      workspaceName: workspaceName,
    );
  }
  if (segments.length >= 3 &&
      segments[0] == 'browse' &&
      segments[1] == 'workflows') {
    final workflow = data.workflow(segments[2]);
    return _DockContext(
      title: workflow.title,
      subtitle: '${workflow.category}  /  ${workflow.driverLabel}',
      fallbackRoute: '/browse/workflows',
    );
  }
  if (segments.length >= 3 &&
      segments[0] == 'browse' &&
      segments[1] == 'collections') {
    final collection = collectionById(segments[2]);
    return _DockContext(
      title: collection.title,
      subtitle: 'Curated collection',
      fallbackRoute: '/browse',
    );
  }
  if (location.path == '/browse/workflows') {
    return const _DockContext(
      title: 'All Workflows',
      subtitle: 'Browse every available workflow',
      fallbackRoute: '/browse',
    );
  }
  return const _DockContext(
    title: 'GizClaw',
    subtitle: 'Back to the previous page',
    fallbackRoute: '/browse',
  );
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
  WorkspaceChatController? _observedChat;
  late final AnimationController _motion = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 2400),
  );

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final chat = MobileDataScope.watch(context).activeWorkspaceChat;
    if (identical(chat, _observedChat)) return;
    _observedChat?.removeListener(_handleChatChanged);
    _observedChat = chat;
    chat?.addListener(_handleChatChanged);
  }

  void _handleChatChanged() {
    if (mounted) setState(() {});
  }

  @override
  void dispose() {
    _observedChat?.removeListener(_handleChatChanged);
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
        (chat?.startingInput ?? false) ||
        (chat?.recording ?? false) ||
        (chat?.outputLevel ?? 0) > 0.01;
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
      size: widget.compact ? 54 : 58,
      enabled: enabled,
      active: chat?.recording ?? false,
      preparing: chat?.startingInput ?? false,
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
    required this.preparing,
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
  final bool preparing;
  final bool realtime;
  final double inputLevel;
  final double outputLevel;
  final Color accent;
  final VoidCallback? onTap;
  final VoidCallback? onLongPressStart;
  final VoidCallback? onLongPressEnd;

  @override
  Widget build(BuildContext context) {
    final speaking = outputLevel > 0.02;
    final engaged = active || preparing;
    final energy = speaking ? const Color(0xFF4F7CFF) : accent;
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTap: onTap,
      onLongPressStart: onLongPressStart == null
          ? null
          : (_) => onLongPressStart!(),
      onLongPressEnd: onLongPressEnd == null ? null : (_) => onLongPressEnd!(),
      child: AnimatedBuilder(
        animation: animation,
        builder: (context, child) {
          final phase = animation.value;
          return AnimatedScale(
            scale: engaged ? 0.96 : 1,
            duration: const Duration(milliseconds: 180),
            curve: Curves.easeOutBack,
            child: CustomPaint(
              painter: _VoiceEnergyPainter(
                phase: phase,
                inputLevel: inputLevel,
                outputLevel: outputLevel,
                engaged: engaged,
                energy: energy,
              ),
              child: Center(
                child: _VoiceEnergySurface(
                  size: size,
                  enabled: enabled,
                  engaged: engaged,
                  realtime: realtime,
                  speaking: speaking,
                  energy: energy,
                ),
              ),
            ),
          );
        },
      ),
    );
  }
}

class _VoiceEnergySurface extends StatelessWidget {
  const _VoiceEnergySurface({
    required this.size,
    required this.enabled,
    required this.engaged,
    required this.realtime,
    required this.speaking,
    required this.energy,
  });

  final double size;
  final bool enabled;
  final bool engaged;
  final bool realtime;
  final bool speaking;
  final Color energy;

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    final energized = enabled && (engaged || speaking);
    final icon = speaking
        ? CupertinoIcons.waveform
        : engaged
        ? CupertinoIcons.waveform_path
        : realtime
        ? CupertinoIcons.dot_radiowaves_left_right
        : CupertinoIcons.mic_fill;
    return AnimatedContainer(
      duration: const Duration(milliseconds: 180),
      curve: Curves.easeOutCubic,
      width: size,
      height: size,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: energized
              ? (dark
                    ? const [Color(0xFFE8F5F0), Color(0xFFB9E7D6)]
                    : const [Color(0xFF13231E), Color(0xFF29443A)])
              : (dark
                    ? const [Color(0xFF343C39), Color(0xFF1D2321)]
                    : const [Color(0xFFFFFFFF), Color(0xFFE9EFEC)]),
        ),
        border: Border.all(
          color: dark ? const Color(0x38FFFFFF) : const Color(0xB8FFFFFF),
        ),
        boxShadow: [
          if (energized)
            BoxShadow(
              color: energy.withValues(alpha: dark ? 0.2 : 0.16),
              blurRadius: 16,
            ),
          BoxShadow(
            color: dark ? const Color(0x42000000) : const Color(0x1717342C),
            blurRadius: 10,
            offset: const Offset(0, 4),
          ),
        ],
      ),
      child: Icon(
        icon,
        color: enabled
            ? (energized
                  ? (dark ? GizColors.ink : CupertinoColors.white)
                  : (dark ? CupertinoColors.white : GizColors.ink))
            : (dark ? const Color(0x66FFFFFF) : const Color(0x55001913)),
        size: size * (engaged ? 0.4 : 0.36),
      ),
    );
  }
}

class _VoiceEnergyPainter extends CustomPainter {
  const _VoiceEnergyPainter({
    required this.phase,
    required this.inputLevel,
    required this.outputLevel,
    required this.engaged,
    required this.energy,
  });

  final double phase;
  final double inputLevel;
  final double outputLevel;
  final bool engaged;
  final Color energy;

  @override
  void paint(Canvas canvas, Size size) {
    if (!engaged && inputLevel <= 0.02 && outputLevel <= 0.02) return;
    final center = Offset(size.width / 2, size.height / 2);
    final unit = math.min(size.width, size.height);
    final pulse = math.sin(phase * math.pi * 2);
    final levels = [inputLevel, outputLevel];
    final colors = [energy, const Color(0xFF6F86D9)];
    for (var ring = 0; ring < levels.length; ring++) {
      final level = levels[ring];
      final radius =
          unit * (0.39 + ring * 0.055) +
          pulse * (0.7 + ring * 0.35) +
          level * 4;
      canvas.drawCircle(
        center,
        radius,
        Paint()
          ..style = PaintingStyle.stroke
          ..strokeWidth = ring == 0 ? 1.4 : 1
          ..color = colors[ring].withValues(alpha: 0.16 + level * 0.38),
      );
    }
  }

  @override
  bool shouldRepaint(_VoiceEnergyPainter oldDelegate) =>
      oldDelegate.phase != phase ||
      oldDelegate.inputLevel != inputLevel ||
      oldDelegate.outputLevel != outputLevel ||
      oldDelegate.engaged != engaged ||
      oldDelegate.energy != energy;
}

Future<void> _showConversationPopover(
  BuildContext context,
  MobileDataController data,
) async {
  await showGeneralDialog<void>(
    context: context,
    barrierDismissible: true,
    barrierLabel: 'Close conversation controls',
    barrierColor: const Color(0x33000806),
    transitionDuration: const Duration(milliseconds: 360),
    pageBuilder: (context, animation, secondaryAnimation) =>
        _ConversationPopover(data: data),
    transitionBuilder: (context, animation, secondaryAnimation, child) {
      final curved = CurvedAnimation(
        parent: animation,
        curve: Curves.easeOutBack,
        reverseCurve: Curves.easeInCubic,
      );
      return FadeTransition(
        opacity: animation,
        child: ScaleTransition(
          scale: Tween<double>(begin: 0.72, end: 1).animate(curved),
          alignment: Alignment.bottomRight,
          child: SlideTransition(
            position: Tween<Offset>(
              begin: const Offset(0.08, 0.08),
              end: Offset.zero,
            ).animate(curved),
            child: child,
          ),
        ),
      );
    },
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
      minimum: const EdgeInsets.fromLTRB(16, 16, 16, 96),
      child: Align(
        alignment: Alignment.bottomRight,
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
