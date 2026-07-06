package gizclaw

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestRPCServerWorkspaceHistoryAudioGetStreamsBinary(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	payload := []byte("opus-payload")
	service := &fakeWorkspaceHistoryAudioService{
		metadata: rpcapi.WorkspaceHistoryAudioGetResponse{
			WorkspaceName: "main",
			HistoryId:     "h1",
			MimeType:      "audio/opus",
			SizeBytes:     int64(len(payload)),
		},
		payload: payload,
	}
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- (&rpcServer{serverResources: service}).Handle(serverSide)
	}()

	stream, err := newRPCStream(context.Background(), clientSide)
	if err != nil {
		t.Fatalf("newRPCStream() error = %v", err)
	}
	defer stream.Close()

	params, err := newRPCRequestParams(rpcapi.WorkspaceHistoryAudioGetRequest{
		WorkspaceName: "main",
		HistoryId:     "h1",
	}, (*rpcapi.RPCRequest_Params).FromWorkspaceHistoryAudioGetRequest)
	if err != nil {
		t.Fatalf("newRPCRequestParams() error = %v", err)
	}
	if err := stream.WriteRequest(newRPCRequest("workspace-history-audio-get", rpcapi.RPCMethodServerWorkspaceHistoryAudioGet, params)); err != nil {
		t.Fatalf("WriteRequest() error = %v", err)
	}
	if err := stream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}

	resp, err := stream.ReadResponse()
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("workspace history audio response error = %+v", resp.Error)
	}
	gotMetadata, err := resp.Result.AsWorkspaceHistoryAudioGetResponse()
	if err != nil {
		t.Fatalf("AsWorkspaceHistoryAudioGetResponse() error = %v", err)
	}
	if gotMetadata != service.metadata {
		t.Fatalf("metadata = %+v, want %+v", gotMetadata, service.metadata)
	}

	frame, err := stream.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame(binary) error = %v", err)
	}
	if frame.Type != rpcapi.FrameTypeBinary || !bytes.Equal(frame.Payload, payload) {
		t.Fatalf("binary frame = %+v", frame)
	}
	frame, err = stream.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame(EOS) error = %v", err)
	}
	if frame.Type != rpcapi.FrameTypeEOS {
		t.Fatalf("last frame type = %d, want EOS", frame.Type)
	}
	if err := clientSide.Close(); err != nil {
		t.Fatalf("client close error = %v", err)
	}
	select {
	case err := <-serverErrCh:
		if err != nil {
			t.Fatalf("server error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish")
	}
	if service.request.WorkspaceName != "main" || service.request.HistoryId != "h1" {
		t.Fatalf("request = %+v", service.request)
	}
}

type fakeWorkspaceHistoryAudioService struct {
	metadata rpcapi.WorkspaceHistoryAudioGetResponse
	payload  []byte
	request  rpcapi.WorkspaceHistoryAudioGetRequest
}

func (f *fakeWorkspaceHistoryAudioService) PrepareWorkspaceHistoryAudioGet(_ context.Context, request rpcapi.WorkspaceHistoryAudioGetRequest) (rpcapi.WorkspaceHistoryAudioGetResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	f.request = request
	return f.metadata, io.NopCloser(bytes.NewReader(f.payload)), nil, nil
}

func (f *fakeWorkspaceHistoryAudioService) Dispatch(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	return nil, false, nil
}
