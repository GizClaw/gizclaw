package gizclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/openaiapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/chatroom"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"google.golang.org/protobuf/proto"
)

func TestPeerConnLifecycleRecordsInputAndClosedTerminal(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-1", "peer-1")
	lifecycle.eventStreamAccepted()
	input := &countingPeerAgentInput{pushed: make(chan *genx.MessageChunk, 1)}
	peer := &PeerConn{agentInput: input, streamLifecycle: lifecycle}
	serverSide, clientSide := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- peer.readEventStream(serverSide) }()

	const untrustedStreamID = "turn-secret-Bearer-credential"
	if err := writePeerStreamEvent(clientSide, &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_BOS,
		Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{
			StreamId: untrustedStreamID,
			Kind:     eventpb.StreamKind_STREAM_KIND_TEXT,
		}},
	}); err != nil {
		t.Fatalf("writePeerStreamEvent() error = %v", err)
	}
	select {
	case <-input.pushed:
	case <-time.After(time.Second):
		t.Fatal("Peer input did not reach the Agent input pusher")
	}
	if err := clientSide.Close(); err != nil {
		t.Fatalf("clientSide.Close() error = %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("readEventStream() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("readEventStream did not stop after peer close")
	}

	records := capturedLifecycleRecords(t, capture)
	if len(records) != 3 {
		t.Fatalf("lifecycle records = %d, want accept, first input, and terminal", len(records))
	}
	inputAttrs := lifecycleRecordAttrs(records[1])
	if inputAttrs["stage"] != "input_first_event" || inputAttrs["stream_id_hash"] != safeStreamIDHash(untrustedStreamID) {
		t.Fatalf("input lifecycle record = %#v", inputAttrs)
	}
	if strings.Contains(fmt.Sprint(inputAttrs), untrustedStreamID) {
		t.Fatalf("input lifecycle record exposed raw stream ID: %#v", inputAttrs)
	}
	terminal := lifecycleRecordAttrs(records[2])
	if terminal["component"] != "peer_input" || terminal["result"] != "closed" || terminal["last_stage"] != "input_first_event" || terminal["input_event_observed"] != true {
		t.Fatalf("Peer input terminal = %#v", terminal)
	}
}

func TestPeerConnRetireDetachesOnlyItsActiveConnection(t *testing.T) {
	key := giznet.PublicKey{44}
	manager := &Manager{}
	conn := &testGiznetConn{publicKey: key}
	manager.SetPeerUp(key, conn)
	registration := runtimeprofile.Registration{RuntimeProfile: apitypes.RuntimeProfile{Id: "profile-a"}}
	if !manager.SetPeerRegistration(key, conn, registration) {
		t.Fatal("SetPeerRegistration rejected active connection")
	}
	peerConn := &PeerConn{Conn: conn, Service: &PeerService{manager: manager}}
	peerConn.registration.Store(&registration)
	peerConn.retire()
	if !peerConn.isRetiring() {
		t.Fatal("PeerConn was not marked retiring")
	}
	if peerConn.registration.Load() != nil {
		t.Fatal("PeerConn retained its registration")
	}
	if _, ok := manager.Peer(key); ok {
		t.Fatal("Manager retained retiring connection")
	}
	if _, _, err := peerConn.CreateAudioTrack(); !errors.Is(err, ErrPeerConnRetiring) {
		t.Fatalf("CreateAudioTrack error = %v, want ErrPeerConnRetiring", err)
	}

	replacement := &testGiznetConn{publicKey: key}
	manager.SetPeerUp(key, replacement)
	peerConn.retire()
	if got, ok := manager.Peer(key); !ok || got != replacement {
		t.Fatalf("repeated retire removed replacement connection: %v, %v", got, ok)
	}
}

func TestPeerConnBroadcastAgentOutputErrorUsesValidLogicalStream(t *testing.T) {
	output := &peerStreamLockedBuffer{}
	broker := newPeerStreamEventBroker()
	unsubscribe, err := broker.Subscribe(output)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	(&PeerConn{events: broker}).broadcastAgentOutputError(
		t.Context(),
		"workspace-a",
		errors.New("output failed"),
	)
	event := readLockedPeerStreamEvent(t, output)
	if got := event.GetEos().GetStreamId(); got != "agent-output-error" {
		t.Fatalf("agent output error stream id = %q", got)
	}
	if got := event.GetEos().GetError().GetCode(); got != "AGENT_OUTPUT_ERROR" {
		t.Fatalf("agent output error code = %q", got)
	}
}

func TestPeerConnInitAgentHostWiresRouteErrorContext(t *testing.T) {
	ctx := t.Context()
	publicKey := giznet.PublicKey{42}
	runs := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := runs.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "workspace-a"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	peerConn := &PeerConn{
		Conn: &testGiznetConn{publicKey: publicKey},
		Service: &PeerService{manager: &Manager{
			AgentHost: agenthost.New(nil),
			PeerRun:   runs,
		}},
		events: newPeerStreamEventBroker(),
	}
	peerConn.initAgentHost()
	if peerConn.agentHost == nil {
		t.Fatal("initAgentHost() did not initialize AgentHost")
	}
	output, ok := peerConn.agentHost.Consumer.(peerAgentOutput)
	if !ok {
		t.Fatalf("AgentHost consumer type = %T", peerConn.agentHost.Consumer)
	}
	if output.Logger == nil || output.PeerPublicKey != publicKey.String() || output.WorkspaceName == nil {
		t.Fatalf("route error context = %#v", output)
	}
	if got := output.WorkspaceName(ctx); got != "workspace-a" {
		t.Fatalf("route error Workspace = %q, want workspace-a", got)
	}
}

