import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart';

import 'transport.dart';

const _base58Alphabet =
    '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';
final _base58Map = <int, int>{
  for (var i = 0; i < _base58Alphabet.length; i++)
    _base58Alphabet.codeUnitAt(i): i,
};

class GiznetSignalingIdentity {
  const GiznetSignalingIdentity({
    this.clientPublicKey,
    required this.clientPrivateKey,
    required this.serverPublicKey,
  });

  final List<int> clientPrivateKey;
  final List<int>? clientPublicKey;
  final List<int> serverPublicKey;
}

class PreparedGiznetWebRtcOffer {
  const PreparedGiznetWebRtcOffer({
    required this.body,
    required this.clientPublicKey,
    required this.nonce,
    required this.openAnswer,
    required this.timestamp,
  });

  final Uint8List body;
  final String clientPublicKey;
  final String nonce;
  final Future<String> Function(List<int> encryptedAnswer) openAnswer;
  final int timestamp;
}

class GiznetServerInfo {
  const GiznetServerInfo({
    this.endpoint,
    this.protocol,
    required this.publicKey,
    this.signalingPath = giznetWebRtcSignalingPath,
  });

  final String? endpoint;
  final String? protocol;
  final String publicKey;
  final String signalingPath;

  factory GiznetServerInfo.fromJson(Map<String, Object?> json) {
    final protocol = json['protocol'] as String?;
    if (protocol != null && protocol != 'gizclaw-webrtc') {
      throw FormatException(
        'server-info protocol = $protocol, want gizclaw-webrtc',
      );
    }
    final publicKey = (json['public_key'] as String?)?.trim();
    if (publicKey == null || publicKey.isEmpty) {
      throw const FormatException('server-info missing public_key');
    }
    final publicKeyBytes = base58Decode(publicKey);
    if (publicKeyBytes.length != 32 ||
        publicKeyBytes.every((byte) => byte == 0)) {
      throw const FormatException('server-info invalid public_key');
    }
    final signalingPath = _normalizeSignalingPath(
      json['signaling_path'] as String?,
    );
    return GiznetServerInfo(
      endpoint: json['endpoint'] as String?,
      protocol: protocol,
      publicKey: publicKey,
      signalingPath: signalingPath,
    );
  }
}

Future<PreparedGiznetWebRtcOffer> prepareEncryptedGiznetWebRtcOffer(
  GiznetSignalingIdentity identity,
  String offerSdp, {
  List<int>? nonceBytes,
  int? timestamp,
}) async {
  final clientPrivateKey = _expectKeyBytes(
    identity.clientPrivateKey,
    'client private key',
  );
  final x25519 = X25519();
  final clientKeyPair = await x25519.newKeyPairFromSeed(clientPrivateKey);
  final derivedPublicKey = await clientKeyPair.extractPublicKey();
  final clientPublicKey = identity.clientPublicKey == null
      ? derivedPublicKey.bytes
      : _expectKeyBytes(identity.clientPublicKey!, 'client public key');
  final serverPublicKey = _expectKeyBytes(
    identity.serverPublicKey,
    'server public key',
  );
  final nonce = base64UrlEncodeNoPadding(
    nonceBytes == null ? randomBytes(16) : Uint8List.fromList(nonceBytes),
  );
  final timestampValue =
      timestamp ?? DateTime.now().millisecondsSinceEpoch ~/ 1000;
  final keys = await _deriveSignalingKeys(
    clientKeyPair: clientKeyPair,
    serverPublicKey: serverPublicKey,
    nonce: nonce,
    timestamp: timestampValue,
  );
  final requestAad = signalingAad(clientPublicKey, timestampValue, nonce);
  final cipher = Chacha20.poly1305Aead();
  final encrypted = await cipher.encrypt(
    utf8.encode(offerSdp),
    secretKey: SecretKey(keys.requestKey),
    nonce: keys.requestNonce,
    aad: requestAad,
  );

  return PreparedGiznetWebRtcOffer(
    body: encrypted.concatenation(nonce: false),
    clientPublicKey: base58Encode(clientPublicKey),
    nonce: nonce,
    openAnswer: (encryptedAnswer) async {
      if (encryptedAnswer.length < 16) {
        throw const FormatException(
          'encrypted answer is shorter than AEAD tag',
        );
      }
      final responseAad = signalingAad(
        clientPublicKey,
        timestampValue,
        nonce,
        answer: true,
      );
      final secretBox = SecretBox(
        encryptedAnswer.sublist(0, encryptedAnswer.length - 16),
        nonce: keys.responseNonce,
        mac: Mac(encryptedAnswer.sublist(encryptedAnswer.length - 16)),
      );
      final bytes = await cipher.decrypt(
        secretBox,
        secretKey: SecretKey(keys.responseKey),
        aad: responseAad,
      );
      return utf8.decode(bytes);
    },
    timestamp: timestampValue,
  );
}

