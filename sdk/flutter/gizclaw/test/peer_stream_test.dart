import 'dart:convert';

import 'package:gizclaw/gizclaw.dart';
import 'package:test/test.dart';

import 'fake_transport.dart';

void main() {
  test('identifies workspace history replay events', () {
    final replay = PeerStreamEvent.fromJson({
      'type': 'text.delta',
      'stream_id': 'history-replay-42',
      'text': 'saved message',
    });
    final live = PeerStreamEvent.fromJson({
      'type': 'text.delta',
      'stream_id': 'audio-live',
      'text': 'new message',
    });

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
    expect(bosFrame.type, rpcFrameTypeText);
    expect(doneFrame.type, rpcFrameTypeText);
    final bos = jsonDecode(utf8.decode(bosFrame.payload));
    final done = jsonDecode(utf8.decode(doneFrame.payload));
    expect(bos, containsPair('type', 'bos'));
    expect(bos, containsPair('kind', 'audio'));
    expect(done, containsPair('type', 'eos'));
    expect(done, containsPair('stream_id', 'audio-1'));

    await session.close();
  });

  test('decodes streaming events and rejects malformed payloads', () async {
    final factory = FakeDataChannelFactory();
    final session = await WorkspaceEventSession.open(factory);
    final events = <PeerStreamEvent>[];
    final errors = <Object>[];
    final subscription = session.events.listen(events.add, onError: errors.add);

    final message = encodeFrame(
      rpcFrameTypeText,
      utf8.encode(
        jsonEncode({
          'v': 1,
          'type': 'text.delta',
          'stream_id': 'reply-1',
          'label': 'assistant',
          'text': '你好',
        }),
      ),
    );
    factory.channels.single.addMessage(message.sublist(0, 3));
    factory.channels.single.addMessage(message.sublist(3));
    factory.channels.single.addMessage(
      encodeFrame(rpcFrameTypeText, utf8.encode('{}')),
    );
    await Future<void>.delayed(Duration.zero);

    expect(events.single.text, '你好');
    expect(errors.single, isA<FormatException>());
    await subscription.cancel();
    await session.close();
  });
}
