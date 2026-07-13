import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw/src/generated/rpc/common.pb.dart' as common;
import 'package:gizclaw/src/generated/rpc/peer.pb.dart' as peer;
import 'package:protobuf/protobuf.dart';
import 'package:test/test.dart';

import 'fake_transport.dart';

void main() {
  test('lists, presents, adopts, and drives pets', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);

    final listFuture = client.listPets(cursor: 'next', limit: 20);
    final listRequest = await _request(factory, 0);
    final listPayload =
        decodeRpcRequestPayload('server.pet.list', listRequest.payload)
            as ServerPetListRequest;
    expect(listPayload.value.cursor, 'next');
    expect(listPayload.value.limit.toInt(), 20);
    _respond(
      factory.channels[0],
      listRequest.id,
      'server.pet.list',
      ServerPetListResponse(
        value: PetListResponse(items: [Pet(id: 'pet-a')]),
      ),
    );
    expect((await listFuture).value.items.single.id, 'pet-a');

    final presentationFuture = client.getPetPresentation('pet-a');
    final presentationRequest = await _request(factory, 1);
    final presentationPayload =
        decodeRpcRequestPayload(
              'server.pet.presentation.get',
              presentationRequest.payload,
            )
            as ServerPetPresentationGetRequest;
    expect(presentationPayload.value.id, 'pet-a');
    _respond(
      factory.channels[1],
      presentationRequest.id,
      'server.pet.presentation.get',
      ServerPetPresentationGetResponse(value: PetPresentation(petId: 'pet-a')),
    );
    expect((await presentationFuture).value.petId, 'pet-a');

    final adoptFuture = client.adoptPet(displayName: 'Miso');
    final adoptRequest = await _request(factory, 2);
    final adoptPayload =
        decodeRpcRequestPayload('server.pet.adopt', adoptRequest.payload)
            as ServerPetAdoptRequest;
    expect(adoptPayload.value.displayName, 'Miso');
    _respond(
      factory.channels[2],
      adoptRequest.id,
      'server.pet.adopt',
      ServerPetAdoptResponse(
        value: PetAdoptResponse(pet: Pet(id: 'pet-b')),
      ),
    );
    expect((await adoptFuture).value.pet.id, 'pet-b');

    final driveFuture = client.drivePet('pet-b', action: 'bath');
    final driveRequest = await _request(factory, 3);
    final drivePayload =
        decodeRpcRequestPayload('server.pet.drive', driveRequest.payload)
            as ServerPetDriveRequest;
    expect(drivePayload.value.petId, 'pet-b');
    expect(drivePayload.value.action, 'bath');
    _respond(
      factory.channels[3],
      driveRequest.id,
      'server.pet.drive',
      ServerPetDriveResponse(
        value: PetDriveResponse(pet: Pet(id: 'pet-b')),
      ),
    );
    expect((await driveFuture).value.pet.id, 'pet-b');
  });
}

Future<peer.RpcRequest> _request(
  FakeDataChannelFactory factory,
  int index,
) async {
  while (factory.channels.length <= index ||
      factory.channels[index].sent.isEmpty) {
    await Future<void>.delayed(Duration.zero);
  }
  final frame = decodeFrames(factory.channels[index].sent.single).first;
  return peer.RpcRequest.fromBuffer(frame.payload);
}

void _respond(
  FakeDataChannel channel,
  String id,
  String method,
  GeneratedMessage response,
) {
  channel.addMessage(
    concatBytes([
      ...encodeEnvelopeFrames(
        common.RpcResponse(
          id: id,
          payload: encodeRpcResponsePayload(method, response),
        ).writeToBuffer(),
      ),
      encodeFrame(rpcFrameTypeEos),
    ]),
  );
}
