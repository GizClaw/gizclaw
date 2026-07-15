import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/connection/gizclaw_connection_controller.dart';
import 'package:gizclaw_app/identity/app_identity_store.dart';

void main() {
  test('creates and reuses a secure device identity', () async {
    final secureValues = _MemoryValueStore();
    final preferences = _MemoryValueStore();
    final store = AppIdentityStore(
      secureValues: secureValues,
      preferences: preferences,
      fallbackProfile: const GizClawConnectionProfile(
        endpoint: '',
        clientPrivateKey: '',
      ),
    );

    final first = await store.loadProfile();
    final second = await store.loadProfile();

    expect(base58Decode(first.clientPrivateKey), hasLength(32));
    expect(first.clientPrivateKey, second.clientPrivateKey);
    expect(first.clientPublicKey, second.clientPublicKey);
    expect(
      secureValues.values[AppIdentityStore.privateKeyStorageKey],
      first.clientPrivateKey,
    );
  });

  test('migrates the environment private key into secure storage', () async {
    final privateKey = base58Encode(List<int>.generate(32, (i) => i + 1));
    final secureValues = _MemoryValueStore();
    final store = AppIdentityStore(
      secureValues: secureValues,
      preferences: _MemoryValueStore(),
      fallbackProfile: GizClawConnectionProfile(
        endpoint: 'gizclaw.local:9820',
        clientPrivateKey: privateKey,
      ),
    );

    final profile = await store.loadProfile();

    expect(profile.clientPrivateKey, privateKey);
    expect(profile.endpoint, 'gizclaw.local:9820');
    expect(
      secureValues.values[AppIdentityStore.privateKeyStorageKey],
      privateKey,
    );
  });

  test('persists a normalized server endpoint', () async {
    final preferences = _MemoryValueStore();
    final store = AppIdentityStore(
      secureValues: _MemoryValueStore(),
      preferences: preferences,
      fallbackProfile: const GizClawConnectionProfile(
        endpoint: '',
        clientPrivateKey: '',
      ),
    );

    await store.saveEndpoint('  https://gizclaw.example:443/  ');

    expect(
      preferences.values[AppIdentityStore.endpointStorageKey],
      'https://gizclaw.example:443',
    );
  });

  test('loads presets and persists custom servers', () async {
    final preferences = _MemoryValueStore();
    final store = AppIdentityStore(
      secureValues: _MemoryValueStore(),
      preferences: preferences,
      fallbackProfile: const GizClawConnectionProfile(
        endpoint: '',
        clientPrivateKey: '',
      ),
    );

    expect((await store.loadServers()).map((server) => server.name), [
      'Development',
      'Production',
    ]);

    await store.saveCustomServers(const [
      GizClawServer(name: 'Office', accessPoint: 'office.local:9820'),
    ]);

    final servers = await store.loadServers();
    expect(servers, hasLength(3));
    expect(servers.last.name, 'Office');
    expect(servers.last.accessPoint, 'office.local:9820');
  });

  test('includes a legacy selected endpoint in the server list', () async {
    final preferences = _MemoryValueStore()
      ..values[AppIdentityStore.endpointStorageKey] = 'legacy.local:9820';
    final store = AppIdentityStore(
      secureValues: _MemoryValueStore(),
      preferences: preferences,
      fallbackProfile: const GizClawConnectionProfile(
        endpoint: '',
        clientPrivateKey: '',
      ),
    );

    final servers = await store.loadServers();

    expect(servers.last.name, 'legacy.local:9820');
    expect(servers.last.accessPoint, 'legacy.local:9820');
  });

  test('validates domain and IP endpoints with explicit ports', () {
    expect(
      normalizeGizClawEndpoint('gizclaw.local:9820'),
      'gizclaw.local:9820',
    );
    expect(normalizeGizClawEndpoint('192.168.1.12:9820'), '192.168.1.12:9820');
    expect(
      normalizeGizClawEndpoint('115.190.149.150:9820'),
      '115.190.149.150:9820',
    );
    expect(
      normalizeGizClawEndpoint('gizclaw.example.com:9820'),
      'gizclaw.example.com:9820',
    );
    expect(
      normalizeGizClawEndpoint('http://gizclaw.example.com:9820'),
      'http://gizclaw.example.com:9820',
    );
    expect(
      normalizeGizClawEndpoint('https://gizclaw.example:443/'),
      'https://gizclaw.example:443',
    );
    expect(
      () => normalizeGizClawEndpoint('gizclaw.local'),
      throwsFormatException,
    );
    expect(
      () => normalizeGizClawEndpoint('gizclaw.local:9820/admin'),
      throwsFormatException,
    );
  });

  test('defines the selectable GizClaw server endpoints', () {
    expect(gizClawDevelopmentServerEndpoint, 'ap.dev.gizclaw.com:9820');
    expect(gizClawProductionServerEndpoint, 'ap.gizclaw.com:9820');
  });
}

class _MemoryValueStore implements IdentityValueStore {
  final Map<String, String> values = {};

  @override
  Future<String?> read(String key) async => values[key];

  @override
  Future<void> write(String key, String value) async {
    values[key] = value;
  }
}
