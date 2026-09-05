import 'dart:async';
import 'dart:convert';

import 'package:gizclaw_control/gizclaw_control.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:test/test.dart';

const apiKey = 'gizclaw_sk_v1_0123456789abcdefghijklmnopqrstuvwxyzABCDEFG';
final baseUrl = Uri.parse('https://ap.gizclaw.com');

const apiKeyJson = {
  'name': 'key_0123456789abcdefghijkl',
  'display_name': 'LiteLink',
  'prefix': 'gizclaw_sk_v1_0123',
  'api_key': apiKey,
  'manage_api_keys': true,
  'created_at': '2026-09-03T01:02:03Z',
};

const contactJson = {
  'name': 'alice',
  'display_name': 'Alice',
  'phone_number': '+8613800000000',
  'created_at': '2026-09-03T01:02:03Z',
  'updated_at': '2026-09-03T01:02:04Z',
};

/// Records every request and answers with [responses] in order.
class Recorder {
  Recorder(this.responses);

  final List<http.Response Function(http.Request request)> responses;
  final List<http.Request> requests = [];

  http.Client get client => MockClient((request) async {
    requests.add(request);
    if (responses.isEmpty) {
      throw StateError('no response queued for ${request.url}');
    }
    return responses.removeAt(0)(request);
  });

  http.Request get single {
    expect(requests, hasLength(1));
    return requests.single;
  }
}

http.Response Function(http.Request) json(int status, Object body) {
  return (_) => http.Response(
    jsonEncode(body),
    status,
    headers: {'content-type': 'application/json'},
  );
}

http.Response Function(http.Request) error(
  int status,
  String code, {
  String message = 'failed',
  Map<String, String> headers = const {},
}) {
  return (_) => http.Response(
    jsonEncode({
      'error': {'code': code, 'message': message},
    }),
    status,
    headers: {'content-type': 'application/json', ...headers},
  );
}

http.Response Function(http.Request) noContent() {
  return (_) => http.Response('', 204);
}

http.Response Function(http.Request) accepted() {
  return (_) => http.Response('', 202);
}

GizClawControlClient clientWith(
  Recorder recorder, {
  Uri? base,
  Duration timeout = const Duration(seconds: 5),
}) {
  return GizClawControlClient(
    baseUrl: base ?? baseUrl,
    apiKey: apiKey,
    httpClient: recorder.client,
    timeout: timeout,
  );
}

Future<GizClawControlException> failure(Future<Object?> future) async {
  try {
    await future;
  } on GizClawControlException catch (exception) {
    return exception;
  }
  fail('expected GizClawControlException');
}

