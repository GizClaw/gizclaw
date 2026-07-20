#include "gzc.h"

static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_WORKFLOW_LIST == 33);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_WORKFLOW_GET == 34);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_RUNTIME_ADOPT == 69);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_SPEECH_TRANSCRIBE == 93);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_SPEECH_SYNTHESIZE == 94);

int main() {
  gizclaw_rpc_v1_WorkflowListRequest workflows = gizclaw_rpc_v1_WorkflowListRequest_init_zero;
  gizclaw_rpc_v1_WorkspaceCreateBody workspace = gizclaw_rpc_v1_WorkspaceCreateBody_init_zero;
  return workflows.has_limit || workspace.has_parameters;
}
