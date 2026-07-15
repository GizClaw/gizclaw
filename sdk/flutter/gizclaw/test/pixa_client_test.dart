import 'dart:async';

import 'package:fixnum/fixnum.dart';
import 'package:gizclaw/src/client.dart';
import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
import 'package:gizclaw/src/generated/rpc/payload.pb.dart' as payload;
import 'package:gizclaw/src/payload_codec.dart';
import 'package:gizclaw/src/pixa.dart';
import 'package:gizclaw/src/rpc_frame.dart';
import 'package:test/test.dart';

import 'fake_transport.dart';
import 'pixa_test_data.dart';

void main() {
  test('downloads and validates owned pet pixa resources', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);
    final bytes = makePixa(clips: ['default']);

    final future = client.downloadPetPixa('pet-miso');
    await Future<void>.delayed(Duration.zero);
    final request = rpc.RpcRequest.fromBuffer(
      decodeFrames(factory.channels.single.sent.single).first.payload,
    );

    factory.channels.single.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(
          rpc.RpcResponse(
            id: request.id,
            payload: encodeRpcResponsePayload(
              'server.pet.pixa.download',
              payload.ServerPetPixaDownloadResponse(
                value: payload.PetPixaDownloadResponse(
                  petId: 'pet-miso',
                  petdefId: 'petdef-miso',
                  pixaPath: 'pets/miso.pixa',
                  sizeBytes: Int64(bytes.length),
                ),
              ),
            ),
          ).writeToBuffer(),
        ),
        encodeFrame(rpcFrameTypeBinary, bytes),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );

    final result = await future;
    expect(result.metadata.value.petId, 'pet-miso');
    expect(result.bytes, bytes);
    expect(result.asset.clips.single.name, 'default');
  });

  test('downloads and validates badgedef pixa resources', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);
    final bytes = makePixa(clips: ['icon']);

    final future = client.downloadBadgeDefPixa('badge-heart');
    await Future<void>.delayed(Duration.zero);
    final request = rpc.RpcRequest.fromBuffer(
      decodeFrames(factory.channels.single.sent.single).first.payload,
    );

    factory.channels.single.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(
          rpc.RpcResponse(
            id: request.id,
            payload: encodeRpcResponsePayload(
              'server.badge_def.pixa.download',
              payload.BadgeDefPixaDownloadResponse(
                id: 'badge-heart',
                pixaPath: 'badges/heart.pixa',
                sizeBytes: Int64(bytes.length),
              ),
            ),
          ).writeToBuffer(),
        ),
        encodeFrame(rpcFrameTypeBinary, bytes),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );

    final result = await future;
    expect(result.metadata.id, 'badge-heart');
    expect(selectPixaClip(result.asset)?.name, 'icon');
  });
}
