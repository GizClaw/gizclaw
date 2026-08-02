import 'package:gizclaw/src/client.dart';
import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
import 'package:gizclaw/src/generated/rpc/payload.pb.dart' as payload;
import 'package:gizclaw/src/payload_codec.dart';
import 'package:gizclaw/src/rpc_frame.dart';
import 'package:protobuf/protobuf.dart';
import 'package:test/test.dart';

import 'fake_transport.dart';

void main() {
  test('lists friend and group chat workspace references', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);

    final friendsFuture = client.listFriends(
      cursor: 'friend-cursor',
      limit: 20,
    );
    final friendRequest = await _request(factory, 0);
    final friendPayload =
        decodeRpcRequestPayload('server.friend.list', friendRequest.payload)
            as payload.FriendListRequest;
    expect(friendPayload.cursor, 'friend-cursor');
    expect(friendPayload.limit.toInt(), 20);
    _respond(
      factory.channels[0],
      friendRequest.id,
      'server.friend.list',
      payload.FriendListResponse(
        items: [payload.FriendObject(workspaceName: 'social-direct-a')],
      ),
    );
    expect((await friendsFuture).items.single.workspaceName, 'social-direct-a');

    final groupsFuture = client.listFriendGroups(
      cursor: 'group-cursor',
      limit: 30,
    );
    final groupRequest = await _request(factory, 1);
    final groupPayload =
        decodeRpcRequestPayload(
              'server.friend_group.list',
              groupRequest.payload,
            )
            as payload.FriendGroupListRequest;
    expect(groupPayload.cursor, 'group-cursor');
    expect(groupPayload.limit.toInt(), 30);
    _respond(
      factory.channels[1],
      groupRequest.id,
      'server.friend_group.list',
      payload.FriendGroupListResponse(
        items: [payload.FriendGroupObject(workspaceName: 'social-group-a')],
      ),
    );
    expect((await groupsFuture).items.single.workspaceName, 'social-group-a');

    final createGroupFuture = client.createFriendGroup(
      name: 'studio',
      description: 'Daily voice room',
    );
    final createGroupRequest = await _request(factory, 2);
    final createGroupPayload =
        decodeRpcRequestPayload(
              'server.friend_group.create',
              createGroupRequest.payload,
            )
            as payload.FriendGroupCreateRequest;
    expect(createGroupPayload.name, 'studio');
    expect(createGroupPayload.description, 'Daily voice room');
    _respond(
      factory.channels[2],
      createGroupRequest.id,
      'server.friend_group.create',
      payload.FriendGroupCreateResponse(
        value: payload.FriendGroupObject(
          name: 'studio',
          workspaceName: 'social-group-studio',
        ),
      ),
    );
    expect(
      (await createGroupFuture).value.workspaceName,
      'social-group-studio',
    );
  });

  test('manages friend invite lifecycle and relations', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);

    final getFuture = client.getFriendInviteToken();
    final getRequest = await _request(factory, 0);
    expect(
      decodeRpcRequestPayload(
        'server.friend.invite_token.get',
        getRequest.payload,
      ),
      isA<payload.FriendInviteTokenGetRequest>(),
    );
    _respond(
      factory.channels[0],
      getRequest.id,
      'server.friend.invite_token.get',
      payload.FriendInviteTokenGetResponse(inviteToken: 'invite-a'),
    );
    expect((await getFuture).inviteToken, 'invite-a');

    final createFuture = client.createFriendInviteToken();
    final createRequest = await _request(factory, 1);
    _respond(
      factory.channels[1],
      createRequest.id,
      'server.friend.invite_token.create',
      payload.FriendInviteTokenCreateResponse(
        inviteToken: 'invite-b',
        expiresAt: '2026-07-13T00:00:00Z',
      ),
    );
    expect((await createFuture).inviteToken, 'invite-b');

    final addFuture = client.addFriend('invite-peer');
    final addRequest = await _request(factory, 2);
    final addPayload =
        decodeRpcRequestPayload('server.friend.add', addRequest.payload)
            as payload.FriendAddRequest;
    expect(addPayload.inviteToken, 'invite-peer');
    _respond(
      factory.channels[2],
      addRequest.id,
      'server.friend.add',
      payload.FriendAddResponse(
        value: payload.FriendObject(
          id: 'peer-b',
          workspaceName: 'social-direct-a',
        ),
      ),
    );
    expect((await addFuture).value.workspaceName, 'social-direct-a');

    final deleteFuture = client.deleteFriend('peer-b');
    final deleteRequest = await _request(factory, 3);
    final deletePayload =
        decodeRpcRequestPayload('server.friend.delete', deleteRequest.payload)
            as payload.FriendDeleteRequest;
    expect(deletePayload.id, 'peer-b');
    _respond(
      factory.channels[3],
      deleteRequest.id,
      'server.friend.delete',
      payload.FriendDeleteResponse(value: payload.FriendObject(id: 'peer-b')),
    );
    expect((await deleteFuture).value.id, 'peer-b');

    final clearFuture = client.clearFriendInviteToken();
    final clearRequest = await _request(factory, 4);
    _respond(
      factory.channels[4],
      clearRequest.id,
      'server.friend.invite_token.clear',
      payload.FriendInviteTokenClearResponse(),
    );
    await clearFuture;
  });

  test('manages friend group invite lifecycle and joining', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);

    final getFuture = client.getFriendGroupInviteToken('group-a');
    final getRequest = await _request(factory, 0);
    final getPayload =
        decodeRpcRequestPayload(
              'server.friend_group.invite_token.get',
              getRequest.payload,
            )
            as payload.FriendGroupInviteTokenGetRequest;
    expect(getPayload.friendGroupName, 'group-a');
    _respond(
      factory.channels[0],
      getRequest.id,
      'server.friend_group.invite_token.get',
      payload.FriendGroupInviteTokenGetResponse(inviteToken: 'invite-a'),
    );
    expect((await getFuture).inviteToken, 'invite-a');

    final createFuture = client.createFriendGroupInviteToken('group-a');
    final createRequest = await _request(factory, 1);
    final createPayload =
        decodeRpcRequestPayload(
              'server.friend_group.invite_token.create',
              createRequest.payload,
            )
            as payload.FriendGroupInviteTokenCreateRequest;
    expect(createPayload.friendGroupName, 'group-a');
    _respond(
      factory.channels[1],
      createRequest.id,
      'server.friend_group.invite_token.create',
      payload.FriendGroupInviteTokenCreateResponse(
        inviteToken: 'invite-b',
        expiresAt: '2026-07-13T00:00:00Z',
      ),
    );
    expect((await createFuture).inviteToken, 'invite-b');

    final joinFuture = client.joinFriendGroup(
      name: ' group-local ',
      inviteToken: ' invite-group ',
    );
    final joinRequest = await _request(factory, 2);
    final joinPayload =
        decodeRpcRequestPayload('server.friend_group.join', joinRequest.payload)
            as payload.FriendGroupJoinRequest;
    expect(joinPayload.inviteToken, 'invite-group');
    expect(joinPayload.name, 'group-local');
    _respond(
      factory.channels[2],
      joinRequest.id,
      'server.friend_group.join',
      payload.FriendGroupJoinResponse(
        group: payload.FriendGroupObject(
          name: 'group-a',
          workspaceName: 'social-group-a',
        ),
        member: payload.FriendGroupMemberObject(
          friendGroupName: 'group-a',
          peerPublicKey: 'peer-b',
        ),
      ),
    );
    expect((await joinFuture).group.workspaceName, 'social-group-a');

    final clearFuture = client.clearFriendGroupInviteToken('group-a');
    final clearRequest = await _request(factory, 3);
    final clearPayload =
        decodeRpcRequestPayload(
              'server.friend_group.invite_token.clear',
              clearRequest.payload,
            )
            as payload.FriendGroupInviteTokenClearRequest;
    expect(clearPayload.friendGroupName, 'group-a');
    _respond(
      factory.channels[3],
      clearRequest.id,
      'server.friend_group.invite_token.clear',
      payload.FriendGroupInviteTokenClearResponse(),
    );
    await clearFuture;
  });

  test('manages friend group membership and deletion', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);

    final listFuture = client.listFriendGroupMembers(
      'group-a',
      cursor: 'member-cursor',
      limit: 20,
    );
    final listRequest = await _request(factory, 0);
    final listPayload =
        decodeRpcRequestPayload(
              'server.friend_group.members.list',
              listRequest.payload,
            )
            as payload.FriendGroupMemberListRequest;
    expect(listPayload.friendGroupName, 'group-a');
    expect(listPayload.cursor, 'member-cursor');
    expect(listPayload.limit.toInt(), 20);
    _respond(
      factory.channels[0],
      listRequest.id,
      'server.friend_group.members.list',
      payload.FriendGroupMemberListResponse(
        items: [
          payload.FriendGroupMemberObject(
            friendGroupName: 'group-a',
            id: 'peer-b',
            peerPublicKey: 'peer-b',
          ),
        ],
      ),
    );
    expect((await listFuture).items.single.peerPublicKey, 'peer-b');

    final removeMemberFuture = client.deleteFriendGroupMember(
      'group-a',
      'peer-b',
    );
    final removeMemberRequest = await _request(factory, 1);
    final removeMemberPayload =
        decodeRpcRequestPayload(
              'server.friend_group.members.delete',
              removeMemberRequest.payload,
            )
            as payload.FriendGroupMemberDeleteRequest;
    expect(removeMemberPayload.friendGroupName, 'group-a');
    expect(removeMemberPayload.id, 'peer-b');
    _respond(
      factory.channels[1],
      removeMemberRequest.id,
      'server.friend_group.members.delete',
      payload.FriendGroupMemberDeleteResponse(
        value: payload.FriendGroupMemberObject(
          friendGroupName: 'group-a',
          id: 'peer-b',
        ),
      ),
    );
    expect((await removeMemberFuture).value.id, 'peer-b');

    final deleteGroupFuture = client.deleteFriendGroup('group-a');
    final deleteGroupRequest = await _request(factory, 2);
    final deleteGroupPayload =
        decodeRpcRequestPayload(
              'server.friend_group.delete',
              deleteGroupRequest.payload,
            )
            as payload.FriendGroupDeleteRequest;
    expect(deleteGroupPayload.name, 'group-a');
    _respond(
      factory.channels[2],
      deleteGroupRequest.id,
      'server.friend_group.delete',
      payload.FriendGroupDeleteResponse(
        value: payload.FriendGroupObject(name: 'group-a'),
      ),
    );
    expect((await deleteGroupFuture).value.name, 'group-a');
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
  final frames = decodeFrames(factory.channels[index].sent.single);
  return rpc.RpcRequest.fromBuffer(frames.first.payload);
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
