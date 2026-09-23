import 'package:flutter_test/flutter_test.dart';
import 'package:giztest/src/client.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:protobuf/well_known_types/google/protobuf/struct.pb.dart';

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

  test('unknown numeric safety fence levels reach the protobuf wire', () {
    for (final level in [-2147483648, -1, 99, 2147483647]) {
      final request = scenarioRequest('server.workspace.parameters.set', {
        'name': 'x',
        'parameters': {'safety_fence_level': level},
      });
      final wire = request.writeToBuffer();
      expect(
        (request.createEmptyInstance()..mergeFromBuffer(wire)).writeToBuffer(),
        wire,
      );
      if (level == 99) {
        expect(wire, [0x0a, 0x01, 0x78, 0x12, 0x02, 0x20, 0x63]);
      }
    }
    for (final method in [
      'server.run.workspace.reload-with-options',
      'server.workspace.create',
    ]) {
      final parameters = method == 'server.workspace.create'
          ? <String, Object?>{
              'value': {
                'name': 'x',
                'parameters': {
                  'flowcraft_workspace_parameters': {'safety_fence_level': 99},
                },
              },
            }
          : <String, Object?>{
              'workspace_name': 'x',
              'parameters': {'safety_fence_level': 99},
            };
      expect(
        scenarioRequest(method, parameters).writeToBuffer(),
        contains(0x63),
      );
    }
    for (final value in ['', 'SAFETY_FENCE_LEVEL_UNKNOWN', 2147483648]) {
      expect(
        () => scenarioRequest('server.workspace.parameters.set', {
          'parameters': {'safety_fence_level': value},
        }),
        throwsFormatException,
      );
    }
    expect(
      scenarioRequest('server.workspace.parameters.set', {
        'name': 'x',
      }).writeToBuffer(),
      [0x0a, 0x01, 0x78],
    );
  });

  test('tool registry encodes find, while scenarios encode server methods', () {
    expect(
      newPayloadMessage(clientToolByName('device.find').requestType)
        ..mergeFromProto3Json({'durationMs': 8000}),
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
      {
        'result': 'SOCIAL_PING_RESULT_RATE_LIMITED',
        'delivered_count': 0,
        'retry_after_seconds': 42,
      },
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

  test('responses emit implicit-presence defaults like Go and JavaScript', () {
    // Go protojson EmitUnpopulated and the JavaScript alwaysEmitImplicit
    // projection both render unset scalars, enums, repeated fields and maps.
    // proto3 optional, oneof and message fields stay absent.
    expect(camelToSnakeKeys(unwrapValueMessage(AppConfigListResponse())), {
      'keys': <Object?>[],
      'has_next': false,
      'runtime_profile_name': '',
      'runtime_profile_revision': '',
    });
    expect(
      camelToSnakeKeys(
        unwrapValueMessage(
          WorkspaceHistoryListResponse(value: PeerRunHistoryListResponse()),
        ),
      ),
      {'available': false, 'has_next': false, 'items': <Object?>[]},
    );
    expect(camelToSnakeKeys(unwrapValueMessage(FriendPingResponse())), {
      'result': 'SOCIAL_PING_RESULT_UNSPECIFIED',
      'delivered_count': 0,
    });
    expect(
      camelToSnakeKeys(
        unwrapValueMessage(WorkspaceHistoryAudioDownloadResponse()),
      ),
      {
        'history_name': '',
        'mime_type': '',
        'size_bytes': '0',
        'workspace_name': '',
      },
    );
    expect(
      camelToSnakeKeys(
        unwrapValueMessage(
          ProfileGetResponse(items: [PublicProfile(emoji: '🐱')]),
        ),
      ),
      {
        'items': [
          {'peer_public_key': '', 'emoji': '🐱'},
        ],
      },
    );
  });

  test('responses emit empty maps and project message map values', () {
    // An unset map renders as {}; each message value gets its own implicit
    // defaults while its proto3 optional description stays absent. Keys are
    // paired with values without re-encoding. The unset provider_data oneof
    // stays absent.
    expect(camelToSnakeKeys(unwrapValueMessage(Model())), {
      'name': '',
      'i18n': <String, Object?>{},
      'kind': 'MODEL_KIND_UNSPECIFIED',
      'provider_kind': 'MODEL_PROVIDER_KIND_UNSPECIFIED',
    });
    expect(
      camelToSnakeKeys(
        unwrapValueMessage(
          Model(
            name: 'gpt',
            i18n: {
              'en': ResourceI18nText(),
              'zh': ResourceI18nText(displayName: '模型', description: 'd'),
            }.entries,
          ),
        ),
      ),
      {
        'name': 'gpt',
        'i18n': {
          'en': {'display_name': ''},
          'zh': {'display_name': '模型', 'description': 'd'},
        },
        'kind': 'MODEL_KIND_UNSPECIFIED',
        'provider_kind': 'MODEL_PROVIDER_KIND_UNSPECIFIED',
      },
    );
  });

  test('responses keep well-known type JSON shapes', () {
    // google.protobuf.Struct renders as a plain JSON object; its internal
    // fields map is not filled in as if it were a payload message.
    expect(
      camelToSnakeKeys(
        unwrapValueMessage(
          FlowcraftWorkflowSpec(
            fields: Struct(
              fields: {'mode': Value(stringValue: 'idle')}.entries,
            ),
          ),
        ),
      ),
      {
        'fields': {'mode': 'idle'},
      },
    );
  });
}
