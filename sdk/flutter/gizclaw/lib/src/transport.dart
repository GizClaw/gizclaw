import 'dart:async';
import 'dart:typed_data';

const giznetWebRtcPacketDataChannelLabel = 'giznet/v1/packet';
const giznetWebRtcServiceDataChannelPrefix = 'giznet/v1/service/';
const giznetWebRtcSignalingPath = '/webrtc/v1/offer';

const servicePeerRpc = 0x00;
const servicePeerHttp = 0x01;
const servicePeerOpenAi = 0x02;
const serviceAdminHttp = 0x10;
const serviceAgentEvent = 0x20;
const serviceEdgeRpc = 0x31;

final giznetWebRtcEventDataChannelLabel = giznetServiceDataChannelLabel(
  serviceAgentEvent,
);

String giznetServiceDataChannelLabel(int service) {
  if (service < 0) {
    throw ArgumentError.value(service, 'service', 'invalid service id');
  }
  return '$giznetWebRtcServiceDataChannelPrefix$service';
}

enum GizClawDataChannelState { connecting, open, closing, closed }

class GizClawDataChannelOptions {
  const GizClawDataChannelOptions({this.maxRetransmits, this.ordered = true});

  final int? maxRetransmits;
  final bool ordered;
}

abstract interface class GizClawDataChannel {
  int? get bufferedAmount;
  String get label;
  Stream<Uint8List> get messages;
  Stream<GizClawDataChannelState> get states;
  GizClawDataChannelState get state;

  Future<void> close();
  Future<void> send(Uint8List bytes);
}

abstract interface class GizClawDataChannelFactory {
  Future<GizClawDataChannel> createDataChannel(
    String label, {
    GizClawDataChannelOptions options = const GizClawDataChannelOptions(),
  });
}
