import 'package:flutter/cupertino.dart';
import 'package:go_router/go_router.dart';
import 'package:mobile_scanner/mobile_scanner.dart';

import '../../data/mobile_data_controller.dart';
import '../../giz_ui/giz_ui.dart';
import '../../identity/app_identity_store.dart';
import '../../identity/server_qr_payload.dart';
import '../../l10n/l10n.dart';

class ServerListPage extends StatefulWidget {
  const ServerListPage({
    super.key,
    this.addServerRoute = '/identity/servers/new',
  });

  final String addServerRoute;

  @override
  State<ServerListPage> createState() => _ServerListPageState();
}

class _ServerListPageState extends State<ServerListPage> {
  String? _switchingEndpoint;
  Object? _error;

  Future<void> _select(GizClawServer server) async {
    if (_switchingEndpoint != null) return;
    setState(() {
      _switchingEndpoint = server.accessPoint;
      _error = null;
    });
    try {
      await MobileDataScope.watch(context).selectServer(server);
    } catch (error) {
      _error = error;
    } finally {
      if (mounted) setState(() => _switchingEndpoint = null);
    }
  }

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    return CupertinoPageScaffold(
      navigationBar: CupertinoNavigationBar(
        middle: Text(context.l10n.uiText(key: 'servers')),
        border: null,
        transitionBetweenRoutes: false,
        trailing: GizPageActionButton(
          key: const ValueKey('add-server-page-button'),
          icon: GizIcons.add,
          semanticLabel: context.l10n.addServerA11y,
          onPressed: () => context.push(widget.addServerRoute),
        ),
      ),
      child: SafeArea(
        child: ListView(
          padding: const EdgeInsets.only(top: 18, bottom: 32),
          children: [
            if (data.servers.isEmpty)
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 14),
                child: Text(
                  'No servers added yet. Use Add server to continue.',
                  style: GizText.body.copyWith(color: GizColors.secondaryInk),
                ),
              ),
            if (data.servers.isNotEmpty && !data.hasActiveServer)
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 14),
                child: Text(
                  context.l10n.uiText(key: 'chooseServerSetup'),
                  style: GizText.body.copyWith(color: GizColors.secondaryInk),
                ),
              ),
            if (_error != null)
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 14),
                child: Text(
                  context.l10n.uiText(key: 'switchServerFailed'),
                  key: const ValueKey('switch-server-error'),
                  style: GizText.body.copyWith(
                    color: CupertinoColors.systemRed.resolveFrom(context),
                  ),
                ),
              ),
            for (final server in data.servers)
              _ServerListRow(
                key: ValueKey('server-${server.accessPoint}'),
                server: server,
                selected: data.activeServer?.accessPoint == server.accessPoint,
                switching: _switchingEndpoint == server.accessPoint,
                onPressed: () => _select(server),
              ),
          ],
        ),
      ),
    );
  }
}

class _ServerListRow extends StatelessWidget {
  const _ServerListRow({
    super.key,
    required this.server,
    required this.selected,
    required this.switching,
    required this.onPressed,
  });

  final GizClawServer server;
  final bool selected;
  final bool switching;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return GizListRow(
      leading: SizedBox(
        width: 36,
        height: 36,
        child: Icon(
          GizIcons.antenna_radiowaves_left_right,
          size: 22,
          color: selected ? GizColors.primary : GizColors.secondaryInk,
        ),
      ),
      title: server.name,
      subtitle: server.accessPoint,
      onPressed: selected || switching ? null : onPressed,
      trailing: switching
          ? const CupertinoActivityIndicator(radius: 10)
          : selected
          ? const Icon(
              GizIcons.checkmark_alt,
              key: ValueKey('selected-server'),
              size: 20,
              color: GizColors.primary,
            )
          : null,
    );
  }
}

class AddServerPage extends StatefulWidget {
  const AddServerPage({
    super.key,
    this.scanServerRoute = '/identity/servers/scan',
  });

