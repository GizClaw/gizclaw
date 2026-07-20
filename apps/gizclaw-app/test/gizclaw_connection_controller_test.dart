import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/connection/gizclaw_connection_controller.dart';

void main() {
  test('registers before publishing client info', () async {
    final events = <String>[];
    final controller = _controller(
      registrationToken: 'registration-secret',
      registerServer: (_, token) async => events.add('register:$token'),
      publishClientInfo: (_, _) async => events.add('publish'),
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

    expect(events, ['register:registration-secret', 'publish']);
  });

  test('enables remote audio before preparing the output route', () async {
    final remoteTrack = _FakeTrack('remote-1')..enabled = false;
    final peerConnection = _FakePeerConnection(
      receivers: [_FakeReceiver(remoteTrack)],
    );
    var routePrepared = false;
    final controller = _controller(
      acquire: () => Future.error(StateError('No audio input device')),
      connect:
          ({
            required localAudioStream,
            required peerRpcHandlers,
            required prepareOffer,
            required sendOffer,
          }) async => peerConnection,
      prepareAudioOutput: () async {
        expect(remoteTrack.enabled, isTrue);
        routePrepared = true;
      },
    );
    addTearDown(controller.close);

    await controller.connect();

    expect(routePrepared, isTrue);
  });

  test('output routing failure cleans up the pending connection', () async {
    final peerConnection = _FakePeerConnection();
    final controller = _controller(
      acquire: () => Future.error(StateError('No audio input device')),
      connect:
          ({
            required localAudioStream,
            required peerRpcHandlers,
            required prepareOffer,
            required sendOffer,
          }) async => peerConnection,
      prepareAudioOutput: () => Future.error(StateError('routing failed')),
    );

    await expectLater(controller.connect(), throwsStateError);

    expect(controller.peerConnection, isNull);
    expect(peerConnection.closeCalls, 1);
    expect(peerConnection.disposeCalls, 1);
  });

  test(
    'offers a disabled track and gates both the track and RTP sender',
    () async {
      final track = _FakeTrack('mic-1');
      final stream = _FakeStream('stream-1', [track]);
      rtc.MediaStream? offeredStream;
      bool? enabledWhenOffered;
      final sendingStates = <bool>[];
      final routeTrackStates = <bool>[];
      final controller = _controller(
        acquire: () async => stream,
        configureMicrophoneSending: (_, _) async => (active) async {
          sendingStates.add(active);
        },
        connect:
            ({
              required localAudioStream,
              required peerRpcHandlers,
              required prepareOffer,
              required sendOffer,
            }) async {
              offeredStream = localAudioStream;
              enabledWhenOffered = localAudioStream
                  ?.getAudioTracks()
                  .single
                  .enabled;
              return _FakePeerConnection();
            },
        prepareAudioOutput: () async {
          routeTrackStates.add(track.enabled);
        },
      );
      addTearDown(controller.close);
      final statuses = <MicrophoneStatus>[];
      controller.addListener(() => statuses.add(controller.microphoneStatus));

      await controller.connect();

      expect(offeredStream, same(stream));
      expect(enabledWhenOffered, isFalse);
      expect(track.enabled, isFalse);
      expect(sendingStates, [false]);
      expect(routeTrackStates, [false]);
      expect(controller.microphoneTrack, same(track));
      expect(controller.microphoneStatus, const MicrophoneStatus.ready());
      expect(statuses, [
        const MicrophoneStatus.recovering(),
        const MicrophoneStatus.ready(),
      ]);

      await controller.setMicrophoneSending(true);
      expect(track.enabled, isTrue);
      await controller.setMicrophoneSending(false);
      expect(track.enabled, isFalse);
      expect(sendingStates, [false, true, false]);
      expect(routeTrackStates, [false, true, false]);
    },
  );

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

  test('sender setup failure keeps a receive-only connection alive', () async {
    final track = _FakeTrack('mic-1');
    final stream = _FakeStream('stream-1', [track]);
    final controller = _controller(
      acquire: () async => stream,
      configureMicrophoneSending: (_, _) async {
        throw StateError('sender missing');
      },
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
    expect(controller.microphoneTrack, isNull);
    expect(track.stopCalls, 0);
    expect(stream.disposeCalls, 0);
    expect(
      controller.microphoneStatus,
      const MicrophoneStatus.unavailable(
        failureKind: MicrophoneFailureKind.captureUnavailable,
      ),
    );

    await controller.close();
    expect(track.stopCalls, 1);
    expect(stream.disposeCalls, 1);
  });

  test('sender runtime failure marks the microphone unavailable', () async {
    final track = _FakeTrack('mic-1');
    final controller = _controller(
      acquire: () async => _FakeStream('stream-1', [track]),
      configureMicrophoneSending: (_, _) async => (active) async {
        if (active) throw StateError('sender failed');
      },
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

    await expectLater(controller.setMicrophoneSending(true), throwsStateError);

    expect(track.enabled, isFalse);
    expect(
      controller.microphoneStatus,
      const MicrophoneStatus.unavailable(
        failureKind: MicrophoneFailureKind.captureUnavailable,
      ),
    );
  });

  test('notifies when the native peer connection leaves connected', () async {
    final peerConnection = _FakePeerConnection();
    final controller = _controller(
      acquire: () => Future.error(StateError('No audio input device')),
      connect:
          ({
            required localAudioStream,
            required peerRpcHandlers,
            required prepareOffer,
            required sendOffer,
          }) async => peerConnection,
    );
    addTearDown(controller.close);
    var notifications = 0;
    controller.addListener(() => notifications += 1);
    await controller.connect();
    final connectedNotifications = notifications;

    peerConnection.updateConnectionState(
      rtc.RTCPeerConnectionState.RTCPeerConnectionStateDisconnected,
    );

    expect(controller.isConnected, isFalse);
    expect(notifications, connectedNotifications + 1);
  });

  test('signaling failure stops the pending microphone stream', () async {
    final track = _FakeTrack('mic-1');
    final stream = _FakeStream('stream-1', [track]);
    final controller = _controller(
      acquire: () async => stream,
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
    expect(stream.disposeCalls, 1);
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
    final stream = _FakeStream('stream-1', [track]);
    capture.complete(stream);

    await expectLater(connecting, throwsStateError);
    expect(connectCalls, 0);
    expect(track.stopCalls, 1);
    expect(stream.disposeCalls, 1);
  });

  test('close cancels capture that is still in flight', () async {
    final captureStarted = Completer<void>();
    final capture = Completer<rtc.MediaStream>();
    final track = _FakeTrack('mic-1');
    final stream = _FakeStream('stream-1', [track]);
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

    final connecting = controller.connect();
    await captureStarted.future;
    await controller.close();
    capture.complete(stream);

    await expectLater(connecting, throwsStateError);
    expect(connectCalls, 0);
    expect(track.stopCalls, 1);
    expect(stream.disposeCalls, 1);
  });

  test('reconnect disposes each stream and peer connection once', () async {
    final tracks = <_FakeTrack>[];
    final streams = <_FakeStream>[];
    final peerConnections = <_FakePeerConnection>[];
    var captures = 0;
    final controller = _controller(
      acquire: () async {
        final track = _FakeTrack('mic-${++captures}');
        final stream = _FakeStream('stream-$captures', [track]);
        tracks.add(track);
        streams.add(stream);
        return stream;
      },
      connect:
          ({
            required localAudioStream,
            required peerRpcHandlers,
            required prepareOffer,
            required sendOffer,
          }) async {
            final peerConnection = _FakePeerConnection();
            peerConnections.add(peerConnection);
            return peerConnection;
          },
    );

    await controller.connect();
    await controller.reconnect();
    await controller.close();
    await controller.close();

    expect(tracks, hasLength(2));
    expect(tracks.map((track) => track.stopCalls), [1, 1]);
    expect(streams.map((stream) => stream.disposeCalls), [1, 1]);
    expect(peerConnections.map((connection) => connection.closeCalls), [1, 1]);
    expect(peerConnections.map((connection) => connection.disposeCalls), [
      1,
      1,
    ]);
  });

  test('close disposes all native resources after cleanup errors', () async {
    final track = _FakeTrack('mic-1', failStop: true);
    final stream = _FakeStream('stream-1', [track]);
    final peerConnection = _FakePeerConnection(failClose: true);
    final controller = _controller(
      acquire: () async => stream,
      connect:
          ({
            required localAudioStream,
            required peerRpcHandlers,
            required prepareOffer,
            required sendOffer,
          }) async => peerConnection,
    );
    await controller.connect();

    await expectLater(controller.close(), throwsStateError);

    expect(track.stopCalls, 1);
    expect(stream.disposeCalls, 1);
    expect(peerConnection.closeCalls, 1);
    expect(peerConnection.disposeCalls, 1);
  });

  test('close releases local capture before the peer connection', () async {
    final operations = <String>[];
    final track = _FakeTrack('mic-1', operations: operations);
    final stream = _FakeStream('stream-1', [track], operations: operations);
    final peerConnection = _FakePeerConnection(operations: operations);
    final controller = _controller(
      acquire: () async => stream,
      connect:
          ({
            required localAudioStream,
            required peerRpcHandlers,
            required prepareOffer,
            required sendOffer,
          }) async => peerConnection,
    );
    await controller.connect();

    await controller.close();

    expect(operations, [
      'track.stop',
      'stream.dispose',
      'peerConnection.close',
      'peerConnection.dispose',
    ]);
  });

  test('connect waits for an in-flight native cleanup', () async {
    final disposeStarted = Completer<void>();
    final allowDispose = Completer<void>();
    var captures = 0;
    var connections = 0;
    final controller = _controller(
      acquire: () async {
        captures += 1;
        return _FakeStream(
          'stream-$captures',
          [_FakeTrack('mic-$captures')],
          disposeStarted: captures == 1 ? disposeStarted : null,
          allowDispose: captures == 1 ? allowDispose.future : null,
        );
      },
      connect:
          ({
            required localAudioStream,
            required peerRpcHandlers,
            required prepareOffer,
            required sendOffer,
          }) async {
            connections += 1;
            return _FakePeerConnection();
          },
    );
    await controller.connect();

    final closing = controller.close();
    await disposeStarted.future;
    final connecting = controller.connect();
    await Future<void>.delayed(Duration.zero);

    expect(captures, 1);
    expect(connections, 1);

    allowDispose.complete();
    await closing;
    await connecting;

    expect(captures, 2);
    expect(connections, 2);
    await controller.close();
  });
}

GizClawConnectionController _controller({
  required AcquireMicrophoneStream acquire,
  required ConnectGizClawWebRtc connect,
  ConfigureMicrophoneSending? configureMicrophoneSending,
  PrepareAudioOutput? prepareAudioOutput,
  String registrationToken = '',
  RegisterGizClawServer? registerServer,
  PublishGizClawClientInfo? publishClientInfo,
}) {
  const key = '11111111111111111111111111111111';
  return GizClawConnectionController(
    GizClawConnectionProfile(
      endpoint: 'gizclaw.test:9820',
      clientPrivateKey: key,
      registrationToken: registrationToken,
    ),
    acquireMicrophoneStream: acquire,
    configureMicrophoneSending:
        configureMicrophoneSending ?? (_, _) async => (_) async {},
    connectWebRtc: connect,
    fetchServerInfo: (_) async => const GiznetServerInfo(publicKey: key),
    prepareAudioOutput: prepareAudioOutput ?? () async {},
    publishClientInfo: publishClientInfo ?? (_, _) async {},
    registerServer: registerServer,
  );
}

class _FakePeerConnection extends Fake implements rtc.RTCPeerConnection {
  _FakePeerConnection({
    this.failClose = false,
    this.operations,
    List<rtc.RTCRtpReceiver> receivers = const [],
  }) : _receivers = receivers;

  final bool failClose;
  final List<String>? operations;
  final List<rtc.RTCRtpReceiver> _receivers;
  int closeCalls = 0;
  int disposeCalls = 0;
  rtc.RTCPeerConnectionState? _connectionState =
      rtc.RTCPeerConnectionState.RTCPeerConnectionStateConnected;
  void Function(rtc.RTCPeerConnectionState)? _onConnectionState;

  @override
  rtc.RTCPeerConnectionState? get connectionState => _connectionState;

  @override
  void Function(rtc.RTCPeerConnectionState)? get onConnectionState =>
      _onConnectionState;

  @override
  set onConnectionState(void Function(rtc.RTCPeerConnectionState)? callback) {
    _onConnectionState = callback;
  }

  void updateConnectionState(rtc.RTCPeerConnectionState state) {
    _connectionState = state;
    _onConnectionState?.call(state);
  }

  @override
  Future<List<rtc.RTCRtpReceiver>> getReceivers() async => _receivers;

  @override
  Future<void> close() async {
    closeCalls += 1;
    operations?.add('peerConnection.close');
    if (failClose) throw StateError('close failed');
  }

  @override
  Future<void> dispose() async {
    disposeCalls += 1;
    operations?.add('peerConnection.dispose');
  }
}

class _FakeReceiver extends Fake implements rtc.RTCRtpReceiver {
  _FakeReceiver(this.track);

  @override
  final rtc.MediaStreamTrack track;
}

class _FakeStream extends Fake implements rtc.MediaStream {
  _FakeStream(
    this.id,
    this.tracks, {
    this.operations,
    this.disposeStarted,
    this.allowDispose,
  });

  @override
  final String id;
  final List<rtc.MediaStreamTrack> tracks;
  final List<String>? operations;
  final Completer<void>? disposeStarted;
  final Future<void>? allowDispose;
  int disposeCalls = 0;

  @override
  List<rtc.MediaStreamTrack> getAudioTracks() => List.unmodifiable(tracks);

  @override
  List<rtc.MediaStreamTrack> getTracks() => List.unmodifiable(tracks);

  @override
  Future<void> dispose() async {
    disposeCalls += 1;
    operations?.add('stream.dispose');
    disposeStarted?.complete();
    await allowDispose;
  }
}

class _FakeTrack extends Fake implements rtc.MediaStreamTrack {
  _FakeTrack(this.id, {this.failStop = false, this.operations});

  @override
  final String id;
  final bool failStop;
  final List<String>? operations;

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
    operations?.add('track.stop');
    if (failStop) throw StateError('stop failed');
  }
}
