import 'dart:async';
import 'dart:convert';

import 'package:gizclaw/src/service_http.dart';
import 'package:gizclaw/src/transport.dart';
import 'package:test/test.dart';

import 'fake_transport.dart';

void main() {
  test('serializes HTTP request bytes for service channels', () {
    final bytes = encodeHttpRequest(
      const ServiceHttpRequest(
        body: [1, 2],
        headers: {'x-test': 'yes'},
        method: 'POST',
        path: '/openai/v1/chat/completions',
      ),
    );
    final text = ascii.decode(bytes.sublist(0, bytes.length - 2));

    expect(text, contains('POST /openai/v1/chat/completions HTTP/1.1'));
    expect(text, contains('Host: gizclaw'));
    expect(text, contains('X-Test: yes'));
    expect(text, contains('Content-Length: 2'));
  });

  test('replaces controlled HTTP headers case-insensitively', () {
    final bytes = encodeHttpRequest(
      const ServiceHttpRequest(
        headers: {
          'host': 'caller.example',
          'Connection': 'keep-alive',
          'content-length': '999',
          'Transfer-Encoding': 'chunked',
        },
      ),
      host: 'gizclaw.local',
    );
    final text = ascii.decode(bytes);

    expect(RegExp(r'\r\nHost: ').allMatches(text), hasLength(1));
    expect(text, contains('\r\nHost: gizclaw.local\r\n'));
    expect(RegExp(r'\r\nConnection: ').allMatches(text), hasLength(1));
    expect(text, contains('\r\nConnection: close\r\n'));
    expect(text, isNot(contains('Content-Length: 999')));
    expect(text, isNot(contains('Transfer-Encoding: chunked')));
  });

  test('rejects CRLF in encoded HTTP request fields', () {
    expect(
      () => encodeHttpRequest(
        const ServiceHttpRequest(method: 'GET\r\nInjected: yes'),
      ),
      throwsArgumentError,
    );
    expect(
      () => encodeHttpRequest(const ServiceHttpRequest(path: '/ok\n/bad')),
      throwsArgumentError,
    );
    expect(
      () => encodeHttpRequest(
        const ServiceHttpRequest(headers: {'x-test\r\nInjected': 'yes'}),
      ),
      throwsArgumentError,
    );
    expect(
      () => encodeHttpRequest(
        const ServiceHttpRequest(headers: {'x-test': 'yes\r\nInjected: yes'}),
      ),
      throwsArgumentError,
    );
    expect(
      () => encodeHttpRequest(const ServiceHttpRequest(), host: 'gizclaw\r\nx'),
      throwsArgumentError,
    );
  });

  test('parses content-length and close-delimited responses', () {
    final fixed = tryParseHttpResponse(
      ascii.encode('HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n{}'),
    );
    expect(fixed?.status, 200);
    expect(ascii.decode(fixed!.body), '{}');

    final closeDelimited = tryParseHttpResponse(
      ascii.encode('HTTP/1.1 204 No Content\r\n\r\nbody'),
      closed: true,
    );
    expect(closeDelimited?.status, 204);
    expect(ascii.decode(closeDelimited!.body), 'body');
  });

  test('parses chunked responses and validates malformed lengths', () {
    final chunked = tryParseHttpResponse(
      ascii.encode(
        'HTTP/1.1 200 OK\r\n'
        'Transfer-Encoding: chunked\r\n'
        '\r\n'
        '5\r\nhello\r\n'
        '6;ext=yes\r\n world\r\n'
        '0\r\n'
        'X-Trailer: done\r\n'
        '\r\n',
      ),
    );
    expect(chunked?.status, 200);
    expect(ascii.decode(chunked!.body), 'hello world');

    final incomplete = tryParseHttpResponse(
      ascii.encode(
        'HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhe',
      ),
    );
    expect(incomplete, isNull);

    expect(
      () => tryParseHttpResponse(
        ascii.encode('HTTP/1.1 200 OK\r\nContent-Length: nope\r\n\r\n'),
      ),
      throwsFormatException,
    );
  });

  test('uses service DataChannel and returns response', () async {
    final factory = FakeDataChannelFactory();
    final client = ServiceHttpClient(factory);

    final future = client.send(const ServiceHttpRequest(path: '/server-info'));
    await Future<void>.delayed(Duration.zero);
    final channel = factory.channels.single;
    expect(channel.label, 'giznet/v1/service/1');
    expect(channel.sent, hasLength(1));

    channel.addMessage(
      ascii.encode('HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n{}'),
    );

    final response = await future;
    expect(response.status, 200);
    expect(ascii.decode(response.body), '{}');
  });

  test('validates requests before opening a service channel', () async {
    final factory = FakeDataChannelFactory();
    final client = ServiceHttpClient(factory);

    await expectLater(
      client.send(const ServiceHttpRequest(path: '/bad\r\nInjected: yes')),
      throwsArgumentError,
    );

    expect(factory.channels, isEmpty);
  });

  test(
    'sends service HTTP request once when open state is emitted again',
    () async {
      final factory = FakeDataChannelFactory();
      final client = ServiceHttpClient(factory);

      final future = client.send(
        const ServiceHttpRequest(path: '/server-info'),
      );
      await Future<void>.delayed(Duration.zero);

      final channel = factory.channels.single;
      channel.setState(GizClawDataChannelState.open);
      await Future<void>.delayed(Duration.zero);
      expect(channel.sent, hasLength(1));

      channel.addMessage(
        ascii.encode('HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n{}'),
      );
      await future;
    },
  );

  test(
    'surfaces malformed service HTTP responses through the future',
    () async {
      final factory = FakeDataChannelFactory();
      final client = ServiceHttpClient(factory);

      final future = client.send(const ServiceHttpRequest(path: '/bad'));
      await Future<void>.delayed(Duration.zero);
      factory.channels.single.addMessage(
        ascii.encode('HTTP/1.1 200 OK\r\nContent-Length: nope\r\n\r\n'),
      );

      await expectLater(future, throwsFormatException);
    },
  );

  test(
    'fails immediately when the service channel closes mid-response',
    () async {
      final factory = FakeDataChannelFactory();
      final client = ServiceHttpClient(
        factory,
        requestTimeout: const Duration(minutes: 1),
      );

      final future = client.send(const ServiceHttpRequest(path: '/bad'));
      await Future<void>.delayed(Duration.zero);
      factory.channels.single.addMessage(
        ascii.encode('HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\n{}'),
      );
      factory.channels.single.setState(GizClawDataChannelState.closed);

      await expectLater(future, throwsA(isA<StateError>()));
    },
  );

  test('fails immediately when the service channel starts closed', () async {
    final factory = FakeDataChannelFactory(
      initialState: GizClawDataChannelState.closed,
    );
    final client = ServiceHttpClient(
      factory,
      requestTimeout: const Duration(minutes: 1),
    );

    await expectLater(
      client.send(const ServiceHttpRequest(path: '/closed')),
      throwsA(isA<StateError>()),
    );
    expect(factory.channels.single.sent, isEmpty);
  });

  test('times out when service channel never returns headers', () {
    final factory = FakeDataChannelFactory();
    final client = ServiceHttpClient(
      factory,
      requestTimeout: const Duration(milliseconds: 10),
    );

    expect(
      client.send(const ServiceHttpRequest(path: '/slow')),
      throwsA(isA<TimeoutException>()),
    );
  });
}
