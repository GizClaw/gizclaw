import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:fixnum/fixnum.dart' as fixnum;
import 'package:protobuf/protobuf.dart' show GeneratedMessage;

import 'generated/rpc/rpc.pb.dart' as rpc;
import 'generated/rpc/payload.pb.dart' as payload;
import 'method_registry.dart';
import 'payload_codec.dart';
import 'rpc_frame.dart';
import 'transport.dart';

const _rpcSpeedTestFrameSize = 32 * 1024;
const _rpcSpeedTestMaxContentLength = 1 << 30;

typedef GizClawDeviceInfoProvider = FutureOr<payload.DeviceInfo> Function();
typedef GizClawDeviceIdentifiersProvider =
    FutureOr<payload.DeviceIdentifiers> Function();
typedef GizClawToolHandler =
    FutureOr<GeneratedMessage> Function(GeneratedMessage request);

/// Receives `social.ping`: a Friend pinged this device, or a Friend
/// Group member rallied the group when `friendGroupName` is set. It should
/// alert the user and return promptly; the Server counts an error or a late
/// acknowledgement as not delivered.
typedef GizClawSocialPingHandler =
    FutureOr<void> Function(payload.ClientSocialPingRequest request);

/// Thrown by a device control handler to answer the Server with a specific
/// RPC error code, for example `INVALID_PARAMS` for an unknown sound or
/// `NOT_FOUND` for an unknown saved network.
class GizClawDeviceControlException implements Exception {
  const GizClawDeviceControlException(this.code, [this.message = '']);

  final rpc.StatusCode code;
  final String message;

  @override
  String toString() => 'GizClawDeviceControlException($code, $message)';
}

/// Device single-player providers. Playlist mutations must be atomic.
class GizClawAudioPlayerHandlers {
  const GizClawAudioPlayerHandlers({
    this.get,
    this.playlistGet,
    this.playlistSet,
    this.playlistAppend,
    this.play,
    this.stop,
    this.modeSet,
  });
  final FutureOr<payload.ClientDeviceAudioPlayerGetResponse> Function(
    payload.ClientDeviceAudioPlayerGetRequest request,
  )?
  get;
  final FutureOr<payload.ClientDeviceAudioPlayerPlaylistGetResponse> Function(
    payload.ClientDeviceAudioPlayerPlaylistGetRequest request,
  )?
  playlistGet;
  final FutureOr<payload.ClientDeviceAudioPlayerPlaylistSetResponse> Function(
    payload.ClientDeviceAudioPlayerPlaylistSetRequest request,
  )?
  playlistSet;
  final FutureOr<payload.ClientDeviceAudioPlayerPlaylistAppendResponse>
  Function(payload.ClientDeviceAudioPlayerPlaylistAppendRequest request)?
  playlistAppend;
  final FutureOr<payload.ClientDeviceAudioPlayerPlayResponse> Function(
    payload.ClientDeviceAudioPlayerPlayRequest request,
  )?
  play;
  final FutureOr<payload.ClientDeviceAudioPlayerStopResponse> Function(
    payload.ClientDeviceAudioPlayerStopRequest request,
  )?
  stop;
  final FutureOr<payload.ClientDeviceAudioPlayerModeSetResponse> Function(
    payload.ClientDeviceAudioPlayerModeSetRequest request,
  )?
  modeSet;
}

/// Implements the Server-initiated `device.*` and `wifi.*`
/// methods. A null handler answers `METHOD_NOT_FOUND`, which the Server maps
/// to `501 DEVICE_UNSUPPORTED`.
class GizClawDeviceControlHandlers {
  const GizClawDeviceControlHandlers({
    this.audioplayer,
    this.status,
    this.playSound,
    this.find,
    this.reboot,
    this.savedWifi,
    this.forgetWifi,
    this.scanWifi,
    this.connectWifi,
    this.updateFirmware,
    this.factoryReset,
    this.setRunWorkspace,
    this.readMhsStates,
    this.writeMhsStates,
  });

  /// Reads exactly the requested keys; absent hardware returns NOT_FOUND.
  final FutureOr<payload.ClientMhsV0ReadResponse> Function(
    payload.ClientMhsV0ReadRequest request,
  )?
  readMhsStates;

  /// Validate every entry and driver safety limit before applying anything.
  /// Reject the whole batch on error; return the values actually in effect.
  final FutureOr<payload.ClientMhsV0WriteResponse> Function(
    payload.ClientMhsV0WriteRequest request,
  )?
  writeMhsStates;

  final GizClawAudioPlayerHandlers? audioplayer;
  final FutureOr<payload.PeerStatus> Function()? status;
  final FutureOr<void> Function(String sound, int? durationMs)? playSound;