func TestPeerConnRejectsRevokedChatroomTurnWithoutPushingAgentInput(t *testing.T) {
	ctx := t.Context()
	caller := giznet.PublicKey{31}
	other := giznet.PublicKey{32}
	friends := newTestFriendServer(kv.NewMemory(nil))
	relation, err := friends.AdminCreateFriend(ctx, caller.String(), other.String())
	if err != nil {
		t.Fatalf("AdminCreateFriend: %v", err)
	}
	relationWorkspaceName := socialutil.StringValue(relation.WorkspaceName)
	friends.Workspaces = &adminGameplayWorkspaceService{}
	if _, err := friends.DeleteFriend(ctx, caller.String(), rpcapi.FriendDeleteRequest{Name: other.String()}); err != nil {
		t.Fatalf("DeleteFriend: %v", err)
	}
	runs := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := runs.SetRunAgent(ctx, caller, apitypes.AgentSelection{WorkspaceName: relationWorkspaceName}); err != nil {
		t.Fatalf("SetRunAgent: %v", err)
	}
	manager := &Manager{
		Workspaces: staticWorkspaceService{workspace: apitypes.Workspace{
			Id:         "id-" + relationWorkspaceName,
			Name:       relationWorkspaceName,
			Parameters: socialutil.ChatRoomWorkspaceParameters(apitypes.ChatRoomModeDirect),
		}},
		Friends: friends,
		PeerRun: runs,
	}
	input := &countingPeerAgentInput{pushed: make(chan *genx.MessageChunk, 1)}
	peer := &PeerConn{
		Conn:       &testGiznetConn{publicKey: caller},
		Service:    &PeerService{manager: manager},
		agentInput: input,
		events:     newPeerStreamEventBroker(),
	}
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	done := make(chan error, 1)
	go func() {
		done <- peer.handleEventStream(serverSide)
	}()
	if err := clientSide.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}

	firstBOS := &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_BOS,
		Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{
			StreamId: "turn-1",
			Kind:     eventpb.StreamKind_STREAM_KIND_TEXT,
		}},
	}
	if err := writePeerStreamEvent(clientSide, firstBOS); err != nil {
		t.Fatalf("write revoked BOS: %v", err)
	}
	denial, err := readPeerStreamEvent(clientSide)
	if err != nil {
		t.Fatalf("read revoked EOS: %v", err)
	}
	if denial.GetType() != eventpb.PeerEventType_PEER_EVENT_TYPE_EOS ||
		denial.GetEos().GetStreamId() != "turn-1" ||
		denial.GetEos().GetError().GetCode() != "CHATROOM_FRIEND_REMOVED" {
		t.Fatalf("denial = %+v", denial)
	}
	select {
	case chunk := <-input.pushed:
		t.Fatalf("revoked turn reached Agent input: %+v", chunk)
	default:
	}

	if err := writePeerStreamEvent(clientSide, &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
		Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{
			StreamId: "turn-1",
			Kind:     eventpb.StreamKind_STREAM_KIND_TEXT,
		}},
	}); err != nil {
		t.Fatalf("write revoked input EOS: %v", err)
	}
	restored, err := friends.AdminCreateFriend(ctx, caller.String(), other.String())
	if err != nil {
		t.Fatalf("restore friend relationship: %v", err)
	}
	restoredWorkspaceName := socialutil.StringValue(restored.WorkspaceName)
	if restoredWorkspaceName == relationWorkspaceName {
		t.Fatalf("restored Workspace = %q, want a new incarnation", restoredWorkspaceName)
	}
	manager.Workspaces = staticWorkspaceService{workspace: apitypes.Workspace{
		Id:         "id-" + restoredWorkspaceName,
		Name:       restoredWorkspaceName,
		Parameters: socialutil.ChatRoomWorkspaceParameters(apitypes.ChatRoomModeDirect),
	}}
	if _, err := runs.SetRunAgent(
		ctx,
		caller,
		apitypes.AgentSelection{WorkspaceName: restoredWorkspaceName},
	); err != nil {
		t.Fatalf("SetRunAgent restored Workspace: %v", err)
	}
	secondBOS := proto.Clone(firstBOS).(*eventpb.PeerEvent)
	secondBOS.GetBos().StreamId = "turn-2"
	if err := writePeerStreamEvent(clientSide, secondBOS); err != nil {
		t.Fatalf("write restored BOS: %v", err)
	}
	select {
	case chunk := <-input.pushed:
		if chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.StreamID != "turn-2" || !chunk.Ctrl.BeginOfStream {
			t.Fatalf("restored turn input = %+v", chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("restored relationship did not admit the next turn")
	}
	if err := clientSide.Close(); err != nil {
		t.Fatalf("close client stream: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("handleEventStream: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("handleEventStream did not stop")
	}
}

func TestPeerConnFailsClosedWhenActiveWorkspaceCannotBeRead(t *testing.T) {
	caller := giznet.PublicKey{33}
	storeErr := errors.New("forced Peer run read failure")
	broker := newPeerStreamEventBroker()
	var output bytes.Buffer
	unsubscribe, err := broker.Subscribe(&output)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsubscribe()
	peer := &PeerConn{
		Conn: &testGiznetConn{publicKey: caller},
		Service: &PeerService{manager: &Manager{
			PeerRun: &peerrun.Server{
				Store: &failingGetStore{Store: kv.NewMemory(nil), err: storeErr},
			},
		}},
		events: broker,
	}
	input := &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_BOS,
		Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{
			StreamId: "turn-read-failure",
			Kind:     eventpb.StreamKind_STREAM_KIND_TEXT,
		}},
	}

	authorized, err := peer.authorizeChatroomEvent(t.Context(), input)
	if err != nil {
		t.Fatalf("authorizeChatroomEvent: %v", err)
	}
	if authorized {
		t.Fatal("Peer run read failure admitted the input turn")
	}
	denial, err := readPeerStreamEvent(&output)
	if err != nil {
		t.Fatalf("read denial: %v", err)
	}
	if denial.GetEos().GetError().GetCode() != "CHATROOM_ACCESS_CHECK_FAILED" ||
		!denial.GetEos().GetError().GetRetryable() {
		t.Fatalf("denial = %+v, want retryable access-check failure", denial)
	}
}

func TestPeerConnBoundsDeniedInputStreamTracking(t *testing.T) {
	peer := &PeerConn{}
	peer.markDeniedInputStream("audio-turn", eventpb.StreamKind_STREAM_KIND_AUDIO)
	for index := range maxDeniedInputStreams + 20 {
		peer.markDeniedInputStream(string(rune(index+1)), eventpb.StreamKind_STREAM_KIND_TEXT)
	}
	if got := len(peer.deniedInputStreams); got > maxDeniedInputStreams {
		t.Fatalf("denied input stream count = %d, want at most %d", got, maxDeniedInputStreams)
	}
	if !peer.inputStreamDenied("audio") {
		t.Fatal("bounded text tracking cleared the denied audio gate")
	}
	peer.clearDeniedInputStream("audio-turn", eventpb.StreamKind_STREAM_KIND_UNSPECIFIED)
	if peer.inputStreamDenied("audio") {
		t.Fatal("matching EOS with omitted kind did not clear the denied audio gate")
	}
}

func TestPeerConnReauthorizesAudioPacketsAfterChatroomAccessIsRevoked(t *testing.T) {
	ctx := t.Context()
	caller := giznet.PublicKey{34}
	other := giznet.PublicKey{35}
	friends := newTestFriendServer(kv.NewMemory(nil))
	relation, err := friends.AdminCreateFriend(ctx, caller.String(), other.String())
	if err != nil {
		t.Fatalf("AdminCreateFriend: %v", err)
	}
	relationWorkspaceName := socialutil.StringValue(relation.WorkspaceName)
	runs := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := runs.SetRunAgent(ctx, caller, apitypes.AgentSelection{WorkspaceName: relationWorkspaceName}); err != nil {
		t.Fatalf("SetRunAgent: %v", err)
	}
	broker := newPeerStreamEventBroker()
	var output peerStreamLockedBuffer
	unsubscribe, err := broker.Subscribe(&output)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsubscribe()
	input := &countingPeerAgentInput{pushed: make(chan *genx.MessageChunk, 1)}
	peer := &PeerConn{
		Conn: &testGiznetConn{publicKey: caller},
		Service: &PeerService{manager: &Manager{
			Workspaces: staticWorkspaceService{workspace: apitypes.Workspace{
				Id:         "id-" + relationWorkspaceName,
				Name:       relationWorkspaceName,
				Parameters: socialutil.ChatRoomWorkspaceParameters(apitypes.ChatRoomModeDirect),
			}},
			Friends: friends,
			PeerRun: runs,
		}},
		agentInput: input,
		events:     broker,
	}
	bos := &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_BOS,
		Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{
			StreamId: "turn-mid-revoke",
			Kind:     eventpb.StreamKind_STREAM_KIND_AUDIO,
		}},
	}
	authorized, err := peer.authorizeChatroomEvent(ctx, bos)
	if err != nil {
		t.Fatalf("authorize BOS: %v", err)
	}
	if !authorized {
		t.Fatal("authorized BOS was rejected")
	}
	friends.Workspaces = &adminGameplayWorkspaceService{}
	if _, err := friends.DeleteFriend(ctx, caller.String(), rpcapi.FriendDeleteRequest{Name: other.String()}); err != nil {
		t.Fatalf("DeleteFriend: %v", err)
	}
	peer.observePeerEvent(&eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED,
		Payload: &eventpb.PeerEvent_FriendRelationshipUpdated{
			FriendRelationshipUpdated: &eventpb.FriendRelationshipUpdated{
				PeerPublicKey: other.String(),
				WorkspaceName: relationWorkspaceName,
				Change: eventpb.
					FriendRelationshipChange_FRIEND_RELATIONSHIP_CHANGE_DELETED,
			},
		},
	})
	authorized, err = peer.authorizeChatroomAudioPacket(ctx)
	if err != nil {
		t.Fatalf("authorize Opus packet: %v", err)
	}
	if authorized {
		t.Fatal("Opus packet after revocation was admitted")
	}
	// Revocation aborts the in-flight turn by pushing a control-only interrupt
	// BOS into the live input source, not by closing the source (which would
	// end the whole Agent pipeline and surface a spurious output-end failure).
	if got := input.closeCount(); got != 0 {
		t.Fatalf("Agent input close calls = %d, want 0", got)
	}
	assertAgentInputInterrupt(t, input)
	waitForPeerStreamBytes(t, &output)
	denial := readLockedPeerStreamEvent(t, &output)
	if got := denial.GetEos().GetError().GetCode(); got != chatroom.AccessCodeFriendRemoved {
		t.Fatalf("denial code = %q, want %q", got, chatroom.AccessCodeFriendRemoved)
	}
	restored, err := friends.AdminCreateFriend(ctx, caller.String(), other.String())
	if err != nil {
		t.Fatalf("restore friend relationship: %v", err)
	}
	restoredWorkspaceName := socialutil.StringValue(restored.WorkspaceName)
	if restoredWorkspaceName == relationWorkspaceName {
		t.Fatalf("restored Workspace = %q, want a new incarnation", restoredWorkspaceName)
	}
	peer.Service.manager.Workspaces = staticWorkspaceService{workspace: apitypes.Workspace{
		Id:         "id-" + restoredWorkspaceName,
		Name:       restoredWorkspaceName,
		Parameters: socialutil.ChatRoomWorkspaceParameters(apitypes.ChatRoomModeDirect),
	}}
	if _, err := runs.SetRunAgent(
		ctx,
		caller,
		apitypes.AgentSelection{WorkspaceName: restoredWorkspaceName},
	); err != nil {
		t.Fatalf("SetRunAgent restored Workspace: %v", err)
	}
	nextBOS := proto.Clone(bos).(*eventpb.PeerEvent)
	nextBOS.GetBos().StreamId = "turn-after-revoke"
	authorized, err = peer.authorizeChatroomEvent(ctx, nextBOS)
	if err != nil {
		t.Fatalf("authorize restored BOS: %v", err)
	}
	if !authorized || !peer.audioInputAccepted() {
		t.Fatal("terminal denial left the next restored audio turn blocked")
	}
}

