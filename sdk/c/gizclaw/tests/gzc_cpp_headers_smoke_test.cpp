#include "gzc.h"

static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_WORKFLOW_CREATE == 35);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_WORKFLOW_PUT == 36);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_WORKFLOW_DELETE == 37);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_RUNTIME_ADOPT == 82);

int main() {
  gizclaw_rpc_v1_WorkflowListRequest workflows = gizclaw_rpc_v1_WorkflowListRequest_init_zero;
  workflows.source = gizclaw_rpc_v1_ResourceSource_RESOURCE_SOURCE_RUNTIME;

  gizclaw_rpc_v1_WorkflowCreateRequest create = gizclaw_rpc_v1_WorkflowCreateRequest_init_zero;
  create.source = gizclaw_rpc_v1_ResourceSource_RESOURCE_SOURCE_OWNED;

  gizclaw_rpc_v1_WorkspaceUpsert workspace = gizclaw_rpc_v1_WorkspaceUpsert_init_zero;
  workspace.has_workflow_source = true;
  workspace.workflow_source = gizclaw_rpc_v1_ResourceSource_RESOURCE_SOURCE_RUNTIME;
  return workflows.source == create.source || !workspace.has_workflow_source;
}
