import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'transport.dart';

final _crlfcrlf = Uint8List.fromList('\r\n\r\n'.codeUnits);
final _crlf = Uint8List.fromList('\r\n'.codeUnits);

class ServiceHttpRequest {
  const ServiceHttpRequest({
    this.body = const [],
    this.headers = const {},
    this.method = 'GET',
    this.path = '/',
  });

  final List<int> body;
  final Map<String, String> headers;
  final String method;
  final String path;
}

class ServiceHttpResponse {
  const ServiceHttpResponse({
    required this.body,
    required this.headers,
    required this.status,
    required this.statusText,
  });

  final Uint8List body;
  final Map<String, String> headers;
  final int status;
  final String statusText;
}

class ServiceHttpClient {
  ServiceHttpClient(
    this._factory, {
    this.host = 'gizclaw',
    this.requestTimeout = const Duration(seconds: 30),
    this.service = servicePeerHttp,
  });

  final GizClawDataChannelFactory _factory;
  final String host;
  final Duration requestTimeout;
  final int service;

  Future<ServiceHttpResponse> send(ServiceHttpRequest request) {
    late final Uint8List requestBytes;
    try {
      requestBytes = encodeHttpRequest(request, host: host);
    } catch (error, stackTrace) {
      return Future<ServiceHttpResponse>.error(error, stackTrace);
    }
    final completer = Completer<ServiceHttpResponse>();
    final buffer = BytesBuilder(copy: false);
    GizClawDataChannel? channel;
    var requestSent = false;
    Timer? timer;
    StreamSubscription<Uint8List>? messages;
    StreamSubscription<GizClawDataChannelState>? states;

    Future<void> cleanup() async {
      timer?.cancel();
      final messageSubscription = messages;
      if (messageSubscription != null) {
        await messageSubscription.cancel();
      }
      final stateSubscription = states;
      if (stateSubscription != null) {
        await stateSubscription.cancel();
      }
      final activeChannel = channel;
      if (activeChannel != null) {
        await activeChannel.close();
      }
    }

    void fail(Object error, [StackTrace? stackTrace]) {
      if (completer.isCompleted) {
        return;
      }
      completer.completeError(error, stackTrace);
      _unawaited(cleanup());
    }

    void tryComplete({bool closed = false}) {
      try {
        final parsed = tryParseHttpResponse(buffer.toBytes(), closed: closed);
        if (parsed == null) {
          if (closed) {
            fail(
              StateError(
                'HTTP service data channel closed before complete response',
              ),
            );
          }
          return;
        }
        if (!completer.isCompleted) {
          completer.complete(parsed);
          unawaited(cleanup());
        }
      } catch (error, stackTrace) {
        fail(error, stackTrace);
      }
    }

    timer = Timer(requestTimeout, () {
      fail(TimeoutException('HTTP service request timed out', requestTimeout));
    });

    Future<void> openChannel() async {
      try {
        channel = await _factory.createDataChannel(
          giznetServiceDataChannelLabel(service),
          options: const GizClawDataChannelOptions(ordered: true),
        );
      } catch (error, stackTrace) {
        fail(error, stackTrace);
        return;
      }

      final activeChannel = channel;
      if (activeChannel == null) {
        fail(StateError('HTTP service data channel was not created'));
        return;
      }
      if (completer.isCompleted) {
        _unawaited(activeChannel.close());
        return;
      }

      Future<void> sendRequest() async {
        if (requestSent || completer.isCompleted) {
          return;
        }
        requestSent = true;
        try {
          await activeChannel.send(requestBytes);
        } catch (error, stackTrace) {
          fail(error, stackTrace);
        }
      }

      messages = activeChannel.messages.listen(
        (chunk) {
          buffer.add(chunk);
          tryComplete();
        },
        onError: fail,
        onDone: () => tryComplete(closed: true),
      );
      states = activeChannel.states.listen((state) {
        if (state == GizClawDataChannelState.open) {
          _unawaited(sendRequest());
        } else if (state == GizClawDataChannelState.closed) {
          tryComplete(closed: true);
        }
      }, onError: fail);

      if (activeChannel.state == GizClawDataChannelState.open) {
        _unawaited(sendRequest());
      } else if (activeChannel.state == GizClawDataChannelState.closed) {
        fail(StateError('HTTP service data channel is closed'));
      }
    }

    _unawaited(openChannel());
    return completer.future;
  }
}

Uint8List encodeHttpRequest(
  ServiceHttpRequest request, {
  String host = 'gizclaw',
}) {
  _rejectCrlf(request.method, 'method');
  _rejectCrlf(request.path, 'path');
  _rejectCrlf(host, 'host');
  for (final entry in request.headers.entries) {
    _rejectCrlf(entry.key, 'header name');
    _rejectCrlf(entry.value, 'header value');
  }
  final headers = <String, String>{...request.headers};
  headers.removeWhere(
    (key, _) =>
        key.toLowerCase() == 'host' ||
        key.toLowerCase() == 'connection' ||
        key.toLowerCase() == 'content-length' ||
        key.toLowerCase() == 'transfer-encoding',
  );
  headers['Host'] = host;
  headers['Connection'] = 'close';
  if (request.body.isNotEmpty) {
    headers['Content-Length'] = request.body.length.toString();
  }
  final lines = <String>['${request.method} ${request.path} HTTP/1.1'];
  for (final entry in headers.entries) {
    lines.add('${_canonicalHeader(entry.key)}: ${entry.value}');
  }
  lines.addAll(['', '']);
  final head = ascii.encode(lines.join('\r\n'));
  return Uint8List.fromList([...head, ...request.body]);
}

