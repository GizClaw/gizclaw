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

func TestRPCServerPetDefPixaDownloadStreamsBinary(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	payload := []byte("pixa-payload")
	pixaPath := "pet-defs/petdef-a/pixa"
	service := &fakeGameplayPixaDownloadService{
		petMetadata: rpcapi.PetDefPixaDownloadResponse{
			Id:        "petdef-a",
			PixaPath:  &pixaPath,
			SizeBytes: int64(len(payload)),
		},
		petPayload: payload,
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

	params, err := newRPCRequestParams(rpcapi.PetDefPixaDownloadRequest{Id: "petdef-a"}, (*rpcapi.RPCRequest_Params).FromPetDefPixaDownloadRequest)
	if err != nil {
		t.Fatalf("newRPCRequestParams() error = %v", err)
	}
	if err := stream.WriteRequest(newRPCRequest("petdef-pixa-download", rpcapi.RPCMethodServerPetDefPixaDownload, params)); err != nil {
		t.Fatalf("WriteRequest() error = %v", err)
	}
	if err := stream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}

	resp, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerPetDefPixaDownload)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("petdef pixa response error = %+v", resp.Error)
	}
	gotMetadata, err := resp.Result.AsPetDefPixaDownloadResponse()
	if err != nil {
		t.Fatalf("AsPetDefPixaDownloadResponse() error = %v", err)
	}
	if gotMetadata.Id != "petdef-a" || gotMetadata.SizeBytes != int64(len(payload)) || gotMetadata.PixaPath == nil || *gotMetadata.PixaPath != pixaPath {
		t.Fatalf("metadata = %+v", gotMetadata)
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
	if service.petRequest.Id != "petdef-a" {
		t.Fatalf("request = %+v", service.petRequest)
	}
}

type fakeGameplayPixaDownloadService struct {
	petMetadata rpcapi.PetDefPixaDownloadResponse
	petPayload  []byte
	petRequest  rpcapi.PetDefPixaDownloadRequest
}

func (f *fakeGameplayPixaDownloadService) PreparePetDefPixaDownload(_ context.Context, request rpcapi.PetDefPixaDownloadRequest) (rpcapi.PetDefPixaDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	f.petRequest = request
	return f.petMetadata, io.NopCloser(bytes.NewReader(f.petPayload)), nil, nil
}

func (f *fakeGameplayPixaDownloadService) PrepareBadgeDefPixaDownload(context.Context, rpcapi.BadgeDefPixaDownloadRequest) (rpcapi.BadgeDefPixaDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	return rpcapi.BadgeDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "not found"}, nil
}

func (f *fakeGameplayPixaDownloadService) Dispatch(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	return nil, false, nil
}
