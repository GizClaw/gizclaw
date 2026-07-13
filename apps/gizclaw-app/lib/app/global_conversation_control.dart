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

  static const double dockHeight = 76;
  static const double dockTopSpacing = 10;
  static const double dockBottomSpacing = 8;

  static double bottomContentInset(
    BuildContext context, {
    double spacing = 12,
  }) {
    final safeBottom = MediaQuery.paddingOf(context).bottom;
    return dockTopSpacing +
        dockHeight +
        math.max(safeBottom, dockBottomSpacing) +
        spacing;
  }

  @override
  Widget build(BuildContext context) {
    final audioFieldHeight = math.min(
      300.0,
      MediaQuery.sizeOf(context).height * 0.36,
    );
    return Stack(
      fit: StackFit.expand,
      children: [
        child,
        Positioned(
          left: 0,
          right: 0,
          bottom: 0,
          height: audioFieldHeight,
          child: const IgnorePointer(child: _GlobalAudioField()),
        ),
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

  static const _rootPaths = {
    '/browse',
    '/chats',
    '/friends',
    '/groups',
    '/pet',
    '/me',
  };

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    final shell = navigationShell;
    final showTabs = shell != null && _rootPaths.contains(location.path);
    return SafeArea(
      top: false,
      minimum: const EdgeInsets.fromLTRB(
        12,
        GlobalConversationOverlay.dockTopSpacing,
        12,
        GlobalConversationOverlay.dockBottomSpacing,
      ),
      child: SizedBox(
        height: GlobalConversationOverlay.dockHeight,
        child: Row(
          children: [
            Expanded(
              child: _DockCapsule(
                shadows: [
                  BoxShadow(
                    color: dark
                        ? const Color(0x66000000)
                        : const Color(0x1A001812),
                    blurRadius: 22,
                    offset: const Offset(0, 9),
                  ),
                ],
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
            ),
            const SizedBox(width: 10),
            const _DockCapsule(child: GlobalConversationControl(compact: true)),
          ],
        ),
      ),
    );
  }
}

class _DockCapsule extends StatelessWidget {
  const _DockCapsule({required this.child, this.shadows = const []});

  final Widget child;
  final List<BoxShadow> shadows;

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    return Container(
      height: GlobalConversationOverlay.dockHeight,
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(38),
        boxShadow: shadows,
      ),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(38),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 28, sigmaY: 28),
          child: DecoratedBox(
            decoration: BoxDecoration(
              color: dark ? const Color(0xD91B211F) : const Color(0xE8FAFCFB),
              borderRadius: BorderRadius.circular(38),
              border: Border.all(
                color: dark ? const Color(0x3DFFFFFF) : const Color(0x26FFFFFF),
              ),
            ),
            child: child,
          ),
        ),
      ),
    );
  }
}

class _GlobalAudioField extends StatefulWidget {
  const _GlobalAudioField();

  @override
  State<_GlobalAudioField> createState() => _GlobalAudioFieldState();
}

class _GlobalAudioFieldState extends State<_GlobalAudioField>
    with TickerProviderStateMixin {
  WorkspaceChatController? _chat;
  late final AnimationController _phase = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 4200),
  );
  late final AnimationController _presence =
      AnimationController(
        vsync: this,
        duration: const Duration(milliseconds: 260),
        reverseDuration: const Duration(milliseconds: 760),
      )..addStatusListener((status) {
        if (status == AnimationStatus.dismissed) _phase.stop();
      });

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final chat = MobileDataScope.watch(context).activeWorkspaceChat;
    if (identical(chat, _chat)) return;
    _chat?.removeListener(_handleChatChanged);
    _chat = chat;
    chat?.addListener(_handleChatChanged);
    _syncAnimation();
  }

  void _handleChatChanged() {
    _syncAnimation();
    if (mounted) setState(() {});
  }

  void _syncAnimation() {
    final chat = _chat;
    final energized =
        (chat?.startingInput ?? false) ||
        (chat?.recording ?? false) ||
        (chat?.playingOutput ?? false) ||
        (chat?.inputLevel ?? 0) > 0.01 ||
        (chat?.outputLevel ?? 0) > 0.01;
    if (energized) {
      if (!_phase.isAnimating) _phase.repeat();
      _presence.forward();
    } else {
      _presence.reverse();
    }
  }

  @override
  void dispose() {
    _chat?.removeListener(_handleChatChanged);
    _phase.dispose();
    _presence.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    return AnimatedBuilder(
      animation: Listenable.merge([_phase, _presence]),
      builder: (context, child) {
        final chat = _chat;
        return RepaintBoundary(
          key: const ValueKey('global-audio-field'),
          child: CustomPaint(
            painter: _AudioFieldPainter(
              dark: dark,
              phase: _phase.value,
              presence: Curves.easeInOutCubic.transform(_presence.value),
              inputLevel: chat?.inputLevel ?? 0,
              outputLevel: chat?.outputLevel ?? 0,
            ),
            size: Size.infinite,
          ),
        );
      },
    );
  }
}