  /// Rings the built-in find-me sound for `device.find`. `durationMs`
  /// is null when the caller leaves the ring time to the device.
  final FutureOr<void> Function(int? durationMs)? find;
  final FutureOr<void> Function(int? delayMs)? reboot;
  final FutureOr<List<payload.WifiSavedNetwork>> Function()? savedWifi;
  final FutureOr<void> Function(String ssid)? forgetWifi;
  final FutureOr<List<payload.WifiScanResult>> Function(int? timeoutMs)?
  scanWifi;
  final FutureOr<void> Function(String ssid, String? passphrase)? connectWifi;

  /// Runs one OTA for `firmware.update`. `channel` is null when the
  /// caller leaves the choice to the device; `sha256` is the package digest the
  /// caller resolved, and the handler throws
  /// [GizClawDeviceControlException] with `STATUS_CODE_INVALID_ARGUMENT` when it
  /// does not match the package the device resolves.
  final FutureOr<void> Function(
    payload.FirmwareChannelName? channel,
    String? sha256,
  )?
  updateFirmware;

  /// Erases device-local state for `device.factory_reset`.
  /// `keepNetwork` retains saved Wi-Fi and cellular configuration.
  ///
  /// Like [reboot], the handler must complete promptly and only then perform
  /// the reset: the acknowledgement is sent from its return, so a handler that
  /// tears down networking or blocks first leaves the caller without the
  /// response the method promises. Schedule the reset and return.
  final FutureOr<void> Function(bool keepNetwork)? factoryReset;

  /// Switches the Workspace the device runs for `run.workspace.set` to
  /// `request.workspaceName`, already validated; the Server has resolved any
  /// workflow target to this one name.
  ///
  /// The acknowledgement only means the device accepted the request: complete
  /// promptly, then switch through `server.run.workspace.reload-with-options`.
  final FutureOr<void> Function(payload.ClientRunWorkspaceSetRequest request)?
  setRunWorkspace;
}

class GizClawPeerRpcHandlers {
  GizClawPeerRpcHandlers({
    required this.deviceInfo,
    this.observe,
    Map<payload.ClientTool, GizClawToolHandler> tools = const {},
    this.deviceControl,
    this.deviceIdentifiers,
    this.socialPing,
  }) : tools = Map.unmodifiable(tools);

  /// Observes decoded requests, including a tool with no installed provider.
  final void Function(String method, payload.ClientTool? tool)? observe;
  final GizClawDeviceInfoProvider deviceInfo;
  final Map<payload.ClientTool, GizClawToolHandler> tools;
  final GizClawDeviceControlHandlers? deviceControl;

  /// Answers `identifiers.get`. When null the identifiers reported by
  /// [deviceInfo] are used, so a device that already reports them there needs
  /// no separate provider.
  final GizClawDeviceIdentifiersProvider? deviceIdentifiers;

  /// Answers `social.ping`. When null the device answers
  /// `METHOD_NOT_FOUND`, which the Server counts as not delivered. Throw
  /// [GizClawDeviceControlException] to answer a specific RPC error code.
  final GizClawSocialPingHandler? socialPing;
}

const _deviceControlMaxBytes = 32;

void serveGizClawPeerRpcChannel(
  GizClawDataChannel channel, {
  GizClawPeerRpcHandlers? handlers,
}) {
  _InboundPeerRpcChannel(channel, handlers).start();
}

class _InboundPeerRpcChannel {
  _InboundPeerRpcChannel(this.channel, this.handlers);

  final GizClawDataChannel channel;
  final GizClawPeerRpcHandlers? handlers;
  final _envelopeChunks = <Uint8List>[];
  var _buffer = Uint8List(0);
  var _closed = false;
  var _envelopeLength = 0;
  var _ignoreBody = false;
  var _uploaded = 0;
  rpc.RpcRequest? _request;
  late StreamSubscription<Uint8List> _messages;
  late StreamSubscription<GizClawDataChannelState> _states;

  void start() {
    _messages = channel.messages.listen(
      _handleMessage,
      onError: (_) => _close(),
      onDone: _close,
    );
    _states = channel.states.listen((state) {
      if (state == GizClawDataChannelState.closed) {
        _close();
      }
    }, onError: (_) => _close());
  }

  void _handleMessage(Uint8List chunk) {
    if (_closed) {
      return;
    }
    try {
      _buffer = concatBytes([_buffer, chunk]);
      for (;;) {
        final result = tryReadFrame(_buffer);
        if (result == null) {
          return;
        }
        _buffer = result.rest;
        _handleFrame(result.frame);
      }
    } catch (_) {
      _close();
    }
  }

