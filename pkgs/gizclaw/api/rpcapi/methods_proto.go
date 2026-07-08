package rpcapi

import (
	"fmt"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

var rpcMethodToProto = map[RPCMethod]rpcpb.RpcMethod{
	RPCMethodAllPing:                            rpcpb.RpcMethod_RPC_METHOD_ALL_PING,
	RPCMethodAllSpeedTestRun:                    rpcpb.RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN,
	RPCMethodClientInfoGet:                      rpcpb.RpcMethod_RPC_METHOD_CLIENT_INFO_GET,
	RPCMethodClientIdentifiersGet:               rpcpb.RpcMethod_RPC_METHOD_CLIENT_IDENTIFIERS_GET,
	RPCMethodServerInfoGet:                      rpcpb.RpcMethod_RPC_METHOD_SERVER_INFO_GET,
	RPCMethodServerInfoPut:                      rpcpb.RpcMethod_RPC_METHOD_SERVER_INFO_PUT,
	RPCMethodServerRuntimeGet:                   rpcpb.RpcMethod_RPC_METHOD_SERVER_RUNTIME_GET,
	RPCMethodServerStatusGet:                    rpcpb.RpcMethod_RPC_METHOD_SERVER_STATUS_GET,
	RPCMethodServerRunAgentGet:                  rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_AGENT_GET,
	RPCMethodServerRunAgentSet:                  rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_AGENT_SET,
	RPCMethodServerRunWorkspaceGet:              rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_GET,
	RPCMethodServerRunWorkspaceSet:              rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_SET,
	RPCMethodServerRunWorkspaceReload:           rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_RELOAD,
	RPCMethodServerRunWorkspaceHistory:          rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_HISTORY,
	RPCMethodServerRunWorkspaceHistoryPlay:      rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_HISTORY_PLAY,
	RPCMethodServerRunWorkspaceMemoryStats:      rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_MEMORY_STATS,
	RPCMethodServerRunWorkspaceRecall:           rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_RECALL,
	RPCMethodServerRunReload:                    rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_RELOAD,
	RPCMethodServerRunStatus:                    rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_STATUS,
	RPCMethodServerRunStop:                      rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_STOP,
	RPCMethodServerRunSay:                       rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_SAY,
	RPCMethodServerFirmwareList:                 rpcpb.RpcMethod_RPC_METHOD_SERVER_FIRMWARE_LIST,
	RPCMethodServerFirmwareGet:                  rpcpb.RpcMethod_RPC_METHOD_SERVER_FIRMWARE_GET,
	RPCMethodServerFirmwareFilesDownload:        rpcpb.RpcMethod_RPC_METHOD_SERVER_FIRMWARE_FILES_DOWNLOAD,
	RPCMethodServerWorkspaceList:                rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_LIST,
	RPCMethodServerWorkspaceGet:                 rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_GET,
	RPCMethodServerWorkspaceCreate:              rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_CREATE,
	RPCMethodServerWorkspacePut:                 rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_PUT,
	RPCMethodServerWorkspaceDelete:              rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_DELETE,
	RPCMethodServerWorkspaceHistoryList:         rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_HISTORY_LIST,
	RPCMethodServerWorkspaceHistoryGet:          rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_HISTORY_GET,
	RPCMethodServerWorkspaceHistoryAudioGet:     rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_HISTORY_AUDIO_GET,
	RPCMethodServerWorkflowList:                 rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKFLOW_LIST,
	RPCMethodServerWorkflowGet:                  rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKFLOW_GET,
	RPCMethodServerWorkflowCreate:               rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKFLOW_CREATE,
	RPCMethodServerWorkflowPut:                  rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKFLOW_PUT,
	RPCMethodServerWorkflowDelete:               rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKFLOW_DELETE,
	RPCMethodServerModelList:                    rpcpb.RpcMethod_RPC_METHOD_SERVER_MODEL_LIST,
	RPCMethodServerModelGet:                     rpcpb.RpcMethod_RPC_METHOD_SERVER_MODEL_GET,
	RPCMethodServerModelCreate:                  rpcpb.RpcMethod_RPC_METHOD_SERVER_MODEL_CREATE,
	RPCMethodServerModelPut:                     rpcpb.RpcMethod_RPC_METHOD_SERVER_MODEL_PUT,
	RPCMethodServerModelDelete:                  rpcpb.RpcMethod_RPC_METHOD_SERVER_MODEL_DELETE,
	RPCMethodServerVoiceList:                    rpcpb.RpcMethod_RPC_METHOD_SERVER_VOICE_LIST,
	RPCMethodServerVoiceGet:                     rpcpb.RpcMethod_RPC_METHOD_SERVER_VOICE_GET,
	RPCMethodServerCredentialList:               rpcpb.RpcMethod_RPC_METHOD_SERVER_CREDENTIAL_LIST,
	RPCMethodServerCredentialGet:                rpcpb.RpcMethod_RPC_METHOD_SERVER_CREDENTIAL_GET,
	RPCMethodServerCredentialCreate:             rpcpb.RpcMethod_RPC_METHOD_SERVER_CREDENTIAL_CREATE,
	RPCMethodServerCredentialPut:                rpcpb.RpcMethod_RPC_METHOD_SERVER_CREDENTIAL_PUT,
	RPCMethodServerCredentialDelete:             rpcpb.RpcMethod_RPC_METHOD_SERVER_CREDENTIAL_DELETE,
	RPCMethodServerContactList:                  rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_LIST,
	RPCMethodServerContactGet:                   rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_GET,
	RPCMethodServerContactCreate:                rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_CREATE,
	RPCMethodServerContactPut:                   rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_PUT,
	RPCMethodServerContactDelete:                rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_DELETE,
	RPCMethodServerFriendInviteTokenGet:         rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_GET,
	RPCMethodServerFriendInviteTokenCreate:      rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CREATE,
	RPCMethodServerFriendInviteTokenClear:       rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CLEAR,
	RPCMethodServerFriendAdd:                    rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_ADD,
	RPCMethodServerFriendList:                   rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_LIST,
	RPCMethodServerFriendDelete:                 rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_DELETE,
	RPCMethodServerFriendGroupList:              rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_LIST,
	RPCMethodServerFriendGroupGet:               rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_GET,
	RPCMethodServerFriendGroupCreate:            rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_CREATE,
	RPCMethodServerFriendGroupPut:               rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_PUT,
	RPCMethodServerFriendGroupDelete:            rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_DELETE,
	RPCMethodServerFriendGroupInviteTokenGet:    rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_GET,
	RPCMethodServerFriendGroupInviteTokenCreate: rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CREATE,
	RPCMethodServerFriendGroupInviteTokenClear:  rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CLEAR,
	RPCMethodServerFriendGroupJoin:              rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_JOIN,
	RPCMethodServerFriendGroupMembersList:       rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_LIST,
	RPCMethodServerFriendGroupMembersAdd:        rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_ADD,
	RPCMethodServerFriendGroupMembersPut:        rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_PUT,
	RPCMethodServerFriendGroupMembersDelete:     rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_DELETE,
	RPCMethodServerFriendGroupMessagesList:      rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_LIST,
	RPCMethodServerFriendGroupMessagesGet:       rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_GET,
	RPCMethodServerFriendGroupMessagesSend:      rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_SEND,
	RPCMethodServerGameRulesetGet:               rpcpb.RpcMethod_RPC_METHOD_SERVER_GAME_RULESET_GET,
	RPCMethodServerPetDefPixaDownload:           rpcpb.RpcMethod_RPC_METHOD_SERVER_PET_DEF_PIXA_DOWNLOAD,
	RPCMethodServerBadgeDefPixaDownload:         rpcpb.RpcMethod_RPC_METHOD_SERVER_BADGE_DEF_PIXA_DOWNLOAD,
	RPCMethodServerPetList:                      rpcpb.RpcMethod_RPC_METHOD_SERVER_PET_LIST,
	RPCMethodServerPetGet:                       rpcpb.RpcMethod_RPC_METHOD_SERVER_PET_GET,
	RPCMethodServerPetAdopt:                     rpcpb.RpcMethod_RPC_METHOD_SERVER_PET_ADOPT,
	RPCMethodServerPetPut:                       rpcpb.RpcMethod_RPC_METHOD_SERVER_PET_PUT,
	RPCMethodServerPetDelete:                    rpcpb.RpcMethod_RPC_METHOD_SERVER_PET_DELETE,
	RPCMethodServerPetDrive:                     rpcpb.RpcMethod_RPC_METHOD_SERVER_PET_DRIVE,
	RPCMethodServerPointsGet:                    rpcpb.RpcMethod_RPC_METHOD_SERVER_POINTS_GET,
	RPCMethodServerPointsTransactionsList:       rpcpb.RpcMethod_RPC_METHOD_SERVER_POINTS_TRANSACTIONS_LIST,
	RPCMethodServerPointsTransactionsGet:        rpcpb.RpcMethod_RPC_METHOD_SERVER_POINTS_TRANSACTIONS_GET,
	RPCMethodServerBadgeList:                    rpcpb.RpcMethod_RPC_METHOD_SERVER_BADGE_LIST,
	RPCMethodServerBadgeGet:                     rpcpb.RpcMethod_RPC_METHOD_SERVER_BADGE_GET,
	RPCMethodServerGameResultList:               rpcpb.RpcMethod_RPC_METHOD_SERVER_GAME_RESULT_LIST,
	RPCMethodServerGameResultGet:                rpcpb.RpcMethod_RPC_METHOD_SERVER_GAME_RESULT_GET,
	RPCMethodServerRewardGrantList:              rpcpb.RpcMethod_RPC_METHOD_SERVER_REWARD_GRANT_LIST,
	RPCMethodServerRewardGrantGet:               rpcpb.RpcMethod_RPC_METHOD_SERVER_REWARD_GRANT_GET,
}

var rpcMethodFromProto = map[rpcpb.RpcMethod]RPCMethod{
	rpcpb.RpcMethod_RPC_METHOD_ALL_PING:                                RPCMethodAllPing,
	rpcpb.RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN:                      RPCMethodAllSpeedTestRun,
	rpcpb.RpcMethod_RPC_METHOD_CLIENT_INFO_GET:                         RPCMethodClientInfoGet,
	rpcpb.RpcMethod_RPC_METHOD_CLIENT_IDENTIFIERS_GET:                  RPCMethodClientIdentifiersGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_INFO_GET:                         RPCMethodServerInfoGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_INFO_PUT:                         RPCMethodServerInfoPut,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_RUNTIME_GET:                      RPCMethodServerRuntimeGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_STATUS_GET:                       RPCMethodServerStatusGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_AGENT_GET:                    RPCMethodServerRunAgentGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_AGENT_SET:                    RPCMethodServerRunAgentSet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_GET:                RPCMethodServerRunWorkspaceGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_SET:                RPCMethodServerRunWorkspaceSet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_RELOAD:             RPCMethodServerRunWorkspaceReload,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_HISTORY:            RPCMethodServerRunWorkspaceHistory,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_HISTORY_PLAY:       RPCMethodServerRunWorkspaceHistoryPlay,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_MEMORY_STATS:       RPCMethodServerRunWorkspaceMemoryStats,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_WORKSPACE_RECALL:             RPCMethodServerRunWorkspaceRecall,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_RELOAD:                       RPCMethodServerRunReload,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_STATUS:                       RPCMethodServerRunStatus,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_STOP:                         RPCMethodServerRunStop,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_SAY:                          RPCMethodServerRunSay,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FIRMWARE_LIST:                    RPCMethodServerFirmwareList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FIRMWARE_GET:                     RPCMethodServerFirmwareGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FIRMWARE_FILES_DOWNLOAD:          RPCMethodServerFirmwareFilesDownload,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_LIST:                   RPCMethodServerWorkspaceList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_GET:                    RPCMethodServerWorkspaceGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_CREATE:                 RPCMethodServerWorkspaceCreate,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_PUT:                    RPCMethodServerWorkspacePut,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_DELETE:                 RPCMethodServerWorkspaceDelete,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_HISTORY_LIST:           RPCMethodServerWorkspaceHistoryList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_HISTORY_GET:            RPCMethodServerWorkspaceHistoryGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_HISTORY_AUDIO_GET:      RPCMethodServerWorkspaceHistoryAudioGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKFLOW_LIST:                    RPCMethodServerWorkflowList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKFLOW_GET:                     RPCMethodServerWorkflowGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKFLOW_CREATE:                  RPCMethodServerWorkflowCreate,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKFLOW_PUT:                     RPCMethodServerWorkflowPut,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKFLOW_DELETE:                  RPCMethodServerWorkflowDelete,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_MODEL_LIST:                       RPCMethodServerModelList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_MODEL_GET:                        RPCMethodServerModelGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_MODEL_CREATE:                     RPCMethodServerModelCreate,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_MODEL_PUT:                        RPCMethodServerModelPut,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_MODEL_DELETE:                     RPCMethodServerModelDelete,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_VOICE_LIST:                       RPCMethodServerVoiceList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_VOICE_GET:                        RPCMethodServerVoiceGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_CREDENTIAL_LIST:                  RPCMethodServerCredentialList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_CREDENTIAL_GET:                   RPCMethodServerCredentialGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_CREDENTIAL_CREATE:                RPCMethodServerCredentialCreate,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_CREDENTIAL_PUT:                   RPCMethodServerCredentialPut,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_CREDENTIAL_DELETE:                RPCMethodServerCredentialDelete,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_LIST:                     RPCMethodServerContactList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_GET:                      RPCMethodServerContactGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_CREATE:                   RPCMethodServerContactCreate,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_PUT:                      RPCMethodServerContactPut,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_CONTACT_DELETE:                   RPCMethodServerContactDelete,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_GET:          RPCMethodServerFriendInviteTokenGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CREATE:       RPCMethodServerFriendInviteTokenCreate,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CLEAR:        RPCMethodServerFriendInviteTokenClear,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_ADD:                       RPCMethodServerFriendAdd,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_LIST:                      RPCMethodServerFriendList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_DELETE:                    RPCMethodServerFriendDelete,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_LIST:                RPCMethodServerFriendGroupList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_GET:                 RPCMethodServerFriendGroupGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_CREATE:              RPCMethodServerFriendGroupCreate,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_PUT:                 RPCMethodServerFriendGroupPut,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_DELETE:              RPCMethodServerFriendGroupDelete,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_GET:    RPCMethodServerFriendGroupInviteTokenGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CREATE: RPCMethodServerFriendGroupInviteTokenCreate,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CLEAR:  RPCMethodServerFriendGroupInviteTokenClear,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_JOIN:                RPCMethodServerFriendGroupJoin,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_LIST:        RPCMethodServerFriendGroupMembersList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_ADD:         RPCMethodServerFriendGroupMembersAdd,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_PUT:         RPCMethodServerFriendGroupMembersPut,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_DELETE:      RPCMethodServerFriendGroupMembersDelete,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_LIST:       RPCMethodServerFriendGroupMessagesList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_GET:        RPCMethodServerFriendGroupMessagesGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_SEND:       RPCMethodServerFriendGroupMessagesSend,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_GAME_RULESET_GET:                 RPCMethodServerGameRulesetGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_PET_DEF_PIXA_DOWNLOAD:            RPCMethodServerPetDefPixaDownload,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_BADGE_DEF_PIXA_DOWNLOAD:          RPCMethodServerBadgeDefPixaDownload,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_PET_LIST:                         RPCMethodServerPetList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_PET_GET:                          RPCMethodServerPetGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_PET_ADOPT:                        RPCMethodServerPetAdopt,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_PET_PUT:                          RPCMethodServerPetPut,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_PET_DELETE:                       RPCMethodServerPetDelete,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_PET_DRIVE:                        RPCMethodServerPetDrive,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_POINTS_GET:                       RPCMethodServerPointsGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_POINTS_TRANSACTIONS_LIST:         RPCMethodServerPointsTransactionsList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_POINTS_TRANSACTIONS_GET:          RPCMethodServerPointsTransactionsGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_BADGE_LIST:                       RPCMethodServerBadgeList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_BADGE_GET:                        RPCMethodServerBadgeGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_GAME_RESULT_LIST:                 RPCMethodServerGameResultList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_GAME_RESULT_GET:                  RPCMethodServerGameResultGet,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_REWARD_GRANT_LIST:                RPCMethodServerRewardGrantList,
	rpcpb.RpcMethod_RPC_METHOD_SERVER_REWARD_GRANT_GET:                 RPCMethodServerRewardGrantGet,
}

func ProtoMethod(method RPCMethod) (rpcpb.RpcMethod, error) {
	protoMethod, ok := rpcMethodToProto[method]
	if !ok {
		return rpcpb.RpcMethod_RPC_METHOD_UNSPECIFIED, fmt.Errorf("rpc: unknown method %q", method)
	}
	return protoMethod, nil
}

func MethodFromProto(protoMethod rpcpb.RpcMethod) (RPCMethod, error) {
	method, ok := rpcMethodFromProto[protoMethod]
	if !ok {
		return "", fmt.Errorf("rpc: unknown method id %d", protoMethod)
	}
	return method, nil
}

func ValidateProtoMethodRegistry() error {
	if len(rpcMethodToProto) != len(rpcMethodFromProto) {
		return fmt.Errorf("rpc: method registry mismatch: %d names, %d ids", len(rpcMethodToProto), len(rpcMethodFromProto))
	}
	seenIDs := map[rpcpb.RpcMethod]RPCMethod{}
	for method, protoMethod := range rpcMethodToProto {
		if protoMethod == rpcpb.RpcMethod_RPC_METHOD_UNSPECIFIED {
			return fmt.Errorf("rpc: method %q uses unspecified id", method)
		}
		if prev, ok := seenIDs[protoMethod]; ok {
			return fmt.Errorf("rpc: methods %q and %q share id %d", prev, method, protoMethod)
		}
		seenIDs[protoMethod] = method
	}
	return nil
}
