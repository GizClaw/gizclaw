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

func TestLaunchURLReusesPortAndRetainsPerLaunchRuntimeTokens(t *testing.T) {
	manager := New(fstest.MapFS{"admin.html": {Data: []byte("admin")}, "play.html": {Data: []byte("play")}})
	defer manager.Shutdown()
	firstRuntime := testRuntime(t)
	secondRuntime := testRuntime(t)
	first, err := manager.LaunchURL("pod-a", "admin", firstRuntime)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.LaunchURL("pod-a", "admin", secondRuntime)
	if err != nil {
		t.Fatal(err)
	}
	firstURL, _ := url.Parse(first)
	secondURL, _ := url.Parse(second)
	if firstURL.Host != secondURL.Host {
		t.Fatalf("ports differ: %s / %s", firstURL.Host, secondURL.Host)
	}
	play, err := manager.LaunchURL("pod-a", "play", firstRuntime)
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
	if csp := assetResponse.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "media-src 'self' blob:") {
		t.Fatalf("Content-Security-Policy = %q, want blob media source", csp)
	}
	blocked, err := http.Get("http://" + firstURL.Host + "/play.html")
	if err != nil {
		t.Fatal(err)
	}
	_ = blocked.Body.Close()
	if blocked.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-surface HTML status = %d", blocked.StatusCode)
	}
	if strings.Contains(first, firstRuntime.PrivateKeyBase64) {
		t.Fatal("launch URL contains private key")
	}

	if firstURL.Fragment != "" {
		t.Fatalf("launch URL contains a fragment: %s", firstURL.Fragment)
	}
	token := firstURL.Query().Get("token")
	if token == "" {
		t.Fatal("launch URL query is missing its token")
	}
	secondToken := secondURL.Query().Get("token")
	if token == secondToken {
		t.Fatal("separate launches share a runtime token")
	}
	if token == playURL.Query().Get("token") {
		t.Fatal("Admin and Play share a runtime token")
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
	if response.StatusCode != http.StatusOK || !bytes.Contains(data, []byte(firstRuntime.PrivateKeyBase64)) {
		t.Fatalf("handoff = %d %s", response.StatusCode, data)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header.Get("Cache-Control"))
	}
	if len(response.Cookies()) != 0 {
		t.Fatal("runtime handoff set an unexpected cookie")
	}

	request, _ = http.NewRequest(http.MethodPost, "http://"+firstURL.Host+"/__gizclaw/runtime", bytes.NewReader(body))
	request.Header.Set("Origin", "http://"+firstURL.Host)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.Contains(data, []byte(firstRuntime.PrivateKeyBase64)) {
		t.Fatalf("reused runtime token = %d %s", response.StatusCode, data)
	}

	body, _ = json.Marshal(map[string]string{"token": secondToken})
	request, _ = http.NewRequest(http.MethodPost, "http://"+secondURL.Host+"/__gizclaw/runtime", bytes.NewReader(body))
	request.Header.Set("Origin", "http://"+secondURL.Host)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.Contains(data, []byte(secondRuntime.PrivateKeyBase64)) {
		t.Fatalf("second runtime token = %d %s", response.StatusCode, data)
	}
}

func TestRuntimeTokenRejectsUnknownValue(t *testing.T) {
	manager := New(fstest.MapFS{"admin.html": {Data: []byte("admin")}})
	defer manager.Shutdown()
	launch, err := manager.LaunchURL("pod-a", "admin", testRuntime(t))
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(launch)

	body, _ := json.Marshal(map[string]string{"token": "unknown"})
	request, _ := http.NewRequest(http.MethodPost, "http://"+parsed.Host+"/__gizclaw/runtime", bytes.NewReader(body))
	request.Header.Set("Origin", "http://"+parsed.Host)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown token status = %d", response.StatusCode)
	}
}

func TestClosePodClearsBrowserRuntime(t *testing.T) {
	manager := New(fstest.MapFS{"admin.html": {Data: []byte("admin")}})
	runtime := testRuntime(t)
	launch, err := manager.LaunchURL("pod-a", "admin", runtime)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(launch)
	body, _ := json.Marshal(map[string]string{"token": parsed.Query().Get("token")})
	request, _ := http.NewRequest(http.MethodPost, "http://"+parsed.Host+"/__gizclaw/runtime", bytes.NewReader(body))
	request.Header.Set("Origin", "http://"+parsed.Host)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()

	manager.mu.Lock()
	server := manager.servers["pod-a:admin"]
	manager.mu.Unlock()
	manager.ClosePod("pod-a")
	server.mu.Lock()
	handoffs := len(server.handoffs)
	server.mu.Unlock()
	if handoffs != 0 {
		t.Fatal("closed listener retained its runtime state")
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
	body, _ := json.Marshal(map[string]string{"token": parsed.Query().Get("token")})
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
