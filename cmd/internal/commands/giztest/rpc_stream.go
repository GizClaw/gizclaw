package giztestcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

type operationResult struct {
	assertion any
	saved     any
	evidence  map[string]any
}
type boundedBuffer struct {
	bytes.Buffer
	max int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 {
		b.max = 16 << 20
	}
	if b.Len()+len(p) > b.max {
		return 0, fmt.Errorf("stream exceeds max_bytes %d", b.max)
	}
	return b.Buffer.Write(p)
}
func decodeRequest(input any, out any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}
	return nil
}
func jsonObject(input any) (map[string]any, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func invokeRPCStream(ctx context.Context, client *gizcli.Client, step giztest.Step, request any) (operationResult, error) {
	op := step.RPCStream
	if op == nil {
		return operationResult{}, fmt.Errorf("rpc_stream operation required")
	}
	limit := op.MaxBytes
	if limit == 0 {
		limit = 16 << 20
	}
	buf := &boundedBuffer{max: limit}
	var metadata any
	switch op.Method {
	case "all.speed_test.run":
		var req rpcapi.SpeedTestRequest
		if err := decodeRequest(request, &req); err != nil {
			return operationResult{}, err
		}
		result, err := client.SpeedTest(ctx, step.ID, req)
		if err != nil {
			return operationResult{}, err
		}
		return speedTestOperationResult(result), nil
	case "server.pet.pixa.download":
		var req rpcapi.PetPixaDownloadRequest
		if err := decodeRequest(request, &req); err != nil {
			return operationResult{}, err
		}
		result, err := client.DownloadPetPixa(ctx, step.ID, req, buf)
		if err != nil {
			return operationResult{}, err
		}
		metadata = result
	case "server.badge_def.pixa.download":
		var req rpcapi.BadgeDefPixaDownloadRequest
		if err := decodeRequest(request, &req); err != nil {
			return operationResult{}, err
		}
		result, err := client.DownloadBadgeDefPixa(ctx, step.ID, req, buf)
		if err != nil {
			return operationResult{}, err
		}
		metadata = result
	case "server.workspace.icon.download":
		if object, ok := request.(map[string]any); ok {
			request = cloneMap(object)
			switch request.(map[string]any)["format"] {
			case "ICON_FORMAT_PIXA":
				request.(map[string]any)["format"] = string(rpcapi.IconFormatPixa)
			case "ICON_FORMAT_PNG":
				request.(map[string]any)["format"] = string(rpcapi.IconFormatPng)
			}
		}
		var req rpcapi.WorkspaceIconDownloadRequest
		if err := decodeRequest(request, &req); err != nil {
			return operationResult{}, err
		}
		result, err := client.DownloadWorkspaceIcon(ctx, step.ID, req, buf)
		if err != nil {
			return operationResult{}, err
		}
		metadata = result
	case "server.workspace.history.audio.download":
		var req rpcapi.WorkspaceHistoryAudioDownloadRequest
		if err := decodeRequest(request, &req); err != nil {
			return operationResult{}, err
		}
		result, err := client.DownloadWorkspaceHistoryAudio(ctx, step.ID, req, buf)
		if err != nil {
			return operationResult{}, err
		}
		metadata = result
	case "server.friend_group.messages.audio.download":
		var req rpcapi.FriendGroupMessageAudioDownloadRequest
		if err := decodeRequest(request, &req); err != nil {
			return operationResult{}, err
		}
		result, err := client.DownloadFriendGroupMessageAudio(ctx, step.ID, req, buf)
		if err != nil {
			return operationResult{}, err
		}
		metadata = result
	default:
		return operationResult{}, fmt.Errorf("RPC method %s is not a supported binary stream", op.Method)
	}
	object, err := jsonObject(metadata)
	if err != nil {
		return operationResult{}, err
	}
	object["bytes"] = buf.Len()
	saved := any(object)
	if buf.Len() > 0 {
		saved = append([]byte(nil), buf.Bytes()...)
	}
	return operationResult{assertion: object, saved: saved, evidence: map[string]any{"method": op.Method, "bytes": buf.Len()}}, nil
}

func speedTestOperationResult(result gizcli.SpeedTestResult) operationResult {
	evidence := map[string]any{
		"method":              "all.speed_test.run",
		"bytes":               result.DownBytes,
		"up_content_length":   result.UpContentLength,
		"down_content_length": result.DownContentLength,
		"up_bytes":            result.UpBytes,
		"down_bytes":          result.DownBytes,
		"up_duration_ms":      result.UpDuration.Milliseconds(),
		"down_duration_ms":    result.DownDuration.Milliseconds(),
		"duration_ms":         result.Duration.Milliseconds(),
		"up_mbps":             result.UpMbps(),
		"down_mbps":           result.DownMbps(),
	}
	object := maps.Clone(evidence)
	delete(object, "method")
	object["UpContentLength"] = result.UpContentLength
	object["DownContentLength"] = result.DownContentLength
	object["UpBytes"] = result.UpBytes
	object["DownBytes"] = result.DownBytes
	object["UpDuration"] = int64(result.UpDuration)
	object["DownDuration"] = int64(result.DownDuration)
	object["Duration"] = int64(result.Duration)
	return operationResult{assertion: object, saved: object, evidence: evidence}
}

func cloneMap(input map[string]any) map[string]any {
	result := make(map[string]any, len(input))
	maps.Copy(result, input)
	return result
}
