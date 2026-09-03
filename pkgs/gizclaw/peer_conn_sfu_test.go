package gizclaw

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// stubSFUBindings answers every binding lookup with one fixed result.
type stubSFUBindings struct {
	mu  sync.Mutex
	err error
}

func (s *stubSFUBindings) set(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (s *stubSFUBindings) ResolveSFUWorkspaceBinding(_ context.Context, workspaceID, peer string) (socialutil.SFUWorkspaceBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return socialutil.SFUWorkspaceBinding{}, s.err
	}
	return socialutil.SFUWorkspaceBinding{
		WorkspaceID: workspaceID,
		Kind:        socialutil.SFUWorkspaceKindFriend,
		Members:     []string{peer},
		SFU:         socialutil.SFUBinding{URL: "wss://sfu.example", RoomToken: "room-a", Generation: 1},
	}, nil
}

// ResolveSFUWorkspaceBindingByName knows one bound Workspace name; every
// other name resolves as an unbound Workflow Workspace.
func (s *stubSFUBindings) ResolveSFUWorkspaceBindingByName(ctx context.Context, workspaceName, peer string) (socialutil.SFUWorkspaceBinding, error) {
	if workspaceName != testSFUWorkspaceName {
		return socialutil.SFUWorkspaceBinding{}, sfu.ErrNotBound
	}
	binding, err := s.ResolveSFUWorkspaceBinding(ctx, testSFUWorkspaceID, peer)
	if err != nil {
		return socialutil.SFUWorkspaceBinding{}, err
	}
	binding.WorkspaceName = workspaceName
	return binding, nil
}

const (
	testSFUWorkspaceName = "sfu-room"
	testSFUWorkspaceID   = "id-sfu-room"
)

func newSFUTestManager(runs *peerrun.Server, bindings sfu.BindingResolver) *Manager {
	return &Manager{
		Workspaces: staticWorkspaceService{workspace: apitypes.Workspace{
			Id:         testSFUWorkspaceID,
			Name:       testSFUWorkspaceName,
			WorkflowId: socialutil.SFUWorkflowID,
		}},
		PeerRun:            runs,
		sfuBindingResolver: bindings,
	}
}

func audioBOS(streamID string) *eventpb.PeerEvent {
	return &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_BOS,
		Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{
			StreamId: streamID,
			Kind:     eventpb.StreamKind_STREAM_KIND_AUDIO,
		}},
	}
}

