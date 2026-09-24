import 'dart:typed_data';

import 'package:fixnum/fixnum.dart' as fixnum;
import 'package:fixnum/fixnum.dart' show Int64;
import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
import 'package:gizclaw/gizclaw.dart';
import 'package:protobuf/protobuf.dart' show GeneratedMessage;
import 'package:test/test.dart';

import 'fake_transport.dart';

void main() {
  deviceControlTests();
  mhsTests();
  test(
    'audioplayer preserves explicit zero index and rejects missing index',
    () async {
      var calls = 0;
      final handlers = GizClawPeerRpcHandlers(
        deviceInfo: () => DeviceInfo(name: 'player'),
        deviceControl: GizClawDeviceControlHandlers(
          audioplayer: GizClawAudioPlayerHandlers(
            play: (request) {
              calls++;
              expect(request.index, 0);
              return ClientDeviceAudioPlayerPlayResponse(
                value: AudioPlayerStatus(
                  state: 'buffering',
                  currentIndex: 0,
                  repeat: 'off',
                  playlistLength: 1,
                ),
              );
            },
          ),
        ),
      );
      for (final index in <int?>[0, null]) {
        final channel = FakeDataChannel('giznet/v1/service/0');
        addTearDown(channel.close);
        serveGizClawPeerRpcChannel(channel, handlers: handlers);
        final response = await _callInbound(
          channel,
          id: 'player',
          method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
          methodName: 'audioplayer.play',
          request: ClientDeviceAudioPlayerPlayRequest(index: index),
        );
        if (index == null) {
          expect(
            response.status.code,
            rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
          );
        } else {
          expect(response.hasStatus(), isFalse);
          final result =
              decodeClientToolResponsePayload(
                    clientToolByName('audioplayer.play').id,
                    response.payload,
                  )
                  as ClientDeviceAudioPlayerPlayResponse;
          expect(result.value.hasCurrentIndex(), isTrue);
          expect(result.value.currentIndex, 0);
        }
      }
      expect(calls, 1);
    },
  );

  test('serves server-initiated all.ping requests', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    serveGizClawPeerRpcChannel(channel);

    channel.addMessage(
      _rpcRequestBytes(
        id: 'srv-ping',
        method: rpc.RpcMethod.RPC_METHOD_ALL_PING,
        payloadBytes: encodeRpcRequestPayload(
          'all.ping',
          PingRequest(clientSendTime: fixnum.Int64(1)),
        ),
      ),
    );
    await Future<void>.delayed(Duration.zero);

    final response = _singleEnvelopeResponse(channel);
    expect(response.id, 'srv-ping');
    expect(response.hasStatus(), isFalse);
    final decoded =
        decodeRpcResponsePayload('all.ping', response.payload) as PingResponse;
    expect(decoded.serverTime.toInt(), greaterThan(0));
  });

  test('serves server-initiated all.speed_test.run requests', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    serveGizClawPeerRpcChannel(channel);

    channel.addMessage(
      concatBytes([
        _rpcRequestEnvelopeBytes(
          id: 'srv-speed',
          method: rpc.RpcMethod.RPC_METHOD_ALL_SPEED_TEST_RUN,
          payloadBytes: encodeRpcRequestPayload(
            'all.speed_test.run',
            SpeedTestRequest(
              downContentLength: fixnum.Int64(3),
              upContentLength: fixnum.Int64(2),
            ),
          ),
        ),
        encodeFrame(rpcFrameTypeBinary, [7, 8]),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );
    await Future<void>.delayed(Duration.zero);

    final frames = decodeFrames(concatBytes(channel.sent));
    final response = rpc.RpcResponse.fromBuffer(frames.first.payload);
    expect(response.id, 'srv-speed');
    final decoded =
        decodeRpcResponsePayload('all.speed_test.run', response.payload)
            as SpeedTestResponse;
    expect(decoded.downContentLength.toInt(), 3);
    expect(decoded.upContentLength.toInt(), 2);
    expect(frames[1].type, rpcFrameTypeBinary);
    expect(frames[1].payload, [0, 0, 0]);
    expect(frames.last.type, rpcFrameTypeEos);
  });

  test('rejects server-initiated all.ping without payload', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    serveGizClawPeerRpcChannel(channel);

    channel.addMessage(
      _rpcRequestBytes(
        id: 'srv-missing-ping',
        method: rpc.RpcMethod.RPC_METHOD_ALL_PING,
      ),
    );
    await Future<void>.delayed(Duration.zero);

    final response = _singleEnvelopeResponse(channel);
    expect(response.id, 'srv-missing-ping');
    expect(response.status.code, rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT);
  });

  test('rejects server-initiated all.speed_test.run without payload', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    serveGizClawPeerRpcChannel(channel);

    channel.addMessage(
      _rpcRequestEnvelopeBytes(
        id: 'srv-missing-speed',
        method: rpc.RpcMethod.RPC_METHOD_ALL_SPEED_TEST_RUN,
      ),
    );
    await Future<void>.delayed(Duration.zero);

    final response = _singleEnvelopeResponse(channel);
    expect(response.id, 'srv-missing-speed');
    expect(response.status.code, rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT);
  });

  final device = DeviceInfo(
    name: 'Test Phone',
    hardware: HardwareInfo(
      hardwareRevision: 'revision-1',
      manufacturer: 'GizClaw',
      model: 'Phone Pro',
    ),
    identifiers: DeviceIdentifiers(
      sn: 'serial-1',
      imeis: [PeerIMEI(name: 'cellular', serial: 'imei-1')],
      labels: [PeerLabel(key: 'platform', value: 'test')],
    ),
  );

  test('serves client device info and identifiers', () async {
    final infoChannel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(infoChannel.close);
    serveGizClawPeerRpcChannel(
      infoChannel,
      handlers: GizClawPeerRpcHandlers(deviceInfo: () => device),
    );

    final infoResponse = await _callInbound(
      infoChannel,
      id: 'info-1',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'info.get',
      request: ClientGetInfoRequest(),
    );
    final info =
        decodeClientToolResponsePayload(
              clientToolByName('info.get').id,
              infoResponse.payload,
            )
            as ClientGetInfoResponse;
    expect(info.value.hardwareRevision, 'revision-1');
    expect(info.value.manufacturer, 'GizClaw');
    expect(info.value.model, 'Phone Pro');

    final identifiersChannel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(identifiersChannel.close);
    serveGizClawPeerRpcChannel(
      identifiersChannel,
      handlers: GizClawPeerRpcHandlers(deviceInfo: () => device),
    );
    final identifiersResponse = await _callInbound(
      identifiersChannel,
      id: 'identifiers-1',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'identifiers.get',
      request: ClientGetIdentifiersRequest(),
    );
    final identifiers =
        decodeClientToolResponsePayload(
              clientToolByName('identifiers.get').id,
              identifiersResponse.payload,
            )
            as ClientGetIdentifiersResponse;
    expect(identifiers.value.sn, 'serial-1');
    expect(identifiers.value.imeis.single.serial, 'imei-1');
    expect(identifiers.value.labels.single.value, 'test');
  });

  test('prefers a dedicated identifiers provider over device info', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(channel.close);
    var identifiersCalls = 0;
    serveGizClawPeerRpcChannel(
      channel,
      handlers: GizClawPeerRpcHandlers(
        deviceInfo: () => device,
        deviceIdentifiers: () {
          identifiersCalls++;
          return DeviceIdentifiers(
            sn: 'scripted-serial',
            labels: [PeerLabel(key: 'source', value: 'provider')],
          );
        },
      ),
    );
    final response = await _callInbound(
      channel,
      id: 'identifiers-2',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'identifiers.get',
      request: ClientGetIdentifiersRequest(),
    );
    final identifiers =
        decodeClientToolResponsePayload(
              clientToolByName('identifiers.get').id,
              response.payload,
            )
            as ClientGetIdentifiersResponse;
    expect(identifiersCalls, 1);
    expect(identifiers.value.sn, 'scripted-serial');
    expect(identifiers.value.labels.single.value, 'provider');
    expect(identifiers.value.imeis, isEmpty);
  });

  test('serves a configured tool/v0 procedure', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(channel.close);
    var calls = 0;
    serveGizClawPeerRpcChannel(
      channel,
      handlers: GizClawPeerRpcHandlers(
        deviceInfo: () => DeviceInfo(name: 'tool'),
        deviceControl: GizClawDeviceControlHandlers(
          find: (_) {
            calls++;
          },
        ),
      ),
    );
    final response = await _callInbound(
      channel,
      id: 'find',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'device.find',
      request: ClientDeviceFindRequest(),
    );
    expect(response.hasStatus(), isFalse);
    expect(calls, 1);
    expect(
      decodeClientToolResponsePayload(
        clientToolByName('device.find').id,
        response.payload,
      ),
      isA<ClientDeviceFindResponse>(),
    );
  });

  test('waits for tool/v0 request EOS before invoking a handler', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(channel.close);
    var calls = 0;
    serveGizClawPeerRpcChannel(
      channel,
      handlers: GizClawPeerRpcHandlers(
        deviceInfo: () => DeviceInfo(name: 'tool'),
        deviceControl: GizClawDeviceControlHandlers(
          find: (_) {
            calls++;
          },
        ),
      ),
    );
    channel.addMessage(
      _rpcRequestEnvelopeBytes(
        id: 'tool-wait-eos',
        method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
        payloadBytes: encodeRpcRequestPayload(
          'client.tool.v0.invoke',
          ClientToolV0InvokeRequest(
            tool: ClientTool.CLIENT_TOOL_DEVICE_FIND,
            payload: encodeClientToolRequestPayload(
              clientToolByName('device.find').id,
              ClientDeviceFindRequest(),
            ),
          ),
        ),
      ),
    );
    await Future<void>.delayed(Duration.zero);
    expect(calls, 0);
    expect(channel.sent, isEmpty);
    channel.addMessage(encodeFrame(rpcFrameTypeEos));
    for (var attempt = 0; channel.sent.length < 2; attempt++) {
      if (attempt == 20) fail('inbound RPC response was not sent');
      await Future<void>.delayed(Duration.zero);
    }
    expect(calls, 1);
  });

  test('rejects an unexpected tool/v0 request body', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    var calls = 0;
    serveGizClawPeerRpcChannel(
      channel,
      handlers: GizClawPeerRpcHandlers(
        deviceInfo: () => DeviceInfo(name: 'tool'),
        deviceControl: GizClawDeviceControlHandlers(
          find: (_) {
            calls++;
          },
        ),
      ),
    );
    channel.addMessage(
      concatBytes([
        _rpcRequestEnvelopeBytes(
          id: 'tool-body',
          method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
          payloadBytes: encodeRpcRequestPayload(
            'client.tool.v0.invoke',
            ClientToolV0InvokeRequest(tool: ClientTool.CLIENT_TOOL_DEVICE_FIND),
          ),
        ),
        encodeFrame(rpcFrameTypeBinary, [1]),
      ]),
    );
    await Future<void>.delayed(Duration.zero);
    expect(calls, 0);
    expect(channel.state, GizClawDataChannelState.closed);
  });

  test('reports an unconfigured tool/v0 handler', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(channel.close);
    serveGizClawPeerRpcChannel(
      channel,
      handlers: GizClawPeerRpcHandlers(
        deviceInfo: () => DeviceInfo(name: 'tool'),
      ),
    );
    final response = await _callInbound(
      channel,
      id: 'tool-missing',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'device.find',
      request: ClientDeviceFindRequest(),
    );
    expect(response.status.code, rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED);
  });
}

