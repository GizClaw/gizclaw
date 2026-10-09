package giztestcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type inboundCounter struct {
	atomic.Int64
	requestsMu     sync.Mutex
	recordRequests atomic.Bool
	requests       []any
	mutations      *inboundMutationLog
}

// One client shares this ordered log across MHS and predefined procedures.
// Entries are immutable decoded receipts, bounded independently of counters.
type inboundMutationLog struct {
	mu       sync.Mutex
	requests []any
}

func (l *inboundMutationLog) append(request any) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.requests) < 1024 {
		l.requests = append(l.requests, request)
	}
}

func (l *inboundMutationLog) snapshot() []any {
	if l == nil {
		return []any{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]any{}, l.requests...)
}

func isMutatingDeviceRequest(method rpcapi.RPCMethod, tool rpcpb.ClientTool) bool {
	if method == rpcapi.RPCMethodClientMhsV0Write {
		return true
	}
	if method != rpcapi.RPCMethodClientToolV0Invoke {
		return false
	}
	switch tool {
	case rpcpb.ClientTool_CLIENT_TOOL_INFO_GET, rpcpb.ClientTool_CLIENT_TOOL_IDENTIFIERS_GET,
		rpcpb.ClientTool_CLIENT_TOOL_DEVICE_STATUS_GET, rpcpb.ClientTool_CLIENT_TOOL_WIFI_SCAN,
		rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_LIST, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_GET,
		rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_GET, rpcpb.ClientTool_CLIENT_TOOL_LUA_APP_LIST:
		return false
	default:
		return true
	}
}

func (c *inboundCounter) observeRequest(method rpcapi.RPCMethod, tool rpcpb.ClientTool, request proto.Message) {
	if c == nil || !c.recordRequests.Load() {
		return
	}
	value := map[string]any{"method": string(method), "received_at_unix_ms": time.Now().UnixMilli()}
	var message proto.Message = request
	switch request := request.(type) {
	case *rpcpb.ClientMhsV0ReadRequest:
		meta, err := rpcapi.ClientHwdMetadata(request.Hwd)
		if err != nil {
			return
		}
		value["id"], value["hwd"], value["hwd_enum"] = request.Id, meta.Name, int32(request.Hwd)
		message = nil
	case *rpcpb.ClientMhsV0WriteRequest:
		meta, err := rpcapi.ClientHwdMetadata(request.Hwd)
		if err != nil {
			return
		}
		value["id"], value["hwd"], value["hwd_enum"] = request.Id, meta.Name, int32(request.Hwd)
		message, err = rpcapi.ClientHwdWriteRequestFromBytes(request.Hwd, request.Payload)
		if err != nil {
			return
		}
	}
	if play, ok := request.(*rpcpb.ClientDeviceAudioPlayerPlayRequest); ok {
		value["effective_index"] = play.GetIndex()
	}
	if tool != 0 {
		meta, err := rpcapi.ClientToolMetadata(tool)
		if err != nil {
			return
		}
		value["tool"], value["tool_enum"] = meta.Name, int32(tool)
	}
	if message != nil {
		data, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(message)
		if err != nil {
			return
		}
		var args any
		if err := json.Unmarshal(data, &args); err != nil {
			return
		}
		if _, sensitive := request.(*rpcpb.ClientWifiConnectRequest); sensitive {
			if fields, ok := args.(map[string]any); ok {
				if _, exists := fields["passphrase"]; exists {
					fields["passphrase"] = "[redacted]"
				}
			}
		}
		if _, workspace := request.(*rpcpb.ClientRunWorkspaceSetRequest); workspace {
			if fields, ok := args.(map[string]any); ok {
				if _, exists := fields["kickoff"]; !exists {
					fields["kickoff"] = false
				}
			}
		}
		value["args"] = args
	}
	if isMutatingDeviceRequest(method, tool) {
		c.mutations.append(value)
	}
	c.requestsMu.Lock()
	defer c.requestsMu.Unlock()
	if len(c.requests) < 1024 {
		c.requests = append(c.requests, value)
	}
}

