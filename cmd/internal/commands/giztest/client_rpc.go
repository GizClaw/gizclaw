package giztest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

type inboundCounter struct{ atomic.Int64 }

func configureClientRPC(client *gizcli.Client, clientName string, steps []Step, vars *variables, counts map[string]*inboundCounter) error {
	if err := client.ObserveClientRPC(func(method rpcapi.RPCMethod) {
		if counter := counts[clientName+":"+string(method)]; counter != nil {
			counter.Add(1)
		}
	}); err != nil {
		return err
	}
	var device gizcli.DeviceControlHandlers
	haveDevice := false
	for _, step := range steps {
		if step.Client != clientName || step.ClientRPC == nil {
			continue
		}
		response, err := vars.resolve(step.ClientRPC.Response)
		if err != nil && step.ClientRPC.Response != nil {
			return fmt.Errorf("step %s client_rpc response: %w", step.ID, err)
		}
		key := clientName + ":" + step.ClientRPC.Method
		counter := &inboundCounter{}
		counts[key] = counter
		switch step.ClientRPC.Method {
		case "client.info.get":
			var device apitypes.DeviceInfo
			if response != nil {
				if err := decodeRequest(response, &device); err != nil {
					return fmt.Errorf("step %s response: %w", step.ID, err)
				}
			}
			client.Device = device
		case "client.identifiers.get":
			var identifiers apitypes.DeviceIdentifiers
			if response != nil {
				if err := decodeRequest(response, &identifiers); err != nil {
					return fmt.Errorf("step %s response: %w", step.ID, err)
				}
			}
			client.Device.Identifiers = &identifiers
		case "client.tool.invoke":
			object, ok := response.(map[string]any)
			if !ok {
				return fmt.Errorf("step %s tool response must be an object", step.ID)
			}
			name, ok := object["name"].(string)
			if !ok || strings.TrimSpace(name) == "" {
				return fmt.Errorf("step %s tool response requires name", step.ID)
			}
			payload, err := json.Marshal(object["result"])
			if err != nil {
				return err
			}
			if err := client.HandleTool(name, func(context.Context, json.RawMessage) (json.RawMessage, error) {
				return append(json.RawMessage(nil), payload...), nil
			}); err != nil {
				return err
			}
		case "client.device.status.get", "client.device.volume.set", "client.device.sound.play", "client.device.reboot",
			"client.wifi.status.get", "client.wifi.saved.list", "client.wifi.saved.forget":
			if err := installDeviceControl(&device, step.ClientRPC.Method, response); err != nil {
				return fmt.Errorf("step %s response: %w", step.ID, err)
			}
			haveDevice = true
		default:
			return fmt.Errorf("unsupported client RPC %q", step.ClientRPC.Method)
		}
	}
	if haveDevice {
		return client.HandleDeviceControl(device)
	}
	return nil
}

// deviceControlErrorResponse lets a script make a device provider answer a
// fixed RPC error: {error_code: -32602} or {error_code: 404}.
func deviceControlErrorResponse(response any) error {
	object, ok := response.(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := object["error_code"]
	if !ok {
		return nil
	}
	var code float64
	switch v := raw.(type) {
	case float64:
		code = v
	case int:
		code = float64(v)
	case int64:
		code = float64(v)
	default:
		return fmt.Errorf("error_code must be an integer")
	}
	message, _ := object["error_message"].(string)
	return rpcapi.Error{Code: rpcapi.RPCErrorCode(int32(code)), Message: message}
}

func installDeviceControl(handlers *gizcli.DeviceControlHandlers, method string, response any) error {
	if scripted := deviceControlErrorResponse(response); scripted != nil {
		fail := func(context.Context) error { return scripted }
		switch method {
		case "client.device.status.get":
			handlers.Status = func(ctx context.Context) (rpcapi.PeerStatus, error) { return rpcapi.PeerStatus{}, fail(ctx) }
		case "client.device.volume.set":
			handlers.SetVolume = func(ctx context.Context, _ int64, _ bool) (rpcapi.PeerStatus, error) {
				return rpcapi.PeerStatus{}, fail(ctx)
			}
		case "client.device.sound.play":
			handlers.PlaySound = func(ctx context.Context, _ string, _ *int64) error { return fail(ctx) }
		case "client.device.reboot":
			handlers.Reboot = func(ctx context.Context, _ *int64) error { return fail(ctx) }
		case "client.wifi.status.get":
			handlers.WifiStatus = func(ctx context.Context) (rpcapi.WifiStatus, error) { return rpcapi.WifiStatus{}, fail(ctx) }
		case "client.wifi.saved.list":
			handlers.SavedWifi = func(ctx context.Context) ([]rpcapi.WifiSavedNetwork, error) { return nil, fail(ctx) }
		case "client.wifi.saved.forget":
			handlers.ForgetWifi = func(ctx context.Context, _ string) error { return fail(ctx) }
		}
		return nil
	}
	switch method {
	case "client.device.status.get":
		var status rpcapi.PeerStatus
		if response != nil {
			if err := decodeRequest(response, &status); err != nil {
				return err
			}
		}
		handlers.Status = func(context.Context) (rpcapi.PeerStatus, error) { return status, nil }
	case "client.device.volume.set":
		var status rpcapi.PeerStatus
		if response != nil {
			if err := decodeRequest(response, &status); err != nil {
				return err
			}
		}
		// The scripted status is echoed back with the requested level and
		// mute state so the round trip can be asserted through HTTP.
		handlers.SetVolume = func(_ context.Context, level int64, muted bool) (rpcapi.PeerStatus, error) {
			applied := status
			levelValue := int(level)
			applied.Volume = &levelValue
			applied.Muted = &muted
			return applied, nil
		}
	case "client.device.sound.play":
		handlers.PlaySound = func(context.Context, string, *int64) error { return nil }
	case "client.device.reboot":
		handlers.Reboot = func(context.Context, *int64) error { return nil }
	case "client.wifi.status.get":
		var status rpcapi.WifiStatus
		if response != nil {
			if err := decodeRequest(response, &status); err != nil {
				return err
			}
		}
		handlers.WifiStatus = func(context.Context) (rpcapi.WifiStatus, error) { return status, nil }
	case "client.wifi.saved.list":
		var list rpcapi.ClientWifiSavedListResponse
		if response != nil {
			if err := decodeRequest(response, &list); err != nil {
				return err
			}
		}
		handlers.SavedWifi = func(context.Context) ([]rpcapi.WifiSavedNetwork, error) { return list.Networks, nil }
	case "client.wifi.saved.forget":
		handlers.ForgetWifi = func(context.Context, string) error { return nil }
	}
	return nil
}
