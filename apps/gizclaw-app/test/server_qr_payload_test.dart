import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw_app/identity/server_qr_payload.dart';

void main() {
  test('parses a GizClaw server URI', () {
    final server = parseGizClawServerQr(
      'gizclaw://ap/office.local:9820?name=Office%20Server',
    );

    expect(server.name, 'Office Server');
    expect(server.accessPoint, 'office.local:9820');
  });

  test('requires a server name', () {
    expect(
      () => parseGizClawServerQr('gizclaw://ap/gizclaw.example.com:9820'),
      throwsFormatException,
    );
  });

  test('rejects the old server URI format', () {
    expect(
      () => parseGizClawServerQr(
        'gizclaw://server?name=Office&access_point=office.local%3A9820',
      ),
      throwsFormatException,
    );
  });

  test('rejects an access point without a port', () {
    expect(
      () =>
          parseGizClawServerQr('gizclaw://ap/gizclaw.example.com?name=Example'),
      throwsFormatException,
    );
  });
}
