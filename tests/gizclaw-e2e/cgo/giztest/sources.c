/*
 * Single translation unit for the two C SDKs plus nanopb, so the cgo runner
 * links the real SDK sources instead of a prebuilt library.
 */
#include "../../../../sdk/c/gizclaw/generated/events/peer_event.pb.c"
#include "../../../../sdk/c/gizclaw/generated/google/protobuf/struct.pb.c"
#include "../../../../sdk/c/gizclaw/generated/payload/ai.pb.c"
#include "../../../../sdk/c/gizclaw/generated/payload/edge.pb.c"
#include "../../../../sdk/c/gizclaw/generated/payload/enums.pb.c"
#include "../../../../sdk/c/gizclaw/generated/payload/firmware.pb.c"
#include "../../../../sdk/c/gizclaw/generated/payload/gameplay.pb.c"
#include "../../../../sdk/c/gizclaw/generated/payload/icon.pb.c"
#include "../../../../sdk/c/gizclaw/generated/payload/social.pb.c"
#include "../../../../sdk/c/gizclaw/generated/payload/system.pb.c"
#include "../../../../sdk/c/gizclaw/generated/payload/workspace.pb.c"
#include "../../../../sdk/c/gizclaw/generated/rpc.pb.c"
#include "../../../../sdk/c/gizclaw/src/gzc_buffer.c"
#include "../../../../sdk/c/gizclaw/src/gzc_client.c"
#include "../../../../sdk/c/gizclaw/src/gzc_common.c"
#define ascii_is_space gzc_event_ascii_is_space
#include "../../../../sdk/c/gizclaw/src/gzc_event.c"
#undef ascii_is_space
#define ascii_is_digit gzc_json_ascii_is_digit
#define ascii_is_space gzc_json_ascii_is_space
#include "../../../../sdk/c/gizclaw/src/gzc_json.c"
#undef ascii_is_space
#undef ascii_is_digit
#include "../../../../sdk/c/gizclaw/src/gzc_keys.c"
#include "../../../../sdk/c/gizclaw/src/gzc_platform.c"
#include "../../../../sdk/c/gizclaw/src/gzc_rpc.c"
#include "../../../../sdk/c/gizclaw/src/gzc_rpc_frame.c"
#include "../../../../sdk/c/gizclaw/src/gzc_signaling.c"
#include "../../../../sdk/c/gizclaw/src/gzc_telemetry.c"
#include "../../../../sdk/c/gizclaw_control/src/gzc_control_api.c"
#include "../../../../sdk/c/gizclaw_control/src/gzc_control_error.c"
#include "../../../../sdk/c/gizclaw_control/src/gzc_control_http.c"
#include "../../../../sdk/c/gizclaw_control/src/gzc_control_model.c"
#include "../../../../third_party/nanopb/upstream/pb_common.c"
#include "../../../../third_party/nanopb/upstream/pb_decode.c"
#include "../../../../third_party/nanopb/upstream/pb_encode.c"