func (c *inboundCounter) requestSnapshot() []any {
	c.requestsMu.Lock()
	defer c.requestsMu.Unlock()
	return append([]any{}, c.requests...)
}

func configureClientRPC(client *gizcli.Client, clientName string, steps []giztest.Step, vars *giztest.Variables, counts map[string]*inboundCounter) error {
	localCounts := map[string]*inboundCounter{}
	var mutations *inboundMutationLog
	for _, step := range steps {
		if step.Client != clientName || step.ClientRPC == nil {
			continue
		}
		key := clientName + ":" + step.ClientRPC.Key()
		counter := counts[key]
		if counter == nil {
			counter = &inboundCounter{}
			counts[key] = counter
		}
		localCounts[key] = counter
		if counter.mutations != nil {
			mutations = counter.mutations
		}
	}
	if mutations == nil {
		mutations = &inboundMutationLog{}
	}
	for _, counter := range localCounts {
		if counter.mutations == nil {
			counter.mutations = mutations
		}
	}
	if err := client.ObserveClientRPC(func(method rpcapi.RPCMethod) {
		if counter := localCounts[clientName+":"+string(method)]; counter != nil {
			counter.Add(1)
		}
	}); err != nil {
		return err
	}
	client.ObserveClientTool(func(tool rpcpb.ClientTool) {
		metadata, err := rpcapi.ClientToolMetadata(tool)
		if err == nil {
			if counter := localCounts[clientName+":client.tool.v0.invoke:"+metadata.Name]; counter != nil {
				counter.Add(1)
			}
		}
	})
	client.ObserveDeviceRequest(func(method rpcapi.RPCMethod, tool rpcpb.ClientTool, request proto.Message) {
		key := clientName + ":" + string(method)
		if tool != 0 {
			if meta, err := rpcapi.ClientToolMetadata(tool); err == nil {
				key += ":" + meta.Name
			}
		}
		localCounts[key].observeRequest(method, tool, request)
	})
	var device gizcli.DeviceControlHandlers
	var audio *audioFixture
	var luaApps *luaAppFixture
	var luaConfig []byte
	haveDevice := false
	for _, step := range steps {
		if step.Client != clientName || step.ClientRPC == nil {
			continue
		}
		operation := step.ClientRPC
		key := clientName + ":" + operation.Key()
		if counts[key] == nil {
			counts[key] = &inboundCounter{}
		}
		for pointer := range step.Expect {
			if strings.HasPrefix(pointer, "/requests") {
				counts[key].recordRequests.Store(true)
			}
		}
		if operation.ObserveOnly {
			continue
		}
		response, err := vars.Resolve(operation.Response)
		if err != nil && operation.Response != nil {
			return fmt.Errorf("step %s client_rpc response: %w", step.ID, err)
		}
		if operation.Method != "client.tool.v0.invoke" && operation.Tool != "" {
			return fmt.Errorf("step %s: tool requires client.tool.v0.invoke", step.ID)
		}
		switch operation.Method {
		case "client.rpc.methods.list", "client.tool.v0.list":
			if operation.Response != nil {
				return fmt.Errorf("step %s: %s is answered from the installed providers and takes no response", step.ID, operation.Method)
			}
		case "client.mhs.v0.read", "client.mhs.v0.write":
			if err := installMhs(&device, operation.Method, response); err != nil {
				return fmt.Errorf("step %s response: %w", step.ID, err)
			}
			haveDevice = true
		case "client.tool.v0.invoke":
			if _, err := rpcapi.ClientToolByName(operation.Tool); err != nil {
				return fmt.Errorf("step %s: %w", step.ID, err)
			}
			if object, ok := response.(map[string]any); ok {
				if unavailable, _ := object["unavailable"].(bool); unavailable {
					if _, hasResult := object["result"]; hasResult {
						return fmt.Errorf("step %s unavailable tool response cannot set result", step.ID)
					}
					continue
				}
			}
			if strings.HasPrefix(operation.Tool, "lua.app.") {
				tool, _ := rpcapi.ClientToolByName(operation.Tool)
				var handler gizcli.ClientToolHandler
				if object, ok := response.(map[string]any); ok && object["lua_apps"] != nil {
					config, err := json.Marshal(object["lua_apps"])
					if err != nil {
						return err
					}
					if luaApps == nil {
						luaConfig = config
						luaApps, err = newLuaAppFixture(object["lua_apps"])
						if err != nil {
							return fmt.Errorf("step %s: %w", step.ID, err)
						}
					} else if !bytes.Equal(config, luaConfig) {
						return fmt.Errorf("step %s: conflicting Lua app simulator configurations", step.ID)
					}
					handler = luaApps.invoke
				} else {
					scripted, scriptErr := deviceControlErrorResponse(response)
					if scriptErr != nil {
						return scriptErr
					}
					result, err := rpcapi.ClientToolResponseMessage(tool, nil)
					if err != nil {
						return err
					}
					if scripted == nil {
						data, err := json.Marshal(response)
						if response == nil {
							data = []byte(`{}`)
						}
						if err != nil {
							return err
						}
						if err := protojson.Unmarshal(data, result); err != nil {
							return err
						}
						if err := rpcapi.ValidateLuaAppResponse(result); err != nil {
							return err
						}
					}
					handler = func(context.Context, proto.Message) (proto.Message, error) { return proto.Clone(result), scripted }
				}
				if err := client.HandleClientTool(tool, handler); err != nil {
					return err
				}
				continue
			}
			if object, ok := response.(map[string]any); ok && object["run_workspace"] == true {
				tool, err := rpcapi.ClientToolByName(operation.Tool)
				if err != nil {
					return err
				}
				if err := client.HandleClientTool(tool, func(ctx context.Context, message proto.Message) (proto.Message, error) {
					request, ok := message.(*rpcpb.ClientRunWorkspaceSetRequest)
					if !ok {
						return nil, fmt.Errorf("wrong workspace procedure request")
					}
					_, err := client.SetServerRunWorkspace(ctx, "giztest-program-select", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: request.WorkspaceName})
					if err != nil {
						return nil, err
					}
					return &rpcpb.ClientRunWorkspaceSetResponse{}, nil
				}); err != nil {
					return err
				}
				continue
			}
			if object, ok := response.(map[string]any); ok && object["audio_player"] != nil {
				if audio == nil {
					audio, err = newAudioFixture(object["audio_player"])
					if err != nil {
						return err
					}
				}
				tool, err := rpcapi.ClientToolByName(operation.Tool)
				if err != nil {
					return err
				}
				if err := client.HandleClientTool(tool, audio.invoke); err != nil {
					return err
				}
				continue
			}
			switch operation.Tool {
			case "info.get":
				var info apitypes.DeviceInfo
				if response != nil {
					if err := decodeRequest(response, &info); err != nil {
						return fmt.Errorf("step %s response: %w", step.ID, err)
					}
				}
				client.Device = info
			case "identifiers.get":
				var identifiers apitypes.DeviceIdentifiers
				if response != nil {
					if err := decodeRequest(response, &identifiers); err != nil {
						return fmt.Errorf("step %s response: %w", step.ID, err)
					}
				}
				client.Device.Identifiers = &identifiers
			case "social.ping":
				scripted, err := deviceControlErrorResponse(response)
				if err != nil {
					return fmt.Errorf("step %s response: %w", step.ID, err)
				}
				if err := client.HandleSocialPing(func(context.Context, rpcapi.ClientSocialPingRequest) error { return scripted }); err != nil {
					return err
				}
			default:
				if err := installDeviceControl(&device, operation.Tool, response); err != nil {
					return fmt.Errorf("step %s response: %w", step.ID, err)
				}
				haveDevice = true
			}
		default:
			return fmt.Errorf("unsupported client RPC %q", operation.Method)
		}
	}
	if haveDevice {
		return client.HandleDeviceControl(device)
	}
	return nil
}

