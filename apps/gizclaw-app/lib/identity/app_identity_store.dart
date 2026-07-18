import 'dart:convert';

import 'package:cryptography/cryptography.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../connection/gizclaw_connection_controller.dart';

abstract interface class IdentityValueStore {
  Future<String?> read(String key);

  Future<void> write(String key, String value);
}

class GizClawServer {
  const GizClawServer({
    required this.name,
    required this.accessPoint,
    this.registrationToken = '',
  });

  final String name;
  final String accessPoint;
  final String registrationToken;
}

class AppIdentityStore {
  AppIdentityStore({
    IdentityValueStore? secureValues,
    IdentityValueStore? preferences,
    GizClawConnectionProfile? fallbackProfile,
  }) : _secureValues = secureValues ?? KeychainIdentityValueStore(),
       _preferences = preferences ?? PreferencesIdentityValueStore(),
       _fallbackProfile =
           fallbackProfile ?? GizClawConnectionProfile.fromEnvironment();

  static const privateKeyStorageKey = 'gizclaw.client.private-key.v1';
  static const endpointStorageKey = 'gizclaw.server.endpoint.v1';
  static const customServersStorageKey = 'gizclaw.servers.custom.v1';
  static const registrationTokensStorageKey =
      'gizclaw.servers.registration-tokens.v1';

  final IdentityValueStore _secureValues;
  final IdentityValueStore _preferences;
  final GizClawConnectionProfile _fallbackProfile;

  Future<GizClawConnectionProfile> loadProfile() async {
    var privateKey = (await _secureValues.read(privateKeyStorageKey))?.trim();
    if (privateKey == null || privateKey.isEmpty) {
      final fallbackKey = _fallbackProfile.clientPrivateKey.trim();
      privateKey = fallbackKey.isEmpty ? _newPrivateKey() : fallbackKey;
      _validatePrivateKey(privateKey);
      await _secureValues.write(privateKeyStorageKey, privateKey);
    } else {
      _validatePrivateKey(privateKey);
    }

    final savedEndpoint = (await _preferences.read(endpointStorageKey))?.trim();
    final endpointValue = savedEndpoint == null || savedEndpoint.isEmpty
        ? _fallbackProfile.endpoint.trim()
        : savedEndpoint;
    final endpoint = endpointValue.isEmpty
        ? ''
        : normalizeGizClawEndpoint(endpointValue);
    final registrationTokens = await _loadRegistrationTokens();
    final fallbackEndpoint = _fallbackProfile.endpoint.trim().isEmpty
        ? ''
        : normalizeGizClawEndpoint(_fallbackProfile.endpoint);
    final registrationToken = endpoint.isEmpty
        ? ''
        : registrationTokens[endpoint] ??
              (endpoint == fallbackEndpoint
                  ? _fallbackProfile.registrationToken
                  : '');
    final publicKey = await _deriveClientPublicKey(base58Decode(privateKey));
    return GizClawConnectionProfile(
      endpoint: endpoint,
      clientPrivateKey: privateKey,
      clientPublicKey: publicKey,
      registrationToken: registrationToken,
    );
  }

  Future<void> saveEndpoint(String endpoint) {
    return _preferences.write(
      endpointStorageKey,
      normalizeGizClawEndpoint(endpoint),
    );
  }

  Future<List<GizClawServer>> loadServers() async {
    final customServers = <GizClawServer>[];
    final registrationTokens = await _loadRegistrationTokens();
    final encoded = await _preferences.read(customServersStorageKey);
    if (encoded != null && encoded.trim().isNotEmpty) {
      try {
        final values = jsonDecode(encoded);
        if (values is List<Object?>) {
          for (final value in values) {
            final server = _decodeServer(value, registrationTokens);
            if (server != null) customServers.add(server);
          }
        }
      } on FormatException {
        // Ignore malformed preferences and let the user add a valid server.
      }
    }

    final savedEndpoint = (await _preferences.read(endpointStorageKey))?.trim();
    final fallbackEndpoint = _fallbackProfile.endpoint.trim();
    final legacyEndpoint = savedEndpoint == null || savedEndpoint.isEmpty
        ? fallbackEndpoint
        : savedEndpoint;
    if (legacyEndpoint.isNotEmpty) {
      final normalized = normalizeGizClawEndpoint(legacyEndpoint);
      final known = customServers.any(
        (server) => server.accessPoint == normalized,
      );
      if (!known) {
        customServers.add(
          GizClawServer(
            name: normalized,
            accessPoint: normalized,
            registrationToken:
                registrationTokens[normalized] ??
                (normalized == normalizeGizClawEndpoint(fallbackEndpoint)
                    ? _fallbackProfile.registrationToken
                    : ''),
          ),
        );
      }
    }

    return List.unmodifiable(customServers);
  }

