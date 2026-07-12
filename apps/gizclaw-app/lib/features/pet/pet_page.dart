import 'dart:async';
import 'dart:math' as math;
import 'dart:ui';

import 'package:flutter/cupertino.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:go_router/go_router.dart';

import '../../data/mobile_data_controller.dart';
import '../../data/workspace_chat_controller.dart';
import '../../giz_ui/giz_ui.dart';
import '../../pixa_sprite.dart';

const _petSceneColor = Color(0xFFDCEFE8);
const _petDetailBackground = Color(0xFFD8E7DF);

class PetPage extends StatefulWidget {
  const PetPage({super.key});

  @override
  State<PetPage> createState() => _PetPageState();
}

class _PetPageState extends State<PetPage> {
  GizClawClient? _client;
  List<Pet> _pets = const [];
  final Map<String, _PetVisual> _visuals = {};
  Object? _error;
  bool _loading = false;
  bool _adopting = false;
  int _request = 0;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final data = MobileDataScope.watch(context);
    final client = data.connectionState == MobileConnectionState.connected
        ? data.connection.client
        : null;
    if (identical(client, _client)) return;
    _client = client;
    _request += 1;
    if (client == null) {
      setState(() {
        _pets = const [];
        _visuals.clear();
        _loading = false;
      });
      return;
    }
    unawaited(_loadPets());
  }

  @override
  void dispose() {
    _request += 1;
    super.dispose();
  }

  Future<void> _loadPets() async {
    final client = _client;
    if (client == null || _loading) return;
    final request = ++_request;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final pets = <Pet>[];
      String? cursor;
      do {
        final response = await client.listPets(cursor: cursor, limit: 100);
        pets.addAll(response.value.items);
        cursor = response.value.hasNext ? response.value.nextCursor : null;
      } while (cursor != null && cursor.isNotEmpty);
      if (!mounted || request != _request) return;
      setState(() {
        _pets = pets;
        _visuals.removeWhere((id, _) => !pets.any((pet) => pet.id == id));
        _loading = false;
      });
      await Future.wait([
        for (final pet in pets) _loadVisual(client, pet, request),
      ]);
    } catch (error) {
      if (!mounted || request != _request) return;
      setState(() {
        _loading = false;
        _error = error;
      });
    }
  }

  Future<void> _loadVisual(GizClawClient client, Pet pet, int request) async {
    try {
      final presentation = (await client.getPetPresentation(pet.id)).value;
      PixaAsset? pixa;
      try {
        pixa = (await client.downloadPetPixa(pet.id)).asset;
      } catch (_) {
        // A PetDef can be visible before its optional PIXA asset is uploaded.
      }
      if (!mounted || request != _request) return;
      setState(() {
        _visuals[pet.id] = _PetVisual(presentation: presentation, pixa: pixa);
      });
    } catch (_) {
      // Keep the cover usable even if its presentation is temporarily missing.
    }
  }

  Future<void> _adopt() async {
    final client = _client;
    if (client == null || _adopting) return;
    final name = await _askPetName(context);
    if (name == null || !mounted) return;
    setState(() {
      _adopting = true;
      _error = null;
    });
    try {
      final response = await client.adoptPet(
        displayName: name.trim().isEmpty ? null : name.trim(),
      );
      await _loadPets();
      if (mounted) context.push('/pet/${response.value.pet.id}');
    } catch (error) {
      if (mounted) setState(() => _error = error);
    } finally {
      if (mounted) setState(() => _adopting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    if (_client == null) {
      return _PetMessagePage(
        title: 'Pet',
        message: data.connectionState == MobileConnectionState.connecting
            ? 'Connecting to your pets...'
            : 'Connect to GizClaw to meet your pets.',
        loading: data.connectionState == MobileConnectionState.connecting,
      );
    }
    if (_loading && _pets.isEmpty) {
      return const _PetMessagePage(
        title: 'Pet',
        message: 'Looking for your pets...',
        loading: true,
      );
    }
    if (_pets.isEmpty) {
      return _PetEmptyPage(
        adopting: _adopting,
        error: _error,
        onAdopt: _adopt,
        onRetry: _loadPets,
      );
    }

    return CupertinoPageScaffold(
      child: SafeArea(
        bottom: false,
        child: CustomScrollView(
          key: const PageStorageKey('pet-covers'),
          slivers: [
            CupertinoSliverRefreshControl(onRefresh: _loadPets),
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 112),
              sliver: SliverList.list(
                children: [
                  _PetPageHeader(adopting: _adopting, onAdopt: _adopt),
                  if (_error != null) ...[
                    const SizedBox(height: 10),
                    Text(
                      _petError(_error!),
                      style: GizText.body.copyWith(
                        color: CupertinoColors.systemRed.resolveFrom(context),
                      ),
                    ),
                  ],
                  const SizedBox(height: 20),
                  if (_pets.length == 1)
                    _PetCoverCard(
                      pet: _pets.first,
                      visual: _visuals[_pets.first.id],
                      onPressed: () => context.push('/pet/${_pets.first.id}'),
                    ),
                  if (_pets.length > 1)
                    GridView.builder(
                      padding: EdgeInsets.zero,
                      shrinkWrap: true,
                      physics: const NeverScrollableScrollPhysics(),
                      gridDelegate:
                          const SliverGridDelegateWithFixedCrossAxisCount(
                            crossAxisCount: 2,
                            crossAxisSpacing: 12,
                            mainAxisSpacing: 12,
                            childAspectRatio: 0.78,
                          ),
                      itemCount: _pets.length,
                      itemBuilder: (context, index) => _PetCoverCard(
                        pet: _pets[index],
                        visual: _visuals[_pets[index].id],
                        compact: true,
                        onPressed: () =>
                            context.push('/pet/${_pets[index].id}'),
                      ),
                    ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class PetDetailPage extends StatefulWidget {
  const PetDetailPage({super.key, required this.petId});

  final String petId;

  @override
  State<PetDetailPage> createState() => _PetDetailPageState();
}

class _PetDetailPageState extends State<PetDetailPage> {
  final _actionFabKey = GlobalKey<_PetActionFabState>();
  GizClawClient? _client;
  WorkspaceChatController? _chat;
  String? _chatWorkspaceName;
  Pet? _pet;
  PetPresentation? _presentation;
  PixaAsset? _pixa;
  Object? _error;
  bool _loading = false;
  bool _statusVisible = true;
  String? _clipName;
  String? _drivingAction;
  int _request = 0;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final data = MobileDataScope.watch(context);
    final client = data.connectionState == MobileConnectionState.connected
        ? data.connection.client
        : null;
    if (identical(client, _client)) return;
    _replaceChat(null);
    _client = client;
    _request += 1;
    if (client == null) {
      setState(() {
        _pet = null;
        _presentation = null;
        _pixa = null;
        _loading = false;
      });
      return;
    }
    unawaited(_load(data));
  }

  @override
  void dispose() {
    _request += 1;
    _replaceChat(null);
    super.dispose();
  }

  void _handleChatChanged() {
    if (mounted) setState(() {});
  }

  void _replaceChat(WorkspaceChatController? chat, [String? workspaceName]) {
    if (identical(chat, _chat)) return;
    _chat?.removeListener(_handleChatChanged);
    _chat?.dispose();
    _chat = chat;
    _chatWorkspaceName = workspaceName;
    if (chat != null) {
      chat.addListener(_handleChatChanged);
      unawaited(chat.start());
    }
  }

  Future<void> _load(MobileDataController data) async {
    final client = _client;
    if (client == null || _loading) return;
    final request = ++_request;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final pet = (await client.getPet(widget.petId)).value;
      final presentation = (await client.getPetPresentation(
        widget.petId,
      )).value;
      PixaAsset? pixa;
      Object? pixaError;
      try {
        pixa = (await client.downloadPetPixa(widget.petId)).asset;
      } catch (error) {
        pixaError = error;
      }
      if (!mounted || request != _request) return;
      if (_chatWorkspaceName != pet.workspaceName) {
        _replaceChat(
          data.createWorkspaceChat(pet.workspaceName),
          pet.workspaceName,
        );
      }
      setState(() {
        _pet = pet;
        _presentation = presentation;
        _pixa = pixa;
        _clipName = _defaultClip(presentation, pet);
        _loading = false;
        _error = pixaError;
      });
    } catch (error) {
      if (!mounted || request != _request) return;
      setState(() {
        _loading = false;
        _error = error;
      });
    }
  }

  Future<void> _drive(PetPresentationActionSpec action) async {
    final client = _client;
    final pet = _pet;
    if (client == null || pet == null || _drivingAction != null) return;
    final actionClip = _clipForAction(_presentation, action.id);
    final duration = _clipDuration(_pixa, actionClip);
    setState(() {
      _drivingAction = action.id;
      _error = null;
      _clipName = actionClip ?? _clipName;
    });
    try {
      final animation = Future<void>.delayed(duration);
      final response = await client.drivePet(pet.id, action: action.id);
      if (!mounted) return;
      setState(() => _pet = response.value.pet);
      await animation;
      if (!mounted) return;
      setState(() {
        _drivingAction = null;
        _clipName = _defaultClip(_presentation, response.value.pet);
      });
    } catch (error) {
      if (!mounted) return;
      setState(() {
        _drivingAction = null;
        _clipName = _defaultClip(_presentation, _pet);
        _error = error;
      });
    }
  }

  Future<void> _activateMenuAction(_PetMenuAction action) async {
    final driveAction = action.driveAction;
    if (driveAction != null) {
      await _drive(driveAction);
      return;
    }
    if (_drivingAction != null) return;
    final duration = _clipDuration(_pixa, action.clipName);
    setState(() {
      _drivingAction = action.id;
      _error = null;
      _clipName = action.clipName;
    });
    await Future<void>.delayed(duration);
    if (!mounted || _drivingAction != action.id) return;
    setState(() {
      _drivingAction = null;
      _clipName = _defaultClip(_presentation, _pet);
    });
  }

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    if (_client == null) {
      return _PetDetailMessage(
        message: data.connectionState == MobileConnectionState.connecting
            ? 'Connecting...'
            : 'Pet is unavailable while disconnected.',
        loading: data.connectionState == MobileConnectionState.connecting,
      );
    }
    if (_loading && _pet == null) {
      return const _PetDetailMessage(
        message: 'Waking your pet...',
        loading: true,
      );
    }
    final pet = _pet;
    if (pet == null) {
      return _PetDetailMessage(
        message: _error == null ? 'Pet not found.' : _petError(_error!),
        loading: false,
        onRetry: () => _load(MobileDataScope.watch(context)),
      );
    }

    final catalog = _catalogFor(context, _presentation);
    final metrics = _petMetrics(pet, catalog).take(4).toList();
    final progression = pet.progression.value.entries.isEmpty
        ? pet.rulesetName
        : pet.progression.value.entries
              .map((entry) => '${_title(entry.key)} ${entry.value}')
              .join('  |  ');
    final actions = _petMenuActions(_presentation);
    final chat = _chat;
    final messages = chat?.messages ?? const <WorkspaceChatMessage>[];
    return CupertinoPageScaffold(
      backgroundColor: _petDetailBackground,
      child: Stack(
        fit: StackFit.expand,
        children: [
          const Positioned.fill(child: _PetMosaicBackground()),
          Positioned(
            left: 14,
            right: 14,
            top: MediaQuery.paddingOf(context).top + 86,
            bottom: MediaQuery.paddingOf(context).bottom + 106,
            child: _PetConversationDrift(messages: messages),
          ),
          Positioned(
            left: 18,
            top: MediaQuery.paddingOf(context).top + 12,
            child: _SceneButton(
              label: 'Back',
              icon: CupertinoIcons.back,
              onPressed: () => context.pop(),
            ),
          ),
          Positioned(
            left: 76,
            right: 20,
            top: MediaQuery.paddingOf(context).top + 14,
            child: Text(
              _petName(pet, catalog),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: GizText.sectionTitle,
            ),
          ),
          Positioned(
            left: 20,
            right: 20,
            top: MediaQuery.paddingOf(context).top + 76,
            bottom: MediaQuery.paddingOf(context).bottom + 106,
            child: SingleChildScrollView(
              child: _PetGameConsole(
                pixa: _pixa,
                clipName: _clipName,
                loading: _loading,
              ),
            ),
          ),
          if (_error != null)
            Positioned(
              left: 72,
              right: 18,
              bottom: MediaQuery.paddingOf(context).bottom + 108,
              child: _PetErrorToast(error: _petError(_error!)),
            ),
          Positioned(
            left: 0,
            right: 0,
            bottom: MediaQuery.paddingOf(context).bottom + 8,
            child: Center(
              child: SizedBox(
                width: 230,
                height: 78,
                child: Stack(
                  clipBehavior: Clip.none,
                  alignment: Alignment.bottomCenter,
                  children: [
                    Positioned(
                      left: 0,
                      bottom: 10,
                      child: _PetActionFab(
                        key: _actionFabKey,
                        actions: actions,
                        catalog: catalog,
                        activeAction: _drivingAction,
                        onAction: _activateMenuAction,
                        onExpand: () {
                          if (_statusVisible) {
                            setState(() => _statusVisible = false);
                          }
                        },
                      ),
                    ),
                    Positioned(
                      left: 122,
                      bottom: 88,
                      width: 158,
                      child: IgnorePointer(
                        ignoring: !_statusVisible,
                        child: AnimatedSlide(
                          offset: _statusVisible
                              ? Offset.zero
                              : const Offset(0, 0.12),
                          duration: const Duration(milliseconds: 240),
                          curve: Curves.easeOutCubic,
                          child: AnimatedScale(
                            scale: _statusVisible ? 1 : 0.94,
                            alignment: Alignment.bottomCenter,
                            duration: const Duration(milliseconds: 240),
                            curve: Curves.easeOutCubic,
                            child: AnimatedOpacity(
                              opacity: _statusVisible ? 1 : 0,
                              duration: const Duration(milliseconds: 180),
                              child: _PetStatusNameplate(
                                metrics: metrics,
                                progression: progression,
                                title: _petName(pet, catalog),
                                visible: _statusVisible,
                              ),
                            ),
                          ),
                        ),
                      ),
                    ),
                    _PetVoiceFab(chat: chat),
                    Positioned(
                      right: 0,
                      bottom: 10,
                      child: _PetStatusFab(
                        visible: _statusVisible,
                        onPressed: () {
                          _actionFabKey.currentState?.collapse();
                          setState(() => _statusVisible = !_statusVisible);
                        },
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _PetConversationDrift extends StatelessWidget {
  const _PetConversationDrift({required this.messages});

  final List<WorkspaceChatMessage> messages;

  @override
  Widget build(BuildContext context) {
    final visible = messages
        .where((message) => message.text.trim().isNotEmpty)
        .toList(growable: false)
        .reversed
        .take(6)
        .toList(growable: false);
    return IgnorePointer(
      child: LayoutBuilder(
        builder: (context, constraints) {
          return Stack(
            clipBehavior: Clip.none,
            children: [
              for (var index = 0; index < visible.length; index++)
                AnimatedPositioned(
                  key: ValueKey(visible[index].id),
                  duration: const Duration(milliseconds: 520),
                  curve: Curves.easeOutCubic,
                  left: visible[index].incoming
                      ? 0
                      : constraints.maxWidth * 0.25,
                  right: visible[index].incoming
                      ? constraints.maxWidth * 0.23
                      : 0,
                  bottom: 30 + index * 72,
                  child: TweenAnimationBuilder<double>(
                    tween: Tween(begin: 18, end: 0),
                    duration: const Duration(milliseconds: 460),
                    curve: Curves.easeOutCubic,
                    builder: (context, offset, child) => Transform.translate(
                      offset: Offset(0, offset),
                      child: child,
                    ),
                    child: AnimatedOpacity(
                      opacity: (0.72 - index * 0.11).clamp(0.14, 0.72),
                      duration: const Duration(milliseconds: 520),
                      curve: Curves.easeOutCubic,
                      child: _PetDriftingMessage(message: visible[index]),
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

class _PetDriftingMessage extends StatelessWidget {
  const _PetDriftingMessage({required this.message});

  final WorkspaceChatMessage message;

  @override
  Widget build(BuildContext context) {
    return GizSquircle(
      borderRadius: BorderRadius.circular(12),
      child: BackdropFilter(
        filter: ImageFilter.blur(sigmaX: 9, sigmaY: 9),
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: message.incoming
                ? const Color(0x8CFFFFFF)
                : const Color(0x8C18342D),
          ),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 9),
            child: Text(
              message.text.trim(),
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
              style: GizText.body.copyWith(
                color: message.incoming ? GizColors.ink : GizColors.surface,
                fontSize: 12,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _PetMosaicBackground extends StatefulWidget {
  const _PetMosaicBackground();

  @override
  State<_PetMosaicBackground> createState() => _PetMosaicBackgroundState();
}

class _PetMosaicBackgroundState extends State<_PetMosaicBackground>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(
      vsync: this,
      duration: const Duration(seconds: 14),
    );
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (MediaQuery.disableAnimationsOf(context)) {
      _controller
        ..stop()
        ..value = 0.28;
    } else if (!_controller.isAnimating) {
      _controller.repeat();
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return RepaintBoundary(
      child: AnimatedBuilder(
        animation: _controller,
        builder: (context, _) =>
            CustomPaint(painter: _PetMosaicPainter(_controller.value)),
      ),
    );
  }
}

class _PetMosaicPainter extends CustomPainter {
  const _PetMosaicPainter(this.progress);

  final double progress;

  static const _palette = [
    Color(0xFFD9E8E0),
    Color(0xFFCFE4E3),
    Color(0xFFC4DDD3),
    Color(0xFFDDE8BE),
    Color(0xFFD8D7E6),
    Color(0xFFCADFD8),
  ];

  @override
  void paint(Canvas canvas, Size size) {
    const cellSize = 14.0;
    final columns = (size.width / cellSize).ceil();
    final rows = (size.height / cellSize).ceil();
    final phase = progress * math.pi * 2;
    final timeX = math.cos(phase);
    final timeY = math.sin(phase);

    for (var row = 0; row < rows; row++) {
      for (var column = 0; column < columns; column++) {
        final x = (column + 0.5) / columns;
        final y = (row + 0.5) / rows;
        final warpX =
            math.sin(
              (x * 1.37 + y * 0.71) * math.pi * 2 + timeX * 1.15 + timeY * 0.42,
            ) *
            0.13;
        final warpY =
            math.cos(
              (x * 0.63 - y * 1.43) * math.pi * 2 + timeX * 0.36 - timeY * 1.08,
            ) *
            0.11;
        final warpedX = x + warpX;
        final warpedY = y + warpY;
        final primary = math.sin(
          (warpedX * 0.91 + warpedY * 0.62) * math.pi * 2 +
              timeX * 0.82 +
              timeY * 1.26,
        );
        final crossWave = math.cos(
          (warpedX * 0.47 - warpedY * 1.16) * math.pi * 2 -
              timeX * 1.31 +
              timeY * 0.55,
        );
        final detail = math.sin(
          (warpedX * 2.21 + warpedY * 1.73) * math.pi * 2 +
              timeX * 0.27 -
              timeY * 0.68,
        );
        final hash = ((column * 73856093) ^ (row * 19349663)) & 0xff;
        final jitter = (hash / 255 - 0.5) * 0.045;
        final value =
            (0.5 + primary * 0.23 + crossWave * 0.14 + detail * 0.055 + jitter)
                .clamp(0.0, 1.0);
        canvas.drawRect(
          Rect.fromLTWH(column * cellSize, row * cellSize, cellSize, cellSize),
          Paint()..color = _colorAt(value),
        );
      }
    }

    final gridPaint = Paint()
      ..color = const Color(0x18FFFFFF)
      ..strokeWidth = 0.5;
    for (var column = 1; column < columns; column++) {
      final x = column * cellSize;
      canvas.drawLine(Offset(x, 0), Offset(x, size.height), gridPaint);
    }
    for (var row = 1; row < rows; row++) {
      final y = row * cellSize;
      canvas.drawLine(Offset(0, y), Offset(size.width, y), gridPaint);
    }
  }

  Color _colorAt(double value) {
    final scaled = value * (_palette.length - 1);
    final lower = scaled.floor().clamp(0, _palette.length - 1);
    final upper = math.min(lower + 1, _palette.length - 1);
    final blend = Curves.easeInOut.transform(scaled - lower);
    return Color.lerp(_palette[lower], _palette[upper], blend)!;
  }

  @override
  bool shouldRepaint(covariant _PetMosaicPainter oldDelegate) {
    return oldDelegate.progress != progress;
  }
}

class _PetPageHeader extends StatelessWidget {
  const _PetPageHeader({required this.adopting, required this.onAdopt});

  final bool adopting;
  final VoidCallback onAdopt;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        const Expanded(child: Text('Pet', style: GizText.pageTitle)),
        Semantics(
          label: 'Adopt a pet',
          button: true,
          child: CupertinoButton(
            padding: EdgeInsets.zero,
            minimumSize: const Size(44, 44),
            onPressed: adopting ? null : onAdopt,
            child: adopting
                ? const CupertinoActivityIndicator()
                : const Icon(CupertinoIcons.add_circled_solid, size: 30),
          ),
        ),
      ],
    );
  }
}

class _PetCoverCard extends StatelessWidget {
  const _PetCoverCard({
    required this.pet,
    required this.visual,
    required this.onPressed,
    this.compact = false,
  });

  final Pet pet;
  final _PetVisual? visual;
  final VoidCallback onPressed;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final catalog = _catalogFor(context, visual?.presentation);
    return GizPressable(
      onPressed: onPressed,
      borderRadius: BorderRadius.circular(12),
      scaleWhenPressed: 0.975,
      child: AspectRatio(
        aspectRatio: compact ? 0.78 : 0.86,
        child: GizSquircle(
          borderRadius: BorderRadius.circular(12),
          child: Stack(
            fit: StackFit.expand,
            children: [
              const _PetMosaicBackground(),
              const DecoratedBox(
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.topCenter,
                    end: Alignment.bottomCenter,
                    colors: [
                      Color(0x00FFFFFF),
                      Color(0x0AFFFFFF),
                      Color(0x40101916),
                    ],
                    stops: [0, 0.58, 1],
                  ),
                ),
              ),
              Positioned(
                left: compact ? 16 : 48,
                right: compact ? 16 : 48,
                top: compact ? 38 : 64,
                bottom: compact ? 82 : 124,
                child: visual?.pixa == null
                    ? const Center(child: CupertinoActivityIndicator())
                    : GizSquircle(
                        borderRadius: BorderRadius.circular(compact ? 13 : 18),
                        child: ColoredBox(
                          color: const Color(0x8CFFFFFF),
                          child: Padding(
                            padding: const EdgeInsets.all(2),
                            child: GizSquircle(
                              borderRadius: BorderRadius.circular(
                                compact ? 11 : 16,
                              ),
                              child: ColoredBox(
                                color: _petSceneColor,
                                child: Padding(
                                  padding: EdgeInsets.all(compact ? 6 : 10),
                                  child: _PetCoverSprite(
                                    child: _AnimatedPetSprite(
                                      asset: visual!.pixa!,
                                      clipName: _defaultClip(
                                        visual!.presentation,
                                        pet,
                                      ),
                                    ),
                                  ),
                                ),
                              ),
                            ),
                          ),
                        ),
                      ),
              ),
              Positioned(
                top: compact ? 12 : 16,
                right: compact ? 12 : 16,
                child: GizSquircle(
                  borderRadius: BorderRadius.circular(9),
                  child: BackdropFilter(
                    filter: ImageFilter.blur(sigmaX: 10, sigmaY: 10),
                    child: Container(
                      color: const Color(0x8CFFFFFF),
                      padding: const EdgeInsets.symmetric(
                        horizontal: 9,
                        vertical: 7,
                      ),
                      child: Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          const SizedBox.square(
                            dimension: 6,
                            child: ColoredBox(color: GizColors.accent),
                          ),
                          const SizedBox(width: 6),
                          Text(
                            _petStateLabel(visual?.presentation, pet),
                            style: GizText.label.copyWith(fontSize: 9),
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
              Positioned(
                left: 0,
                right: 0,
                bottom: 0,
                child: ClipRect(
                  child: BackdropFilter(
                    filter: ImageFilter.blur(sigmaX: 18, sigmaY: 18),
                    child: Container(
                      color: const Color(0x9E101916),
                      padding: EdgeInsets.fromLTRB(
                        compact ? 13 : 18,
                        compact ? 11 : 15,
                        compact ? 11 : 14,
                        compact ? 12 : 16,
                      ),
                      child: Row(
                        children: [
                          Expanded(
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              mainAxisSize: MainAxisSize.min,
                              children: [
                                Text(
                                  _petName(pet, catalog),
                                  maxLines: 1,
                                  overflow: TextOverflow.ellipsis,
                                  style: GizText.sectionTitle.copyWith(
                                    color: GizColors.surface,
                                    fontSize: compact ? 15 : null,
                                  ),
                                ),
                                if (!compact) ...[
                                  const SizedBox(height: 4),
                                  Text(
                                    '${pet.rulesetName}  |  ${_petProgressionLabel(pet)}',
                                    maxLines: 1,
                                    overflow: TextOverflow.ellipsis,
                                    style: GizText.label.copyWith(
                                      color: const Color(0xBFFFFFFF),
                                      fontSize: 9,
                                    ),
                                  ),
                                ],
                              ],
                            ),
                          ),
                          const SizedBox(width: 8),
                          GizIconTile(
                            icon: CupertinoIcons.arrow_up_right,
                            size: compact ? 34 : 38,
                            iconSize: compact ? 16 : 18,
                            backgroundColor: const Color(0x2EFFFFFF),
                            foregroundColor: GizColors.surface,
                          ),
                        ],
                      ),
                    ),
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

class _PetCoverSprite extends StatefulWidget {
  const _PetCoverSprite({required this.child});

  final Widget child;

  @override
  State<_PetCoverSprite> createState() => _PetCoverSpriteState();
}

class _PetCoverSpriteState extends State<_PetCoverSprite>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 2600),
  )..repeat(reverse: true);

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _controller,
      child: widget.child,
      builder: (context, child) => Transform.translate(
        offset: Offset(0, -5 * Curves.easeInOut.transform(_controller.value)),
        child: child,
      ),
    );
  }
}

class _PetVisual {
  const _PetVisual({required this.presentation, required this.pixa});

  final PetPresentation presentation;
  final PixaAsset? pixa;
}

class _AnimatedPetSprite extends StatefulWidget {
  const _AnimatedPetSprite({required this.asset, required this.clipName});

  final PixaAsset asset;
  final String? clipName;

  @override
  State<_AnimatedPetSprite> createState() => _AnimatedPetSpriteState();
}

class _AnimatedPetSpriteState extends State<_AnimatedPetSprite> {
  late final Timer _timer;
  Duration _elapsed = Duration.zero;

  @override
  void initState() {
    super.initState();
    _timer = Timer.periodic(const Duration(milliseconds: 80), (_) {
      if (mounted) {
        setState(() => _elapsed += const Duration(milliseconds: 80));
      }
    });
  }

  @override
  void didUpdateWidget(covariant _AnimatedPetSprite oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (!identical(oldWidget.asset, widget.asset) ||
        oldWidget.clipName != widget.clipName) {
      _elapsed = Duration.zero;
    }
  }

  @override
  void dispose() {
    _timer.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return PixaSprite(
      asset: widget.asset,
      clipName: widget.clipName,
      elapsed: _elapsed,
      fit: BoxFit.contain,
    );
  }
}

class _PetGameConsole extends StatelessWidget {
  const _PetGameConsole({
    required this.pixa,
    required this.clipName,
    required this.loading,
  });

  final PixaAsset? pixa;
  final String? clipName;
  final bool loading;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 350),
        child: AspectRatio(
          aspectRatio: 1,
          child: _PetDevice(pixa: pixa, clipName: clipName, loading: loading),
        ),
      ),
    );
  }
}

class _PetStatusFab extends StatelessWidget {
  const _PetStatusFab({required this.visible, required this.onPressed});

  final bool visible;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: visible ? 'Hide pet status' : 'Show pet status',
      button: true,
      child: GestureDetector(
        onTap: onPressed,
        child: Container(
          width: 58,
          height: 58,
          decoration: BoxDecoration(
            color: GizColors.ink,
            shape: BoxShape.circle,
            border: Border.all(color: const Color(0x2EFFFFFF)),
            boxShadow: const [
              BoxShadow(
                color: Color(0x33000000),
                blurRadius: 20,
                offset: Offset(0, 8),
              ),
            ],
          ),
          child: AnimatedSwitcher(
            duration: const Duration(milliseconds: 180),
            transitionBuilder: (child, animation) => RotationTransition(
              turns: Tween(begin: 0.86, end: 1.0).animate(animation),
              child: FadeTransition(opacity: animation, child: child),
            ),
            child: Icon(
              visible ? CupertinoIcons.xmark : CupertinoIcons.waveform_path_ecg,
              key: ValueKey(visible),
              color: GizColors.surface,
              size: visible ? 22 : 25,
            ),
          ),
        ),
      ),
    );
  }
}

class _PetVoiceFab extends StatefulWidget {
  const _PetVoiceFab({required this.chat});

  final WorkspaceChatController? chat;

  @override
  State<_PetVoiceFab> createState() => _PetVoiceFabState();
}

class _PetVoiceFabState extends State<_PetVoiceFab>
    with SingleTickerProviderStateMixin {
  late final AnimationController _pulse = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1300),
  )..repeat(reverse: true);

  @override
  void dispose() {
    _pulse.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final chat = widget.chat;
    final enabled = chat?.canRecord ?? false;
    final recording = chat?.recording ?? false;
    final preparing = chat?.startingInput ?? false;
    final label = recording
        ? 'Release to send voice'
        : preparing
        ? 'Opening microphone'
        : enabled
        ? 'Hold to speak'
        : 'Voice unavailable';
    return Listener(
      onPointerDown: enabled ? (_) => unawaited(chat!.startInput()) : null,
      onPointerUp: enabled ? (_) => unawaited(chat!.finishInput()) : null,
      onPointerCancel: enabled
          ? (_) => unawaited(chat!.finishInput(error: 'recording canceled'))
          : null,
      child: Semantics(
        button: true,
        enabled: enabled,
        label: label,
        child: AnimatedBuilder(
          animation: _pulse,
          builder: (context, child) {
            final energy = recording ? _pulse.value : 0.0;
            return AnimatedScale(
              scale: recording ? 0.92 : 1,
              duration: const Duration(milliseconds: 140),
              curve: Curves.easeOutCubic,
              child: Container(
                width: 78,
                height: 78,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: enabled ? GizColors.accent : const Color(0xFF91A099),
                  border: Border.all(color: const Color(0x66FFFFFF)),
                  boxShadow: [
                    BoxShadow(
                      color: (enabled ? GizColors.accent : GizColors.ink)
                          .withValues(alpha: 0.2 + energy * 0.3),
                      blurRadius: 22 + energy * 18,
                      spreadRadius: 2 + energy * 7,
                      offset: const Offset(0, 8),
                    ),
                  ],
                ),
                child: child,
              ),
            );
          },
          child: preparing
              ? const CupertinoActivityIndicator(color: GizColors.ink)
              : Icon(
                  recording ? CupertinoIcons.waveform : CupertinoIcons.mic_fill,
                  size: recording ? 29 : 27,
                  color: enabled ? GizColors.ink : const Color(0xCCFFFFFF),
                ),
        ),
      ),
    );
  }
}

class _PetDevice extends StatelessWidget {
  const _PetDevice({
    required this.pixa,
    required this.clipName,
    required this.loading,
  });

  final PixaAsset? pixa;
  final String? clipName;
  final bool loading;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final extent = constraints.maxWidth;
        final shellExtent = extent - 24;
        return Stack(
          children: [
            Positioned(
              left: 12 + shellExtent * 0.307,
              top: 12 + shellExtent * 0.287,
              width: shellExtent * 0.386,
              height: shellExtent * 0.392,
              child: ClipRSuperellipse(
                borderRadius: BorderRadius.circular(extent * 0.018),
                child: ColoredBox(
                  color: _petSceneColor,
                  child: Padding(
                    padding: EdgeInsets.all(extent * 0.025),
                    child: pixa == null
                        ? Center(
                            child: loading
                                ? const CupertinoActivityIndicator(
                                    color: GizColors.ink,
                                  )
                                : const Icon(
                                    CupertinoIcons.sparkles,
                                    color: GizColors.secondaryInk,
                                    size: 36,
                                  ),
                          )
                        : _AnimatedPetSprite(asset: pixa!, clipName: clipName),
                  ),
                ),
              ),
            ),
            Positioned.fill(
              child: Padding(
                padding: const EdgeInsets.all(12),
                child: Image.asset(
                  'assets/pet/digipet-console.png',
                  fit: BoxFit.contain,
                  filterQuality: FilterQuality.high,
                ),
              ),
            ),
          ],
        );
      },
    );
  }
}

class _PetStatusNameplate extends StatefulWidget {
  const _PetStatusNameplate({
    required this.metrics,
    required this.progression,
    required this.title,
    required this.visible,
  });

  final List<_PetMetric> metrics;
  final String progression;
  final String title;
  final bool visible;

  @override
  State<_PetStatusNameplate> createState() => _PetStatusNameplateState();
}

class _PetStatusNameplateState extends State<_PetStatusNameplate>
    with TickerProviderStateMixin {
  late final AnimationController _scanController;
  late final AnimationController _pulseController;

  static const _colors = [
    GizColors.accent,
    Color(0xFFFF9470),
    Color(0xFF55BDA7),
    Color(0xFFA690D2),
  ];

  @override
  void initState() {
    super.initState();
    _scanController = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 720),
    );
    _pulseController = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1400),
    )..repeat(reverse: true);
    if (widget.visible) _scanController.forward();
  }

  @override
  void didUpdateWidget(covariant _PetStatusNameplate oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.visible && !oldWidget.visible) {
      _scanController.forward(from: 0);
    }
  }

  @override
  void dispose() {
    _scanController.dispose();
    _pulseController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(18),
        boxShadow: [
          BoxShadow(
            color: const Color(0xFF17241F).withValues(alpha: 0.28),
            blurRadius: 26,
            offset: const Offset(0, 12),
          ),
        ],
      ),
      child: ClipRSuperellipse(
        borderRadius: BorderRadius.circular(18),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 19, sigmaY: 19),
          child: DecoratedBox(
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(18),
              border: Border.all(color: const Color(0x70FFFFFF)),
              gradient: const LinearGradient(
                begin: Alignment.topLeft,
                end: Alignment.bottomRight,
                colors: [
                  Color(0xA31A3029),
                  Color(0x941D4145),
                  Color(0x9C46354D),
                ],
                stops: [0, 0.56, 1],
              ),
            ),
            child: Stack(
              children: [
                const Positioned.fill(
                  child: IgnorePointer(
                    child: DecoratedBox(
                      decoration: BoxDecoration(
                        gradient: LinearGradient(
                          begin: Alignment.topLeft,
                          end: Alignment.bottomRight,
                          colors: [
                            Color(0x32FFFFFF),
                            Color(0x08FFFFFF),
                            Color(0x001EDEB1),
                          ],
                          stops: [0, 0.38, 0.72],
                        ),
                      ),
                    ),
                  ),
                ),
                Padding(
                  padding: const EdgeInsets.fromLTRB(14, 12, 13, 13),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Row(
                        children: [
                          _HudStatusIndicator(animation: _pulseController),
                          const SizedBox(width: 8),
                          Text(
                            'VITALS',
                            style: GizText.label.copyWith(
                              color: GizColors.surface,
                              fontSize: 9,
                            ),
                          ),
                          const Spacer(),
                          Text(
                            widget.progression.toUpperCase(),
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: GizText.label.copyWith(
                              color: const Color(0xA8FFFFFF),
                              fontSize: 8,
                            ),
                          ),
                        ],
                      ),
                      const SizedBox(height: 5),
                      Text(
                        widget.title.toUpperCase(),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: GizText.label.copyWith(
                          color: const Color(0xBFFFFFFF),
                          fontSize: 8,
                        ),
                      ),
                      const SizedBox(height: 7),
                      Container(height: 1, color: const Color(0x24FFFFFF)),
                      const SizedBox(height: 10),
                      for (
                        var index = 0;
                        index < widget.metrics.length;
                        index++
                      ) ...[
                        _NameplateMetric(
                          metric: widget.metrics[index],
                          color: _colors[index % _colors.length],
                        ),
                        if (index != widget.metrics.length - 1)
                          const SizedBox(height: 10),
                      ],
                    ],
                  ),
                ),
                Positioned.fill(
                  child: IgnorePointer(
                    child: AnimatedBuilder(
                      animation: _scanController,
                      builder: (context, child) {
                        final progress = _scanController.value;
                        return FractionalTranslation(
                          translation: Offset(0, progress - 0.5),
                          child: Opacity(
                            opacity: math.sin(progress * math.pi) * 0.48,
                            child: child,
                          ),
                        );
                      },
                      child: Align(
                        alignment: Alignment.center,
                        child: Container(
                          height: 18,
                          decoration: const BoxDecoration(
                            gradient: LinearGradient(
                              begin: Alignment.topCenter,
                              end: Alignment.bottomCenter,
                              colors: [
                                Color(0x001EDEB1),
                                Color(0x801EDEB1),
                                Color(0x001EDEB1),
                              ],
                            ),
                          ),
                        ),
                      ),
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _HudStatusIndicator extends StatelessWidget {
  const _HudStatusIndicator({required this.animation});

  final Animation<double> animation;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: animation,
      builder: (context, child) {
        return Container(
          width: 7,
          height: 7,
          decoration: BoxDecoration(
            color: Color.lerp(
              const Color(0xFF159878),
              GizColors.accent,
              animation.value,
            ),
            boxShadow: [
              BoxShadow(
                color: GizColors.accent.withValues(
                  alpha: 0.18 + animation.value * 0.42,
                ),
                blurRadius: 4 + animation.value * 7,
              ),
            ],
          ),
        );
      },
    );
  }
}

class _NameplateMetric extends StatelessWidget {
  const _NameplateMetric({required this.metric, required this.color});

  final _PetMetric metric;
  final Color color;

  @override
  Widget build(BuildContext context) {
    const segmentCount = 8;
    final activeSegments = ((metric.value / 100) * segmentCount).ceil().clamp(
      0,
      segmentCount,
    );
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            Expanded(
              child: Text(
                metric.label.toUpperCase(),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: GizText.label.copyWith(
                  color: const Color(0xBFFFFFFF),
                  fontSize: 9,
                ),
              ),
            ),
            const SizedBox(width: 6),
            Text(
              '${metric.value}',
              textAlign: TextAlign.right,
              style: GizText.label.copyWith(color: GizColors.surface),
            ),
          ],
        ),
        const SizedBox(height: 5),
        Row(
          children: List.generate(
            segmentCount,
            (index) => Expanded(
              child: Container(
                height: 6,
                margin: EdgeInsets.only(
                  right: index == segmentCount - 1 ? 0 : 2,
                ),
                color: index < activeSegments ? color : const Color(0x24FFFFFF),
              ),
            ),
          ),
        ),
      ],
    );
  }
}

class _PetMenuAction {
  const _PetMenuAction({
    required this.id,
    required this.clipName,
    this.driveAction,
    this.icon,
  });

  final String id;
  final String? clipName;
  final PetPresentationActionSpec? driveAction;
  final String? icon;
}

class _PetActionFab extends StatefulWidget {
  const _PetActionFab({
    super.key,
    required this.actions,
    required this.catalog,
    required this.activeAction,
    required this.onAction,
    required this.onExpand,
  });

  final List<_PetMenuAction> actions;
  final PetPresentationI18nCatalog? catalog;
  final String? activeAction;
  final ValueChanged<_PetMenuAction> onAction;
  final VoidCallback onExpand;

  @override
  State<_PetActionFab> createState() => _PetActionFabState();
}

class _PetActionFabState extends State<_PetActionFab>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;
  bool _expanded = false;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 320),
      reverseDuration: const Duration(milliseconds: 220),
    );
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _toggle() {
    if (widget.actions.isEmpty) return;
    setState(() => _expanded = !_expanded);
    if (_expanded) {
      widget.onExpand();
      _controller.forward();
    } else {
      _controller.reverse();
    }
  }

  void collapse() {
    if (!_expanded) return;
    setState(() => _expanded = false);
    _controller.reverse();
  }

  void _select(_PetMenuAction action) {
    if (widget.activeAction != null) return;
    setState(() => _expanded = false);
    _controller.reverse();
    widget.onAction(action);
  }

  @override
  Widget build(BuildContext context) {
    final menuHeight =
        math.max(0, widget.actions.length - 1) * 52.0 +
        (widget.actions.isEmpty ? 0 : 58.0);
    return SizedBox(
      width: 210,
      height: menuHeight + 64,
      child: AnimatedBuilder(
        animation: _controller,
        builder: (context, _) {
          return Stack(
            clipBehavior: Clip.none,
            alignment: Alignment.bottomLeft,
            children: [
              for (var index = 0; index < widget.actions.length; index++)
                _buildAction(widget.actions[index], index),
              Semantics(
                label: _expanded ? 'Close pet actions' : 'Open pet actions',
                button: true,
                child: GestureDetector(
                  onTap: _toggle,
                  child: Container(
                    width: 58,
                    height: 58,
                    decoration: BoxDecoration(
                      color: GizColors.ink,
                      shape: BoxShape.circle,
                      boxShadow: const [
                        BoxShadow(
                          color: Color(0x33000000),
                          blurRadius: 20,
                          offset: Offset(0, 8),
                        ),
                      ],
                    ),
                    child: widget.activeAction == null
                        ? Stack(
                            alignment: Alignment.center,
                            children: [
                              Opacity(
                                opacity: 1 - _controller.value,
                                child: Transform.scale(
                                  scale: 1 - _controller.value * 0.2,
                                  child: const Icon(
                                    CupertinoIcons.game_controller_solid,
                                    color: GizColors.surface,
                                    size: 27,
                                  ),
                                ),
                              ),
                              Opacity(
                                opacity: _controller.value,
                                child: Transform.rotate(
                                  angle: (1 - _controller.value) * -math.pi / 4,
                                  child: const Icon(
                                    CupertinoIcons.xmark,
                                    color: GizColors.surface,
                                    size: 23,
                                  ),
                                ),
                              ),
                            ],
                          )
                        : const CupertinoActivityIndicator(
                            color: GizColors.surface,
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

  Widget _buildAction(_PetMenuAction action, int index) {
    final count = widget.actions.length;
    final start = count <= 1 ? 0.0 : index * 0.08;
    final animation = CurvedAnimation(
      parent: _controller,
      curve: Interval(start.clamp(0.0, 0.65), 1, curve: Curves.easeOutBack),
      reverseCurve: Curves.easeIn,
    );
    final verticalOffset = 70.0 + index * 52.0;
    final arcProgress = (index + 1) / math.max(count, 1);
    final actionDensity = ((count - 3) / 7).clamp(0.0, 1.0);
    final arcExtent = 42 + actionDensity * 42;
    const arcStrength = 1.6;
    final horizontalOffset =
        ((math.exp(arcStrength * arcProgress) - 1) /
            (math.exp(arcStrength) - 1)) *
        arcExtent;
    return Positioned(
      left: 0,
      bottom: verticalOffset * animation.value,
      child: Transform.translate(
        offset: Offset(44 + horizontalOffset * animation.value, 0),
        child: FractionalTranslation(
          translation: const Offset(-1, 0),
          child: IgnorePointer(
            ignoring: animation.value < 0.8 || widget.activeAction != null,
            child: Opacity(
              opacity: animation.value.clamp(0.0, 1.0),
              child: Transform.scale(
                alignment: Alignment.bottomRight,
                scale: 0.82 + animation.value * 0.18,
                child: GestureDetector(
                  onTap: () => _select(action),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      DecoratedBox(
                        decoration: BoxDecoration(
                          color: GizColors.ink,
                          borderRadius: BorderRadius.circular(7),
                        ),
                        child: Padding(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 11,
                            vertical: 8,
                          ),
                          child: Text(
                            _actionName(widget.catalog, action.id),
                            style: GizText.label.copyWith(
                              color: GizColors.surface,
                            ),
                          ),
                        ),
                      ),
                      const SizedBox(width: 9),
                      Container(
                        width: 44,
                        height: 44,
                        decoration: const BoxDecoration(
                          color: GizColors.surface,
                          shape: BoxShape.circle,
                          boxShadow: [
                            BoxShadow(
                              color: Color(0x22000000),
                              blurRadius: 12,
                              offset: Offset(0, 5),
                            ),
                          ],
                        ),
                        child: Icon(
                          _actionIcon(action.icon, action.id),
                          size: 20,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _SceneButton extends StatelessWidget {
  const _SceneButton({
    required this.label,
    required this.icon,
    required this.onPressed,
  });

  final String label;
  final IconData icon;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: label,
      button: true,
      child: GestureDetector(
        onTap: onPressed,
        child: ClipOval(
          child: BackdropFilter(
            filter: ImageFilter.blur(sigmaX: 14, sigmaY: 14),
            child: Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(
                color: const Color(0xCFFFFFFF),
                shape: BoxShape.circle,
                border: Border.all(color: const Color(0x16000000)),
              ),
              child: Icon(icon, size: 21),
            ),
          ),
        ),
      ),
    );
  }
}

class _PetErrorToast extends StatelessWidget {
  const _PetErrorToast({required this.error});

  final String error;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 9),
      decoration: BoxDecoration(
        color: const Color(0xE6FFFFFF),
        borderRadius: BorderRadius.circular(8),
      ),
      child: Text(
        error,
        maxLines: 2,
        overflow: TextOverflow.ellipsis,
        style: GizText.label.copyWith(
          color: CupertinoColors.systemRed.resolveFrom(context),
        ),
      ),
    );
  }
}

class _PetEmptyPage extends StatelessWidget {
  const _PetEmptyPage({
    required this.adopting,
    required this.error,
    required this.onAdopt,
    required this.onRetry,
  });

  final bool adopting;
  final Object? error;
  final VoidCallback onAdopt;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return CupertinoPageScaffold(
      child: SafeArea(
        bottom: false,
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 112),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _PetPageHeader(adopting: adopting, onAdopt: onAdopt),
              const Spacer(),
              const Icon(CupertinoIcons.sparkles, size: 64),
              const SizedBox(height: 22),
              const Text(
                'Your next companion is waiting.',
                textAlign: TextAlign.center,
                style: GizText.sectionTitle,
              ),
              const SizedBox(height: 8),
              Text(
                'Use the add button to adopt your first pet.',
                textAlign: TextAlign.center,
                style: GizText.body.copyWith(color: GizColors.secondaryInk),
              ),
              if (error != null) ...[
                const SizedBox(height: 12),
                Text(
                  _petError(error!),
                  textAlign: TextAlign.center,
                  style: GizText.body.copyWith(
                    color: CupertinoColors.systemRed.resolveFrom(context),
                  ),
                ),
                CupertinoButton(onPressed: onRetry, child: const Text('Retry')),
              ],
              const Spacer(),
            ],
          ),
        ),
      ),
    );
  }
}

class _PetMessagePage extends StatelessWidget {
  const _PetMessagePage({
    required this.title,
    required this.message,
    required this.loading,
  });

  final String title;
  final String message;
  final bool loading;

  @override
  Widget build(BuildContext context) {
    return CupertinoPageScaffold(
      child: SafeArea(
        bottom: false,
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 112),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(title, style: GizText.pageTitle),
              const Spacer(),
              if (loading) const CupertinoActivityIndicator(radius: 14),
              if (loading) const SizedBox(height: 18),
              Text(
                message,
                textAlign: TextAlign.center,
                style: GizText.body.copyWith(color: GizColors.secondaryInk),
              ),
              const Spacer(),
            ],
          ),
        ),
      ),
    );
  }
}

class _PetDetailMessage extends StatelessWidget {
  const _PetDetailMessage({
    required this.message,
    required this.loading,
    this.onRetry,
  });

  final String message;
  final bool loading;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    return CupertinoPageScaffold(
      backgroundColor: _petSceneColor,
      child: SafeArea(
        child: Stack(
          children: [
            Positioned(
              left: 18,
              top: 12,
              child: _SceneButton(
                label: 'Back',
                icon: CupertinoIcons.back,
                onPressed: () => context.pop(),
              ),
            ),
            Center(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  if (loading) const CupertinoActivityIndicator(radius: 15),
                  if (loading) const SizedBox(height: 16),
                  Text(
                    message,
                    textAlign: TextAlign.center,
                    style: GizText.body,
                  ),
                  if (onRetry != null)
                    CupertinoButton(
                      onPressed: onRetry,
                      child: const Text('Retry'),
                    ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _PetMetric {
  const _PetMetric(this.label, this.value);

  final String label;
  final int value;
}

List<_PetMetric> _petMetrics(Pet pet, PetPresentationI18nCatalog? catalog) {
  return [
    for (final entry in pet.life.value.entries)
      _PetMetric(
        catalog?.attr.life.value[entry.key]?.displayName ?? _title(entry.key),
        entry.value.toInt(),
      ),
    for (final entry in pet.progression.value.entries)
      _PetMetric(
        catalog?.attr.progression.value[entry.key]?.displayName ??
            _title(entry.key),
        entry.value.toInt(),
      ),
  ];
}

PetPresentationI18nCatalog? _catalogFor(
  BuildContext context,
  PetPresentation? presentation,
) {
  if (presentation == null || presentation.i18n.value.isEmpty) return null;
  final catalogs = presentation.i18n.value;
  final locale = Localizations.localeOf(context);
  return catalogs[locale.toLanguageTag()] ??
      catalogs[locale.languageCode] ??
      catalogs[presentation.defaultLocale] ??
      catalogs.values.first;
}

String _petName(Pet pet, PetPresentationI18nCatalog? catalog) {
  if (pet.displayName.trim().isNotEmpty) return pet.displayName;
  if (catalog?.displayName.trim().isNotEmpty == true) {
    return catalog!.displayName;
  }
  return 'Unnamed pet';
}

String _petStateLabel(PetPresentation? presentation, Pet pet) {
  final activeClip = _defaultClip(presentation, pet);
  if (presentation != null && activeClip != null) {
    for (final clip in presentation.pixaMetadata.clips) {
      if (clip.pixaClipName == activeClip) return _title(clip.id).toUpperCase();
    }
  }
  return 'IDLE';
}

String _petProgressionLabel(Pet pet) {
  if (pet.progression.value.isEmpty) return pet.rulesetName;
  final entry = pet.progression.value.entries.first;
  return '${entry.key.toUpperCase()} ${entry.value}';
}

String _actionName(PetPresentationI18nCatalog? catalog, String id) =>
    catalog?.drive.actions[id]?.displayName ?? _title(id);

List<_PetMenuAction> _petMenuActions(PetPresentation? presentation) {
  if (presentation == null) return const [];
  final actions = <_PetMenuAction>[];
  final claimedClips = <String>{};
  for (final action in presentation.drive.actions) {
    if (action.id.toLowerCase() == 'idle') continue;
    final clipName = _clipForAction(presentation, action.id);
    if (clipName != null) claimedClips.add(clipName);
    actions.add(
      _PetMenuAction(
        id: action.id,
        clipName: clipName,
        driveAction: action,
        icon: action.hasIcon() ? action.icon : null,
      ),
    );
  }
  for (final clip in presentation.pixaMetadata.clips) {
    final id = clip.id.isEmpty ? clip.pixaClipName : clip.id;
    if (id.toLowerCase() == 'idle' ||
        clip.pixaClipName.toLowerCase() == 'idle' ||
        claimedClips.contains(clip.pixaClipName)) {
      continue;
    }
    actions.add(_PetMenuAction(id: id, clipName: clip.pixaClipName));
  }
  return actions;
}

String? _defaultClip(PetPresentation? presentation, [Pet? pet]) {
  if (presentation == null) return null;
  final stateClip = _petStateClip(presentation, pet);
  if (stateClip != null) return stateClip;
  for (final clip in presentation.pixaMetadata.clips) {
    if (clip.actionId == 'idle' || clip.id == 'idle') return clip.pixaClipName;
  }
  return presentation.pixaMetadata.clips.isEmpty
      ? null
      : presentation.pixaMetadata.clips.first.pixaClipName;
}

String? _petStateClip(PetPresentation presentation, Pet? pet) {
  if (pet == null) return null;
  final life = pet.life.value;
  final candidates = <String>[
    if ((life['hp']?.toInt() ?? 100) <= 0) 'dead',
    if ((life['hp']?.toInt() ?? 100) <= 20) 'dying',
    if ((life['cleanliness']?.toInt() ?? 100) <= 30) 'dirty',
    if ((life['wellness']?.toInt() ?? 100) <= 30) 'sick',
    if ((life['energy']?.toInt() ?? 100) <= 30) 'hungry',
  ];
  for (final candidate in candidates) {
    for (final clip in presentation.pixaMetadata.clips) {
      if (clip.id == candidate) return clip.pixaClipName;
    }
  }
  return null;
}

String? _clipForAction(PetPresentation? presentation, String actionId) {
  if (presentation == null) return null;
  for (final clip in presentation.pixaMetadata.clips) {
    if (clip.actionId == actionId || clip.id == actionId) {
      return clip.pixaClipName;
    }
  }
  return null;
}

Duration _clipDuration(PixaAsset? asset, String? clipName) {
  if (asset == null || clipName == null) return const Duration(seconds: 2);
  for (final clip in asset.clips) {
    if (clip.name == clipName) {
      return Duration(milliseconds: math.max(900, clip.totalDurationMs + 120));
    }
  }
  return const Duration(seconds: 2);
}

IconData _actionIcon(String? token, String id) {
  final semantic = token?.toLowerCase();
  if (semantic == 'bath' || semantic == 'clean') {
    return CupertinoIcons.drop_fill;
  }
  if (semantic == 'food' || semantic == 'feed' || semantic == 'eat') {
    return CupertinoIcons.cart_fill;
  }
  if (semantic == 'heal' || semantic == 'health') {
    return CupertinoIcons.plus_circle_fill;
  }
  if (semantic == 'sleep') return CupertinoIcons.moon_fill;
  if (semantic == 'play') return CupertinoIcons.game_controller_solid;
  if (semantic == 'idle' || semantic == 'magic') {
    return CupertinoIcons.sparkles;
  }

  final value = id.toLowerCase();
  if (value.contains('bath') || value.contains('clean')) {
    return CupertinoIcons.drop_fill;
  }
  if (value.contains('feed') || value.contains('eat')) {
    return CupertinoIcons.cart_fill;
  }
  if (value.contains('heal')) return CupertinoIcons.plus_circle_fill;
  if (value.contains('hungry')) return CupertinoIcons.cart_fill;
  if (value.contains('sick')) return CupertinoIcons.bandage_fill;
  if (value.contains('dirty')) return CupertinoIcons.drop_fill;
  if (value.contains('confuse')) return CupertinoIcons.question_circle_fill;
  if (value.contains('dying')) return CupertinoIcons.heart_slash_fill;
  if (value.contains('dead')) return CupertinoIcons.xmark_circle_fill;
  if (value.contains('reborn')) return CupertinoIcons.sparkles;
  if (value.contains('sleep')) return CupertinoIcons.moon_fill;
  if (value.contains('play')) return CupertinoIcons.game_controller_solid;
  return CupertinoIcons.sparkles;
}

String _title(String value) {
  if (value.isEmpty) return value;
  final words = value.replaceAll('_', ' ').split(' ');
  return words
      .where((word) => word.isNotEmpty)
      .map((word) => '${word[0].toUpperCase()}${word.substring(1)}')
      .join(' ');
}

String _petError(Object error) {
  final text = error.toString();
  return text.startsWith('Bad state: ') ? text.substring(11) : text;
}

Future<String?> _askPetName(BuildContext context) async {
  final controller = TextEditingController();
  try {
    return await showCupertinoDialog<String>(
      context: context,
      builder: (context) => CupertinoAlertDialog(
        title: const Text('Name your pet'),
        content: Padding(
          padding: const EdgeInsets.only(top: 12),
          child: CupertinoTextField(
            controller: controller,
            autofocus: true,
            placeholder: 'Optional name',
            textInputAction: TextInputAction.done,
            onSubmitted: (value) => Navigator.pop(context, value),
          ),
        ),
        actions: [
          CupertinoDialogAction(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
          CupertinoDialogAction(
            isDefaultAction: true,
            onPressed: () => Navigator.pop(context, controller.text),
            child: const Text('Adopt'),
          ),
        ],
      ),
    );
  } finally {
    controller.dispose();
  }
}
