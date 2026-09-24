package gizclaw

import (
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type inputModeWorkflowCatalog struct {
	workflow.WorkflowAdminService
	item apitypes.Workflow
}

func (c inputModeWorkflowCatalog) GetWorkflow(context.Context, adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	return adminhttp.GetWorkflow200JSONResponse(c.item), nil
}

func TestDoubaoAudioInputModeDenial(t *testing.T) {
	realtime := apitypes.WorkspaceInputModeRealtime
	parameters := &apitypes.WorkspaceParameters{}
	if err := parameters.FromDoubaoRealtimeWorkspaceParameters(apitypes.DoubaoRealtimeWorkspaceParameters{Input: &realtime}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		params   *apitypes.WorkspaceParameters
		declared eventpb.AudioInputMode
		wantCode string
	}{
		{name: "default accepts push-to-talk", declared: eventpb.AudioInputMode_AUDIO_INPUT_MODE_PUSH_TO_TALK},
		{name: "default rejects realtime", declared: eventpb.AudioInputMode_AUDIO_INPUT_MODE_REALTIME, wantCode: "WORKSPACE_INPUT_MODE_MISMATCH"},
		{name: "explicit realtime accepts realtime", params: parameters, declared: eventpb.AudioInputMode_AUDIO_INPUT_MODE_REALTIME},
		{name: "explicit realtime accepts push-to-talk", params: parameters, declared: eventpb.AudioInputMode_AUDIO_INPUT_MODE_PUSH_TO_TALK},
		{name: "legacy unspecified keeps behavior", declared: eventpb.AudioInputMode_AUDIO_INPUT_MODE_UNSPECIFIED},
		{name: "unknown enum rejects", declared: eventpb.AudioInputMode(99), wantCode: "INVALID_AUDIO_INPUT_MODE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace := apitypes.Workspace{Name: "chat-room", WorkflowId: "conversation", Parameters: tc.params}
			manager := &Manager{
				Workspaces: sfuTestWorkspaceCatalog{items: []apitypes.Workspace{workspace}},
				Workflows:  inputModeWorkflowCatalog{item: apitypes.Workflow{Id: "conversation", Spec: apitypes.WorkflowSpec{DoubaoRealtime: &apitypes.DoubaoRealtimeWorkflowSpec{}}}},
			}
			peer := &PeerConn{Conn: &testGiznetConn{publicKey: giznet.PublicKey{9}}, Service: &PeerService{manager: manager}}
			event := &eventpb.PeerEvent{Version: eventpb.Version, Type: eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{StreamId: "turn", Kind: eventpb.StreamKind_STREAM_KIND_AUDIO, InputMode: tc.declared}}}
			denial := peer.audioInputModeDenial(t.Context(), event, "chat-room", false)
			if tc.wantCode == "" {
				if denial != nil {
					t.Fatalf("audioInputModeDenial() = %+v, want allowed", denial)
				}
				return
			}
			if denial == nil || denial.Code != tc.wantCode || denial.Retryable {
				t.Fatalf("audioInputModeDenial() = %+v, want non-retryable %q", denial, tc.wantCode)
			}
			if tc.wantCode == "WORKSPACE_INPUT_MODE_MISMATCH" && !strings.Contains(denial.Message, "WORKSPACE_INPUT_MODE_REALTIME") {
				t.Fatalf("mode mismatch message = %q, want REALTIME setup instruction", denial.Message)
			}
		})
	}
}

func TestSFUAudioInputModeBypassesDoubaoCheck(t *testing.T) {
	peer := &PeerConn{}
	event := &eventpb.PeerEvent{Version: eventpb.Version, Type: eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{StreamId: "turn", Kind: eventpb.StreamKind_STREAM_KIND_AUDIO, InputMode: eventpb.AudioInputMode_AUDIO_INPUT_MODE_REALTIME}}}
	if denial := peer.audioInputModeDenial(t.Context(), event, "sfu-room", true); denial != nil {
		t.Fatalf("SFU audioInputModeDenial() = %+v, want allowed", denial)
	}
}