func TestPeerConnRejectsAudioPacketsAfterWorkspaceSwitch(t *testing.T) {
	ctx := t.Context()
	caller := giznet.PublicKey{38}
	runs := &peerrun.Server{Store: kv.NewMemory(nil)}
	workspaceA := apitypes.AgentSelection{WorkspaceName: "workspace-a"}
	if _, err := runs.SetRunAgent(ctx, caller, workspaceA); err != nil {
		t.Fatalf("SetRunAgent(workspace-a): %v", err)
	}
	if _, err := runs.ActivateRunAgent(ctx, caller, workspaceA); err != nil {
		t.Fatalf("ActivateRunAgent(workspace-a): %v", err)
	}
	peer := &PeerConn{
		Conn: &testGiznetConn{publicKey: caller},
		Service: &PeerService{manager: &Manager{
			PeerRun: runs,
		}},
	}
	peer.acceptInputEvent(&eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_BOS,
		Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{
			StreamId: "audio-workspace-a",
			Kind:     eventpb.StreamKind_STREAM_KIND_AUDIO,
		}},
	}, "audio-workspace-a", workspaceA.WorkspaceName, false)

	authorized, err := peer.authorizeChatroomAudioPacket(ctx)
	if err != nil {
		t.Fatalf("authorize packet in workspace-a: %v", err)
	}
	if !authorized {
		t.Fatal("audio packet was rejected before the Workspace switch")
	}

	workspaceB := apitypes.AgentSelection{WorkspaceName: "workspace-b"}
	if _, err := runs.SetRunAgent(ctx, caller, workspaceB); err != nil {
		t.Fatalf("SetRunAgent(workspace-b): %v", err)
	}
	if _, err := runs.ActivateRunAgent(ctx, caller, workspaceB); err != nil {
		t.Fatalf("ActivateRunAgent(workspace-b): %v", err)
	}
	authorized, err = peer.authorizeChatroomAudioPacket(ctx)
	if err != nil {
		t.Fatalf("authorize packet after Workspace switch: %v", err)
	}
	if authorized {
		t.Fatal("audio packet authorized for workspace-a reached workspace-b")
	}
	if peer.audioInputAccepted() {
		t.Fatal("Workspace switch retained the previous audio authorization gate")
	}
}

func TestPeerConnKeepsGroupAudioWhenAnotherMemberIsRemoved(t *testing.T) {
	caller := giznet.PublicKey{36}
	other := giznet.PublicKey{37}
	input := &countingPeerAgentInput{pushed: make(chan *genx.MessageChunk, 1)}
	peer := &PeerConn{
		Conn:       &testGiznetConn{publicKey: caller},
		agentInput: input,
	}
	peer.acceptInputEvent(&eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_BOS,
		Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{
			StreamId: "group-turn",
			Kind:     eventpb.StreamKind_STREAM_KIND_AUDIO,
		}},
	}, "group-turn", "group-room", true)
	peer.observePeerEvent(&eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED,
		Payload: &eventpb.PeerEvent_FriendGroupUpdated{
			FriendGroupUpdated: &eventpb.FriendGroupUpdated{
				FriendGroupName:       "group-a",
				WorkspaceName:         "group-room",
				Change:                eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_MEMBER_REMOVED,
				AffectedPeerPublicKey: other.String(),
			},
		},
	})
	if !peer.audioInputAccepted() {
		t.Fatal("another member's removal revoked the caller's audio turn")
	}
	if got := input.closeCount(); got != 0 {
		t.Fatalf("Agent input close calls = %d, want 0", got)
	}
}

func TestPeerConnDropsOpusPacketsForDeniedChatroomTurn(t *testing.T) {
	input := &countingPeerAgentInput{pushed: make(chan *genx.MessageChunk, 1)}
	conn := &peerConnPacketConn{
		packets: []peerConnTestPacket{{
			protocol: giznet.ProtocolOpusPacket,
			payload:  []byte{1, 2, 3},
		}},
	}
	peer := &PeerConn{
		Conn:       conn,
		agentInput: input,
	}
	peer.markDeniedInputStream("turn-1", eventpb.StreamKind_STREAM_KIND_AUDIO)

	if err := peer.serveDirectPackets(); err != nil {
		t.Fatalf("serveDirectPackets() error = %v", err)
	}
	select {
	case chunk := <-input.pushed:
		t.Fatalf("denied Opus packet reached Agent input: %+v", chunk)
	default:
	}
	if got := conn.reads; got != 2 {
		t.Fatalf("direct packet reads = %d, want packet plus closed read", got)
	}
}

func TestPeerConnAcceptsOpusPacketsOnlyAfterAuthorizedAudioBOS(t *testing.T) {
	input := &countingPeerAgentInput{pushed: make(chan *genx.MessageChunk, 1)}
	peer := &PeerConn{
		Conn: &peerConnPacketConn{
			packets: []peerConnTestPacket{{
				protocol: giznet.ProtocolOpusPacket,
				payload:  []byte{1, 2, 3},
			}},
		},
		agentInput: input,
	}
	if err := peer.serveDirectPackets(); err != nil {
		t.Fatalf("serveDirectPackets(before BOS) error = %v", err)
	}
	select {
	case chunk := <-input.pushed:
		t.Fatalf("Opus packet before BOS reached Agent input: %+v", chunk)
	default:
	}
	peer.Conn = &peerConnPacketConn{
		packets: []peerConnTestPacket{{
			protocol: giznet.ProtocolOpusPacket,
			payload:  []byte{1, 2, 3},
		}},
	}
	peer.acceptInputEvent(&eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_BOS,
		Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{
			StreamId: "turn-1",
			Kind:     eventpb.StreamKind_STREAM_KIND_AUDIO,
		}},
	}, "turn-1", "", false)

	if err := peer.serveDirectPackets(); err != nil {
		t.Fatalf("serveDirectPackets() error = %v", err)
	}
	select {
	case chunk := <-input.pushed:
		if chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.StreamID != "audio" {
			t.Fatalf("accepted Opus packet chunk = %+v", chunk)
		}
	default:
		t.Fatal("authorized Opus packet did not reach Agent input")
	}
}

func TestRejectRetiringHTTP(t *testing.T) {
	called := false
	handler := rejectRetiringHTTP(func() bool { return true }, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusConflict)
	}
	if !strings.Contains(response.Body.String(), peer.PeerPendingDeletionCode) {
		t.Fatalf("body = %q, want stable pending-deletion code", response.Body.String())
	}
	if called {
		t.Fatal("retiring request reached the underlying handler")
	}
}

