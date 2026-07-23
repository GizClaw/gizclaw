import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/cupertino.dart';
import 'package:flutter/scheduler.dart';
import 'package:flutter/services.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:go_router/go_router.dart';

import '../connection/gizclaw_connection_controller.dart';
import '../data/mobile_data_controller.dart';
import '../data/workspace_chat_controller.dart';
import '../giz_ui/giz_ui.dart';
import '../l10n/l10n.dart';
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
    if (!MobileDataScope.watch(context).hasActiveServer) return child;
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
    '/collections/assistants',
    '/collections/translates',
    '/collections/raids',
    '/collections/story-teller',
    '/collections/role-play',
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
                          location: location,
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
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: dark ? const Color(0xF013211C) : const Color(0xFAF5F6F2),
            borderRadius: BorderRadius.circular(38),
            border: Border.all(
              color: dark ? const Color(0x3DFFFFFF) : const Color(0x26FFFFFF),
            ),
          ),
          child: child,
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

typedef _AudioFieldSnapshot = ({
  bool startingInput,
  bool recording,
  bool playingOutput,
  double inputLevel,
  double outputLevel,
});

_AudioFieldSnapshot _audioFieldSnapshot(WorkspaceChatController? chat) => (
  startingInput: chat?.startingInput ?? false,
  recording: chat?.recording ?? false,
  playingOutput: chat?.playingOutput ?? false,
  inputLevel: chat?.inputLevel ?? 0,
  outputLevel: chat?.outputLevel ?? 0,
);

