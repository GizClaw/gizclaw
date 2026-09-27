import 'dart:convert';

import 'package:gizclaw_control/gizclaw_control.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:test/test.dart';

void main() {
  test(
    'MHS manifest lists instances and control calls use typed HWD values',
    () async {
      final seen = <http.Request>[];
      final client = GizClawControlClient(
        baseUrl: Uri.parse('https://example.test'),
        apiKey: 'test-key',
        httpClient: MockClient((request) async {
          seen.add(request);
          expect(request.headers['Authorization'], 'Bearer test-key');
          if (request.method == 'GET') {
            return http.Response(
              '{"devices":[{"id":"led.left","hwd":"led"},{"id":"led.right","hwd":"led"}]}',
              200,
            );
          }
          return http.Response(
            '{"id":"led.left","hwd":"led","value":{"enabled":false,"brightness_percent":0}}',
            200,
          );
        }),
      );
      addTearDown(client.close);
      final manifest = await client.getMhsManifest();
      expect(manifest.devices.map((device) => device.id), [
        'led.left',
        'led.right',
      ]);
      final read = await client.readMhsHwd('led.left', 'led');
      expect((read.value as MhsLedReadValue).enabled, false);
      final written = await client.writeMhsHwd(
        const MhsHwdWriteRequest(
          id: 'led.left',
          value: MhsLedWriteValue(enabled: false, brightnessPercent: 0),
        ),
      );
      expect((written.value as MhsLedReadValue).brightnessPercent, 0);
      expect(seen.map((request) => request.method), ['GET', 'POST', 'POST']);
      expect(seen.map((request) => request.url.path), [
        '/gizclaw/v1/device/mhs/v0/manifest',
        '/gizclaw/v1/device/mhs/v0/read',
        '/gizclaw/v1/device/mhs/v0/write',
      ]);
      expect(jsonDecode(seen[1].body), {'id': 'led.left', 'hwd': 'led'});
      expect(jsonDecode(seen[2].body), {
        'id': 'led.left',
        'hwd': 'led',
        'value': {'enabled': false, 'brightness_percent': 0},
      });
    },
  );

  test('MHS models decode all eight HWD read structures', () {
    final values = <String, Map<String, Object>>{
      'wifi': {'connected': true},
      'ble': {'powered': true},
      'modem': {'registered': true},
      'battery': {'percent': 80.0},
      'mic': {'available': true},
      'display': {'brightness_percent': 0},
      'led': {'enabled': false},
      'speaker': {'volume_percent': 0},
    };
    for (final entry in values.entries) {
      final result = MhsHwdReadResult.fromJson({
        'id': '${entry.key}.main',
        'hwd': entry.key,
        'value': entry.value,
      });
      expect(result.hwd, entry.key);
    }
    expect(
      () => MhsDevice.fromJson({'id': 'device.main', 'hwd': 'device'}),
      throwsFormatException,
    );
    expect(
      () =>
          MhsHwdReadResult.fromJson({'id': 'x', 'hwd': 'device', 'value': {}}),
      throwsFormatException,
    );
    expect(
      () => MhsHwdReadResult.fromJson({
        'id': 'led.main',
        'hwd': 'led',
        'value': {},
      }),
      throwsFormatException,
    );
  });

  test('MHS HWD NOT_FOUND retains the server error', () async {
    final client = GizClawControlClient(
      baseUrl: Uri.parse('https://example.test'),
      apiKey: 'test-key',
      httpClient: MockClient(
        (_) async => http.Response(
          '{"error":{"code":"MHS_HWD_NOT_FOUND","message":"device has no matching resource"}}',
          404,
        ),
      ),
    );
    addTearDown(client.close);
    await expectLater(
      client.readMhsHwd('led.main', 'led'),
      throwsA(
        isA<GizClawControlException>()
            .having(
              (error) => error.kind,
              'kind',
              GizClawControlErrorKind.notFound,
            )
            .having((error) => error.code, 'code', 'MHS_HWD_NOT_FOUND'),
      ),
    );
  });

  test('MHS missing HWD value is a malformed response', () async {
    final client = GizClawControlClient(
      baseUrl: Uri.parse('https://example.test'),
      apiKey: 'test-key',
      httpClient: MockClient((_) async => http.Response('{}', 200)),
    );
    addTearDown(client.close);
    await expectLater(
      client.readMhsHwd('led.main', 'led'),
      throwsA(
        isA<GizClawControlException>().having(
          (error) => error.kind,
          'kind',
          GizClawControlErrorKind.malformedResponse,
        ),
      ),
    );
  });
}
