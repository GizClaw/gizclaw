import 'dart:convert';
import 'package:gizclaw_control/gizclaw_control.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:test/test.dart';

void main() {
  test('MHS routes preserve plain JSON values and API key', () async {
    final seen = <http.Request>[];
    final client = GizClawControlClient(
      baseUrl: Uri.parse('https://example.test'),
      apiKey: 'test-key',
      httpClient: MockClient((request) async {
        seen.add(request);
        expect(request.headers['Authorization'], 'Bearer test-key');
        if (request.method == 'GET') {
          return http.Response('{"devices":[]}', 200);
        }
        return http.Response(
          '{"states":[{"device_id":"led.main","state":"enabled","value":false}]}',
          200,
        );
      }),
    );
    addTearDown(client.close);
    expect((await client.getMhsManifest()).devices, isEmpty);
    final read = await client.readMhsStates([
      const MhsStateRef(deviceId: 'led.main', state: 'enabled'),
    ]);
    expect(read.single.value, false);
    final written = await client.writeMhsStates([
      MhsStateValue(deviceId: 'led.main', state: 'enabled', value: false),
    ]);
    expect(written.single.value, false);
    expect(seen.map((r) => r.method), ['GET', 'POST', 'PATCH']);
    expect(seen.map((r) => r.url.path), [
      '/gizclaw/v1/device/mhs/v0/manifest',
      '/gizclaw/v1/device/mhs/v0/read',
      '/gizclaw/v1/device/mhs/v0/states',
    ]);
    expect(jsonDecode(seen.last.body), {
      'states': [
        {'device_id': 'led.main', 'state': 'enabled', 'value': false},
      ],
    });
  });
  test('MHS models render product controls and reject malformed values', () {
    final manifest = MhsManifest.fromJson({
      'devices': [
        {
          'id': 'display.main',
          'kind': 'custom-display',
          'tags': ['front screen'],
          'states': [
            {
              'name': 'brightness',
              'type': 'int',
              'access': 'read_write',
              'min': 0,
              'max': 100,
              'step': 5,
            },
            {
              'name': 'mode',
              'type': 'enum',
              'access': 'read',
              'enum_values': ['auto', 'off'],
            },
          ],
        },
      ],
    });
    expect(manifest.devices.single.kind, 'custom-display');
    expect(manifest.devices.single.states.first.writable, true);
    expect(manifest.devices.single.states.last.enumValues, ['auto', 'off']);
    for (final value in [false, 0, 1.5, '']) {
      expect(
        MhsStateValue.fromJson({
          'device_id': 'd',
          'state': 's',
          'value': value,
        }).value,
        value,
      );
    }
    for (final value in [null, [], {}, double.nan]) {
      expect(
        () => MhsStateValue.fromJson({
          'device_id': 'd',
          'state': 's',
          'value': value,
        }),
        throwsFormatException,
      );
    }
  });
  test('MHS device NOT_FOUND retains the server error', () async {
    final client = GizClawControlClient(
      baseUrl: Uri.parse('https://example.test'),
      apiKey: 'test-key',
      httpClient: MockClient(
        (_) async => http.Response(
          '{"error":{"code":"MHS_STATE_NOT_FOUND","message":"device has no matching resource"}}',
          404,
        ),
      ),
    );
    addTearDown(client.close);
    await expectLater(
      client.readMhsStates([
        const MhsStateRef(deviceId: 'led.main', state: 'missing'),
      ]),
      throwsA(
        isA<GizClawControlException>()
            .having((e) => e.kind, 'kind', GizClawControlErrorKind.notFound)
            .having((e) => e.code, 'code', 'MHS_STATE_NOT_FOUND'),
      ),
    );
  });
  test('MHS missing response states is a malformed response', () async {
    final client = GizClawControlClient(
      baseUrl: Uri.parse('https://example.test'),
      apiKey: 'test-key',
      httpClient: MockClient((_) async => http.Response('{}', 200)),
    );
    addTearDown(client.close);
    await expectLater(
      client.readMhsStates([
        const MhsStateRef(deviceId: 'led.main', state: 'enabled'),
      ]),
      throwsA(
        isA<GizClawControlException>().having(
          (e) => e.kind,
          'kind',
          GizClawControlErrorKind.malformedResponse,
        ),
      ),
    );
  });
}
