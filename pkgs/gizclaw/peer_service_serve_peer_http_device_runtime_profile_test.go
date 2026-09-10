package gizclaw

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func runtimeProfileHTTPBinding(resourceID, displayName string) apitypes.RuntimeProfileBinding {
	return apitypes.RuntimeProfileBinding{ResourceId: resourceID, I18n: map[string]apitypes.RuntimeProfileI18nText{
		"en": {DisplayName: displayName}, "zh-CN": {DisplayName: displayName},
	}}
}

// seedRuntimeProfile stores a RuntimeProfile and binds it to owner, so the
// read resolves it exactly like a registered device.
func seedRuntimeProfile(t *testing.T, f *deviceHTTPFixture, owner giznet.PublicKey, id string, spec apitypes.RuntimeProfileSpec) apitypes.RuntimeProfile {
	t.Helper()
	ctx := context.Background()
	response, err := f.manager.RuntimeProfiles.CreateRuntimeProfile(ctx, adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{Id: id, Spec: spec}})
	if err != nil {
		t.Fatalf("CreateRuntimeProfile() error = %v", err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("CreateRuntimeProfile() response = %#v", response)
	}
	if err := f.manager.RuntimeProfiles.BindOwnerProfile(ctx, owner.String(), id); err != nil {
		t.Fatalf("BindOwnerProfile() error = %v", err)
	}
	profile, err := f.manager.RuntimeProfiles.ResolveOwnerProfile(ctx, owner.String())
	if err != nil {
		t.Fatalf("ResolveOwnerProfile() error = %v", err)
	}
	return profile
}

func storyTellerSpec() apitypes.RuntimeProfileSpec {
	models := map[string]apitypes.RuntimeProfileBinding{"chat": runtimeProfileHTTPBinding("secret-model-resource", "Chat Model")}
	voices := map[string]apitypes.RuntimeProfileBinding{"narrator": runtimeProfileHTTPBinding("secret-voice-resource", "Narrator Voice")}
	appConfig := apitypes.RuntimeProfileAppConfig{"theme": "secret-app-config-value"}
	return apitypes.RuntimeProfileSpec{
		AppConfig: &appConfig,
		Resources: apitypes.RuntimeProfileResources{Models: &models, Voices: &voices},
		Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{
			"story-teller": {
				"story.alice": runtimeProfileHTTPBinding("secret-workflow-alice", "Alice Display"),
				"story.aesop": runtimeProfileHTTPBinding("secret-workflow-aesop", "Aesop Display"),
			},
			"games": {"game.riddle": runtimeProfileHTTPBinding("secret-workflow-riddle", "Riddle Display")},
		}},
	}
}

func TestGetDeviceRuntimeProfileReturnsSortedCatalogWhileOffline(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	profile := seedRuntimeProfile(t, f, f.owner, "h106-tiga", storyTellerSpec())

	// No connection is registered for the owner, so the read never needs the device.
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/runtime-profile", "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET runtime-profile status = %d body=%s", response.Code, response.Body.String())
	}
	got := decodeJSON[peerhttp.DeviceRuntimeProfile](t, response)
	want := peerhttp.DeviceRuntimeProfile{
		Name: "h106-tiga", Revision: profile.Revision,
		Collections: []peerhttp.DeviceRuntimeProfileCollection{
			{Name: "games", Workflows: []peerhttp.DeviceRuntimeProfileWorkflow{{Name: "game.riddle"}}},
			{Name: "story-teller", Workflows: []peerhttp.DeviceRuntimeProfileWorkflow{{Name: "story.aesop"}, {Name: "story.alice"}}},
		},
	}
	if profile.Revision == "" || !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime profile = %#v, want %#v", got, want)
	}
}

func TestGetDeviceRuntimeProfileExposesOnlyNames(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	seedRuntimeProfile(t, f, f.owner, "h106-tiga", storyTellerSpec())

	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/runtime-profile", "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET runtime-profile status = %d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, secret := range []string{"secret-", "resource_id", "i18n", "Display", "Chat Model", "Narrator", "app_config", "driver", "models", "voices"} {
		if strings.Contains(body, secret) {
			t.Fatalf("response leaks %q: %s", secret, body)
		}
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatal(err)
	}
	assertJSONKeys(t, raw, "name", "revision", "collections")
	var collections []map[string]json.RawMessage
	if err := json.Unmarshal(raw["collections"], &collections); err != nil {
		t.Fatal(err)
	}
	for _, collection := range collections {
		assertJSONKeys(t, collection, "name", "workflows")
		var workflows []map[string]json.RawMessage
		if err := json.Unmarshal(collection["workflows"], &workflows); err != nil {
			t.Fatal(err)
		}
		for _, workflow := range workflows {
			assertJSONKeys(t, workflow, "name")
		}
	}
}

func assertJSONKeys(t *testing.T, object map[string]json.RawMessage, want ...string) {
	t.Helper()
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	slices.Sort(want)
	if !slices.Equal(keys, want) {
		t.Fatalf("JSON keys = %v, want %v", keys, want)
	}
}

