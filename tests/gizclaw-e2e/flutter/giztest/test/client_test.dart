import 'package:flutter_test/flutter_test.dart';
import 'package:giztest/src/client.dart';
import 'package:gizclaw/gizclaw.dart';

void main() {
  test('scenario requests wrap single-value messages exactly once', () {
    const value = <String, Object?>{'name': 'example'};
    final implicit = scenarioRequest('server.workspace.create', value);
    final explicit = scenarioRequest('server.workspace.create', {
      'value': value,
    });
    expect(implicit.writeToBuffer(), explicit.writeToBuffer());
    expect(implicit.toProto3Json(), {'value': value});
  });

  test('scenario requests keep ordinary messages unwrapped', () {
    final request = scenarioRequest('server.workspace.get', {
      'name': 'example',
    });
    expect(request.toProto3Json(), {'name': 'example'});
  });

  test('wrapped scenario requests still reject unknown fields', () {
    expect(
      () => scenarioRequest('server.workspace.create', {'unknown': true}),
      throwsFormatException,
    );
  });

  test('scenario requests encode find, social ping and profile methods', () {
    expect(
      scenarioRequest('client.device.find', {'duration_ms': 8000}),
      isA<ClientDeviceFindRequest>(),
    );
    expect(
      scenarioRequest('server.friend.ping', {
        'name': 'friend-a',
      }).toProto3Json(),
      {'name': 'friend-a'},
    );
    expect(
      scenarioRequest('server.friend_group.ping', {
        'name': 'my-team',
      }).toProto3Json(),
      {'name': 'my-team'},
    );
    expect(
      scenarioRequest('server.profile.get', {
        'peer_public_keys': ['peer-a', 'peer-b'],
      }).toProto3Json(),
      {
        'peerPublicKeys': ['peer-a', 'peer-b'],
      },
    );
  });

  test('ping and profile responses project to snake_case proto JSON', () {
    // Enum values render by their proto name, the form Go protojson and the
    // JavaScript runner's toJson emit.
    expect(
      camelToSnakeKeys(
        unwrapValueMessage(
          FriendPingResponse(
            result: SocialPingResult.SOCIAL_PING_RESULT_DELIVERED,
            deliveredCount: 1,
          ),
        ),
      ),
      {'result': 'SOCIAL_PING_RESULT_DELIVERED', 'delivered_count': 1},
    );
    expect(
      camelToSnakeKeys(
        unwrapValueMessage(
          FriendGroupPingResponse(
            result: SocialPingResult.SOCIAL_PING_RESULT_RATE_LIMITED,
            retryAfterSeconds: 42,
          ),
        ),
      ),
      {'result': 'SOCIAL_PING_RESULT_RATE_LIMITED', 'retry_after_seconds': 42},
    );
    expect(
      camelToSnakeKeys(
        unwrapValueMessage(
          ProfileGetResponse(
            items: [
              PublicProfile(
                peerPublicKey: 'peer-a',
                displayName: 'Carol',
                emoji: '🐱',
              ),
            ],
          ),
        ),
      ),
      {
        'items': [
          {'peer_public_key': 'peer-a', 'display_name': 'Carol', 'emoji': '🐱'},
        ],
      },
    );
  });
}
