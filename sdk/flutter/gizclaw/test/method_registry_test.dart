import 'package:fixnum/fixnum.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:protobuf/protobuf.dart';
import 'package:test/test.dart';

void main() {
  test('contains canonical workflow and workspace RPC method IDs', () {
    expect(rpcMethodByName('server.firmware.get').id, 22);
    expect(rpcMethodByName('server.workflow.list').id, 32);
    expect(rpcMethodByName('server.workspace.list').id, 24);
    expect(rpcMethodByName('server.workspace.get').id, 25);
    expect(rpcMethodByName('server.run.say').id, 21);
    expect(rpcMethodByName('all.ping').id, 1);
    expect(rpcMethodByName('server.route.resolve').id, 85);
    expect(rpcMethodByName('server.speech.extract').id, 94);
    expect(rpcMethodByName('server.api_key.create').id, 96);
    expect(rpcMethodByName('server.api_key.list').id, 97);
    expect(rpcMethodByName('server.api_key.revoke').id, 98);
    expect(rpcMethodByName('server.api_key.resolve').id, 99);
    expect(rpcMethodByName('client.mhs.v0.read').id, 133);
    expect(rpcMethodByName('client.mhs.v0.write').id, 134);
    expect(rpcMethodByName('client.tool.v0.invoke').id, 135);
    expect(rpcMethodByName('client.tool.v0.list').id, 136);
    expect(rpcMethodByName('client.rpc.methods.list').id, 137);
    expect(clientToolByName('device.find').id, 6);
    expect(clientToolByName('social.ping').id, 21);
    expect(
      () => rpcMethodByName('server.firmware.download'),
      throwsArgumentError,
    );
  });

  test('tool/v0 registry selects typed request and response payloads', () {
    expect(
      clientToolByName('sound.play').requestType,
      'ClientDeviceSoundPlayRequest',
    );
    expect(
      clientToolByName('wifi.saved.list').responseType,
      'ClientWifiSavedListResponse',
    );
    final sound = ClientDeviceSoundPlayRequest(
      sound: 'chime',
      durationMs: Int64(1500),
    );
    final decodedSound =
        decodeClientToolRequestPayload(
              clientToolByName('sound.play').id,
              encodeClientToolRequestPayload(
                clientToolByName('sound.play').id,
                sound,
              ),
            )
            as ClientDeviceSoundPlayRequest;
    expect(decodedSound.sound, 'chime');
    expect(decodedSound.durationMs, Int64(1500));
    final saved = ClientWifiSavedListResponse(
      networks: [WifiSavedNetwork(ssid: 'home')],
    );
    final decodedSaved =
        decodeClientToolResponsePayload(
              clientToolByName('wifi.saved.list').id,
              encodeClientToolResponsePayload(
                clientToolByName('wifi.saved.list').id,
                saved,
              ),
            )
            as ClientWifiSavedListResponse;
    expect(decodedSaved.networks.single.ssid, 'home');
  });

  test('round-trips find, social ping and public profile payloads', () {
    final find =
        decodeClientToolRequestPayload(
              clientToolByName('device.find').id,
              encodeClientToolRequestPayload(
                clientToolByName('device.find').id,
                ClientDeviceFindRequest(durationMs: Int64(8000)),
              ),
            )
            as ClientDeviceFindRequest;
    expect(find.durationMs, Int64(8000));
    final findDefault =
        decodeClientToolRequestPayload(
              clientToolByName('device.find').id,
              const [],
            )
            as ClientDeviceFindRequest;
    expect(findDefault.hasDurationMs(), isFalse);
    final ping =
        decodeClientToolRequestPayload(
              clientToolByName('social.ping').id,
              encodeClientToolRequestPayload(
                clientToolByName('social.ping').id,
                ClientSocialPingRequest(
                  fromPeerPublicKey: 'peer-a',
                  fromDisplayName: 'Alice',
                  friendGroupName: 'my-team',
                ),
              ),
            )
            as ClientSocialPingRequest;
    expect(ping.fromPeerPublicKey, 'peer-a');
    expect(ping.friendGroupName, 'my-team');
    final pinged =
        decodeRpcResponsePayload(
              'server.friend.ping',
              encodeRpcResponsePayload(
                'server.friend.ping',
                FriendPingResponse(
                  result: SocialPingResult.SOCIAL_PING_RESULT_RATE_LIMITED,
                  retryAfterSeconds: 30,
                ),
              ),
            )
            as FriendPingResponse;
    expect(pinged.retryAfterSeconds, 30);
    final profiles =
        decodeRpcResponsePayload(
              'server.profile.get',
              encodeRpcResponsePayload(
                'server.profile.get',
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
            )
            as ProfileGetResponse;
    expect(profiles.items.single.displayName, 'Carol');
  });

  test('round-trips Edge API key route payloads', () {
    final request = ServerAPIKeyResolveRequest(apiKey: 'gzk_test');
    final decoded =
        decodeRpcRequestPayload(
              'server.api_key.resolve',
              encodeRpcRequestPayload('server.api_key.resolve', request),
            )
            as ServerAPIKeyResolveRequest;
    expect(decoded.apiKey, 'gzk_test');
  });

  test('round-trips API key root management payloads', () {
    final list = APIKeyListRequest(cursor: 'key_cursor', limit: Int64(25));
    final decodedList =
        decodeRpcRequestPayload(
              'server.api_key.list',
              encodeRpcRequestPayload('server.api_key.list', list),
            )
            as APIKeyListRequest;
    expect(decodedList.cursor, 'key_cursor');
    expect(decodedList.limit, Int64(25));

    final revoke = APIKeyRevokeRequest(name: 'key_name');
    final decodedRevoke =
        decodeRpcRequestPayload(
              'server.api_key.revoke',
              encodeRpcRequestPayload('server.api_key.revoke', revoke),
            )
            as APIKeyRevokeRequest;
    expect(decodedRevoke.name, 'key_name');
  });

  test('round-trips every Firmware channel and response field', () {
    final channels = [
      FirmwareChannelName.FIRMWARE_CHANNEL_NAME_STABLE,
      FirmwareChannelName.FIRMWARE_CHANNEL_NAME_BETA,
      FirmwareChannelName.FIRMWARE_CHANNEL_NAME_DEVELOP,
    ];
    for (final channel in channels) {
      final request = FirmwareGetRequest(channel: channel);
      final decoded =
          decodeRpcRequestPayload(
                'server.firmware.get',
                encodeRpcRequestPayload('server.firmware.get', request),
              )
              as FirmwareGetRequest;
      expect(decoded.channel, channel);
    }

    final response = FirmwareGetResponse(
      version: '1.5.0-beta.1+abc123',
      channel: FirmwareChannelName.FIRMWARE_CHANNEL_NAME_STABLE,
      description: 'stable package',
      url: 'https://firmware.example.invalid/devkit/stable.tar.zlib',
      sha256:
          '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
      size: Int64(9007199254740991),
    );
    final decoded =
        decodeRpcResponsePayload(
              'server.firmware.get',
              encodeRpcResponsePayload('server.firmware.get', response),
            )
            as FirmwareGetResponse;
    expect(decoded.channel, response.channel);
    expect(decoded.description, response.description);
    expect(decoded.url, response.url);
    expect(decoded.sha256, response.sha256);
    expect(decoded.size, response.size);
    expect(decoded.version, response.version);
    expect(decoded.hasVersion(), isTrue);

    final withoutDescription = FirmwareGetResponse(
      channel: FirmwareChannelName.FIRMWARE_CHANNEL_NAME_DEVELOP,
      url: 'https://firmware.example.invalid/devkit/develop.tar.zlib',
      sha256:
          'abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789',
      size: Int64(4096),
    );
    final decodedWithoutDescription =
        decodeRpcResponsePayload(
              'server.firmware.get',
              encodeRpcResponsePayload(
                'server.firmware.get',
                withoutDescription,
              ),
            )
            as FirmwareGetResponse;
    expect(decodedWithoutDescription.hasDescription(), isFalse);
    expect(decodedWithoutDescription.hasVersion(), isFalse);
  });

  test('encodes and decodes structured speech extraction payloads', () {
    final request = SpeechExtractRequest(
      asrModelName: 'asr-main',
      extractModelName: 'extract-main',
      contentType: 'audio/L16;rate=16000;channels=1',
      schemaJson: '{"type":"object"}',
    );
    final encoded = encodeRpcRequestPayload('server.speech.extract', request);
    final decoded =
        decodeRpcRequestPayload('server.speech.extract', encoded)
            as SpeechExtractRequest;

    expect(decoded.asrModelName, 'asr-main');
    expect(decoded.extractModelName, 'extract-main');
    expect(decoded.schemaJson, '{"type":"object"}');
  });

  test('encodes and decodes typed payloads by method metadata', () {
    final request = WorkspaceGetRequest(name: 'demo-workspace');
    final encoded = encodeRpcRequestPayload('server.workspace.get', request);
    final decoded =
        decodeRpcRequestPayload('server.workspace.get', encoded)
            as WorkspaceGetRequest;

    expect(decoded.name, 'demo-workspace');
  });

  test('rejects mismatched payload type', () {
    expect(
      () => encodeRpcRequestPayload(
        'server.workspace.get',
        WorkflowListRequest(),
      ),
      throwsArgumentError,
    );
  });

  test('exports generated enum payload types from public barrel', () {
    expect(ASTTranslateMode.ASTTRANSLATE_MODE_S2S.value, 2);
  });

  test('registers MHS and tool/v0 RPC method IDs', () {
    expect(rpcMethodByName('client.mhs.v0.read').id, 133);
    expect(rpcMethodByName('client.mhs.v0.write').id, 134);
    expect(rpcMethodByName('client.tool.v0.invoke').id, 135);
    expect(rpcMethodByName('client.tool.v0.list').id, 136);
    expect(rpcMethodByName('client.rpc.methods.list').id, 137);
  });

  // Presence of optional procedure and status fields must survive encoding.
  test('treats procedure and status observation fields as optional', () {
    void expectOptional(GeneratedMessage message, Iterable<int> tags) {
      for (final tag in tags) {
        expect(
          payloadFieldIsProto3Optional(message, tag),
          isTrue,
          reason: '${message.info_.qualifiedMessageName} field $tag',
        );
      }
    }

    expectOptional(ClientDeviceFactoryResetRequest(), [1]);
    expectOptional(PeerStatus(), [17, 18, 19, 20]);
    expectOptional(PeerStatusTelemetryObservedAt(), [
      1,
      2,
      3,
      4,
      5,
      6,
      7,
      8,
      9,
      10,
    ]);
  });
}
