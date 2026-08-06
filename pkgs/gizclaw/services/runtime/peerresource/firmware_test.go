package peerresource

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

const firmwareTestSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type peerFirmwareBindingFunc func(context.Context, giznet.PublicKey) (apitypes.Peer, error)

func (f peerFirmwareBindingFunc) LoadPeer(ctx context.Context, publicKey giznet.PublicKey) (apitypes.Peer, error) {
	return f(ctx, publicKey)
}

type firmwarePeerServiceFunc func(context.Context, adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error)

func (f firmwarePeerServiceFunc) GetFirmware(ctx context.Context, request adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
	return f(ctx, request)
}

func TestFirmwareGetReturnsRequestedChannelConfiguration(t *testing.T) {
	caller := giznet.PublicKey{1}
	firmwareID := "firmware-01"
	description := "beta firmware"
	server := &Server{
		Caller: caller,
		Peers: peerFirmwareBindingFunc(func(_ context.Context, got giznet.PublicKey) (apitypes.Peer, error) {
			if got != caller {
				t.Fatalf("LoadPeer public key = %s, want %s", got.String(), caller.String())
			}
			return apitypes.Peer{FirmwareId: &firmwareID}, nil
		}),
		Firmwares: firmwarePeerServiceFunc(func(_ context.Context, request adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
			if request.Id != firmwareID {
				t.Fatalf("GetFirmware id = %q, want %q", request.Id, firmwareID)
			}
			return adminhttp.GetFirmware200JSONResponse(apitypes.Firmware{
				Id: firmwareID,
				Slots: apitypes.FirmwareSlots{
					Stable: apitypes.FirmwareSlot{Package: &apitypes.FirmwarePackage{Url: "https://firmware.example/stable.tar.zlib", Sha256: firmwareTestSHA256, Size: 10}},
					Beta:   apitypes.FirmwareSlot{Description: &description, Package: &apitypes.FirmwarePackage{Url: "https://firmware.example/beta.tar.zlib", Sha256: firmwareTestSHA256, Size: 20}},
				},
			}), nil
		}),
	}

	request := firmwareRPCRequest(t, "firmware-get", rpcapi.FirmwareGetRequest{Channel: rpcapi.FirmwareChannelNameBeta})
	response := server.handleFirmwareGet(context.Background(), request)
	if response.Error != nil {
		t.Fatalf("handleFirmwareGet error = %#v", response.Error)
	}
	got, err := response.Result.AsFirmwareGetResponse()
	if err != nil {
		t.Fatalf("AsFirmwareGetResponse: %v", err)
	}
	if got.Channel != rpcapi.FirmwareChannelNameBeta || got.Description == nil || *got.Description != description {
		t.Fatalf("response metadata = %#v", got)
	}
	if got.Url != "https://firmware.example/beta.tar.zlib" || got.Sha256 != firmwareTestSHA256 || got.Size != 20 {
		t.Fatalf("response package = %#v", got)
	}
}

func TestFirmwareGetRejectsInvalidChannel(t *testing.T) {
	server := &Server{}
	request := firmwareRPCRequest(t, "firmware-get", rpcapi.FirmwareGetRequest{})
	response := server.handleFirmwareGet(context.Background(), request)
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
		t.Fatalf("response = %#v, want invalid params", response)
	}
}

func TestFirmwareGetRejectsUnboundCaller(t *testing.T) {
	server := &Server{
		Caller: giznet.PublicKey{2},
		Peers: peerFirmwareBindingFunc(func(context.Context, giznet.PublicKey) (apitypes.Peer, error) {
			return apitypes.Peer{}, nil
		}),
	}
	request := firmwareRPCRequest(t, "firmware-get", rpcapi.FirmwareGetRequest{Channel: rpcapi.FirmwareChannelNameStable})
	response := server.handleFirmwareGet(context.Background(), request)
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeNotFound || response.Error.Message != errFirmwareNotBound.Error() {
		t.Fatalf("response = %#v, want unbound not found", response)
	}
}

func TestFirmwareGetRejectsEmptyChannelPackage(t *testing.T) {
	firmwareID := "firmware-01"
	server := &Server{
		Caller: giznet.PublicKey{3},
		Peers: peerFirmwareBindingFunc(func(context.Context, giznet.PublicKey) (apitypes.Peer, error) {
			return apitypes.Peer{FirmwareId: &firmwareID}, nil
		}),
		Firmwares: firmwarePeerServiceFunc(func(context.Context, adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
			return adminhttp.GetFirmware200JSONResponse(apitypes.Firmware{Id: firmwareID, Slots: apitypes.FirmwareSlots{}}), nil
		}),
	}
	request := firmwareRPCRequest(t, "firmware-get", rpcapi.FirmwareGetRequest{Channel: rpcapi.FirmwareChannelNamePending})
	response := server.handleFirmwareGet(context.Background(), request)
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeNotFound || response.Error.Message != errFirmwarePackageNotFound.Error() {
		t.Fatalf("response = %#v, want package not found", response)
	}
}

func TestFirmwareGetMapsLookupFailuresToStableErrors(t *testing.T) {
	firmwareID := "firmware-01"
	tests := []struct {
		name        string
		lookup      firmwarePeerServiceFunc
		wantCode    rpcapi.RPCErrorCode
		wantMessage string
	}{
		{
			name: "missing firmware",
			lookup: func(context.Context, adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
				return adminhttp.GetFirmware404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_NOT_FOUND", "internal id")), nil
			},
			wantCode:    rpcapi.RPCErrorCodeNotFound,
			wantMessage: "firmware not found",
		},
		{
			name: "admin service failure",
			lookup: func(context.Context, adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
				return adminhttp.GetFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "secret storage error")), nil
			},
			wantCode:    rpcapi.RPCErrorCodeInternalError,
			wantMessage: "firmware lookup unavailable",
		},
		{
			name: "transport failure",
			lookup: func(context.Context, adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
				return nil, errors.New("secret transport error")
			},
			wantCode:    rpcapi.RPCErrorCodeInternalError,
			wantMessage: "firmware lookup unavailable",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{
				Caller: giznet.PublicKey{4},
				Peers: peerFirmwareBindingFunc(func(context.Context, giznet.PublicKey) (apitypes.Peer, error) {
					return apitypes.Peer{FirmwareId: &firmwareID}, nil
				}),
				Firmwares: test.lookup,
			}
			request := firmwareRPCRequest(t, "firmware-get", rpcapi.FirmwareGetRequest{Channel: rpcapi.FirmwareChannelNameStable})
			response := server.handleFirmwareGet(context.Background(), request)
			if response.Error == nil || response.Error.Code != test.wantCode || response.Error.Message != test.wantMessage {
				t.Fatalf("response = %#v, want %v %q", response, test.wantCode, test.wantMessage)
			}
		})
	}
}

func firmwareRPCRequest(t *testing.T, id string, value rpcapi.FirmwareGetRequest) *rpcapi.RPCRequest {
	t.Helper()
	var params rpcapi.RPCPayload
	if err := params.FromFirmwareGetRequest(value); err != nil {
		t.Fatalf("encode firmware request: %v", err)
	}
	return &rpcapi.RPCRequest{V: rpcapi.RPCVersionV1, Id: id, Method: rpcapi.RPCMethodServerFirmwareGet, Params: &params}
}
