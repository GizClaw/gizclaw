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
const _petDetailBackground = Color(0xFFE4E6E4);

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
    final data = MobileDataScope.watch(context);
    final request = ++_request;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final pets = <Pet>[];
      String? cursor;
      do {
        final response = await data.runRpc(
          (client) => client.listPets(cursor: cursor, limit: 100),
          retryOnTransportError: true,
        );
        pets.addAll(response.value.items);
        cursor = response.value.hasNext ? response.value.nextCursor : null;
      } while (cursor != null && cursor.isNotEmpty);
      if (!mounted || request != _request) return;
      for (final pet in pets) {
        data.rememberPetRouteContext(
          petId: pet.id,
          title: pet.displayName.trim().isEmpty
              ? 'Pet companion'
              : pet.displayName,
          workspaceName: pet.workspaceName,
        );
      }
      setState(() {
        _pets = pets;
        _visuals.removeWhere((id, _) => !pets.any((pet) => pet.id == id));
        _loading = false;
      });
      await Future.wait([for (final pet in pets) _loadVisual(pet, request)]);
    } catch (error) {
      if (!mounted || request != _request) return;
      setState(() {
        _loading = false;
        _error = error;
      });
    }
  }

  Future<void> _loadVisual(Pet pet, int request) async {
    try {
      final data = MobileDataScope.watch(context);
      final presentation = (await data.runRpc(
        (client) => client.getPetActions(pet.id),
        retryOnTransportError: true,
      )).value;
      PixaAsset? pixa;
      try {
        pixa = (await data.runRpc(
          (client) => client.downloadPetPixa(pet.id),
          retryOnTransportError: true,
        )).asset;
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
      final response = await MobileDataScope.watch(context).runRpc(
        (client) => client.adoptPet(
          displayName: name.trim().isEmpty ? null : name.trim(),
        ),
      );
      await _loadPets();
      if (mounted) context.push('/pets/${response.value.pet.id}');
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
        title: 'Pets',
        message: data.connectionState == MobileConnectionState.connecting
            ? 'Connecting to your pets...'
            : 'Connect to GizClaw to meet your pets.',
        loading: data.connectionState == MobileConnectionState.connecting,
      );
    }
    if (_loading && _pets.isEmpty) {
      return const _PetMessagePage(
        title: 'Pets',
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
                      onPressed: () => context.push('/pets/${_pets.first.id}'),
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
                            context.push('/pets/${_pets[index].id}'),
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
  bool _ownsChat = false;
  Pet? _pet;
  PetActions? _presentation;
  PixaAsset? _pixa;
  Object? _error;
  bool _loading = false;
  bool _statusVisible = false;
  Object? _displayedChatError;
  Object? _dismissedChatError;
  Timer? _errorDismissTimer;
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
    if (identical(client, _client)) {
      final pet = _pet;
      if (pet != null) unawaited(_syncPetChat(data, pet));
      return;
    }
    _replaceChat(null, null, ownsChat: false);
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
    _errorDismissTimer?.cancel();
    _replaceChat(null, null, ownsChat: false);
    super.dispose();
  }

  void _handleChatChanged() {
    if (!mounted) return;
    final chat = _chat;
    final error = chat?.lastError;
    if (error != null &&
        !identical(error, _displayedChatError) &&
        !identical(error, _dismissedChatError)) {
      _showChatError(error);
      return;
    }
    if ((chat?.startingInput ?? false) || (chat?.recording ?? false)) {
      _errorDismissTimer?.cancel();
      _displayedChatError = null;
      _dismissedChatError = null;
    }
    setState(() {});
  }

  void _showChatError(Object error) {
    _errorDismissTimer?.cancel();
    setState(() {
      _displayedChatError = error;
      _dismissedChatError = null;
    });
    _errorDismissTimer = Timer(const Duration(seconds: 8), () {
      if (!mounted || !identical(_displayedChatError, error)) return;
      setState(() {
        _displayedChatError = null;
        _dismissedChatError = error;
      });
    });
  }

  void _dismissError() {
    _errorDismissTimer?.cancel();
    setState(() {
      _dismissedChatError = _chat?.lastError ?? _displayedChatError;
      _displayedChatError = null;
      _error = null;
    });
  }

  void _replaceChat(
    WorkspaceChatController? chat,
    String? workspaceName, {
    required bool ownsChat,
  }) {
    if (identical(chat, _chat)) return;
    _chat?.removeListener(_handleChatChanged);
    if (_ownsChat) _chat?.dispose();
    _chat = chat;
    _chatWorkspaceName = workspaceName;
    _ownsChat = ownsChat;
    if (chat != null) {
      chat.addListener(_handleChatChanged);
      final error = chat.lastError;
      if (error != null) _showChatError(error);
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
      final pet = (await data.runRpc(
        (client) => client.getPet(widget.petId),
        retryOnTransportError: true,
      )).value;
      final presentation = (await data.runRpc(
        (client) => client.getPetActions(widget.petId),
        retryOnTransportError: true,
      )).value;
      PixaAsset? pixa;
      try {
        pixa = (await data.runRpc(
          (client) => client.downloadPetPixa(widget.petId),
          retryOnTransportError: true,
        )).asset;
      } catch (_) {
        // The pet detail remains usable while its optional PIXA is unavailable.
      }
      if (!mounted || request != _request) return;
      data.rememberPetRouteContext(
        petId: pet.id,
        title: pet.displayName.trim().isEmpty
            ? 'Pet companion'
            : pet.displayName,
        workspaceName: pet.workspaceName,
      );
      setState(() {
        _pet = pet;
        _presentation = presentation;
        _pixa = pixa;
        _clipName = _defaultClip(presentation, pet);
        _loading = false;
      });
      await _syncPetChat(data, pet);
    } catch (error) {
      if (!mounted || request != _request) return;
      setState(() {
        _loading = false;
        _error = error;
      });
    }
  }

  Future<void> _syncPetChat(MobileDataController data, Pet pet) async {
    final active = data.activeWorkspaceChat;
    if (data.activeWorkspaceName == pet.workspaceName && active != null) {
      _replaceChat(active, pet.workspaceName, ownsChat: false);
      if (mounted) setState(() {});
      return;
    }
    if (_ownsChat && _chatWorkspaceName == pet.workspaceName) return;
    final viewer = WorkspaceChatController(
      workspaceName: pet.workspaceName,
      repository: data.workspaceChatRepository,
      serverId: data.activeServerId,
      client: data.connection.client,
    );
    _replaceChat(viewer, pet.workspaceName, ownsChat: true);
    await viewer.start(conversation: false);
    if (mounted) setState(() {});
  }

  Future<void> _drive(PetAction action) async {
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
      final response = await MobileDataScope.watch(
        context,
      ).runRpc((client) => client.drivePet(pet.id, action: action.id));
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
    final currentChatError = chat?.lastError;
    final visibleError =
        _displayedChatError ??
        (identical(currentChatError, _dismissedChatError)
            ? null
            : currentChatError) ??
        _error;
    final safeTop = MediaQuery.paddingOf(context).top;
    return CupertinoPageScaffold(
      backgroundColor: _petDetailBackground,
      child: Stack(
        fit: StackFit.expand,
        children: [
          const Positioned.fill(
            child: IgnorePointer(child: _PetMosaicBackground(opacity: 0.58)),
          ),
          Positioned(
            left: 14,
            right: 14,
            top: safeTop + 86,
            bottom: MediaQuery.paddingOf(context).bottom + 106,
            child: _PetConversationDrift(messages: messages),
          ),
          Positioned(
            left: 20,
            right: 20,
            top: safeTop + 58,
            bottom: MediaQuery.paddingOf(context).bottom + 106,
            child: IgnorePointer(
              child: SingleChildScrollView(
                child: _PetGameConsole(
                  pixa: _pixa,
                  clipName: _clipName,
                  loading: _loading,
                ),
              ),
            ),
          ),
          if (visibleError != null)
            Positioned(
              left: 72,
              right: 18,
              bottom: MediaQuery.paddingOf(context).bottom + 108,
              child: _PetErrorToast(
                error: _petError(visibleError),
                recoverable: _isEmptyTranscriptError(visibleError),
                onDismiss: _dismissError,
              ),
            ),
          Positioned(
            right: 18,
            top: safeTop + 74,
            width: 158,
            child: IgnorePointer(
              ignoring: !_statusVisible,
              child: AnimatedSlide(
                offset: _statusVisible ? Offset.zero : const Offset(0, -0.08),
                duration: const Duration(milliseconds: 240),
                curve: Curves.easeOutCubic,
                child: AnimatedScale(
                  scale: _statusVisible ? 1 : 0.94,
                  alignment: Alignment.topRight,
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
          Positioned(
            right: 18,
            top: safeTop + 12,
            child: _PetStatusFab(
              visible: _statusVisible,
              onPressed: () {
                _actionFabKey.currentState?.collapse();
                setState(() => _statusVisible = !_statusVisible);
              },
            ),
          ),
          Positioned(
            left: 18 - _petActionAnchor,
            top: safeTop + 12,
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
        .toList(growable: false);
    return ShaderMask(
      blendMode: BlendMode.dstIn,
      shaderCallback: (bounds) => const LinearGradient(
        begin: Alignment.topCenter,
        end: Alignment.bottomCenter,
        colors: [
          Color(0x00FFFFFF),
          Color(0x42FFFFFF),
          Color(0xFFFFFFFF),
          Color(0xFFFFFFFF),
        ],
        stops: [0, 0.42, 0.68, 1],
      ).createShader(bounds),
      child: ListView.builder(
        key: const PageStorageKey<String>('pet-conversation-history'),
        reverse: true,
        primary: false,
        physics: const BouncingScrollPhysics(
          parent: AlwaysScrollableScrollPhysics(),
        ),
        padding: const EdgeInsets.fromLTRB(0, 140, 0, 24),
        itemCount: visible.length,
        itemBuilder: (context, index) {
          final message = visible[index];
          final alignment = message.incoming
              ? Alignment.centerLeft
              : Alignment.centerRight;
          return Align(
            key: ValueKey(message.id),
            alignment: alignment,
            child: FractionallySizedBox(
              widthFactor: 0.77,
              alignment: alignment,
              child: Padding(
                padding: const EdgeInsets.only(bottom: 10),
                child: TweenAnimationBuilder<double>(
                  tween: Tween(begin: 0, end: 1),
                  duration: const Duration(milliseconds: 650),
                  curve: Curves.easeOutQuart,
                  builder: (context, progress, child) => Transform.translate(
                    offset: Offset(0, 24 * (1 - progress)),
                    child: Opacity(opacity: progress, child: child),
                  ),
                  child: _PetDriftingMessage(message: message),
                ),
              ),
            ),
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
      borderRadius: GizCorners.compactCard,
      child: BackdropFilter(
        filter: ImageFilter.blur(sigmaX: 9, sigmaY: 9),
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: message.incoming
                ? GizColors.messageIncoming
                : GizColors.messageBlue,
            boxShadow: const [
              BoxShadow(
                color: Color(0x17000000),
                blurRadius: 14,
                offset: Offset(0, 5),
              ),
            ],
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
  const _PetMosaicBackground({this.opacity = 1});

  final double opacity;

  @override
  State<_PetMosaicBackground> createState() => _PetMosaicBackgroundState();
}

class _PetMosaicBackgroundState extends State<_PetMosaicBackground>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;
  var _sequence = 0;
  var _animationsDisabled = false;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 30000),
    )..addStatusListener(_handleAnimationStatus);
  }

  void _handleAnimationStatus(AnimationStatus status) {
    if (status != AnimationStatus.completed || _animationsDisabled) return;
    setState(() => _sequence += 1);
    _controller.forward(from: 0);
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _animationsDisabled = MediaQuery.disableAnimationsOf(context);
    if (_animationsDisabled) {
      _controller
        ..stop()
        ..value = 0.3;
    } else if (!_controller.isAnimating) {
      _controller.forward(from: _controller.value == 1 ? 0 : _controller.value);
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
        builder: (context, _) => CustomPaint(
          painter: _PetMosaicPainter(
            progress: _controller.value,
            opacity: widget.opacity,
            sequence: _sequence,
          ),
        ),
      ),
    );
  }
}

class _PetMosaicPainter extends CustomPainter {
  const _PetMosaicPainter({
    required this.progress,
    required this.opacity,
    required this.sequence,
  });

  final double opacity;
  final double progress;
  final int sequence;

  static const _mosaicLight = Color(0xFFF4F5F4);
  static const _mosaicShade = Color(0xFFE1E4E2);
  static const _flashColor = Color(0xFF111312);

  @override
  void paint(Canvas canvas, Size size) {
    const cellSize = 14.0;
    final columns = (size.width / cellSize).ceil();
    final rows = (size.height / cellSize).ceil();
    final time = sequence + progress;

    for (var row = 0; row < rows; row++) {
      for (var column = 0; column < columns; column++) {
        final verticalOpacity = _verticalOpacity(row / math.max(rows - 1, 1));
        final cellVariation = _cellNoise(column, row, 11);
        final phase = _cellNoise(column, row, 37) * math.pi * 2;
        final speed = 0.65 + _cellNoise(column, row, 71) * 0.3;
        final density = _cellNoise(column, row, 103);
        final wave = (math.sin(time * math.pi * 2 * speed + phase) + 1) / 2;
        final twinkle = math.pow(wave, 1.8).toDouble();
        final slowVariation =
            0.7 +
            0.3 *
                ((math.sin(time * math.pi * 0.44 + column * 0.17 + row * 0.11) +
                        1) /
                    2);
        final activity = density < 0.58
            ? 0.0
            : ((density - 0.58) / 0.42).clamp(0.0, 1.0);
        final depth = 0.1 + cellVariation * 0.32;
        final flash = (twinkle * slowVariation * activity * depth).clamp(
          0.0,
          1.0,
        );
        final base = Color.lerp(
          _mosaicLight,
          _mosaicShade,
          cellVariation * 0.55,
        )!;
        canvas.drawRect(
          Rect.fromLTWH(column * cellSize, row * cellSize, cellSize, cellSize),
          Paint()
            ..color = Color.lerp(
              base,
              _flashColor,
              flash,
            )!.withValues(alpha: opacity * verticalOpacity),
        );
      }
    }

    final gridAlpha = 0.16 * opacity;
    final gridPaint = Paint()
      ..shader = LinearGradient(
        begin: Alignment.topCenter,
        end: Alignment.bottomCenter,
        colors: [
          Color.fromRGBO(255, 255, 255, gridAlpha),
          const Color(0x00FFFFFF),
          const Color(0x00FFFFFF),
        ],
        stops: const [0, 0.5, 1],
      ).createShader(Offset.zero & size)
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

  double _verticalOpacity(double position) {
    final progress = (position / 0.5).clamp(0.0, 1.0);
    final eased = progress * progress * (3 - 2 * progress);
    return lerpDouble(1, 0, eased)!;
  }

  double _cellNoise(int column, int row, int salt) {
    final value =
        math.sin(column * 127.1 + row * 311.7 + salt * 74.7) * 43758.5453;
    return value - value.floor();
  }

  @override
  bool shouldRepaint(covariant _PetMosaicPainter oldDelegate) {
    return oldDelegate.progress != progress ||
        oldDelegate.opacity != opacity ||
        oldDelegate.sequence != sequence;
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
        const Expanded(child: Text('Pets', style: GizText.pageTitle)),
        GizPageActionButton(
          icon: GizIcons.add_circled_solid,
          semanticLabel: 'Adopt a pet',
          loading: adopting,
          onPressed: adopting ? null : onAdopt,
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
    final cardRadius = compact ? GizCorners.card : GizCorners.hero;
    final accent = _petCoverAccent(pet.id);
    return GizPressable(
      onPressed: onPressed,
      borderRadius: cardRadius,
      scaleWhenPressed: 0.975,
      child: AspectRatio(
        aspectRatio: compact ? 0.8 : 1.08,
        child: GizSquircle(
          borderRadius: cardRadius,
          child: Stack(
            fit: StackFit.expand,
            children: [
              const _PetMosaicBackground(),
              DecoratedBox(
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.topLeft,
                    end: Alignment.bottomRight,
                    colors: [
                      accent.withValues(alpha: 0.34),
                      const Color(0x00FFFFFF),
                      const Color(0x24FFFFFF),
                    ],
                    stops: const [0, 0.52, 1],
                  ),
                ),
              ),
              Positioned(
                left: compact ? 18 : 76,
                right: compact ? 18 : 76,
                top: compact ? 42 : 34,
                bottom: compact ? 76 : 76,
                child: visual == null
                    ? const Center(child: CupertinoActivityIndicator())
                    : visual!.pixa == null
                    ? const Center(
                        child: Icon(
                          GizIcons.sparkles,
                          color: GizColors.secondaryInk,
                          size: 36,
                        ),
                      )
                    : _PetCoverSprite(
                        child: _AnimatedPetSprite(
                          asset: visual!.pixa!,
                          clipName: _defaultClip(visual!.presentation, pet),
                          transparentEdgeBackground: true,
                        ),
                      ),
              ),
              Positioned(
                top: compact ? 11 : 14,
                left: compact ? 11 : 14,
                child: _PetCoverLabel(compact: compact),
              ),
              Positioned(
                top: compact ? 11 : 14,
                right: compact ? 11 : 14,
                child: GizSquircle(
                  borderRadius: GizCorners.compactCard,
                  child: BackdropFilter(
                    filter: ImageFilter.blur(sigmaX: 14, sigmaY: 14),
                    child: Container(
                      color: const Color(0xA8FFFFFF),
                      padding: const EdgeInsets.symmetric(
                        horizontal: 8,
                        vertical: 6,
                      ),
                      child: Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          DecoratedBox(
                            decoration: BoxDecoration(
                              color: accent,
                              shape: BoxShape.circle,
                              boxShadow: [
                                BoxShadow(
                                  color: accent.withValues(alpha: 0.38),
                                  blurRadius: 6,
                                ),
                              ],
                            ),
                            child: const SizedBox.square(dimension: 6),
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
                child: DecoratedBox(
                  decoration: const BoxDecoration(
                    gradient: LinearGradient(
                      begin: Alignment.topCenter,
                      end: Alignment.bottomCenter,
                      colors: [Color(0x00111B18), Color(0xD6111B18)],
                    ),
                  ),
                  child: Padding(
                    padding: EdgeInsets.fromLTRB(
                      compact ? 13 : 18,
                      compact ? 36 : 44,
                      compact ? 10 : 14,
                      compact ? 12 : 16,
                    ),
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.end,
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
                              const SizedBox(height: 3),
                              Text(
                                compact
                                    ? _petProgressionLabel(pet)
                                    : '${pet.rulesetName}  /  ${_petProgressionLabel(pet)}',
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                                style: GizText.label.copyWith(
                                  color: const Color(0xBFFFFFFF),
                                  fontSize: compact ? 8 : 9,
                                ),
                              ),
                            ],
                          ),
                        ),
                        const SizedBox(width: 8),
                        Container(
                          width: compact ? 32 : 36,
                          height: compact ? 32 : 36,
                          decoration: BoxDecoration(
                            color: const Color(0x24FFFFFF),
                            shape: BoxShape.circle,
                            border: Border.all(color: const Color(0x38FFFFFF)),
                          ),
                          child: Icon(
                            GizIcons.arrow_up_right,
                            size: compact ? 15 : 17,
                            color: GizColors.surface,
                          ),
                        ),
                      ],
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

class _PetCoverLabel extends StatelessWidget {
  const _PetCoverLabel({required this.compact});

  final bool compact;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(
          GizIcons.sparkles,
          size: compact ? 13 : 15,
          color: const Color(0xB8001913),
        ),
        if (!compact) ...[
          const SizedBox(width: 6),
          Text(
            'COMPANION',
            style: GizText.label.copyWith(
              color: const Color(0xA8001913),
              fontSize: 9,
            ),
          ),
        ],
      ],
    );
  }
}

Color _petCoverAccent(String id) {
  const accents = [
    Color(0xFF25A97F),
    Color(0xFFDA765E),
    Color(0xFF5478D8),
    Color(0xFF9A73C4),
  ];
  return accents[id.hashCode.abs() % accents.length];
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

  final PetActions presentation;
  final PixaAsset? pixa;
}

class _AnimatedPetSprite extends StatefulWidget {
  const _AnimatedPetSprite({
    required this.asset,
    required this.clipName,
    this.transparentEdgeBackground = false,
  });

  final PixaAsset asset;
  final String? clipName;
  final bool transparentEdgeBackground;

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
      transparentEdgeBackground: widget.transparentEdgeBackground,
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
        child: _PetDetailFabSurface(
          child: AnimatedSwitcher(
            duration: const Duration(milliseconds: 180),
            transitionBuilder: (child, animation) => RotationTransition(
              turns: Tween(begin: 0.86, end: 1.0).animate(animation),
              child: FadeTransition(opacity: animation, child: child),
            ),
            child: Icon(
              visible ? GizIcons.xmark : GizIcons.waveform_path_ecg,
              key: ValueKey(visible),
              color: _petDetailFabForeground,
              size: visible ? 20 : 22,
            ),
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
                                    GizIcons.sparkles,
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
        borderRadius: GizCorners.card,
        boxShadow: [
          BoxShadow(
            color: const Color(0xFF17241F).withValues(alpha: 0.28),
            blurRadius: 26,
            offset: const Offset(0, 12),
          ),
        ],
      ),
      child: ClipRSuperellipse(
        borderRadius: GizCorners.card,
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 19, sigmaY: 19),
          child: DecoratedBox(
            decoration: BoxDecoration(
              borderRadius: GizCorners.card,
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
  });

  final String id;
  final String? clipName;
  final PetAction? driveAction;
}

const _petActionAnchor = 160.0;
const _petActionItemExtent = 52.0;
const _petActionRailHeight = 270.0;
const _petActionRailTop = 48.0;
const _petActionMenuHeight = _petActionRailHeight + _petActionRailTop;

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
  final PetActionsI18nCatalog? catalog;
  final String? activeAction;
  final ValueChanged<_PetMenuAction> onAction;
  final VoidCallback onExpand;

  @override
  State<_PetActionFab> createState() => _PetActionFabState();
}

class _PetActionFabState extends State<_PetActionFab>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;
  late final ScrollController _scrollController;
  bool _expanded = false;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 320),
      reverseDuration: const Duration(milliseconds: 220),
    );
    _scrollController = ScrollController();
  }

  @override
  void dispose() {
    _controller.dispose();
    _scrollController.dispose();
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
    return SizedBox(
      width: 400,
      height: _petActionMenuHeight,
      child: AnimatedBuilder(
        animation: _controller,
        builder: (context, _) {
          return Stack(
            clipBehavior: Clip.none,
            alignment: Alignment.topLeft,
            children: [
              Positioned(
                left: 0,
                right: 0,
                top: _petActionRailTop,
                height: _petActionRailHeight,
                child: IgnorePointer(
                  ignoring: !_expanded || widget.activeAction != null,
                  child: Opacity(
                    opacity: _controller.value,
                    child: Transform.translate(
                      offset: Offset(0, -16 * (1 - _controller.value)),
                      child: Transform.scale(
                        alignment: Alignment.topLeft,
                        scale: 0.92 + _controller.value * 0.08,
                        child: _buildActionRail(),
                      ),
                    ),
                  ),
                ),
              ),
              Positioned(
                left: _petActionAnchor,
                top: 0,
                child: Semantics(
                  label: _expanded ? 'Close pet actions' : 'Open pet actions',
                  button: true,
                  child: GestureDetector(
                    onTap: _toggle,
                    child: _PetDetailFabSurface(
                      child: widget.activeAction == null
                          ? Stack(
                              alignment: Alignment.center,
                              children: [
                                Opacity(
                                  opacity: 1 - _controller.value,
                                  child: Transform.scale(
                                    scale: 1 - _controller.value * 0.2,
                                    child: const Icon(
                                      GizIcons.game_controller_solid,
                                      color: _petDetailFabForeground,
                                      size: 24,
                                    ),
                                  ),
                                ),
                                Opacity(
                                  opacity: _controller.value,
                                  child: Transform.rotate(
                                    angle:
                                        (1 - _controller.value) * -math.pi / 4,
                                    child: const Icon(
                                      GizIcons.xmark,
                                      color: _petDetailFabForeground,
                                      size: 20,
                                    ),
                                  ),
                                ),
                              ],
                            )
                          : const CupertinoActivityIndicator(
                              color: _petDetailFabForeground,
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

  Widget _buildActionRail() {
    return ShaderMask(
      blendMode: BlendMode.dstIn,
      shaderCallback: (bounds) => const LinearGradient(
        begin: Alignment.topCenter,
        end: Alignment.bottomCenter,
        colors: [
          Color(0x00FFFFFF),
          Color(0xFFFFFFFF),
          Color(0xFFFFFFFF),
          Color(0x00FFFFFF),
        ],
        stops: [0, 0.16, 0.84, 1],
      ).createShader(bounds),
      child: ListView.builder(
        controller: _scrollController,
        itemExtent: _petActionItemExtent,
        padding: const EdgeInsets.only(left: _petActionAnchor + 3),
        physics: const BouncingScrollPhysics(
          parent: AlwaysScrollableScrollPhysics(),
        ),
        itemCount: widget.actions.length,
        itemBuilder: (context, index) => _buildAction(widget.actions[index]),
      ),
    );
  }

  Widget _buildAction(_PetMenuAction action) {
    return Align(
      alignment: Alignment.centerLeft,
      child: CupertinoButton(
        key: ValueKey('pet-action-${action.id}'),
        minimumSize: Size.zero,
        padding: EdgeInsets.zero,
        pressedOpacity: 0.62,
        onPressed: widget.activeAction == null ? () => _select(action) : null,
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
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
              child: Icon(_actionIcon(action.id), size: 20),
            ),
            const SizedBox(width: 9),
            DecoratedBox(
              decoration: BoxDecoration(
                color: GizColors.ink,
                borderRadius: GizCorners.compactCard,
              ),
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 132),
                child: Padding(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 12,
                    vertical: 8,
                  ),
                  child: Text(
                    _actionName(widget.catalog, action.id),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: GizText.label.copyWith(color: GizColors.surface),
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

const _petDetailFabForeground = Color(0xFF1D5143);

class _PetDetailFabSurface extends StatelessWidget {
  const _PetDetailFabSurface({required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 50,
      height: 50,
      decoration: const BoxDecoration(
        shape: BoxShape.circle,
        boxShadow: [
          BoxShadow(
            color: Color(0x30649E89),
            blurRadius: 18,
            spreadRadius: -2,
            offset: Offset(0, 7),
          ),
          BoxShadow(
            color: Color(0x29B2E43A),
            blurRadius: 16,
            spreadRadius: -4,
            offset: Offset(0, 2),
          ),
        ],
      ),
      child: ClipOval(
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 16, sigmaY: 16),
          child: DecoratedBox(
            decoration: BoxDecoration(
              gradient: const LinearGradient(
                begin: Alignment.topLeft,
                end: Alignment.bottomRight,
                colors: [Color(0xF5FFFFFF), Color(0xD8DCECE4)],
              ),
              shape: BoxShape.circle,
              border: Border.all(color: const Color(0xCFFFFFFF), width: 0.8),
            ),
            child: Center(child: child),
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
  const _PetErrorToast({
    required this.error,
    required this.recoverable,
    required this.onDismiss,
  });

  final String error;
  final VoidCallback onDismiss;
  final bool recoverable;

  @override
  Widget build(BuildContext context) {
    final accent = recoverable
        ? const Color(0xFF28705C)
        : CupertinoColors.systemRed.resolveFrom(context);
    return Container(
      padding: const EdgeInsets.fromLTRB(12, 9, 7, 9),
      decoration: BoxDecoration(
        color: recoverable ? const Color(0xEDF4FAF7) : const Color(0xF2FFFFFF),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: accent.withValues(alpha: 0.18)),
        boxShadow: [
          BoxShadow(
            color: accent.withValues(alpha: 0.1),
            blurRadius: 16,
            offset: const Offset(0, 6),
          ),
        ],
      ),
      child: Row(
        children: [
          Icon(
            recoverable
                ? GizIcons.waveform
                : GizIcons.exclamationmark_triangle_fill,
            color: accent,
            size: 16,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              error,
              maxLines: 3,
              overflow: TextOverflow.ellipsis,
              style: GizText.label.copyWith(color: accent, height: 1.35),
            ),
          ),
          CupertinoButton(
            minimumSize: const Size(34, 34),
            padding: EdgeInsets.zero,
            onPressed: onDismiss,
            child: Icon(GizIcons.xmark, color: accent, size: 14),
          ),
        ],
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
              const Icon(GizIcons.sparkles, size: 64),
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
                icon: GizIcons.back,
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

List<_PetMetric> _petMetrics(Pet pet, PetActionsI18nCatalog? catalog) {
  return [
    for (final entry in pet.life.value.entries)
      _PetMetric(_title(entry.key), entry.value.toInt()),
    for (final entry in pet.progression.value.entries)
      _PetMetric(_title(entry.key), entry.value.toInt()),
  ];
}

PetActionsI18nCatalog? _catalogFor(
  BuildContext context,
  PetActions? presentation,
) {
  if (presentation == null || presentation.i18n.value.isEmpty) return null;
  final catalogs = presentation.i18n.value;
  final locale = Localizations.localeOf(context);
  return catalogs[locale.toLanguageTag()] ??
      catalogs[locale.languageCode] ??
      catalogs[presentation.defaultLocale] ??
      catalogs.values.first;
}

String _petName(Pet pet, PetActionsI18nCatalog? catalog) {
  if (pet.displayName.trim().isNotEmpty) return pet.displayName;
  return 'Unnamed pet';
}

String _petStateLabel(PetActions? presentation, Pet pet) {
  final activeClip = _defaultClip(presentation, pet);
  if (activeClip != null && activeClip.trim().isNotEmpty) {
    return _title(activeClip).toUpperCase();
  }
  return 'IDLE';
}

String _petProgressionLabel(Pet pet) {
  if (pet.progression.value.isEmpty) return pet.rulesetName;
  final entry = pet.progression.value.entries.first;
  return '${entry.key.toUpperCase()} ${entry.value}';
}

String _actionName(PetActionsI18nCatalog? catalog, String id) =>
    catalog?.actions[id]?.name ?? _title(id);

List<_PetMenuAction> _petMenuActions(PetActions? presentation) {
  if (presentation == null) return const [];
  final actions = <_PetMenuAction>[];
  for (final action in presentation.actions) {
    if (action.id.toLowerCase() == 'idle') continue;
    final clipName = _clipForAction(presentation, action.id);
    actions.add(
      _PetMenuAction(id: action.id, clipName: clipName, driveAction: action),
    );
  }
  return actions;
}

String? _defaultClip(PetActions? presentation, [Pet? pet]) {
  if (presentation == null) return 'idle';
  for (final action in presentation.actions) {
    if (action.id.toLowerCase() == 'idle') {
      return action.pixaClipName.isNotEmpty
          ? action.pixaClipName
          : action.visualClipId.isNotEmpty
          ? action.visualClipId
          : action.id;
    }
  }
  return presentation.actions.isEmpty
      ? 'idle'
      : _clipForAction(presentation, presentation.actions.first.id);
}

String? _clipForAction(PetActions? presentation, String actionId) {
  if (presentation == null) return null;
  for (final action in presentation.actions) {
    if (action.id == actionId) {
      return action.pixaClipName.isNotEmpty
          ? action.pixaClipName
          : action.visualClipId.isNotEmpty
          ? action.visualClipId
          : action.id;
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

IconData _actionIcon(String id) {
  final value = id.toLowerCase();
  if (value.contains('bath') || value.contains('clean')) {
    return GizIcons.drop_fill;
  }
  if (value.contains('feed') || value.contains('eat')) {
    return GizIcons.cart_fill;
  }
  if (value.contains('heal')) return GizIcons.plus_circle_fill;
  if (value.contains('hungry')) return GizIcons.cart_fill;
  if (value.contains('sick')) return GizIcons.bandage_fill;
  if (value.contains('dirty')) return GizIcons.drop_fill;
  if (value.contains('confuse')) return GizIcons.question_circle_fill;
  if (value.contains('dying')) return GizIcons.heart_slash_fill;
  if (value.contains('dead')) return GizIcons.xmark_circle_fill;
  if (value.contains('reborn')) return GizIcons.sparkles;
  if (value.contains('sleep')) return GizIcons.moon_fill;
  if (value.contains('play')) return GizIcons.game_controller_solid;
  return GizIcons.sparkles;
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
  if (text.contains('ASR produced empty transcript')) {
    return "I couldn't hear that. Hold the mic and speak again.";
  }
  return text.startsWith('Bad state: ') ? text.substring(11) : text;
}

bool _isEmptyTranscriptError(Object error) =>
    error.toString().contains('ASR produced empty transcript');

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
