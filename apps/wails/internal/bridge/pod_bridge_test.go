package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

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
	if len(first.Local.LANAddresses) != 0 && first.Local.LANAddresses[0] != appconfig.PreferredLANEndpoint(first.Local.Port) {
		t.Fatalf("shared LAN address = %q, workspace endpoint = %q", first.Local.LANAddresses[0], appconfig.PreferredLANEndpoint(first.Local.Port))
	}
	reloaded, err := bridge.GetPod(context.Background(), "local-a")
	if err != nil || reloaded.Local.Port != first.Local.Port {
		t.Fatalf("reloaded port = %+v, %v", reloaded.Local, err)
	}
}

func TestUpdatePodHonorsExplicitIdentityClearing(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	web := webui.New(fstest.MapFS{})
	defer web.Shutdown()
	b := &PodBridge{Paths: paths, Store: appconfig.Store{Paths: paths}, Health: endpointhealth.New(), Local: localserver.New(), WebUI: web}
	created, err := b.CreatePod(context.Background(), PodInput{Version: 1, ID: "clear-identities", Name: "Clear Identities", LocalServer: &LocalServerInput{Port: 0}})
	if err != nil {
		t.Fatal(err)
	}
	empty := ""
	updated, err := b.UpdatePod(context.Background(), PodInput{Version: 1, ID: created.ID, Name: created.Name, LocalServer: &LocalServerInput{Port: created.Local.Port, AdminPrivateKey: &empty}, ClientPrivateKey: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if updated.PlayConfigured || updated.Local.AdminConfigured {
		t.Fatalf("explicitly cleared identities were regenerated: %+v", updated)
	}
	listed, err := b.ListPods(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].PlayConfigured || listed[0].Local.AdminConfigured {
		t.Fatalf("cleared identities did not persist: %+v", listed)
	}
}

func TestStopLocalRejectsRemotePod(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	web := webui.New(fstest.MapFS{})
	defer web.Shutdown()
	b := &PodBridge{Paths: paths, Store: appconfig.Store{Paths: paths}, Health: endpointhealth.New(), Local: localserver.New(), WebUI: web}
	created, err := b.CreatePod(context.Background(), PodInput{Version: 1, ID: "remote-stop", Name: "Remote", RemoteAccessPoint: "127.0.0.1:19820"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.StopLocal(context.Background(), created.ID); err == nil || !strings.Contains(err.Error(), "is remote") {
		t.Fatalf("StopLocal error = %v", err)
	}
}

func TestSupersededHealthRefreshCannotOverwriteNewerResult(t *testing.T) {
	kp, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	firstStarted := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(firstStarted)
			<-r.Context().Done()
			return
		}
		_, _ = fmt.Fprintf(w, `{"endpoint":"127.0.0.1:9820","protocol":"gizclaw-webrtc","public_key":%q,"server_time":1,"signaling_path":"/webrtc/v1/offer"}`, kp.Public.String())
	}))
	defer server.Close()
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	web := webui.New(fstest.MapFS{})
	defer web.Shutdown()
	b := &PodBridge{Paths: paths, Store: appconfig.Store{Paths: paths}, Health: endpointhealth.New(), Local: localserver.New(), WebUI: web}
	endpoint := strings.TrimPrefix(server.URL, "http://")
	created, err := b.CreatePod(context.Background(), PodInput{Version: 1, ID: "refresh-generation", Name: "Refresh", RemoteAccessPoint: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan struct{})
	go func() {
		_, _ = b.RefreshHealth(context.Background(), created.ID)
		close(firstDone)
	}()
	<-firstStarted
	newer, err := b.RefreshHealth(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	<-firstDone
	if newer.Remote.AccessPoint.State != endpointhealth.Reachable || b.Health.Get(endpoint).State != endpointhealth.Reachable {
		t.Fatalf("newer health was overwritten: summary=%+v cache=%+v", newer.Remote.AccessPoint, b.Health.Get(endpoint))
	}
}

func TestConcurrentLocalPodCreationAssignsDistinctPorts(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	web := webui.New(fstest.MapFS{})
	defer web.Shutdown()
	b := &PodBridge{Paths: paths, Store: appconfig.Store{Paths: paths}, Health: endpointhealth.New(), Local: localserver.New(), WebUI: web}
	results := make(chan PodSummary, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"concurrent-a", "concurrent-b"} {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := b.CreatePod(context.Background(), PodInput{Version: 1, ID: id, Name: id, LocalServer: &LocalServerInput{Port: 0}})
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	ports := map[int]bool{}
	for result := range results {
		if result.Local == nil || result.Local.Port == 0 || ports[result.Local.Port] {
			t.Fatalf("duplicate or invalid local port: %+v", result.Local)
		}
		ports[result.Local.Port] = true
	}
}

func TestRefreshHealthMarksStoppedLocalServerUnreachable(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	web := webui.New(fstest.MapFS{})
	defer web.Shutdown()
	b := &PodBridge{Paths: paths, Store: appconfig.Store{Paths: paths}, Health: endpointhealth.New(), Local: localserver.New(), WebUI: web}
	created, err := b.CreatePod(context.Background(), PodInput{Version: 1, ID: "health-local", Name: "Health Local", LocalServer: &LocalServerInput{Port: 0}})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", created.Local.Port))
	if err != nil {
		t.Fatal(err)
	}
	kp, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"endpoint":"127.0.0.1:%d","protocol":"gizclaw-webrtc","public_key":%q,"server_time":1,"signaling_path":"/webrtc/v1/offer"}`, created.Local.Port, kp.Public.String())
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	endpoint := fmt.Sprintf("127.0.0.1:%d", created.Local.Port)
	if result := b.Health.Probe(context.Background(), endpoint); result.State != endpointhealth.Reachable {
		t.Fatalf("initial probe = %+v", result)
	}
	refreshed, err := b.RefreshHealth(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Local.Health.State != endpointhealth.Unreachable || refreshed.Local.Health.Message != "local server is stopped" {
		t.Fatalf("stopped local health = %+v", refreshed.Local.Health)
	}
}

func TestUpdatePodDoesNotStopRunningLocalServerBeforeModeChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "fake-gizclaw")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\ntrap 'exit 0' INT TERM\nwhile :; do sleep 1; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	local := localserver.New()
	local.Executable = executable
	web := webui.New(fstest.MapFS{})
	defer web.Shutdown()
	b := &PodBridge{Paths: paths, Store: appconfig.Store{Paths: paths}, Health: endpointhealth.New(), Local: local, WebUI: web}
	created, err := b.CreatePod(context.Background(), PodInput{Version: 1, ID: "running-local", Name: "Running Local", LocalServer: &LocalServerInput{Port: 0}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.Start(created.ID, filepath.Join(paths.PodsDir, created.ID, "workspace")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = local.Stop(ctx, created.ID)
	}()
	_, err = b.UpdatePod(context.Background(), PodInput{Version: 1, ID: created.ID, Name: created.Name, RemoteAccessPoint: "127.0.0.1:19820"})
	if err == nil || !strings.Contains(err.Error(), "stop the local server") {
		t.Fatalf("UpdatePod error = %v", err)
	}
	if status := local.Status(created.ID); status.State != "running" {
		t.Fatalf("local process state = %q, want running", status.State)
	}
	loaded, err := b.Store.Load(created.ID)
	if err != nil || loaded.LocalServer == nil {
		t.Fatalf("persisted Pod changed mode: %+v, %v", loaded, err)
	}
}

func TestPodCreationGeneratesInternalIDsAndAllowsEmptyRemoteInventory(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	web := webui.New(fstest.MapFS{"admin.html": {Data: []byte("admin")}})
	defer web.Shutdown()
	bridge := &PodBridge{Paths: paths, Store: appconfig.Store{Paths: paths}, Health: endpointhealth.New(), Local: localserver.New(), WebUI: web}
	local, err := bridge.CreatePod(context.Background(), PodInput{Version: 1, Name: "Local Server", LocalServer: &LocalServerInput{Port: 0}})
	if err != nil {
		t.Fatal(err)
	}
	remote, err := bridge.CreatePod(context.Background(), PodInput{Version: 1, Name: "Remote Server", RemoteAccessPoint: "127.0.0.1:19820"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(local.ID, "pod-") || !strings.HasPrefix(remote.ID, "pod-") || local.ID == remote.ID {
		t.Fatalf("generated IDs = %q / %q", local.ID, remote.ID)
	}
	if !local.PlayConfigured || local.PlayPublicKey == "" || local.Local == nil || !local.Local.AdminConfigured || local.Local.AdminPublicKey == "" || local.Local.ServerPublicKey == "" {
		t.Fatalf("generated local identities = %+v", local)
	}
	if !remote.PlayConfigured || remote.PlayPublicKey == "" {
		t.Fatalf("generated remote Play identity = %+v", remote)
	}
	if remote.Remote == nil || len(remote.Remote.Servers) != 0 {
		t.Fatalf("remote summary = %+v", remote.Remote)
	}

	updated, err := bridge.UpdatePod(context.Background(), PodInput{
		Version: 1,
		ID:      remote.ID,
		Name:    remote.Name,
		RemoteServers: []RemoteServerInput{
			{Endpoint: "127.0.0.1:19821"},
		},
		RemoteAccessPoint: "127.0.0.1:19820",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Remote.Servers) != 1 || !strings.HasPrefix(updated.Remote.Servers[0].ID, "server-") || updated.Remote.Servers[0].Name != "127.0.0.1:19821" || updated.Remote.Servers[0].AdminConfigured || updated.Remote.Servers[0].AdminPublicKey != "" {
		t.Fatalf("Server without configured Admin key = %+v", updated.Remote.Servers)
	}
	remotePersisted, err := bridge.Store.Load(remote.ID)
	if err != nil {
		t.Fatal(err)
	}
	if remotePersisted.RemoteServers[0].AdminPrivateKey != "" {
		t.Fatal("Remote Server Admin private key was generated instead of remaining unconfigured")
	}
	persisted, err := bridge.Store.Load(local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ClientPrivateKey == "" || persisted.LocalServer.AdminPrivateKey == "" {
		t.Fatalf("local private identities were not persisted: %+v", persisted)
	}
}

func TestListPodsMigratesMissingDesktopIdentities(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store := appconfig.Store{Paths: paths}
	pod := appconfig.Pod{Version: 1, ID: "legacy-local", Name: "Legacy Local", LocalServer: &appconfig.LocalServer{Port: 19824}}
	if err := store.Save(pod); err != nil {
		t.Fatal(err)
	}
	b := &PodBridge{Paths: paths, Store: store, Health: endpointhealth.New(), Local: localserver.New(), WebUI: webui.New(fstest.MapFS{})}
	defer b.WebUI.Shutdown()
	pods, err := b.ListPods(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 1 || !pods[0].PlayConfigured || pods[0].PlayPublicKey == "" || pods[0].Local == nil || !pods[0].Local.AdminConfigured || pods[0].Local.AdminPublicKey == "" {
		t.Fatalf("migrated summary = %+v", pods)
	}
	loaded, err := store.Load("legacy-local")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ClientPrivateKey == "" || loaded.LocalServer.AdminPrivateKey == "" {
		t.Fatalf("migrated pod = %+v", loaded)
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
