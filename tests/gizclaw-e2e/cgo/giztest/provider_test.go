package main

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"google.golang.org/protobuf/proto"
)

// delay_ms is a runner instruction, not a payload field: it must be removed
// before the rest is encoded, and it must never silently become an immediate
// answer, which would let a timeout scenario pass without reaching that path.
func TestScriptedDelay(t *testing.T) {
	tests := []struct {
		name      string
		response  any
		wantValue any
		wantMs    int64
		wantErr   bool
	}{
		{name: "not an object", response: "text", wantValue: "text"},
		{name: "absent", response: map[string]any{"networks": []any{}}, wantValue: map[string]any{"networks": []any{}}},
		{name: "removed before encoding", response: map[string]any{"delay_ms": 1200, "networks": []any{}}, wantValue: map[string]any{"networks": []any{}}, wantMs: 1200},
		{name: "at the bound", response: map[string]any{"delay_ms": maxScriptedDelayMs}, wantValue: map[string]any{}, wantMs: maxScriptedDelayMs},
		{name: "above the bound", response: map[string]any{"delay_ms": int64(maxScriptedDelayMs) + 1}, wantErr: true},
		{name: "negative", response: map[string]any{"delay_ms": -1}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, delay, err := scriptedDelay(test.response)
			if test.wantErr {
				if err == nil {
					t.Fatalf("value=%#v delay=%v, want an error", value, delay)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if delay.Milliseconds() != test.wantMs {
				t.Fatalf("delay = %v, want %dms", delay, test.wantMs)
			}
			want, wantObject := test.wantValue.(map[string]any)
			got, gotObject := value.(map[string]any)
			if wantObject != gotObject {
				t.Fatalf("value = %#v, want %#v", value, test.wantValue)
			}
			if !wantObject {
				if value != test.wantValue {
					t.Fatalf("value = %#v, want %#v", value, test.wantValue)
				}
				return
			}
			if _, ok := got["delay_ms"]; ok {
				t.Fatalf("delay_ms survived into the encoded payload: %#v", got)
			}
			if !slices.Equal(slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want))) {
				t.Fatalf("value keys = %#v, want %#v", got, want)
			}
		})
	}
}

// Every shared playback fixture must reach the C controller route table and
// encode its scripted device response with the registered protobuf descriptor.
func TestAudioPlayerDocuments(t *testing.T) {
	paths, err := filepath.Glob("../../giztest/server.device.audioplayer.*.giztest.yaml")
	if err != nil || len(paths) != 6 {
		t.Fatalf("paths=%v err=%v", paths, err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, ".telemetry.giztest.yaml") {
			continue
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			doc, err := giztest.LoadDocument(path, driver{})
			if err != nil {
				t.Fatal(err)
			}
			for _, step := range doc.Steps {
				if step.ClientRPC == nil {
					continue
				}
				if object, ok := step.ClientRPC.Response.(map[string]any); ok {
					if _, scriptedError := object["error_code"]; scriptedError {
						if _, _, err := errorResponse(object); err != nil {
							t.Fatal(err)
						}
						continue
					}
				}
				info, err := lookupMethod(step.ClientRPC.Method)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := encodePayload(info.response, step.ClientRPC.Response); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// The friend and friend group HTTP fixtures must pass the C runner's
// validate, so every route they use is one the controller SDK dispatches.
func TestFriendAndFriendGroupHTTPDocuments(t *testing.T) {
	for _, name := range []string{
		"server.friends.http.giztest.yaml",
		"server.friend_groups.http.giztest.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			doc, err := giztest.LoadDocument(filepath.Join("../../giztest", name), driver{})
			if err != nil {
				t.Fatal(err)
			}
			routes := 0
			for _, step := range doc.Steps {
				if step.HTTP == nil {
					continue
				}
				routes++
				if err := validateControlRoute(step); err != nil {
					t.Fatalf("step %s: %v", step.ID, err)
				}
			}
			if routes == 0 {
				t.Fatal("document has no http steps")
			}
		})
	}
}

// The find and social ping fixtures must pass the C runner's validate, reach
// the controller route table, and have their client_rpc providers answer the
// way each document scripts: an ack by default, the scripted status otherwise.
func TestFindAndSocialPingDocuments(t *testing.T) {
	for _, name := range []string{
		"server.device.find.giztest.yaml",
		"server.device.find.unsupported.giztest.yaml",
		"server.friend.ping.giztest.yaml",
		"server.friend_group.ping.giztest.yaml",
		"server.profile.get.giztest.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			doc, err := giztest.LoadDocument(filepath.Join("../../giztest", name), driver{})
			if err != nil {
				t.Fatal(err)
			}
			for _, step := range doc.Steps {
				if step.ClientRPC == nil {
					continue
				}
				info, err := lookupMethod(step.ClientRPC.Method)
				if err != nil {
					t.Fatal(err)
				}
				provider := newClientRPCProvider()
				if err := provider.install(step.ClientRPC.Method, step.ClientRPC.Response); err != nil {
					t.Fatal(err)
				}
				payload, code, _, err := provider.answer(info.id, nil)
				if err != nil {
					t.Fatal(err)
				}
				wantCode := int32(0)
				if object, ok := step.ClientRPC.Response.(map[string]any); ok && object["error_code"] != nil {
					if wantCode, _, err = errorResponse(object); err != nil {
						t.Fatal(err)
					}
				}
				if code != wantCode || len(payload) != 0 {
					t.Fatalf("%s answered code=%d payload=%x, want code=%d and an empty ack",
						step.ClientRPC.Method, code, payload, wantCode)
				}
				if got := provider.callCount(step.ClientRPC.Method); got != 1 {
					t.Fatalf("%s call count = %d, want 1", step.ClientRPC.Method, got)
				}
			}
		})
	}
}

