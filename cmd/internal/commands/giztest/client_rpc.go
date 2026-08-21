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
		default:
			return fmt.Errorf("unsupported client RPC %q", step.ClientRPC.Method)
		}
	}
	return nil
}
