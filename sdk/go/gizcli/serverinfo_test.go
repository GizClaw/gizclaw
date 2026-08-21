package gizcli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

func serverInfoTestKeyText(t *testing.T, fill byte) string {
	t.Helper()
	var key giznet.Key
	for i := range key {
		key[i] = fill
	}
	kp, err := giznet.NewKeyPair(key)
	if err != nil {
		t.Fatalf("NewKeyPair error = %v", err)
	}
	return kp.Public.String()
}

func newServerInfoTestServer(t *testing.T, body string) (endpoint string, closeServer func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/server-info" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	return strings.TrimPrefix(server.URL, "http://"), server.Close
}

func TestFetchServerInfoDefaultsSignalingPath(t *testing.T) {
	endpoint, closeServer := newServerInfoTestServer(t, `{"protocol":"gizclaw-webrtc","public_key":"`+serverInfoTestKeyText(t, 0xab)+`"}`)
	defer closeServer()

	info, err := FetchServerInfo(context.Background(), endpoint)
	if err != nil {
		t.Fatalf("FetchServerInfo error = %v", err)
	}
	if info.SignalingURL != "http://"+endpoint+gizwebrtc.SignalingPath {
		t.Fatalf("signaling URL = %q", info.SignalingURL)
	}
	if info.TransportPublicKey != info.PublicKey {
		t.Fatal("transport key should default to authoritative key")
	}
}

func TestFetchServerInfoParsesSignalingPathAndICEServers(t *testing.T) {
	serverKey := serverInfoTestKeyText(t, 0xab)
	endpoint, closeServer := newServerInfoTestServer(t, `{
		"protocol":"gizclaw-webrtc",
		"public_key":"`+serverKey+`",
		"signaling_path":"/custom/offer",
		"ice_servers":[{"urls":["turn:turn.example:3478"],"username":"user","credential":"secret"}]
	}`)
	defer closeServer()

	info, err := FetchServerInfo(context.Background(), endpoint)
	if err != nil {
		t.Fatalf("FetchServerInfo error = %v", err)
	}
	if info.PublicKey.String() != serverKey {
		t.Fatalf("public key = %s, want %s", info.PublicKey, serverKey)
	}
	if info.SignalingURL != "http://"+endpoint+"/custom/offer" {
		t.Fatalf("signaling URL = %q", info.SignalingURL)
	}
	want := []gizwebrtc.ICEServer{{URLs: []string{"turn:turn.example:3478"}, Username: "user", Credential: "secret"}}
	if len(info.ICEServers) != 1 || info.ICEServers[0].URLs[0] != want[0].URLs[0] ||
		info.ICEServers[0].Username != want[0].Username || info.ICEServers[0].Credential != want[0].Credential {
		t.Fatalf("ICE servers = %+v, want %+v", info.ICEServers, want)
	}
}

// The wire format uses snake_case field names; this guards against struct
// definitions that drop the JSON tags (signaling_path, public_key) inside
// the transport object.
func TestFetchServerInfoSelectsGatewayTransportIdentity(t *testing.T) {
	serverKey := serverInfoTestKeyText(t, 0xab)
	transportKey := serverInfoTestKeyText(t, 0xcd)
	endpoint, closeServer := newServerInfoTestServer(t, `{
		"protocol":"gizclaw-webrtc",
		"public_key":"`+serverKey+`",
		"signaling_path":"/server/offer",
		"ice_servers":[{"urls":["turn:server.example:3478"]}],
		"transport":{
			"mode":"edge-gateway",
			"endpoint":"edge.example:9821",
			"public_key":"`+transportKey+`",
			"signaling_path":"/edge/offer"
		}
	}`)
	defer closeServer()

	info, err := FetchServerInfo(context.Background(), endpoint)
	if err != nil {
		t.Fatalf("FetchServerInfo error = %v", err)
	}
	if info.PublicKey.String() != serverKey {
		t.Fatalf("authoritative key = %s, want %s", info.PublicKey, serverKey)
	}
	if info.TransportPublicKey.String() != transportKey {
		t.Fatalf("transport key = %s, want %s", info.TransportPublicKey, transportKey)
	}
	if info.SignalingURL != "http://edge.example:9821/edge/offer" {
		t.Fatalf("signaling URL = %q", info.SignalingURL)
	}
	if info.ICEServers != nil {
		t.Fatalf("gateway inherited authoritative ICE servers: %+v", info.ICEServers)
	}
}

