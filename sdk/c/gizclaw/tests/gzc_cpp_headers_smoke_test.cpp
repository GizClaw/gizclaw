#include "gzc.h"

static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_FIRMWARE_GET == 22);
static_assert(sizeof(gizclaw_rpc_v1_FirmwareGetResponse::firmware_name) == 257);
static_assert(sizeof(gizclaw_rpc_v1_FirmwareGetResponse::description) == 1025);
static_assert(sizeof(gizclaw_rpc_v1_FirmwareGetResponse::url) == 2049);
static_assert(sizeof(gizclaw_rpc_v1_FirmwareGetResponse::sha256) == 65);
static_assert(sizeof(gizclaw_rpc_v1_ServerRegisterResponse::firmware_name) == 257);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_WORKFLOW_LIST == 32);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_WORKFLOW_GET == 33);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_RUNTIME_ADOPT == 67);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_SPEECH_TRANSCRIBE == 91);
static_assert(gizclaw_rpc_v1_RpcMethod_RPC_METHOD_SERVER_SPEECH_SYNTHESIZE == 92);
static_assert(GZC_API_VERSION == 4);
static_assert(GZC_ERR_CHANNEL_LIMIT == -12);

int main() {
  gzc_webrtc_media_vtable_t media{};
  media.struct_size = sizeof(media);
  gizclaw_rpc_v1_WorkflowListRequest workflows = gizclaw_rpc_v1_WorkflowListRequest_init_zero;
  gizclaw_rpc_v1_WorkspaceCreateBody workspace = gizclaw_rpc_v1_WorkspaceCreateBody_init_zero;
  return media.struct_size != sizeof(media) || workflows.has_limit ||
         workspace.has_parameters;
}
