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
    '/active',
    '/raids',
    '/friends',
    '/groups',
    '/pets',
    '/identity',
  };

  @override
  Widget build(BuildContext context) {
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
  const _DockCapsule({required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    return Container(
      height: GlobalConversationOverlay.dockHeight,
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(38),
        boxShadow: dark
            ? const [
                BoxShadow(
                  color: Color(0x52000000),
                  blurRadius: 24,
                  offset: Offset(0, 10),
                ),
                BoxShadow(
                  color: Color(0x30000000),
                  blurRadius: 6,
                  offset: Offset(0, 2),
                ),
              ]
            : const [
                BoxShadow(
                  color: Color(0x17000000),
                  blurRadius: 24,
                  offset: Offset(0, 10),
                ),
                BoxShadow(
                  color: Color(0x0D000000),
                  blurRadius: 6,
                  offset: Offset(0, 2),
                ),
              ],
      ),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(38),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 28, sigmaY: 28),
          child: DecoratedBox(
            decoration: BoxDecoration(
              color: dark ? const Color(0xE013211C) : const Color(0xEDF5F6F2),
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
              inputActive:
                  (chat?.startingInput ?? false) || (chat?.recording ?? false),
              outputActive: chat?.playingOutput ?? false,
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
    required this.inputActive,
    required this.outputActive,
  });

  final bool dark;
  final double phase;
  final double presence;
  final double inputLevel;
  final double outputLevel;
  final bool inputActive;
  final bool outputActive;

  @override
  void paint(Canvas canvas, Size size) {
    if (presence <= 0.001 || size.isEmpty) return;
    final sampledInput = math.pow(inputLevel.clamp(0.0, 1.0), 0.38).toDouble();
    final sampledOutput = math
        .pow(outputLevel.clamp(0.0, 1.0), 0.38)
        .toDouble();
    final input = math.max(sampledInput, inputActive ? 0.1 : 0.0);
    final output = math.max(sampledOutput, outputActive ? 0.14 : 0.0);
    final overall = math.max(input, output);
    final angle = phase * math.pi * 2;
    const inputColor = GizColors.teal;
    const outputColor = GizColors.lavender;
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
            blend.withValues(
              alpha: presence * (dark ? 0.025 : 0.018) * (0.4 + overall),
            ),
            blend.withValues(
              alpha: presence * (dark ? 0.2 : 0.15) * (0.55 + overall * 0.45),
            ),
          ],
          stops: const [0, 0.42, 1],
        ).createShader(Offset.zero & size),
    );

    if (output > 0.001) {
      _paintFlameLayer(
        canvas,
        size,
        color: outputColor,
        level: output,
        overall: overall,
        phase: angle * 0.86 + 1.7,
        frequency: 4.3,
      );
    }
    if (input > 0.001) {
      _paintFlameLayer(
        canvas,
        size,
        color: inputColor,
        level: input,
        overall: overall,
        phase: -angle * 1.08,
        frequency: 5.1,
      );
    }
  }

  void _paintFlameLayer(
    Canvas canvas,
    Size size, {
    required Color color,
    required double level,
    required double overall,
    required double phase,
    required double frequency,
  }) {
    final flameHeight = size.height * (0.22 + level * 0.36 + overall * 0.12);
    final path = Path()..moveTo(0, size.height);
    const segments = 52;
    for (var index = 0; index <= segments; index++) {
      final progress = index / segments;
      final tongue = math.pow(
        (math.sin(progress * math.pi * frequency + phase) + 1) / 2,
        2.6,
      );
      final detail = math.pow(
        (math.sin(progress * math.pi * (frequency * 1.83) - phase * 0.71) + 1) /
            2,
        3.2,
      );
      final drift =
          (math.sin(progress * math.pi * 2.2 + phase * 0.37) + 1) * 0.08;
      final edgeFade = math.pow(
        math.sin(progress * math.pi).clamp(0.0, 1.0),
        0.38,
      );
      final profile = 0.18 + tongue * 0.54 + detail * 0.2 + drift;
      final y = size.height - flameHeight * profile * edgeFade;
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
        ..blendMode = dark ? BlendMode.screen : BlendMode.srcOver
        ..shader = LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            color.withValues(alpha: presence * (dark ? 0.025 : 0.018)),
            color.withValues(alpha: presence * (dark ? 0.13 : 0.095)),
            color.withValues(alpha: presence * (dark ? 0.32 : 0.24)),
          ],
          stops: const [0, 0.5, 1],
        ).createShader(bounds),
    );
  }

  @override
  bool shouldRepaint(_AudioFieldPainter oldDelegate) =>
      oldDelegate.dark != dark ||
      oldDelegate.phase != phase ||
      oldDelegate.presence != presence ||
      oldDelegate.inputLevel != inputLevel ||
      oldDelegate.outputLevel != outputLevel ||
      oldDelegate.inputActive != inputActive ||
      oldDelegate.outputActive != outputActive;
}