func TestFetchServerInfoRejectsInvalidGatewayTransport(t *testing.T) {
	serverKey := serverInfoTestKeyText(t, 0xab)
	transportKey := serverInfoTestKeyText(t, 0xcd)
	for _, transport := range []string{
		`{"mode":"future","endpoint":"edge.example:9821","public_key":"` + transportKey + `","signaling_path":"/offer"}`,
		`{"mode":"edge-gateway","endpoint":"https://edge.example","public_key":"` + transportKey + `","signaling_path":"/offer"}`,
		`{"mode":"edge-gateway","endpoint":"","public_key":"` + transportKey + `","signaling_path":"/offer"}`,
		`{"mode":"edge-gateway","endpoint":"edge.example:9821","public_key":"bad","signaling_path":"/offer"}`,
		`{"mode":"edge-gateway","endpoint":"edge.example:9821","public_key":"","signaling_path":"/offer"}`,
		`{"mode":"edge-gateway","endpoint":"edge.example:9821","public_key":"` + serverKey + `","signaling_path":"/offer"}`,
		`{"mode":"edge-gateway","endpoint":"edge.example:9821","public_key":"` + transportKey + `","signaling_path":"offer"}`,
	} {
		endpoint, closeServer := newServerInfoTestServer(t, `{"public_key":"`+serverKey+`","transport":`+transport+`}`)
		_, err := FetchServerInfo(context.Background(), endpoint)
		closeServer()
		if err == nil {
			t.Fatalf("transport %s unexpectedly accepted", transport)
		}
		if IsRetryableServerInfoError(err) {
			t.Fatalf("transport %s error should not be retryable: %v", transport, err)
		}
	}
}

func TestFetchServerInfoRejectsURLEndpoint(t *testing.T) {
	for _, endpoint := range []string{
		"",
		"   ",
		"https://example.test",
		"server.example/other-path",
		"server.example?query=1",
		"server.example#fragment",
		"user@server.example",
	} {
		if _, err := FetchServerInfo(context.Background(), endpoint); err == nil {
			t.Fatalf("endpoint %q accepted", endpoint)
		}
	}
}

func TestFetchServerInfoRejectsOversizedResponse(t *testing.T) {
	endpoint, closeServer := newServerInfoTestServer(t, strings.Repeat(" ", MaxServerInfoBytes+1))
	defer closeServer()

	_, err := FetchServerInfo(context.Background(), endpoint)
	if err == nil || !strings.Contains(err.Error(), "server-info exceeds") {
		t.Fatalf("error = %v", err)
	}
}

func TestFetchServerInfoRetryableClassification(t *testing.T) {
	_, err := FetchServerInfo(context.Background(), "127.0.0.1:1")
	if err == nil || !strings.Contains(err.Error(), "server-info fetch") {
		t.Fatalf("error = %v", err)
	}
	if !IsRetryableServerInfoError(err) {
		t.Fatalf("network error should be retryable: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "stale upstream", http.StatusBadGateway)
	}))
	defer server.Close()
	_, err = FetchServerInfo(context.Background(), strings.TrimPrefix(server.URL, "http://"))
	if err == nil || !IsRetryableServerInfoError(err) {
		t.Fatalf("5xx should be retryable, error = %v", err)
	}

	notFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer notFound.Close()
	_, err = FetchServerInfo(context.Background(), strings.TrimPrefix(notFound.URL, "http://"))
	if err == nil || IsRetryableServerInfoError(err) {
		t.Fatalf("4xx should not be retryable, error = %v", err)
	}

	invalid, closeServer := newServerInfoTestServer(t, `{"protocol":"not-gizclaw","public_key":"ignored"}`)
	defer closeServer()
	_, err = FetchServerInfo(context.Background(), invalid)
	if err == nil || IsRetryableServerInfoError(err) {
		t.Fatalf("invalid document should not be retryable, error = %v", err)
	}
}

func TestFetchServerInfoMissingPublicKey(t *testing.T) {
	endpoint, closeServer := newServerInfoTestServer(t, `{"protocol":"gizclaw-webrtc"}`)
	defer closeServer()

	_, err := FetchServerInfo(context.Background(), endpoint)
	if err == nil || !strings.Contains(err.Error(), "server-info missing public_key") {
		t.Fatalf("error = %v", err)
	}
}
