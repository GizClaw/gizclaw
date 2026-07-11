import 'dart:async';

import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:gizclaw/gizclaw.dart';
import 'package:test/test.dart';

void main() {
  test('treats a newly created native data channel as connecting', () async {
    final native = _FakeRtcDataChannel();
    final channel = FlutterWebRtcDataChannel(native);

    expect(channel.state, GizClawDataChannelState.connecting);

    final states = <GizClawDataChannelState>[];
    final subscription = channel.states.listen(states.add);
    native.emitState(rtc.RTCDataChannelState.RTCDataChannelOpen);
    await Future<void>.delayed(Duration.zero);
    expect(states, [GizClawDataChannelState.open]);

    native.emitState(rtc.RTCDataChannelState.RTCDataChannelClosed);
    await subscription.asFuture<void>();
    expect(states.last, GizClawDataChannelState.closed);
  });
}

class _FakeRtcDataChannel extends rtc.RTCDataChannel {
  _FakeRtcDataChannel() {
    stateChangeStream = const Stream.empty();
    messageStream = const Stream.empty();
  }

  rtc.RTCDataChannelState? _state;

  void emitState(rtc.RTCDataChannelState state) {
    _state = state;
    onDataChannelState?.call(state);
  }

  @override
  int? get bufferedAmount => 0;

  @override
  Future<void> close() async {}

  @override
  int? get id => 1;

  @override
  String? get label => 'test';

  @override
  Future<void> send(rtc.RTCDataChannelMessage message) async {}

  @override
  rtc.RTCDataChannelState? get state => _state;
}
