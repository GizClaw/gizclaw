import 'dart:async';

import 'package:flutter/cupertino.dart';
import 'package:gizclaw/gizclaw.dart';

import '../../data/mobile_data_controller.dart';
import '../../giz_ui/giz_ui.dart';
import '../../pixa_sprite.dart';

class PetPage extends StatefulWidget {
  const PetPage({super.key});

  @override
  State<PetPage> createState() => _PetPageState();
}

class _PetPageState extends State<PetPage> {
  GizClawClient? _client;
  List<Pet> _pets = const [];
  Pet? _pet;
  PetPresentation? _presentation;
  PixaAsset? _pixa;
  Object? _error;
  Object? _pixaError;
  bool _loading = false;
  bool _adopting = false;
  String? _drivingAction;
  String? _clipName;
  Timer? _idleTimer;
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
        _loading = false;
        _pets = const [];
        _pet = null;
        _presentation = null;
        _pixa = null;
      });
      return;
    }
    unawaited(_loadPets());
  }

  @override
  void dispose() {
    _request += 1;
    _idleTimer?.cancel();
    super.dispose();
  }

  Future<void> _loadPets({String? selectedId}) async {
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
      _pets = pets;
      if (pets.isEmpty) {
        setState(() {
          _pet = null;
          _presentation = null;
          _pixa = null;
          _loading = false;
        });
        return;
      }
      Pet selected = pets.first;
      final wantedId = selectedId ?? _pet?.id;
      if (wantedId != null) {
        for (final candidate in pets) {
          if (candidate.id == wantedId) selected = candidate;
        }
      }
      await _loadPet(selected, request: request);
    } catch (error) {
      if (!mounted || request != _request) return;
      setState(() {
        _loading = false;
        _error = error;
      });
    }
  }

  Future<void> _loadPet(Pet pet, {int? request}) async {
    final client = _client;
    if (client == null) return;
    final activeRequest = request ?? ++_request;
    setState(() {
      _pet = pet;
      _loading = true;
      _error = null;
      _pixaError = null;
      _presentation = null;
      _pixa = null;
    });
    try {
      final response = await client.getPetPresentation(pet.id);
      PixaAsset? pixa;
      Object? pixaError;
      try {
        pixa = (await client.downloadPetPixa(pet.id)).asset;
      } catch (error) {
        pixaError = error;
      }
      if (!mounted || activeRequest != _request) return;
      final presentation = response.value;
      setState(() {
        _presentation = presentation;
        _pixa = pixa;
        _pixaError = pixaError;
        _clipName = _defaultClip(presentation);
        _loading = false;
      });
    } catch (error) {
      if (!mounted || activeRequest != _request) return;
      setState(() {
        _loading = false;
        _error = error;
      });
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
      await _loadPets(selectedId: response.value.pet.id);
    } catch (error) {
      if (mounted) setState(() => _error = error);
    } finally {
      if (mounted) setState(() => _adopting = false);
    }
  }

  Future<void> _drive(PetPresentationActionSpec action) async {
    final client = _client;
    final pet = _pet;
    if (client == null || pet == null || _drivingAction != null) return;
    setState(() {
      _drivingAction = action.id;
      _error = null;
      _clipName = _clipForAction(_presentation, action.id) ?? _clipName;
    });
    try {
      final response = await client.drivePet(pet.id, action: action.id);
      if (!mounted) return;
      final updated = response.value.pet;
      setState(() {
        _pet = updated;
        _pets = [
          for (final item in _pets) item.id == updated.id ? updated : item,
        ];
      });
      _idleTimer?.cancel();
      _idleTimer = Timer(const Duration(seconds: 3), () {
        if (mounted) setState(() => _clipName = _defaultClip(_presentation));
      });
    } catch (error) {
      if (mounted) setState(() => _error = error);
    } finally {
      if (mounted) setState(() => _drivingAction = null);
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
    if (_loading && _pet == null) {
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

    final pet = _pet ?? _pets.first;
    final presentation = _presentation;
    final catalog = _catalogFor(context, presentation);
    final metrics = _petMetrics(pet, catalog);
    final actions = presentation?.drive.actions ?? const [];
    return CupertinoPageScaffold(
      child: SafeArea(
        bottom: false,
        child: CustomScrollView(
          key: const PageStorageKey('pet-scroll'),
          slivers: [
            CupertinoSliverRefreshControl(
              onRefresh: () => _loadPets(selectedId: pet.id),
            ),
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 112),
              sliver: SliverList.list(
                children: [
                  Row(
                    children: [
                      const Expanded(
                        child: Text('Pet', style: GizText.pageTitle),
                      ),
                      if (_pets.length > 1)
                        CupertinoButton(
                          padding: const EdgeInsets.all(8),
                          onPressed: () => _choosePet(context),
                          child: const Icon(
                            CupertinoIcons.chevron_up_chevron_down,
                          ),
                        ),
                    ],
                  ),
                  const SizedBox(height: 18),
                  _PetPortrait(
                    pet: pet,
                    catalog: catalog,
                    pixa: _pixa,
                    pixaError: _pixaError,
                    clipName: _clipName,
                    loading: _loading,
                  ),
                  if (_error != null) ...[
                    const SizedBox(height: 12),
                    Text(
                      _petError(_error!),
                      textAlign: TextAlign.center,
                      style: GizText.body.copyWith(
                        color: CupertinoColors.systemRed.resolveFrom(context),
                      ),
                    ),
                  ],
                  if (metrics.isNotEmpty) ...[
                    const SizedBox(height: 22),
                    const Text('Vitals', style: GizText.sectionTitle),
                    const SizedBox(height: 10),
                    _PetMetricGrid(metrics: metrics),
                  ],
                  if (actions.isNotEmpty) ...[
                    const SizedBox(height: 24),
                    const Text(
                      'Spend time together',
                      style: GizText.sectionTitle,
                    ),
                    const SizedBox(height: 10),
                    for (final action in actions)
                      _PetActionRow(
                        action: action,
                        title: _actionName(catalog, action.id),
                        busy: _drivingAction == action.id,
                        enabled: _drivingAction == null,
                        onPressed: () => _drive(action),
                      ),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _choosePet(BuildContext context) async {
    final id = await showCupertinoModalPopup<String>(
      context: context,
      builder: (context) => CupertinoActionSheet(
        title: const Text('Choose a pet'),
        actions: [
          for (final pet in _pets)
            CupertinoActionSheetAction(
              onPressed: () => Navigator.pop(context, pet.id),
              child: Text(pet.displayName.isEmpty ? pet.id : pet.displayName),
            ),
        ],
        cancelButton: CupertinoActionSheetAction(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
      ),
    );
    if (!mounted || id == null || id == _pet?.id) return;
    await _loadPet(_pets.firstWhere((pet) => pet.id == id));
  }
}

class _PetPortrait extends StatelessWidget {
  const _PetPortrait({
    required this.pet,
    required this.catalog,
    required this.pixa,
    required this.pixaError,
    required this.clipName,
    required this.loading,
  });

  final Pet pet;
  final PetPresentationI18nCatalog? catalog;
  final PixaAsset? pixa;
  final Object? pixaError;
  final String? clipName;
  final bool loading;

  @override
  Widget build(BuildContext context) {
    final name = pet.displayName.trim().isEmpty
        ? (catalog?.displayName.trim().isNotEmpty == true
              ? catalog!.displayName
              : 'Unnamed pet')
        : pet.displayName;
    final progression = pet.progression.value.entries.isEmpty
        ? pet.rulesetName
        : pet.progression.value.entries
              .map((entry) => '${_title(entry.key)} ${entry.value}')
              .join('  |  ');
    return AspectRatio(
      aspectRatio: 0.78,
      child: ClipRRect(
        borderRadius: BorderRadius.circular(12),
        child: ColoredBox(
          color: const Color(0xFFDDEFE6),
          child: Stack(
            fit: StackFit.expand,
            children: [
              Positioned(
                left: 26,
                right: 26,
                top: 42,
                bottom: 126,
                child: pixa == null
                    ? Center(
                        child: loading
                            ? const CupertinoActivityIndicator(radius: 14)
                            : Icon(
                                pixaError == null
                                    ? CupertinoIcons.sparkles
                                    : CupertinoIcons.exclamationmark_triangle,
                                size: 56,
                                color: GizColors.secondaryInk,
                              ),
                      )
                    : _AnimatedPetSprite(asset: pixa!, clipName: clipName),
              ),
              Positioned(
                left: 0,
                right: 0,
                bottom: 0,
                child: Container(
                  color: GizColors.ink,
                  padding: const EdgeInsets.fromLTRB(20, 18, 20, 20),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        name,
                        style: GizText.pageTitle.copyWith(
                          color: GizColors.surface,
                        ),
                      ),
                      const SizedBox(height: 5),
                      Text(
                        progression,
                        style: GizText.body.copyWith(
                          color: const Color(0xCFFFFFFF),
                        ),
                      ),
                      if (catalog?.description.trim().isNotEmpty == true) ...[
                        const SizedBox(height: 8),
                        Text(
                          catalog!.description,
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                          style: GizText.label.copyWith(
                            color: const Color(0xAFFFFFFF),
                          ),
                        ),
                      ],
                    ],
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
    _timer = Timer.periodic(const Duration(milliseconds: 100), (_) {
      if (mounted) {
        setState(() => _elapsed += const Duration(milliseconds: 100));
      }
    });
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

class _PetMetricGrid extends StatelessWidget {
  const _PetMetricGrid({required this.metrics});

  final List<_PetMetric> metrics;

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 10,
      runSpacing: 10,
      children: [
        for (var index = 0; index < metrics.length; index++)
          SizedBox(
            width: (MediaQuery.sizeOf(context).width - 50) / 2,
            child: Container(
              height: 88,
              padding: const EdgeInsets.all(14),
              decoration: BoxDecoration(
                color: index.isEven
                    ? GizColors.accent
                    : const Color(0xFFFFDDD2),
                borderRadius: BorderRadius.circular(8),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Text(metrics[index].label, style: GizText.label),
                  const SizedBox(height: 5),
                  Text('${metrics[index].value}', style: GizText.title),
                ],
              ),
            ),
          ),
      ],
    );
  }
}

class _PetActionRow extends StatelessWidget {
  const _PetActionRow({
    required this.action,
    required this.title,
    required this.busy,
    required this.enabled,
    required this.onPressed,
  });

  final PetPresentationActionSpec action;
  final String title;
  final bool busy;
  final bool enabled;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return GizListRow(
      leading: Container(
        width: 46,
        height: 46,
        alignment: Alignment.center,
        color: const Color(0xFFDDEFE6),
        child: Icon(_actionIcon(action.id), color: GizColors.ink),
      ),
      title: title,
      subtitle: action.cost == 0 ? 'Free' : '${action.cost} points',
      onPressed: enabled ? onPressed : () {},
      trailing: busy
          ? const CupertinoActivityIndicator()
          : const Icon(CupertinoIcons.chevron_forward, size: 18),
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
              const Text('Pet', style: GizText.pageTitle),
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
                'Adoption uses the active GizClaw gameplay ruleset.',
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
              ],
              const SizedBox(height: 22),
              CupertinoButton.filled(
                onPressed: adopting ? null : onAdopt,
                child: adopting
                    ? const CupertinoActivityIndicator()
                    : const Text('Adopt a pet'),
              ),
              if (error != null)
                CupertinoButton(onPressed: onRetry, child: const Text('Retry')),
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

IconData _actionIcon(String id) {
  final value = id.toLowerCase();
  if (value.contains('bath') || value.contains('clean')) {
    return CupertinoIcons.drop_fill;
  }
  if (value.contains('feed') || value.contains('eat')) {
    return CupertinoIcons.heart_fill;
  }
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
