import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/connection/gizclaw_connection_controller.dart';

void main() {
  test('offers one disabled microphone track before connecting', () async {
    final track = _FakeTrack('mic-1');
    final stream = _FakeStream('stream-1', [track]);
    rtc.MediaStream? offeredStream;
    final controller = _controller(
      acquire: () async => stream,
      connect:
          ({
            required localAudioStream,
            required peerRpcHandlers,
            required prepareOffer,
            required sendOffer,
          }) async {
            offeredStream = localAudioStream;
            return _FakePeerConnection();
          },
    );
    addTearDown(controller.close);
    final statuses = <MicrophoneStatus>[];
    controller.addListener(() => statuses.add(controller.microphoneStatus));

    await controller.connect();

    expect(offeredStream, same(stream));
    expect(track.enabled, isFalse);
    expect(controller.microphoneTrack, same(track));
    expect(controller.microphoneStatus, const MicrophoneStatus.ready());
    expect(statuses, [
      const MicrophoneStatus.recovering(),
      const MicrophoneStatus.ready(),
    ]);
  });

  test(
    'generic capture failure is reported without failing connection',
    () async {
      final controller = _controller(
        acquire: () => Future.error(StateError('No audio input device')),
        connect:
            ({
              required localAudioStream,
              required peerRpcHandlers,
              required prepareOffer,
              required sendOffer,
            }) async => _FakePeerConnection(),
      );
      addTearDown(controller.close);

      await controller.connect();

      expect(controller.isConnected, isTrue);
      expect(
        controller.microphoneStatus,
        const MicrophoneStatus.unavailable(
          failureKind: MicrophoneFailureKind.captureUnavailable,
        ),
      );
    },
  );

  test('permission denial keeps receive-only connection alive', () async {
    rtc.MediaStream? offeredStream;
    final controller = _controller(
      acquire: () => Future.error(StateError('NotAllowedError')),
      connect:
          ({
            required localAudioStream,
            required peerRpcHandlers,
            required prepareOffer,
            required sendOffer,
          }) async {
            offeredStream = localAudioStream;
            return _FakePeerConnection();
          },
    );
    addTearDown(controller.close);

    final client = await controller.connect();

    expect(client, same(controller.client));
    expect(offeredStream, isNull);
    expect(controller.isConnected, isTrue);
    expect(
      controller.microphoneStatus,
      const MicrophoneStatus.unavailable(
        failureKind: MicrophoneFailureKind.permissionDenied,
      ),
    );
  });

  test('signaling failure stops the pending microphone stream', () async {
    final track = _FakeTrack('mic-1');
    final controller = _controller(
      acquire: () async => _FakeStream('stream-1', [track]),
      connect:
          ({
            required localAudioStream,
            required peerRpcHandlers,
            required prepareOffer,
            required sendOffer,
          }) => Future.error(StateError('signaling failed')),
    );
    addTearDown(controller.close);

    await expectLater(controller.connect(), throwsStateError);

    expect(track.stopCalls, 1);
    expect(controller.microphoneTrack, isNull);
  });

  test('profile change during capture stops the stale stream', () async {
    final captureStarted = Completer<void>();
    final capture = Completer<rtc.MediaStream>();
    final track = _FakeTrack('mic-1');
    var connectCalls = 0;
    final controller = _controller(
      acquire: () {
        captureStarted.complete();
        return capture.future;
      },
      connect:
          ({
            required localAudioStream,
            required peerRpcHandlers,
            required prepareOffer,
            required sendOffer,
          }) async {
            connectCalls += 1;
            return _FakePeerConnection();
          },
    );
    addTearDown(controller.close);

    final connecting = controller.connect();
    await captureStarted.future;
    await controller.updateProfile(
      controller.profile.copyWith(endpoint: 'new.gizclaw.test:9820'),
    );
    capture.complete(_FakeStream('stream-1', [track]));

    await expectLater(connecting, throwsStateError);
    expect(connectCalls, 0);
    expect(track.stopCalls, 1);
  });

  test('reconnect captures a fresh track and close stops each once', () async {
    final tracks = <_FakeTrack>[];
    var captures = 0;
    final controller = _controller(
      acquire: () async {
        final track = _FakeTrack('mic-${++captures}');
        tracks.add(track);
        return _FakeStream('stream-$captures', [track]);
      },
      connect:
          ({
            required localAudioStream,
            required peerRpcHandlers,
            required prepareOffer,
            required sendOffer,
          }) async => _FakePeerConnection(),
    );

    await controller.connect();
    await controller.reconnect();
    await controller.close();
    await controller.close();

    expect(tracks, hasLength(2));
    expect(tracks.map((track) => track.stopCalls), [1, 1]);
  });
}

GizClawConnectionController _controller({
  required AcquireMicrophoneStream acquire,
  required ConnectGizClawWebRtc connect,
}) {
  const key = '11111111111111111111111111111111';
  return GizClawConnectionController(
    const GizClawConnectionProfile(
      endpoint: 'gizclaw.test:9820',
      clientPrivateKey: key,
    ),
    acquireMicrophoneStream: acquire,
    connectWebRtc: connect,
    fetchServerInfo: (_) async => const GiznetServerInfo(publicKey: key),
    publishClientInfo: (_, _) async {},
  );
}

class _FakePeerConnection extends Fake implements rtc.RTCPeerConnection {
  int closeCalls = 0;

  @override
  rtc.RTCPeerConnectionState? get connectionState =>
      rtc.RTCPeerConnectionState.RTCPeerConnectionStateConnected;

  @override
  Future<List<rtc.RTCRtpReceiver>> getReceivers() async => const [];

  @override
  Future<void> close() async {
    closeCalls += 1;
  }
}

class _FakeStream extends Fake implements rtc.MediaStream {
  _FakeStream(this.id, this.tracks);

  @override
  final String id;
  final List<rtc.MediaStreamTrack> tracks;

  @override
  List<rtc.MediaStreamTrack> getAudioTracks() => List.unmodifiable(tracks);

  @override
  List<rtc.MediaStreamTrack> getTracks() => List.unmodifiable(tracks);
}

class _FakeTrack extends Fake implements rtc.MediaStreamTrack {
  _FakeTrack(this.id);

  @override
  final String id;

  @override
  String get kind => 'audio';

  @override
  bool enabled = true;

  @override
  void Function()? onEnded;

  int stopCalls = 0;

  @override
  Future<void> stop() async {
    stopCalls += 1;
  }
}
