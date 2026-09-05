import 'package:gizclaw/src/generated/rpc/payload/system.pb.dart';
import 'package:test/test.dart';

void main() {
  test('OTA status preserves zero progress and unknown future states', () {
    for (final state in ['downloading', 'future-state']) {
      final status = PeerStatus(
        ota: PeerOtaStatus(
          state: state,
          updateId: 'one',
          observedAt: '2026-09-06T00:00:00Z',
          downloadPercent: 0,
          targetVersion: '2.0',
        ),
      );
      final decoded = PeerStatus.fromBuffer(status.writeToBuffer());
      expect(decoded.ota.state, state);
      expect(decoded.ota.updateId, 'one');
      expect(decoded.ota.hasDownloadPercent(), isTrue);
      expect(decoded.ota.downloadPercent, 0);
      expect(decoded.ota.targetVersion, '2.0');
    }
  });
}