Uint8List signalingAad(
  List<int> clientPublicKey,
  int timestamp,
  String nonce, {
  bool answer = false,
}) {
  final parts = [
    'POST',
    giznetWebRtcSignalingPath,
    base58Encode(clientPublicKey),
    timestamp.toString(),
    nonce,
    if (answer) 'answer',
  ];
  return Uint8List.fromList(utf8.encode(parts.join('\n')));
}

String base58Encode(List<int> bytes) {
  var value = BigInt.zero;
  for (final byte in bytes) {
    value = (value << 8) + BigInt.from(byte);
  }
  var text = '';
  while (value > BigInt.zero) {
    final mod = (value % BigInt.from(58)).toInt();
    text = _base58Alphabet[mod] + text;
    value ~/= BigInt.from(58);
  }
  for (final byte in bytes) {
    if (byte != 0) {
      break;
    }
    text = '1$text';
  }
  return text.isEmpty ? '1' : text;
}

Uint8List base58Decode(String text) {
  var value = BigInt.zero;
  for (final unit in text.codeUnits) {
    final digit = _base58Map[unit];
    if (digit == null) {
      throw FormatException(
        'invalid base58 character ${String.fromCharCode(unit)}',
      );
    }
    value = value * BigInt.from(58) + BigInt.from(digit);
  }
  final bytes = <int>[];
  while (value > BigInt.zero) {
    bytes.add((value & BigInt.from(0xff)).toInt());
    value >>= 8;
  }
  for (final unit in text.codeUnits) {
    if (unit != '1'.codeUnitAt(0)) {
      break;
    }
    bytes.add(0);
  }
  return Uint8List.fromList(bytes.reversed.toList());
}

String base64UrlEncodeNoPadding(List<int> bytes) =>
    base64Url.encode(bytes).replaceAll('=', '');

Uint8List base64UrlDecodeNoPadding(String text) {
  final normalized = text.padRight((text.length + 3) ~/ 4 * 4, '=');
  return Uint8List.fromList(base64Url.decode(normalized));
}

Uint8List randomBytes(int length, {Random? random}) {
  final source = random ?? Random.secure();
  return Uint8List.fromList(
    List<int>.generate(length, (_) => source.nextInt(256)),
  );
}

String _normalizeSignalingPath(String? path) {
  final value = path?.trim() ?? '';
  if (value.isEmpty) {
    return giznetWebRtcSignalingPath;
  }
  if (!value.startsWith('/') || value.startsWith('//')) {
    throw FormatException('server-info invalid signaling_path $value');
  }
  return value;
}

Uint8List _expectKeyBytes(List<int> bytes, String name) {
  if (bytes.length != 32) {
    throw ArgumentError.value(bytes.length, name, 'invalid key length');
  }
  return Uint8List.fromList(bytes);
}

Future<_SignalingKeys> _deriveSignalingKeys({
  required SimpleKeyPair clientKeyPair,
  required List<int> serverPublicKey,
  required String nonce,
  required int timestamp,
}) async {
  final x25519 = X25519();
  final shared = await x25519.sharedSecretKey(
    keyPair: clientKeyPair,
    remotePublicKey: SimplePublicKey(serverPublicKey, type: KeyPairType.x25519),
  );
  final salt = Uint8List.fromList([
    ...base64UrlDecodeNoPadding(nonce),
    ...utf8.encode(timestamp.toString()),
  ]);
  final hkdf32 = Hkdf(hmac: Hmac.sha256(), outputLength: 32);
  final hkdf12 = Hkdf(hmac: Hmac.sha256(), outputLength: 12);
  final requestKey = await hkdf32.deriveKey(
    secretKey: shared,
    nonce: salt,
    info: utf8.encode('giznet/gizwebrtc/http-signaling/v1 c2s'),
  );
  final requestNonce = await hkdf12.deriveKey(
    secretKey: shared,
    nonce: salt,
    info: utf8.encode('giznet/gizwebrtc/http-signaling/v1 c2s nonce'),
  );
  final responseKey = await hkdf32.deriveKey(
    secretKey: shared,
    nonce: salt,
    info: utf8.encode('giznet/gizwebrtc/http-signaling/v1 s2c'),
  );
  final responseNonce = await hkdf12.deriveKey(
    secretKey: shared,
    nonce: salt,
    info: utf8.encode('giznet/gizwebrtc/http-signaling/v1 s2c nonce'),
  );
  return _SignalingKeys(
    requestKey: requestKey.bytes,
    requestNonce: requestNonce.bytes,
    responseKey: responseKey.bytes,
    responseNonce: responseNonce.bytes,
  );
}

class _SignalingKeys {
  const _SignalingKeys({
    required this.requestKey,
    required this.requestNonce,
    required this.responseKey,
    required this.responseNonce,
  });

  final List<int> requestKey;
  final List<int> requestNonce;
  final List<int> responseKey;
  final List<int> responseNonce;
}
