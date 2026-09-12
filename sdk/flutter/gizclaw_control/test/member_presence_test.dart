import 'package:gizclaw_control/gizclaw_control.dart';
import 'package:test/test.dart';

void main() {
  test('member list preserves online false and optional last seen', () {
    final base = {'name': 'peer', 'peer_public_key': 'peer', 'role': 'member'};
    final online = FriendGroupMember.fromJson({
      ...base,
      'online': true,
      'last_seen_at': '2026-09-12T00:30:00Z',
    });
    expect(online.online, isTrue);
    expect(online.lastSeenAt, DateTime.utc(2026, 9, 12, 0, 30));
    final offline = FriendGroupMember.fromJson({...base, 'online': false});
    expect(offline.online, isFalse);
    expect(offline.lastSeenAt, isNull);
    final added = FriendGroupMember.fromJson(base);
    expect(added.online, isNull);
    expect(added.lastSeenAt, isNull);
    expect(
      () => FriendGroupMember.fromJson({...base, 'online': 'false'}),
      throwsFormatException,
    );
    expect(
      () => FriendGroupMember.fromJson({...base, 'last_seen_at': 42}),
      throwsFormatException,
    );
  });
}
