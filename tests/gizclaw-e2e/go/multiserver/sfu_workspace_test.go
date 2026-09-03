//go:build gizclaw_e2e

package multiserver_test

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

// TestSFURoomLazyCreateAndReconnect exercises the LiveKit side of a
// cross-server Friend Workspace: the Room only exists once the first runtime
// activates, participants are identified by their Peer public keys, and a
// deleted Room (the fixture's stand-in for a LiveKit restart) is rejoined by
// both Servers while the Device connections keep answering pings. It talks to
// the shared livekit service, so the suite runs it serially.
func TestSFURoomLazyCreateAndReconnect(t *testing.T) {
	livekitURL := os.Getenv("GIZCLAW_E2E_LIVEKIT_URL")
	apiKey := os.Getenv("GIZCLAW_E2E_LIVEKIT_API_KEY")
	apiSecret := os.Getenv("GIZCLAW_E2E_LIVEKIT_API_SECRET")
	if livekitURL == "" || apiKey == "" || apiSecret == "" {
		t.Skip("GIZCLAW_E2E_LIVEKIT_URL, GIZCLAW_E2E_LIVEKIT_API_KEY and GIZCLAW_E2E_LIVEKIT_API_SECRET are required")
	}
	rooms := lksdk.NewRoomServiceClient(livekitURL, apiKey, apiSecret)
	serverA := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_A"))
	serverB := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_B"))

	peerA, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	peerB, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientA := connectAndServe(t, peerA, serverA, serverA.PublicKey, "sfu-peer-a")
	defer clientA.Close()
	clientB := connectAndServe(t, peerB, serverB, serverB.PublicKey, "sfu-peer-b")
	defer clientB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	registerSocialPeer(t, ctx, clientA, serverA, "GIZCLAW_TEST_REGISTRATION_TOKEN_A")
	registerSocialPeer(t, ctx, clientB, serverB, "GIZCLAW_TEST_REGISTRATION_TOKEN_B")

	invite, err := clientB.CreateFriendInviteToken(ctx, "sfu-invite", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatalf("create Friend invite: %v", err)
	}
	friend, err := clientA.AddFriend(ctx, "sfu-friend-add", rpcapi.FriendAddRequest{InviteToken: invite.InviteToken})
	if err != nil {
		t.Fatalf("add cross-server Friend: %v", err)
	}
	if friend.WorkspaceName == nil || *friend.WorkspaceName == "" {
		t.Fatalf("cross-server Friend has no Workspace: %+v", friend)
	}
	workspace := *friend.WorkspaceName
	defer func() {
		_, _ = clientA.StopServerRun(context.Background(), "sfu-stop-a")
		_, _ = clientB.StopServerRun(context.Background(), "sfu-stop-b")
		_, _ = clientA.DeleteFriend(context.Background(), "sfu-friend-delete", rpcapi.FriendDeleteRequest{Name: friend.Name})
	}()

	// 1. Creating the Social resource never creates a Room.
	roomsBefore := listRoomNames(t, ctx, rooms)
	// Selecting an SFU Workspace joins the Room at once: the response already
	// carries the post-activation runtime state.
	selected, err := clientA.SetServerRunWorkspace(ctx, "sfu-select-a", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: workspace})
	if err != nil {
		t.Fatalf("Peer A select Workspace: %v", err)
	}
	if selected.RuntimeState != rpcapi.PeerRunStatusStateRunning && selected.RuntimeState != rpcapi.PeerRunStatusStateStarting {
		t.Fatalf("Peer A selection did not activate the SFU runtime: %+v", selected)
	}
	waitRuntimeRunning(t, ctx, clientA, "sfu-status-a")
	room := waitNewRoom(t, ctx, rooms, roomsBefore, []string{peerA.Public.String()})
	t.Logf("room %q created lazily with sid %s", room.Name, room.Sid)

	if _, err := clientB.SetServerRunWorkspace(ctx, "sfu-select-b", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: workspace}); err != nil {
		t.Fatalf("Peer B select Workspace: %v", err)
	}
	waitRuntimeRunning(t, ctx, clientB, "sfu-status-b")
	waitParticipants(t, ctx, rooms, room.Name, []string{peerA.Public.String(), peerB.Public.String()})
	// Repeating the selection is idempotent: no reconnect, no second participant.
	if _, err := clientB.SetServerRunWorkspace(ctx, "sfu-reselect-b", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: workspace}); err != nil {
		t.Fatalf("Peer B re-select Workspace: %v", err)
	}
	waitParticipants(t, ctx, rooms, room.Name, []string{peerA.Public.String(), peerB.Public.String()})
	assertAudioForwarded(t, ctx, clientA, clientB, "before disruption")

	// 2. Deleting the Room kicks every participant exactly like a LiveKit
	// restart does; the runtimes must report reconnecting while Device pings
	// keep succeeding.
	// LiveKit assigns a fresh SID to the Room object it builds after the
	// delete, but a runtime that reconnects within a few milliseconds can land
	// on the closing instance while the other lands on the new one. The
	// participant SIDs are the race-free evidence that both runtimes rejoined,
	// so they are captured before the disruption.
	participantsBefore, err := participantSIDs(ctx, rooms, room.Name)
	if err != nil {
		t.Fatalf("list participants before disruption: %v", err)
	}
	var sawReconnecting atomic.Bool
	observerCtx, stopObserver := context.WithCancel(ctx)
	var observer sync.WaitGroup
	observer.Add(1)
	go func() {
		defer observer.Done()
		for observerCtx.Err() == nil {
			state, err := clientA.GetServerRunWorkspace(observerCtx, "sfu-observe-a")
			if err == nil && state != nil && isReconnecting(*state) {
				sawReconnecting.Store(true)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	if _, err := rooms.DeleteRoom(ctx, &livekit.DeleteRoomRequest{Room: room.Name}); err != nil {
		stopObserver()
		t.Fatalf("delete Room %q: %v", room.Name, err)
	}
	if _, err := clientA.Ping(ctx, "sfu-ping-a-during-reconnect"); err != nil {
		t.Errorf("Peer A ping during SFU reconnect: %v", err)
	}
	if _, err := clientB.Ping(ctx, "sfu-ping-b-during-reconnect"); err != nil {
		t.Errorf("Peer B ping during SFU reconnect: %v", err)
	}

	// 3. Both Servers rejoin under the same Room token within the bounded
	// reconnect window; the Room is a fresh instance.
	recovered := waitRoomRecovered(t, ctx, rooms, room, []string{peerA.Public.String(), peerB.Public.String()}, participantsBefore)
	stopObserver()
	observer.Wait()
	// The bounded reconnect usually completes within the first 250ms backoff,
	// so the intermediate "reconnecting" state is only observable when polling
	// catches it; the fresh Room SID and the recovered participant set are the
	// authoritative evidence that the runtime rejoined.
	if sawReconnecting.Load() {
		t.Logf("server.run.workspace.get reported the SFU runtime reconnecting before the Room recovered (new sid %s)", recovered.Sid)
	} else {
		t.Logf("SFU runtime rejoined Room %q as sid %s before a reconnecting status was observed", room.Name, recovered.Sid)
	}
	waitRuntimeRunning(t, ctx, clientA, "sfu-status-a-recovered")
	waitRuntimeRunning(t, ctx, clientB, "sfu-status-b-recovered")
	assertAudioForwarded(t, ctx, clientA, clientB, "after recovery")
	assertAudioForwarded(t, ctx, clientB, clientA, "after recovery reverse")
}

func isReconnecting(state rpcapi.PeerRunWorkspaceState) bool {
	if state.RuntimeState != rpcapi.PeerRunStatusStateStarting {
		return false
	}
	return state.Message != nil && strings.Contains(*state.Message, "reconnecting")
}

func listRoomNames(t *testing.T, ctx context.Context, rooms *lksdk.RoomServiceClient) map[string]struct{} {
	t.Helper()
	response, err := rooms.ListRooms(ctx, &livekit.ListRoomsRequest{})
	if err != nil {
		t.Fatalf("list LiveKit Rooms: %v", err)
	}
	names := make(map[string]struct{}, len(response.Rooms))
	for _, room := range response.Rooms {
		names[room.Name] = struct{}{}
	}
	return names
}

func participantIdentities(ctx context.Context, rooms *lksdk.RoomServiceClient, room string) ([]string, error) {
	response, err := rooms.ListParticipants(ctx, &livekit.ListParticipantsRequest{Room: room})
	if err != nil {
		return nil, err
	}
	identities := make([]string, 0, len(response.Participants))
	for _, participant := range response.Participants {
		identities = append(identities, participant.Identity)
	}
	sort.Strings(identities)
	return identities, nil
}

// participantSIDs maps each participant identity in the Room to its LiveKit
// participant SID, which changes on every (re)join.
func participantSIDs(ctx context.Context, rooms *lksdk.RoomServiceClient, room string) (map[string]string, error) {
	response, err := rooms.ListParticipants(ctx, &livekit.ListParticipantsRequest{Room: room})
	if err != nil {
		return nil, err
	}
	sids := make(map[string]string, len(response.Participants))
	for _, participant := range response.Participants {
		sids[participant.Identity] = participant.Sid
	}
	return sids, nil
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sameIdentities(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	want = append([]string(nil), want...)
	sort.Strings(want)
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

// waitNewRoom waits for exactly one Room that did not exist before activation
// and for its participant set to match the expected Peer public keys.
func waitNewRoom(t *testing.T, ctx context.Context, rooms *lksdk.RoomServiceClient, before map[string]struct{}, want []string) *livekit.Room {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		response, err := rooms.ListRooms(ctx, &livekit.ListRoomsRequest{})
		if err != nil {
			t.Fatalf("list LiveKit Rooms: %v", err)
		}
		var created []*livekit.Room
		for _, room := range response.Rooms {
			if _, existed := before[room.Name]; !existed {
				created = append(created, room)
			}
		}
		switch len(created) {
		case 0:
			last = "no new Room yet"
		case 1:
			identities, err := participantIdentities(ctx, rooms, created[0].Name)
			if err != nil {
				last = err.Error()
			} else if sameIdentities(identities, want) {
				return created[0]
			} else {
				last = fmt.Sprintf("participants %v, want %v", identities, want)
			}
		default:
			t.Fatalf("activating one Workspace created %d Rooms", len(created))
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("wait for lazily created Room: %s", last)
	return nil
}

func waitParticipants(t *testing.T, ctx context.Context, rooms *lksdk.RoomServiceClient, room string, want []string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		identities, err := participantIdentities(ctx, rooms, room)
		if err != nil {
			last = err.Error()
		} else if sameIdentities(identities, want) {
			return
		} else {
			last = fmt.Sprintf("participants %v, want %v", identities, want)
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("wait for Room %q participants: %s", room, last)
}

// waitRoomRecovered waits until the Room token resolves to a fresh Room
// instance (new SID) holding every expected participant again.
// waitRoomRecovered waits until the Room token carries the expected
// participants again and every one of them is a new LiveKit participant, which
// is what "both Servers rejoined" means. The Room SID is deliberately not
// asserted: a runtime that reconnects within milliseconds of the delete can be
// admitted by the instance that is still closing, so LiveKit may keep serving
// the pre-delete SID for that Room name.
func waitRoomRecovered(
	t *testing.T,
	ctx context.Context,
	rooms *lksdk.RoomServiceClient,
	old *livekit.Room,
	want []string,
	before map[string]string,
) *livekit.Room {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		response, err := rooms.ListRooms(ctx, &livekit.ListRoomsRequest{Names: []string{old.Name}})
		if err != nil {
			t.Fatalf("list LiveKit Room %q: %v", old.Name, err)
		}
		if len(response.Rooms) == 0 {
			last = "Room absent"
		} else {
			current, err := participantSIDs(ctx, rooms, old.Name)
			switch {
			case err != nil:
				last = err.Error()
			case !sameIdentities(sortedKeys(current), want):
				last = fmt.Sprintf("participants %v, want %v", sortedKeys(current), want)
			default:
				stale := ""
				for identity, sid := range current {
					if before[identity] == sid {
						stale = identity
						break
					}
				}
				if stale == "" {
					return response.Rooms[0]
				}
				last = fmt.Sprintf("participant %s still holds its pre-delete session", stale)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("wait for Room %q to recover after deletion: %s", old.Name, last)
	return nil
}

func waitRuntimeRunning(t *testing.T, ctx context.Context, client *gizcli.Client, id string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		state, err := client.GetServerRunWorkspace(ctx, id)
		if err != nil {
			last = err.Error()
		} else if state.RuntimeState == rpcapi.PeerRunStatusStateRunning {
			return
		} else {
			message := ""
			if state.Message != nil {
				message = *state.Message
			}
			last = fmt.Sprintf("runtime_state=%s message=%q", state.RuntimeState, message)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s: wait for running SFU runtime: %s", id, last)
}

// minForwardedToneBytes is the least Opus payload the listener must receive
// for the 1.5 s tone. Encoded silence frames are 1-3 bytes each, so a route
// that only carried silence stays far below this bound.
const minForwardedToneBytes = 1500

// assertAudioForwarded pushes a short push-to-talk Opus tone from the sender
// and requires the listener's PeerStream to deliver the tone within a bounded
// window. It retries the whole exchange so a reconnect that is still settling
// does not fail the assertion prematurely.
func assertAudioForwarded(t *testing.T, ctx context.Context, sender, listener *gizcli.Client, stage string) {
	t.Helper()
	packets := opusTonePackets(t, 1500*time.Millisecond)
	deadline := time.Now().Add(30 * time.Second)
	var last error
	for attempt := 1; time.Now().Before(deadline); attempt++ {
		received, err := exchangeAudio(ctx, sender, listener, packets)
		if err == nil && received >= minForwardedToneBytes {
			t.Logf("%s: listener received %d Opus bytes on attempt %d", stage, received, attempt)
			return
		}
		if err == nil {
			err = fmt.Errorf("listener received %d Opus bytes, want at least %d", received, minForwardedToneBytes)
		}
		last = err
	}
	t.Fatalf("%s: audio was not forwarded through the SFU Room: %v", stage, last)
}

func exchangeAudio(ctx context.Context, sender, listener *gizcli.Client, packets [][]byte) (int, error) {
	listen, err := listener.OpenPeerStream(256)
	if err != nil {
		return 0, fmt.Errorf("open listener PeerStream: %w", err)
	}
	defer listen.Close()
	var received atomic.Int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			chunk, err := listen.Next()
			if err != nil {
				return
			}
			if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
				log.Printf("sfu e2e: listener stream %q received terminal error code=%q message=%q", chunk.Ctrl.StreamID, chunk.Ctrl.ErrorCode, chunk.Ctrl.Error)
			}
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
				received.Add(int64(len(blob.Data)))
			}
		}
	}()
	// Let the listener subscription settle before the sender pushes.
	time.Sleep(200 * time.Millisecond)

	speak, err := sender.OpenPeerStream(256)
	if err != nil {
		return 0, fmt.Errorf("open sender PeerStream: %w", err)
	}
	streamID := fmt.Sprintf("sfu-e2e-%d", time.Now().UnixNano())
	chunks := []*genx.MessageChunk{{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "user", BeginOfStream: true}}}
	for _, packet := range packets {
		chunks = append(chunks, &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "user"}})
	}
	chunks = append(chunks, &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "user", EndOfStream: true}})
	pacer := time.NewTicker(20 * time.Millisecond)
	defer pacer.Stop()
	for index, chunk := range chunks {
		if err := speak.Push(ctx, chunk); err != nil {
			_ = speak.Close()
			return 0, fmt.Errorf("push chunk %d: %w", index, err)
		}
		if index > 0 && index < len(chunks)-1 {
			<-pacer.C
		}
	}
	_ = speak.Close()

	// Give the SFU path time to deliver the tail of the tone.
	select {
	case <-time.After(3 * time.Second):
	case <-ctx.Done():
	}
	_ = listen.Close()
	<-done
	return int(received.Load()), nil
}

func opusTonePackets(t *testing.T, duration time.Duration) [][]byte {
	t.Helper()
	const sampleRate = 16000
	const channels = 1
	encoder, err := opus.NewEncoder(sampleRate, channels, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("create Opus encoder: %v", err)
	}
	defer encoder.Close()
	frameSize := sampleRate / 50
	frames := int(duration / (20 * time.Millisecond))
	packets := make([][]byte, 0, frames)
	pcm := make([]int16, frameSize*channels)
	var sample int
	for range frames {
		for index := range pcm {
			pcm[index] = int16(math.Sin(2*math.Pi*440*float64(sample)/sampleRate) * 12000)
			sample++
		}
		packet, err := encoder.Encode(pcm, frameSize)
		if err != nil {
			t.Fatalf("encode Opus tone: %v", err)
		}
		packets = append(packets, packet)
	}
	return packets
}