// deviceControlErrorResponse lets a script make a device provider answer a
// fixed RPC status: {error_code: 3} for INVALID_ARGUMENT or {error_code: 5}
// for NOT_FOUND.
// It returns the scripted error, or a nil error when the response scripts a
// value rather than a failure. A malformed error_code is returned separately so
// the document fails instead of installing a provider that answers with the
// validation message.
func deviceControlErrorResponse(response any) (error, error) {
	object, ok := response.(map[string]any)
	if !ok {
		return nil, nil
	}
	raw, ok := object["error_code"]
	if !ok {
		return nil, nil
	}
	code, err := scriptedErrorCode(raw)
	if err != nil {
		return nil, err
	}
	status := rpcapi.StatusCode(code)
	if !status.Valid() || status == rpcapi.StatusCodeOK {
		return nil, fmt.Errorf("error_code must be a canonical status code other than OK, got %d", code)
	}
	message, _ := object["error_message"].(string)
	return rpcapi.Error{Code: status, Message: message}, nil
}

// scriptedErrorCode reads one RPC status code from a decoded scenario value. A
// YAML document decodes a negative code as int and a non-negative one as
// uint64, and a JSON round trip decodes either as float64, so every integral
// form is accepted. A value that is not integral, or that does not fit the
// int32 wire field, is rejected rather than silently converted to a different
// code. The caller checks the decoded value against the canonical status set.
func scriptedErrorCode(raw any) (int32, error) {
	var value int64
	switch v := raw.(type) {
	case int:
		value = int64(v)
	case int32:
		return v, nil
	case int64:
		value = v
	case uint:
		if uint64(v) > math.MaxInt64 {
			return 0, fmt.Errorf("error_code must fit in int32, got %d", v)
		}
		value = int64(v)
	case uint32:
		value = int64(v)
	case uint64:
		if v > math.MaxInt64 {
			return 0, fmt.Errorf("error_code must fit in int32, got %d", v)
		}
		value = int64(v)
	case float64:
		if v != math.Trunc(v) {
			return 0, fmt.Errorf("error_code must be an integer, got %v", v)
		}
		if v < math.MinInt32 || v > math.MaxInt32 {
			return 0, fmt.Errorf("error_code must fit in int32, got %v", v)
		}
		return int32(v), nil
	default:
		return 0, fmt.Errorf("error_code must be an integer, got %T", raw)
	}
	if value < math.MinInt32 || value > math.MaxInt32 {
		return 0, fmt.Errorf("error_code must fit in int32, got %d", value)
	}
	return int32(value), nil
}

