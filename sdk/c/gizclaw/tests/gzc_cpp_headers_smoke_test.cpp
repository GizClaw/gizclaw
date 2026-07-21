#include "gzc.h"

static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_FIRMWARE_GET == 22);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_WORKFLOW_LIST == 32);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_WORKFLOW_GET == 33);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_RUNTIME_ADOPT == 68);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_SPEECH_TRANSCRIBE == 92);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_SPEECH_SYNTHESIZE == 93);

int main() {
  gizclaw_rpc_v1_WorkflowListRequest workflows = gizclaw_rpc_v1_WorkflowListRequest_init_zero;
  gizclaw_rpc_v1_WorkspaceCreateBody workspace = gizclaw_rpc_v1_WorkspaceCreateBody_init_zero;
  return workflows.has_limit || workspace.has_parameters;
}