class _PrimaryDockNavigation extends StatefulWidget {
  const _PrimaryDockNavigation({super.key, required this.navigationShell});

  final StatefulNavigationShell navigationShell;

  static const _items = [
    (CupertinoIcons.waveform, CupertinoIcons.waveform, 'Active'),
    (
      CupertinoIcons.rectangle_3_offgrid,
      CupertinoIcons.rectangle_3_offgrid_fill,
      'Raids',
    ),
    (CupertinoIcons.person_2, CupertinoIcons.person_2_fill, 'Friends'),
    (CupertinoIcons.person_3, CupertinoIcons.person_3_fill, 'Groups'),
    (CupertinoIcons.sparkles, CupertinoIcons.sparkles, 'Pets'),
    (
      CupertinoIcons.person_crop_circle,
      CupertinoIcons.person_crop_circle_fill,
      'Identity',
    ),
  ];

  @override
  State<_PrimaryDockNavigation> createState() => _PrimaryDockNavigationState();
}

class _PrimaryDockNavigationState extends State<_PrimaryDockNavigation> {
  static const _itemSize = 58.0;
  static const _itemSpacing = 4.0;
  final ScrollController _scrollController = ScrollController();
  late int _lastIndex = widget.navigationShell.currentIndex;
  bool _canScrollBackward = false;
  bool _canScrollForward = false;

  @override
  void initState() {
    super.initState();
    _scrollController.addListener(_updateEdgeFades);
    WidgetsBinding.instance.addPostFrameCallback((_) => _updateEdgeFades());
  }

  @override
  void dispose() {
    _scrollController.removeListener(_updateEdgeFades);
    _scrollController.dispose();
    super.dispose();
  }

  void _updateEdgeFades() {
    if (!mounted || !_scrollController.hasClients) return;
    final position = _scrollController.position;
    final canScrollBackward = position.pixels > position.minScrollExtent + 0.5;
    final canScrollForward = position.pixels < position.maxScrollExtent - 0.5;
    if (canScrollBackward == _canScrollBackward &&
        canScrollForward == _canScrollForward) {
      return;
    }
    setState(() {
      _canScrollBackward = canScrollBackward;
      _canScrollForward = canScrollForward;
    });
  }

