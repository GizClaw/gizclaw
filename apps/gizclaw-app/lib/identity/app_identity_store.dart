import 'package:cryptography/cryptography.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../connection/gizclaw_connection_controller.dart';

abstract interface class IdentityValueStore {
  Future<String?> read(String key);

  Future<void> write(String key, String value);
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
    final publicKey = await _deriveClientPublicKey(base58Decode(privateKey));
    return GizClawConnectionProfile(
      endpoint: endpoint,
      clientPrivateKey: privateKey,
      clientPublicKey: publicKey,
    );
  }

  Future<void> saveEndpoint(String endpoint) {
    return _preferences.write(
      endpointStorageKey,
      normalizeGizClawEndpoint(endpoint),
    );
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
