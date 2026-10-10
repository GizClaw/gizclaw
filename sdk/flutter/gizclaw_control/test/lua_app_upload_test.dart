import 'dart:convert';
import 'package:gizclaw_control/gizclaw_control.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:test/test.dart';

void main() {
  test('Lua upload preserves raw streamed bytes and metadata', () async {
    var calls = 0;
    final client = GizClawControlClient(
      baseUrl: Uri.parse('https://example.test'),
      apiKey: 'test-key',
      httpClient: MockClient((request) async {
        calls++;
        expect(request.url.path, '/gizclaw/v1/device/lua-app/install');
        expect(request.url.queryParameters['content_length'], '164484');
        expect(request.headers['Content-Type'], 'application/octet-stream');
        expect(request.bodyBytes.length, 164484);
        expect(request.bodyBytes.every((v) => v == 42), isTrue);
        return http.Response(
          jsonEncode({
            'app': {'app_id': 'demo', 'version': '1.0.0'},
          }),
          200,
        );
      }),
    );
    final result = await client.installLuaApp(
      Stream.fromIterable([
        List.filled(65535, 42),
        List.filled(65535, 42),
        List.filled(33414, 42),
      ]),
      contentLength: 164484,
      sha256: 'a' * 64,
    );
    expect(result['app_id'], 'demo');
    expect(calls, 1);
    client.close();
  });
}
