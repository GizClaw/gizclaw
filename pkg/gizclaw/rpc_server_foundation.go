package gizclaw

import (
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
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
		rpcapi.RPCMethodServerFirmwareDownload,
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
		rpcapi.RPCMethodServerPetList,
		rpcapi.RPCMethodServerPetGet,
		rpcapi.RPCMethodServerPetAdopt,
		rpcapi.RPCMethodServerPetPut,
		rpcapi.RPCMethodServerPetDelete,
		rpcapi.RPCMethodServerPetFeed,
		rpcapi.RPCMethodServerPetWash,
		rpcapi.RPCMethodServerPetPlay,
		rpcapi.RPCMethodServerWalletGet,
		rpcapi.RPCMethodServerWalletTransactionsList,
		rpcapi.RPCMethodServerWalletTransactionsGet,
		rpcapi.RPCMethodServerContactList,
		rpcapi.RPCMethodServerContactGet,
		rpcapi.RPCMethodServerContactCreate,
		rpcapi.RPCMethodServerContactPut,
		rpcapi.RPCMethodServerContactDelete,
		rpcapi.RPCMethodServerFriendRequestsList,
		rpcapi.RPCMethodServerFriendRequestsCreate,
		rpcapi.RPCMethodServerFriendRequestsAccept,
		rpcapi.RPCMethodServerFriendRequestsReject,
		rpcapi.RPCMethodServerFriendList,
		rpcapi.RPCMethodServerFriendDelete,
		rpcapi.RPCMethodServerFriendGroupList,
		rpcapi.RPCMethodServerFriendGroupGet,
		rpcapi.RPCMethodServerFriendGroupCreate,
		rpcapi.RPCMethodServerFriendGroupPut,
		rpcapi.RPCMethodServerFriendGroupDelete,
		rpcapi.RPCMethodServerFriendGroupMembersList,
		rpcapi.RPCMethodServerFriendGroupMembersAdd,
		rpcapi.RPCMethodServerFriendGroupMembersPut,
		rpcapi.RPCMethodServerFriendGroupMembersDelete,
		rpcapi.RPCMethodServerFriendGroupMessagesList,
		rpcapi.RPCMethodServerFriendGroupMessagesGet,
		rpcapi.RPCMethodServerFriendGroupMessagesSend,
		rpcapi.RPCMethodServerRewardList,
		rpcapi.RPCMethodServerRewardGet,
		rpcapi.RPCMethodServerRewardClaim:
		return true
	default:
		return false
	}
}