func TestPeerConnAdmitsSFUTurnsOnlyWhileRuntimeAttachedAndMember(t *testing.T) {
	ctx := t.Context()
	caller := giznet.PublicKey{41}
	runs := &peerrun.Server{Store: kv.NewMemory(nil)}
	selection := apitypes.AgentSelection{WorkspaceName: testSFUWorkspaceName}
	if _, err := runs.SetRunAgent(ctx, caller, selection); err != nil {
		t.Fatalf("SetRunAgent: %v", err)
	}
	bindings := &stubSFUBindings{}
	broker := newPeerStreamEventBroker()
	var output peerStreamLockedBuffer
	unsubscribe, err := broker.Subscribe(&output)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsubscribe()
	input := &countingPeerAgentInput{pushed: make(chan *genx.MessageChunk, 1)}
	peer := &PeerConn{
		Conn:       &testGiznetConn{publicKey: caller},
		Service:    &PeerService{manager: newSFUTestManager(runs, bindings)},
		agentInput: input,
		events:     broker,
	}

	// Pending selection: the SFU runtime is not attached yet.
	authorized, err := peer.authorizeInputEvent(ctx, audioBOS("turn-pending"))
	if err != nil {
		t.Fatalf("authorize pending BOS: %v", err)
	}
	if authorized {
		t.Fatal("audio BOS was admitted before the SFU runtime attached")
	}
	waitForPeerStreamBytes(t, &output)
	denial := readLockedPeerStreamEvent(t, &output)
	if code := denial.GetEos().GetError().GetCode(); code != sfuRuntimeNotAttachedCode || !denial.GetEos().GetError().GetRetryable() {
		t.Fatalf("pending denial = %+v", denial)
	}
	if _, err := peer.authorizeInputEvent(ctx, audioEndEvent("turn-pending")); err != nil {
		t.Fatalf("authorize pending EOS: %v", err)
	}

	// Active runtime and current membership: input flows.
	if _, err := runs.ActivateRunAgent(ctx, caller, selection); err != nil {
		t.Fatalf("ActivateRunAgent: %v", err)
	}
	authorized, err = peer.authorizeInputEvent(ctx, audioBOS("turn-active"))
	if err != nil {
		t.Fatalf("authorize active BOS: %v", err)
	}
	if !authorized || !peer.audioInputAccepted() {
		t.Fatal("audio BOS was rejected on the attached SFU runtime")
	}
	peer.inputAccessMu.Lock()
	isSFU := peer.acceptedAudioSFU
	peer.inputAccessMu.Unlock()
	if !isSFU {
		t.Fatal("accepted audio stream was not marked as SFU input")
	}
	if authorized, err := peer.authorizeAudioPacket(ctx); err != nil || !authorized {
		t.Fatalf("authorizeAudioPacket() = %v, %v; want admitted", authorized, err)
	}
	if _, err := peer.authorizeInputEvent(ctx, audioEndEvent("turn-active")); err != nil {
		t.Fatalf("authorize active EOS: %v", err)
	}

	// Membership lost: the next turn is denied without caching.
	bindings.set(sfu.ErrNotMember)
	authorized, err = peer.authorizeInputEvent(ctx, audioBOS("turn-removed"))
	if err != nil {
		t.Fatalf("authorize removed BOS: %v", err)
	}
	if authorized {
		t.Fatal("audio BOS was admitted after membership was lost")
	}
	waitForPeerStreamBytes(t, &output)
	denial = readLockedPeerStreamEvent(t, &output)
	if code := denial.GetEos().GetError().GetCode(); code != sfuAccessRevokedCode || denial.GetEos().GetError().GetRetryable() {
		t.Fatalf("removed denial = %+v", denial)
	}
	select {
	case chunk := <-input.pushed:
		t.Fatalf("denied turn reached Agent input: %+v", chunk)
	default:
	}

	// A lookup failure fails closed but stays retryable.
	if _, err := peer.authorizeInputEvent(ctx, audioEndEvent("turn-removed")); err != nil {
		t.Fatalf("authorize removed EOS: %v", err)
	}
	bindings.set(errors.New("social kv unavailable"))
	if authorized, err := peer.authorizeInputEvent(ctx, audioBOS("turn-failed")); err != nil || authorized {
		t.Fatalf("authorize BOS during lookup failure = %v, %v; want denied", authorized, err)
	}
	waitForPeerStreamBytes(t, &output)
	denial = readLockedPeerStreamEvent(t, &output)
	if code := denial.GetEos().GetError().GetCode(); code != sfuAccessCheckFailedCode || !denial.GetEos().GetError().GetRetryable() {
		t.Fatalf("lookup failure denial = %+v", denial)
	}

	// A Workspace without any binding is a plain Workflow Workspace.
	if _, err := peer.authorizeInputEvent(ctx, audioEndEvent("turn-failed")); err != nil {
		t.Fatalf("authorize failed EOS: %v", err)
	}
	bindings.set(sfu.ErrNotBound)
	if authorized, err := peer.authorizeInputEvent(ctx, audioBOS("turn-workflow")); err != nil || !authorized {
		t.Fatalf("authorize Workflow BOS = %v, %v; want admitted", authorized, err)
	}
	peer.inputAccessMu.Lock()
	isSFU = peer.acceptedAudioSFU
	peer.inputAccessMu.Unlock()
	if isSFU {
		t.Fatal("unbound Workspace input was marked as SFU input")
	}
}

func TestManagerAllowSFURestrictedReloadUsesMembership(t *testing.T) {
	ctx := t.Context()
	caller := giznet.PublicKey{48}
	bindings := &stubSFUBindings{}
	manager := newSFUTestManager(nil, bindings)
	if !manager.allowSFURestrictedReload(ctx, caller, testSFUWorkspaceName) {
		t.Fatal("current member was refused the restricted reload")
	}
	bindings.set(sfu.ErrNotMember)
	if manager.allowSFURestrictedReload(ctx, caller, testSFUWorkspaceName) {
		t.Fatal("removed member was admitted to the restricted reload")
	}
	bindings.set(sfu.ErrNotBound)
	if manager.allowSFURestrictedReload(ctx, caller, testSFUWorkspaceName) {
		t.Fatal("unbound Workspace was admitted to the restricted reload")
	}
	bindings.set(nil)
	if manager.allowSFURestrictedReload(ctx, caller, "unknown-room") {
		t.Fatal("unknown Workspace was admitted to the restricted reload")
	}
	if (&Manager{}).allowSFURestrictedReload(ctx, caller, testSFUWorkspaceName) {
		t.Fatal("Manager without Workspaces admitted the restricted reload")
	}
}