Uint8List _rpcRequestBytes({
  required String id,
  required rpc.RpcMethod method,
  List<int>? payloadBytes,
}) {
  return concatBytes([
    _rpcRequestEnvelopeBytes(
      id: id,
      method: method,
      payloadBytes: payloadBytes,
    ),
    encodeFrame(rpcFrameTypeEos),
  ]);
}

Uint8List _rpcRequestEnvelopeBytes({
  required String id,
  required rpc.RpcMethod method,
  List<int>? payloadBytes,
}) {
  return concatBytes(
    encodeEnvelopeFrames(
      rpc.RpcRequest(
        id: id,
        method: method,
        payload: payloadBytes,
      ).writeToBuffer(),
    ),
  );
}

rpc.RpcResponse _singleEnvelopeResponse(FakeDataChannel channel) {
  final frames = decodeFrames(concatBytes(channel.sent));
  expect(frames, hasLength(2));
  expect(frames.first.type, rpcFrameTypeBinary);
  expect(frames.last.type, rpcFrameTypeEos);
  return rpc.RpcResponse.fromBuffer(frames.first.payload);
}

Future<rpc.RpcResponse> _callInbound(
  FakeDataChannel channel, {
  required String id,
  required rpc.RpcMethod method,
  required String methodName,
  required GeneratedMessage request,
}) async {
  final tool = clientToolsByName[methodName];
  final wireMethod = tool == null
      ? method
      : rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE;
  final wirePayload = tool == null
      ? encodeRpcRequestPayload(methodName, request)
      : encodeRpcRequestPayload(
          'client.tool.v0.invoke',
          ClientToolV0InvokeRequest(
            tool: ClientTool.valueOf(tool.id),
            payload: encodeClientToolRequestPayload(tool.id, request),
          ),
        );
  final sentBefore = channel.sent.length;
  channel.addMessage(
    concatBytes([
      ...encodeEnvelopeFrames(
        rpc.RpcRequest(
          id: id,
          method: wireMethod,
          payload: wirePayload,
        ).writeToBuffer(),
      ),
      encodeFrame(rpcFrameTypeEos),
    ]),
  );
  for (var attempt = 0; channel.sent.length < sentBefore + 2; attempt++) {
    if (attempt == 20) fail('inbound RPC response was not sent');
    await Future<void>.delayed(Duration.zero);
  }
  final frames = decodeFrames(
    Uint8List.fromList(
      channel.sent.skip(sentBefore).expand((message) => message).toList(),
    ),
  );
  expect(frames.last.type, rpcFrameTypeEos);
  final response = rpc.RpcResponse.fromBuffer(frames.first.payload);
  if (tool != null && !response.hasStatus()) {
    response.payload = ClientToolV0InvokeResponse.fromBuffer(
      response.payload,
    ).payload;
  }
  return response;
}

