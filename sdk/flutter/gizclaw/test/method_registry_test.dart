import 'package:fixnum/fixnum.dart';
import 'package:gizclaw/gizclaw.dart';
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
    expect(rpcMethodByName('client.device.status.get').id, 99);
    expect(rpcMethodByName('client.device.volume.set').id, 100);
    expect(rpcMethodByName('client.device.sound.play').id, 101);
    expect(rpcMethodByName('client.device.reboot').id, 102);
    expect(rpcMethodByName('client.wifi.status.get').id, 103);
    expect(rpcMethodByName('client.wifi.saved.list').id, 104);
    expect(rpcMethodByName('client.wifi.saved.forget').id, 105);
    expect(
      () => rpcMethodByName('server.firmware.download'),
      throwsArgumentError,
    );
  });

  test(
    'maps device control methods to their request and response messages',
    () {
      expect(
        rpcMethodByName('client.device.volume.set').requestType,
        'ClientDeviceVolumeSetRequest',
      );
      expect(
        rpcMethodByName('client.device.volume.set').responseType,
        'ClientDeviceVolumeSetResponse',
      );
      expect(
        rpcMethodByName('client.wifi.saved.list').responseType,
        'ClientWifiSavedListResponse',
      );

      final volume = ClientDeviceVolumeSetRequest(
        level: Int64(35),
        muted: true,
      );
      final decodedVolume =
          decodeRpcRequestPayload(
                'client.device.volume.set',
                encodeRpcRequestPayload('client.device.volume.set', volume),
              )
              as ClientDeviceVolumeSetRequest;
      expect(decodedVolume.level, Int64(35));
      expect(decodedVolume.muted, isTrue);

      final status = ClientDeviceVolumeSetResponse(
        value: PeerStatus(
          volume: Int64(35),
          muted: true,
          batteryPercent: Int64(80),
        ),
      );
      final decodedStatus =
          decodeRpcResponsePayload(
                'client.device.volume.set',
                encodeRpcResponsePayload('client.device.volume.set', status),
              )
              as ClientDeviceVolumeSetResponse;
      expect(decodedStatus.value.volume, Int64(35));
      expect(decodedStatus.value.muted, isTrue);
      expect(decodedStatus.value.batteryPercent, Int64(80));

      final sound = ClientDeviceSoundPlayRequest(
        sound: 'chime',
        durationMs: Int64(1500),
      );
      final decodedSound =
          decodeRpcRequestPayload(
                'client.device.sound.play',
                encodeRpcRequestPayload('client.device.sound.play', sound),
              )
              as ClientDeviceSoundPlayRequest;
      expect(decodedSound.sound, 'chime');
      expect(decodedSound.durationMs, Int64(1500));

      final wifi = ClientWifiStatusGetResponse(
        value: WifiStatus(
          connected: true,
          ssid: 'home',
          rssiDbm: Int64(-55),
          ip: '192.0.2.10',
        ),
      );
      final decodedWifi =
          decodeRpcResponsePayload(
                'client.wifi.status.get',
                encodeRpcResponsePayload('client.wifi.status.get', wifi),
              )
              as ClientWifiStatusGetResponse;
      expect(decodedWifi.value.connected, isTrue);
      expect(decodedWifi.value.ssid, 'home');
      expect(decodedWifi.value.rssiDbm, Int64(-55));

      final saved = ClientWifiSavedListResponse(
        networks: [
          WifiSavedNetwork(ssid: 'home'),
          WifiSavedNetwork(ssid: 'office'),
        ],
      );
      final decodedSaved =
          decodeRpcResponsePayload(
                'client.wifi.saved.list',
                encodeRpcResponsePayload('client.wifi.saved.list', saved),
              )
              as ClientWifiSavedListResponse;
      expect(decodedSaved.networks.map((n) => n.ssid), ['home', 'office']);

      final forget = ClientWifiSavedForgetRequest(ssid: 'office');
      final decodedForget =
          decodeRpcRequestPayload(
                'client.wifi.saved.forget',
                encodeRpcRequestPayload('client.wifi.saved.forget', forget),
              )
              as ClientWifiSavedForgetRequest;
      expect(decodedForget.ssid, 'office');
    },
  );

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
}
