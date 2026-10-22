import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw/src/management.dart';
import 'package:test/test.dart';

void main() {
  test('KV query preserves nested JSON and null false zero presence', () {
    for (final json in ['null', 'false', '0', '{"a":[null,false,0]}']) {
      final status = PeerStatus(
        reportedKv: <String, PeerKVEntry>{
          'key': PeerKVEntry(
            jsonValue: json,
            observedAt: '2026-10-22T00:00:00Z',
          ),
        }.entries,
      );
      final value = PeerStatus.fromBuffer(status.writeToBuffer());
      expect(value.reportedKv.containsKey('key'), isTrue);
      expect(value.reportedKv['key']!.jsonValue, json);
      if (json == 'null') expect(value.reportedKv['key']!.value, isNull);
      if (json == 'false') expect(value.reportedKv['key']!.value, isFalse);
      if (json == '0') expect(value.reportedKv['key']!.value, 0);
    }
  });
  test('Modem unknown power differs from a measured false', () {
    final unknown = ModemHwdReadResponse.fromBuffer(
      ModemHwdReadResponse(enabled: false, registered: false).writeToBuffer(),
    );
    expect(unknown.hasEnabled(), isTrue);
    expect(unknown.enabled, isFalse);
    expect(unknown.hasPowered(), isFalse);
    final measured = ModemHwdReadResponse.fromBuffer(
      ModemHwdReadResponse(powered: false).writeToBuffer(),
    );
    expect(measured.hasPowered(), isTrue);
    expect(measured.powered, isFalse);
  });
  test('management results require effective typed values', () {
    final request = ClientUsageTimeSetRequest(limitSeconds: 9000);
    expect(
      validManagementResult(
        request,
        ClientUsageTimeSetResponse(
          applied: UsageTimeState(
            limitSeconds: 3600,
            usedSeconds: 0,
            exhausted: false,
          ),
        ),
      ),
      isTrue,
    );
    expect(
      validManagementResult(request, ClientUsageTimeSetResponse()),
      isFalse,
    );
    expect(
      validManagementResult(
        ClientManagementPasswordResetRequest(),
        ClientManagementPasswordResetResponse(isSet: false),
      ),
      isTrue,
    );
    expect(
      validManagementResult(
        ClientManagementPasswordResetRequest(),
        ClientManagementPasswordResetResponse(),
      ),
      isFalse,
    );
    expect(clientToolByName('management.password.reset').id, 38);
  });
}
