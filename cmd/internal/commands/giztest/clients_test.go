package giztest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestFetchServerInfo(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"public_key":%q,"protocol":"gizclaw-webrtc","signaling_path":"/signal"}`, key.Public.String())
	}))
	defer server.Close()
	info, err := fetchServerInfo(context.Background(), strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	if info.publicKey != key.Public || !strings.HasSuffix(info.signalingURL, "/signal") {
		t.Fatalf("server info = %#v", info)
	}
}

func TestFetchServerInfoRejectsURLAccessPoint(t *testing.T) {
	if _, err := fetchServerInfo(context.Background(), "https://example.test"); err == nil {
		t.Fatal("URL access point accepted")
	}
}

func TestFetchServerInfoRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat(" ", maxServerInfoBytes+1)))
	}))
	defer server.Close()
	_, err := fetchServerInfo(context.Background(), strings.TrimPrefix(server.URL, "http://"))
	if err == nil || !strings.Contains(err.Error(), "server-info exceeds") {
		t.Fatalf("error = %v", err)
	}
}
