package peerresource

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type peerFirmwareBindingFunc func(context.Context, giznet.PublicKey) (apitypes.Peer, error)

func (f peerFirmwareBindingFunc) LoadPeer(ctx context.Context, publicKey giznet.PublicKey) (apitypes.Peer, error) {
	return f(ctx, publicKey)
}

type firmwarePeerServiceFuncs struct {
	get     func(context.Context, adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error)
	prepare func(context.Context, string, string, string) (apitypes.FirmwareArtifact, apitypes.FirmwareArtifactEntry, io.ReadCloser, error)
}

func (f firmwarePeerServiceFuncs) GetFirmware(ctx context.Context, request adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
	return f.get(ctx, request)
}

func (f firmwarePeerServiceFuncs) PrepareArtifactEntryDownload(ctx context.Context, firmwareID, channel, path string) (apitypes.FirmwareArtifact, apitypes.FirmwareArtifactEntry, io.ReadCloser, error) {
	return f.prepare(ctx, firmwareID, channel, path)
}

func TestFirmwareGetResolvesCallerBinding(t *testing.T) {
	caller := giznet.PublicKey{1}
	firmwareID := "h106"
	server := &Server{
		Caller: caller,
		Peers: peerFirmwareBindingFunc(func(_ context.Context, got giznet.PublicKey) (apitypes.Peer, error) {
			if got != caller {
				t.Fatalf("LoadPeer() public key = %s, want %s", got.String(), caller.String())
			}
			return apitypes.Peer{FirmwareId: &firmwareID}, nil
		}),
		Firmwares: firmwarePeerServiceFuncs{
			get: func(_ context.Context, request adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
				if request.Name != firmwareID {
					t.Fatalf("GetFirmware() name = %q, want %q", request.Name, firmwareID)
				}
				return adminhttp.GetFirmware200JSONResponse(apitypes.Firmware{
					Name: firmwareID, CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0), Slots: apitypes.FirmwareSlots{},
				}), nil
			},
			prepare: unexpectedFirmwareDownload(t),
		},
	}

	request := firmwareRPCRequest(t, "firmware-get", rpcapi.FirmwareGetRequest{}, (*rpcapi.RPCPayload).FromFirmwareGetRequest)
	response := server.handleFirmwareGet(context.Background(), request)
	if response.Error != nil {
		t.Fatalf("handleFirmwareGet() error = %#v", response.Error)
	}
	got, err := response.Result.AsFirmwareGetResponse()
	if err != nil || got.Name != firmwareID {
		t.Fatalf("firmware get response = (%#v, %v)", got, err)
	}
}

func TestFirmwareGetRejectsUnboundCaller(t *testing.T) {
	server := &Server{
		Caller: giznet.PublicKey{2},
		Peers: peerFirmwareBindingFunc(func(context.Context, giznet.PublicKey) (apitypes.Peer, error) {
			return apitypes.Peer{}, nil
		}),
	}
	request := firmwareRPCRequest(t, "firmware-get", rpcapi.FirmwareGetRequest{}, (*rpcapi.RPCPayload).FromFirmwareGetRequest)
	response := server.handleFirmwareGet(context.Background(), request)
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeNotFound || response.Error.Message != errFirmwareNotBound.Error() {
		t.Fatalf("handleFirmwareGet() = %#v, want unbound not found", response)
	}
}

func TestFirmwareDownloadResolvesCallerBinding(t *testing.T) {
	firmwareID := "h106"
	payload := "firmware-payload"
	server := &Server{
		Caller: giznet.PublicKey{3},
		Peers: peerFirmwareBindingFunc(func(context.Context, giznet.PublicKey) (apitypes.Peer, error) {
			return apitypes.Peer{FirmwareId: &firmwareID}, nil
		}),
		Firmwares: firmwarePeerServiceFuncs{
			get: func(context.Context, adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
				t.Fatal("PrepareFirmwareDownload() unexpectedly called GetFirmware")
				return nil, nil
			},
			prepare: func(_ context.Context, gotFirmwareID, channel, path string) (apitypes.FirmwareArtifact, apitypes.FirmwareArtifactEntry, io.ReadCloser, error) {
				if gotFirmwareID != firmwareID || channel != "beta" || path != "firmware/main.bin" {
					t.Fatalf("PrepareArtifactEntryDownload() = (%q, %q, %q)", gotFirmwareID, channel, path)
				}
				return apitypes.FirmwareArtifact{ContentType: "application/x-tar", Size: int64(len(payload))},
					apitypes.FirmwareArtifactEntry{Path: path, Size: int64(len(payload)), Type: apitypes.FirmwareArtifactEntryTypeFile},
					io.NopCloser(strings.NewReader(payload)), nil
			},
		},
	}

	metadata, reader, rpcErr, err := server.PrepareFirmwareDownload(context.Background(), rpcapi.FirmwareFilesDownloadRequest{
		Channel: rpcapi.FirmwareChannelNameBeta,
		Path:    "firmware/main.bin",
	})
	if err != nil || rpcErr != nil {
		t.Fatalf("PrepareFirmwareDownload() = (error=%v, rpcError=%#v)", err, rpcErr)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if metadata.FirmwareId != firmwareID || metadata.Channel != rpcapi.FirmwareChannelNameBeta || metadata.Path != "firmware/main.bin" || string(data) != payload {
		t.Fatalf("PrepareFirmwareDownload() metadata = %#v, payload = %q", metadata, data)
	}
}

func firmwareRPCRequest[T any](t *testing.T, id string, value T, encode func(*rpcapi.RPCPayload, T) error) *rpcapi.RPCRequest {
	t.Helper()
	var params rpcapi.RPCPayload
	if err := encode(&params, value); err != nil {
		t.Fatalf("encode firmware request: %v", err)
	}
	return &rpcapi.RPCRequest{V: rpcapi.RPCVersionV1, Id: id, Params: &params}
}

func unexpectedFirmwareDownload(t *testing.T) func(context.Context, string, string, string) (apitypes.FirmwareArtifact, apitypes.FirmwareArtifactEntry, io.ReadCloser, error) {
	t.Helper()
	return func(context.Context, string, string, string) (apitypes.FirmwareArtifact, apitypes.FirmwareArtifactEntry, io.ReadCloser, error) {
		t.Fatal("unexpected firmware download")
		return apitypes.FirmwareArtifact{}, apitypes.FirmwareArtifactEntry{}, nil, nil
	}
}