class _AudioFieldPainter extends CustomPainter {
  const _AudioFieldPainter({
    required this.dark,
    required this.phase,
    required this.presence,
    required this.inputLevel,
    required this.outputLevel,
  });

  final bool dark;
  final double phase;
  final double presence;
  final double inputLevel;
  final double outputLevel;

  @override
  void paint(Canvas canvas, Size size) {
    if (presence <= 0.001 || size.isEmpty) return;
    final input = math.pow(inputLevel.clamp(0.0, 1.0), 0.42).toDouble();
    final output = math.pow(outputLevel.clamp(0.0, 1.0), 0.42).toDouble();
    final angle = phase * math.pi * 2;
    const inputColor = Color(0xFF42DDB4);
    const outputColor = Color(0xFF7588FF);
    final blend = Color.lerp(
      inputColor,
      outputColor,
      output / (input + output + 0.01),
    )!;

    canvas.drawRect(
      Offset.zero & size,
      Paint()
        ..shader = LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            const Color(0x00000000),
            blend.withValues(alpha: presence * (dark ? 0.035 : 0.025)),
            blend.withValues(alpha: presence * (dark ? 0.18 : 0.13)),
          ],
          stops: const [0, 0.48, 1],
        ).createShader(Offset.zero & size),
    );

    _paintWave(
      canvas,
      size,
      color: outputColor,
      level: output,
      phase: angle + 1.9,
      verticalOffset: 0.05,
    );
    _paintWave(
      canvas,
      size,
      color: inputColor,
      level: input,
      phase: -angle * 1.13,
      verticalOffset: 0,
    );
  }

  void _paintWave(
    Canvas canvas,
    Size size, {
    required Color color,
    required double level,
    required double phase,
    required double verticalOffset,
  }) {
    final energy = presence * (0.12 + level * 0.88);
    final baseY = size.height * (0.79 - verticalOffset - energy * 0.24);
    final amplitude = 5 + energy * size.height * 0.085;
    final path = Path()..moveTo(0, baseY);
    const segments = 36;
    for (var index = 0; index <= segments; index++) {
      final progress = index / segments;
      final primary = math.sin(progress * math.pi * 2.15 + phase);
      final detail = math.sin(progress * math.pi * 5.2 - phase * 0.62) * 0.34;
      final edgeFade = math.sin(progress * math.pi).clamp(0.0, 1.0);
      final y = baseY - (primary + detail) * amplitude * edgeFade;
      path.lineTo(progress * size.width, y);
    }
    path
      ..lineTo(size.width, size.height)
      ..lineTo(0, size.height)
      ..close();
    final bounds = path.getBounds();
    canvas.drawPath(
      path,
      Paint()
        ..shader = LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            color.withValues(alpha: presence * (dark ? 0.018 : 0.012)),
            color.withValues(alpha: presence * (dark ? 0.11 : 0.08)),
            color.withValues(alpha: presence * (dark ? 0.3 : 0.22)),
          ],
          stops: const [0, 0.42, 1],
        ).createShader(bounds),
    );
  }

  @override
  bool shouldRepaint(_AudioFieldPainter oldDelegate) =>
      oldDelegate.dark != dark ||
      oldDelegate.phase != phase ||
      oldDelegate.presence != presence ||
      oldDelegate.inputLevel != inputLevel ||
      oldDelegate.outputLevel != outputLevel;
}

class _PrimaryDockNavigation extends StatelessWidget {
  const _PrimaryDockNavigation({super.key, required this.navigationShell});

  final StatefulNavigationShell navigationShell;