  Future<void> saveCustomServers(List<GizClawServer> servers) async {
    final encoded = jsonEncode([
      for (final server in servers)
        {
          'name': server.name.trim(),
          'access_point': normalizeGizClawEndpoint(server.accessPoint),
        },
    ]);
    final registrationTokens = <String, String>{};
    for (final server in servers) {
      final token = server.registrationToken.trim();
      if (token.isEmpty) continue;
      registrationTokens[normalizeGizClawEndpoint(server.accessPoint)] = token;
    }
    await _preferences.write(customServersStorageKey, encoded);
    await _secureValues.write(
      registrationTokensStorageKey,
      jsonEncode(registrationTokens),
    );
  }

  Future<Map<String, String>> _loadRegistrationTokens() async {
    final encoded = await _secureValues.read(registrationTokensStorageKey);
    if (encoded == null || encoded.trim().isEmpty) return const {};
    try {
      final value = jsonDecode(encoded);
      if (value is! Map<String, Object?>) return const {};
      final out = <String, String>{};
      for (final entry in value.entries) {
        if (entry.value case final String token when token.trim().isNotEmpty) {
          try {
            out[normalizeGizClawEndpoint(entry.key)] = token.trim();
          } on FormatException {
            // Ignore malformed endpoint keys in secure storage.
          }
        }
      }
      return out;
    } on FormatException {
      return const {};
    }
  }
}

GizClawServer? _decodeServer(
  Object? value,
  Map<String, String> registrationTokens,
) {
  if (value is! Map<String, Object?>) return null;
  final name = value['name'];
  final accessPoint = value['access_point'];
  if (name is! String || accessPoint is! String || name.trim().isEmpty) {
    return null;
  }
  try {
    final normalizedEndpoint = normalizeGizClawEndpoint(accessPoint);
    if (normalizedEndpoint.isEmpty) return null;
    return GizClawServer(
      name: name.trim(),
      accessPoint: normalizedEndpoint,
      registrationToken: registrationTokens[normalizedEndpoint] ?? '',
    );
  } on FormatException {
    return null;
  }
}

Future<String> _deriveClientPublicKey(List<int> privateKey) async {
  final keyPair = await X25519().newKeyPairFromSeed(privateKey);
  final publicKey = await keyPair.extractPublicKey();
  return base58Encode(publicKey.bytes);
}

class KeychainIdentityValueStore implements IdentityValueStore {
  KeychainIdentityValueStore({FlutterSecureStorage? storage})
    : _storage =
          storage ??
          const FlutterSecureStorage(
            iOptions: IOSOptions(
              accessibility: KeychainAccessibility.first_unlock_this_device,
            ),
            aOptions: AndroidOptions(),
          );

  final FlutterSecureStorage _storage;

  @override
  Future<String?> read(String key) => _storage.read(key: key);

  @override
  Future<void> write(String key, String value) {
    return _storage.write(key: key, value: value);
  }
}

class PreferencesIdentityValueStore implements IdentityValueStore {
  PreferencesIdentityValueStore({SharedPreferencesAsync? preferences})
    : _preferences = preferences ?? SharedPreferencesAsync();

  final SharedPreferencesAsync _preferences;

  @override
  Future<String?> read(String key) => _preferences.getString(key);

  @override
  Future<void> write(String key, String value) {
    return _preferences.setString(key, value);
  }
}

String _newPrivateKey() {
  while (true) {
    final bytes = randomBytes(32);
    if (bytes.any((byte) => byte != 0)) return base58Encode(bytes);
  }
}

void _validatePrivateKey(String value) {
  final bytes = base58Decode(value);
  if (bytes.length != 32 || bytes.every((byte) => byte == 0)) {
    throw const FormatException(
      'GizClaw private key must be 32 non-zero bytes',
    );
  }
}