  void _handleFrame(RpcFrame frame) {
    final request = _request;
    if (request == null) {
      if (frame.type == rpcFrameTypeText) {
        _envelopeLength += frame.payload.length;
        if (_envelopeLength > rpcMaxEnvelopeSize) {
          throw const FormatException('RPC protobuf envelope too large');
        }
        _envelopeChunks.add(Uint8List.fromList(frame.payload));
        return;
      }
      if (frame.type == rpcFrameTypeBinary) {
        if (_envelopeChunks.isNotEmpty) {
          throw const FormatException('RPC request has duplicate envelope');
        }
        _startRequest(rpc.RpcRequest.fromBuffer(frame.payload));
        return;
      }
      if (frame.type == rpcFrameTypeEos && _envelopeChunks.isNotEmpty) {
        final continuedRequest = rpc.RpcRequest.fromBuffer(
          concatBytes(_envelopeChunks),
        );
        _startRequest(continuedRequest);
        final methodName = _methodName(continuedRequest);
        if (methodName == 'all.ping') {
          _finishPing(continuedRequest);
        } else if (_isClientMethod(methodName)) {
          _finishClientRequest(continuedRequest);
        }
        return;
      }
      throw FormatException('expected RPC request envelope, got ${frame.type}');
    }

    if (_ignoreBody) {
      return;
    }
    final methodName = _methodName(request);
    if (methodName == 'all.ping') {
      if (frame.type != rpcFrameTypeEos) {
        throw FormatException('expected ping EOS frame, got ${frame.type}');
      }
      _finishPing(request);
      return;
    }
    if (methodName == 'all.speed_test.run') {
      if (frame.type == rpcFrameTypeBinary) {
        _uploaded += frame.payload.length;
        return;
      }
      if (frame.type != rpcFrameTypeEos) {
        throw FormatException(
          'expected speed-test body/EOS, got ${frame.type}',
        );
      }
      final params =
          decodeRpcRequestPayload(methodName, request.payload)
              as payload.SpeedTestRequest;
      if (_uploaded != params.upContentLength.toInt()) {
        throw StateError(
          'speed test upload length mismatch: got $_uploaded, '
          'want ${params.upContentLength}',
        );
      }
      _ignoreBody = true;
      return;
    }
    if (_isClientMethod(methodName)) {
      if (frame.type != rpcFrameTypeEos) {
        throw FormatException(
          'expected client RPC EOS frame, got ${frame.type}',
        );
      }
      _finishClientRequest(request);
      return;
    }
    _ignoreBody = true;
  }

  void _startRequest(rpc.RpcRequest request) {
    if (request.id.isEmpty || !request.hasMethod()) {
      throw const FormatException('invalid RPC request envelope');
    }
    _request = request;
    final methodName = _methodName(request);
    switch (methodName) {
      case 'all.ping':
        return;
      case 'all.speed_test.run':
        final params = _validSpeedTestParams(request);
        if (params == null) {
          _ignoreBody = true;
          _unawaited(
            _sendEnvelopeOnly(
              _rpcErrorResponse(
                request.id,
                rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
                'invalid params',
              ),
            ).catchError((_) => _close()),
          );
          return;
        }
        _unawaited(
          _sendSpeedTestResponse(
            request.id,
            params,
          ).catchError((_) => _close()),
        );
        return;
      case 'client.tool.v0.invoke':
      case 'client.tool.v0.list':
      case 'client.rpc.methods.list':
      case 'client.mhs.v0.read':
      case 'client.mhs.v0.write':
        return;
      default:
        _ignoreBody = true;
        _unawaited(
          _sendEnvelopeOnly(
            _rpcErrorResponse(
              request.id,
              rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED,
              'unsupported method: $methodName',
            ),
          ).catchError((_) => _close()),
        );
    }
  }

