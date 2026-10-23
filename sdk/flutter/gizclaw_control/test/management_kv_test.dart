import 'dart:convert';
import 'package:gizclaw_control/gizclaw_control.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:test/test.dart';

void main() {
  test(
    'runtime and status retain JSON null false zero and observation times',
    () {
      final reported = {
        'null': {
          'value': null,
          'json_value': 'null',
          'observed_at': '2026-10-22T00:00:00Z',
        },
        'false': {
          'value': false,
          'json_value': 'false',
          'observed_at': '2026-10-22T00:00:00Z',
        },
        'zero': {
          'value': 0,
          'json_value': '0',
          'observed_at': '2026-10-22T00:00:00Z',
        },
      };
      final runtime = DeviceRuntime.fromJson({
        'online': false,
        'last_seen_at': '2026-10-22T00:00:00Z',
        'reported_kv': reported,
      });
      final status = PeerStatus.fromJson({'reported_kv': reported});
      expect(runtime.reportedKv!['null']!.value, isNull);
      expect(status.reportedKv!['false']!.value, isFalse);
      expect(runtime.reportedKv!['zero']!.value, 0);
      expect(runtime.toJson()['reported_kv'], reported);
    },
  );
  test(
    'NFC and Modem models keep missing observations and binary snapshots',
    () {
      final modem = MhsModemReadValue.fromJson({
        'enabled': false,
        'registered': false,
      });
      expect(modem.enabled, isFalse);
      expect(modem.powered, isNull);
      final nfc =
          MhsHwdReadResult.fromJson({
                'id': 'nfc.left',
                'hwd': 'nfc',
                'value': {
                  'enabled': false,
                  'card_present': false,
                  'last_read': {
                    'read_at_unix_ms': '1000',
                    'uid': 'AAE=',
                    'raw_data': 'AP8=',
                    'result': 'failed',
                    'error_code': 'READ_FAILED',
                  },
                },
              }).value
              as MhsNfcReadValue;
      expect(nfc.cardPresent, isFalse);
      expect(nfc.lastRead!.uid, [0, 1]);
      expect(nfc.lastRead!.rawData, [0, 255]);
      expect(nfc.lastRead!.result, 'failed');
      expect(const MhsNfcWriteValue(enabled: false).toJson(), {
        'enabled': false,
      });
    },
  );
  test('management methods use invoke and return applied values', () async {
    final seen = <String>[];
    final client = GizClawControlClient(
      baseUrl: Uri.parse('https://edge.example'),
      apiKey: 'fixture',
      httpClient: MockClient((request) async {
        expect(request.url.path, '/gizclaw/v1/device/tool/v0/invoke');
        final body = jsonDecode(request.body) as Map<String, Object?>;
        seen.add(body['tool'] as String);
        final result = body['tool'] == 'usage.time.set'
            ? {
                'applied': {
                  'limit_seconds': 3600,
                  'used_seconds': 0,
                  'exhausted': false,
                },
              }
            : {'is_set': false};
        return http.Response(
          jsonEncode({'result': result}),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    expect((await client.setUsageTime(9000)).limitSeconds, 3600);
    expect(await client.resetManagementPassword(), isFalse);
    expect(seen, ['usage.time.set', 'management.password.reset']);
  });
}
