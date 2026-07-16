import 'package:fixnum/fixnum.dart';
import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
import 'package:gizclaw/gizclaw.dart';
import 'package:test/test.dart';

import 'fake_transport.dart';

void main() {
  test('downloads an authorized shared asset', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);
    const ref = 'asset://7d9c87aa1a224de6b93082026f30c77e';
    final body = <int>[1, 2, 3, 4];

    final future = client.downloadAsset(ref);
    await Future<void>.delayed(Duration.zero);
    final channel = factory.channels.single;
    final request = rpc.RpcRequest.fromBuffer(
      decodeFrames(channel.sent.single).first.payload,
    );
    expect(request.method, rpc.RpcMethod.RPC_METHOD_SERVER_ASSET_DOWNLOAD);
    final params =
        decodeRpcRequestPayload('server.asset.download', request.payload)
            as AssetDownloadRequest;
    expect(params.ref, ref);

    final metadata = AssetMetadata(
      ref: ref,
      mediaType: 'image/png',
      sizeBytes: Int64(body.length),
      sha256:
          '9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08',
      createdAt: '2026-07-16T08:00:00Z',
    );
    channel.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(
          rpc.RpcResponse(
            id: request.id,
            payload: encodeRpcResponsePayload(
              'server.asset.download',
              AssetDownloadResponse(metadata: metadata),
            ),
          ).writeToBuffer(),
        ),
        encodeFrame(rpcFrameTypeBinary, body),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );

    final result = await future;
    expect(result.metadata.ref, ref);
    expect(result.metadata.mediaType, 'image/png');
    expect(result.bytes, body);
  });
}