func TestPeerConnHelpersAndRPCHandle(t *testing.T) {
	t.Run("audio mixer lifecycle", func(t *testing.T) {
		var nilPeer *PeerConn
		if _, err := nilPeer.audioMixer(); err != ErrNilPeerConn {
			t.Fatalf("audioMixer(nil) err = %v, want %v", err, ErrNilPeerConn)
		}

		peer := &PeerConn{}
		if _, err := peer.audioMixer(); err != ErrNilPeerConnMixer {
			t.Fatalf("audioMixer() err = %v, want %v", err, ErrNilPeerConnMixer)
		}

		peer.init()
		if _, err := peer.audioMixer(); err != nil {
			t.Fatalf("audioMixer() after init error = %v", err)
		}

		track, ctrl, err := peer.CreateAudioTrack()
		if err != nil {
			t.Fatalf("CreateAudioTrack() error = %v", err)
		}
		if track == nil || ctrl == nil {
			t.Fatalf("CreateAudioTrack() = (%v, %v)", track, ctrl)
		}
		if err := peer.close(); err != nil {
			t.Fatalf("close() error = %v", err)
		}
		if !peer.isClosed() {
			t.Fatal("peer should be closed")
		}
	})

	t.Run("dispatch missing params", func(t *testing.T) {
		server := &rpcServer{}
		resp, err := server.dispatch(context.Background(), &rpcapi.RPCRequest{
			Id:     "missing",
			Method: rpcapi.RPCMethodAllPing,
		})
		if err != nil {
			t.Fatalf("dispatch() error = %v", err)
		}
		if resp == nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
			t.Fatalf("dispatch() response = %+v", resp)
		}
	})

	t.Run("dispatch ping and unknown method", func(t *testing.T) {
		server := &rpcServer{}
		params, err := newRPCPingRequestParams(rpcapi.PingRequest{})
		if err != nil {
			t.Fatalf("newRPCPingRequestParams() error = %v", err)
		}
		resp, err := server.dispatch(context.Background(), &rpcapi.RPCRequest{
			Id:     "ping",
			Method: rpcapi.RPCMethodAllPing,
			Params: params,
		})
		if err != nil {
			t.Fatalf("dispatch(ping) error = %v", err)
		}
		if resp == nil || resp.Result == nil {
			t.Fatalf("dispatch(ping) response = %+v", resp)
		}
		result, err := resp.Result.AsPingResponse()
		if err != nil {
			t.Fatalf("dispatch(ping) result decode error = %v", err)
		}
		if result.ServerTime <= 0 {
			t.Fatalf("dispatch(ping) response = %+v", result)
		}

		resp, err = server.dispatch(context.Background(), &rpcapi.RPCRequest{
			Id:     "unknown",
			Method: "rpc.unknown",
		})
		if err != nil {
			t.Fatalf("dispatch(unknown) error = %v", err)
		}
		if resp == nil || resp.Error == nil || !strings.Contains(resp.Error.Message, "unknown method") {
			t.Fatalf("dispatch(unknown) response = %+v", resp)
		}
	})

	t.Run("openai handler routes under v1", func(t *testing.T) {
		keyPair, err := giznet.GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair() error = %v", err)
		}
		var voiceRequests []adminhttp.ListVoicesRequestObject
		handler := newOpenAIHTTPHandler(&openaiapi.Server{
			Caller: keyPair.Public,
			Models: peerConnModelListerFunc(func(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
				return adminhttp.ListModels200JSONResponse(adminhttp.ModelList{Items: []apitypes.Model{
					{Id: "chat", Provider: apitypes.ModelProvider{Id: "main"}},
				}}), nil
			}),
			Voices: peerConnVoiceListerFunc(func(_ context.Context, req adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error) {
				voiceRequests = append(voiceRequests, req)
				return adminhttp.ListVoices200JSONResponse(adminhttp.VoiceList{Items: []apitypes.Voice{
					{
						Id: "voice-a",
						Provider: apitypes.VoiceProvider{
							Kind: apitypes.VoiceProviderKindOpenaiTenant,
							Id:   "main",
						},
						Source: apitypes.VoiceSourceManual,
					},
				}}), nil
			}),
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("GET /v1/models status = %d body=%s", resp.Code, resp.Body.String())
		}
		var models struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &models); err != nil {
			t.Fatalf("decode /v1/models response: %v", err)
		}
		if len(models.Data) != 1 || models.Data[0].ID != "chat" {
			t.Fatalf("/v1/models response = %#v", models)
		}

		req = httptest.NewRequest(http.MethodGet, "/v1/voices?cursor=voice-before&limit=10", nil)
		resp = httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("GET /v1/voices status = %d body=%s", resp.Code, resp.Body.String())
		}
		var voices struct {
			Object string           `json:"object"`
			Data   []apitypes.Voice `json:"data"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &voices); err != nil {
			t.Fatalf("decode /v1/voices response: %v", err)
		}
		if voices.Object != "list" || len(voices.Data) != 1 || voices.Data[0].Id != "voice-a" {
			t.Fatalf("/v1/voices response = %#v", voices)
		}
		if len(voiceRequests) != 1 {
			t.Fatalf("voice requests = %d, want 1", len(voiceRequests))
		}
		params := voiceRequests[0].Params
		if params.Cursor == nil || *params.Cursor != "voice-before" {
			t.Fatalf("voice cursor param = %#v", params.Cursor)
		}
		if params.Limit == nil || *params.Limit != 10 {
			t.Fatalf("voice limit param = %#v", params.Limit)
		}
		if params.Source != nil || params.ProviderKind != nil || params.ProviderId != nil {
			t.Fatalf("unexpected voice filters = %#v", params)
		}

		req = httptest.NewRequest(http.MethodGet, "/models", nil)
		resp = httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("GET /models status = %d, want 404", resp.Code)
		}
	})
}

func TestOpenAIHandlerEnforcesShellBoundary(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	var modelCalls atomic.Int32
	handler := newOpenAIHTTPHandler(&openaiapi.Server{
		Caller: keyPair.Public,
		Models: peerConnModelListerFunc(func(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
			modelCalls.Add(1)
			return adminhttp.ListModels200JSONResponse(adminhttp.ModelList{}), nil
		}),
	})
	for _, request := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/conversations"},
		{http.MethodPost, "/v1/embeddings"},
		{http.MethodGet, "/v1/realtime"},
		{http.MethodGet, "/v1/models/model-a"},
		{http.MethodPost, "/v1/audio/translations"},
		{http.MethodGet, "/v1/chat/completions/completion-a"},
		{http.MethodPost, "/v1/models"},
		{http.MethodGet, "/v1/conversations/conv-a/"},
		{http.MethodPost, "/v1/conversations/conv-a/items"},
		{http.MethodDelete, "/v1/conversations/conv-a"},
		{http.MethodGet, "/v1/responses/resp-a?include=output_text"},
		{http.MethodPost, "/v1/responses?beta=true"},
		{http.MethodGet, "/v1/conversations/conv-a/items?before=msg-a"},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(request.method, request.path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s %s status = %d body=%s", request.method, request.path, recorder.Code, recorder.Body.String())
		}
	}
	if modelCalls.Load() != 0 {
		t.Fatalf("unsupported routes dispatched %d model calls", modelCalls.Load())
	}

	protocol, err := newOpenAIProtocolHandler()
	if err != nil {
		t.Fatalf("newOpenAIProtocolHandler() error = %v", err)
	}
	recorder := httptest.NewRecorder()
	protocol.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing binding status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestOpenAIRouteMatcherAcceptsOnlySupportedDynamicPaths(t *testing.T) {
	for _, test := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/conversations"},
		{http.MethodGet, "/v1/conversations/conv-a"},
		{http.MethodGet, "/v1/conversations/conv-a/items"},
		{http.MethodGet, "/v1/conversations/conv-a/items/msg-a"},
		{http.MethodPost, "/v1/responses"},
		{http.MethodGet, "/v1/responses/resp-a"},
		{http.MethodGet, "/v1/responses/resp-a/input_items"},
		{http.MethodPost, "/v1/responses/resp-a/cancel"},
	} {
		if _, ok := supportedOpenAIRoute(test.method, test.path); !ok {
			t.Errorf("%s %s was rejected", test.method, test.path)
		}
	}
	for _, path := range []string{"/v1/conversations", "/v1/conversations/conv-a/", "/v1/conversations/a/b", "/v1/responses/resp-a/events"} {
		if _, ok := supportedOpenAIRoute(http.MethodGet, path); ok {
			t.Errorf("GET %s was accepted", path)
		}
	}
}

func TestOpenAIHandlerPreservesThinkingAndBodyLimit(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	var generatorCalls atomic.Int32
	handler := newOpenAIHTTPHandler(&openaiapi.Server{
		Caller: keyPair.Public,
		Generator: peerConnOpenAIGeneratorFunc(func(_ context.Context, _ string, modelContext genx.ModelContext) (genx.Stream, error) {
			generatorCalls.Add(1)
			params := modelContext.Params()
			if params == nil || params.Thinking == nil || params.Thinking.Enabled == nil || !*params.Thinking.Enabled || params.Thinking.Level != "high" {
				t.Fatalf("thinking params = %#v", params)
			}
			return &peerConnOpenAITextStream{text: "ok"}, nil
		}),
	})
	body := `{"model":"chat","messages":[{"role":"user","content":"hello"}],"thinking":{"enabled":true,"level":"high"}}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("thinking status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	oversized := `{"model":"chat","messages":[],"padding":"` + strings.Repeat("x", int(openAIMaxBodyBytes)) + `"}`
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(oversized))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if generatorCalls.Load() != 1 {
		t.Fatalf("generator calls = %d, want 1", generatorCalls.Load())
	}
}

type peerConnOpenAIGeneratorFunc func(context.Context, string, genx.ModelContext) (genx.Stream, error)

func (f peerConnOpenAIGeneratorFunc) GenerateStream(ctx context.Context, pattern string, modelContext genx.ModelContext) (genx.Stream, error) {
	return f(ctx, pattern, modelContext)
}

func (peerConnOpenAIGeneratorFunc) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not implemented")
}

