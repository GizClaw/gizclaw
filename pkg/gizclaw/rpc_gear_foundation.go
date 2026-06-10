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

func isPlannedGearMethod(method rpcapi.RPCMethod) bool {
	switch method {
	case rpcapi.RPCMethodAudioSay,
		rpcapi.RPCMethodWorkspaceList,
		rpcapi.RPCMethodWorkspaceGet,
		rpcapi.RPCMethodWorkspaceCreate,
		rpcapi.RPCMethodWorkspacePut,
		rpcapi.RPCMethodWorkspaceDelete,
		rpcapi.RPCMethodWorkflowList,
		rpcapi.RPCMethodWorkflowGet,
		rpcapi.RPCMethodWorkflowCreate,
		rpcapi.RPCMethodWorkflowPut,
		rpcapi.RPCMethodWorkflowDelete,
		rpcapi.RPCMethodModelList,
		rpcapi.RPCMethodModelGet,
		rpcapi.RPCMethodModelCreate,
		rpcapi.RPCMethodModelPut,
		rpcapi.RPCMethodModelDelete,
		rpcapi.RPCMethodCredentialList,
		rpcapi.RPCMethodCredentialGet,
		rpcapi.RPCMethodCredentialCreate,
		rpcapi.RPCMethodCredentialPut,
		rpcapi.RPCMethodCredentialDelete,
		rpcapi.RPCMethodPetList,
		rpcapi.RPCMethodPetGet,
		rpcapi.RPCMethodPetCreate,
		rpcapi.RPCMethodPetPut,
		rpcapi.RPCMethodPetDelete,
		rpcapi.RPCMethodPetFeed,
		rpcapi.RPCMethodPetPlay,
		rpcapi.RPCMethodPetLevelUp,
		rpcapi.RPCMethodWalletGet,
		rpcapi.RPCMethodWalletTransactionsList,
		rpcapi.RPCMethodContactList,
		rpcapi.RPCMethodContactGet,
		rpcapi.RPCMethodContactCreate,
		rpcapi.RPCMethodContactPut,
		rpcapi.RPCMethodContactDelete,
		rpcapi.RPCMethodContactBlock,
		rpcapi.RPCMethodContactUnblock,
		rpcapi.RPCMethodFriendRequestsList,
		rpcapi.RPCMethodFriendRequestsCreate,
		rpcapi.RPCMethodFriendRequestsAccept,
		rpcapi.RPCMethodFriendRequestsReject,
		rpcapi.RPCMethodFriendList,
		rpcapi.RPCMethodFriendDelete,
		rpcapi.RPCMethodGroupList,
		rpcapi.RPCMethodGroupGet,
		rpcapi.RPCMethodGroupCreate,
		rpcapi.RPCMethodGroupPut,
		rpcapi.RPCMethodGroupDelete,
		rpcapi.RPCMethodGroupMembersList,
		rpcapi.RPCMethodGroupMembersAdd,
		rpcapi.RPCMethodGroupMembersDelete,
		rpcapi.RPCMethodGroupMessagesList,
		rpcapi.RPCMethodGroupMessagesSend,
		rpcapi.RPCMethodCallList,
		rpcapi.RPCMethodCallGet,
		rpcapi.RPCMethodCallCreate,
		rpcapi.RPCMethodCallAnswer,
		rpcapi.RPCMethodCallReject,
		rpcapi.RPCMethodCallEnd,
		rpcapi.RPCMethodGameResultsCreate,
		rpcapi.RPCMethodRewardList,
		rpcapi.RPCMethodRewardGet,
		rpcapi.RPCMethodRewardCreate,
		rpcapi.RPCMethodRewardClaim:
		return true
	default:
		return false
	}
}