void main() {
  group('constructor', () {
    test('rejects an empty API key', () {
      expect(
        () => GizClawControlClient(baseUrl: baseUrl, apiKey: ''),
        throwsArgumentError,
      );
    });

    test('rejects a plaintext base URL unless allowed', () {
      expect(
        () => GizClawControlClient(
          baseUrl: Uri.parse('http://ap.gizclaw.com'),
          apiKey: apiKey,
        ),
        throwsArgumentError,
      );
      // A local test deployment opts in explicitly.
      final client = GizClawControlClient(
        allowInsecureTransport: true,
        apiKey: apiKey,
        baseUrl: Uri.parse('http://127.0.0.1:9821'),
      );
      expect(client.baseUrl.scheme, 'http');
      client.close();
    });

    test('rejects a non-http scheme', () {
      expect(
        () => GizClawControlClient(
          baseUrl: Uri.parse('ftp://ap.gizclaw.com'),
          apiKey: apiKey,
        ),
        throwsArgumentError,
      );
    });

    test('rejects a relative base URL', () {
      expect(
        () => GizClawControlClient(
          baseUrl: Uri.parse('/gizclaw'),
          apiKey: apiKey,
        ),
        throwsArgumentError,
      );
    });
  });

  group('request shape', () {
    test('sends the bearer header and JSON accept on every request', () async {
      final recorder = Recorder([
        json(200, apiKeyJson),
        noContent(),
        json(200, {'connected': false}),
      ]);
      final client = clientWith(recorder);
      await client.getSelfApiKey();
      await client.revokeSelfApiKey();
      await client.getDeviceWifi();
      expect(recorder.requests, hasLength(3));
      for (final request in recorder.requests) {
        expect(request.headers['Authorization'], 'Bearer $apiKey');
        expect(request.headers['Accept'], 'application/json');
      }
    });

    test('joins a base URL path prefix without doubling slashes', () async {
      final recorder = Recorder([
        json(200, {'online': true, 'last_seen_at': '2026-09-03T00:00:00Z'}),
      ]);
      final client = clientWith(
        recorder,
        base: Uri.parse('https://example.test/prefix/'),
      );
      await client.getDeviceRuntime();
      expect(
        recorder.single.url.toString(),
        'https://example.test/prefix/gizclaw/v1/device/runtime',
      );
    });

    test('omits the query string when no parameter is set', () async {
      final recorder = Recorder([
        json(200, {'items': []}),
      ]);
      await clientWith(recorder).listApiKeys();
      expect(recorder.single.url.hasQuery, isFalse);
      expect(
        recorder.single.url.toString(),
        'https://ap.gizclaw.com/gizclaw/v1/api-keys',
      );
    });
  });

  group('api keys', () {
    test('createApiKey posts the request body and decodes 201', () async {
      final recorder = Recorder([
        json(201, {'value': apiKeyJson, 'api_key': apiKey}),
      ]);
      final result = await clientWith(recorder).createApiKey(
        const ApiKeyCreateRequest(
          displayName: 'LiteLink',
          manageApiKeys: false,
        ),
      );
      final request = recorder.single;
      expect(request.method, 'POST');
      expect(request.url.path, '/gizclaw/v1/api-keys');
      expect(request.headers['Content-Type'], startsWith('application/json'));
      expect(jsonDecode(request.body), {
        'display_name': 'LiteLink',
        'manage_api_keys': false,
      });
      expect(result.apiKey, apiKey);
      expect(result.value.name, 'key_0123456789abcdefghijkl');
      expect(result.value.createdAt, DateTime.utc(2026, 9, 3, 1, 2, 3));
    });

    test('listApiKeys passes cursor and limit', () async {
      final recorder = Recorder([
        json(200, {
          'items': [apiKeyJson],
          'next_cursor': 'key_abcdefghijklmnopqrstuv',
        }),
      ]);
      final list = await clientWith(
        recorder,
      ).listApiKeys(cursor: 'key_0123456789abcdefghijkl', limit: 10);
      final request = recorder.single;
      expect(request.method, 'GET');
      expect(request.url.path, '/gizclaw/v1/api-keys');
      expect(request.url.queryParameters, {
        'cursor': 'key_0123456789abcdefghijkl',
        'limit': '10',
      });
      expect(list.items, hasLength(1));
      expect(list.nextCursor, 'key_abcdefghijklmnopqrstuv');
    });

    test('self routes', () async {
      final recorder = Recorder([json(200, apiKeyJson), noContent()]);
      final client = clientWith(recorder);
      final key = await client.getSelfApiKey();
      await client.revokeSelfApiKey();
      expect(key.manageApiKeys, isTrue);
      expect(recorder.requests[0].method, 'GET');
      expect(recorder.requests[0].url.path, '/gizclaw/v1/api-keys/self');
      expect(recorder.requests[1].method, 'DELETE');
      expect(recorder.requests[1].url.path, '/gizclaw/v1/api-keys/self');
    });

    test('named routes encode the key name', () async {
      final recorder = Recorder([json(200, apiKeyJson), noContent()]);
      final client = clientWith(recorder);
      await client.getApiKey('key_0123456789abcdefghijkl');
      await client.revokeApiKey('key_0123456789abcdefghijkl');
      expect(
        recorder.requests[0].url.path,
        '/gizclaw/v1/api-keys/key_0123456789abcdefghijkl',
      );
      expect(recorder.requests[1].method, 'DELETE');
      expect(
        recorder.requests[1].url.path,
        '/gizclaw/v1/api-keys/key_0123456789abcdefghijkl',
      );
    });
  });

  group('device reads', () {
    test('getDevice keeps unmodeled keys in raw', () async {
      final recorder = Recorder([
        json(200, {
          'name': 'Kitchen',
          'emoji': '🍳',
          'hardware': {'manufacturer': 'GizClaw', 'model': 'G1'},
          'identifiers': {
            'sn': 'SN-1',
            'imeis': [
              {'tac': '35012345', 'serial': '123456'},
            ],
            'labels': [
              {'key': 'room', 'value': 'kitchen'},
            ],
          },
          'firmware': {'version': '9.9.9'},
        }),
      ]);
      final device = await clientWith(recorder).getDevice();
      expect(recorder.single.url.path, '/gizclaw/v1/device');
      expect(device.name, 'Kitchen');
      expect(device.hardware?.model, 'G1');
      expect(device.identifiers?.sn, 'SN-1');
      expect(device.identifiers?.imeis.single.tac, '35012345');
      expect(device.identifiers?.labels.single.value, 'kitchen');
      expect(device.raw['firmware'], {'version': '9.9.9'});
    });

    test('getDeviceRuntime decodes counters', () async {
      final recorder = Recorder([
        json(200, {
          'online': true,
          'last_seen_at': '2026-09-03T00:00:00Z',
          'last_addr': '203.0.113.1:5000',
          'rx_bytes': 10,
          'tx_bytes': 20,
        }),
      ]);
      final runtime = await clientWith(recorder).getDeviceRuntime();
      expect(runtime.online, isTrue);
      expect(runtime.lastAddr, '203.0.113.1:5000');
      expect(runtime.rxBytes, 10);
      expect(runtime.txBytes, 20);
    });

    test('getDeviceStatus decodes the typed core and raw', () async {
      final recorder = Recorder([
        json(200, {
          'reported_at': '2026-09-03T00:00:00Z',
          'volume': 35,
          'muted': false,
          'battery_percent': 80,
          'charging': true,
          'gnss_latitude': 31.2,
          'gnss_longitude': 121.5,
          'labels': {'mode': 'home'},
          'details': {'firmware': '1.2.3'},
          'future_field': 1,
        }),
      ]);
      final status = await clientWith(recorder).getDeviceStatus();
      expect(recorder.single.url.path, '/gizclaw/v1/device/status');
      expect(status.volume, 35);
      expect(status.muted, isFalse);
      expect(status.batteryPercent, 80);
      expect(status.charging, isTrue);
      expect(status.gnssLatitude, 31.2);
      expect(status.labels, {'mode': 'home'});
      expect(status.details, {'firmware': '1.2.3'});
      expect(status.raw['future_field'], 1);
    });

    test('getDeviceTelemetryLatest selects one path field', () async {
      final recorder = Recorder([
        json(200, {
          'peer_public_key': 'pk',
          'values': [
            {
              'field': 'battery.percent',
              'value': 80,
              'observed_at_unix_ms': 1000,
            },
          ],
        }),
      ]);
      final latest = await clientWith(
        recorder,
      ).getDeviceTelemetryLatest(field: PeerTelemetryField.batteryPercent);
      expect(
        recorder.single.url.path,
        '/gizclaw/v1/device/telemetry/battery.percent/latest',
      );
      expect(recorder.single.url.hasQuery, isFalse);
      expect(latest.values.single.value, 80.0);
    });

    test('getDeviceTelemetryLatest encodes the field', () async {
      final recorder = Recorder([
        json(200, {'peer_public_key': 'pk', 'values': []}),
      ]);
      await clientWith(
        recorder,
      ).getDeviceTelemetryLatest(field: 'invalid/field');
      expect(recorder.single.url.hasQuery, isFalse);
    });

    test('queryDeviceTelemetry sends every query parameter', () async {
      final recorder = Recorder([
        json(200, {
          'peer_public_key': 'pk',
          'field': 'battery.percent',
          'start_time_ms': 0,
          'end_time_ms': 1000,
          'step_ms': 100,
          'points': [
            {'observed_at_unix_ms': 0, 'value': 1.5},
          ],
        }),
      ]);
      final range = await clientWith(recorder).queryDeviceTelemetry(
        field: PeerTelemetryField.batteryPercent,
        startTimeMs: 0,
        endTimeMs: 1000,
        stepMs: 100,
        limit: 5,
        order: PeerTelemetryOrder.desc,
      );
      expect(recorder.single.url.path, '/gizclaw/v1/device/telemetry');
      expect(recorder.single.url.queryParameters, {
        'field': 'battery.percent',
        'start_time_ms': '0',
        'end_time_ms': '1000',
        'step_ms': '100',
        'limit': '5',
        'order': 'desc',
      });
      expect(range.points.single.value, 1.5);
      expect(range.stepMs, 100);
    });

    test('aggregateDeviceTelemetry sends the aggregate mode', () async {
      final recorder = Recorder([
        json(200, {
          'peer_public_key': 'pk',
          'field': 'battery.percent',
          'aggregate': 'avg',
          'bucket_ms': 60000,
          'points': [
            {'bucket_start_time_ms': 0, 'value': 2},
          ],
        }),
      ]);
      final aggregate = await clientWith(recorder).aggregateDeviceTelemetry(
        field: PeerTelemetryField.batteryPercent,
        startTimeMs: 0,
        endTimeMs: 1000,
        bucketMs: 60000,
        aggregate: PeerTelemetryAggregate.avg,
      );
      expect(
        recorder.single.url.path,
        '/gizclaw/v1/device/telemetry/aggregate',
      );
      expect(recorder.single.url.queryParameters, {
        'field': 'battery.percent',
        'start_time_ms': '0',
        'end_time_ms': '1000',
        'bucket_ms': '60000',
        'aggregate': 'avg',
      });
      expect(aggregate.aggregate, 'avg');
      expect(aggregate.points.single.bucketStartTimeMs, 0);
    });
  });

  group('device control', () {
    test('setDeviceVolume puts the body and returns the status', () async {
      final recorder = Recorder([
        json(200, {
          'status': {'volume': 35, 'muted': false},
        }),
      ]);
      final result = await clientWith(
        recorder,
      ).setDeviceVolume(level: 35, muted: false);
      expect(recorder.single.method, 'PUT');
      expect(recorder.single.url.path, '/gizclaw/v1/device/volume');
      expect(jsonDecode(recorder.single.body), {'level': 35, 'muted': false});
      expect(result.status.volume, 35);
      expect(result.status.muted, isFalse);
    });

    test('playDeviceSound posts and accepts 204', () async {
      final recorder = Recorder([noContent()]);
      await clientWith(
        recorder,
      ).playDeviceSound(sound: 'chime', durationMs: 500);
      expect(recorder.single.method, 'POST');
      expect(recorder.single.url.path, '/gizclaw/v1/device/actions/play-sound');
      expect(jsonDecode(recorder.single.body), {
        'sound': 'chime',
        'duration_ms': 500,
      });
    });

    test('playDeviceSound omits duration when unset', () async {
      final recorder = Recorder([noContent()]);
      await clientWith(recorder).playDeviceSound(sound: 'chime');
      expect(jsonDecode(recorder.single.body), {'sound': 'chime'});
    });

    test('rebootDevice posts an empty object by default', () async {
      final recorder = Recorder([noContent(), noContent()]);
      final client = clientWith(recorder);
      await client.rebootDevice();
      await client.rebootDevice(delayMs: 3000);
      expect(recorder.requests[0].method, 'POST');
      expect(
        recorder.requests[0].url.path,
        '/gizclaw/v1/device/actions/reboot',
      );
      expect(jsonDecode(recorder.requests[0].body), <String, Object?>{});
      expect(jsonDecode(recorder.requests[1].body), {'delay_ms': 3000});
    });

    test('getDeviceFirmware decodes every channel', () async {
      final recorder = Recorder([
        json(200, {
          'description': 'Devkit firmware channels',
          'slots': {
            'stable': {
              'description': 'Devkit firmware 1.0.3',
              'package': {
                'url': 'https://firmware.example.com/devkit/1.0.3.tar.zlib',
                'sha256':
                    'a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90',
                'size': 4096,
              },
            },
            'beta': {
              'package': {
                'url': 'https://firmware.example.com/devkit/1.1.0.tar.zlib',
                'sha256':
                    'b1c2d3e4f5061728394a5b6c7d8e9f0ab1c2d3e4f5061728394a5b6c7d8e9f0a',
                'size': 8192,
              },
            },
            'develop': <String, Object?>{},
          },
        }),
      ]);
      final firmware = await clientWith(recorder).getDeviceFirmware();
      expect(recorder.single.method, 'GET');
      expect(recorder.single.url.path, '/gizclaw/v1/device/firmware');
      expect(firmware.description, 'Devkit firmware channels');
      expect(firmware.stable.description, 'Devkit firmware 1.0.3');
      expect(firmware.stable.package?.size, 4096);
      expect(
        firmware.slot(FirmwareChannelName.beta).package?.url,
        'https://firmware.example.com/devkit/1.1.0.tar.zlib',
      );
      // An unconfigured channel decodes as an empty slot, not as an error.
      expect(firmware.develop.package, isNull);
      expect(firmware.develop.description, isNull);
    });

    test('getDeviceFirmware maps an unbound device to notFound', () async {
      final recorder = Recorder([error(404, 'FIRMWARE_NOT_FOUND')]);
      final exception = await failure(clientWith(recorder).getDeviceFirmware());
      expect(exception.kind, GizClawControlErrorKind.notFound);
      expect(exception.code, 'FIRMWARE_NOT_FOUND');
    });

    test('updateDeviceFirmware posts an empty object by default', () async {
      final recorder = Recorder([noContent(), noContent()]);
      final client = clientWith(recorder);
      await client.updateDeviceFirmware();
      await client.updateDeviceFirmware(
        channel: FirmwareChannelName.beta,
        sha256:
            'b1c2d3e4f5061728394a5b6c7d8e9f0ab1c2d3e4f5061728394a5b6c7d8e9f0a',
      );
      expect(recorder.requests[0].method, 'POST');
      expect(
        recorder.requests[0].url.path,
        '/gizclaw/v1/device/actions/firmware-update',
      );
      expect(jsonDecode(recorder.requests[0].body), <String, Object?>{});
      expect(jsonDecode(recorder.requests[1].body), {
        'channel': 'beta',
        'sha256':
            'b1c2d3e4f5061728394a5b6c7d8e9f0ab1c2d3e4f5061728394a5b6c7d8e9f0a',
      });
    });

    test(
      'updateDeviceFirmware maps old firmware to deviceUnsupported',
      () async {
        final recorder = Recorder([error(501, 'DEVICE_UNSUPPORTED')]);
        final exception = await failure(
          clientWith(recorder).updateDeviceFirmware(),
        );
        expect(exception.kind, GizClawControlErrorKind.deviceUnsupported);
      },
    );

    test('wifi reads', () async {
      final recorder = Recorder([
        json(200, {
          'connected': true,
          'ssid': 'Home',
          'rssi_dbm': -50,
          'ip': '192.0.2.10',
          'bssid': 'aa:bb:cc:dd:ee:ff',
        }),
        json(200, {
          'networks': [
            {'ssid': 'Home'},
            {'ssid': 'Office'},
          ],
        }),
      ]);
      final client = clientWith(recorder);
      final status = await client.getDeviceWifi();
      final saved = await client.listDeviceSavedWifi();
      expect(recorder.requests[0].url.path, '/gizclaw/v1/device/wifi');
      expect(recorder.requests[1].url.path, '/gizclaw/v1/device/wifi/saved');
      expect(status.connected, isTrue);
      expect(status.rssiDbm, -50);
      expect(saved.networks.map((n) => n.ssid), ['Home', 'Office']);
    });

    test('scanDeviceWifi posts timeout and decodes networks', () async {
      final recorder = Recorder([
        json(200, {
          'networks': [
            {
              'ssid': 'Office',
              'bssid': 'aa:bb:cc:dd:ee:ff',
              'rssi_dbm': -42,
              'frequency_mhz': 5180,
              'security': 'wpa3',
            },
          ],
        }),
      ]);
      final result = await clientWith(
        recorder,
      ).scanDeviceWifi(const DeviceWifiScanRequest(timeoutMs: 8000));
      expect(recorder.single.method, 'POST');
      expect(recorder.single.url.path, '/gizclaw/v1/device/wifi/scan');
      expect(jsonDecode(recorder.single.body), {'timeout_ms': 8000});
      expect(result.networks.single.ssid, 'Office');
      expect(result.networks.single.rssiDbm, -42);
      expect(result.networks.single.frequencyMhz, 5180);
      expect(result.networks.single.security, 'wpa3');
    });

    test('connectDeviceWifi puts credentials and accepts 202', () async {
      final recorder = Recorder([accepted(), accepted()]);
      final client = clientWith(recorder);
      await client.connectDeviceWifi(
        const DeviceWifiConnectRequest(
          ssid: 'Office',
          passphrase: 'correct-horse',
        ),
      );
      await client.connectDeviceWifi(
        const DeviceWifiConnectRequest(ssid: 'Open Network'),
      );
      expect(recorder.requests[0].method, 'PUT');
      expect(recorder.requests[0].url.path, '/gizclaw/v1/device/wifi');
      expect(jsonDecode(recorder.requests[0].body), {
        'ssid': 'Office',
        'passphrase': 'correct-horse',
      });
      expect(jsonDecode(recorder.requests[1].body), {'ssid': 'Open Network'});
    });

    test('forgetDeviceSavedWifi percent-encodes the SSID', () async {
      final recorder = Recorder([noContent()]);
      await clientWith(recorder).forgetDeviceSavedWifi('Café Wi-Fi/5G #2');
      expect(recorder.single.method, 'DELETE');
      expect(
        recorder.single.url.toString(),
        'https://ap.gizclaw.com/gizclaw/v1/device/wifi/saved/'
        'Caf%C3%A9%20Wi-Fi%2F5G%20%232',
      );
      expect(recorder.single.url.pathSegments.last, 'Café Wi-Fi/5G #2');
    });

    test('rejects an empty path parameter before sending', () {
      final recorder = Recorder([]);
      expect(
        () => clientWith(recorder).forgetDeviceSavedWifi(''),
        throwsArgumentError,
      );
      expect(recorder.requests, isEmpty);
    });
  });

  group('contacts', () {
    test('listContacts passes cursor and limit', () async {
      final recorder = Recorder([
        json(200, {
          'items': [contactJson],
          'has_next': true,
          'next_cursor': 'alice',
        }),
      ]);
      final list = await clientWith(
        recorder,
      ).listContacts(cursor: 'aaron', limit: 1);
      expect(recorder.single.url.path, '/gizclaw/v1/contacts');
      expect(recorder.single.url.queryParameters, {
        'cursor': 'aaron',
        'limit': '1',
      });
      expect(list.hasNext, isTrue);
      expect(list.nextCursor, 'alice');
      expect(list.items.single.displayName, 'Alice');
    });

    test('createContact posts and decodes 201', () async {
      final recorder = Recorder([json(201, contactJson)]);
      final contact = await clientWith(recorder).createContact(
        const ContactCreateRequest(name: 'alice', displayName: 'Alice'),
      );
      expect(recorder.single.method, 'POST');
      expect(jsonDecode(recorder.single.body), {
        'name': 'alice',
        'display_name': 'Alice',
      });
      expect(contact.phoneNumber, '+8613800000000');
      expect(contact.updatedAt, DateTime.utc(2026, 9, 3, 1, 2, 4));
    });

    test('named routes percent-encode the contact name', () async {
      final recorder = Recorder([
        json(200, contactJson),
        json(200, contactJson),
        noContent(),
      ]);
      final client = clientWith(recorder);
      await client.getContact('爱丽丝/1');
      await client.putContact(
        '爱丽丝/1',
        const ContactPutRequest(phoneNumber: '+8613900000000'),
      );
      await client.deleteContact('爱丽丝/1');
      const encoded = '/gizclaw/v1/contacts/%E7%88%B1%E4%B8%BD%E4%B8%9D%2F1';
      expect(recorder.requests[0].method, 'GET');
      expect(recorder.requests[0].url.toString(), endsWith(encoded));
      expect(recorder.requests[1].method, 'PUT');
      expect(recorder.requests[1].url.toString(), endsWith(encoded));
      expect(jsonDecode(recorder.requests[1].body), {
        'phone_number': '+8613900000000',
      });
      expect(recorder.requests[2].method, 'DELETE');
      expect(recorder.requests[2].url.toString(), endsWith(encoded));
    });
  });

  group('error mapping', () {
    final cases = <(int, String, GizClawControlErrorKind)>[
      (401, 'UNAUTHORIZED', GizClawControlErrorKind.unauthorized),
      (403, 'FORBIDDEN', GizClawControlErrorKind.forbidden),
      (404, 'NOT_FOUND', GizClawControlErrorKind.notFound),
      (409, 'DEVICE_OFFLINE', GizClawControlErrorKind.deviceOffline),
      (504, 'DEVICE_TIMEOUT', GizClawControlErrorKind.deviceTimeout),
      (400, 'DEVICE_REJECTED', GizClawControlErrorKind.deviceRejected),
      (501, 'DEVICE_UNSUPPORTED', GizClawControlErrorKind.deviceUnsupported),
      (502, 'DEVICE_ERROR', GizClawControlErrorKind.deviceError),
      (409, 'PENDING_DELETION', GizClawControlErrorKind.conflict),
      (400, 'INVALID_ARGUMENT', GizClawControlErrorKind.invalidRequest),
      (500, 'INTERNAL', GizClawControlErrorKind.server),
      (503, 'UNAVAILABLE', GizClawControlErrorKind.server),
      (418, 'TEAPOT', GizClawControlErrorKind.unexpectedStatus),
    ];
    for (final (status, code, kind) in cases) {
      test('$status $code -> ${kind.name}', () async {
        final recorder = Recorder([
          error(status, code, message: 'm', headers: {'x-request-id': 'req-1'}),
        ]);
        final exception = await failure(
          clientWith(recorder).setDeviceVolume(level: 1, muted: false),
        );
        expect(exception.kind, kind);
        expect(exception.statusCode, status);
        expect(exception.code, code);
        expect(exception.message, 'm');
        expect(exception.requestId, 'req-1');
        expect(exception.toString(), contains(kind.name));
      });
    }

    test('classifies on status alone when the body is not an error', () async {
      final recorder = Recorder([(_) => http.Response('gateway', 502)]);
      final exception = await failure(clientWith(recorder).getDevice());
      expect(exception.kind, GizClawControlErrorKind.server);
      expect(exception.statusCode, 502);
      expect(exception.code, isNull);
      expect(exception.requestId, isNull);
      expect(exception.message, 'HTTP 502');
    });

    test('keeps error details', () async {
      final recorder = Recorder([
        (_) => http.Response(
          jsonEncode({
            'error': {
              'code': 'DEVICE_REJECTED',
              'message': 'unknown sound',
              'details': {'sound': 'nope'},
            },
          }),
          400,
        ),
      ]);
      final exception = await failure(
        clientWith(recorder).playDeviceSound(sound: 'nope'),
      );
      expect(exception.kind, GizClawControlErrorKind.deviceRejected);
      expect(exception.details, {'sound': 'nope'});
    });

    test('maps transport failures to network', () async {
      final recorder = Recorder([
        (_) => throw http.ClientException('connection refused'),
      ]);
      final exception = await failure(clientWith(recorder).getDevice());
      expect(exception.kind, GizClawControlErrorKind.network);
      expect(exception.statusCode, isNull);
      expect(exception.cause, isA<http.ClientException>());
    });

    test('maps a timeout to network', () async {
      final client = GizClawControlClient(
        baseUrl: baseUrl,
        apiKey: apiKey,
        timeout: const Duration(milliseconds: 20),
        httpClient: MockClient((_) async {
          await Future<void>.delayed(const Duration(milliseconds: 200));
          return http.Response('{}', 200);
        }),
      );
      final exception = await failure(client.getDevice());
      expect(exception.kind, GizClawControlErrorKind.network);
      expect(exception.cause, isA<TimeoutException>());
    });

    test('maps a 2xx body that is not JSON to malformedResponse', () async {
      final recorder = Recorder([(_) => http.Response('<html>', 200)]);
      final exception = await failure(clientWith(recorder).getDevice());
      expect(exception.kind, GizClawControlErrorKind.malformedResponse);
      expect(exception.statusCode, 200);
    });

    test('maps a 2xx body with a wrong shape to malformedResponse', () async {
      final recorder = Recorder([
        json(200, {'online': 'yes', 'last_seen_at': 'never'}),
      ]);
      final exception = await failure(clientWith(recorder).getDeviceRuntime());
      expect(exception.kind, GizClawControlErrorKind.malformedResponse);
      expect(exception.message, contains('online'));
    });

    test('fails with network after close', () async {
      final recorder = Recorder([]);
      final client = clientWith(recorder)..close();
      final exception = await failure(client.getDevice());
      expect(exception.kind, GizClawControlErrorKind.network);
      expect(recorder.requests, isEmpty);
    });
  });

  group('send', () {
    test('reaches an unmodeled route and returns the status', () async {
      final recorder = Recorder([
        json(200, {'anything': true}),
      ]);
      final response = await clientWith(
        recorder,
      ).send(method: 'GET', path: '/gizclaw/v1/device/future?limit=5');
      expect(recorder.single.method, 'GET');
      expect(recorder.single.headers['Authorization'], 'Bearer $apiKey');
      expect(
        recorder.single.url.toString(),
        'https://ap.gizclaw.com/gizclaw/v1/device/future?limit=5',
      );
      expect(response.statusCode, 200);
      expect(response.isSuccess, isTrue);
      expect(response.json, {'anything': true});
    });

    test('returns a non-2xx status instead of throwing', () async {
      final recorder = Recorder([
        error(409, 'DEVICE_OFFLINE', headers: {'x-request-id': 'req-9'}),
      ]);
      final response = await clientWith(recorder).send(
        method: 'PUT',
        path: '/gizclaw/v1/device/volume',
        body: {'level': 1},
      );
      expect(response.statusCode, 409);
      expect(response.isSuccess, isFalse);
      expect(response.requestId, 'req-9');
      expect(
        classifyGizClawControlError(409, 'DEVICE_OFFLINE'),
        GizClawControlErrorKind.deviceOffline,
      );
      expect(jsonDecode(recorder.single.body), {'level': 1});
    });

    test(
      'returns null for an empty body and rejects a relative path',
      () async {
        final recorder = Recorder([noContent()]);
        final client = clientWith(recorder);
        final response = await client.send(
          method: 'DELETE',
          path: '/gizclaw/v1/contacts/alice',
        );
        expect(response.statusCode, 204);
        expect(response.json, isNull);
        expect(
          () => client.send(method: 'GET', path: 'relative'),
          throwsArgumentError,
        );
      },
    );
  });

  group('classifyGizClawControlError', () {
    test('device codes win over status', () {
      expect(
        classifyGizClawControlError(409, 'DEVICE_OFFLINE'),
        GizClawControlErrorKind.deviceOffline,
      );
      expect(
        classifyGizClawControlError(409, null),
        GizClawControlErrorKind.conflict,
      );
      expect(
        classifyGizClawControlError(504, 'UPSTREAM'),
        GizClawControlErrorKind.server,
      );
    });
  });

  group('models', () {
    test('ignore unknown keys and round-trip toJson', () {
      final key = ApiKey.fromJson({...apiKeyJson, 'unknown': true});
      expect(key.toJson(), apiKeyJson);
      final contact = Contact.fromJson({...contactJson, 'extra': 1});
      expect(contact.toJson(), contactJson);
      final wifi = DeviceWifiStatus.fromJson({'connected': false, 'x': 1});
      expect(wifi.toJson(), {'connected': false});
    });

    test('PeerStatus with no fields decodes to nulls', () {
      final status = PeerStatus.fromJson(<String, Object?>{});
      expect(status.volume, isNull);
      expect(status.labels, isEmpty);
      expect(status.details, isEmpty);
      expect(status.toJson(), isEmpty);
    });

    test('integers arrive as doubles from some encoders', () {
      final point = PeerTelemetryPoint.fromJson({
        'observed_at_unix_ms': 1000.0,
        'value': 3,
      });
      expect(point.observedAtUnixMs, 1000);
      expect(point.value, 3.0);
    });

    test('PeerTelemetryField lists the contract enumeration', () {
      expect(PeerTelemetryField.values, hasLength(13));
      expect(PeerTelemetryField.values, contains('system.temperature_c'));
    });
  });
}
