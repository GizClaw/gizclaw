package gizclaw

import (
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func rpcNotImplemented(id string, method rpcapi.RPCMethod) *rpcapi.RPCResponse {
	return rpcapi.Error{
		RequestID: id,
		Code:      rpcapi.RPCErrorCodeMethodNotFound,
		Message:   fmt.Sprintf("method not implemented: %s", method),
	}.RPCResponse()
}

func isPlannedServerMethod(method rpcapi.RPCMethod) bool {
	switch method {
	case rpcapi.RPCMethodServerFirmwareList,
		rpcapi.RPCMethodServerFirmwareGet,
		rpcapi.RPCMethodServerFirmwareFilesDownload,
		rpcapi.RPCMethodServerWorkspaceList,
		rpcapi.RPCMethodServerWorkspaceGet,
		rpcapi.RPCMethodServerWorkspaceCreate,
		rpcapi.RPCMethodServerWorkspacePut,
		rpcapi.RPCMethodServerWorkspaceDelete,
		rpcapi.RPCMethodServerWorkflowList,
		rpcapi.RPCMethodServerWorkflowGet,
		rpcapi.RPCMethodServerWorkflowCreate,
		rpcapi.RPCMethodServerWorkflowPut,
		rpcapi.RPCMethodServerWorkflowDelete,
		rpcapi.RPCMethodServerModelList,
		rpcapi.RPCMethodServerModelGet,
		rpcapi.RPCMethodServerModelCreate,
		rpcapi.RPCMethodServerModelPut,
		rpcapi.RPCMethodServerModelDelete,
		rpcapi.RPCMethodServerCredentialList,
		rpcapi.RPCMethodServerCredentialGet,
		rpcapi.RPCMethodServerCredentialCreate,
		rpcapi.RPCMethodServerCredentialPut,
		rpcapi.RPCMethodServerCredentialDelete,
		rpcapi.RPCMethodServerContactList,
		rpcapi.RPCMethodServerContactGet,
		rpcapi.RPCMethodServerContactCreate,
		rpcapi.RPCMethodServerContactPut,
		rpcapi.RPCMethodServerContactDelete,
		rpcapi.RPCMethodServerFriendInviteTokenGet,
		rpcapi.RPCMethodServerFriendInviteTokenCreate,
		rpcapi.RPCMethodServerFriendInviteTokenClear,
		rpcapi.RPCMethodServerFriendAdd,
		rpcapi.RPCMethodServerFriendList,
		rpcapi.RPCMethodServerFriendDelete,
		rpcapi.RPCMethodServerFriendGroupList,
		rpcapi.RPCMethodServerFriendGroupGet,
		rpcapi.RPCMethodServerFriendGroupCreate,
		rpcapi.RPCMethodServerFriendGroupPut,
		rpcapi.RPCMethodServerFriendGroupDelete,
		rpcapi.RPCMethodServerFriendGroupInviteTokenGet,
		rpcapi.RPCMethodServerFriendGroupInviteTokenCreate,
		rpcapi.RPCMethodServerFriendGroupInviteTokenClear,
		rpcapi.RPCMethodServerFriendGroupJoin,
		rpcapi.RPCMethodServerFriendGroupMembersList,
		rpcapi.RPCMethodServerFriendGroupMembersAdd,
		rpcapi.RPCMethodServerFriendGroupMembersPut,
		rpcapi.RPCMethodServerFriendGroupMembersDelete,
		rpcapi.RPCMethodServerFriendGroupMessagesList,
		rpcapi.RPCMethodServerFriendGroupMessagesGet,
		rpcapi.RPCMethodServerFriendGroupMessagesSend:
		return true
	default:
		return false
	}
}