  Future<void> _serveClientRequest(rpc.RpcRequest request) async {
    final methodName = _methodName(request);
    late rpc.RpcResponse response;
    try {
      if (methodName != 'client.tool.v0.invoke') handlers?.observe?.call(methodName, null);
      response = switch (methodName) {
        'client.tool.v0.invoke' => await _invokeClientTool(request),
        'client.tool.v0.list' => _serveToolList(request),
        'client.rpc.methods.list' => _serveRpcMethods(request),
        'client.mhs.v0.read' || 'client.mhs.v0.write' => await _serveDeviceControl(request, methodName),
        _ => throw StateError('unsupported client method: $methodName'),
      };
    } on GizClawDeviceControlException catch (error) {
      response = _rpcErrorResponse(request.id, error.code, error.message);
    } catch (error) {
      response = _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INTERNAL,
        error.toString(),
      );
    }
    await _sendEnvelopeOnly(response);
  }

  void _finishClientRequest(rpc.RpcRequest request) {
    _ignoreBody = true;
    _unawaited(_serveClientRequest(request).catchError((_) => _close()));
  }

  Future<rpc.RpcResponse> _getClientInfo(rpc.RpcRequest request) async {
    final invalid = _validateClientRequest(request, 'info.get');
    if (invalid != null) return invalid;
    final provider = handlers?.deviceInfo;
    if (provider == null) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INTERNAL,
        'peer client not configured',
      );
    }
    final device = await provider();
    final info = payload.HardwareInfo();
    if (device.hasHardware()) {
      final hardware = device.hardware;
      if (hardware.hasHardwareRevision()) {
        info.hardwareRevision = hardware.hardwareRevision;
      }
      if (hardware.hasManufacturer()) {
        info.manufacturer = hardware.manufacturer;
      }
      if (hardware.hasModel()) info.model = hardware.model;
    }
    return _rpcPayloadResponse(
      request.id,
      'info.get',
      payload.ClientGetInfoResponse(value: info),
    );
  }

  Future<rpc.RpcResponse> _getClientIdentifiers(rpc.RpcRequest request) async {
    final invalid = _validateClientRequest(request, 'identifiers.get');
    if (invalid != null) return invalid;
    final identifiersProvider = handlers?.deviceIdentifiers;
    final provider = handlers?.deviceInfo;
    if (identifiersProvider == null && provider == null) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INTERNAL,
        'peer client not configured',
      );
    }
    final source = identifiersProvider != null
        ? await identifiersProvider()
        : (await provider!()).identifiers;
    final identifiers = payload.DeviceIdentifiers();
    if (source.hasSn()) identifiers.sn = source.sn;
    identifiers.imeis.addAll(source.imeis);
    identifiers.labels.addAll(source.labels);
    return _rpcPayloadResponse(
      request.id,
      'identifiers.get',
      payload.ClientGetIdentifiersResponse(value: identifiers),
    );
  }

  Future<rpc.RpcResponse> _invokeClientTool(rpc.RpcRequest request) async {
    late payload.ClientToolV0InvokeRequest invocation;
    late GeneratedMessage arguments;
    try {
      invocation = payload.ClientToolV0InvokeRequest.fromBuffer(request.payload);
      if (!clientToolNamesById.containsKey(invocation.tool.value)) {
        return _rpcErrorResponse(request.id, rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED, 'unsupported tool');
      }
      arguments = decodeClientToolRequestPayload(invocation.tool.value, invocation.payload);
      if (!_validToolArguments(arguments)) { throw const FormatException('invalid tool arguments'); }
    } catch (_) {
      return _rpcErrorResponse(request.id, rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT, 'invalid params');
    }
    handlers?.observe?.call('client.tool.v0.invoke', invocation.tool);
    final name = clientToolById(invocation.tool.value).name;
    final inner = rpc.RpcRequest(id: request.id, payload: invocation.payload);
    final handler = handlers?.tools[invocation.tool];
    final rpc.RpcResponse result;
    if (handler != null) {
      result = _rpcPayloadResponse(request.id, name, await handler(arguments));
    } else {
      result = switch (name) {
        'info.get' => await _getClientInfo(inner),
        'identifiers.get' => await _getClientIdentifiers(inner),
        'social.ping' => await _serveSocialPing(inner),
        _ => await _serveDeviceControl(inner, name),
      };
    }
    if (result.hasStatus()) return result;
    return _rpcPayloadResponse(request.id, 'client.tool.v0.invoke', payload.ClientToolV0InvokeResponse(payload: result.payload));
  }

  rpc.RpcResponse _serveRpcMethods(rpc.RpcRequest request) {
    final invalid = _validateClientRequest(request, 'client.rpc.methods.list');
    if (invalid != null) return invalid;
    final methods = <rpc.RpcMethod>[
      rpc.RpcMethod.RPC_METHOD_ALL_PING, rpc.RpcMethod.RPC_METHOD_ALL_SPEED_TEST_RUN,
      if (handlers?.deviceControl?.readMhsStates != null) rpc.RpcMethod.RPC_METHOD_CLIENT_MHS_V0_READ,
      if (handlers?.deviceControl?.writeMhsStates != null) rpc.RpcMethod.RPC_METHOD_CLIENT_MHS_V0_WRITE,
      rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_INVOKE, rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_V0_LIST,
      rpc.RpcMethod.RPC_METHOD_CLIENT_RPC_METHODS_LIST,
    ];
    return _rpcPayloadResponse(request.id, 'client.rpc.methods.list', payload.ClientRpcMethodsListResponse(methods: methods));
  }

  rpc.RpcResponse _serveToolList(rpc.RpcRequest request) {
    final invalid = _validateClientRequest(request, 'client.tool.v0.list');
    if (invalid != null) return invalid;
    final control = handlers?.deviceControl;
    final player = control?.audioplayer;
    final installed = <String, Object?>{
      'social.ping': handlers?.socialPing,
      'device.status.get': control?.status,
      'sound.play': control?.playSound,
      'device.find': control?.find,
      'device.reboot': control?.reboot,
      'device.factory_reset': control?.factoryReset,
      'run.workspace.set': control?.setRunWorkspace,
      'firmware.update': control?.updateFirmware,
      'wifi.saved.list': control?.savedWifi,
      'wifi.saved.forget': control?.forgetWifi,
      'wifi.scan': control?.scanWifi,
      'wifi.connect': control?.connectWifi,
      'audioplayer.get': player?.get,
      'audioplayer.playlist.get': player?.playlistGet,
      'audioplayer.playlist.set': player?.playlistSet,
      'audioplayer.playlist.append': player?.playlistAppend,
      'audioplayer.play': player?.play,
      'audioplayer.stop': player?.stop,
      'audioplayer.mode.set': player?.modeSet,
    };
    final tools = <payload.ClientTool>{
      if (handlers != null) payload.ClientTool.CLIENT_TOOL_INFO_GET,
      if (handlers != null) payload.ClientTool.CLIENT_TOOL_IDENTIFIERS_GET,
      for (final entry in installed.entries)
        if (entry.value != null) payload.ClientTool.valueOf(clientToolByName(entry.key).id)!,
      ...?handlers?.tools.keys,
    }.toList()..sort((a,b) => a.value.compareTo(b.value));
    return _rpcPayloadResponse(request.id, 'client.tool.v0.list', payload.ClientToolV0ListResponse(tools: tools));
  }

  Future<rpc.RpcResponse> _serveSocialPing(rpc.RpcRequest request) async {
    const methodName = 'social.ping';
    late payload.ClientSocialPingRequest params;
    try {
      params =
          _decodeProviderRequest(
                methodName,
                request.hasPayload() ? request.payload : const [],
              )
              as payload.ClientSocialPingRequest;
    } catch (_) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
        'invalid params',
      );
    }
    if (params.fromPeerPublicKey.isEmpty) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
        'invalid params',
      );
    }
    final handler = handlers?.socialPing;
    if (handler == null) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED,
        'unsupported method: $methodName',
      );
    }
    await handler(params);
    return _rpcPayloadResponse(
      request.id,
      methodName,
      payload.ClientSocialPingResponse(),
    );
  }

  Future<rpc.RpcResponse> _serveDeviceControl(
    rpc.RpcRequest request,
    String methodName,
  ) async {
    final handlers = this.handlers?.deviceControl;
    rpc.RpcResponse unsupported() => _rpcErrorResponse(
      request.id,
      rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED,
      'unsupported method: $methodName',
    );
    rpc.RpcResponse invalid() => _rpcErrorResponse(
      request.id,
      rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
      'invalid params',
    );
    GeneratedMessage? params;
    try {
      params = _decodeProviderRequest(
        methodName,
        request.hasPayload() ? request.payload : const [],
      );
    } catch (_) {
      return invalid();
    }
    switch (methodName) {
      case 'audioplayer.get':
        final handler = handlers?.audioplayer?.get;
        if (handler == null) return unsupported();
        final player = params as payload.ClientDeviceAudioPlayerGetRequest;
        return _rpcPayloadResponse(
          request.id,
          methodName,
          await handler(player),
        );
      case 'audioplayer.playlist.get':
        final handler = handlers?.audioplayer?.playlistGet;
        if (handler == null) return unsupported();
        final player =
            params as payload.ClientDeviceAudioPlayerPlaylistGetRequest;
        return _rpcPayloadResponse(
          request.id,
          methodName,
          await handler(player),
        );
      case 'audioplayer.playlist.set':
        final handler = handlers?.audioplayer?.playlistSet;
        if (handler == null) return unsupported();
        final player =
            params as payload.ClientDeviceAudioPlayerPlaylistSetRequest;
        if (!_validAudioPlayerItems(player.items, false)) return invalid();
        return _rpcPayloadResponse(
          request.id,
          methodName,
          await handler(player),
        );
      case 'audioplayer.playlist.append':
        final handler = handlers?.audioplayer?.playlistAppend;
        if (handler == null) return unsupported();
        final player =
            params as payload.ClientDeviceAudioPlayerPlaylistAppendRequest;
        if (!_validAudioPlayerItems(player.items, true)) return invalid();
        return _rpcPayloadResponse(
          request.id,
          methodName,
          await handler(player),
        );
      case 'audioplayer.play':
        final handler = handlers?.audioplayer?.play;
        if (handler == null) return unsupported();
        final player = params as payload.ClientDeviceAudioPlayerPlayRequest;
        if (!player.hasIndex() || player.index >= 32) return invalid();
        return _rpcPayloadResponse(
          request.id,
          methodName,
          await handler(player),
        );
      case 'audioplayer.stop':
        final handler = handlers?.audioplayer?.stop;
        if (handler == null) return unsupported();
        final player = params as payload.ClientDeviceAudioPlayerStopRequest;
        return _rpcPayloadResponse(
          request.id,
          methodName,
          await handler(player),
        );
      case 'audioplayer.mode.set':
        final handler = handlers?.audioplayer?.modeSet;
        if (handler == null) return unsupported();
        final player = params as payload.ClientDeviceAudioPlayerModeSetRequest;
        if (!const {'off', 'one', 'all'}.contains(player.repeat)) {
          return invalid();
        }
        return _rpcPayloadResponse(
          request.id,
          methodName,
          await handler(player),
        );
      case 'device.status.get':
        final handler = handlers?.status;
        if (handler == null) return unsupported();
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientDeviceStatusGetResponse(value: await handler()),
        );
      case 'sound.play':
        final handler = handlers?.playSound;
        if (handler == null) return unsupported();
        final sound = params as payload.ClientDeviceSoundPlayRequest;
        if (sound.sound.isEmpty ||
            utf8.encode(sound.sound).length > _deviceControlMaxBytes) {
          return invalid();
        }
        await handler(
          sound.sound,
          sound.hasDurationMs() ? sound.durationMs.toInt() : null,
        );
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientDeviceSoundPlayResponse(),
        );
      case 'device.find':
        final handler = handlers?.find;
        if (handler == null) return unsupported();
        final find = params as payload.ClientDeviceFindRequest;
        final durationMs = find.hasDurationMs()
            ? find.durationMs.toInt()
            : null;
        if (durationMs != null && durationMs < 0) return invalid();
        await handler(durationMs);
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientDeviceFindResponse(),
        );
      case 'device.reboot':
        final handler = handlers?.reboot;
        if (handler == null) return unsupported();
        final reboot = params as payload.ClientDeviceRebootRequest;
        await handler(reboot.hasDelayMs() ? reboot.delayMs.toInt() : null);
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientDeviceRebootResponse(),
        );
      case 'client.mhs.v0.read':
        final handler = handlers?.readMhsStates;
        if (handler == null) return unsupported();
        final batch = params as payload.ClientMhsV0ReadRequest;
        if (!_validMhsRefs(batch.states)) return invalid();
        final result = await handler(batch);
        if (!_validMhsStates(result.states)) {
          throw StateError('invalid MHS handler response');
        }
        return _rpcPayloadResponse(request.id, methodName, result);
      case 'client.mhs.v0.write':
        final handler = handlers?.writeMhsStates;
        if (handler == null) return unsupported();
        final batch = params as payload.ClientMhsV0WriteRequest;
        if (!_validMhsStates(batch.states)) return invalid();
        final result = await handler(batch);
        if (!_validMhsStates(result.states)) {
          throw StateError('invalid MHS handler response');
        }
        return _rpcPayloadResponse(request.id, methodName, result);
      case 'device.factory_reset':
        final handler = handlers?.factoryReset;
        if (handler == null) return unsupported();
        final reset = params as payload.ClientDeviceFactoryResetRequest;
        await handler(reset.hasKeepNetwork() && reset.keepNetwork);
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientDeviceFactoryResetResponse(),
        );
      case 'run.workspace.set':
        final handler = handlers?.setRunWorkspace;
        if (handler == null) return unsupported();
        final target = params as payload.ClientRunWorkspaceSetRequest;
        if (!_validRunWorkspaceRequest(target)) return invalid();
        await handler(target);
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientRunWorkspaceSetResponse(),
        );
      case 'firmware.update':
        final handler = handlers?.updateFirmware;
        if (handler == null) return unsupported();
        final update = params as payload.ClientFirmwareUpdateRequest;
        await handler(
          update.hasChannel() ? update.channel : null,
          update.hasSha256() ? update.sha256 : null,
        );
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientFirmwareUpdateResponse(),
        );
      case 'wifi.saved.list':
        final handler = handlers?.savedWifi;
        if (handler == null) return unsupported();
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientWifiSavedListResponse(networks: await handler()),
        );
      case 'wifi.saved.forget':
        final handler = handlers?.forgetWifi;
        if (handler == null) return unsupported();
        final forget = params as payload.ClientWifiSavedForgetRequest;
        if (forget.ssid.isEmpty ||
            utf8.encode(forget.ssid).length > _deviceControlMaxBytes) {
          return invalid();
        }
        await handler(forget.ssid);
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientWifiSavedForgetResponse(),
        );
      case 'wifi.scan':
        final handler = handlers?.scanWifi;
        if (handler == null) return unsupported();
        final scan = params as payload.ClientWifiScanRequest;
        final timeoutMs = scan.hasTimeoutMs() ? scan.timeoutMs.toInt() : null;
        if (timeoutMs != null && (timeoutMs < 1000 || timeoutMs > 15000)) {
          return invalid();
        }
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientWifiScanResponse(networks: await handler(timeoutMs)),
        );
      case 'wifi.connect':
        final handler = handlers?.connectWifi;
        if (handler == null) return unsupported();
        final connect = params as payload.ClientWifiConnectRequest;
        final ssidBytes = utf8.encode(connect.ssid).length;
        final passphrase = connect.hasPassphrase() ? connect.passphrase : null;
        final passphraseBytes = passphrase == null
            ? null
            : utf8.encode(passphrase).length;
        if (ssidBytes == 0 ||
            ssidBytes > _deviceControlMaxBytes ||
            (passphraseBytes != null &&
                (passphraseBytes < 8 || passphraseBytes > 63))) {
          return invalid();
        }
        await handler(connect.ssid, passphrase);
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientWifiConnectResponse(),
        );
      default:
        return unsupported();
    }
  }

  rpc.RpcResponse? _validateClientRequest(
    rpc.RpcRequest request,
    String methodName,
  ) {
    try {
      _decodeProviderRequest(
        methodName,
        request.hasPayload() ? request.payload : const [],
      );
      return null;
    } catch (_) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
        'invalid params',
      );
    }
  }

  rpc.RpcResponse _rpcPayloadResponse(
    String id,
    String methodName,
    GeneratedMessage response,
  ) {
    return rpc.RpcResponse(
      id: id,
      payload: clientToolsByName.containsKey(methodName)
        ? encodeClientToolResponsePayload(clientToolByName(methodName).id, response)
        : encodeRpcResponsePayload(methodName, response),
    );
  }

  void _finishPing(rpc.RpcRequest request) {
    if (!request.hasPayload()) {
      _ignoreBody = true;
      _unawaited(
        _sendEnvelopeOnly(
          _rpcErrorResponse(
            request.id,
            rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
            'missing params',
          ),
        ).catchError((_) => _close()),
      );
      return;
    }
    decodeRpcRequestPayload('all.ping', request.payload);
    _ignoreBody = true;
    _unawaited(
      _sendEnvelopeOnly(
        rpc.RpcResponse(
          id: request.id,
          payload: encodeRpcResponsePayload(
            'all.ping',
            payload.PingResponse(
              serverTime: fixnum.Int64(DateTime.now().millisecondsSinceEpoch),
            ),
          ),
        ),
      ).catchError((_) => _close()),
    );
  }

  payload.SpeedTestRequest? _validSpeedTestParams(rpc.RpcRequest request) {
    if (!request.hasPayload()) {
      return null;
    }
    final params =
        decodeRpcRequestPayload('all.speed_test.run', request.payload)
            as payload.SpeedTestRequest;
    final down = params.downContentLength.toInt();
    final up = params.upContentLength.toInt();
    if (down < 0 ||
        up < 0 ||
        down > _rpcSpeedTestMaxContentLength ||
        up > _rpcSpeedTestMaxContentLength) {
      return null;
    }
    return params;
  }

  Future<void> _sendSpeedTestResponse(
    String id,
    payload.SpeedTestRequest params,
  ) async {
    final responseEnvelope = rpc.RpcResponse(
      id: id,
      payload: encodeRpcResponsePayload(
        'all.speed_test.run',
        payload.SpeedTestResponse(
          downContentLength: params.downContentLength,
          upContentLength: params.upContentLength,
        ),
      ),
    ).writeToBuffer();
    final frames = encodeEnvelopeFrames(responseEnvelope);
    await _sendFrames(frames);
    if (responseEnvelope.length > rpcMaxFramePayloadSize) {
      await _sendFrame(encodeFrame(rpcFrameTypeEos));
    }
    final chunk = Uint8List(_rpcSpeedTestFrameSize);
    final downLength = params.downContentLength.toInt();
    for (var offset = 0; offset < downLength; offset += chunk.length) {
      final remaining = downLength - offset;
      final size = remaining < chunk.length ? remaining : chunk.length;
      await _sendFrame(encodeFrame(rpcFrameTypeBinary, chunk.sublist(0, size)));
    }
    await _sendFrame(encodeFrame(rpcFrameTypeEos));
  }

  Future<void> _sendEnvelopeOnly(rpc.RpcResponse response) async {
    await _sendFrames(encodeEnvelopeFrames(response.writeToBuffer()));
    await _sendFrame(encodeFrame(rpcFrameTypeEos));
  }

  Future<void> _sendFrames(List<Uint8List> frames) async {
    for (final frame in frames) {
      await _sendFrame(frame);
    }
  }

  Future<void> _sendFrame(Uint8List frame) async {
    if (channel.state != GizClawDataChannelState.open) {
      throw StateError('RPC data channel is ${channel.state}, want open');
    }
    await channel.send(frame);
  }

  rpc.RpcResponse _rpcErrorResponse(
    String id,
    rpc.StatusCode code,
    String message,
  ) {
    return rpc.RpcResponse(
      id: id,
      status: rpc.RpcStatus(code: code, message: message),
    );
  }

  String _methodName(rpc.RpcRequest request) {
    return rpcMethodNamesById[request.method.value] ??
        'unknown:${request.method.value}';
  }

  bool _isClientMethod(String methodName) {
    return const {'client.tool.v0.invoke', 'client.tool.v0.list', 'client.rpc.methods.list', 'client.mhs.v0.read', 'client.mhs.v0.write'}.contains(methodName);
  }

  void _close() {
    if (_closed) {
      return;
    }
    _closed = true;
    _unawaited(_messages.cancel());
    _unawaited(_states.cancel());
    _unawaited(channel.close());
  }
}