  final String scanServerRoute;

  @override
  State<AddServerPage> createState() => _AddServerPageState();
}

class _AddServerPageState extends State<AddServerPage> {
  final _nameController = TextEditingController();
  final _accessPointController = TextEditingController();
  final _registrationTokenController = TextEditingController();
  bool _busy = false;
  Object? _error;

  @override
  void dispose() {
    _nameController.dispose();
    _accessPointController.dispose();
    _registrationTokenController.dispose();
    super.dispose();
  }

  Future<void> _scan() async {
    final server = await context.push<GizClawServer>(widget.scanServerRoute);
    if (!mounted || server == null) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await MobileDataScope.watch(context).addOrSelectServer(
        name: server.name,
        accessPoint: server.accessPoint,
        registrationToken: server.registrationToken,
      );
      if (mounted) Navigator.of(context).pop();
    } catch (error) {
      if (!mounted) return;
      setState(() {
        _busy = false;
        _error = error;
      });
    }
  }

  Future<void> _add() async {
    if (_busy) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await MobileDataScope.watch(context).addServer(
        name: _nameController.text,
        accessPoint: _accessPointController.text,
        registrationToken: _registrationTokenController.text,
      );
      if (mounted) Navigator.of(context).pop();
    } catch (error) {
      if (!mounted) return;
      setState(() {
        _busy = false;
        _error = error;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return CupertinoPageScaffold(
      navigationBar: CupertinoNavigationBar(
        middle: Text(context.l10n.uiText(key: 'addServer')),
        border: null,
        transitionBetweenRoutes: false,
      ),
      child: SafeArea(
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 24, 20, 32),
          children: [
            Text(
              context.l10n.uiText(key: 'addServerDescription'),
              style: GizText.body.copyWith(color: GizColors.secondaryInk),
            ),
            const SizedBox(height: 20),
            CupertinoButton(
              key: const ValueKey('scan-server-qr'),
              color: GizColors.surface,
              padding: const EdgeInsets.symmetric(vertical: 15),
              onPressed: _busy ? null : _scan,
              child: Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  const Icon(GizIcons.qr_code, size: 22),
                  const SizedBox(width: 8),
                  Text(context.l10n.uiText(key: 'scanQr')),
                ],
              ),
            ),
            const SizedBox(height: 28),
            Text(
              context.l10n.uiText(key: 'serverDetails'),
              style: GizText.label.copyWith(color: GizColors.secondaryInk),
            ),
            const SizedBox(height: 8),
            CupertinoTextField(
              key: const ValueKey('server-name-field'),
              controller: _nameController,
              placeholder: context.l10n.uiText(key: 'name'),
              textInputAction: TextInputAction.next,
              padding: const EdgeInsets.all(14),
              onChanged: (_) => setState(() => _error = null),
            ),
            const SizedBox(height: 10),
            CupertinoTextField(
              key: const ValueKey('server-access-point-field'),
              controller: _accessPointController,
              placeholder: 'gizclaw.example.com:9820',
              keyboardType: TextInputType.url,
              autocorrect: false,
              enableSuggestions: false,
              textInputAction: TextInputAction.done,
              padding: const EdgeInsets.all(14),
              onChanged: (_) => setState(() => _error = null),
              onSubmitted: (_) => _add(),
            ),
            const SizedBox(height: 10),
            CupertinoTextField(
              key: const ValueKey('server-registration-token-field'),
              controller: _registrationTokenController,
              placeholder: 'Registration token (optional)',
              obscureText: true,
              autocorrect: false,
              enableSuggestions: false,
              textInputAction: TextInputAction.done,
              padding: const EdgeInsets.all(14),
              onChanged: (_) => setState(() => _error = null),
              onSubmitted: (_) => _add(),
            ),
            if (_error != null) ...[
              const SizedBox(height: 10),
              Text(
                _serverErrorMessage(context, _error!),
                key: const ValueKey('add-server-error'),
                style: GizText.body.copyWith(
                  color: CupertinoColors.systemRed.resolveFrom(context),
                ),
              ),
            ],
            const SizedBox(height: 16),
            CupertinoButton.filled(
              key: const ValueKey('add-server'),
              onPressed: _busy ? null : _add,
              child: _busy
                  ? const CupertinoActivityIndicator()
                  : Text(context.l10n.uiText(key: 'addServer')),
            ),
          ],
        ),
      ),
    );
  }
}

