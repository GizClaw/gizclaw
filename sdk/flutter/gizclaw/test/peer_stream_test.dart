import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:gizclaw/gizclaw.dart';
import 'package:test/test.dart';

import 'fake_transport.dart';

void main() {
  test('identifies workspace history replay events', () {
    final replay = PeerStreamEvent(
      type: 'text.delta',
      streamId: 'history-replay-42',
      text: 'saved message',
    );
    final live = PeerStreamEvent(
      type: 'text.delta',
      streamId: 'audio-live',
      text: 'new message',
    );

    expect(replay.isHistoryReplay, isTrue);
    expect(live.isHistoryReplay, isFalse);
  });

  test('sends one ordered audio turn lifecycle', () async {
    final factory = FakeDataChannelFactory();
    final session = await WorkspaceEventSession.open(factory);

    await session.beginAudio('audio-1');
    await session.endAudio('audio-1');

    final channel = factory.channels.single;
    expect(channel.label, giznetWebRtcEventDataChannelLabel);
    expect(channel.label, giznetServiceDataChannelLabel(serviceAgentEvent));
    expect(channel.sent, hasLength(2));
    final bosFrame = decodeFrames(channel.sent.first).single;
    final doneFrame = decodeFrames(channel.sent.last).single;
    expect(bosFrame.type, rpcFrameTypeBinary);
    expect(doneFrame.type, rpcFrameTypeBinary);
    final bos = PeerEvent.fromBuffer(bosFrame.payload);
    final done = PeerEvent.fromBuffer(doneFrame.payload);
    expect(bos.type, PeerEventType.PEER_EVENT_TYPE_BOS);
    expect(bos.bos.kind, StreamKind.STREAM_KIND_AUDIO);
    expect(done.type, PeerEventType.PEER_EVENT_TYPE_EOS);
    expect(done.eos.streamId, 'audio-1');

    await session.close();
  });

  test('decodes streaming events and rejects malformed payloads', () async {
    final factory = FakeDataChannelFactory();
    final session = await WorkspaceEventSession.open(factory);
    final events = <PeerStreamEvent>[];
    final errors = <Object>[];
    final subscription = session.events.listen(events.add, onError: errors.add);

    final message = encodeFrame(
      rpcFrameTypeBinary,
      PeerEvent(
        version: 1,
        type: PeerEventType.PEER_EVENT_TYPE_TEXT_DELTA,
        textDelta: TextDelta(
          streamId: 'reply-1',
          label: 'assistant',
          text: '你好',
        ),
      ).writeToBuffer(),
    );
    factory.channels.single.addMessage(message.sublist(0, 3));
    factory.channels.single.addMessage(message.sublist(3));
    factory.channels.single.addMessage(encodeFrame(rpcFrameTypeText, [1]));
    await Future<void>.delayed(Duration.zero);

    expect(events.single.text, '你好');
    expect(errors.single, isA<FormatException>());
    expect(factory.channels.single.state, GizClawDataChannelState.closed);
    expect(() => session.beginAudio('after-protocol-error'), throwsStateError);
    await subscription.cancel();
    await session.close();
  });

  test('one connection-owned session broadcasts to local consumers', () async {
    final factory = FakeDataChannelFactory();
    final session = await WorkspaceEventSession.open(factory);
    final first = <PeerStreamEvent>[];
    final second = <PeerStreamEvent>[];
    final firstSubscription = session.events.listen(first.add);
    final secondSubscription = session.events.listen(second.add);

    final event = PeerEvent(
      version: 1,
      type: PeerEventType.PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED,
      workspaceHistoryUpdated: WorkspaceHistoryUpdated(
        workspaceName: 'room-a',
        workspaceKind: WorkspaceKind.WORKSPACE_KIND_GROUP_CHATROOM,
      ),
    );
    factory.channels.single.addMessage(
      encodeFrame(rpcFrameTypeBinary, event.writeToBuffer()),
    );
    await Future<void>.delayed(Duration.zero);

    expect(factory.channels, hasLength(1));
    expect(first.single.workspaceHistoryUpdated?.workspaceName, 'room-a');
    expect(second.single.workspaceHistoryUpdated?.workspaceName, 'room-a');
    await firstSubscription.cancel();
    await secondSubscription.cancel();
    await session.close();
  });

  test('matches every cross-language Peer Event golden vector', () {
    final source = File(
      '../../../api/proto/events/testdata/peer_event_vectors.json',
    ).readAsStringSync();
    final vectors = (jsonDecode(source) as List<Object?>)
        .cast<Map<String, Object?>>();
    expect(vectors, hasLength(7));
    for (final vector in vectors) {
      final expected = _hexBytes(vector['hex']! as String);
      final event = PeerEvent.fromBuffer(expected);
      final decoded = PeerStreamEvent.decode(expected);
      expect(decoded.type, isNot('unknown'), reason: vector['name']! as String);
      expect(
        event.writeToBuffer(),
        expected,
        reason: vector['name']! as String,
      );
    }
  });

  test('keeps a future event type consumable', () {
    final decoded = PeerStreamEvent.decode(_hexBytes('080110638a0100'));
    expect(decoded.type, 'unknown');
  });

  test('rejects domain events with missing resource identifiers', () {
    expect(
      () => PeerStreamEvent(
        type: 'workspace.history.updated',
        workspaceHistoryUpdated: WorkspaceHistoryUpdated(workspaceName: ' '),
      ),
      throwsFormatException,
    );
  });

  test('rejects stream events with missing stream identifiers', () {
    expect(
      () => PeerStreamEvent(type: 'bos', kind: 'audio', streamId: ' '),
      throwsFormatException,
    );
  });
}

Uint8List _hexBytes(String value) => Uint8List.fromList([
  for (var offset = 0; offset < value.length; offset += 2)
    int.parse(value.substring(offset, offset + 2), radix: 16),
]);