void _unawaited(Future<void> future) {}

bool _validAudioPlayerItems(List<payload.AudioPlayerItem> items, bool append) {
  if (items.length > 32 || (append && items.isEmpty)) return false;
  return items.every((item) {
    final uri = Uri.tryParse(item.url);
    if (utf8.encode(item.url).length > 1024 ||
        uri == null ||
        uri.scheme != 'https' ||
        uri.host.isEmpty ||
        uri.userInfo.isNotEmpty ||
        uri.hasFragment) {
      return false;
    }
    return utf8.encode(item.title).length <= 128 &&
        utf8.encode(item.sourceRef).length <= 128;
  });
}

// Mirrors the ClientRunWorkspaceSetRequest name bound in
// api/proto/rpc/nanopb.options.
const _runWorkspaceTargetMaxBytes = 256;

/// Accepts a non-empty `workspaceName` within the nanopb bound.
bool _validRunWorkspaceRequest(payload.ClientRunWorkspaceSetRequest request) {
  return request.workspaceName.isNotEmpty &&
      utf8.encode(request.workspaceName).length <= _runWorkspaceTargetMaxBytes;
}

final _mhsKeyPattern = RegExp(r'^[a-z][a-z0-9]*([.-][a-z0-9]+)*$');
bool _validMhsKey(String value) =>
    value.length <= 64 &&
    _mhsKeyPattern.matchAsPrefix(value)?.end == value.length;
