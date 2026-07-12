import 'dart:async';
import 'dart:math' as math;
import 'dart:ui';

import 'package:flutter/cupertino.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:go_router/go_router.dart';

import '../../data/mobile_data_controller.dart';
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
  GizClawClient? _client;
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
    unawaited(_load());
  }

  @override
  void dispose() {
    _request += 1;
    super.dispose();
  }

  Future<void> _load() async {
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
      setState(() {
        _pet = pet;
        _presentation = presentation;
        _pixa = pixa;
        _clipName = _defaultClip(presentation);
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
        _clipName = _defaultClip(_presentation);
      });
    } catch (error) {
      if (!mounted) return;
      setState(() {
        _drivingAction = null;
        _clipName = _defaultClip(_presentation);
        _error = error;
      });
    }
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
        onRetry: _load,
      );
    }

    final catalog = _catalogFor(context, _presentation);
    final metrics = _petMetrics(pet, catalog).take(4).toList();
    final progression = pet.progression.value.entries.isEmpty
        ? pet.rulesetName
        : pet.progression.value.entries
              .map((entry) => '${_title(entry.key)} ${entry.value}')
              .join('  |  ');
    final actions =
        (_presentation?.drive.actions ?? const <PetPresentationActionSpec>[])
            .where((action) => action.id.toLowerCase() != 'idle')
            .toList();
    return CupertinoPageScaffold(
      backgroundColor: _petDetailBackground,
      child: Stack(
        fit: StackFit.expand,
        children: [
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
            bottom: MediaQuery.paddingOf(context).bottom + 86,
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
              bottom: MediaQuery.paddingOf(context).bottom + 22,
              child: _PetErrorToast(error: _petError(_error!)),
            ),
          Positioned(
            left: 18,
            bottom: MediaQuery.paddingOf(context).bottom + 18,
            child: _PetActionFab(
              actions: actions,
              catalog: catalog,
              activeAction: _drivingAction,
              onAction: _drive,
            ),
          ),
          Positioned(
            right: 18,
            bottom: MediaQuery.paddingOf(context).bottom + 86,
            width: 204,
            child: IgnorePointer(
              ignoring: !_statusVisible,
              child: AnimatedSlide(
                offset: _statusVisible ? Offset.zero : const Offset(0, 0.12),
                duration: const Duration(milliseconds: 240),
                curve: Curves.easeOutCubic,
                child: AnimatedScale(
                  scale: _statusVisible ? 1 : 0.94,
                  alignment: Alignment.bottomRight,
                  duration: const Duration(milliseconds: 240),
                  curve: Curves.easeOutCubic,
                  child: AnimatedOpacity(
                    opacity: _statusVisible ? 1 : 0,
                    duration: const Duration(milliseconds: 180),
                    child: _PetStatusNameplate(
                      metrics: metrics,
                      progression: progression,
                    ),
                  ),
                ),
              ),
            ),
          ),
          Positioned(
            right: 18,
            bottom: MediaQuery.paddingOf(context).bottom + 18,
            child: _PetStatusFab(
              visible: _statusVisible,
              onPressed: () => setState(() => _statusVisible = !_statusVisible),
            ),
          ),
        ],
      ),
    );
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
      scaleWhenPressed: 0.985,
      child: AspectRatio(
        aspectRatio: compact ? 0.78 : 1.08,
        child: ClipRRect(
          borderRadius: BorderRadius.circular(12),
          child: ColoredBox(
            color: _petSceneColor,
            child: Stack(
              fit: StackFit.expand,
              children: [
                Positioned(
                  left: compact ? 16 : 34,
                  right: compact ? 16 : 34,
                  top: compact ? 18 : 24,
                  bottom: compact ? 58 : 72,
                  child: visual?.pixa == null
                      ? const Center(child: CupertinoActivityIndicator())
                      : AnimatedOpacity(
                          opacity: 1,
                          duration: const Duration(milliseconds: 280),
                          child: _AnimatedPetSprite(
                            asset: visual!.pixa!,
                            clipName: _defaultClip(visual!.presentation),
                          ),
                        ),
                ),
                Positioned(
                  left: 0,
                  right: 0,
                  bottom: 0,
                  child: Container(
                    color: GizColors.ink,
                    padding: EdgeInsets.fromLTRB(
                      compact ? 12 : 18,
                      compact ? 10 : 14,
                      compact ? 10 : 16,
                      compact ? 11 : 15,
                    ),
                    child: Row(
                      children: [
                        Expanded(
                          child: Text(
                            _petName(pet, catalog),
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: GizText.sectionTitle.copyWith(
                              color: GizColors.surface,
                              fontSize: compact ? 15 : null,
                            ),
                          ),
                        ),
                        Icon(
                          CupertinoIcons.arrow_up_right,
                          color: GizColors.surface,
                          size: compact ? 17 : 21,
                        ),
                      ],
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
              child: ClipRRect(
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

class _PetStatusNameplate extends StatelessWidget {
  const _PetStatusNameplate({required this.metrics, required this.progression});

  final List<_PetMetric> metrics;
  final String progression;

  static const _colors = [
    GizColors.accent,
    Color(0xFFFF9470),
    Color(0xFF55BDA7),
    Color(0xFFA690D2),
  ];

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(12, 11, 12, 12),
      decoration: BoxDecoration(
        color: const Color(0xF2111916),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: const Color(0x4DFFFFFF)),
        boxShadow: const [
          BoxShadow(
            color: Color(0x33000000),
            blurRadius: 22,
            offset: Offset(0, 10),
          ),
        ],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Container(
                width: 7,
                height: 7,
                decoration: const BoxDecoration(
                  color: GizColors.accent,
                  shape: BoxShape.circle,
                ),
              ),
              const SizedBox(width: 7),
              Expanded(
                child: Text(
                  'VITAL STATUS',
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: GizText.label.copyWith(color: GizColors.surface),
                ),
              ),
              Text(
                progression.toUpperCase(),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: GizText.label.copyWith(
                  color: const Color(0xA8FFFFFF),
                  fontSize: 9,
                ),
              ),
            ],
          ),
          const SizedBox(height: 9),
          for (var index = 0; index < metrics.length; index++) ...[
            _NameplateMetric(
              metric: metrics[index],
              color: _colors[index % _colors.length],
            ),
            if (index != metrics.length - 1) const SizedBox(height: 7),
          ],
        ],
      ),
    );
  }
}

class _NameplateMetric extends StatelessWidget {
  const _NameplateMetric({required this.metric, required this.color});

  final _PetMetric metric;
  final Color color;

  @override
  Widget build(BuildContext context) {
    final value = (metric.value / 100).clamp(0.0, 1.0);
    return Row(
      children: [
        SizedBox(
          width: 64,
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
        Expanded(
          child: ClipRRect(
            borderRadius: BorderRadius.circular(99),
            child: SizedBox(
              height: 4,
              child: Stack(
                fit: StackFit.expand,
                children: [
                  const ColoredBox(color: Color(0x24FFFFFF)),
                  FractionallySizedBox(
                    alignment: Alignment.centerLeft,
                    widthFactor: value,
                    child: ColoredBox(color: color),
                  ),
                ],
              ),
            ),
          ),
        ),
        const SizedBox(width: 8),
        SizedBox(
          width: 25,
          child: Text(
            '${metric.value}',
            textAlign: TextAlign.right,
            style: GizText.label.copyWith(color: GizColors.surface),
          ),
        ),
      ],
    );
  }
}

class _PetActionFab extends StatefulWidget {
  const _PetActionFab({
    required this.actions,
    required this.catalog,
    required this.activeAction,
    required this.onAction,
  });

  final List<PetPresentationActionSpec> actions;
  final PetPresentationI18nCatalog? catalog;
  final String? activeAction;
  final ValueChanged<PetPresentationActionSpec> onAction;

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
      _controller.forward();
    } else {
      _controller.reverse();
    }
  }

  void _select(PetPresentationActionSpec action) {
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
                    child: Transform.rotate(
                      angle: _controller.value * math.pi / 4,
                      child: widget.activeAction == null
                          ? const Icon(
                              CupertinoIcons.add,
                              color: GizColors.surface,
                              size: 27,
                            )
                          : const CupertinoActivityIndicator(
                              color: GizColors.surface,
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

  Widget _buildAction(PetPresentationActionSpec action, int index) {
    final count = widget.actions.length;
    final start = count <= 1 ? 0.0 : index * 0.08;
    final animation = CurvedAnimation(
      parent: _controller,
      curve: Interval(start.clamp(0.0, 0.65), 1, curve: Curves.easeOutBack),
      reverseCurve: Curves.easeIn,
    );
    final offset = 70.0 + index * 52.0;
    return Positioned(
      left: 0,
      bottom: offset * animation.value,
      child: IgnorePointer(
        ignoring: animation.value < 0.8 || widget.activeAction != null,
        child: Opacity(
          opacity: animation.value.clamp(0.0, 1.0),
          child: Transform.scale(
            alignment: Alignment.bottomLeft,
            scale: 0.82 + animation.value * 0.18,
            child: GestureDetector(
              onTap: () => _select(action),
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
                      borderRadius: BorderRadius.circular(7),
                    ),
                    child: Padding(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 11,
                        vertical: 8,
                      ),
                      child: Text(
                        _actionName(widget.catalog, action.id),
                        style: GizText.label.copyWith(color: GizColors.surface),
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

String _actionName(PetPresentationI18nCatalog? catalog, String id) =>
    catalog?.drive.actions[id]?.displayName ?? _title(id);

String? _defaultClip(PetPresentation? presentation) {
  if (presentation == null) return null;
  for (final clip in presentation.pixaMetadata.clips) {
    if (clip.actionId == 'idle' || clip.id == 'idle') return clip.pixaClipName;
  }
  return presentation.pixaMetadata.clips.isEmpty
      ? null
      : presentation.pixaMetadata.clips.first.pixaClipName;
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

IconData _actionIcon(String id) {
  final value = id.toLowerCase();
  if (value.contains('bath') || value.contains('clean')) {
    return CupertinoIcons.drop_fill;
  }
  if (value.contains('feed') || value.contains('eat')) {
    return CupertinoIcons.heart_fill;
  }
  if (value.contains('heal')) return CupertinoIcons.plus_circle_fill;
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
