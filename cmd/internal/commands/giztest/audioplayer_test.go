package giztestcmd

import (
	"context"
	"path/filepath"
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestAudioPlayerGiztestDocuments(t *testing.T) {
	paths, err := filepath.Glob("../../../../tests/gizclaw-e2e/giztest/server.device.audioplayer.*.giztest.yaml")
	if err != nil || len(paths) != 6 {
		t.Fatalf("scenarios=%v err=%v", paths, err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			doc, err := giztest.LoadDocument(path, newDriver(false, nil))
			if err != nil {
				t.Fatal(err)
			}
			var handlers gizcli.DeviceControlHandlers
			for _, step := range doc.Steps {
				if step.ClientRPC != nil {
					if err := installDeviceControl(&handlers, step.ClientRPC.Method, step.ClientRPC.Response); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func TestAudioPlayerScriptedResponseIsolation(t *testing.T) {
	var handlers gizcli.DeviceControlHandlers
	if err := installDeviceControl(&handlers, "client.device.audioplayer.play", map[string]any{"state": "playing", "repeat": "off", "playlist_length": 1, "current_index": 0}); err != nil {
		t.Fatal(err)
	}
	first, err := handlers.AudioPlayer.Play(context.Background(), &rpcpb.ClientDeviceAudioPlayerPlayRequest{Index: new(uint32(0))})
	if err != nil || first.Value.CurrentIndex == nil || *first.Value.CurrentIndex != 0 {
		t.Fatalf("response=%v err=%v", first, err)
	}
	first.Value.State = "error"
	second, err := handlers.AudioPlayer.Play(context.Background(), &rpcpb.ClientDeviceAudioPlayerPlayRequest{Index: new(uint32(0))})
	if err != nil || second.Value.State != "playing" {
		t.Fatalf("response=%v err=%v", second, err)
	}
	if err := installDeviceControl(&handlers, "client.device.audioplayer.stop", map[string]any{"error_code": 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := handlers.AudioPlayer.Stop(context.Background(), new(rpcpb.ClientDeviceAudioPlayerStopRequest)); err == nil {
		t.Fatal("scripted failure lost")
	}
}

func TestTelemetryFrameValidation(t *testing.T) {
	for _, input := range []any{map[string]any{}, map[string]any{"observations": []any{map[string]any{}}}, map[string]any{"observations": []any{map[string]any{"unknown": true}}}} {
		if _, err := decodeTelemetryFrame(input); err == nil {
			t.Fatalf("accepted malformed frame %v", input)
		}
	}
}