type peerConnOpenAITextStream struct {
	text string
	done bool
}

func (s *peerConnOpenAITextStream) Next() (*genx.MessageChunk, error) {
	if s.done {
		return nil, genx.ErrDone
	}
	s.done = true
	return &genx.MessageChunk{Part: genx.Text(s.text)}, nil
}

func (s *peerConnOpenAITextStream) Close() error {
	s.done = true
	return nil
}

func (s *peerConnOpenAITextStream) CloseWithError(error) error { return s.Close() }

func TestPeerConnPacesMixedAudioAtEgress(t *testing.T) {
	mx := pcm.NewMixer(peerConnMixerFormat)
	track, ctrl, err := mx.CreateTrack()
	if err != nil {
		t.Fatalf("CreateTrack() error = %v", err)
	}
	frame := make([]byte, peerConnMixerFormat.BytesInDuration(peerConnOpusFrameDuration))
	for i := range frame {
		frame[i] = byte(i)
	}
	if err := track.Write(peerConnMixerFormat.DataChunk(append(append([]byte(nil), frame...), frame...))); err != nil {
		t.Fatalf("track.Write() error = %v", err)
	}
	if err := track.Write(peerConnMixerFormat.DataChunk(frame)); err != nil {
		t.Fatalf("track.Write() error = %v", err)
	}
	if err := ctrl.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() error = %v", err)
	}

	conn := &recordingGiznetConn{written: make(chan struct{}, 3)}
	ticks := make(chan time.Time)
	peer := &PeerConn{Conn: conn, mixer: mx, audioPacing: ticks}
	result := make(chan error, 1)
	go func() {
		_, err := peer.streamMixedAudio(false)
		result <- err
	}()

	for want := 1; want <= 3; want++ {
		ticks <- time.Now()
		select {
		case <-conn.written:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for packet %d", want)
		}
		if got := conn.packetCount(); got != want {
			t.Fatalf("packet count after tick %d = %d, want %d", want, got, want)
		}
		select {
		case <-conn.written:
			t.Fatalf("wrote packet without the next pacing tick")
		case <-time.After(10 * time.Millisecond):
		}
	}
	for index, packet := range conn.recordedPackets() {
		if ticks := codecconv.OpusPacketRTPTicks(packet); ticks != 960 {
			t.Fatalf("packet %d RTP ticks = %d, want 960", index, ticks)
		}
	}
	select {
	case <-ctrl.Done():
		t.Fatal("track was marked drained before the next pacing tick")
	default:
	}

	peer.closed.Store(true)
	close(ticks)
	if err := mx.Close(); err != nil {
		t.Fatalf("mixer.Close() error = %v", err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("streamMixedAudio() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("streamMixedAudio() did not stop")
	}
}

type recordingGiznetConn struct {
	testGiznetConn

	mu      sync.Mutex
	packets [][]byte
	written chan struct{}
}

func (c *recordingGiznetConn) Write(protocol byte, packet []byte) (int, error) {
	if protocol != giznet.ProtocolOpusPacket {
		return 0, errors.New("unexpected protocol")
	}
	c.mu.Lock()
	c.packets = append(c.packets, append([]byte(nil), packet...))
	c.mu.Unlock()
	c.written <- struct{}{}
	return len(packet), nil
}

func (c *recordingGiznetConn) packetCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.packets)
}

func (c *recordingGiznetConn) recordedPackets() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	packets := make([][]byte, len(c.packets))
	for i, packet := range c.packets {
		packets[i] = append([]byte(nil), packet...)
	}
	return packets
}

type peerConnModelListerFunc func(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error)

func (f peerConnModelListerFunc) ListModels(ctx context.Context, req adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
	return f(ctx, req)
}

type peerConnVoiceListerFunc func(context.Context, adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error)

func (f peerConnVoiceListerFunc) ListVoices(ctx context.Context, req adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error) {
	return f(ctx, req)
}

func TestPeerConnCloseClosesConn(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	clientConn, serverConn := newTestWebRTCConnPair(t, serverKey, clientKey, testGiznetSecurityPolicy{}, testGiznetSecurityPolicy{})
	defer clientConn.Close()

	peer := &PeerConn{Conn: serverConn}
	if err := peer.close(); err != nil {
		t.Fatalf("PeerConn.close() error = %v", err)
	}
	if err := serverConn.Close(); err != nil && !errors.Is(err, giznet.ErrConnClosed) {
		t.Fatalf("server Conn.Close() after PeerConn.close err=%v, want nil or %v", err, giznet.ErrConnClosed)
	}
}

func TestPeerConnMandatoryEventStreamTimesOutAndClosesConnection(t *testing.T) {
	listener := &blockingServiceListener{closed: make(chan struct{})}
	conn := &closeRecordingConn{testGiznetConn: testGiznetConn{}}
	peer := &PeerConn{Conn: conn, eventAcceptTimeout: 20 * time.Millisecond}
	stream, err := peer.acceptMandatoryEventStream(listener)
	if stream != nil {
		t.Fatalf("mandatory Event stream = %v, want nil", stream)
	}
	if !errors.Is(err, errPeerEventStreamClosed) {
		t.Fatalf("mandatory Event stream error = %v, want %v", err, errPeerEventStreamClosed)
	}
	if !conn.closed.Load() {
		t.Fatal("missing mandatory Event stream did not close the Peer connection")
	}
	select {
	case <-listener.closed:
	default:
		t.Fatal("missing mandatory Event stream did not close its listener")
	}
}

func TestHandleRPCStreamAlwaysClosesStream(t *testing.T) {
	for _, test := range []struct {
		name      string
		handleErr error
	}{
		{name: "normal EOF"},
		{name: "handler error", handleErr: errors.New("handle failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, client := net.Pipe()
			defer client.Close()

			handleRPCStream(server, func(net.Conn) error {
				return test.handleErr
			})

			if _, err := client.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
				t.Fatalf("peer read error = %v, want EOF", err)
			}
		})
	}
}

type blockingServiceListener struct {
	closed chan struct{}
	once   sync.Once
}

func (l *blockingServiceListener) Accept() (net.Conn, error) {
	<-l.closed
	return nil, net.ErrClosed
}

func (l *blockingServiceListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (*blockingServiceListener) Addr() net.Addr { return nil }

type closeRecordingConn struct {
	testGiznetConn
	closed atomic.Bool
}

func (c *closeRecordingConn) Close() error {
	c.closed.Store(true)
	return nil
}

func TestPeerConnCloseStopsAgentRuntime(t *testing.T) {
	ctx := context.Background()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, keyPair.Public, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	output := newPeerConnBlockingStream()
	runtime := &agenthost.Service{
		Host:      peerConnTestHost{output: output},
		PeerRun:   store,
		PublicKey: keyPair.Public,
		Source: agenthost.StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return agenthost.NewInputStream(1), nil
		}),
		Consumer: agenthost.StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}
	if _, err := runtime.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	peer := &PeerConn{agentHost: runtime}
	if err := peer.close(); err != nil {
		t.Fatalf("close() error = %v", err)
	}
	status, err := runtime.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.State != apitypes.PeerRunStatusStateStopped {
		t.Fatalf("runtime status after close = %+v", status)
	}
	if !output.closed() {
		t.Fatal("agent output stream was not closed")
	}
}