class ScanServerQrPage extends StatefulWidget {
  const ScanServerQrPage({super.key});

  @override
  State<ScanServerQrPage> createState() => _ScanServerQrPageState();
}

class _ScanServerQrPageState extends State<ScanServerQrPage> {
  bool _handled = false;
  String? _error;

  void _onDetect(BarcodeCapture capture) {
    if (_handled) return;
    final value = capture.barcodes
        .map((barcode) => barcode.rawValue)
        .whereType<String>()
        .firstOrNull;
    if (value == null) return;
    try {
      final server = parseGizClawServerQr(value);
      _handled = true;
      Navigator.of(context).pop(server);
    } on FormatException catch (error) {
      setState(() => _error = error.message);
    }
  }

  @override
  Widget build(BuildContext context) {
    return CupertinoPageScaffold(
      backgroundColor: CupertinoColors.black,
      navigationBar: CupertinoNavigationBar(
        middle: Text(context.l10n.uiText(key: 'scanServer')),
        backgroundColor: Color(0xCC000000),
        border: null,
        transitionBetweenRoutes: false,
      ),
      child: Stack(
        fit: StackFit.expand,
        children: [
          MobileScanner(
            key: const ValueKey('server-qr-scanner'),
            onDetect: _onDetect,
            errorBuilder: (context, error) => _ScannerError(error: error),
          ),
          IgnorePointer(
            child: Center(
              child: Container(
                width: 250,
                height: 250,
                decoration: BoxDecoration(
                  border: Border.all(color: CupertinoColors.white, width: 3),
                  borderRadius: BorderRadius.circular(24),
                ),
              ),
            ),
          ),
          Positioned(
            left: 24,
            right: 24,
            bottom: 44 + MediaQuery.paddingOf(context).bottom,
            child: Text(
              _error == null
                  ? context.l10n.uiText(key: 'pointCamera')
                  : context.l10n.uiText(key: 'serverAddFailed'),
              textAlign: TextAlign.center,
              style: GizText.body.copyWith(
                color: _error == null
                    ? CupertinoColors.white
                    : CupertinoColors.systemRed,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _ScannerError extends StatelessWidget {
  const _ScannerError({required this.error});

  final MobileScannerException error;

  @override
  Widget build(BuildContext context) {
    final permissionDenied =
        error.errorCode == MobileScannerErrorCode.permissionDenied;
    return ColoredBox(
      color: CupertinoColors.black,
      child: Center(
        child: Padding(
          padding: const EdgeInsets.all(28),
          child: Text(
            permissionDenied
                ? context.l10n.uiText(key: 'cameraRequired')
                : context.l10n.uiText(key: 'cameraFailed'),
            textAlign: TextAlign.center,
            style: GizText.body.copyWith(color: CupertinoColors.white),
          ),
        ),
      ),
    );
  }
}

String _serverErrorMessage(BuildContext context, Object error) {
  if (error is FormatException) {
    final key = switch (error.message) {
      'Enter a server name' => 'serverNameRequired',
      'Enter a server access point' => 'serverAccessPointRequired',
      'Use a domain or IP address with a port, for example gizclaw.local:9820' =>
        'serverAccessPointInvalid',
      'This access point is already in the list' =>
        'serverAccessPointDuplicate',
      _ => 'serverAddFailed',
    };
    return context.l10n.uiText(key: key);
  }
  return context.l10n.uiText(key: 'serverAddFailed');
}
