//go:build gizclaw_e2e

package rpc_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	cgointernal "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cgo/internal"
	"github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/internal/serverrpc"
)

func TestCSDKHistoryAudioDownloads(t *testing.T) {
	server, err := serverrpc.New()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	serverErr := make(chan error, 1)
	clientDone := make(chan struct{})
	defer close(clientDone)
	go func() {
		conn, err := server.Accept(ctx)
		if err != nil {
			serverErr <- err
			return
		}
		serverErr <- serverrpc.ServeAudioDownloads(ctx, conn)
		<-clientDone
		_ = conn.Close()
	}()

	identity, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	client, err := cgointernal.NewClientWithCredentials(
		"http://"+server.Endpoint,
		identity.Private.String(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	baseline := requireTransportSnapshot(t, client)
	requireMandatoryCTransports(t, baseline)

	workspaceFrames, err := client.CallStream(
		rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_HISTORY_AUDIO_DOWNLOAD,
		&rpcpb.WorkspaceHistoryAudioDownloadRequest{
			WorkspaceName: "workspace-a",
			HistoryName:   "history-a",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	workspaceResponse := requireAudioDownloadFrames(
		t,
		rpcapi.RPCMethodServerWorkspaceHistoryAudioDownload,
		workspaceFrames,
		serverrpc.WorkspaceAudioPayload,
	)
	if workspaceResponse.Result == nil {
		t.Fatal("workspace audio response has no result")
	}
	workspaceMetadata, err := workspaceResponse.Result.AsWorkspaceHistoryAudioDownloadResponse()
	if err != nil {
		t.Fatal(err)
	}
	if workspaceMetadata.WorkspaceName != "workspace-a" ||
		workspaceMetadata.HistoryName != "history-a" ||
		workspaceMetadata.MimeType != "audio/ogg; codecs=opus" ||
		workspaceMetadata.SizeBytes != int64(len(serverrpc.WorkspaceAudioPayload)) {
		t.Fatalf("workspace audio metadata = %+v", workspaceMetadata)
	}
	requireNoRPCChannels(t, client, baseline)

	friendGroupFrames, err := client.CallStream(
		rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_AUDIO_DOWNLOAD,
		&rpcpb.FriendGroupMessageAudioDownloadRequest{
			FriendGroupName: "group-a",
			HistoryName:     "history-b",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	friendGroupResponse := requireAudioDownloadFrames(
		t,
		rpcapi.RPCMethodServerFriendGroupMessagesAudioDownload,
		friendGroupFrames,
		serverrpc.FriendGroupAudioPayload,
	)
	if friendGroupResponse.Result == nil {
		t.Fatal("Friend Group audio response has no result")
	}
	friendGroupMetadata, err := friendGroupResponse.Result.AsFriendGroupMessageAudioDownloadResponse()
	if err != nil {
		t.Fatal(err)
	}
	if friendGroupMetadata.FriendGroupName != "group-a" ||
		friendGroupMetadata.HistoryName != "history-b" ||
		friendGroupMetadata.MimeType != "audio/ogg; codecs=opus" ||
		friendGroupMetadata.SizeBytes != int64(len(serverrpc.FriendGroupAudioPayload)) {
		t.Fatalf("Friend Group audio metadata = %+v", friendGroupMetadata)
	}
	requireNoRPCChannels(t, client, baseline)

	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func requireAudioDownloadFrames(
	t *testing.T,
	method rpcapi.RPCMethod,
	frames []cgointernal.StreamFrame,
	wantAudio string,
) *rpcapi.RPCResponse {
	t.Helper()
	if len(frames) < 4 || frames[0].Type != cgointernal.RPCFrameBinary ||
		frames[len(frames)-1].Type != cgointernal.RPCFrameEOS {
		t.Fatalf("%s frames = %+v", method, frames)
	}
	response, err := rpcapi.DecodeResponseFrameForMethod(method, rpcapi.Frame{
		Type: rpcapi.FrameTypeBinary, Payload: frames[0].Data,
	})
	if err != nil {
		t.Fatal(err)
	}
	var audio bytes.Buffer
	for _, frame := range frames[1 : len(frames)-1] {
		if frame.Type != cgointernal.RPCFrameBinary {
			t.Fatalf("%s audio frame type = %d", method, frame.Type)
		}
		audio.Write(frame.Data)
	}
	if audio.String() != wantAudio {
		t.Fatalf("%s audio = %q, want %q", method, audio.String(), wantAudio)
	}
	return response
}