func TestPeerConnCloseDoesNotWaitForBlockedRuntimeTransition(t *testing.T) {
	ctx := context.Background()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, keyPair.Public, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	source := newPeerConnBlockingOpenInput()
	runtime := &agenthost.Service{
		Host:      peerConnTestHost{output: newPeerConnBlockingStream()},
		PeerRun:   store,
		PublicKey: keyPair.Public,
		Source:    source,
		Consumer: agenthost.StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}
	reloadDone := make(chan error, 1)
	go func() {
		_, err := runtime.Reload(ctx)
		reloadDone <- err
	}()
	select {
	case <-source.openEntered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for blocked Reload")
	}
	peer := &PeerConn{agentHost: runtime, agentInput: source, runtimeStopTimeout: 50 * time.Millisecond}
	startedAt := time.Now()
	err = peer.close()
	if err != nil {
		t.Fatalf("close() error = %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("close() waited %s for blocked runtime transition", elapsed)
	}
	close(source.openRelease)
	select {
	case err := <-reloadDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Reload() error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Reload after close")
	}
	status, statusErr := runtime.Status(ctx)
	if statusErr != nil {
		t.Fatalf("Status() error = %v", statusErr)
	}
	if status.State == apitypes.PeerRunStatusStateRunning {
		t.Fatalf("runtime status after canceled Reload = %+v", status)
	}
}

func TestPeerConnHandleTelemetryPacket(t *testing.T) {
	ctx := context.Background()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	peerRun := &peerrun.Server{Store: kv.NewMemory(nil)}
	metricStore := &peerConnFakeMetrics{}
	manager := NewManager(&peer.Server{Store: kv.NewMemory(nil)})
	manager.PeerRun = peerRun
	manager.Metrics = metricStore
	conn := &testGiznetConn{publicKey: keyPair.Public}
	peerConn := &PeerConn{
		Conn:    conn,
		Service: &PeerService{manager: manager},
	}
	percent := 77.0
	charging := true
	payload, err := proto.Marshal(&telemetrypb.TelemetryFrame{
		ObservedAtUnixMs: time.Unix(300, 0).UnixMilli(),
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{
				Percent:  &percent,
				Charging: &charging,
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := peerConn.handleTelemetryPacket(ctx, payload); err != nil {
		t.Fatalf("handleTelemetryPacket() error = %v", err)
	}
	if len(metricStore.samples) != 2 {
		t.Fatalf("metrics samples = %d, want 2", len(metricStore.samples))
	}
	if metricStore.samples[0].Name != peertelemetry.MetricBatteryPercent || metricStore.samples[0].Value != 77 {
		t.Fatalf("first metric = %+v", metricStore.samples[0])
	}
	status, err := peerRun.GetStatus(ctx, keyPair.Public)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.BatteryPercent == nil || *status.BatteryPercent != 77 {
		t.Fatalf("BatteryPercent = %#v, want 77", status.BatteryPercent)
	}
	if status.Charging == nil || !*status.Charging {
		t.Fatalf("Charging = %#v, want true", status.Charging)
	}
}

func TestPeerConnServeDirectPacketsDoesNotBlockOnTelemetry(t *testing.T) {
	originalShutdownTimeout := peerConnTelemetryShutdownTimeout
	peerConnTelemetryShutdownTimeout = 50 * time.Millisecond
	t.Cleanup(func() { peerConnTelemetryShutdownTimeout = originalShutdownTimeout })

	ctx := context.Background()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	percent := 77.0
	payload, err := proto.Marshal(&telemetrypb.TelemetryFrame{
		ObservedAtUnixMs: time.Unix(300, 0).UnixMilli(),
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{
				Percent: &percent,
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	packets := []peerConnTestPacket{{protocol: EventStreamTelemetry, payload: payload}}
	for range peerConnTelemetryQueueSize + 5 {
		packets = append(packets, peerConnTestPacket{protocol: EventStreamTelemetry, payload: payload})
	}
	packets = append(packets, peerConnTestPacket{protocol: giznet.ProtocolOpusPacket, payload: []byte{1, 2, 3}})
	conn := &peerConnPacketConn{
		testGiznetConn: testGiznetConn{publicKey: keyPair.Public},
		packets:        packets,
	}
	metricStore := newPeerConnBlockingMetrics()
	manager := NewManager(&peer.Server{Store: kv.NewMemory(nil)})
	manager.PeerRun = &peerrun.Server{Store: kv.NewMemory(nil)}
	manager.Metrics = metricStore
	peerConn := &PeerConn{
		Conn:    conn,
		Service: &PeerService{manager: manager},
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- peerConn.serveDirectPackets()
	}()

	select {
	case <-metricStore.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for telemetry metrics append")
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serveDirectPackets() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serveDirectPackets stayed blocked behind telemetry shutdown")
	}
	if got, want := conn.reads, len(packets)+1; got != want {
		t.Fatalf("direct packet reads = %d, want %d", got, want)
	}
	close(metricStore.release)
	select {
	case <-metricStore.finished:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for telemetry metrics append to finish")
	}
	_, err = manager.PeerRun.GetStatus(ctx, keyPair.Public)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
}

func TestManagerTelemetryStatusLockIsScopedByPeer(t *testing.T) {
	first, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(first) error = %v", err)
	}
	second, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(second) error = %v", err)
	}
	manager := NewManager(&peer.Server{Store: kv.NewMemory(nil)})
	if a, b := manager.telemetryStatusLock(first.Public), manager.telemetryStatusLock(first.Public); a == nil || a != b {
		t.Fatalf("same peer status locks = %p and %p, want same non-nil lock", a, b)
	}
	if a, b := manager.telemetryStatusLock(first.Public), manager.telemetryStatusLock(second.Public); a == nil || b == nil || a == b {
		t.Fatalf("different peer status locks = %p and %p, want different non-nil locks", a, b)
	}
	retained := manager.retainTelemetryStatusLock(first.Public, true)
	if retained == nil {
		t.Fatal("retainTelemetryStatusLock returned nil")
	}
	manager.releaseTelemetryStatusLock(first.Public)
	if _, ok := manager.telemetryStatusLocks[first.Public]; ok {
		t.Fatal("releaseTelemetryStatusLock should delete unreferenced peer lock")
	}
}

func TestPeerConnTelemetryStatusSyncSerializesCalls(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	next := &peerConnBlockingStatusSync{
		entered: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	syncer := peerConnTelemetryStatusSync{
		mu:   &sync.Mutex{},
		next: next,
	}
	errCh := make(chan error, 2)
	go func() {
		errCh <- syncer.SyncTelemetryStatus(context.Background(), keyPair.Public, peertelemetry.StatusPatch{BatteryPercent: new(10)})
	}()
	select {
	case <-next.entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first status sync")
	}
	go func() {
		errCh <- syncer.SyncTelemetryStatus(context.Background(), keyPair.Public, peertelemetry.StatusPatch{Charging: new(true)})
	}()
	select {
	case <-next.entered:
		t.Fatal("second status sync entered before first released")
	case <-time.After(100 * time.Millisecond):
	}
	close(next.release)
	for range 2 {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("SyncTelemetryStatus() error = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for status sync to finish")
		}
	}
	if next.calls != 2 {
		t.Fatalf("status sync calls = %d, want 2", next.calls)
	}
}

func TestPeerConnReloadsRuntimeWhenInputIsInactive(t *testing.T) {
	ctx := context.Background()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, keyPair.Public, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	source := newPeerRealtimeSource(genx.WithRealtimeStreamDelay(0))
	received := make(chan *genx.MessageChunk, 1)
	runtime := &agenthost.Service{
		Host:      peerConnTestHost{output: &peerConnBlockingStream{done: make(chan struct{})}},
		PeerRun:   store,
		PublicKey: keyPair.Public,
		Source:    source,
		Consumer: agenthost.StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}
	peer := &PeerConn{agentHost: runtime, agentInput: source}
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "audio", BeginOfStream: true}}
	if err := peer.pushAgentInputChunk(ctx, chunk); err != nil {
		t.Fatalf("pushAgentInputChunk() error = %v", err)
	}
	source.mu.RLock()
	input := source.current
	source.mu.RUnlock()
	if input == nil {
		t.Fatal("reload did not open an agent input stream")
	}
	go func() {
		got, _ := input.Next()
		received <- got
	}()
	select {
	case got := <-received:
		if got != chunk {
			t.Fatalf("received chunk = %p, want %p", got, chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for pushed chunk")
	}
}

func TestPeerConnDropsInactiveInputAfterWorkspaceSelectionChanges(t *testing.T) {
	ctx := context.Background()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, keyPair.Public, apitypes.AgentSelection{WorkspaceName: "realtime"}); err != nil {
		t.Fatalf("SetRunAgent(realtime) error = %v", err)
	}
	source := newPeerRealtimeSource(genx.WithRealtimeStreamDelay(0))
	runtime := &agenthost.Service{
		Host:      peerConnTestHost{output: &peerConnBlockingStream{done: make(chan struct{})}},
		PeerRun:   store,
		PublicKey: keyPair.Public,
		Source:    source,
		Consumer: agenthost.StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}
	if _, err := runtime.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	defer func() {
		if _, err := runtime.Stop(ctx); err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	}()
	if _, err := runtime.SetRunAgent(ctx, apitypes.AgentSelection{WorkspaceName: "assistant"}); err != nil {
		t.Fatalf("SetRunAgent(assistant) error = %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	peer := &PeerConn{agentHost: runtime, agentInput: source}
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "audio", BeginOfStream: true}}
	if err := peer.pushAgentInputChunk(ctx, chunk); err != nil {
		t.Fatalf("pushAgentInputChunk() error = %v", err)
	}
	source.mu.RLock()
	input := source.current
	source.mu.RUnlock()
	if input != nil {
		t.Fatal("stale Realtime chunk reopened the pending Assistant workspace")
	}
	agent, err := store.GetRunAgent(ctx, keyPair.Public)
	if err != nil {
		t.Fatalf("GetRunAgent() error = %v", err)
	}
	if agent.Pending == nil || agent.Pending.WorkspaceName != "assistant" {
		t.Fatalf("run agent = %+v, want pending assistant", agent)
	}
}

func TestPeerConnPushSerializesWithRuntimeTransition(t *testing.T) {
	ctx := context.Background()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, keyPair.Public, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	input := newBlockingPeerAgentInput()
	runtime := &agenthost.Service{
		Host:      peerConnTestHost{output: &peerConnBlockingStream{done: make(chan struct{})}},
		PeerRun:   store,
		PublicKey: keyPair.Public,
		Source:    input,
		Consumer: agenthost.StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}
	if _, err := runtime.Reload(ctx); err != nil {
		t.Fatalf("initial Reload() error = %v", err)
	}
	defer func() {
		if _, err := runtime.Stop(ctx); err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	}()
	peer := &PeerConn{agentHost: runtime, agentInput: input}
	pushDone := make(chan error, 1)
	go func() {
		pushDone <- peer.pushAgentInputChunk(ctx, &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "audio", BeginOfStream: true}})
	}()
	select {
	case <-input.pushEntered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for input push")
	}
	reloadDone := make(chan error, 1)
	go func() {
		_, err := runtime.Reload(ctx)
		reloadDone <- err
	}()
	select {
	case err := <-reloadDone:
		t.Fatalf("Reload() completed while the input write held the transition boundary: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if got := input.openCalls(); got != 1 {
		t.Fatalf("OpenAgentInput calls during push = %d, want 1", got)
	}
	close(input.pushRelease)
	select {
	case err := <-pushDone:
		if err != nil {
			t.Fatalf("pushAgentInputChunk() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for input push")
	}
	select {
	case err := <-reloadDone:
		if err != nil {
			t.Fatalf("Reload() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Reload")
	}
	if got := input.openCalls(); got != 2 {
		t.Fatalf("OpenAgentInput calls after push = %d, want 2", got)
	}
}

func TestPeerConnPushReturnsTransitionWaitError(t *testing.T) {
	ctx := context.Background()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, keyPair.Public, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	input := newBlockingPeerAgentInput()
	runtime := &agenthost.Service{
		Host:      peerConnTestHost{output: &peerConnBlockingStream{done: make(chan struct{})}},
		PeerRun:   store,
		PublicKey: keyPair.Public,
		Source:    input,
		Consumer: agenthost.StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}
	if _, err := runtime.Reload(ctx); err != nil {
		t.Fatalf("initial Reload() error = %v", err)
	}
	defer func() {
		input.releasePush()
		if _, err := runtime.Stop(ctx); err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	}()
	peer := &PeerConn{agentHost: runtime, agentInput: input}
	firstPushDone := make(chan error, 1)
	go func() {
		firstPushDone <- peer.pushAgentInputChunk(ctx, &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "audio", BeginOfStream: true}})
	}()
	select {
	case <-input.pushEntered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first input push")
	}
	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	err = peer.pushAgentInputChunk(waitCtx, &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "audio", BeginOfStream: true}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pushAgentInputChunk() error = %v, want deadline exceeded", err)
	}
	input.releasePush()
	select {
	case err := <-firstPushDone:
		if err != nil {
			t.Fatalf("first pushAgentInputChunk() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first input push")
	}
}

func TestPeerConnRestoresInactiveInputForSameWorkspaceSelection(t *testing.T) {
	ctx := context.Background()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, keyPair.Public, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent(demo) error = %v", err)
	}
	source := newPeerRealtimeSource(genx.WithRealtimeStreamDelay(0))
	runtime := &agenthost.Service{
		Host:      peerConnTestHost{output: &peerConnBlockingStream{done: make(chan struct{})}},
		PeerRun:   store,
		PublicKey: keyPair.Public,
		Source:    source,
		Consumer: agenthost.StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}
	if _, err := runtime.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	defer func() {
		if _, err := runtime.Stop(ctx); err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	}()
	observedRevision := runtime.RuntimeRevision()
	if _, err := runtime.SetRunAgent(ctx, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent(demo) error = %v", err)
	}
	if got := runtime.RuntimeRevision(); got != observedRevision {
		t.Fatalf("RuntimeRevision() after same-workspace selection = %d, want %d", got, observedRevision)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	peer := &PeerConn{agentHost: runtime, agentInput: source}
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "audio", BeginOfStream: true}}
	if err := peer.pushAgentInputChunk(ctx, chunk); err != nil {
		t.Fatalf("pushAgentInputChunk() error = %v", err)
	}
	source.mu.RLock()
	input := source.current
	source.mu.RUnlock()
	if input == nil {
		t.Fatal("same-workspace selection did not restore the inactive source")
	}
	received := make(chan *genx.MessageChunk, 1)
	go func() {
		got, _ := input.Next()
		received <- got
	}()
	select {
	case got := <-received:
		if got != chunk {
			t.Fatalf("received chunk = %p, want %p", got, chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for restored input")
	}
	agent, err := store.GetRunAgent(ctx, keyPair.Public)
	if err != nil {
		t.Fatalf("GetRunAgent() error = %v", err)
	}
	if agent.Pending != nil || agent.Active == nil || agent.Active.WorkspaceName != "demo" {
		t.Fatalf("run agent after same-workspace recovery = %+v", agent)
	}
}

type blockingPeerAgentInput struct {
	mu          sync.Mutex
	pushEntered chan struct{}
	pushRelease chan struct{}
	pushOnce    sync.Once
	releaseOnce sync.Once
	opens       int
}

type peerConnBlockingOpenInput struct {
	openEntered chan struct{}
	openRelease chan struct{}
	openOnce    sync.Once
}

func newPeerConnBlockingOpenInput() *peerConnBlockingOpenInput {
	return &peerConnBlockingOpenInput{openEntered: make(chan struct{}), openRelease: make(chan struct{})}
}

func (s *peerConnBlockingOpenInput) OpenAgentInput(context.Context) (genx.Stream, error) {
	s.openOnce.Do(func() { close(s.openEntered) })
	<-s.openRelease
	return agenthost.NewInputStream(1), nil
}

func (s *peerConnBlockingOpenInput) Push(context.Context, *genx.MessageChunk) error {
	return agenthost.ErrNoActiveInput
}

func (s *peerConnBlockingOpenInput) Close() error {
	return nil
}

func newBlockingPeerAgentInput() *blockingPeerAgentInput {
	return &blockingPeerAgentInput{pushEntered: make(chan struct{}), pushRelease: make(chan struct{})}
}

func (s *blockingPeerAgentInput) OpenAgentInput(context.Context) (genx.Stream, error) {
	s.mu.Lock()
	s.opens++
	s.mu.Unlock()
	return agenthost.NewInputStream(1), nil
}

func (s *blockingPeerAgentInput) Push(context.Context, *genx.MessageChunk) error {
	s.pushOnce.Do(func() { close(s.pushEntered) })
	<-s.pushRelease
	return nil
}

func (s *blockingPeerAgentInput) releasePush() {
	s.releaseOnce.Do(func() { close(s.pushRelease) })
}

func (s *blockingPeerAgentInput) Close() error {
	return nil
}

func (s *blockingPeerAgentInput) openCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opens
}

type peerConnFakeMetrics struct {
	samples []metrics.Sample
}

func (s *peerConnFakeMetrics) Append(_ context.Context, samples []metrics.Sample) error {
	s.samples = append(s.samples, samples...)
	return nil
}

func (s *peerConnFakeMetrics) Latest(context.Context, metrics.LatestQuery) (metrics.SeriesSet, error) {
	return nil, nil
}

func (s *peerConnFakeMetrics) Range(context.Context, metrics.RangeQuery) (metrics.SeriesSet, error) {
	return nil, nil
}

func (s *peerConnFakeMetrics) Aggregate(context.Context, metrics.AggregateQuery) (metrics.SeriesSet, error) {
	return nil, nil
}

func (s *peerConnFakeMetrics) Close() error {
	return nil
}

type peerConnBlockingMetrics struct {
	peerConnFakeMetrics
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
	once     sync.Once
	finish   sync.Once
}

func newPeerConnBlockingMetrics() *peerConnBlockingMetrics {
	return &peerConnBlockingMetrics{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func (s *peerConnBlockingMetrics) Append(_ context.Context, samples []metrics.Sample) error {
	s.once.Do(func() { close(s.started) })
	defer s.finish.Do(func() { close(s.finished) })
	select {
	case <-s.release:
	}
	return s.peerConnFakeMetrics.Append(context.Background(), samples)
}

type peerConnTestPacket struct {
	protocol byte
	payload  []byte
}

type peerConnPacketConn struct {
	testGiznetConn
	packets []peerConnTestPacket
	reads   int
}

func (c *peerConnPacketConn) Read(buf []byte) (byte, int, error) {
	c.reads++
	if len(c.packets) == 0 {
		return 0, 0, giznet.ErrClosed
	}
	packet := c.packets[0]
	c.packets = c.packets[1:]
	return packet.protocol, copy(buf, packet.payload), nil
}

type peerConnBlockingStatusSync struct {
	entered chan struct{}
	release chan struct{}
	calls   int
}

func (s *peerConnBlockingStatusSync) SyncTelemetryStatus(context.Context, giznet.PublicKey, peertelemetry.StatusPatch) error {
	s.calls++
	s.entered <- struct{}{}
	<-s.release
	return nil
}

//go:fix inline
func peerConnBoolPtr(v bool) *bool {
	return new(v)
}

//go:fix inline
func peerConnIntPtr(v int) *int {
	return new(v)
}

func TestPeerConnPCMChunkToInt16(t *testing.T) {
	chunk := &pcm.DataChunk{Data: []byte{0x34, 0x12, 0x78, 0x56}}
	got := peerConnPCMChunkToInt16(chunk)
	if len(got) != 2 {
		t.Fatalf("len(peerConnPCMChunkToInt16()) = %d", len(got))
	}
	if got[0] != 0x1234 || got[1] != 0x5678 {
		t.Fatalf("peerConnPCMChunkToInt16() = %#v", got)
	}
	if out := peerConnPCMChunkToInt16(nil); out != nil {
		t.Fatalf("peerConnPCMChunkToInt16(nil) = %#v", out)
	}
}

type peerConnTestHost struct {
	output genx.Stream
}

func TestAbortAgentInputTurnInterruptsWithoutClosingSource(t *testing.T) {
	input := &countingPeerAgentInput{pushed: make(chan *genx.MessageChunk, 1)}
	peer := &PeerConn{Conn: &testGiznetConn{publicKey: giznet.PublicKey{1}}, agentInput: input}
	if err := peer.abortAgentInputTurn(context.Background()); err != nil {
		t.Fatalf("abortAgentInputTurn() error = %v", err)
	}
	assertAgentInputInterrupt(t, input)
	if got := input.closeCount(); got != 0 {
		t.Fatalf("input close calls = %d, want 0", got)
	}
}

func TestAbortAgentInputTurnTreatsMissingInputAsNoop(t *testing.T) {
	peer := &PeerConn{Conn: &testGiznetConn{publicKey: giznet.PublicKey{1}}, agentInput: noActiveAgentInput{}}
	if err := peer.abortAgentInputTurn(context.Background()); err != nil {
		t.Fatalf("abortAgentInputTurn() with no active input error = %v, want nil", err)
	}
}

// TestAbortAgentInputTurnDoesNotBlockOnAFullInputQueue proves the denied-turn
// abort cannot pin agentInputMu behind an input source that never accepts the
// chunk: a cancelled caller context returns immediately, and an uncancelled
// caller is still bounded by peerConnInputAbortTimeout.
func TestAbortAgentInputTurnDoesNotBlockOnAFullInputQueue(t *testing.T) {
	input := &fullQueuePeerAgentInput{released: make(chan struct{})}
	defer close(input.released)
	peer := &PeerConn{Conn: &testGiznetConn{publicKey: giznet.PublicKey{1}}, agentInput: input}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- peer.abortAgentInputTurn(ctx) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("abortAgentInputTurn(cancelled) error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("abortAgentInputTurn() blocked on a full input queue despite a cancelled context")
	}

	// The lock must be free for a following caller.
	locked := make(chan struct{})
	go func() {
		peer.agentInputMu.Lock()
		peer.agentInputMu.Unlock()
		close(locked)
	}()
	select {
	case <-locked:
	case <-time.After(time.Second):
		t.Fatal("agentInputMu remained held after a cancelled abort")
	}
}

// fullQueuePeerAgentInput models an input source that waits for queue capacity:
// Push blocks until the caller's context ends or the queue is released.
type fullQueuePeerAgentInput struct{ released chan struct{} }

func (s *fullQueuePeerAgentInput) OpenAgentInput(context.Context) (genx.Stream, error) {
	return nil, agenthost.ErrNoActiveInput
}

func (s *fullQueuePeerAgentInput) Push(ctx context.Context, _ *genx.MessageChunk) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.released:
		return nil
	}
}

func (s *fullQueuePeerAgentInput) Close() error { return nil }

type noActiveAgentInput struct{}

func (noActiveAgentInput) OpenAgentInput(context.Context) (genx.Stream, error) {
	return nil, agenthost.ErrNoActiveInput
}

func (noActiveAgentInput) Push(context.Context, *genx.MessageChunk) error {
	return agenthost.ErrNoActiveInput
}

func (noActiveAgentInput) Close() error { return nil }

type countingPeerAgentInput struct {
	mu         sync.Mutex
	pushed     chan *genx.MessageChunk
	closeCalls int
}

func (s *countingPeerAgentInput) OpenAgentInput(context.Context) (genx.Stream, error) {
	return nil, nil
}

func (s *countingPeerAgentInput) Push(_ context.Context, chunk *genx.MessageChunk) error {
	s.pushed <- chunk
	return nil
}

func (s *countingPeerAgentInput) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeCalls++
	return nil
}

func assertAgentInputInterrupt(t *testing.T, input *countingPeerAgentInput) {
	t.Helper()
	select {
	case chunk := <-input.pushed:
		if chunk == nil || chunk.Ctrl == nil || !chunk.IsBeginOfStream() || chunk.Part != nil {
			t.Fatalf("abort pushed %#v, want control-only interrupt BOS", chunk)
		}
		if strings.TrimSpace(chunk.Ctrl.StreamID) == "" {
			t.Fatal("interrupt BOS is missing a fresh StreamID")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the interrupt chunk")
	}
}

func (s *countingPeerAgentInput) closeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeCalls
}

func (h peerConnTestHost) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	return h.output, nil
}

