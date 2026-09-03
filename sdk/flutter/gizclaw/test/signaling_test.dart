import 'dart:convert';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart';
import 'package:gizclaw/src/signaling.dart';
import 'package:test/test.dart';

void main() {
  transportTests();
  test('validates server-info payloads', () {
    final publicKey = base58Encode(List<int>.filled(32, 7));
    final info = GiznetServerInfo.fromJson({
      'build_commit': 'deadbeef',
      'protocol': 'gizclaw-webrtc',
      'public_key': publicKey,
      'signaling_path': '/webrtc/v1/offer',
      'version': '0.2.5',
    });

    expect(info.publicKey, publicKey);
    expect(info.version, '0.2.5');
    expect(info.buildCommit, 'deadbeef');
    expect(info.signalingPath, '/webrtc/v1/offer');
    expect(
      () =>
          GiznetServerInfo.fromJson({'public_key': publicKey, 'protocol': 'x'}),
      throwsFormatException,
    );
    expect(
      () => GiznetServerInfo.fromJson({'public_key': 'bad0'}),
      throwsFormatException,
    );
  });

  test('prepares encrypted offer and opens encrypted answer', () async {
    final clientPrivateKey = List<int>.generate(32, (index) => index + 1);
    final serverPrivateKey = List<int>.generate(32, (index) => 32 - index);
    final x25519 = X25519();
    final serverKeyPair = await x25519.newKeyPairFromSeed(serverPrivateKey);
    final serverPublicKey = await serverKeyPair.extractPublicKey();

    final prepared = await prepareEncryptedGiznetWebRtcOffer(
      GiznetSignalingIdentity(
        clientPrivateKey: clientPrivateKey,
        serverPublicKey: serverPublicKey.bytes,
      ),
      'offer-sdp',
      nonceBytes: List<int>.filled(16, 9),
      timestamp: 12345,
    );

    expect(prepared.clientPublicKey, isNotEmpty);
    expect(prepared.body, isNotEmpty);
    expect(
      signalingAad(
        base58Decode(prepared.clientPublicKey),
        12345,
        prepared.nonce,
      ),
      utf8.encode(
        'POST\n/webrtc/v1/offer\n${prepared.clientPublicKey}\n12345\n${prepared.nonce}',
      ),
    );

    final encryptedAnswer = await _encryptAnswer(
      answerSdp: 'answer-sdp',
      clientPublicKey: base58Decode(prepared.clientPublicKey),
      nonce: prepared.nonce,
      serverKeyPair: serverKeyPair,
      timestamp: prepared.timestamp,
    );
    expect(await prepared.openAnswer(encryptedAnswer), 'answer-sdp');

    encryptedAnswer[0] ^= 0xff;
    expect(
      prepared.openAnswer(encryptedAnswer),
      throwsA(isA<SecretBoxAuthenticationError>()),
    );
  });

  test('rejects invalid key lengths and malformed base58', () async {
    expect(
      prepareEncryptedGiznetWebRtcOffer(
        const GiznetSignalingIdentity(
          clientPrivateKey: [1],
          serverPublicKey: [2],
        ),
        'offer',
      ),
      throwsArgumentError,
    );
    expect(() => base58Decode('0'), throwsFormatException);
  });
}

Future<Uint8List> _encryptAnswer({
  required String answerSdp,
  required List<int> clientPublicKey,
  required String nonce,
  required SimpleKeyPair serverKeyPair,
  required int timestamp,
}) async {
  final x25519 = X25519();
  final shared = await x25519.sharedSecretKey(
    keyPair: serverKeyPair,
    remotePublicKey: SimplePublicKey(clientPublicKey, type: KeyPairType.x25519),
  );
  final salt = [
    ...base64UrlDecodeNoPadding(nonce),
    ...utf8.encode(timestamp.toString()),
  ];
  final responseKey = await Hkdf(hmac: Hmac.sha256(), outputLength: 32)
      .deriveKey(
        secretKey: shared,
        nonce: salt,
        info: utf8.encode('giznet/gizwebrtc/http-signaling/v1 s2c'),
      );
  final responseNonce = await Hkdf(hmac: Hmac.sha256(), outputLength: 12)
      .deriveKey(
        secretKey: shared,
        nonce: salt,
        info: utf8.encode('giznet/gizwebrtc/http-signaling/v1 s2c nonce'),
      );
  final box = await Chacha20.poly1305Aead().encrypt(
    utf8.encode(answerSdp),
    secretKey: SecretKey(responseKey.bytes),
    nonce: responseNonce.bytes,
    aad: signalingAad(clientPublicKey, timestamp, nonce, answer: true),
  );
  return box.concatenation(nonce: false);
}

