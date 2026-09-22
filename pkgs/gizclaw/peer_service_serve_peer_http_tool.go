package gizclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/proto"
)

type clientToolHTTPResponse struct {
	status int
	body   any
}

func (r clientToolHTTPResponse) VisitInvokeClientToolResponse(w *fiber.Ctx) error {
	return w.Status(r.status).JSON(r.body)
}

func (r clientToolHTTPResponse) VisitListClientToolsResponse(w *fiber.Ctx) error {
	return w.Status(r.status).JSON(r.body)
}

func clientToolFailure(e *deviceControlError) clientToolHTTPResponse {
	return clientToolHTTPResponse{status: e.Status, body: e.response()}
}

func (s *peerHTTP) ListClientTools(ctx context.Context, _ peerhttp.ListClientToolsRequestObject) (peerhttp.ListClientToolsResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return clientToolHTTPResponse{http.StatusUnauthorized, unauthorizedPublicHTTP()}, nil
	}
	result, failure := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientToolV0ListResponse, error) {
		params, err := newRPCRequestParams(new(rpcpb.ClientToolV0ListRequest), (*rpcapi.RPCPayload).FromClientToolV0ListRequest)
		if err != nil {
			return nil, err
		}
		response, err := callRPCResult(ctx, conn, newRPCRequest("client.tool.v0.list", rpcapi.RPCMethodClientToolV0List, params), rpcapi.RPCPayload.AsClientToolV0ListResponse)
		if err != nil {
			return nil, err
		}
		return *response, nil
	}, nil)
	if failure != nil {
		return clientToolFailure(failure), nil
	}
	response := peerhttp.ClientToolV0ListResponse{Tools: []peerhttp.ClientToolV0ListResponseTools{}}
	seen := make(map[rpcpb.ClientTool]bool)
	for _, tool := range result.GetTools() {
		meta, err := rpcapi.ClientToolMetadata(tool)
		if err != nil || seen[tool] {
			continue
		}
		seen[tool] = true
		response.Tools = append(response.Tools, peerhttp.ClientToolV0ListResponseTools(meta.Name))
	}
	return peerhttp.ListClientTools200JSONResponse(response), nil
}

