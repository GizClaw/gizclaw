import '../connection/gizclaw_connection_controller.dart';
import 'app_identity_store.dart';

GizClawServer parseGizClawServerQr(String payload) {
  final value = payload.trim();
  if (value.isEmpty) throw const FormatException('The QR code is empty');

  final uri = Uri.tryParse(value);
  if (uri == null ||
      uri.scheme.toLowerCase() != 'gizclaw' ||
      uri.host.toLowerCase() != 'ap' ||
      uri.hasPort ||
      uri.userInfo.isNotEmpty ||
      uri.pathSegments.length != 1 ||
      uri.hasFragment) {
    throw const FormatException('The QR code is not a GizClaw server code');
  }
  final name = uri.queryParameters['name']?.trim() ?? '';
  if (name.isEmpty) {
    throw const FormatException('The QR code is missing a server name');
  }
  final endpoint = normalizeGizClawEndpoint(uri.pathSegments.single);
  final registrationToken =
      uri.queryParameters['registration_token']?.trim() ?? '';
  return GizClawServer(
    name: name,
    accessPoint: endpoint,
    registrationToken: registrationToken,
  );
}