  void _scheduleSelectedItem() {
    final index = widget.navigationShell.currentIndex;
    if (_lastIndex == index) return;
    _lastIndex = index;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted || !_scrollController.hasClients) return;
      final position = _scrollController.position;
      final itemCenter = 7 + index * (_itemSize + _itemSpacing) + _itemSize / 2;
      final target = (itemCenter - position.viewportDimension / 2).clamp(
        position.minScrollExtent,
        position.maxScrollExtent,
      );
      _scrollController.animateTo(
        target,
        duration: const Duration(milliseconds: 280),
        curve: Curves.easeOutCubic,
      );
    });
  }

  @override
  Widget build(BuildContext context) {
    _scheduleSelectedItem();
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    return SizedBox(
      height: GlobalConversationOverlay.dockHeight,
      child: ShaderMask(
        key: const ValueKey('primary-nav-edge-fade'),
        blendMode: BlendMode.dstIn,
        shaderCallback: (bounds) => LinearGradient(
          colors: [
            _canScrollBackward
                ? const Color(0x0DFFFFFF)
                : CupertinoColors.white,
            CupertinoColors.white,
            CupertinoColors.white,
            _canScrollForward ? const Color(0x0DFFFFFF) : CupertinoColors.white,
          ],
          stops: const [0, 0.2, 0.8, 1],
        ).createShader(bounds),
        child: ListView.separated(
          key: const ValueKey('primary-nav-scroll'),
          controller: _scrollController,
          scrollDirection: Axis.horizontal,
          physics: const BouncingScrollPhysics(
            parent: AlwaysScrollableScrollPhysics(),
          ),
          padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 9),
          itemCount: _PrimaryDockNavigation._items.length,
          separatorBuilder: (context, index) =>
              const SizedBox(width: _itemSpacing),
          itemBuilder: (context, index) {
            final item = _PrimaryDockNavigation._items[index];
            final selected = widget.navigationShell.currentIndex == index;
            final foreground = selected
                ? GizColors.accent
                : (dark
                      ? const Color(0xB8D7E1DC)
                      : GizColors.secondaryInk.withValues(alpha: 0.74));
            return Semantics(
              label: item.$3,
              selected: selected,
              button: true,
              child: CupertinoButton(
                key: ValueKey('primary-nav-${item.$3.toLowerCase()}'),
                minimumSize: const Size.square(_itemSize),
                padding: EdgeInsets.zero,
                pressedOpacity: 0.68,
                onPressed: () => widget.navigationShell.goBranch(
                  index,
                  initialLocation: selected,
                ),
                child: AnimatedContainer(
                  duration: const Duration(milliseconds: 240),
                  curve: Curves.easeOutCubic,
                  width: _itemSize,
                  height: _itemSize,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    gradient: selected
                        ? LinearGradient(
                            begin: Alignment.topLeft,
                            end: Alignment.bottomRight,
                            colors: [
                              Color.lerp(
                                GizColors.teal,
                                CupertinoColors.white,
                                dark ? 0.2 : 0.08,
                              )!,
                              Color.lerp(
                                GizColors.teal,
                                GizColors.ink,
                                dark ? 0.08 : 0.24,
                              )!,
                            ],
                          )
                        : null,
                    boxShadow: selected
                        ? [
                            BoxShadow(
                              color: GizColors.teal.withValues(
                                alpha: dark ? 0.34 : 0.28,
                              ),
                              blurRadius: 14,
                              offset: const Offset(0, 5),
                            ),
                          ]
                        : null,
                  ),
                  child: Icon(
                    selected ? item.$2 : item.$1,
                    size: 23,
                    color: foreground,
                  ),
                ),
              ),
            );
          },
        ),
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
              color: dark ? const Color(0xFFA4D8C9) : GizColors.teal,
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
                          color: GizColors.success,
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
            _WorkspaceActivationButton(
              active: info.active,
              workspaceName: info.workspaceName!,
            ),
            const SizedBox(width: 9),
          ],
        ],
      ),
    );
  }
}

class _WorkspaceActivationButton extends StatefulWidget {
  const _WorkspaceActivationButton({
    required this.active,
    required this.workspaceName,
  });

  final bool active;
  final String workspaceName;

  @override
  State<_WorkspaceActivationButton> createState() =>
      _WorkspaceActivationButtonState();
}