bool _validMhsRefs(List<payload.MhsStateRef> states) {
  if (states.isEmpty || states.length > 32) return false;
  final keys = <String>{};
  for (final state in states) {
    if (!_validMhsKey(state.deviceId) ||
        !_validMhsKey(state.state) ||
        !keys.add('${state.deviceId}/${state.state}')) {
      return false;
    }
  }
  return true;
}

bool _validMhsString(String text) {
  final bytes = utf8.encode(text);
  return !text.contains('\u0000') &&
      bytes.length <= 256 &&
      utf8.decode(bytes) == text;
}

bool _validMhsStates(List<payload.MhsStateValue> states) {
  if (!_validMhsRefs([
    for (final state in states)
      payload.MhsStateRef(deviceId: state.deviceId, state: state.state),
  ])) {
    return false;
  }
  for (final state in states) {
    if (!state.hasValue()) return false;
    final value = state.value;
    final valid = switch (value.whichValue()) {
      payload.MhsValue_Value.boolValue => true,
      payload.MhsValue_Value.intValue =>
        value.intValue.toInt() >= -9007199254740991 &&
            value.intValue.toInt() <= 9007199254740991,
      payload.MhsValue_Value.doubleValue => value.doubleValue.isFinite,
      payload.MhsValue_Value.stringValue => _validMhsString(value.stringValue),
      payload.MhsValue_Value.notSet => false,
    };
    if (!valid) return false;
  }
  return true;
}

