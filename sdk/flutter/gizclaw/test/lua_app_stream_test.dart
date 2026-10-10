import 'dart:async';
import 'dart:typed_data';

import 'package:cryptography/dart.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
import 'package:test/test.dart';

import 'fake_transport.dart';

class _Sink implements LuaAppInstallSession {
  var bytes = 0;
  var chunks = 0;
  var finishes = 0;
  var closes = 0;
  @override
  void write(Uint8List chunk) {
    bytes += chunk.length;
    chunks++;
  }

  @override
  ClientLuaAppInstallResponse finish() {
    finishes++;
    return ClientLuaAppInstallResponse(
      app: LuaAppInfo(appId: 'demo', version: '1.0.0'),
    );
  }

  @override
  void close(Object? error) {
    closes++;
  }
}

Future<void> _waitFor(bool Function() ready) async {
  for (var i = 0; i < 100 && !ready(); i++) {
    await Future<void>.delayed(Duration.zero);
  }
  expect(ready(), isTrue);
}

void main() {
  test(
    'Binary caller rejects 512 KiB plus one before opening transport',
    () async {
      final factory = FakeDataChannelFactory();
      final client = PeerRpcClient(factory);
      await expectLater(
        client.installLuaApp(
          ClientLuaAppInstallStreamRequest(
            contentLength: 524289,
            sha256: 'a' * 64,
          ),
          const Stream<Uint8List>.empty(),
        ),
        throwsArgumentError,
      );
      expect(factory.channels, isEmpty);
    },
  );

  test('Binary provider verifies exact length and SHA before finish', () async {
    final bytes = Uint8List(524288);
    final hash = (const DartSha256())
        .hashSync(bytes)
        .bytes
        .map((v) => v.toRadixString(16).padLeft(2, '0'))
        .join();
    for (final variant in ['valid', 'short', 'long', 'sha', 'unsupported']) {
      final channel = FakeDataChannel('giznet/v1/service/0');
      addTearDown(channel.close);
      final sink = _Sink();
      serveGizClawPeerRpcChannel(
        channel,
        handlers: GizClawPeerRpcHandlers(
          deviceInfo: () => DeviceInfo(name: 'test'),
          installLuaApp: variant == 'unsupported' ? null : (_, _) => sink,
        ),
      );
      final envelope = encodeRpcRequest(
        'client.lua.app.install',
        ClientLuaAppInstallStreamRequest(
          contentLength: bytes.length,
          sha256: variant == 'sha' ? '0' * 64 : hash,
        ),
        id: 'install',
      );
      channel.addMessage(envelope.sublist(0, envelope.length - 4));
      final length = variant == 'short'
          ? bytes.length - 1
          : variant == 'long'
          ? bytes.length + 1
          : bytes.length;
      for (var offset = 0; offset < length; offset += 8192) {
        final count = (length - offset).clamp(0, 8192);
        channel.addMessage(encodeFrame(rpcFrameTypeBinary, Uint8List(count)));
      }
      channel.addMessage(encodeFrame(rpcFrameTypeEos));
      await _waitFor(() => channel.sent.length >= 2);
      final frames = decodeFrames(concatBytes(channel.sent));
      final result = rpc.RpcResponse.fromBuffer(frames.first.payload);
      expect(sink.finishes, variant == 'valid' ? 1 : 0);
      expect(sink.closes, variant == 'unsupported' ? 0 : 1);
      if (variant == 'valid') {
        expect(result.hasStatus(), isFalse);
        expect(sink.bytes, bytes.length);
        expect(sink.chunks, greaterThan(2));
      } else {
        expect(
          result.status.code,
          variant == 'unsupported'
              ? rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED
              : rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
        );
      }
    }
  });

  test(
    'Binary caller splits stream chunks and returns the installed app',
    () async {
      final factory = FakeDataChannelFactory();
      final client = PeerRpcClient(factory, createId: () => 'upload');
      final future = client.installLuaApp(
        ClientLuaAppInstallStreamRequest(
          contentLength: 164484,
          sha256: 'a' * 64,
        ),
        Stream.value(Uint8List(164484)),
      );
      await _waitFor(
        () =>
            factory.channels.isNotEmpty &&
            factory.channels.single.sent.length >= 5,
      );
      final channel = factory.channels.single;
      final frames = decodeFrames(concatBytes(channel.sent));
      expect(frames.map((f) => f.type).toList(), [2, 2, 2, 2, 0]);
      expect(frames.skip(1).take(3).map((f) => f.payload.length).toList(), [
        65535,
        65535,
        33414,
      ]);
      final response = rpc.RpcResponse(
        id: 'upload',
        payload: encodeRpcResponsePayload(
          'client.lua.app.install',
          ClientLuaAppInstallResponse(
            app: LuaAppInfo(appId: 'demo', version: '1.0.0'),
          ),
        ),
      );
      channel.addMessage(
        concatBytes([
          ...encodeEnvelopeFrames(response.writeToBuffer()),
          encodeFrame(rpcFrameTypeEos),
        ]),
      );
      expect((await future).app.appId, 'demo');
    },
  );
}
