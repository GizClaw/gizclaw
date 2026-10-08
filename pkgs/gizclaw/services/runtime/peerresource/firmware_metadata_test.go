package peerresource

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestFirmwareMetadataRPCUsesBoundFirmwareAndExactKey(t *testing.T) {
	metadata := apitypes.FirmwareMetadata{
		"modem":         json.RawMessage(`{"version":"vendor-2026.10","urls":["https://firmware.example/a","https://firmware.example/b"]}`),
		"modem.version": json.RawMessage(`"separate-key"`),
		"null":          nil,
		"boolean":       json.RawMessage(`true`),
		"number":        json.RawMessage(`9007199254740993`),
		"array":         json.RawMessage(`[1,"x",null]`),
	}
	server := &Server{
		Caller: giznet.PublicKey{1},
		Peers: peerFirmwareBindingFunc(func(_ context.Context, key giznet.PublicKey) (apitypes.Peer, error) {
			if key != (giznet.PublicKey{1}) {
				t.Fatal("lookup did not use authenticated caller")
			}
			return apitypes.Peer{FirmwareId: new("bound-firmware")}, nil
		}),
		Firmwares: firmwarePeerServiceFunc(func(_ context.Context, request adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
			if request.Id != "bound-firmware" {
				t.Fatal("lookup did not use bound Firmware")
			}
			return adminhttp.GetFirmware200JSONResponse(apitypes.Firmware{Id: request.Id, Metadata: &metadata}), nil
		}),
	}
	for _, key := range []string{"modem", "modem.version", "null", "boolean", "number", "array", "missing", "", "../modem"} {
		t.Run(key, func(t *testing.T) {
			var body rpcapi.RPCPayload
			if err := body.FromFirmwareMetadataGetRequest(&rpcpb.FirmwareMetadataGetRequest{Key: key}); err != nil {
				t.Fatal(err)
			}
			response, handled, err := server.Dispatch(t.Context(), &rpcapi.RPCRequest{V: rpcapi.RPCVersionV1, Id: "lookup", Method: rpcapi.RPCMethodServerFirmwareMetadataGet, Params: &body})
			if err != nil || !handled {
				t.Fatalf("Dispatch = %v, %v", handled, err)
			}
			if key == "missing" {
				if response.Error == nil || response.Error.Code != rpcapi.StatusCodeNotFound {
					t.Fatalf("missing key response = %#v", response)
				}
				return
			}
			if key == "" || key == "../modem" {
				if response.Error == nil || response.Error.Code != rpcapi.StatusCodeInvalidArgument {
					t.Fatalf("invalid key response = %#v", response)
				}
				return
			}
			if response.Error != nil {
				t.Fatal(response.Error)
			}
			got, err := response.Result.AsFirmwareMetadataGetResponse()
			expected, marshalErr := json.Marshal(metadata[key])
			if err != nil || marshalErr != nil || got.Key != key || got.Value != string(expected) {
				t.Fatalf("metadata response = %v, %v", got, err)
			}
		})
	}
}

func TestFirmwareMetadataRPCRequiresBindingAndMetadata(t *testing.T) {
	var body rpcapi.RPCPayload
	if err := body.FromFirmwareMetadataGetRequest(&rpcpb.FirmwareMetadataGetRequest{Key: "modem"}); err != nil {
		t.Fatal(err)
	}
	for _, bound := range []bool{false, true} {
		server := &Server{
			Peers: peerFirmwareBindingFunc(func(context.Context, giznet.PublicKey) (apitypes.Peer, error) {
				if bound {
					return apitypes.Peer{FirmwareId: new("bound")}, nil
				}
				return apitypes.Peer{}, nil
			}),
			Firmwares: firmwarePeerServiceFunc(func(context.Context, adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
				return adminhttp.GetFirmware200JSONResponse(apitypes.Firmware{}), nil
			}),
		}
		response := server.handleFirmwareMetadataGet(t.Context(), &rpcapi.RPCRequest{Id: "lookup", Params: &body})
		if response.Error == nil || response.Error.Code != rpcapi.StatusCodeNotFound {
			t.Fatalf("bound=%v response = %#v", bound, response)
		}
	}
}
