import 'dart:convert';

import '../connection/gizclaw_connection_controller.dart';
import 'app_identity_store.dart';

GizClawServer parseGizClawServerQr(String payload) {
  final value = payload.trim();
  if (value.isEmpty) throw const FormatException('The QR code is empty');

  if (value.startsWith('{')) {
    final decoded = jsonDecode(value);
    if (decoded is! Map<String, Object?>) {
      throw const FormatException('The QR code does not contain a server');
    }
    return _serverFromValues(
      name: decoded['name'],
      accessPoint: decoded['access_point'] ?? decoded['accessPoint'],
    );
  }

  final uri = Uri.tryParse(value);
  if (uri != null && uri.scheme.toLowerCase() == 'gizclaw') {
    if (uri.host.toLowerCase() != 'server') {
      throw const FormatException('The GizClaw QR code is not a server code');
    }
    return _serverFromValues(
      name: uri.queryParameters['name'],
      accessPoint:
          uri.queryParameters['access_point'] ??
          uri.queryParameters['accessPoint'],
    );
  }

  final endpoint = normalizeGizClawEndpoint(value);
  return GizClawServer(name: endpoint, accessPoint: endpoint);
}

GizClawServer _serverFromValues({
  required Object? name,
  required Object? accessPoint,
}) {
  if (accessPoint is! String || accessPoint.trim().isEmpty) {
    throw const FormatException('The QR code is missing an access point');
  }
  final endpoint = normalizeGizClawEndpoint(accessPoint);
  final serverName = name is String && name.trim().isNotEmpty
      ? name.trim()
      : endpoint;
  return GizClawServer(name: serverName, accessPoint: endpoint);
}