func installDeviceControl(handlers *gizcli.DeviceControlHandlers, method string, response any) error {
	if strings.HasPrefix(method, "client.mhs.v0.") {
		return installMhs(handlers, method, response)
	}
	if strings.HasPrefix(method, "audioplayer.") {
		return installAudioPlayer(handlers, method, response)
	}
	scripted, scriptErr := deviceControlErrorResponse(response)
	if scriptErr != nil {
		return scriptErr
	}
	if scripted != nil {
		fail := func(context.Context) error { return scripted }
		switch method {
		case "device.status.get":
			handlers.Status = func(ctx context.Context) (rpcapi.PeerStatus, error) { return rpcapi.PeerStatus{}, fail(ctx) }
		case "sound.play":
			handlers.PlaySound = func(ctx context.Context, _ string, _ *int64) error { return fail(ctx) }
		case "device.find":
			handlers.Find = func(ctx context.Context, _ *int64) error { return fail(ctx) }
		case "device.reboot":
			handlers.Reboot = func(ctx context.Context, _ *int64) error { return fail(ctx) }
		case "device.factory_reset":
			handlers.FactoryReset = func(ctx context.Context, _ bool) error { return fail(ctx) }
		case "run.workspace.set":
			handlers.SetRunWorkspace = func(ctx context.Context, _ rpcapi.ClientRunWorkspaceSetRequest) error { return fail(ctx) }
		case "wifi.saved.list":
			handlers.SavedWifi = func(ctx context.Context) ([]rpcapi.WifiSavedNetwork, error) { return nil, fail(ctx) }
		case "wifi.saved.forget":
			handlers.ForgetWifi = func(ctx context.Context, _ string) error { return fail(ctx) }
		case "wifi.scan":
			handlers.ScanWifi = func(ctx context.Context, _ *int64) ([]rpcapi.WifiScanResult, error) { return nil, fail(ctx) }
		case "firmware.update":
			handlers.UpdateFirmware = func(ctx context.Context, _ *rpcapi.FirmwareChannelName, _ *string) error { return fail(ctx) }
		case "wifi.connect":
			handlers.ConnectWifi = func(ctx context.Context, _ string, _ *string) error { return fail(ctx) }
		}
		return nil
	}
	switch method {
	case "device.status.get":
		var status rpcapi.PeerStatus
		if response != nil {
			if err := decodeRequest(response, &status); err != nil {
				return err
			}
		}
		handlers.Status = func(context.Context) (rpcapi.PeerStatus, error) { return status, nil }
	case "sound.play":
		handlers.PlaySound = func(context.Context, string, *int64) error { return nil }
	case "device.find":
		handlers.Find = func(context.Context, *int64) error { return nil }
	case "device.reboot":
		handlers.Reboot = func(context.Context, *int64) error { return nil }
	case "device.factory_reset":
		handlers.FactoryReset = func(context.Context, bool) error { return nil }
	case "run.workspace.set":
		handlers.SetRunWorkspace = func(context.Context, rpcapi.ClientRunWorkspaceSetRequest) error { return nil }
	case "wifi.saved.list":
		var list rpcapi.ClientWifiSavedListResponse
		if response != nil {
			if err := decodeRequest(response, &list); err != nil {
				return err
			}
		}
		handlers.SavedWifi = func(context.Context) ([]rpcapi.WifiSavedNetwork, error) { return list.Networks, nil }
	case "wifi.saved.forget":
		handlers.ForgetWifi = func(context.Context, string) error { return nil }
	case "wifi.scan":
		var result rpcapi.ClientWifiScanResponse
		delay, err := scriptedDeviceDelay(response)
		if err != nil {
			return err
		}
		if response != nil {
			if err := decodeRequest(response, &result); err != nil {
				return err
			}
		}
		handlers.ScanWifi = func(ctx context.Context, _ *int64) ([]rpcapi.WifiScanResult, error) {
			if delay > 0 {
				timer := time.NewTimer(delay)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return result.Networks, nil
		}
	case "firmware.update":
		handlers.UpdateFirmware = func(context.Context, *rpcapi.FirmwareChannelName, *string) error { return nil }
	case "wifi.connect":
		handlers.ConnectWifi = func(context.Context, string, *string) error { return nil }
	}
	return nil
}

// maxScriptedDelayMs bounds a scripted device delay across every runner. It is
// the largest value Node accepts for setTimeout; a larger one is clamped to a
// single millisecond there, which would turn a scenario written to exercise a
// timeout into one that passes on an immediate answer. The Go and Dart runners
// reject the same values so a document behaves identically everywhere.
const maxScriptedDelayMs = 2147483647

func scriptedDeviceDelay(response any) (time.Duration, error) {
	object, ok := response.(map[string]any)
	if !ok || object["delay_ms"] == nil {
		return 0, nil
	}
	value, err := scriptedErrorCode(object["delay_ms"])
	if err != nil || value < 0 {
		return 0, fmt.Errorf("delay_ms must be a non-negative integer")
	}
	if value > maxScriptedDelayMs {
		return 0, fmt.Errorf("delay_ms must be at most %d", maxScriptedDelayMs)
	}
	return time.Duration(value) * time.Millisecond, nil
}

// awaitInboundCalls blocks until the installed provider has been called at
// least want times, or ctx ends.
func awaitInboundCalls(ctx context.Context, counter *inboundCounter, want int64, method string) (int64, error) {
	calls := counter.Load()
	if calls >= want {
		return calls, nil
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if calls = counter.Load(); calls >= want {
				return calls, nil
			}
		case <-ctx.Done():
			return calls, fmt.Errorf("client RPC %s calls = %d, want at least %d: %w", method, calls, want, context.Cause(ctx))
		}
	}
}
