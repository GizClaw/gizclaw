import 'package:flutter/cupertino.dart';
import 'package:go_router/go_router.dart';
import 'package:mobile_scanner/mobile_scanner.dart';

import '../../data/mobile_data_controller.dart';
import '../../giz_ui/giz_ui.dart';
import '../../identity/app_identity_store.dart';
import '../../identity/server_qr_payload.dart';

class AddServerPage extends StatefulWidget {
  const AddServerPage({super.key});

  @override
  State<AddServerPage> createState() => _AddServerPageState();
}

class _AddServerPageState extends State<AddServerPage> {
  final _nameController = TextEditingController();
  final _accessPointController = TextEditingController();
  bool _busy = false;
  Object? _error;

  @override
  void dispose() {
    _nameController.dispose();
    _accessPointController.dispose();
    super.dispose();
  }

  Future<void> _scan() async {
    final server = await context.push<GizClawServer>('/identity/servers/scan');
    if (!mounted || server == null) return;
    _nameController.text = server.name;
    _accessPointController.text = server.accessPoint;
    setState(() => _error = null);
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
      navigationBar: const CupertinoNavigationBar(
        middle: Text('Add Server'),
        border: null,
      ),
      child: SafeArea(
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 24, 20, 32),
          children: [
            Text(
              'Add a server by entering its details or scanning a GizClaw QR code.',
              style: GizText.body.copyWith(color: GizColors.secondaryInk),
            ),
            const SizedBox(height: 20),
            CupertinoButton(
              key: const ValueKey('scan-server-qr'),
              color: GizColors.surface,
              padding: const EdgeInsets.symmetric(vertical: 15),
              onPressed: _busy ? null : _scan,
              child: const Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Icon(GizIcons.qr_code, size: 22),
                  SizedBox(width: 8),
                  Text('Scan QR Code'),
                ],
              ),
            ),
            const SizedBox(height: 28),
            Text(
              'SERVER DETAILS',
              style: GizText.label.copyWith(color: GizColors.secondaryInk),
            ),
            const SizedBox(height: 8),
            CupertinoTextField(
              key: const ValueKey('server-name-field'),
              controller: _nameController,
              placeholder: 'Name',
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
            if (_error != null) ...[
              const SizedBox(height: 10),
              Text(
                _serverErrorMessage(_error!),
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
                  : const Text('Add Server'),
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
      navigationBar: const CupertinoNavigationBar(
        middle: Text('Scan Server'),
        backgroundColor: Color(0xCC000000),
        border: null,
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
              _error ?? 'Point the camera at a GizClaw server QR code.',
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
                ? 'Camera access is required to scan a server QR code. Enable it in Settings and try again.'
                : 'The camera could not start. Go back and try again.',
            textAlign: TextAlign.center,
            style: GizText.body.copyWith(color: CupertinoColors.white),
          ),
        ),
      ),
    );
  }
}

String _serverErrorMessage(Object error) {
  if (error is FormatException) return error.message;
  return 'Could not add the server. Please try again.';
}