func TestGetDeviceRuntimeProfileIsOwnerScoped(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	ctx := context.Background()
	seedRuntimeProfile(t, f, f.owner, "h106-tiga", storyTellerSpec())

	otherKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.peers.SavePeer(ctx, apitypes.Peer{
		PublicKey: otherKey.Public.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive,
	}); err != nil {
		t.Fatal(err)
	}
	otherProfile := seedRuntimeProfile(t, f, otherKey.Public, "other-profile", apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{
			"other-collection": {"other.workflow": runtimeProfileHTTPBinding("other-workflow", "Other")},
		}},
	})
	otherSecret, err := f.apiKeys.Create(ctx, otherKey.Public.String(), "other-phone", true)
	if err != nil {
		t.Fatal(err)
	}

	// A caller-supplied Peer selector is ignored: each key only sees its owner.
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/runtime-profile?public_key="+otherKey.Public.String(), "")
	if response.Code != http.StatusOK {
		t.Fatalf("owner GET status = %d body=%s", response.Code, response.Body.String())
	}
	if got := decodeJSON[peerhttp.DeviceRuntimeProfile](t, response); got.Name != "h106-tiga" || len(got.Collections) != 2 {
		t.Fatalf("owner runtime profile = %#v", got)
	}
	response = f.doWithSecret(t, otherSecret.Secret, http.MethodGet, "/gizclaw/v1/device/runtime-profile", "")
	if response.Code != http.StatusOK {
		t.Fatalf("other GET status = %d body=%s", response.Code, response.Body.String())
	}
	want := peerhttp.DeviceRuntimeProfile{
		Name: "other-profile", Revision: otherProfile.Revision,
		Collections: []peerhttp.DeviceRuntimeProfileCollection{
			{Name: "other-collection", Workflows: []peerhttp.DeviceRuntimeProfileWorkflow{{Name: "other.workflow"}}},
		},
	}
	if got := decodeJSON[peerhttp.DeviceRuntimeProfile](t, response); !reflect.DeepEqual(got, want) {
		t.Fatalf("other runtime profile = %#v, want %#v", got, want)
	}
}

func TestGetDeviceRuntimeProfileFollowsCurrentBinding(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	seedRuntimeProfile(t, f, f.owner, "h106-tiga", storyTellerSpec())
	seedRuntimeProfile(t, f, f.owner, "h106-next", apitypes.RuntimeProfileSpec{})

	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/runtime-profile", "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET runtime-profile status = %d body=%s", response.Code, response.Body.String())
	}
	got := decodeJSON[peerhttp.DeviceRuntimeProfile](t, response)
	if got.Name != "h106-next" || got.Collections == nil || len(got.Collections) != 0 {
		t.Fatalf("rebound runtime profile = %#v, want h106-next with no collections", got)
	}
}

func TestGetDeviceRuntimeProfileWithoutBinding(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	if err := f.manager.RuntimeProfiles.DeleteOwnerProfileBinding(context.Background(), f.owner.String()); err != nil {
		t.Fatal(err)
	}
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/runtime-profile", "")
	if response.Code != http.StatusForbidden || errorCode(t, response) != "API_KEY_OWNER_UNAVAILABLE" {
		t.Fatalf("unbound GET status = %d body=%s", response.Code, response.Body.String())
	}
}

// The binding can disappear after the middleware validated the owner; the
// handler answers the same 403 instead of a redacted 500.
func TestGetDeviceRuntimeProfileHandlerMapsLostBinding(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	if err := f.manager.RuntimeProfiles.DeleteOwnerProfileBinding(context.Background(), f.owner.String()); err != nil {
		t.Fatal(err)
	}
	service := &PeerService{manager: f.manager}
	handler := &peerHTTP{DeviceReads: service.deviceReadsForAPIKey}
	ctx := peerhttp.WithCallerPublicKey(context.Background(), f.owner)
	response, err := handler.GetDeviceRuntimeProfile(ctx, peerhttp.GetDeviceRuntimeProfileRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	forbidden, ok := response.(peerhttp.GetDeviceRuntimeProfile403JSONResponse)
	if !ok || forbidden.Error.Code != "API_KEY_OWNER_UNAVAILABLE" {
		t.Fatalf("response = %#v", response)
	}

	if response, _ := (&peerHTTP{}).GetDeviceRuntimeProfile(ctx, peerhttp.GetDeviceRuntimeProfileRequestObject{}); !isResponseType[peerhttp.GetDeviceRuntimeProfile500JSONResponse](response) {
		t.Fatalf("unconfigured response = %#v", response)
	}
	if response, _ := handler.GetDeviceRuntimeProfile(context.Background(), peerhttp.GetDeviceRuntimeProfileRequestObject{}); !isResponseType[peerhttp.GetDeviceRuntimeProfile401JSONResponse](response) {
		t.Fatalf("ownerless response = %#v", response)
	}
}

func isResponseType[T any](value any) bool {
	_, ok := value.(T)
	return ok
}
