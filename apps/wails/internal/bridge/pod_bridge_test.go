package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/appconfig"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/endpointhealth"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/localserver"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/webui"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestRemotePodPreservesWriteOnlyKeysAndHandsAdminAllServers(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	web := webui.New(fstest.MapFS{"admin.html": {Data: []byte("admin")}, "play.html": {Data: []byte("play")}})
	defer web.Shutdown()
	bridge := &PodBridge{Paths: paths, Store: appconfig.Store{Paths: paths}, Health: endpointhealth.New(), Local: localserver.New(), WebUI: web}
	adminA, adminB, client := bridgeTestKey(t, 0x71), bridgeTestKey(t, 0x72), bridgeTestKey(t, 0x73)
	created, err := bridge.CreatePod(context.Background(), PodInput{
		Version: 1,
		ID:      "remote-lab",
		Name:    "Remote Lab",
		RemoteServers: []RemoteServerInput{
			{ID: "server-a", Name: "Server A", Endpoint: "127.0.0.1:19001", AdminPrivateKey: &adminA},
			{ID: "server-b", Name: "Server B", Endpoint: "127.0.0.1:19002", AdminPrivateKey: &adminB},
		},
		RemoteAccessPoint: "127.0.0.1:19820",
		ClientPrivateKey:  &client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Valid || created.Remote == nil || len(created.Remote.Servers) != 2 || !created.PlayConfigured {
		t.Fatalf("CreatePod() = %+v", created)
	}

	updated, err := bridge.UpdatePod(context.Background(), PodInput{
		Version: 1,
		ID:      "remote-lab",
		Name:    "Renamed Lab",
		RemoteServers: []RemoteServerInput{
			{ID: "server-a", Name: "Server A", Endpoint: "127.0.0.1:19001"},
			{ID: "server-b", Name: "Server B", Endpoint: "127.0.0.1:19002"},
		},
		RemoteAccessPoint: "127.0.0.1:19820",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Renamed Lab" || !updated.Remote.Servers[0].AdminConfigured || !updated.PlayConfigured {
		t.Fatalf("UpdatePod() = %+v", updated)
	}
	persisted, err := bridge.Store.Load("remote-lab")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.RemoteServers[0].AdminPrivateKey != adminA || persisted.RemoteServers[1].AdminPrivateKey != adminB || persisted.ClientPrivateKey != client {
		t.Fatal("omitted write-only keys were not preserved")
	}

	launch, err := bridge.AdminURL(context.Background(), "remote-lab", "server-b")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(launch)
	token := strings.TrimPrefix(parsed.Fragment, "launch=")
	body, _ := json.Marshal(map[string]string{"token": token})
	request, _ := http.NewRequest(http.MethodPost, "http://"+parsed.Host+"/__gizclaw/runtime", bytes.NewReader(body))
	request.Header.Set("Origin", "http://"+parsed.Host)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var runtime webui.Runtime
	if err := json.NewDecoder(response.Body).Decode(&runtime); err != nil {
		t.Fatal(err)
	}
	if runtime.AdminServerID != "server-b" || len(runtime.AdminServers) != 2 || runtime.AdminServers[1].Context.Endpoint != "127.0.0.1:19002" {
		t.Fatalf("Admin runtime = %+v", runtime)
	}
}

func TestLocalPodCreationAssignsDistinctStablePorts(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	web := webui.New(fstest.MapFS{"admin.html": {Data: []byte("admin")}})
	defer web.Shutdown()
	bridge := &PodBridge{Paths: paths, Store: appconfig.Store{Paths: paths}, Health: endpointhealth.New(), Local: localserver.New(), WebUI: web}
	first, err := bridge.CreatePod(context.Background(), PodInput{Version: 1, ID: "local-a", Name: "Local A", LocalServer: &LocalServerInput{Port: 0}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := bridge.CreatePod(context.Background(), PodInput{Version: 1, ID: "local-b", Name: "Local B", LocalServer: &LocalServerInput{Port: 0}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Local == nil || second.Local == nil || first.Local.Port == second.Local.Port || first.Local.Port == 0 || second.Local.Port == 0 {
		t.Fatalf("assigned ports = %+v / %+v", first.Local, second.Local)
	}
	reloaded, err := bridge.GetPod(context.Background(), "local-a")
	if err != nil || reloaded.Local.Port != first.Local.Port {
		t.Fatalf("reloaded port = %+v, %v", reloaded.Local, err)
	}
}

func bridgeTestKey(t *testing.T, fill byte) string {
	t.Helper()
	var key giznet.Key
	for i := range key {
		key[i] = fill
	}
	kp, err := giznet.NewKeyPair(key)
	if err != nil {
		t.Fatal(err)
	}
	return kp.Private.String()
}
