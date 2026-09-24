#include "gzc.h"
#include <type_traits>

static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_TOOL_V0_INVOKE == 135);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_TOOL_V0_LIST == 136);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_CLIENT_RPC_METHODS_LIST == 137);
static_assert(std::is_same_v<decltype(gizclaw_rpc_v1_ClientToolV0InvokeRequest::payload), pb_callback_t>);
static_assert(std::is_same_v<decltype(gzc_tool_handler_t::tool), gizclaw_rpc_v1_ClientTool>);

static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_FIRMWARE_GET == 22);
static_assert(sizeof(gizclaw_rpc_v1_FirmwareGetResponse::description) == 1025);
static_assert(sizeof(gizclaw_rpc_v1_FirmwareGetResponse::url) == 2049);
static_assert(sizeof(gizclaw_rpc_v1_FirmwareGetResponse::sha256) == 65);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_WORKFLOW_LIST == 32);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_WORKFLOW_GET == 33);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_SPEECH_TRANSCRIBE == 91);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_SPEECH_SYNTHESIZE == 92);
static_assert(GZC_ERR_CHANNEL_LIMIT == -12);

int main() {
  gzc_webrtc_media_vtable_t media{};
  media.struct_size = sizeof(media);
  gizclaw_rpc_v1_WorkflowListRequest workflows = gizclaw_rpc_v1_WorkflowListRequest_init_zero;
  gizclaw_rpc_v1_WorkspaceCreateBody workspace = gizclaw_rpc_v1_WorkspaceCreateBody_init_zero;
  return media.struct_size != sizeof(media) || workflows.has_limit ||
         workspace.has_parameters;
}
