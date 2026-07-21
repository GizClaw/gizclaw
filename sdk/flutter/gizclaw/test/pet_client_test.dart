import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
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

    final actionsFuture = client.getPetActions('pet-a');
    final actionsRequest = await _request(factory, 1);
    final actionsPayload =
        decodeRpcRequestPayload(
              'server.pet.actions.get',
              actionsRequest.payload,
            )
            as ServerPetActionsGetRequest;
    expect(actionsPayload.value.id, 'pet-a');
    _respond(
      factory.channels[1],
      actionsRequest.id,
      'server.pet.actions.get',
      ServerPetActionsGetResponse(value: PetActions(petId: 'pet-a')),
    );
    expect((await actionsFuture).value.petId, 'pet-a');

    final adoptFuture = client.adoptPet(displayName: 'Miso');
    final adoptRequest = await _request(factory, 2);
    final adoptPayload =
        decodeRpcRequestPayload('runtime.adopt', adoptRequest.payload)
            as RuntimeAdoptRequest;
    expect(adoptPayload.value.displayName, 'Miso');
    _respond(
      factory.channels[2],
      adoptRequest.id,
      'runtime.adopt',
      RuntimeAdoptResponse(
        value: PetAdoptResponse(pet: Pet(id: 'pet-b')),
      ),
    );
    expect((await adoptFuture).value.pet.id, 'pet-b');

    final driveFuture = client.drivePet(
      'pet-b',
      behavior: PetBehavior.PET_BEHAVIOR_BATHE,
      idempotencyKey: 'care-1',
    );
    final driveRequest = await _request(factory, 3);
    final drivePayload =
        decodeRpcRequestPayload('server.pet.drive', driveRequest.payload)
            as ServerPetDriveRequest;
    expect(drivePayload.value.petId, 'pet-b');
    expect(drivePayload.value.behavior, PetBehavior.PET_BEHAVIOR_BATHE);
    expect(drivePayload.value.idempotencyKey, 'care-1');
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

Future<rpc.RpcRequest> _request(
  FakeDataChannelFactory factory,
  int index,
) async {
  while (factory.channels.length <= index ||
      factory.channels[index].sent.isEmpty) {
    await Future<void>.delayed(Duration.zero);
  }
  final frame = decodeFrames(factory.channels[index].sent.single).first;
  return rpc.RpcRequest.fromBuffer(frame.payload);
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
        rpc.RpcResponse(
          id: id,
          payload: encodeRpcResponsePayload(method, response),
        ).writeToBuffer(),
      ),
      encodeFrame(rpcFrameTypeEos),
    ]),
  );
}
