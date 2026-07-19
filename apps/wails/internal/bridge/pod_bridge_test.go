package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	registrationToken := "remote-registration-secret"
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
		RegistrationToken: &registrationToken,
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
	if persisted.RemoteServers[0].AdminPrivateKey != adminA || persisted.RemoteServers[1].AdminPrivateKey != adminB || persisted.ClientPrivateKey != client || persisted.RegistrationToken != registrationToken {
		t.Fatal("omitted write-only keys were not preserved")
	}

	launch, err := bridge.AdminURL(context.Background(), "remote-lab", "server-b")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(launch)
	token := parsed.Query().Get("token")
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

	playLaunch, err := bridge.PlayURL(context.Background(), "remote-lab")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(playLaunch, registrationToken) {
		t.Fatal("Play URL contains the RegistrationToken")
	}
	playParsed, _ := url.Parse(playLaunch)
	playBody, _ := json.Marshal(map[string]string{"token": playParsed.Query().Get("token")})
	playRequest, _ := http.NewRequest(http.MethodPost, "http://"+playParsed.Host+"/__gizclaw/runtime", bytes.NewReader(playBody))
	playRequest.Header.Set("Origin", "http://"+playParsed.Host)
	playResponse, err := http.DefaultClient.Do(playRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer playResponse.Body.Close()
	var playRuntime webui.Runtime
	if err := json.NewDecoder(playResponse.Body).Decode(&playRuntime); err != nil {
		t.Fatal(err)
	}
	if playRuntime.RegistrationToken != registrationToken {
		t.Fatalf("remote Play RegistrationToken = %q", playRuntime.RegistrationToken)
	}
}