GeneratedMessage _decodeProviderRequest(String name, List<int> bytes) => clientToolsByName.containsKey(name) ? decodeClientToolRequestPayload(clientToolByName(name).id, bytes) : decodeRpcRequestPayload(name, bytes);

bool _validToolArguments(GeneratedMessage request) {
  bool text(String value, int max) => value.isNotEmpty && utf8.encode(value).length <= max && !value.contains('\u0000');
  return switch (request) {
    payload.ClientDeviceSoundPlayRequest r => text(r.sound, 32) && (!r.hasDurationMs() || r.durationMs >= 0),
    payload.ClientDeviceFindRequest r => !r.hasDurationMs() || r.durationMs >= 0,
    payload.ClientDeviceRebootRequest r => !r.hasDelayMs() || r.delayMs >= 0,
    payload.ClientWifiConnectRequest r => text(r.ssid, 32) && (!r.hasPassphrase() || (text(r.passphrase, 63) && utf8.encode(r.passphrase).length >= 8)),
    payload.ClientWifiSavedForgetRequest r => text(r.ssid, 32),
    payload.ClientWifiScanRequest r => !r.hasTimeoutMs() || (r.timeoutMs >= 1000 && r.timeoutMs <= 15000),
    payload.ClientFirmwareUpdateRequest r => (!r.hasSha256() || RegExp(r'^[a-fA-F0-9]{64}$').hasMatch(r.sha256)) && (!r.hasChannel() || r.channel.value >= 1 && r.channel.value <= 3),
    payload.ClientDeviceAudioPlayerPlaylistSetRequest r => _validAudioPlayerItems(r.items, false),
    payload.ClientDeviceAudioPlayerPlaylistAppendRequest r => _validAudioPlayerItems(r.items, true),
    payload.ClientDeviceAudioPlayerPlayRequest r => r.index >= 0 && r.index < 32,
    payload.ClientDeviceAudioPlayerModeSetRequest r => const ['off', 'one', 'all'].contains(r.repeat),
    payload.ClientRunWorkspaceSetRequest r => _validRunWorkspaceRequest(r),
    payload.ClientSocialPingRequest r => text(r.fromPeerPublicKey, 128),
    _ => true,
  };
}