class _WorkspaceActivationButtonState
    extends State<_WorkspaceActivationButton> {
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
        ? GizColors.accent
        : (dark ? const Color(0xFFA8C6D6) : GizColors.blue);
    final colors = active
        ? (dark
              ? const [Color(0xFF4A9B84), Color(0xFF245F50)]
              : const [Color(0xFF3C8B76), Color(0xFF1F6554)])
        : (dark
              ? const [Color(0xFF303A37), Color(0xFF26302D)]
              : const [Color(0xFFF8FAF8), Color(0xFFE9EFEB)]);

    return Semantics(
      label: active ? 'Active workspace' : 'Set active workspace',
      selected: active,
      button: true,
      child: CupertinoButton(
        key: const ValueKey('workspace-activation-button'),
        padding: EdgeInsets.zero,
        minimumSize: const Size.square(58),
        pressedOpacity: active ? 1 : 0.66,
        onPressed: active ? null : _activate,
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 260),
          curve: Curves.easeOutCubic,
          width: 58,
          height: 58,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            gradient: LinearGradient(
              begin: Alignment.topLeft,
              end: Alignment.bottomRight,
              colors: colors,
            ),
            border: Border.all(
              color: active
                  ? foreground.withValues(alpha: 0.22)
                  : foreground.withValues(alpha: dark ? 0.14 : 0.1),
            ),
            boxShadow: [
              BoxShadow(
                color: active
                    ? const Color(0x2E001913)
                    : const Color(0x12001913),
                blurRadius: active ? 12 : 8,
                offset: const Offset(0, 4),
              ),
            ],
          ),
          child: AnimatedSwitcher(
            duration: const Duration(milliseconds: 190),
            transitionBuilder: (child, animation) => ScaleTransition(
              scale: CurvedAnimation(
                parent: animation,
                curve: Curves.easeOutBack,
              ),
              child: FadeTransition(opacity: animation, child: child),
            ),
            child: _activating
                ? CupertinoActivityIndicator(
                    key: const ValueKey('activating'),
                    radius: 10,
                    color: foreground,
                  )
                : Icon(
                    active
                        ? CupertinoIcons.checkmark_alt
                        : CupertinoIcons.scope,
                    key: ValueKey(active),
                    size: active ? 25 : 24,
                    color: foreground,
                  ),
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
  if (segments.length >= 4 && segments[0] == 'raids') {
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
      fallbackRoute: '/raids/drivers/${driver.routeKey}',
      active: active,
      workspaceName: workspaceName,
    );
  }
  if (segments.length >= 3 && segments[0] == 'raids') {
    final driver = WorkflowDriverKind.fromRouteKey(segments[2]);
    return _DockContext(
      title: driver.label,
      subtitle: 'Available workspaces',
      fallbackRoute: '/raids',
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
  if (segments.length >= 2 && segments[0] == 'pets') {
    final pet = data.petRouteContext(segments[1]);
    final workspaceName = pet?.workspaceName;
    final active =
        workspaceName != null && data.activeWorkspaceName == workspaceName;
    return _DockContext(
      title: pet?.title ?? 'Pet companion',
      subtitle: active ? 'Pet  /  Connected' : 'Pet  /  Viewing',
      fallbackRoute: '/pets',
      active: active,
      workspaceName: workspaceName,
    );
  }
  return const _DockContext(
    title: 'GizClaw',
    subtitle: 'Back to the previous page',
    fallbackRoute: '/active',
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

class _VoiceModeToggle extends StatefulWidget {
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
  State<_VoiceModeToggle> createState() => _VoiceModeToggleState();
}

class _VoiceModeToggleState extends State<_VoiceModeToggle> {
  static const _dragThreshold = 14.0;
  Timer? _holdTimer;
  int? _pointer;
  Offset? _pointerOrigin;
  bool _dragged = false;
  bool _pttStarted = false;
  bool _startedInRealtime = false;

  void _handlePointerDown(PointerDownEvent event) {
    if (_pointer != null || widget.switchingMode) return;
    _pointer = event.pointer;
    _pointerOrigin = event.position;
    _dragged = false;
    _pttStarted = false;
    _startedInRealtime =
        widget.mode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME;
    if (_startedInRealtime || widget.onPttStart == null) return;
    _holdTimer = Timer(const Duration(milliseconds: 90), () {
      if (_pointer == event.pointer && !_dragged) {
        _pttStarted = true;
        widget.onPttStart?.call();
      }
    });
  }

  void _handlePointerMove(PointerMoveEvent event) {
    final origin = _pointerOrigin;
    if (_pointer != event.pointer || origin == null || _dragged) return;
    final delta = event.position.dx - origin.dx;
    if (_startedInRealtime && delta < -_dragThreshold) {
      _switchMode(WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK);
    } else if (!_startedInRealtime && delta > _dragThreshold) {
      _holdTimer?.cancel();
      _finishPtt();
      _switchMode(WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME);
    }
  }

  void _switchMode(WorkspaceInputMode mode) {
    final selectMode = widget.onSelectMode;
    if (selectMode == null || widget.switchingMode) return;
    _dragged = true;
    selectMode(mode);
  }

  void _handlePointerUp(PointerUpEvent event) {
    if (_pointer != event.pointer) return;
    _holdTimer?.cancel();
    if (_startedInRealtime) {
      if (!_dragged) widget.onRealtimeTap?.call();
    } else {
      _finishPtt();
    }
    _resetPointer();
  }

  void _handlePointerCancel(PointerCancelEvent event) {
    if (_pointer != event.pointer) return;
    _holdTimer?.cancel();
    _finishPtt();
    _resetPointer();
  }

  void _finishPtt() {
    if (!_pttStarted) return;
    _pttStarted = false;
    widget.onPttEnd?.call();
  }

  void _resetPointer() {
    _pointer = null;
    _pointerOrigin = null;
    _dragged = false;
  }

  @override
  void dispose() {
    _holdTimer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    final realtime =
        widget.mode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME;
    final engaged = widget.recording || widget.preparing;
    final pttAccent = dark ? const Color(0xFF94D3C0) : GizColors.teal;
    final realtimeAccent = dark ? const Color(0xFFC1B4DC) : GizColors.lavender;
    final thumb = _VoiceModeThumb(
      enabled: widget.enabled,
      realtime: realtime,
      engaged: engaged,
      playingOutput: widget.playingOutput,
    );
    final interactiveThumb = Listener(
      behavior: HitTestBehavior.opaque,
      onPointerDown: _handlePointerDown,
      onPointerMove: _handlePointerMove,
      onPointerUp: _handlePointerUp,
      onPointerCancel: _handlePointerCancel,
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
                  color: pttAccent.withValues(alpha: realtime ? 0.48 : 0.72),
                  loading: widget.switchingMode && realtime,
                  onPressed: !realtime || widget.switchingMode
                      ? null
                      : () => widget.onSelectMode?.call(
                          WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
                        ),
                ),
              ),
              Expanded(
                child: _VoiceModeTarget(
                  key: const ValueKey('voice-mode-realtime'),
                  label: 'Realtime',
                  icon: CupertinoIcons.phone_fill,
                  color: realtimeAccent.withValues(
                    alpha: realtime ? 0.72 : 0.48,
                  ),
                  loading: widget.switchingMode && !realtime,
                  onPressed: realtime || widget.switchingMode
                      ? null
                      : () => widget.onSelectMode?.call(
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
                  ? widget.recording
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
    final accent = realtime ? GizColors.lavender : GizColors.teal;
    final gradientColors = dark
        ? [
            Color.lerp(accent, CupertinoColors.white, 0.34)!,
            Color.lerp(accent, CupertinoColors.black, 0.02)!,
          ]
        : [
            Color.lerp(accent, CupertinoColors.white, 0.1)!,
            Color.lerp(accent, CupertinoColors.black, 0.13)!,
          ];
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
            colors: gradientColors,
          ),
          border: Border.all(
            color: dark ? const Color(0x5CFFFFFF) : const Color(0x52FFFFFF),
          ),
          boxShadow: [
            BoxShadow(
              color: energized
                  ? accent.withValues(alpha: 0.42)
                  : accent.withValues(alpha: dark ? 0.3 : 0.24),
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
                ? (realtime ? CupertinoColors.white : GizColors.accent)
                : CupertinoColors.white.withValues(alpha: 0.46),
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
