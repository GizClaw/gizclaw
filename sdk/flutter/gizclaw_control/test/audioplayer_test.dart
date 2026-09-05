import 'dart:convert';
import 'package:gizclaw_control/gizclaw_control.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:test/test.dart';

void main() {
  test(
    'player routes and snapshot preserve zero index and catalog metadata',
    () async {
      final requests = <http.Request>[];
      final status = {
        'state': 'buffering',
        'current_index': 0,
        'position_ms': 0,
        'repeat': 'all',
        'playlist_length': 1,
        'playlist_revision': 3,
        'observed_at_unix_ms': 1700000000000,
      };
      const item = AudioPlayerItem(
        url: 'https://media.example/music.mp3',
        title: 'music',
        sourceRef: 'catalog/song',
      );
      final client = GizClawControlClient(
        baseUrl: Uri.parse('https://example.com'),
        apiKey: 'test-key',
        httpClient: MockClient((request) async {
          requests.add(request);
          final Object body =
              request.method == 'GET' && request.url.path.endsWith('/playlist')
              ? {
                  'items': [item.toJson()],
                  'playlist_revision': 3,
                }
              : request.url.path.endsWith('/device/status')
              ? {'audioplayer': status}
              : {'status': status};
          return http.Response(jsonEncode(body), 200);
        }),
      );
      expect((await client.getAudioPlayer()).status.currentIndex, 0);
      expect(
        (await client.getAudioPlayerPlaylist()).items.single.sourceRef,
        'catalog/song',
      );
      await client.setAudioPlayerPlaylist([item]);
      await client.appendAudioPlayerPlaylist([item]);
      await client.playAudioPlayer(0);
      await client.stopAudioPlayer();
      await client.setAudioPlayerMode('all');
      expect((await client.getDeviceStatus()).audioplayer!.positionMs, 0);
      expect(requests.map((request) => request.method).toList(), [
        'GET',
        'GET',
        'PUT',
        'POST',
        'POST',
        'POST',
        'PUT',
        'GET',
      ]);
      expect(jsonDecode(requests[2].body), {
        'items': [item.toJson()],
      });
      expect(jsonDecode(requests[4].body), {'index': 0});
      expect(
        requests[3].url.path,
        '/gizclaw/v1/device/audioplayer/playlist/append',
      );
      expect(
        requests.every(
          (request) => request.headers['authorization'] == 'Bearer test-key',
        ),
        isTrue,
      );
    },
  );
}
