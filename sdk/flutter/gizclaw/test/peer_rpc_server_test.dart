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
    expect(response.hasError(), isFalse);
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
    expect(response.error.code, rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS);
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
    expect(response.error.code, rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS);
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
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_INFO_GET,
      methodName: 'client.info.get',
      request: ClientGetInfoRequest(),
    );
    final info =
        decodeRpcResponsePayload('client.info.get', infoResponse.payload)
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
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_IDENTIFIERS_GET,
      methodName: 'client.identifiers.get',
      request: ClientGetIdentifiersRequest(),
    );
    final identifiers =
        decodeRpcResponsePayload(
              'client.identifiers.get',
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
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_IDENTIFIERS_GET,
      methodName: 'client.identifiers.get',
      request: ClientGetIdentifiersRequest(),
    );
    final identifiers =
        decodeRpcResponsePayload('client.identifiers.get', response.payload)
            as ClientGetIdentifiersResponse;
    expect(identifiersCalls, 1);
    expect(identifiers.value.sn, 'scripted-serial');
    expect(identifiers.value.labels.single.value, 'provider');
    expect(identifiers.value.imeis, isEmpty);
  });

  test('serves configured client tool invocations', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(channel.close);
    Map<String, Object?>? invoked;
    serveGizClawPeerRpcChannel(
      channel,
      handlers: GizClawPeerRpcHandlers(
        deviceInfo: () => device,
        tools: {
          'music_play': (arguments) {
            invoked = arguments;
            return {'ok': true};
          },
        },
      ),
    );

    final response = await _callInbound(
      channel,
      id: 'tool-1',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_INVOKE,
      methodName: 'client.tool.invoke',
      request: ToolInvokeRequest(invokeName: 'music_play'),
    );
    final result =
        decodeRpcResponsePayload('client.tool.invoke', response.payload)
            as ToolInvokeResponse;
    expect(invoked, isEmpty);
    expect(result.dataJson, '{"ok":true}');
  });

  test('waits for client request EOS before invoking a handler', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(channel.close);
    var invocationCount = 0;
    serveGizClawPeerRpcChannel(
      channel,
      handlers: GizClawPeerRpcHandlers(
        deviceInfo: () => device,
        tools: {
          'music_play': (arguments) {
            invocationCount++;
            return {'ok': true};
          },
        },
      ),
    );

    channel.addMessage(
      _rpcRequestEnvelopeBytes(
        id: 'tool-wait-eos',
        method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_INVOKE,
        payloadBytes: encodeRpcRequestPayload(
          'client.tool.invoke',
          ToolInvokeRequest(invokeName: 'music_play'),
        ),
      ),
    );
    await Future<void>.delayed(Duration.zero);

    expect(invocationCount, 0);
    expect(channel.sent, isEmpty);

    channel.addMessage(encodeFrame(rpcFrameTypeEos));
    for (var attempt = 0; channel.sent.length < 2; attempt++) {
      if (attempt == 20) fail('inbound RPC response was not sent');
      await Future<void>.delayed(Duration.zero);
    }

    expect(invocationCount, 1);
    expect(_singleEnvelopeResponse(channel).id, 'tool-wait-eos');
  });

  test('rejects an unexpected client request body', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    var invocationCount = 0;
    serveGizClawPeerRpcChannel(
      channel,
      handlers: GizClawPeerRpcHandlers(
        deviceInfo: () => device,
        tools: {
          'music_play': (arguments) {
            invocationCount++;
            return null;
          },
        },
      ),
    );

    channel.addMessage(
      concatBytes([
        _rpcRequestEnvelopeBytes(
          id: 'tool-body',
          method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_INVOKE,
          payloadBytes: encodeRpcRequestPayload(
            'client.tool.invoke',
            ToolInvokeRequest(invokeName: 'music_play'),
          ),
        ),
        encodeFrame(rpcFrameTypeBinary, [1]),
      ]),
    );
    await Future<void>.delayed(Duration.zero);

    expect(invocationCount, 0);
    expect(channel.sent, isEmpty);
    expect(channel.state, GizClawDataChannelState.closed);
  });

  test('reports an unconfigured client tool handler', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(channel.close);
    serveGizClawPeerRpcChannel(
      channel,
      handlers: GizClawPeerRpcHandlers(deviceInfo: () => device),
    );

    final response = await _callInbound(
      channel,
      id: 'tool-missing',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_INVOKE,
      methodName: 'client.tool.invoke',
      request: ToolInvokeRequest(invokeName: 'missing_tool'),
    );
    expect(
      response.error.code,
      rpc.RpcErrorCode.RPC_ERROR_CODE_METHOD_NOT_FOUND,
    );
    expect(response.error.message, 'Tool unavailable');
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
  final sentBefore = channel.sent.length;
  channel.addMessage(
    concatBytes([
      ...encodeEnvelopeFrames(
        rpc.RpcRequest(
          id: id,
          method: method,
          payload: encodeRpcRequestPayload(methodName, request),
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
  return rpc.RpcResponse.fromBuffer(frames.first.payload);
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
    var volume = 50;
    var muted = false;
    String? lastSound;
    int? lastDuration;
    int? lastDelay;
    final saved = ['home', 'office'];
    final handlers = GizClawPeerRpcHandlers(
      deviceInfo: () => device,
      deviceControl: GizClawDeviceControlHandlers(
        status: () => PeerStatus(volume: Int64(volume), muted: muted),
        setVolume: (level, isMuted) {
          volume = level;
          muted = isMuted;
          return PeerStatus(
            volume: Int64(level),
            muted: isMuted,
            batteryPercent: Int64(88),
          );
        },
        playSound: (sound, durationMs) {
          if (sound != 'chime') {
            throw const GizClawDeviceControlException(
              rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS,
              'unknown sound',
            );
          }
          lastSound = sound;
          lastDuration = durationMs;
        },
        reboot: (delayMs) => lastDelay = delayMs,
        wifiStatus: () =>
            WifiStatus(connected: true, ssid: 'home', rssiDbm: Int64(-61)),
        savedWifi: () => [
          for (final ssid in saved) WifiSavedNetwork(ssid: ssid),
        ],
        forgetWifi: (ssid) {
          if (!saved.remove(ssid)) {
            throw const GizClawDeviceControlException(
              rpc.RpcErrorCode.RPC_ERROR_CODE_NOT_FOUND,
              'unknown network',
            );
          }
        },
      ),
    );

    var response = await callDevice(
      handlers,
      id: 'volume',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_DEVICE_VOLUME_SET,
      methodName: 'client.device.volume.set',
      request: ClientDeviceVolumeSetRequest(level: Int64(35), muted: true),
    );
    expect(response.hasError(), isFalse);
    final applied =
        decodeRpcResponsePayload('client.device.volume.set', response.payload)
            as ClientDeviceVolumeSetResponse;
    expect(applied.value.volume, Int64(35));
    expect(applied.value.muted, isTrue);
    expect(applied.value.batteryPercent, Int64(88));
    expect(volume, 35);

    response = await callDevice(
      handlers,
      id: 'volume-range',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_DEVICE_VOLUME_SET,
      methodName: 'client.device.volume.set',
      request: ClientDeviceVolumeSetRequest(level: Int64(101), muted: false),
    );
    expect(response.error.code, rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS);
    expect(volume, 35);

    response = await callDevice(
      handlers,
      id: 'status',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_DEVICE_STATUS_GET,
      methodName: 'client.device.status.get',
      request: ClientDeviceStatusGetRequest(),
    );
    final status =
        decodeRpcResponsePayload('client.device.status.get', response.payload)
            as ClientDeviceStatusGetResponse;
    expect(status.value.volume, Int64(35));

    response = await callDevice(
      handlers,
      id: 'sound',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_DEVICE_SOUND_PLAY,
      methodName: 'client.device.sound.play',
      request: ClientDeviceSoundPlayRequest(
        sound: 'chime',
        durationMs: Int64(1500),
      ),
    );
    expect(response.hasError(), isFalse);
    expect(lastSound, 'chime');
    expect(lastDuration, 1500);
    response = await callDevice(
      handlers,
      id: 'sound-rejected',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_DEVICE_SOUND_PLAY,
      methodName: 'client.device.sound.play',
      request: ClientDeviceSoundPlayRequest(sound: 'unknown'),
    );
    expect(response.error.code, rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS);
    expect(response.error.message, 'unknown sound');
    response = await callDevice(
      handlers,
      id: 'sound-too-long',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_DEVICE_SOUND_PLAY,
      methodName: 'client.device.sound.play',
      request: ClientDeviceSoundPlayRequest(sound: 'a' * 33),
    );
    expect(response.error.code, rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS);

    response = await callDevice(
      handlers,
      id: 'reboot',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_DEVICE_REBOOT,
      methodName: 'client.device.reboot',
      request: ClientDeviceRebootRequest(delayMs: Int64(2000)),
    );
    expect(response.hasError(), isFalse);
    expect(lastDelay, 2000);

    response = await callDevice(
      handlers,
      id: 'wifi',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_WIFI_STATUS_GET,
      methodName: 'client.wifi.status.get',
      request: ClientWifiStatusGetRequest(),
    );
    final wifi =
        decodeRpcResponsePayload('client.wifi.status.get', response.payload)
            as ClientWifiStatusGetResponse;
    expect(wifi.value.connected, isTrue);
    expect(wifi.value.ssid, 'home');
    expect(wifi.value.rssiDbm, Int64(-61));

    response = await callDevice(
      handlers,
      id: 'forget',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_WIFI_SAVED_FORGET,
      methodName: 'client.wifi.saved.forget',
      request: ClientWifiSavedForgetRequest(ssid: 'office'),
    );
    expect(response.hasError(), isFalse);
    response = await callDevice(
      handlers,
      id: 'forget-missing',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_WIFI_SAVED_FORGET,
      methodName: 'client.wifi.saved.forget',
      request: ClientWifiSavedForgetRequest(ssid: 'office'),
    );
    expect(response.error.code, rpc.RpcErrorCode.RPC_ERROR_CODE_NOT_FOUND);

    response = await callDevice(
      handlers,
      id: 'saved',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_WIFI_SAVED_LIST,
      methodName: 'client.wifi.saved.list',
      request: ClientWifiSavedListRequest(),
    );
    final list =
        decodeRpcResponsePayload('client.wifi.saved.list', response.payload)
            as ClientWifiSavedListResponse;
    expect(list.networks.map((n) => n.ssid), ['home']);
  });

  test(
    'answers METHOD_NOT_FOUND for device control without handlers',
    () async {
      final partial = GizClawPeerRpcHandlers(
        deviceInfo: () => device,
        deviceControl: GizClawDeviceControlHandlers(
          wifiStatus: () => WifiStatus(connected: false),
        ),
      );
      var response = await callDevice(
        partial,
        id: 'no-volume',
        method: rpc.RpcMethod.RPC_METHOD_CLIENT_DEVICE_VOLUME_SET,
        methodName: 'client.device.volume.set',
        request: ClientDeviceVolumeSetRequest(level: Int64(1), muted: false),
      );
      expect(
        response.error.code,
        rpc.RpcErrorCode.RPC_ERROR_CODE_METHOD_NOT_FOUND,
      );
      response = await callDevice(
        partial,
        id: 'wifi-ok',
        method: rpc.RpcMethod.RPC_METHOD_CLIENT_WIFI_STATUS_GET,
        methodName: 'client.wifi.status.get',
        request: ClientWifiStatusGetRequest(),
      );
      expect(response.hasError(), isFalse);

      response = await callDevice(
        null,
        id: 'no-handlers',
        method: rpc.RpcMethod.RPC_METHOD_CLIENT_DEVICE_REBOOT,
        methodName: 'client.device.reboot',
        request: ClientDeviceRebootRequest(),
      );
      expect(
        response.error.code,
        rpc.RpcErrorCode.RPC_ERROR_CODE_METHOD_NOT_FOUND,
      );
    },
  );
}