void deviceControlTests() {
  final device = DeviceInfo(name: 'device-control');

  // Each server-initiated request owns one service channel, so every call
  // serves a fresh channel with the same handlers.
  Future<rpc.RpcResponse> callDevice(
    GizClawPeerRpcHandlers? handlers, {
    required String id,
    required rpc.RpcMethod method,
    required String methodName,
    required GeneratedMessage request,
  }) {
    final channel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(channel.close);
    serveGizClawPeerRpcChannel(channel, handlers: handlers);
    return _callInbound(
      channel,
      id: id,
      method: method,
      methodName: methodName,
      request: request,
    );
  }

  test('serves configured device control providers', () async {
    String? lastSound;
    int? lastDuration;
    final findDurations = <int?>[];
    int? lastDelay;
    int? lastScanTimeout;
    String? lastConnectSsid;
    String? lastPassphrase;
    final saved = ['home', 'office'];
    final handlers = GizClawPeerRpcHandlers(
      deviceInfo: () => device,
      deviceControl: GizClawDeviceControlHandlers(
        status: () => PeerStatus(volume: Int64(35), muted: true),
        playSound: (sound, durationMs) {
          if (sound != 'chime') {
            throw const GizClawDeviceControlException(
              rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
              'unknown sound',
            );
          }
          lastSound = sound;
          lastDuration = durationMs;
        },
        find: (durationMs) {
          if (durationMs == 1) {
            throw const GizClawDeviceControlException(
              rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED,
              'no speaker',
            );
          }
          findDurations.add(durationMs);
        },
        reboot: (delayMs) => lastDelay = delayMs,
        savedWifi: () => [
          for (final ssid in saved) WifiSavedNetwork(ssid: ssid),
        ],
        forgetWifi: (ssid) {
          if (!saved.remove(ssid)) {
            throw const GizClawDeviceControlException(
              rpc.StatusCode.STATUS_CODE_NOT_FOUND,
              'unknown network',
            );
          }
        },
        scanWifi: (timeoutMs) {
          lastScanTimeout = timeoutMs;
          return [WifiScanResult(ssid: 'office', rssiDbm: Int64(-42))];
        },
        connectWifi: (ssid, passphrase) {
          lastConnectSsid = ssid;
          lastPassphrase = passphrase;
        },
      ),
    );

    var response = await callDevice(
      handlers,
      id: 'status',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'device.status.get',
      request: ClientDeviceStatusGetRequest(),
    );
    final status =
        decodeClientToolResponsePayload(
              clientToolByName('device.status.get').id,
              response.payload,
            )
            as ClientDeviceStatusGetResponse;
    expect(status.value.volume, Int64(35));

    response = await callDevice(
      handlers,
      id: 'sound',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'sound.play',
      request: ClientDeviceSoundPlayRequest(
        sound: 'chime',
        durationMs: Int64(1500),
      ),
    );
    expect(response.hasStatus(), isFalse);
    expect(lastSound, 'chime');
    expect(lastDuration, 1500);
    response = await callDevice(
      handlers,
      id: 'sound-rejected',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'sound.play',
      request: ClientDeviceSoundPlayRequest(sound: 'unknown'),
    );
    expect(response.status.code, rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT);
    expect(response.status.message, 'unknown sound');
    response = await callDevice(
      handlers,
      id: 'sound-too-long',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'sound.play',
      request: ClientDeviceSoundPlayRequest(sound: 'a' * 33),
    );
    expect(response.status.code, rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT);

    for (final duration in <int?>[8000, null]) {
      response = await callDevice(
        handlers,
        id: 'find',
        method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
        methodName: 'device.find',
        request: ClientDeviceFindRequest(
          durationMs: duration == null ? null : Int64(duration),
        ),
      );
      expect(response.hasStatus(), isFalse);
      expect(
        decodeClientToolResponsePayload(
          clientToolByName('device.find').id,
          response.payload,
        ),
        isA<ClientDeviceFindResponse>(),
      );
    }
    expect(findDurations, [8000, null]);
    response = await callDevice(
      handlers,
      id: 'find-negative',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'device.find',
      request: ClientDeviceFindRequest(durationMs: Int64(-1)),
    );
    expect(response.status.code, rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT);
    response = await callDevice(
      handlers,
      id: 'find-rejected',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'device.find',
      request: ClientDeviceFindRequest(durationMs: Int64(1)),
    );
    expect(response.status.code, rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED);
    expect(response.status.message, 'no speaker');
    expect(findDurations, [8000, null]);

    response = await callDevice(
      handlers,
      id: 'reboot',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'device.reboot',
      request: ClientDeviceRebootRequest(delayMs: Int64(2000)),
    );
    expect(response.hasStatus(), isFalse);
    expect(lastDelay, 2000);

    response = await callDevice(
      handlers,
      id: 'forget',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'wifi.saved.forget',
      request: ClientWifiSavedForgetRequest(ssid: 'office'),
    );
    expect(response.hasStatus(), isFalse);
    response = await callDevice(
      handlers,
      id: 'forget-missing',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'wifi.saved.forget',
      request: ClientWifiSavedForgetRequest(ssid: 'office'),
    );
    expect(response.status.code, rpc.StatusCode.STATUS_CODE_NOT_FOUND);

    response = await callDevice(
      handlers,
      id: 'saved',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'wifi.saved.list',
      request: ClientWifiSavedListRequest(),
    );
    final list =
        decodeClientToolResponsePayload(
              clientToolByName('wifi.saved.list').id,
              response.payload,
            )
            as ClientWifiSavedListResponse;
    expect(list.networks.map((n) => n.ssid), ['home']);

    response = await callDevice(
      handlers,
      id: 'scan',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'wifi.scan',
      request: ClientWifiScanRequest(timeoutMs: Int64(8000)),
    );
    final scan =
        decodeClientToolResponsePayload(
              clientToolByName('wifi.scan').id,
              response.payload,
            )
            as ClientWifiScanResponse;
    expect(scan.networks.single.ssid, 'office');
    expect(scan.networks.single.rssiDbm, Int64(-42));
    expect(lastScanTimeout, 8000);

    response = await callDevice(
      handlers,
      id: 'connect',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'wifi.connect',
      request: ClientWifiConnectRequest(
        ssid: 'office',
        passphrase: 'correct-horse',
      ),
    );
    expect(response.hasStatus(), isFalse);
    expect(lastConnectSsid, 'office');
    expect(lastPassphrase, 'correct-horse');
  });

  test('answers METHOD_NOT_FOUND for uninstalled tool handlers', () async {
    final partial = GizClawPeerRpcHandlers(
      deviceInfo: () => DeviceInfo(name: 'tool'),
      deviceControl: GizClawDeviceControlHandlers(
        status: () => PeerStatus(volume: Int64(20)),
      ),
    );
    var response = await callDevice(
      partial,
      id: 'status',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'device.status.get',
      request: ClientDeviceStatusGetRequest(),
    );
    expect(response.hasStatus(), isFalse);
    for (final unsupported in [
      ('wifi.scan', ClientWifiScanRequest()),
      ('wifi.connect', ClientWifiConnectRequest(ssid: 'home')),
      ('device.find', ClientDeviceFindRequest()),
    ]) {
      response = await callDevice(
        partial,
        id: 'unsupported',
        method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
        methodName: unsupported.$1,
        request: unsupported.$2,
      );
      expect(response.status.code, rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED);
    }
  });

  test('serves configured social ping handler', () async {
    final pings = <ClientSocialPingRequest>[];
    final handlers = GizClawPeerRpcHandlers(
      deviceInfo: () => device,
      socialPing: (request) {
        if (request.fromPeerPublicKey == 'busy') {
          throw const GizClawDeviceControlException(
            rpc.StatusCode.STATUS_CODE_UNAVAILABLE,
            'busy',
          );
        }
        pings.add(request);
      },
    );

    var response = await callDevice(
      handlers,
      id: 'friend-ping',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'social.ping',
      request: ClientSocialPingRequest(
        fromPeerPublicKey: 'peer-a',
        fromDisplayName: 'Alice',
      ),
    );
    expect(response.hasStatus(), isFalse);
    expect(
      decodeClientToolResponsePayload(
        clientToolByName('social.ping').id,
        response.payload,
      ),
      isA<ClientSocialPingResponse>(),
    );
    response = await callDevice(
      handlers,
      id: 'group-ping',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'social.ping',
      request: ClientSocialPingRequest(
        fromPeerPublicKey: 'peer-b',
        friendGroupName: 'my-team',
      ),
    );
    expect(response.hasStatus(), isFalse);
    expect(pings.map((ping) => ping.fromPeerPublicKey), ['peer-a', 'peer-b']);
    expect(pings.first.fromDisplayName, 'Alice');
    expect(pings.first.hasFriendGroupName(), isFalse);
    expect(pings.last.hasFromDisplayName(), isFalse);
    expect(pings.last.friendGroupName, 'my-team');

    response = await callDevice(
      handlers,
      id: 'ping-missing-sender',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'social.ping',
      request: ClientSocialPingRequest(),
    );
    expect(response.status.code, rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT);
    response = await callDevice(
      handlers,
      id: 'ping-rejected',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'social.ping',
      request: ClientSocialPingRequest(fromPeerPublicKey: 'busy'),
    );
    expect(response.status.code, rpc.StatusCode.STATUS_CODE_UNAVAILABLE);
    expect(response.status.message, 'busy');
    expect(pings, hasLength(2));
  });

  test('answers METHOD_NOT_FOUND for social ping without handler', () async {
    for (final handlers in [
      GizClawPeerRpcHandlers(deviceInfo: () => device),
      null,
    ]) {
      final response = await callDevice(
        handlers,
        id: 'no-social-ping',
        method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
        methodName: 'social.ping',
        request: ClientSocialPingRequest(fromPeerPublicKey: 'peer-a'),
      );
      expect(response.status.code, rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED);
    }
  });

  test('serves factory reset and separate capability lists', () async {
    bool? keepNetwork;
    final handlers = GizClawPeerRpcHandlers(
      deviceInfo: () => device,
      socialPing: (_) {},
      deviceControl: GizClawDeviceControlHandlers(
        find: (_) {},
        factoryReset: (keep) => keepNetwork = keep,
        writeMhsStates: (request) =>
            ClientMhsV0WriteResponse(states: request.states),
      ),
    );
    var response = await callDevice(
      handlers,
      id: 'reset-default',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'device.factory_reset',
      request: ClientDeviceFactoryResetRequest(),
    );
    expect(response.hasStatus(), isFalse);
    expect(keepNetwork, isFalse);
    response = await callDevice(
      handlers,
      id: 'reset-keep',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'device.factory_reset',
      request: ClientDeviceFactoryResetRequest(keepNetwork: true),
    );
    expect(response.hasStatus(), isFalse);
    expect(keepNetwork, isTrue);
    response = await callDevice(
      handlers,
      id: 'methods',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_RPC_METHODS_LIST,
      methodName: 'client.rpc.methods.list',
      request: ClientRpcMethodsListRequest(),
    );
    expect(response.hasStatus(), isFalse);
    final methods =
        decodeRpcResponsePayload('client.rpc.methods.list', response.payload)
            as ClientRpcMethodsListResponse;
    expect(methods.methods.map((method) => method.value).toList(), [
      1,
      2,
      134,
      135,
      136,
      137,
    ]);
    response = await callDevice(
      handlers,
      id: 'tools',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_LIST,
      methodName: 'client.tool.v0.list',
      request: ClientToolV0ListRequest(),
    );
    final tools =
        decodeRpcResponsePayload('client.tool.v0.list', response.payload)
            as ClientToolV0ListResponse;
    expect(tools.tools, contains(ClientTool.CLIENT_TOOL_DEVICE_FIND));
    expect(tools.tools, contains(ClientTool.CLIENT_TOOL_DEVICE_FACTORY_RESET));
    expect(tools.tools, contains(ClientTool.CLIENT_TOOL_SOCIAL_PING));
  });

  test('answers method list without device control handlers', () async {
    final response = await callDevice(
      GizClawPeerRpcHandlers(deviceInfo: () => device),
      id: 'methods-bare',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_RPC_METHODS_LIST,
      methodName: 'client.rpc.methods.list',
      request: ClientRpcMethodsListRequest(),
    );
    expect(response.hasStatus(), isFalse);
    final methods =
        decodeRpcResponsePayload('client.rpc.methods.list', response.payload)
            as ClientRpcMethodsListResponse;
    expect(methods.methods.map((method) => method.value).toList(), [
      1,
      2,
      135,
      136,
      137,
    ]);
  });

  test('serves client.run.workspace.set for a named Workspace', () async {
    final seen = <ClientRunWorkspaceSetRequest>[];
    final handlers = GizClawPeerRpcHandlers(
      deviceInfo: () => device,
      deviceControl: GizClawDeviceControlHandlers(setRunWorkspace: seen.add),
    );
    var response = await callDevice(
      handlers,
      id: 'run-workflow',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
      methodName: 'run.workspace.set',
      request: ClientRunWorkspaceSetRequest(
        workspaceName: 'bedtime',
        kickoff: true,
      ),
    );
    expect(response.hasStatus(), isFalse);
    for (final bad in [
      ClientRunWorkspaceSetRequest(),
      ClientRunWorkspaceSetRequest(workspaceName: ''),
    ]) {
      response = await callDevice(
        handlers,
        id: 'run-bad',
        method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE,
        methodName: 'run.workspace.set',
        request: bad,
      );
      expect(
        response.status.code,
        rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
        reason: '$bad',
      );
    }
    expect(seen, hasLength(1));
    expect(seen.single.workspaceName, 'bedtime');
    expect(seen.single.kickoff, isTrue);
  });
}

