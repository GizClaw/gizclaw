//go:build gizclaw_e2e

package admission_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// The admission lane supplies an isolated real Server with the built-in
// registration-token policy. Selecting this suite without its fixture fails.
func TestGoSDKAdmission(t *testing.T) {
	endpoint, token := os.Getenv("GIZCLAW_TEST_ENDPOINT"), os.Getenv("GIZCLAW_TEST_REGISTRATION_TOKEN")
	if endpoint == "" || token == "" {
		t.Fatal("admission endpoint and token are required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	info, err := gizcli.FetchServerInfo(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	for _, accepted := range []bool{true, false} {
		value := token
		if !accepted {
			value = "wrong-sdk-admission-token"
		}
		key, err := giznet.GenerateKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		client := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			credential, err := gizcli.RegistrationTokenCredential(value)
			if err != nil {
				return nil, nil, err
			}
			return gizwebrtc.Dial(ctx, key, info.TransportPublicKey, gizwebrtc.DialConfig{SignalingURL: info.SignalingURL, ICEServers: info.ICEServers, Credential: credential, SecurityPolicy: policy})
		}}
		err = client.Dial(info.PublicKey, endpoint)
		if !accepted {
			_ = client.Close()
			if err == nil || !strings.Contains(err.Error(), "peer_forbidden") {
				t.Fatalf("wrong token rejection: %v", err)
			}
			continue
		}
		if err != nil {
			_ = client.Close()
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- client.Serve() }()
		if _, err := client.Register(ctx, "sdk-admission-register", token); err != nil {
			_ = client.Close()
			t.Fatal(err)
		}
		if _, err := client.Ping(ctx, "sdk-admission-ping"); err != nil {
			_ = client.Close()
			t.Fatal(err)
		}
		_ = client.Close()
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("client did not stop")
		}
	}
}