  static const _items = [
    (CupertinoIcons.compass, CupertinoIcons.compass_fill, 'Browse'),
    (CupertinoIcons.chat_bubble_2, CupertinoIcons.chat_bubble_2_fill, 'Chats'),
    (CupertinoIcons.person_2, CupertinoIcons.person_2_fill, 'Friends'),
    (CupertinoIcons.person_3, CupertinoIcons.person_3_fill, 'Groups'),
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
  if (segments.length >= 2 && segments[0] == 'groups') {
    final workspaceName = segments[1];
    final group = data.chatroomWorkspace(workspaceName);
    final active = data.activeWorkspaceName == workspaceName;
    final mode =
        data.activeInputMode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME
        ? 'Realtime'
        : 'Push to Talk';
    return _DockContext(
      title: group?.title ?? data.workspace(workspaceName).title,
      subtitle: active ? 'Group chat  /  $mode' : 'Group chat  /  Viewing',
      fallbackRoute: '/groups',
      active: active,
      workspaceName: workspaceName,
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

class _GlobalConversationControlState extends State<GlobalConversationControl> {
  WorkspaceChatController? _observedChat;
  bool _switchingMode = false;

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
    final title = workspace?.title ?? 'No active workspace';
    final status = _statusLabel(data, chat, mode);
    final control = _VoiceModeToggle(
      enabled: enabled,
      mode: mode,
      switchingMode: _switchingMode,
      recording: chat?.recording ?? false,
      preparing: chat?.startingInput ?? false,
      playingOutput: chat?.playingOutput ?? false,
      onSelectMode: workspaceName == null
          ? null
          : (target) => _setMode(data, target),
      onPttStart: enabled ? () => _startInput(chat!) : null,
      onPttEnd: enabled ? () => unawaited(chat!.finishInput()) : null,
      onRealtimeTap: enabled ? () => _toggleRealtime(chat!) : null,
    );

    if (widget.compact) {
      return Semantics(
        label: '$title, $status',
        container: true,
        child: control,
      );
    }

    return Semantics(
      label: '$title, $status',
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          control,
          const SizedBox(height: 8),
          Text(
            status,
            style: GizText.label.copyWith(
              color: MediaQuery.platformBrightnessOf(context) == Brightness.dark
                  ? const Color(0xCCFFFFFF)
                  : GizColors.secondaryInk,
              fontSize: 10,
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _startInput(WorkspaceChatController chat) async {
    unawaited(HapticFeedback.mediumImpact());
    await chat.startInput();
  }

  Future<void> _toggleRealtime(WorkspaceChatController chat) async {
    if (chat.startingInput) return;
    unawaited(HapticFeedback.mediumImpact());
    if (chat.recording) {
      await chat.finishInput();
    } else {
      await chat.startInput();
    }
  }

  Future<void> _setMode(
    MobileDataController data,
    WorkspaceInputMode mode,
  ) async {
    if (_switchingMode || _effectiveMode(data.activeInputMode) == mode) return;
    setState(() => _switchingMode = true);
    unawaited(HapticFeedback.selectionClick());
    try {
      await data.setActiveInputMode(mode);
    } catch (error) {
      if (!mounted) return;
      await showCupertinoDialog<void>(
        context: context,
        builder: (context) => CupertinoAlertDialog(
          title: const Text('Unable to switch mode'),
          content: Text('$error'),
          actions: [
            CupertinoDialogAction(
              onPressed: () => Navigator.pop(context),
              child: const Text('OK'),
            ),
          ],
        ),
      );
    } finally {
      if (mounted) setState(() => _switchingMode = false);
    }
  }
}

class _VoiceModeToggle extends StatelessWidget {
  const _VoiceModeToggle({
    required this.enabled,
    required this.mode,
    required this.switchingMode,
    required this.recording,
    required this.preparing,
    required this.playingOutput,
    required this.onSelectMode,
    required this.onPttStart,
    required this.onPttEnd,
    required this.onRealtimeTap,
  });

  final bool enabled;
  final WorkspaceInputMode mode;
  final bool switchingMode;
  final bool recording;
  final bool preparing;
  final bool playingOutput;
  final ValueChanged<WorkspaceInputMode>? onSelectMode;
  final VoidCallback? onPttStart;
  final VoidCallback? onPttEnd;
  final VoidCallback? onRealtimeTap;

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    final realtime = mode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME;
    final engaged = recording || preparing;
    final inactive = dark ? const Color(0x8FFFFFFF) : const Color(0x73001913);
    final thumb = _VoiceModeThumb(
      enabled: enabled,
      realtime: realtime,
      engaged: engaged,
      playingOutput: playingOutput,
    );
    final interactiveThumb = realtime
        ? GestureDetector(
            behavior: HitTestBehavior.opaque,
            onTap: onRealtimeTap,
            child: thumb,
          )
        : Listener(
            behavior: HitTestBehavior.opaque,
            onPointerDown: onPttStart == null ? null : (_) => onPttStart!(),
            onPointerUp: onPttEnd == null ? null : (_) => onPttEnd!(),
            onPointerCancel: onPttEnd == null ? null : (_) => onPttEnd!(),
            child: thumb,
          );

    return SizedBox(
      key: const ValueKey('voice-mode-toggle'),
      width: 132,
      height: GlobalConversationOverlay.dockHeight,
      child: Stack(
        children: [
          Row(
            children: [
              Expanded(
                child: _VoiceModeTarget(
                  key: const ValueKey('voice-mode-ptt'),
                  label: 'Push to talk',
                  icon: CupertinoIcons.mic_fill,
                  color: inactive,
                  loading: switchingMode && realtime,
                  onPressed: !realtime || switchingMode
                      ? null
                      : () => onSelectMode?.call(
                          WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
                        ),
                ),
              ),
              Expanded(
                child: _VoiceModeTarget(
                  key: const ValueKey('voice-mode-realtime'),
                  label: 'Realtime',
                  icon: CupertinoIcons.phone_fill,
                  color: inactive,
                  loading: switchingMode && !realtime,
                  onPressed: realtime || switchingMode
                      ? null
                      : () => onSelectMode?.call(
                          WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME,
                        ),
                ),
              ),
            ],
          ),
          AnimatedPositioned(
            key: const ValueKey('voice-mode-thumb'),
            duration: const Duration(milliseconds: 300),
            curve: Curves.easeInOutCubic,
            top: 9,
            left: realtime ? 68 : 6,
            width: 58,
            height: 58,
            child: Semantics(
              label: realtime
                  ? recording
                        ? 'End realtime call'
                        : 'Start realtime call'
                  : 'Hold to talk',
              button: true,
              child: interactiveThumb,
            ),
          ),
        ],
      ),
    );
  }
}

class _VoiceModeTarget extends StatelessWidget {
  const _VoiceModeTarget({
    super.key,
    required this.label,
    required this.icon,
    required this.color,
    required this.loading,
    required this.onPressed,
  });

  final String label;
  final IconData icon;
  final Color color;
  final bool loading;
  final VoidCallback? onPressed;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: 'Switch to $label',
      button: onPressed != null,
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTap: onPressed,
        child: Center(
          child: loading
              ? CupertinoActivityIndicator(radius: 8, color: color)
              : Icon(icon, size: 18, color: color),
        ),
      ),
    );
  }
}

class _VoiceModeThumb extends StatelessWidget {
  const _VoiceModeThumb({
    required this.enabled,
    required this.realtime,
    required this.engaged,
    required this.playingOutput,
  });

  final bool enabled;
  final bool realtime;
  final bool engaged;
  final bool playingOutput;

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    final energized = engaged || playingOutput;
    return AnimatedScale(
      scale: engaged ? 0.92 : 1,
      duration: const Duration(milliseconds: 150),
      curve: Curves.easeOutCubic,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 220),
        curve: Curves.easeOutCubic,
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          gradient: LinearGradient(
            begin: Alignment.topLeft,
            end: Alignment.bottomRight,
            colors: dark
                ? const [Color(0xFFF4FFF9), Color(0xFFBCEBD9)]
                : const [Color(0xFF10231D), Color(0xFF24473B)],
          ),
          border: Border.all(
            color: dark ? const Color(0x5CFFFFFF) : const Color(0x52FFFFFF),
          ),
          boxShadow: [
            BoxShadow(
              color: energized
                  ? const Color(0x4542DDB4)
                  : (dark ? const Color(0x38000000) : const Color(0x26001913)),
              blurRadius: energized ? 14 : 9,
              offset: const Offset(0, 4),
            ),
          ],
        ),
        child: AnimatedSwitcher(
          duration: const Duration(milliseconds: 180),
          transitionBuilder: (child, animation) => ScaleTransition(
            scale: animation,
            child: FadeTransition(opacity: animation, child: child),
          ),
          child: Icon(
            realtime ? CupertinoIcons.phone_fill : CupertinoIcons.mic_fill,
            key: ValueKey(realtime),
            size: realtime ? 22 : 21,
            color: enabled
                ? (dark ? GizColors.ink : CupertinoColors.white)
                : (dark ? const Color(0x66001913) : const Color(0x73FFFFFF)),
          ),
        ),
      ),
    );
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
  if (chat.playingOutput) return 'SPEAKING';
  return mode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME
      ? 'REALTIME READY'
      : 'HOLD TO TALK';
}
