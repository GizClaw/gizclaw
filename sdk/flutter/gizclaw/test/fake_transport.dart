import 'dart:async';
import 'dart:typed_data';

import 'package:gizclaw/src/transport.dart';

class FakeDataChannelFactory implements GizClawDataChannelFactory {
  FakeDataChannelFactory({
    this.createGate,
    this.initialState = GizClawDataChannelState.open,
    this.sendGate,
  });

  final Future<void>? createGate;
  final channels = <FakeDataChannel>[];
  final GizClawDataChannelState initialState;
  final Future<void>? sendGate;

  @override
  Future<GizClawDataChannel> createDataChannel(
    String label, {
    GizClawDataChannelOptions options = const GizClawDataChannelOptions(),
  }) async {
    final gate = createGate;
    if (gate != null) {
      await gate;
    }
    final channel = FakeDataChannel(
      label,
      initialState: initialState,
      sendGate: sendGate,
    );
    channels.add(channel);
    return channel;
  }
}

class FakeDataChannel implements GizClawDataChannel {
  FakeDataChannel(
    this.label, {
    GizClawDataChannelState initialState = GizClawDataChannelState.open,
    this.sendGate,
  }) : _state = initialState {
    if (initialState == GizClawDataChannelState.closed) {
      _closeStreams();
    }
  }

  final sent = <Uint8List>[];
  final Future<void>? sendGate;
  final _messages = StreamController<Uint8List>.broadcast();
  final _states = StreamController<GizClawDataChannelState>.broadcast();
  GizClawDataChannelState _state;
  var _streamsClosed = false;

  void addMessage(List<int> bytes) {
    _messages.add(Uint8List.fromList(bytes));
  }

  void setState(GizClawDataChannelState state) {
    _state = state;
    _states.add(state);
    if (state == GizClawDataChannelState.closed) {
      _closeStreams();
    }
  }

  @override
  int? get bufferedAmount => null;

  @override
  final String label;

  @override
  Stream<Uint8List> get messages => _messages.stream;

  @override
  GizClawDataChannelState get state => _state;

  @override
  Stream<GizClawDataChannelState> get states => _states.stream;

  @override
  Future<void> close() async {
    if (_state != GizClawDataChannelState.closed) {
      setState(GizClawDataChannelState.closed);
    } else {
      _closeStreams();
    }
  }

  @override
  Future<void> send(Uint8List bytes) async {
    final gate = sendGate;
    if (gate != null) {
      await gate;
    }
    if (_state == GizClawDataChannelState.closed) {
      throw StateError('data channel is closed');
    }
    sent.add(bytes);
  }

  void _closeStreams() {
    if (_streamsClosed) {
      return;
    }
    _streamsClosed = true;
    _messages.close();
    _states.close();
  }
}
