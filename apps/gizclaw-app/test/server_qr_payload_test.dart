import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw_app/identity/server_qr_payload.dart';

void main() {
  test('parses a GizClaw server URI', () {
    final server = parseGizClawServerQr(
      'gizclaw://server?name=Office&access_point=office.local%3A9820',
    );

    expect(server.name, 'Office');
    expect(server.accessPoint, 'office.local:9820');
  });

  test('parses a server JSON payload', () {
    final server = parseGizClawServerQr(
      '{"name":"Lab","access_point":"http://lab.local:9820"}',
    );

    expect(server.name, 'Lab');
    expect(server.accessPoint, 'http://lab.local:9820');
  });

  test('parses a plain access point and uses it as the name', () {
    final server = parseGizClawServerQr('ap.gizclaw.com:9820');

    expect(server.name, 'ap.gizclaw.com:9820');
    expect(server.accessPoint, 'ap.gizclaw.com:9820');
  });

  test('rejects a non-server GizClaw QR code', () {
    expect(
      () => parseGizClawServerQr('gizclaw://friend?token=secret'),
      throwsFormatException,
    );
  });
}