void mhsTests() {
  test(
    'MHS preserves oneof defaults and advertises installed providers',
    () async {
      var calls = 0;
      final handlers = GizClawPeerRpcHandlers(
        deviceInfo: () => DeviceInfo(name: 'mhs'),
        deviceControl: GizClawDeviceControlHandlers(
          writeMhsStates: (request) {
            calls++;
            return ClientMhsV0WriteResponse(states: request.states);
          },
        ),
      );
      for (final value in [
        MhsValue(boolValue: false),
        MhsValue(intValue: Int64.ZERO),
        MhsValue(doubleValue: 0),
        MhsValue(stringValue: ''),
      ]) {
        final channel = FakeDataChannel('giznet/v1/service/0');
        addTearDown(channel.close);
        serveGizClawPeerRpcChannel(channel, handlers: handlers);
        final response = await _callInbound(
          channel,
          id: 'mhs',
          method: rpc.RpcMethod.RPC_METHOD_CLIENT_MHS_V0_WRITE,
          methodName: 'client.mhs.v0.write',
          request: ClientMhsV0WriteRequest(
            states: [
              MhsStateValue(deviceId: 'led.main', state: 'state', value: value),
            ],
          ),
        );
        expect(response.hasStatus(), isFalse);
        final result =
            decodeRpcResponsePayload('client.mhs.v0.write', response.payload)
                as ClientMhsV0WriteResponse;
        expect(result.states.single.value.whichValue(), value.whichValue());
        expect(result.states.single.value, value);
      }
      final channel = FakeDataChannel('giznet/v1/service/0');
      addTearDown(channel.close);
      serveGizClawPeerRpcChannel(channel, handlers: handlers);
      final response = await _callInbound(
        channel,
        id: 'methods',
        method: rpc.RpcMethod.RPC_METHOD_CLIENT_RPC_METHODS_LIST,
        methodName: 'client.rpc.methods.list',
        request: ClientRpcMethodsListRequest(),
      );
      final methods =
          (decodeRpcResponsePayload('client.rpc.methods.list', response.payload)
                  as ClientRpcMethodsListResponse)
              .methods;
      expect(methods.map((method) => method.value), contains(134));
      expect(methods.map((method) => method.value), isNot(contains(133)));
      expect(calls, 4);
    },
  );
  test('MHS rejects malformed batches before provider invocation', () async {
    var calls = 0;
    final handlers = GizClawPeerRpcHandlers(
      deviceInfo: () => DeviceInfo(name: 'mhs'),
      deviceControl: GizClawDeviceControlHandlers(
        writeMhsStates: (request) {
          calls++;
          return ClientMhsV0WriteResponse(states: request.states);
        },
      ),
    );
    final key = MhsStateValue(
      deviceId: 'led.main',
      state: 'enabled',
      value: MhsValue(boolValue: false),
    );
    for (final states in <List<MhsStateValue>>[
      [],
      [key, key],
      [
        MhsStateValue(
          deviceId: 'led.main',
          state: 'enabled\n',
          value: MhsValue(boolValue: false),
        ),
      ],
      [MhsStateValue(deviceId: 'led.main', state: 'enabled')],
      [
        MhsStateValue(
          deviceId: 'led.main',
          state: 'enabled',
          value: MhsValue(stringValue: 'x' * 257),
        ),
      ],
    ]) {
      final channel = FakeDataChannel('giznet/v1/service/0');
      addTearDown(channel.close);
      serveGizClawPeerRpcChannel(channel, handlers: handlers);
      final response = await _callInbound(
        channel,
        id: 'invalid',
        method: rpc.RpcMethod.RPC_METHOD_CLIENT_MHS_V0_WRITE,
        methodName: 'client.mhs.v0.write',
        request: ClientMhsV0WriteRequest(states: states),
      );
      expect(response.status.code, rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT);
    }
    expect(calls, 0);
  });
  test('MHS read dispatches and absent handlers are unsupported', () async {
    for (final installed in [false, true]) {
      final channel = FakeDataChannel('giznet/v1/service/0');
      addTearDown(channel.close);
      serveGizClawPeerRpcChannel(
        channel,
        handlers: GizClawPeerRpcHandlers(
          deviceInfo: () => DeviceInfo(name: 'mhs'),
          deviceControl: GizClawDeviceControlHandlers(
            readMhsStates: installed
                ? (request) => ClientMhsV0ReadResponse(
                    states: [
                      for (final ref in request.states)
                        MhsStateValue(
                          deviceId: ref.deviceId,
                          state: ref.state,
                          value: MhsValue(boolValue: false),
                        ),
                    ],
                  )
                : null,
          ),
        ),
      );
      final response = await _callInbound(
        channel,
        id: 'read',
        method: rpc.RpcMethod.RPC_METHOD_CLIENT_MHS_V0_READ,
        methodName: 'client.mhs.v0.read',
        request: ClientMhsV0ReadRequest(
          states: [MhsStateRef(deviceId: 'led.main', state: 'enabled')],
        ),
      );
      if (!installed) {
        expect(response.status.code, rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED);
      } else {
        expect(response.hasStatus(), isFalse);
      }
    }
  });
}
