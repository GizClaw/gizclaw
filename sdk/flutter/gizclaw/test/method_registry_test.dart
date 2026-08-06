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
    expect(
      () => rpcMethodByName('server.firmware.download'),
      throwsArgumentError,
    );
  });

  test('round-trips every Firmware channel and response field', () {
    final channels = [
      FirmwareChannelName.FIRMWARE_CHANNEL_NAME_STABLE,
      FirmwareChannelName.FIRMWARE_CHANNEL_NAME_BETA,
      FirmwareChannelName.FIRMWARE_CHANNEL_NAME_DEVELOP,
      FirmwareChannelName.FIRMWARE_CHANNEL_NAME_PENDING,
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
      channel: FirmwareChannelName.FIRMWARE_CHANNEL_NAME_PENDING,
      description: 'candidate package',
      url: 'https://firmware.example.invalid/devkit/pending.tar.zlib',
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
