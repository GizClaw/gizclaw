import 'dart:convert';
import 'package:gizclaw_control/gizclaw_control.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:test/test.dart';

void main() {
  test(
    'oversized Binary control requires URL before reading or sending',
    () async {
      final client = GizClawControlClient(
        baseUrl: Uri.parse('https://example.test'),
        apiKey: 'test-key',
        httpClient: MockClient((_) async {
          fail('oversized request was sent');
        }),
      );
      await expectLater(
        client.installLuaApp(
          const Stream<List<int>>.empty(),
          contentLength: 524289,
          sha256: 'a' * 64,
        ),
        throwsArgumentError,
      );
      expect(
        classifyGizClawControlError(413, 'LUA_APP_PACKAGE_TOO_LARGE'),
        GizClawControlErrorKind.invalidRequest,
      );
      client.close();
    },
  );

  test('Lua upload preserves raw streamed bytes and metadata', () async {
    var calls = 0;
    final client = GizClawControlClient(
      baseUrl: Uri.parse('https://example.test'),
      apiKey: 'test-key',
      httpClient: MockClient((request) async {
        calls++;
        expect(request.url.path, '/gizclaw/v1/device/lua-app/install');
        expect(request.url.queryParameters['content_length'], '524288');
        expect(request.headers['Content-Type'], 'application/octet-stream');
        expect(request.bodyBytes.length, 524288);
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
      Stream.fromIterable([List.filled(262144, 42), List.filled(262144, 42)]),
      contentLength: 524288,
      sha256: 'a' * 64,
    );
    expect(result['app_id'], 'demo');
    expect(calls, 1);
    client.close();
  });
}