// A device that installs no provider must look unimplemented, which is what
// turns into not_online for a ping and 501 DEVICE_UNSUPPORTED for find.
func TestUninstalledClientMethodsAreUnimplemented(t *testing.T) {
	provider := newClientRPCProvider()
	for _, method := range []string{"client.device.find", "client.social.ping"} {
		info, err := lookupMethod(method)
		if err != nil {
			t.Fatal(err)
		}
		_, code, _, err := provider.answer(info.id, nil)
		if err != nil {
			t.Fatal(err)
		}
		if code != int32(rpcpb.StatusCode_STATUS_CODE_UNIMPLEMENTED) {
			t.Fatalf("%s code = %d, want UNIMPLEMENTED", method, code)
		}
	}
}

// Ping and profile responses render as protobuf JSON, like every other rpc
// step: proto field names, full enum value names, numeric int32 counts, and
// absent optional fields omitted rather than zero-filled.
func TestSocialResponsesRenderProtoJSON(t *testing.T) {
	ping, err := proto.Marshal(&rpcpb.FriendPingResponse{
		Result:            rpcpb.SocialPingResult_SOCIAL_PING_RESULT_RATE_LIMITED,
		RetryAfterSeconds: proto.Int32(42),
	})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodePayload("FriendPingResponse", ping)
	if err != nil {
		t.Fatal(err)
	}
	if decoded["result"] != "SOCIAL_PING_RESULT_RATE_LIMITED" ||
		decoded["delivered_count"] != float64(0) || decoded["retry_after_seconds"] != float64(42) {
		t.Fatalf("rate-limited ping = %#v", decoded)
	}
	delivered, err := proto.Marshal(&rpcpb.FriendGroupPingResponse{
		Result:         rpcpb.SocialPingResult_SOCIAL_PING_RESULT_DELIVERED,
		DeliveredCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err = decodePayload("FriendGroupPingResponse", delivered); err != nil {
		t.Fatal(err)
	}
	if _, present := decoded["retry_after_seconds"]; present ||
		decoded["result"] != "SOCIAL_PING_RESULT_DELIVERED" || decoded["delivered_count"] != float64(1) {
		t.Fatalf("delivered rally = %#v", decoded)
	}
	profiles, err := proto.Marshal(&rpcpb.ProfileGetResponse{Items: []*rpcpb.PublicProfile{
		{PeerPublicKey: "carol", DisplayName: new("Carol"), Emoji: new("🐱")},
		{PeerPublicKey: "unknown"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err = decodePayload("ProfileGetResponse", profiles); err != nil {
		t.Fatal(err)
	}
	items, _ := decoded["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("profile items = %#v", decoded)
	}
	carol, _ := items[0].(map[string]any)
	if carol["peer_public_key"] != "carol" || carol["display_name"] != "Carol" || carol["emoji"] != "🐱" {
		t.Fatalf("carol profile = %#v", carol)
	}
	unknown, _ := items[1].(map[string]any)
	if _, present := unknown["display_name"]; present {
		t.Fatalf("unknown profile = %#v", unknown)
	}
	if _, present := unknown["emoji"]; present {
		t.Fatalf("unknown profile = %#v", unknown)
	}
}

// The settings, factory reset, RPC methods, Workspace switch and Tool fixtures
// must pass the C runner's validate and reach the controller route table.
func TestDeviceSettingsAndControlDocuments(t *testing.T) {
	for _, name := range []string{
		"server.device.settings.giztest.yaml",
		"server.device.factory_reset.giztest.yaml",
		"server.device.rpc_methods.giztest.yaml",
		"server.device.run_workspace.set.giztest.yaml",
		"server.device.tools.giztest.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			doc, err := giztest.LoadDocument(filepath.Join("../../giztest", name), driver{})
			if err != nil {
				t.Fatal(err)
			}
			for _, step := range doc.Steps {
				if step.HTTP != nil {
					if err := validateControlRoute(step); err != nil {
						t.Fatalf("step %s: %v", step.ID, err)
					}
				}
			}
		})
	}
	for _, route := range []struct{ method, path string }{
		{"PATCH", "/gizclaw/v1/device/settings"},
		{"POST", "/gizclaw/v1/device/tools/lamp/actions/invoke"},
	} {
		step := giztest.Step{HTTP: &giztest.HTTPOperation{Method: route.method, Path: route.path}}
		if err := validateControlRoute(step); err != nil {
			t.Fatalf("%s %s: %v", route.method, route.path, err)
		}
	}
	for _, route := range []struct{ method, path string }{
		{"PATCH", "/gizclaw/v1/device/volume"},
		{"POST", "/gizclaw/v1/device/tools/lamp"},
	} {
		step := giztest.Step{HTTP: &giztest.HTTPOperation{Method: route.method, Path: route.path}}
		if err := validateControlRoute(step); err == nil {
			t.Fatalf("%s %s was accepted", route.method, route.path)
		}
	}
}

// client.device.settings.set answers the scripted settings with the patch's
// members overlaid; absent patch members leave the scripted ones unchanged.
func TestSettingsSetOverlaysPatch(t *testing.T) {
	provider := newClientRPCProvider()
	if err := provider.install("client.device.settings.set", map[string]any{
		"cellular_enabled": true, "screen_brightness": 60, "locale": "zh-CN",
	}); err != nil {
		t.Fatal(err)
	}
	info, err := lookupMethod("client.device.settings.set")
	if err != nil {
		t.Fatal(err)
	}
	request, err := proto.Marshal(&rpcpb.ClientDeviceSettingsSetRequest{Value: &rpcpb.DeviceSettings{
		ScreenBrightness: new(int64(80)), NfcEnabled: new(false),
	}})
	if err != nil {
		t.Fatal(err)
	}
	payload, code, _, err := provider.answer(info.id, request)
	if err != nil || code != 0 {
		t.Fatalf("answer code=%d err=%v", code, err)
	}
	settings, err := decodePayload(info.response, payload)
	if err != nil {
		t.Fatal(err)
	}
	if settings["screen_brightness"] != "80" || settings["locale"] != "zh-CN" || settings["cellular_enabled"] != true ||
		settings["nfc_enabled"] != false {
		t.Fatalf("patched settings = %#v", settings)
	}
	if _, present := settings["led_brightness"]; present {
		t.Fatalf("unsupported member became present: %#v", settings)
	}
	// An empty patch changes nothing.
	payload, _, _, err = provider.answer(info.id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if settings, err = decodePayload(info.response, payload); err != nil || settings["screen_brightness"] != "60" {
		t.Fatalf("empty patch settings = %#v, %v", settings, err)
	}
}

// client.rpc.methods.get lists the scripted providers whether or not a step
// scripts it, counts the calls when one does, and takes no scripted response.
func TestRPCMethodsListsScriptedProviders(t *testing.T) {
	info, err := lookupMethod(rpcMethodsGet)
	if err != nil {
		t.Fatal(err)
	}
	provider := newClientRPCProvider()
	for _, method := range []string{"client.run.workspace.set", "client.device.factory_reset", "client.device.settings.get"} {
		if err := provider.install(method, nil); err != nil {
			t.Fatal(err)
		}
	}
	payload, code, _, err := provider.answer(info.id, nil)
	if err != nil || code != 0 {
		t.Fatalf("unscripted answer code=%d err=%v", code, err)
	}
	decoded, err := decodePayload(info.response, payload)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{"client.device.factory_reset", "client.device.settings.get", "client.run.workspace.set", rpcMethodsGet}
	if got, _ := decoded["methods"].([]any); !slices.Equal(got, want) {
		t.Fatalf("methods = %#v, want %#v", decoded["methods"], want)
	}
	if err := provider.install(rpcMethodsGet, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := provider.answer(info.id, nil); err != nil {
		t.Fatal(err)
	}
	if got := provider.callCount(rpcMethodsGet); got != 1 {
		t.Fatalf("%s call count = %d, want 1", rpcMethodsGet, got)
	}
	if err := provider.install(rpcMethodsGet, map[string]any{"methods": []any{}}); err == nil {
		t.Fatal("a scripted client.rpc.methods.get answer was accepted")
	}
	// Factory reset and the Workspace switch acknowledge by default.
	for _, method := range []string{"client.device.factory_reset", "client.run.workspace.set"} {
		method, err := lookupMethod(method)
		if err != nil {
			t.Fatal(err)
		}
		payload, code, _, err := provider.answer(method.id, nil)
		if err != nil || code != 0 || len(payload) != 0 {
			t.Fatalf("ack code=%d payload=%x err=%v", code, payload, err)
		}
	}
}