func (s *peerHTTP) InvokeClientTool(ctx context.Context, request peerhttp.InvokeClientToolRequestObject) (peerhttp.InvokeClientToolResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return clientToolHTTPResponse{http.StatusUnauthorized, unauthorizedPublicHTTP()}, nil
	}
	if request.Body == nil {
		return clientToolFailure(invalidDeviceRequest("request body is required")), nil
	}
	data, err := json.Marshal(request.Body)
	if err != nil || apitypes.ValidateClientToolJSON(data) != nil {
		return clientToolFailure(invalidDeviceRequest("invalid tool or arguments")), nil
	}
	var body struct {
		Tool string         `json:"tool"`
		Args map[string]any `json:"args"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&body); err != nil {
		return clientToolFailure(invalidDeviceRequest("invalid tool arguments")), nil
	}
	tool, err := rpcapi.ClientToolByName(body.Tool)
	if err != nil {
		return clientToolFailure(invalidDeviceRequest("unknown tool")), nil
	}
	if tool == rpcpb.ClientTool_CLIENT_TOOL_RUN_WORKSPACE_SET {
		if failure := s.resolveToolWorkspace(ctx, owner, body.Args); failure != nil {
			return clientToolFailure(failure), nil
		}
	}
	message, err := rpcapi.ClientToolRequestMessage(tool, body.Args)
	if err != nil {
		return clientToolFailure(invalidDeviceRequest("invalid tool arguments")), nil
	}
	opts, failure := validateClientToolRequest(message)
	if failure != nil {
		return clientToolFailure(failure), nil
	}
	payload, err := proto.Marshal(message)
	if err != nil {
		return clientToolFailure(internalDeviceControlError()), nil
	}
	var responseMessage proto.Message
	_, failure = callDeviceControl(ctx, s.DeviceControl, owner, opts, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientToolV0InvokeResponse, error) {
		params, err := newRPCRequestParams(&rpcpb.ClientToolV0InvokeRequest{Tool: tool, Payload: payload}, (*rpcapi.RPCPayload).FromClientToolV0InvokeRequest)
		if err != nil {
			return nil, err
		}
		result, err := callRPCResult(ctx, conn, newRPCRequest("client.tool.v0.invoke", rpcapi.RPCMethodClientToolV0Invoke, params), rpcapi.RPCPayload.AsClientToolV0InvokeResponse)
		if err != nil {
			return nil, err
		}
		responseMessage, err = rpcapi.ClientToolResponseMessage(tool, (*result).GetPayload())
		if err != nil {
			return nil, err
		}
		if err := validateClientToolResponse(responseMessage); err != nil {
			return nil, err
		}
		return *result, nil
	}, func(ctx context.Context, _ *rpcpb.ClientToolV0InvokeResponse) error {
		switch result := responseMessage.(type) {
		case interface {
			GetValue() *rpcpb.AudioPlayerStatus
		}:
			status := result.GetValue()
			if status.ObservedAtUnixMs == 0 {
				status.ObservedAtUnixMs = s.DeviceControl.clock().UnixMilli()
			}
			return s.DeviceControl.applyAudioPlayer(ctx, owner, status)
		case *rpcpb.ClientDeviceStatusGetResponse:
			status, err := convertRPCType[rpcapi.PeerStatus](result.GetValue())
			if err != nil {
				return err
			}
			_, err = s.DeviceControl.applyReportedStatus(ctx, owner, status)
			return err
		}
		return nil
	})
	if failure != nil {
		return clientToolFailure(failure), nil
	}
	value, err := rpcapi.ClientToolResultJSON(responseMessage)
	if err != nil {
		return clientToolFailure(&deviceControlError{http.StatusBadGateway, deviceErrorCode, "device returned an invalid result"}), nil
	}
	return peerhttp.InvokeClientTool200JSONResponse{Result: value}, nil
}

func (s *peerHTTP) resolveToolWorkspace(ctx context.Context, owner giznet.PublicKey, args map[string]any) *deviceControlError {
	name, _ := args["workspace_name"].(string)
	collection, _ := args["collection"].(string)
	workflow, _ := args["workflow_name"].(string)
	if (name == "" && (collection == "" || workflow == "")) || (name != "" && (collection != "" || workflow != "")) || len(name) > 256 || len(collection) > 256 || len(workflow) > 256 {
		return invalidDeviceRequest("set exactly one of workspace_name, or collection with workflow_name")
	}
	reads, ok := s.deviceReads(owner)
	if !ok {
		return internalDeviceControlError()
	}
	resolved, err := reads.ResolveRunWorkspace(ctx, name, collection, workflow)
	if errors.Is(err, peerresource.ErrDeviceWorkspaceNotFound) {
		return &deviceControlError{http.StatusNotFound, "WORKSPACE_NOT_FOUND", "no available Workspace matches the target"}
	}
	if err != nil {
		return internalDeviceControlError()
	}
	delete(args, "collection")
	delete(args, "workflow_name")
	args["workspace_name"] = resolved
	return nil
}

func validateClientToolRequest(message proto.Message) (deviceControlOptions, *deviceControlError) {
	opts := deviceControlOptions{}
	invalid := func() (deviceControlOptions, *deviceControlError) {
		return opts, invalidDeviceRequest("invalid tool arguments")
	}
	switch request := message.(type) {
	case *rpcpb.ClientDeviceSoundPlayRequest:
		if failure := validateDeviceString("sound", request.Sound, maxDeviceSoundBytes); failure != nil {
			return opts, failure
		}
		if request.GetDurationMs() < 0 {
			return invalid()
		}
	case *rpcpb.ClientDeviceFindRequest:
		if request.GetDurationMs() < 0 {
			return invalid()
		}
	case *rpcpb.ClientDeviceRebootRequest:
		if request.GetDelayMs() < 0 {
			return invalid()
		}
		opts.markTransition = true
	case *rpcpb.ClientDeviceFactoryResetRequest:
		opts.markTransition = true
	case *rpcpb.ClientFirmwareUpdateRequest:
		if request.Sha256 != nil && !firmwareSha256Pattern.MatchString(*request.Sha256) {
			return invalid()
		}
		opts.markTransition = true
	case *rpcpb.ClientWifiScanRequest:
		opts.timeout = deviceWifiScanTimeout
		if request.TimeoutMs != nil {
			opts.timeout = wifiScanTimeout(*request.TimeoutMs)
		}
		request.TimeoutMs = new(opts.timeout.Milliseconds())
	case *rpcpb.ClientWifiConnectRequest:
		if failure := validateDeviceString("ssid", request.Ssid, maxDeviceSSIDBytes); failure != nil {
			return opts, failure
		}
		if request.Passphrase != nil && (len(*request.Passphrase) < minWifiPassphraseBytes || len(*request.Passphrase) > maxWifiPassphraseBytes) {
			return invalid()
		}
		opts.markTransition = true
	case *rpcpb.ClientWifiSavedForgetRequest:
		if failure := validateDeviceString("ssid", request.Ssid, maxDeviceSSIDBytes); failure != nil {
			return opts, failure
		}
		opts.notFoundCode = wifiNetworkNotFoundKey
	case *rpcpb.ClientSocialPingRequest:
		var key giznet.PublicKey
		if err := key.UnmarshalText([]byte(request.FromPeerPublicKey)); err != nil {
			return invalid()
		}
		if len(request.GetFromDisplayName()) > 256 || len(request.GetFriendGroupName()) > 255 {
			return invalid()
		}
	default:
		if strings.HasPrefix(string(message.ProtoReflect().Descriptor().Name()), "ClientDeviceAudioPlayer") {
			if err := rpcapi.ValidateAudioPlayerRequest(message); err != nil {
				return invalid()
			}
		}
	}
	return opts, nil
}

func validateClientToolResponse(message proto.Message) error {
	if strings.HasPrefix(string(message.ProtoReflect().Descriptor().Name()), "ClientDeviceAudioPlayer") {
		return rpcapi.ValidateAudioPlayerResponse(message)
	}
	if response, ok := message.(*rpcpb.ClientWifiScanResponse); ok {
		if len(response.Networks) > maxWifiScanResults {
			return fmt.Errorf("invalid Wi-Fi scan response")
		}
		for _, network := range response.Networks {
			if network == nil || !validDeviceScanString(&network.Ssid, maxDeviceSSIDBytes, true) || !validDeviceScanString(network.Bssid, maxWifiScanBSSIDBytes, false) || !validDeviceScanString(network.Security, maxWifiSecurityBytes, false) {
				return fmt.Errorf("invalid Wi-Fi scan response")
			}
		}
	}
	return nil
}
