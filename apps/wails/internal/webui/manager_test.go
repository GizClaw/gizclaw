package webui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestLaunchURLReusesPortAndConsumesHandoffOnce(t *testing.T) {
	manager := New(fstest.MapFS{"admin.html": {Data: []byte("admin")}, "play.html": {Data: []byte("play")}})
	defer manager.Shutdown()
	runtime := testRuntime(t)
	first, err := manager.LaunchURL("pod-a", "admin", runtime)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.LaunchURL("pod-a", "admin", runtime)
	if err != nil {
		t.Fatal(err)
	}
	firstURL, _ := url.Parse(first)
	secondURL, _ := url.Parse(second)
	if firstURL.Host != secondURL.Host {
		t.Fatalf("ports differ: %s / %s", firstURL.Host, secondURL.Host)
	}
	play, err := manager.LaunchURL("pod-a", "play", runtime)
	if err != nil {
		t.Fatal(err)
	}
	playURL, _ := url.Parse(play)
	if playURL.Host == firstURL.Host {
		t.Fatalf("Admin and Play share a listener: %s", firstURL.Host)
	}
	assetResponse, err := http.Get("http://" + playURL.Host + "/")
	if err != nil {
		t.Fatal(err)
	}
	asset, _ := io.ReadAll(assetResponse.Body)
	_ = assetResponse.Body.Close()
	if string(asset) != "play" {
		t.Fatalf("Play asset = %q", asset)
	}
	blocked, err := http.Get("http://" + firstURL.Host + "/play.html")
	if err != nil {
		t.Fatal(err)
	}
	_ = blocked.Body.Close()
	if blocked.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-surface HTML status = %d", blocked.StatusCode)
	}
	if strings.Contains(first, runtime.PrivateKeyBase64) {
		t.Fatal("launch URL contains private key")
	}

	if firstURL.RawQuery != "" {
		t.Fatalf("launch token is present in query: %s", firstURL.RawQuery)
	}
	token := strings.TrimPrefix(firstURL.Fragment, "launch=")
	if token == "" {
		t.Fatal("launch URL fragment is missing its token")
	}
	body, _ := json.Marshal(map[string]string{"token": token})
	request, _ := http.NewRequest(http.MethodPost, "http://"+firstURL.Host+"/__gizclaw/runtime", bytes.NewReader(body))
	request.Header.Set("Origin", "http://"+firstURL.Host)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.Contains(data, []byte(runtime.PrivateKeyBase64)) {
		t.Fatalf("handoff = %d %s", response.StatusCode, data)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header.Get("Cache-Control"))
	}

	request, _ = http.NewRequest(http.MethodPost, "http://"+firstURL.Host+"/__gizclaw/runtime", bytes.NewReader(body))
	request.Header.Set("Origin", "http://"+firstURL.Host)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusGone {
		t.Fatalf("second handoff status = %d", response.StatusCode)
	}
}

func TestHandoffRejectsCrossOrigin(t *testing.T) {
	manager := New(fstest.MapFS{"admin.html": {Data: []byte("admin")}})
	defer manager.Shutdown()
	launch, err := manager.LaunchURL("pod-a", "admin", testRuntime(t))
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(launch)
	body, _ := json.Marshal(map[string]string{"token": strings.TrimPrefix(parsed.Fragment, "launch=")})
	request, _ := http.NewRequest(http.MethodPost, "http://"+parsed.Host+"/__gizclaw/runtime", bytes.NewReader(body))
	request.Header.Set("Origin", "http://evil.invalid")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestLaunchURLRecreatesStoppedListener(t *testing.T) {
	manager := New(fstest.MapFS{"admin.html": {Data: []byte("admin")}})
	defer manager.Shutdown()
	_, err := manager.LaunchURL("pod-a", "admin", testRuntime(t))
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	server := manager.servers["pod-a:admin"]
	manager.mu.Unlock()
	if err := server.server.Close(); err != nil {
		t.Fatal(err)
	}
	<-server.done
	second, err := manager.LaunchURL("pod-a", "admin", testRuntime(t))
	if err != nil {
		t.Fatal(err)
	}
	secondURL, _ := url.Parse(second)
	manager.mu.Lock()
	recreated := manager.servers["pod-a:admin"]
	manager.mu.Unlock()
	if recreated == nil || recreated == server {
		t.Fatal("stopped listener was not replaced")
	}
	response, err := http.Get("http://" + secondURL.Host + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
}

func testRuntime(t *testing.T) Runtime {
	t.Helper()
	kp, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := RuntimeFromPrivateKey("Test", "", "127.0.0.1:9820", kp.Private.String())
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}