func TestLocalPlayRuntimeHandsOffRegistrationTokenWithoutPuttingItInURL(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	web := webui.New(fstest.MapFS{"play.html": {Data: []byte("play")}})
	defer web.Shutdown()
	store := appconfig.Store{Paths: paths}
	pod := appconfig.Pod{
		Version:               1,
		ID:                    "local-play",
		Name:                  "Local Play",
		IdentitiesInitialized: true,
		LocalServer:           &appconfig.LocalServer{Port: 19820, AdminPrivateKey: bridgeTestKey(t, 0x74)},
		ClientPrivateKey:      bridgeTestKey(t, 0x75),
	}
	if err := store.Save(pod); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(paths.PodsDir, pod.ID, "workspace", localserver.RegistrationTokenFile)
	if err := os.WriteFile(tokenPath, []byte("local-registration-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	bridge := &PodBridge{Paths: paths, Store: store, Health: endpointhealth.New(), Local: localserver.New(), WebUI: web}
	launch, err := bridge.PlayURL(context.Background(), pod.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(launch, "local-registration-secret") {
		t.Fatal("Play URL contains the RegistrationToken")
	}
	parsed, _ := url.Parse(launch)
	body, _ := json.Marshal(map[string]string{"token": parsed.Query().Get("token")})
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
	if runtime.RegistrationToken != "local-registration-secret" {
		t.Fatalf("Play RegistrationToken = %q", runtime.RegistrationToken)
	}
}

func TestLocalPlayRuntimeRecoversMissingRegistrationToken(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	web := webui.New(fstest.MapFS{"play.html": {Data: []byte("play")}})
	defer web.Shutdown()
	store := appconfig.Store{Paths: paths}
	pod := appconfig.Pod{
		Version:               1,
		ID:                    "legacy-local-play",
		Name:                  "Legacy Local Play",
		IdentitiesInitialized: true,
		LocalServer:           &appconfig.LocalServer{Port: 19821, AdminPrivateKey: bridgeTestKey(t, 0x76)},
		ClientPrivateKey:      bridgeTestKey(t, 0x77),
	}
	if err := store.Save(pod); err != nil {
		t.Fatal(err)
	}
	bootstrapper := &fakeLocalPodBootstrapper{recoveryToken: "recovered-registration-secret"}
	bridge := &PodBridge{
		Paths:                paths,
		Store:                store,
		BootstrapEnvironment: appconfig.BootstrapEnvironmentStore{Path: paths.BootstrapEnvFile},
		Bootstrapper:         bootstrapper,
		Health:               endpointhealth.New(),
		Local:                localserver.New(),
		WebUI:                web,
	}
	if _, err := bridge.PlayURL(context.Background(), pod.ID); err != nil {
		t.Fatal(err)
	}
	if !bootstrapper.recoveryCalled.Load() {
		t.Fatal("PlayURL did not recover the missing RegistrationToken")
	}
	tokenPath := filepath.Join(paths.PodsDir, pod.ID, "workspace", localserver.RegistrationTokenFile)
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(token) != "recovered-registration-secret" {
		t.Fatalf("recovered RegistrationToken = %q", token)
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

func TestCreatePodRejectsConcurrentDuplicateRequest(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	b := &PodBridge{
		Paths:  paths,
		Store:  appconfig.Store{Paths: paths},
		Health: endpointhealth.New(),
		Local:  localserver.New(),
	}
	b.mutationMu.Lock()
	firstDone := make(chan error, 1)
	go func() {
		_, err := b.CreatePod(context.Background(), PodInput{
			Version: 1, ID: "first-create", Name: "First", RemoteAccessPoint: "127.0.0.1:19820",
		})
		firstDone <- err
	}()
	deadline := time.Now().Add(time.Second)
	_, creating := b.creating.Load("first-create")
	for !creating && time.Now().Before(deadline) {
		runtime.Gosched()
		_, creating = b.creating.Load("first-create")
	}
	if !creating {
		b.mutationMu.Unlock()
		t.Fatal("first CreatePod did not enter the single-flight section")
	}
	_, duplicateErr := b.CreatePod(context.Background(), PodInput{
		Version: 1, ID: "first-create", Name: "Duplicate", RemoteAccessPoint: "127.0.0.1:19821",
	})
	b.mutationMu.Unlock()
	if duplicateErr == nil || !strings.Contains(duplicateErr.Error(), "creation is already in progress") {
		t.Fatalf("duplicate CreatePod() error = %v", duplicateErr)
	}
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	listed, err := b.ListPods(context.Background())
	if err != nil || len(listed) != 1 || listed[0].ID != "first-create" {
		t.Fatalf("ListPods() = %+v, %v", listed, err)
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
		wg.Go(func() {
			result, err := b.CreatePod(context.Background(), PodInput{Version: 1, ID: id, Name: id, LocalServer: &LocalServerInput{Port: 0}})
			results <- result
			errs <- err
		})
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
	if remote.PlayConfigured || remote.PlayPublicKey == "" {
		t.Fatalf("remote Play should wait for a RegistrationToken: %+v", remote)
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

type fakeLocalPodBootstrapper struct {
	called         atomic.Bool
	calls          atomic.Int32
	err            error
	recoveryCalled atomic.Bool
	recoveryErr    error
	recoveryToken  string
	started        chan struct{}
	release        chan struct{}
	once           sync.Once
}

func (f *fakeLocalPodBootstrapper) RecoverRegistrationToken(_ context.Context, podDir string, _ map[string]string) error {
	f.recoveryCalled.Store(true)
	if f.recoveryErr != nil {
		return f.recoveryErr
	}
	workspaceDir := filepath.Join(podDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(workspaceDir, localserver.RegistrationTokenFile), []byte(f.recoveryToken), 0o600)
}

func (f *fakeLocalPodBootstrapper) Apply(ctx context.Context, _ string, _ map[string]string) error {
	f.called.Store(true)
	f.calls.Add(1)
	if f.started != nil {
		f.once.Do(func() { close(f.started) })
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.err
}

func TestLocalPodCreationReturnsWhileBootstrapRunsInBackground(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	environment := appconfig.BootstrapEnvironmentStore{Path: paths.BootstrapEnvFile}
	if err := environment.Replace("BOOTSTRAP_REQUIRED=configured\n"); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "fake-gizclaw")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\ntrap 'exit 0' INT TERM\nwhile :; do sleep 1; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	local := localserver.New()
	local.Executable = executable
	port, err := appconfig.FindAvailablePort(0)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapper := &fakeLocalPodBootstrapper{started: make(chan struct{}), release: make(chan struct{})}
	b := &PodBridge{
		Paths:                paths,
		Store:                appconfig.Store{Paths: paths},
		BootstrapEnvironment: environment,
		Catalog:              &localserver.Catalog{Requirements: []localserver.EnvironmentRequirement{{Name: "BOOTSTRAP_REQUIRED"}}},
		Bootstrapper:         bootstrapper,
		WaitLocalReady:       func(context.Context, string, int) error { return nil },
		Health:               endpointhealth.New(),
		Local:                local,
		WebUI:                webui.New(fstest.MapFS{}),
	}
	defer b.WebUI.Shutdown()
	created, err := b.CreatePod(context.Background(), PodInput{Version: 1, ID: "bootstrapped", Name: "Bootstrapped", LocalServer: &LocalServerInput{Port: port}})
	if err != nil {
		t.Fatal(err)
	}
	if created.Initialization == nil || created.Initialization.State != "initializing" {
		t.Fatalf("CreatePod() = %+v", created)
	}
	listed, err := b.ListPods(context.Background())
	if err != nil || len(listed) != 1 || listed[0].Initialization == nil {
		t.Fatalf("ListPods() during initialization = %+v, %v", listed, err)
	}
	select {
	case <-bootstrapper.started:
	case <-time.After(2 * time.Second):
		t.Fatal("background bootstrap did not start")
	}
	close(bootstrapper.release)
	waitForInitializationState(t, b.Store, created.ID, "")
	ready, err := b.GetPod(context.Background(), created.ID)
	if err != nil || ready.Initialization != nil || ready.Local == nil || ready.Local.Process.State != "running" {
		t.Fatalf("GetPod() after initialization = %+v, %v", ready, err)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if _, err := b.StopLocal(stopCtx, created.ID); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	if _, err := b.StartLocal(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	if bootstrapper.calls.Load() != 1 {
		t.Fatalf("bootstrap calls after restart = %d, want 1", bootstrapper.calls.Load())
	}
	stopCtx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = local.Stop(stopCtx, created.ID)
}

func TestDeletePodCancelsBackgroundInitialization(t *testing.T) {
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
	bootstrapper := &fakeLocalPodBootstrapper{started: make(chan struct{}), release: make(chan struct{})}
	b := &PodBridge{
		Paths:                paths,
		Store:                appconfig.Store{Paths: paths},
		BootstrapEnvironment: appconfig.BootstrapEnvironmentStore{Path: paths.BootstrapEnvFile},
		Catalog:              &localserver.Catalog{},
		Bootstrapper:         bootstrapper,
		WaitLocalReady:       func(context.Context, string, int) error { return nil },
		Health:               endpointhealth.New(),
		Local:                local,
		WebUI:                webui.New(fstest.MapFS{}),
	}
	defer b.WebUI.Shutdown()
	created, err := b.CreatePod(context.Background(), PodInput{Version: 1, ID: "cancel-initialization", Name: "Cancel", LocalServer: &LocalServerInput{Port: 0}})
	if err != nil || created.Initialization == nil {
		t.Fatalf("CreatePod() = %+v, %v", created, err)
	}
	select {
	case <-bootstrapper.started:
	case <-time.After(2 * time.Second):
		t.Fatal("background bootstrap did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := b.DeletePod(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.PodsDir, created.ID)); !os.IsNotExist(err) {
		t.Fatalf("deleted initializing Pod still exists: %v", err)
	}
	if local.Status(created.ID).State != "stopped" {
		t.Fatalf("deleted process state = %+v", local.Status(created.ID))
	}
}

func TestLocalPodBootstrapFailureRemainsVisibleAndStopsProcess(t *testing.T) {
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
	bootstrapper := &fakeLocalPodBootstrapper{err: errors.New("apply rejected")}
	b := &PodBridge{
		Paths:                paths,
		Store:                appconfig.Store{Paths: paths},
		BootstrapEnvironment: appconfig.BootstrapEnvironmentStore{Path: paths.BootstrapEnvFile},
		Catalog:              &localserver.Catalog{},
		Bootstrapper:         bootstrapper,
		WaitLocalReady:       func(context.Context, string, int) error { return nil },
		Health:               endpointhealth.New(),
		Local:                local,
		WebUI:                webui.New(fstest.MapFS{}),
	}
	defer b.WebUI.Shutdown()
	created, err := b.CreatePod(context.Background(), PodInput{Version: 1, ID: "bootstrap-failure", Name: "Failure", LocalServer: &LocalServerInput{Port: 0}})
	if err != nil || created.Initialization == nil || created.Initialization.State != "initializing" {
		t.Fatalf("CreatePod() = %+v, %v", created, err)
	}
	status := waitForInitializationState(t, b.Store, created.ID, "failed")
	if !strings.Contains(status.Error, "apply rejected") {
		t.Fatalf("initialization failure = %+v", status)
	}
	failed, err := b.GetPod(context.Background(), created.ID)
	if err != nil || failed.Initialization == nil || failed.Initialization.State != "failed" {
		t.Fatalf("GetPod() after failure = %+v, %v", failed, err)
	}
	if local.Status("bootstrap-failure").State != "stopped" {
		t.Fatalf("failed process state = %+v", local.Status("bootstrap-failure"))
	}
}

func TestMissingBootstrapEnvironmentFailsBeforePodReservation(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	bootstrapper := &fakeLocalPodBootstrapper{}
	b := &PodBridge{
		Paths:                paths,
		Store:                appconfig.Store{Paths: paths},
		BootstrapEnvironment: appconfig.BootstrapEnvironmentStore{Path: paths.BootstrapEnvFile},
		Catalog:              &localserver.Catalog{Requirements: []localserver.EnvironmentRequirement{{Name: "DEFINITELY_MISSING_BOOTSTRAP_VALUE"}}},
		Bootstrapper:         bootstrapper,
		Health:               endpointhealth.New(),
		Local:                localserver.New(),
		WebUI:                webui.New(fstest.MapFS{}),
	}
	defer b.WebUI.Shutdown()
	_, err := b.CreatePod(context.Background(), PodInput{Version: 1, ID: "not-reserved", Name: "Missing", LocalServer: &LocalServerInput{Port: 0}})
	if err == nil || !strings.Contains(err.Error(), "DEFINITELY_MISSING_BOOTSTRAP_VALUE") {
		t.Fatalf("CreatePod() error = %v", err)
	}
	if bootstrapper.called.Load() {
		t.Fatal("bootstrap ran with missing environment")
	}
	if _, err := os.Stat(filepath.Join(paths.PodsDir, "not-reserved")); !os.IsNotExist(err) {
		t.Fatalf("Pod was reserved before preflight: %v", err)
	}
	remote, err := b.CreatePod(context.Background(), PodInput{
		Version: 1, ID: "remote-without-bootstrap", Name: "Remote", RemoteAccessPoint: "127.0.0.1:19820",
	})
	if err != nil || remote.Remote == nil || bootstrapper.called.Load() {
		t.Fatalf("remote CreatePod() = %+v, %v; bootstrap called = %v", remote, err, bootstrapper.called.Load())
	}
	t.Setenv("DEFINITELY_MISSING_BOOTSTRAP_VALUE", "from-process")
	state, err := b.GetBootstrapEnvironment(context.Background())
	if err != nil || !state.Ready || len(state.Missing) != 0 || !state.Variables[0].Configured {
		t.Fatalf("process environment state = %+v, %v", state, err)
	}
}

func TestInvalidBootstrapEnvironmentRemainsEditable(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	const content = "TOKEN=first\nTOKEN='unterminated\n"
	if err := os.WriteFile(paths.BootstrapEnvFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	b := &PodBridge{
		Paths:                paths,
		Store:                appconfig.Store{Paths: paths},
		BootstrapEnvironment: appconfig.BootstrapEnvironmentStore{Path: paths.BootstrapEnvFile},
		Catalog:              &localserver.Catalog{Requirements: []localserver.EnvironmentRequirement{{Name: "TOKEN"}}},
		Bootstrapper:         &fakeLocalPodBootstrapper{},
		Health:               endpointhealth.New(),
		Local:                localserver.New(),
		WebUI:                webui.New(fstest.MapFS{}),
	}
	defer b.WebUI.Shutdown()
	bootstrap, err := b.Bootstrap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state := bootstrap.BootstrapEnvironment
	if state.Ready || state.Content != content || !strings.Contains(state.Error, "duplicate name") || state.Missing == nil || state.Variables == nil {
		t.Fatalf("BootstrapEnvironment = %+v", state)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"missing":[]`)) || !bytes.Contains(data, []byte(`"variables":[]`)) {
		t.Fatalf("BootstrapEnvironment JSON = %s", data)
	}
	_, err = b.CreatePod(context.Background(), PodInput{Version: 1, ID: "invalid-env", Name: "Invalid Env", LocalServer: &LocalServerInput{Port: 0}})
	if err == nil || !strings.Contains(err.Error(), "duplicate name") {
		t.Fatalf("CreatePod() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.PodsDir, "invalid-env")); !os.IsNotExist(err) {
		t.Fatalf("Pod was reserved with invalid bootstrap.env: %v", err)
	}
}

func TestRecoverLocalServerRejectsMismatchedServerIdentity(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	b := &PodBridge{Paths: paths, Store: appconfig.Store{Paths: paths}, Health: endpointhealth.New(), Local: localserver.New(), WebUI: webui.New(fstest.MapFS{})}
	defer b.WebUI.Shutdown()
	created, err := b.CreatePod(context.Background(), PodInput{Version: 1, ID: "recover-mismatch", Name: "Mismatch", LocalServer: &LocalServerInput{Port: 0}})
	if err != nil {
		t.Fatal(err)
	}
	pidPath := filepath.Join(paths.PodsDir, created.ID, "workspace", localserver.PIDFile)
	if err := os.WriteFile(pidPath, fmt.Appendf(nil, "%d\n", os.Getpid()), 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", created.Local.Port))
	if err != nil {
		t.Fatal(err)
	}
	mismatch, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"endpoint":       fmt.Sprintf("127.0.0.1:%d", created.Local.Port),
			"protocol":       "gizclaw-webrtc",
			"public_key":     mismatch.Public.String(),
			"server_time":    time.Now().Unix(),
			"signaling_path": "/webrtc",
		})
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	status, err := b.RecoverLocalServer(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "stopped" || status.PID != 0 {
		t.Fatalf("RecoverLocalServer() = %+v", status)
	}
	if current := b.Local.Status(created.ID); current.State != "stopped" || current.PID != 0 {
		t.Fatalf("process status = %+v", current)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("mismatched PID file error = %v", err)
	}
}

func TestRecoverLocalServersSurfacesEveryUnverifiedLivePID(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	b := &PodBridge{
		Paths:  paths,
		Store:  appconfig.Store{Paths: paths},
		Health: endpointhealth.New(),
		Local:  localserver.New(),
		WebUI:  webui.New(fstest.MapFS{}),
	}
	defer b.WebUI.Shutdown()
	for _, id := range []string{"recover-first", "recover-second"} {
		created, err := b.CreatePod(context.Background(), PodInput{Version: 1, ID: id, Name: id, LocalServer: &LocalServerInput{Port: 0}})
		if err != nil {
			t.Fatal(err)
		}
		pidPath := filepath.Join(paths.PodsDir, created.ID, "workspace", localserver.PIDFile)
		if err := os.WriteFile(pidPath, fmt.Appendf(nil, "%d\n", os.Getpid()), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := b.RecoverLocalServers(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"recover-first", "recover-second"} {
		pidPath := filepath.Join(paths.PodsDir, id, "workspace", localserver.PIDFile)
		if _, statErr := os.Stat(pidPath); statErr != nil {
			t.Fatalf("Pod %q PID file was not preserved: %v", id, statErr)
		}
		if status := b.Local.Status(id); status.State != "failed" || status.PID != 0 {
			t.Fatalf("Pod %q process status = %+v", id, status)
		}
		pod, err := b.Store.Load(id)
		if err != nil {
			t.Fatal(err)
		}
		summary := b.summary(pod)
		if summary.Local == nil || summary.Local.Process.State != "failed" || !strings.Contains(summary.Local.Health.Message, "recovery failed") {
			t.Fatalf("Pod %q summary = %+v", id, summary)
		}
	}
}

func TestLifecycleMutationsRejectUnverifiedLivePID(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	b := &PodBridge{
		Paths:  paths,
		Store:  appconfig.Store{Paths: paths},
		Health: endpointhealth.New(),
		Local:  localserver.New(),
		WebUI:  webui.New(fstest.MapFS{}),
	}
	defer b.WebUI.Shutdown()
	created, err := b.CreatePod(context.Background(), PodInput{Version: 1, ID: "unverified", Name: "Unverified", LocalServer: &LocalServerInput{Port: 0}})
	if err != nil {
		t.Fatal(err)
	}
	pidPath := filepath.Join(paths.PodsDir, created.ID, "workspace", localserver.PIDFile)
	if err := os.WriteFile(pidPath, fmt.Appendf(nil, "%d\n", os.Getpid()), 0o600); err != nil {
		t.Fatal(err)
	}
	operations := map[string]func(context.Context) error{
		"delete": func(ctx context.Context) error { return b.DeletePod(ctx, created.ID) },
		"restart": func(ctx context.Context) error {
			_, err := b.RestartLocal(ctx, created.ID)
			return err
		},
		"start": func(ctx context.Context) error {
			_, err := b.StartLocal(ctx, created.ID)
			return err
		},
		"stop": func(ctx context.Context) error {
			_, err := b.StopLocal(ctx, created.ID)
			return err
		},
		"update": func(ctx context.Context) error {
			_, err := b.UpdatePod(ctx, PodInput{Version: 1, ID: created.ID, Name: "Changed", LocalServer: &LocalServerInput{Port: created.Local.Port}})
			return err
		},
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			if err := operation(ctx); err == nil || !strings.Contains(err.Error(), "verify local server") {
				t.Fatalf("operation error = %v", err)
			}
		})
	}
	if _, err := os.Stat(pidPath); err != nil {
		t.Fatalf("PID file was not preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.PodsDir, created.ID)); err != nil {
		t.Fatalf("Pod directory was not preserved: %v", err)
	}
}

func TestDeleteInvalidPodRejectsPersistedPID(t *testing.T) {
	paths := appconfig.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	podDir := filepath.Join(paths.PodsDir, "invalid-live")
	workspace := filepath.Join(podDir, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(podDir, appconfig.PodManifestFile), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	pidPath := filepath.Join(workspace, localserver.PIDFile)
	if err := os.WriteFile(pidPath, fmt.Appendf(nil, "%d\n", os.Getpid()), 0o600); err != nil {
		t.Fatal(err)
	}
	b := &PodBridge{Paths: paths, Store: appconfig.Store{Paths: paths}}
	if err := b.DeletePod(context.Background(), "invalid-live"); err == nil || !strings.Contains(err.Error(), "requires verification") {
		t.Fatalf("DeletePod() error = %v", err)
	}
	if _, err := os.Stat(pidPath); err != nil {
		t.Fatalf("PID file was not preserved: %v", err)
	}
}

func waitForInitializationState(t *testing.T, store appconfig.Store, id, want string) *appconfig.PodInitialization {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status, err := store.Initialization(id)
		if err != nil {
			t.Fatal(err)
		}
		if want == "" && status == nil {
			return nil
		}
		if status != nil && status.State == want {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Pod %q initialization did not reach %q", id, want)
	return nil
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