func TestManagerSFUBindingsFallsThroughToNotBound(t *testing.T) {
	manager := NewManager(nil)
	_, err := manager.sfuBindings().ResolveSFUWorkspaceBinding(t.Context(), testSFUWorkspaceID, giznet.PublicKey{49}.String())
	if !errors.Is(err, sfu.ErrNotBound) {
		t.Fatalf("ResolveSFUWorkspaceBinding() error = %v, want ErrNotBound", err)
	}
}

func TestOpenAIWorkspaceAdapterRejectsSFUWorkspace(t *testing.T) {
	ctx := t.Context()
	adapter := openAIWorkspaceAdapter{
		caller: giznet.PublicKey{50},
		manager: &Manager{
			AgentHost: agenthost.New(nil),
			Workflows: fixedWorkflowAdminService{value: apitypes.Workflow{
				Id:   "wf-sfu",
				Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverSfu},
			}},
		},
	}
	item := apitypes.Workspace{Id: "ws-sfu", Name: testSFUWorkspaceName, WorkflowId: "wf-sfu"}
	if _, err := adapter.ExecuteWorkspaceText(ctx, item, "hello", nil); !errors.Is(err, errOpenAISFUWorkspace) {
		t.Fatalf("ExecuteWorkspaceText() error = %v, want %v", err, errOpenAISFUWorkspace)
	}
	if err := adapter.rejectSFUWorkspace(ctx, item); !errors.Is(err, errOpenAISFUWorkspace) {
		t.Fatalf("rejectSFUWorkspace() error = %v, want %v", err, errOpenAISFUWorkspace)
	}
	systemItem := apitypes.Workspace{Id: "ws-system", Name: "friend-room", WorkflowId: socialutil.SFUWorkflowID}
	if err := (openAIWorkspaceAdapter{}).rejectSFUWorkspace(ctx, systemItem); !errors.Is(err, errOpenAISFUWorkspace) {
		t.Fatalf("rejectSFUWorkspace(system-sfu) error = %v, want %v", err, errOpenAISFUWorkspace)
	}
	adapter.manager.Workflows = fixedWorkflowAdminService{value: apitypes.Workflow{
		Id:   "wf-flowcraft",
		Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft},
	}}
	if err := adapter.rejectSFUWorkspace(ctx, apitypes.Workspace{Id: "ws-a", Name: "chat", WorkflowId: "wf-flowcraft"}); err != nil {
		t.Fatalf("rejectSFUWorkspace(flowcraft) error = %v", err)
	}
}

// retiredNameWorkspaceService models a Social Workspace whose retirement
// marker already denies every access-checked name lookup while the canonical
// record is still readable by ID.
type retiredNameWorkspaceService struct {
	staticWorkspaceService
}

func (s retiredNameWorkspaceService) GetWorkspaceByName(context.Context, string) (apitypes.Workspace, error) {
	return apitypes.Workspace{}, workspace.ErrWorkspacePendingDeletion
}

func TestManagerHandleWorkspaceActivatedSkipsSFUWorkspaces(t *testing.T) {
	manager := &Manager{Gameplay: &gameplay.Runtime{WorkspaceRewards: &workspaceRewardEnvironment{}}}
	// Both records carry an invalid ID: Gameplay rejects it, so reaching the
	// enqueue is observable, while the SFU Workspace must return before it.
	social := apitypes.Workspace{Name: "social-direct-1", WorkflowId: socialutil.SFUWorkflowID, System: new(true)}
	if err := manager.handleWorkspaceActivated(t.Context(), social); err != nil {
		t.Fatalf("handleWorkspaceActivated(sfu) error = %v, want nil", err)
	}
	if err := manager.handleWorkspaceActivated(t.Context(), apitypes.Workspace{Name: "chat", WorkflowId: "workflow-1"}); err == nil {
		t.Fatal("handleWorkspaceActivated(workflow) did not reach the reward activation")
	}
	if err := (&Manager{}).handleWorkspaceActivated(t.Context(), social); err != nil {
		t.Fatalf("handleWorkspaceActivated without Gameplay error = %v", err)
	}
}

func audioEndEvent(streamID string) *eventpb.PeerEvent {
	return &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
		Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{
			StreamId: streamID,
			Kind:     eventpb.StreamKind_STREAM_KIND_AUDIO,
		}},
	}
}