type peerConnBlockingStream struct {
	done chan struct{}
	once sync.Once
}

func newPeerConnBlockingStream() *peerConnBlockingStream {
	return &peerConnBlockingStream{done: make(chan struct{})}
}

func (s *peerConnBlockingStream) Next() (*genx.MessageChunk, error) {
	<-s.done
	return nil, context.Canceled
}

func (s *peerConnBlockingStream) Close() error {
	return s.CloseWithError(context.Canceled)
}

func (s *peerConnBlockingStream) CloseWithError(error) error {
	s.once.Do(func() { close(s.done) })
	return nil
}

func (s *peerConnBlockingStream) closed() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func TestPeerConnSequentialAudioRoutesKeepActiveRuntimeInput(t *testing.T) {
	ctx := context.Background()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, keyPair.Public, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent(demo) error = %v", err)
	}
	source := newPeerRealtimeSource(genx.WithRealtimeStreamDelay(0))
	runtime := &agenthost.Service{
		Host:      peerConnTestHost{output: &peerConnBlockingStream{done: make(chan struct{})}},
		PeerRun:   store,
		PublicKey: keyPair.Public,
		Source:    source,
		Consumer: agenthost.StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}
	status, err := runtime.Reload(ctx)
	if err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	defer func() {
		if _, err := runtime.Stop(ctx); err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	}()
	if status.StartedAt == nil {
		t.Fatalf("Reload() status = %+v, want StartedAt", status)
	}
	startedAt := *status.StartedAt
	revision := runtime.RuntimeRevision()
	source.mu.RLock()
	activeInput := source.current
	source.mu.RUnlock()

	peer := &PeerConn{agentHost: runtime, agentInput: source}
	for _, chunk := range []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "first", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "first", EndOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "second", BeginOfStream: true}},
	} {
		if err := peer.pushAgentInputChunk(ctx, chunk); err != nil {
			t.Fatalf("pushAgentInputChunk(%+v) error = %v", chunk.Ctrl, err)
		}
	}

	source.mu.RLock()
	currentInput := source.current
	source.mu.RUnlock()
	if currentInput != activeInput {
		t.Fatal("sequential route completion replaced the active input source")
	}
	if got := runtime.RuntimeRevision(); got != revision {
		t.Fatalf("RuntimeRevision() after sequential routes = %d, want %d", got, revision)
	}
	currentStatus, err := runtime.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if currentStatus.State != apitypes.PeerRunStatusStateRunning || currentStatus.StartedAt == nil || !currentStatus.StartedAt.Equal(startedAt) {
		t.Fatalf("Status() after sequential routes = %+v, want same running runtime", currentStatus)
	}
}