class _GlobalAudioFieldState extends State<_GlobalAudioField>
    with SingleTickerProviderStateMixin {
  WorkspaceChatController? _chat;
  _AudioFieldSnapshot? _snapshot;
  late final Ticker _levelTicker;
  Duration _lastTick = Duration.zero;
  double _animatedInputLevel = 0;
  double _animatedOutputLevel = 0;
  double _targetInputLevel = 0;
  double _targetOutputLevel = 0;

  @override
  void initState() {
    super.initState();
    _levelTicker = createTicker(_animateAudioLevels);
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final chat = MobileDataScope.watch(context).activeWorkspaceChat;
    if (identical(chat, _chat)) return;
    _chat?.removeListener(_handleChatChanged);
    _chat = chat;
    _snapshot = _audioFieldSnapshot(chat);
    _updateAudioTargets(_snapshot!);
    chat?.addListener(_handleChatChanged);
  }

  void _handleChatChanged() {
    final next = _audioFieldSnapshot(_chat);
    if (next == _snapshot) return;
    _snapshot = next;
    _updateAudioTargets(next);
  }

  void _updateAudioTargets(_AudioFieldSnapshot snapshot) {
    _targetInputLevel = snapshot.inputLevel;
    _targetOutputLevel = snapshot.outputLevel;
    if (!_levelTicker.isActive) {
      _lastTick = Duration.zero;
      _levelTicker.start();
    }
  }

  void _animateAudioLevels(Duration elapsed) {
    if (!mounted) return;
    final delta = _lastTick == Duration.zero
        ? 1 / 60
        : ((elapsed - _lastTick).inMicroseconds /
                  Duration.microsecondsPerSecond)
              .clamp(0.0, 0.05);
    _lastTick = elapsed;
    final nextInput = _followAudioTarget(
      _animatedInputLevel,
      _targetInputLevel,
      delta,
    );
    final nextOutput = _followAudioTarget(
      _animatedOutputLevel,
      _targetOutputLevel,
      delta,
    );
    final settled =
        (nextInput - _targetInputLevel).abs() < 0.0002 &&
        (nextOutput - _targetOutputLevel).abs() < 0.0002;
    setState(() {
      _animatedInputLevel = settled ? _targetInputLevel : nextInput;
      _animatedOutputLevel = settled ? _targetOutputLevel : nextOutput;
    });
    if (settled) {
      _levelTicker.stop();
      _lastTick = Duration.zero;
    }
  }

  @override
  void dispose() {
    _chat?.removeListener(_handleChatChanged);
    _levelTicker.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    final snapshot = _snapshot ?? _audioFieldSnapshot(_chat);
    final inputActive = snapshot.startingInput || snapshot.recording;
    final inputSignal = _responsiveAudioLevel(_animatedInputLevel);
    final outputSignal = _responsiveAudioLevel(_animatedOutputLevel);
    final input = math.max(inputSignal, inputActive ? 0.06 : 0.0);
    final output = math.max(outputSignal, snapshot.playingOutput ? 0.08 : 0.0);
    final overall = math.max(input, output);
    final signal = math.max(inputSignal, outputSignal).clamp(0.0, 1.0);
    final response = math.pow(signal, 0.62).toDouble();
    final energized = inputActive || snapshot.playingOutput || overall > 0.01;
    final outputMix = (output / (input + output + 0.01)).clamp(0.0, 1.0);
    final inputColor = Color.lerp(GizColors.accent, GizColors.success, 0.22)!;
    final outputColor = Color.lerp(
      GizColors.primaryHighlight,
      GizColors.primaryShadow,
      0.18,
    )!;
    final blend = Color.lerp(inputColor, outputColor, outputMix)!;
    final bottomAlpha = dark ? 0.68 : 0.62;
    final fieldWidth = MediaQuery.sizeOf(context).width;
    final dockTop =
        MediaQuery.paddingOf(context).bottom +
        GlobalConversationOverlay.dockBottomSpacing +
        GlobalConversationOverlay.dockHeight;
    final visibleGlowHeight = dockTop + 16;
    final glowScale = 0.88 + response * 0.22;

    return RepaintBoundary(
      key: const ValueKey('global-audio-field'),
      child: AnimatedOpacity(
        opacity: energized ? 1 : 0,
        duration: energized
            ? const Duration(milliseconds: 80)
            : const Duration(milliseconds: 220),
        curve: Curves.easeOutCubic,
        child: Stack(
          clipBehavior: Clip.none,
          children: [
            Positioned(
              width: fieldWidth,
              height: fieldWidth,
              left: 0,
              bottom: visibleGlowHeight - fieldWidth,
              child: Transform.scale(
                key: const ValueKey('global-audio-field-scale'),
                scale: glowScale,
                alignment: Alignment.center,
                child: DecoratedBox(
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    boxShadow: [
                      BoxShadow(
                        color: blend.withValues(alpha: dark ? 0.28 : 0.24),
                        blurRadius: 42,
                        spreadRadius: 12,
                      ),
                    ],
                    gradient: RadialGradient(
                      radius: 0.5,
                      colors: [
                        blend.withValues(alpha: bottomAlpha),
                        blend.withValues(alpha: bottomAlpha * 0.86),
                        blend.withValues(alpha: bottomAlpha * 0.62),
                        blend.withValues(alpha: bottomAlpha * 0.36),
                        blend.withValues(alpha: bottomAlpha * 0.14),
                        blend.withValues(alpha: 0),
                      ],
                      stops: const [0, 0.28, 0.52, 0.72, 0.88, 1],
                    ),
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

double _followAudioTarget(double current, double target, double deltaSeconds) {
  final timeConstant = target > current ? 0.036 : 0.11;
  final amount = 1 - math.exp(-deltaSeconds / timeConstant);
  return current + (target - current) * amount;
}

double _responsiveAudioLevel(double level) {
  const noiseFloor = 0.012;
  const fullScale = 0.1;
  final normalized = ((level - noiseFloor) / (fullScale - noiseFloor)).clamp(
    0.0,
    1.0,
  );
  return math.pow(normalized, 0.7).toDouble();
}

class _PrimaryDockNavigation extends StatefulWidget {
  const _PrimaryDockNavigation({super.key, required this.location});

  final Uri location;

  static const _items = [
    (GizIcons.house, GizIcons.house_fill, 'Home', '/active'),
    (
      GizIcons.waveform_path,
      GizIcons.waveform_path,
      'Assistants',
      '/collections/assistants',
    ),
    (GizIcons.globe, GizIcons.globe, 'Translates', '/collections/translates'),
    (GizIcons.scope, GizIcons.scope, 'Raids', '/collections/raids'),
    (
      GizIcons.wand_stars,
      GizIcons.wand_stars_inverse,
      'Story Teller',
      '/collections/story-teller',
    ),
    (
      GizIcons.person_fill,
      GizIcons.person_fill,
      'Role Play',
      '/collections/role-play',
    ),
    (GizIcons.person_2, GizIcons.person_2_fill, 'Friends', '/friends'),
    (GizIcons.person_3, GizIcons.person_3_fill, 'Groups', '/groups'),
    (GizIcons.paw, GizIcons.paw_solid, 'Pets', '/pets'),
    (
      GizIcons.person_crop_circle,
      GizIcons.person_crop_circle_fill,
      'Identity',
      '/identity',
    ),
  ];

  @override
  State<_PrimaryDockNavigation> createState() => _PrimaryDockNavigationState();
}

class _PrimaryDockNavigationState extends State<_PrimaryDockNavigation> {
  static const _itemSize = 58.0;
  static const _itemSpacing = 4.0;
  final ScrollController _scrollController = ScrollController();
  int _lastIndex = -1;
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
    final index = _selectedIndex;
    if (index < 0) return;
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

  int get _selectedIndex {
    final path = widget.location.path;
    return _PrimaryDockNavigation._items.indexWhere(
      (item) => path == item.$4 || path.startsWith('${item.$4}/'),
    );
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
            final selected = _selectedIndex == index;
            final foreground = selected
                ? const Color(0xFFF7F8F7)
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
                onPressed: () => context.go(item.$4),
                child: AnimatedContainer(
                  duration: const Duration(milliseconds: 240),
                  curve: Curves.easeOutCubic,
                  width: _itemSize,
                  height: _itemSize,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    gradient: selected
                        ? const LinearGradient(
                            begin: Alignment.topLeft,
                            end: Alignment.bottomRight,
                            colors: [
                              GizColors.primaryHighlight,
                              GizColors.primaryShadow,
                            ],
                          )
                        : null,
                    border: selected
                        ? Border.all(
                            color: dark
                                ? const Color(0x38FFFFFF)
                                : const Color(0x12FFFFFF),
                          )
                        : null,
                    boxShadow: selected
                        ? [
                            BoxShadow(
                              color: CupertinoColors.black.withValues(
                                alpha: dark ? 0.42 : 0.2,
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
              GizIcons.chevron_left,
              size: 20,
              color: dark ? const Color(0xFFA4D8C9) : GizColors.primary,
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
          title: Text(context.l10n.actionText(key: 'unableActivate')),
          content: Text(_workspaceActivationErrorMessage(context, error)),
          actions: [
            CupertinoDialogAction(
              onPressed: () => Navigator.of(context).pop(),
              child: Text(context.l10n.commonOk),
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
        ? GizColors.onAccent
        : (dark ? const Color(0xFFA8C6D6) : GizColors.blue);
    final colors = active
        ? const [GizColors.accent, GizColors.accent]
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
                    ? GizColors.accent.withValues(alpha: 0.26)
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
                    active ? GizIcons.checkmark_alt : GizIcons.scope,
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

String _workspaceActivationErrorMessage(BuildContext context, Object error) {
  final message = error.toString().trim();
  if (message.isEmpty) return context.l10n.actionText(key: 'actionFailed');
  return message.startsWith('Bad state: ') ? message.substring(11) : message;
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
  if (segments.length >= 3 && segments[0] == 'collections') {
    final collection = segments[1];
    final workspaceName = segments[2];
    if (workspaceName == 'new') {
      return _DockContext(
        title: 'Choose workflow',
        subtitle: collection,
        fallbackRoute: '/collections/$collection',
      );
    }
    final active = data.activeWorkspaceName == workspaceName;
    final workspace = data.workspace(workspaceName);
    final driver = data
        .workflow(workspace.workflowAlias, collection: workspace.collection)
        .driver;
    final mode =
        data.activeInputMode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME
        ? 'Realtime'
        : 'Push to Talk';
    return _DockContext(
      title: workspace.title,
      subtitle: active
          ? '${driver.label}  /  $mode'
          : '${driver.label}  /  Viewing',
      fallbackRoute: '/collections/$collection',
      active: active,
      workspaceName: workspaceName,
    );
  }
  if (segments.length >= 2 && segments[0] == 'workspaces') {
    final workspaceName = segments[1];
    final active = data.activeWorkspaceName == workspaceName;
    final chatroom = data.chatroomWorkspace(workspaceName);
    final workspace = data.workspace(workspaceName);
    final driver = data
        .workflow(workspace.workflowAlias, collection: workspace.collection)
        .driver;
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
      fallbackRoute: '/workspaces',
      active: active,
      workspaceName: workspaceName,
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

typedef _ConversationControlSnapshot = ({
  WorkspaceChatState? state,
  bool canRecord,
  bool recording,
  bool startingInput,
  bool playingOutput,
});

_ConversationControlSnapshot _conversationControlSnapshot(
  WorkspaceChatController? chat,
) => (
  state: chat?.state,
  canRecord: chat?.canRecord ?? false,
  recording: chat?.recording ?? false,
  startingInput: chat?.startingInput ?? false,
  playingOutput: chat?.playingOutput ?? false,
);

class _GlobalConversationControlState extends State<GlobalConversationControl> {
  WorkspaceChatController? _observedChat;
  _ConversationControlSnapshot? _chatSnapshot;
  bool _switchingMode = false;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final chat = MobileDataScope.watch(context).activeWorkspaceChat;
    if (identical(chat, _observedChat)) return;
    _observedChat?.removeListener(_handleChatChanged);
    _observedChat = chat;
    _chatSnapshot = _conversationControlSnapshot(chat);
    chat?.addListener(_handleChatChanged);
  }

  void _handleChatChanged() {
    final next = _conversationControlSnapshot(_observedChat);
    if (next == _chatSnapshot) return;
    _chatSnapshot = next;
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
    final microphoneUnavailable =
        data.connectionState == MobileConnectionState.connected &&
        data.microphoneStatus.availability ==
            MicrophoneAvailability.unavailable;
    final microphoneRecovering =
        data.connectionState == MobileConnectionState.connected &&
        data.microphoneStatus.availability == MicrophoneAvailability.recovering;
    final microphoneBlocked = microphoneUnavailable || microphoneRecovering;
    final enabled = (chat?.canRecord ?? false) && !microphoneBlocked;
    final inputActive = chat?.recording == true || chat?.startingInput == true;
    final title = workspace?.title ?? 'No active workspace';
    final status = _statusLabel(context, data, chat, mode);
    final control = _VoiceModeToggle(
      enabled: enabled,
      microphoneUnavailable: microphoneUnavailable,
      microphoneRecovering: microphoneRecovering,
      microphoneUnavailableSemantics:
          data.microphoneStatus.failureKind ==
              MicrophoneFailureKind.permissionDenied
          ? context.l10n.microphonePermissionRetrySemantics
          : context.l10n.microphoneCaptureRetrySemantics,
      mode: mode,
      switchingMode: _switchingMode,
      recording: chat?.recording ?? false,
      preparing: chat?.startingInput ?? false,
      playingOutput: chat?.playingOutput ?? false,
      onSelectMode:
          workspaceName == null ||
              microphoneBlocked ||
              !data.canSetActiveInputMode
          ? null
          : (target) => _setMode(data, target),
      onPttStart: enabled ? () => _startInput(chat!) : null,
      onPttEnd: chat != null && (enabled || inputActive)
          ? () => unawaited(chat.finishInput())
          : null,
      onRealtimeTap: chat != null && (enabled || chat.recording)
          ? () => _toggleRealtime(chat)
          : null,
      onUnavailableTap: microphoneUnavailable && !inputActive
          ? () => _recoverMicrophone(data)
          : null,
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
    await chat.startInput();
  }

  Future<void> _toggleRealtime(WorkspaceChatController chat) async {
    if (chat.startingInput) return;
    if (chat.recording) {
      await chat.finishInput();
    } else {
      await chat.startInput();
    }
  }

  Future<void> _recoverMicrophone(MobileDataController data) async {
    unawaited(HapticFeedback.mediumImpact());
    try {
      await data.recoverMicrophone();
    } catch (_) {
      if (!mounted) return;
      await _showMicrophoneError(data, data.microphoneStatus.failureKind);
      return;
    }
    if (mounted &&
        data.microphoneStatus.availability ==
            MicrophoneAvailability.unavailable) {
      await _showMicrophoneError(data, data.microphoneStatus.failureKind);
    }
  }

  Future<void> _showMicrophoneError(
    MobileDataController data,
    MicrophoneFailureKind? failureKind,
  ) {
    final permissionDenied =
        failureKind == MicrophoneFailureKind.permissionDenied;
    return showCupertinoDialog<void>(
      context: context,
      builder: (context) => CupertinoAlertDialog(
        title: Text(
          permissionDenied
              ? context.l10n.microphonePermissionDeniedTitle
              : context.l10n.microphoneUnavailableTitle,
        ),
        content: Text(
          permissionDenied
              ? context.l10n.microphonePermissionDeniedMessage
              : context.l10n.microphoneUnavailableMessage,
        ),
        actions: permissionDenied
            ? [
                CupertinoDialogAction(
                  onPressed: () => Navigator.pop(context),
                  child: Text(context.l10n.commonOk),
                ),
              ]
            : [
                CupertinoDialogAction(
                  onPressed: () => Navigator.pop(context),
                  child: Text(context.l10n.commonCancel),
                ),
                CupertinoDialogAction(
                  isDefaultAction: true,
                  onPressed: () {
                    Navigator.pop(context);
                    unawaited(_recoverMicrophone(data));
                  },
                  child: Text(context.l10n.commonRetry),
                ),
              ],
      ),
    );
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
      if (mode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME) {
        final realtimeChat = data.activeWorkspaceChat;
        if (realtimeChat != null && realtimeChat.canRecord) {
          await realtimeChat.startInput();
        }
      }
    } catch (_) {
      if (!mounted) return;
      await showCupertinoDialog<void>(
        context: context,
        builder: (context) => CupertinoAlertDialog(
          title: Text(context.l10n.actionText(key: 'unableSwitchMode')),
          content: Text(context.l10n.actionText(key: 'actionFailed')),
          actions: [
            CupertinoDialogAction(
              onPressed: () => Navigator.pop(context),
              child: Text(context.l10n.commonOk),
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
    required this.microphoneUnavailable,
    required this.microphoneRecovering,
    required this.microphoneUnavailableSemantics,
    required this.mode,
    required this.switchingMode,
    required this.recording,
    required this.preparing,
    required this.playingOutput,
    required this.onSelectMode,
    required this.onPttStart,
    required this.onPttEnd,
    required this.onRealtimeTap,
    required this.onUnavailableTap,
  });

  final bool enabled;
  final bool microphoneUnavailable;
  final bool microphoneRecovering;
  final String microphoneUnavailableSemantics;
  final WorkspaceInputMode mode;
  final bool switchingMode;
  final bool recording;
  final bool preparing;
  final bool playingOutput;
  final ValueChanged<WorkspaceInputMode>? onSelectMode;
  final VoidCallback? onPttStart;
  final VoidCallback? onPttEnd;
  final VoidCallback? onRealtimeTap;
  final VoidCallback? onUnavailableTap;

  @override
  State<_VoiceModeToggle> createState() => _VoiceModeToggleState();
}

class _VoiceModeToggleState extends State<_VoiceModeToggle> {
  static const _realtimeTapCancelThreshold = 14.0;
  Timer? _holdTimer;
  int? _pointer;
  Offset? _pointerOrigin;
  bool _realtimeTapCancelled = false;
  bool _pttStarted = false;
  bool _startedInRealtime = false;

  void _handlePointerDown(PointerDownEvent event) {
    if (_pointer != null ||
        widget.switchingMode ||
        ((widget.microphoneUnavailable || widget.microphoneRecovering) &&
            !widget.recording &&
            !widget.preparing)) {
      return;
    }
    _pointer = event.pointer;
    _pointerOrigin = event.position;
    _realtimeTapCancelled = false;
    _pttStarted = false;
    _startedInRealtime =
        widget.mode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME;
    if (_startedInRealtime || widget.onPttStart == null) return;
    _holdTimer = Timer(const Duration(milliseconds: 90), () {
      if (_pointer == event.pointer) {
        _pttStarted = true;
        widget.onPttStart?.call();
      }
    });
  }

  void _handlePointerMove(PointerMoveEvent event) {
    final origin = _pointerOrigin;
    if (_pointer != event.pointer ||
        origin == null ||
        !_startedInRealtime ||
        _realtimeTapCancelled) {
      return;
    }
    if ((event.position.dx - origin.dx).abs() > _realtimeTapCancelThreshold) {
      _realtimeTapCancelled = true;
    }
  }

  void _handlePointerUp(PointerUpEvent event) {
    if (_pointer != event.pointer) return;
    _holdTimer?.cancel();
    if (_startedInRealtime) {
      if (!_realtimeTapCancelled) widget.onRealtimeTap?.call();
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
    _realtimeTapCancelled = false;
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
    final pttAccent = dark ? const Color(0xFF94D3C0) : GizColors.accent;
    final realtimeAccent = dark ? const Color(0xFFC1B4DC) : GizColors.lavender;
    final thumb = _VoiceModeThumb(
      enabled: widget.enabled,
      microphoneUnavailable: widget.microphoneUnavailable,
      microphoneRecovering: widget.microphoneRecovering,
      realtime: realtime,
      engaged: engaged,
      playingOutput: widget.playingOutput,
    );
    final interactiveThumb = GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTap: widget.onUnavailableTap,
      child: Listener(
        behavior: HitTestBehavior.opaque,
        onPointerDown: _handlePointerDown,
        onPointerMove: _handlePointerMove,
        onPointerUp: _handlePointerUp,
        onPointerCancel: _handlePointerCancel,
        child: thumb,
      ),
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
                  label: context.l10n.actionText(key: 'pushToTalk'),
                  icon: GizIcons.mic_fill,
                  color: pttAccent.withValues(alpha: realtime ? 0.48 : 0.72),
                  loading: widget.switchingMode && realtime,
                  onPressed:
                      !realtime ||
                          widget.switchingMode ||
                          widget.microphoneUnavailable ||
                          widget.microphoneRecovering
                      ? null
                      : () => widget.onSelectMode?.call(
                          WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
                        ),
                ),
              ),
              Expanded(
                child: _VoiceModeTarget(
                  key: const ValueKey('voice-mode-realtime'),
                  label: context.l10n.actionText(key: 'realtime'),
                  icon: GizIcons.phone_fill,
                  color: realtimeAccent.withValues(
                    alpha: realtime ? 0.72 : 0.48,
                  ),
                  loading: widget.switchingMode && !realtime,
                  onPressed:
                      realtime ||
                          widget.switchingMode ||
                          widget.microphoneUnavailable ||
                          widget.microphoneRecovering
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
              label: widget.microphoneUnavailable
                  ? widget.microphoneUnavailableSemantics
                  : widget.microphoneRecovering
                  ? context.l10n.microphoneRecovering
                  : realtime
                  ? widget.recording
                        ? context.l10n.voiceEndRealtimeSemantics
                        : context.l10n.voiceStartRealtimeSemantics
                  : context.l10n.voiceHoldToTalkSemantics,
              button: !widget.microphoneRecovering,
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
      label: context.l10n.switchToMode(mode: label),
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
    required this.microphoneUnavailable,
    required this.microphoneRecovering,
    required this.realtime,
    required this.engaged,
    required this.playingOutput,
  });

  final bool enabled;
  final bool microphoneUnavailable;
  final bool microphoneRecovering;
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
            colors: widgetColors,
          ),
          border: Border.all(
            color: dark ? const Color(0x5CFFFFFF) : const Color(0x52FFFFFF),
          ),
          boxShadow: [
            BoxShadow(
              color: CupertinoColors.black.withValues(
                alpha: energized ? 0.42 : (dark ? 0.48 : 0.24),
              ),
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
          child: microphoneRecovering
              ? const CupertinoActivityIndicator(
                  key: ValueKey('microphone-recovering'),
                  color: CupertinoColors.white,
                )
              : Icon(
                  realtime ? GizIcons.phone_fill : GizIcons.mic_fill,
                  key: ValueKey(realtime),
                  size: realtime ? 22 : 21,
                  color: enabled || microphoneUnavailable
                      ? const Color(0xFFF7F8F7)
                      : CupertinoColors.white.withValues(alpha: 0.46),
                ),
        ),
      ),
    );
  }

  List<Color> get widgetColors => microphoneUnavailable
      ? const [CupertinoColors.systemRed, CupertinoColors.systemRed]
      : const [GizColors.primaryHighlight, GizColors.primaryShadow];
}

WorkspaceInputMode _effectiveMode(WorkspaceInputMode mode) =>
    mode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME
    ? mode
    : WorkspaceInputMode.WORKSPACE_INPUT_MODE_PUSH_TO_TALK;

String _statusLabel(
  BuildContext context,
  MobileDataController data,
  WorkspaceChatController? chat,
  WorkspaceInputMode mode,
) {
  if (data.connectionState == MobileConnectionState.connecting) {
    return context.l10n.conversationStatusConnecting;
  }
  if (data.connectionState == MobileConnectionState.connected &&
      data.microphoneStatus.availability == MicrophoneAvailability.recovering) {
    return context.l10n.microphoneRecovering;
  }
  if (data.connectionState == MobileConnectionState.connected &&
      data.microphoneStatus.availability ==
          MicrophoneAvailability.unavailable) {
    return data.microphoneStatus.failureKind ==
            MicrophoneFailureKind.permissionDenied
        ? context.l10n.microphonePermissionRequiredStatus
        : context.l10n.microphoneUnavailableStatus;
  }
  if (chat == null) return context.l10n.conversationStatusNoActive;
  if (chat.recording) {
    return mode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME
        ? context.l10n.conversationStatusRealtimeLive
        : context.l10n.conversationStatusListening;
  }
  if (chat.playingOutput) return context.l10n.conversationStatusSpeaking;
  return mode == WorkspaceInputMode.WORKSPACE_INPUT_MODE_REALTIME
      ? context.l10n.conversationStatusRealtimeReady
      : context.l10n.conversationStatusHoldToTalk;
}
