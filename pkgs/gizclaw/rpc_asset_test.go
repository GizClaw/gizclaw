package gizclaw

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func TestRPCAssetDownloadStreamsMetadataAndBytes(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	closed := &atomic.Bool{}
	service := &fakeRPCAssetDownloadService{
		response: rpcpb.AssetDownloadResponse{Metadata: &rpcpb.AssetMetadata{
			Ref:       "asset://01010101010101010101010101010101",
			MediaType: "image/png",
			SizeBytes: 7,
			Sha256:    "239f59ed55e737c77147cf55ad0c1b030b6d7ee748a7426952f9b852d5a935e5",
			CreatedAt: "2026-07-16T08:00:00Z",
		}},
		reader: &trackingReadCloser{Reader: bytes.NewBufferString("payload"), closed: closed},
	}
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- (&rpcServer{serverResources: service}).Handle(serverSide)
	}()

	stream, err := newRPCStream(context.Background(), clientSide)
	if err != nil {
		t.Fatal(err)
	}
	params := &rpcapi.RPCPayload{}
	if err := params.FromAssetDownloadRequest(rpcpb.AssetDownloadRequest{Ref: service.response.Metadata.Ref}); err != nil {
		t.Fatal(err)
	}
	request := newRPCRequest("asset-download", rpcapi.RPCMethodServerAssetDownload, params)
	if err := stream.WriteRequest(request); err != nil {
		t.Fatal(err)
	}
	if err := stream.WriteEOS(); err != nil {
		t.Fatal(err)
	}
	response, _, err := stream.ReadResponseEnvelopeForMethod(rpcapi.RPCMethodServerAssetDownload)
	if err != nil {
		t.Fatal(err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("response = %#v", response)
	}
	metadata, err := response.Result.AsAssetDownloadResponse()
	if err != nil || metadata.GetMetadata().GetRef() != service.response.Metadata.Ref {
		t.Fatalf("metadata = %#v, %v", metadata.GetMetadata(), err)
	}
	var payload bytes.Buffer
	for {
		frame, err := stream.ReadFrame()
		if err != nil {
			t.Fatal(err)
		}
		if frame.Type == rpcapi.FrameTypeEOS {
			break
		}
		if frame.Type != rpcapi.FrameTypeBinary {
			t.Fatalf("frame type = %v", frame.Type)
		}
		payload.Write(frame.Payload)
	}
	if payload.String() != "payload" {
		t.Fatalf("payload = %q", payload.String())
	}
	_ = stream.Close()
	_ = clientSide.Close()
	if err := <-serverErr; err != nil {
		t.Fatalf("server error = %v", err)
	}
	if !closed.Load() {
		t.Fatal("asset reader was not closed")
	}
}

type fakeRPCAssetDownloadService struct {
	response rpcpb.AssetDownloadResponse
	reader   io.ReadCloser
	rpcErr   *rpcapi.RPCError
	err      error
}

func (s *fakeRPCAssetDownloadService) Dispatch(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	return nil, false, nil
}

func (s *fakeRPCAssetDownloadService) PrepareAssetDownload(context.Context, rpcpb.AssetDownloadRequest) (rpcpb.AssetDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	return s.response, s.reader, s.rpcErr, s.err
}

type trackingReadCloser struct {
	io.Reader
	closed *atomic.Bool
}

func (r *trackingReadCloser) Close() error {
	r.closed.Store(true)
	return nil
}
