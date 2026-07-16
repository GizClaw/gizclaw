package gizclaw

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestRPCServerPetPixaDownloadStreamsBinary(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	payload := []byte("pet-pixa-payload")
	pixaPath := "pet-defs/petdef-a/pixa"
	service := &fakeGameplayPixaDownloadService{
		petPixaMetadata: rpcapi.PetPixaDownloadResponse{
			PetId:     "pet-a",
			PetdefId:  "petdef-a",
			PixaPath:  &pixaPath,
			SizeBytes: int64(len(payload)),
		},
		petPixaPayload: payload,
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

	params, err := newRPCRequestParams(rpcapi.PetPixaDownloadRequest{PetId: "pet-a"}, (*rpcapi.RPCPayload).FromServerPetPixaDownloadRequest)
	if err != nil {
		t.Fatalf("newRPCRequestParams() error = %v", err)
	}
	if err := stream.WriteRequest(newRPCRequest("pet-pixa-download", rpcapi.RPCMethodServerPetPixaDownload, params)); err != nil {
		t.Fatalf("WriteRequest() error = %v", err)
	}
	if err := stream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}

	resp, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerPetPixaDownload)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("pet pixa response error = %+v", resp.Error)
	}
	gotMetadata, err := resp.Result.AsServerPetPixaDownloadResponse()
	if err != nil {
		t.Fatalf("AsServerPetPixaDownloadResponse() error = %v", err)
	}
	if gotMetadata.PetId != "pet-a" || gotMetadata.PetdefId != "petdef-a" || gotMetadata.SizeBytes != int64(len(payload)) || gotMetadata.PixaPath == nil || *gotMetadata.PixaPath != pixaPath {
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
	if service.petPixaRequest.PetId != "pet-a" {
		t.Fatalf("request = %+v", service.petPixaRequest)
	}
}

func TestWriteRPCDownloadPreservesWriteEOF(t *testing.T) {
	conn := &failAfterWritesConn{remaining: 2, err: io.EOF}
	stream := &rpcStream{ctx: context.Background(), conn: conn}
	req := &rpcapi.RPCRequest{Id: "download", Method: rpcapi.RPCMethodServerBadgeDefPixaDownload}
	err := writeRPCDownload(
		context.Background(),
		stream,
		req,
		rpcapi.BadgeDefPixaDownloadResponse{Id: "badge"},
		(*rpcapi.RPCPayload).FromBadgeDefPixaDownloadResponse,
		bytes.NewReader([]byte("payload")),
	)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("writeRPCDownload() error = %v, want EOF", err)
	}
}

type failAfterWritesConn struct {
	remaining int
	err       error
}

func (c *failAfterWritesConn) Read([]byte) (int, error) { return 0, io.EOF }

func (c *failAfterWritesConn) Write(body []byte) (int, error) {
	if c.remaining == 0 {
		return 0, c.err
	}
	c.remaining--
	return len(body), nil
}

func (*failAfterWritesConn) Close() error                     { return nil }
func (*failAfterWritesConn) LocalAddr() net.Addr              { return nil }
func (*failAfterWritesConn) RemoteAddr() net.Addr             { return nil }
func (*failAfterWritesConn) SetDeadline(time.Time) error      { return nil }
func (*failAfterWritesConn) SetReadDeadline(time.Time) error  { return nil }
func (*failAfterWritesConn) SetWriteDeadline(time.Time) error { return nil }

type fakeGameplayPixaDownloadService struct {
	petPixaMetadata rpcapi.PetPixaDownloadResponse
	petPixaPayload  []byte
	petPixaRequest  rpcapi.PetPixaDownloadRequest
}

func (f *fakeGameplayPixaDownloadService) PreparePetPixaDownload(_ context.Context, request rpcapi.PetPixaDownloadRequest) (rpcapi.PetPixaDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	f.petPixaRequest = request
	return f.petPixaMetadata, io.NopCloser(bytes.NewReader(f.petPixaPayload)), nil, nil
}

func (f *fakeGameplayPixaDownloadService) PrepareBadgeDefPixaDownload(context.Context, rpcapi.BadgeDefPixaDownloadRequest) (rpcapi.BadgeDefPixaDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	return rpcapi.BadgeDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "not found"}, nil
}

func (f *fakeGameplayPixaDownloadService) Dispatch(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	return nil, false, nil
}