ServiceHttpResponse? tryParseHttpResponse(
  Uint8List buffer, {
  bool closed = false,
}) {
  final headerEnd = _indexOf(buffer, _crlfcrlf);
  if (headerEnd < 0) {
    return null;
  }
  final headerText = ascii.decode(buffer.sublist(0, headerEnd));
  final lines = headerText.split('\r\n');
  final statusMatch = RegExp(
    r'^HTTP/\d(?:\.\d)?\s+(\d{3})(?:\s+(.*))?$',
  ).firstMatch(lines.first);
  if (statusMatch == null) {
    throw const FormatException('invalid HTTP response status line');
  }
  final headers = <String, String>{};
  for (final line in lines.skip(1)) {
    final index = line.indexOf(':');
    if (index <= 0) {
      throw const FormatException('invalid HTTP response header');
    }
    headers[line.substring(0, index).toLowerCase()] = line
        .substring(index + 1)
        .trimLeft();
  }
  final bodyStart = headerEnd + _crlfcrlf.length;
  final body = Uint8List.sublistView(buffer, bodyStart);
  final transferEncoding = headers['transfer-encoding'] ?? '';
  if (RegExp(r'\bchunked\b', caseSensitive: false).hasMatch(transferEncoding)) {
    final decoded = _tryDecodeChunkedBody(body);
    if (decoded == null) {
      return null;
    }
    return ServiceHttpResponse(
      body: decoded,
      headers: headers,
      status: int.parse(statusMatch.group(1)!),
      statusText: statusMatch.group(2) ?? '',
    );
  }
  final contentLengthText = headers['content-length'];
  final contentLength = contentLengthText == null || contentLengthText.isEmpty
      ? null
      : int.tryParse(contentLengthText);
  if (contentLengthText != null &&
      contentLengthText.isNotEmpty &&
      (contentLength == null || contentLength < 0)) {
    throw FormatException(
      'invalid HTTP response content-length $contentLengthText',
    );
  }
  if (contentLength != null) {
    if (body.length < contentLength) {
      return null;
    }
    return ServiceHttpResponse(
      body: Uint8List.fromList(body.sublist(0, contentLength)),
      headers: headers,
      status: int.parse(statusMatch.group(1)!),
      statusText: statusMatch.group(2) ?? '',
    );
  }
  if (!closed) {
    return null;
  }
  return ServiceHttpResponse(
    body: Uint8List.fromList(body),
    headers: headers,
    status: int.parse(statusMatch.group(1)!),
    statusText: statusMatch.group(2) ?? '',
  );
}

int _indexOf(Uint8List data, Uint8List pattern) {
  for (var i = 0; i <= data.length - pattern.length; i++) {
    var match = true;
    for (var j = 0; j < pattern.length; j++) {
      if (data[i + j] != pattern[j]) {
        match = false;
        break;
      }
    }
    if (match) {
      return i;
    }
  }
  return -1;
}

Uint8List? _tryDecodeChunkedBody(Uint8List body) {
  final chunks = <Uint8List>[];
  var offset = 0;
  for (;;) {
    final lineEnd = _indexOfFrom(body, _crlf, offset);
    if (lineEnd < 0) {
      return null;
    }
    final sizeLine = ascii.decode(body.sublist(offset, lineEnd));
    final sizeText = sizeLine.split(';').first.trim();
    final size = int.tryParse(sizeText, radix: 16);
    if (size == null || size < 0) {
      throw FormatException('invalid HTTP chunk size $sizeText');
    }
    offset = lineEnd + _crlf.length;
    if (size == 0) {
      return _tryFinishChunkedTrailers(body, offset, chunks);
    }
    if (body.length < offset + size + _crlf.length) {
      return null;
    }
    chunks.add(Uint8List.sublistView(body, offset, offset + size));
    offset += size;
    if (body[offset] != _crlf[0] || body[offset + 1] != _crlf[1]) {
      throw const FormatException('invalid HTTP chunk terminator');
    }
    offset += _crlf.length;
  }
}

Uint8List? _tryFinishChunkedTrailers(
  Uint8List body,
  int offset,
  List<Uint8List> chunks,
) {
  for (;;) {
    final lineEnd = _indexOfFrom(body, _crlf, offset);
    if (lineEnd < 0) {
      return null;
    }
    if (lineEnd == offset) {
      return Uint8List.fromList(chunks.expand((chunk) => chunk).toList());
    }
    final trailer = ascii.decode(body.sublist(offset, lineEnd));
    if (!trailer.contains(':')) {
      throw const FormatException('invalid HTTP chunk trailer');
    }
    offset = lineEnd + _crlf.length;
  }
}

int _indexOfFrom(Uint8List data, Uint8List pattern, int start) {
  for (var i = start; i <= data.length - pattern.length; i++) {
    var match = true;
    for (var j = 0; j < pattern.length; j++) {
      if (data[i + j] != pattern[j]) {
        match = false;
        break;
      }
    }
    if (match) {
      return i;
    }
  }
  return -1;
}

String _canonicalHeader(String name) => name
    .split('-')
    .map(
      (part) => part.isEmpty
          ? part
          : '${part[0].toUpperCase()}${part.substring(1).toLowerCase()}',
    )
    .join('-');

void _rejectCrlf(String value, String field) {
  if (value.contains('\r') || value.contains('\n')) {
    throw ArgumentError.value(value, field, 'must not contain CR or LF');
  }
}

void _unawaited(Future<void> future) {}