void transportTests() {
  group('server-info transport', () {
    test('parses an advertised Edge gateway', () {
      final info = GiznetServerInfo.fromJson({
        'protocol': 'gizclaw-webrtc',
        'public_key': 'BoYfN5LcjihD8j7HmzDW56s3E9F2R1AX8JsucW5Zvd7T',
        'transport': {
          'mode': 'edge-gateway',
          'endpoint': '127.0.0.1:9821',
          'public_key': 'FNSseo3ePDEyJR27qEbDCSKBX4baMg826xXcanV4Huqs',
          'signaling_path': '/webrtc/v1/offer',
        },
      });
      expect(info.transport?.mode, 'edge-gateway');
      expect(
        info.transportPublicKey,
        'FNSseo3ePDEyJR27qEbDCSKBX4baMg826xXcanV4Huqs',
      );
      expect(info.transportSignalingPath, '/webrtc/v1/offer');
    });

    test('falls back to the Server key when no gateway is advertised', () {
      final info = GiznetServerInfo.fromJson({
        'public_key': 'BoYfN5LcjihD8j7HmzDW56s3E9F2R1AX8JsucW5Zvd7T',
      });
      expect(info.transport, isNull);
      expect(info.transportPublicKey, info.publicKey);
      expect(info.transportSignalingPath, '/webrtc/v1/offer');
    });

    test('exposes the advertised ICE UDP endpoint', () {
      final info = GiznetServerInfo.fromJson({
        'public_key': 'BoYfN5LcjihD8j7HmzDW56s3E9F2R1AX8JsucW5Zvd7T',
        'endpoint': '127.0.0.1:9820',
      });
      expect(info.iceEndpoint, '127.0.0.1:9820');
      expect(
        () => GiznetServerInfo.fromJson({
          'public_key': 'BoYfN5LcjihD8j7HmzDW56s3E9F2R1AX8JsucW5Zvd7T',
          'endpoint': 'ftp://127.0.0.1:9820',
        }),
        throwsFormatException,
      );
    });

    test('normalizes access points and inherits the transport scheme', () {
      expect(
        normalizeGiznetAccessPoint('127.0.0.1:9820').toString(),
        'http://127.0.0.1:9820',
      );
      expect(
        normalizeGiznetAccessPoint('https://ap.gizclaw.com/').toString(),
        'https://ap.gizclaw.com',
      );
      expect(
        normalizeGiznetAccessPoint('https://ap.gizclaw.com/prefix/').toString(),
        'https://ap.gizclaw.com/prefix',
      );
      expect(
        resolveGiznetTransportBaseUrl(
          normalizeGiznetAccessPoint('https://ap.gizclaw.com'),
          'edge.example:9821',
        ).toString(),
        'https://edge.example:9821',
      );
      expect(
        resolveGiznetTransportBaseUrl(
          normalizeGiznetAccessPoint('http://ap.gizclaw.com'),
          'https://edge.example',
        ).toString(),
        'https://edge.example',
      );
      for (final value in <String>[
        '',
        'ftp://ap.gizclaw.com',
        'https://user@ap.gizclaw.com',
        'https://ap.gizclaw.com?probe=1',
        'https://ap.gizclaw.com#fragment',
      ]) {
        expect(
          () => normalizeGiznetAccessPoint(value),
          throwsFormatException,
          reason: value,
        );
      }
    });

    test('rejects a gateway without a usable public key', () {
      expect(
        () => GiznetServerInfo.fromJson({
          'public_key': 'BoYfN5LcjihD8j7HmzDW56s3E9F2R1AX8JsucW5Zvd7T',
          'transport': {'mode': 'edge-gateway'},
        }),
        throwsFormatException,
      );
    });
  });
}
