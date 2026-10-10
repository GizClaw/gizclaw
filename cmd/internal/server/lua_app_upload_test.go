package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// luaTransportSink observes protocol bytes only. It deliberately does not
// implement GizOS package extraction, storage or installation acceptance.
type luaTransportSink struct {
	first  chan struct{}
	closed chan error
	digest hash.Hash
	bytes  int
	chunks int
	finish bool
}

func (s *luaTransportSink) Write(_ context.Context, data []byte) error {
	if s.chunks == 0 {
		close(s.first)
	}
	s.chunks++
	s.bytes += len(data)
	_, err := s.digest.Write(data)
	return err
}

func (s *luaTransportSink) Finish(context.Context) (*rpcpb.ClientLuaAppInstallResponse, error) {
	s.finish = true
	return &rpcpb.ClientLuaAppInstallResponse{App: &rpcpb.LuaAppInfo{AppId: "transport-probe", Version: "1.0.0"}}, nil
}

func (s *luaTransportSink) Close(err error) { s.closed <- err }

func TestLuaAppUploadServerEdgeWebRTC(t *testing.T) {
	withPeerControlServer(t, 32, func(ctx context.Context, srv *gizclaw.Server, _ *gizcli.Client, _, edgeURL string) {
		// The command ingress intentionally denies public APIs. Exercise the
		// authoritative package handler directly without changing that policy.
		publicHTTP := httptest.NewServer(srv)
		t.Cleanup(publicHTTP.Close)
		key, err := giznet.GenerateKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		info, err := gizcli.FetchServerInfo(ctx, edgeURL)
		if err != nil {
			t.Fatal(err)
		}
		credential, err := gizcli.RegistrationTokenCredential("local-peer-sync-test-token")
		if err != nil {
			t.Fatal(err)
		}
		device := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			return gizwebrtc.Dial(ctx, key, info.TransportPublicKey, gizwebrtc.DialConfig{SignalingURL: info.SignalingURL, SecurityPolicy: policy, Credential: credential})
		}}
		sessions := make(chan *luaTransportSink, 8)
		var begins atomic.Int32
		if err := device.HandleDeviceControl(gizcli.DeviceControlHandlers{InstallLuaApp: func(_ context.Context, _ *rpcpb.ClientLuaAppInstallStreamRequest) (gizcli.LuaAppInstallSession, error) {
			sink := &luaTransportSink{first: make(chan struct{}), closed: make(chan error, 1), digest: sha256.New()}
			begins.Add(1)
			sessions <- sink
			return sink, nil
		}}); err != nil {
			t.Fatal(err)
		}
		if err := device.Dial(info.PublicKey, edgeURL); err != nil {
			t.Fatal(err)
		}
		served := make(chan error, 1)
		go func() { served <- device.Serve() }()
		t.Cleanup(func() {
			_ = device.Close()
			select {
			case <-served:
			case <-time.After(5 * time.Second):
				t.Error("device did not stop")
			}
		})
		if _, err := device.Register(ctx, "register", "local-peer-sync-test-token"); err != nil {
			t.Fatal(err)
		}
		apiKey, err := device.CreateAPIKey(ctx, "api-key", rpcapi.APIKeyCreateRequest{DisplayName: "Lua transport probe"})
		if err != nil {
			t.Fatal(err)
		}
		data := bytes.Repeat([]byte{0x78, 0x9c, 0xaa, 0xbb}, rpcapi.LuaAppArchiveMaxBytes/4)
		sum := sha256.Sum256(data)
		digest := hex.EncodeToString(sum[:])
		path := fmt.Sprintf("/gizclaw/v1/device/lua-app/install?content_length=%d&sha256=%s", len(data), digest)
		for _, lane := range []struct {
			name, endpoint string
			client         *http.Client
		}{
			{"server", publicHTTP.URL, http.DefaultClient},
			{"edge", edgeURL, http.DefaultClient},
			{"peer_http", "http://gizclaw", device.HTTPClient(gizcli.ServicePeerHTTP)},
		} {
			t.Run(lane.name, func(t *testing.T) {
				requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
				defer cancel()
				body, producer := io.Pipe()
				defer body.Close()
				defer producer.Close()
				stop := context.AfterFunc(requestCtx, func() { _ = producer.CloseWithError(requestCtx.Err()) })
				defer stop()
				request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, lane.endpoint+path, body)
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Authorization", "Bearer "+apiKey.APIKey)
				request.Header.Set("Content-Type", "application/octet-stream")
				type outcome struct {
					response *http.Response
					err      error
				}
				done := make(chan outcome, 1)
				go func() { response, err := lane.client.Do(request); done <- outcome{response, err} }()
				if _, err := producer.Write(data[:32768]); err != nil {
					t.Fatal(err)
				}
				var sink *luaTransportSink
				select {
				case sink = <-sessions:
				case <-requestCtx.Done():
					t.Fatal("metadata did not reach the WebRTC device")
				}
				select {
				case <-sink.first:
				case <-requestCtx.Done():
					t.Fatal("first chunk was buffered until the complete upload")
				}
				if _, err := producer.Write(data[32768:]); err != nil {
					t.Fatal(err)
				}
				_ = producer.Close()
				result := <-done
				if result.err != nil {
					t.Fatal(result.err)
				}
				defer result.response.Body.Close()
				var response struct {
					App struct {
						AppID string `json:"app_id"`
					} `json:"app"`
				}
				if err := json.NewDecoder(result.response.Body).Decode(&response); err != nil {
					t.Fatal(err)
				}
				if result.response.StatusCode != http.StatusOK || response.App.AppID != "transport-probe" {
					t.Fatalf("response=%d %+v", result.response.StatusCode, response)
				}
				select {
				case err := <-sink.closed:
					if err != nil || !sink.finish || sink.bytes != len(data) || sink.chunks < 8 || hex.EncodeToString(sink.digest.Sum(nil)) != digest {
						t.Fatalf("incomplete transport receipt: bytes=%d chunks=%d finish=%v close=%v", sink.bytes, sink.chunks, sink.finish, err)
					}
				case <-requestCtx.Done():
					t.Fatal("device session did not close")
				}
			})
		}
		t.Run("over_limit", func(t *testing.T) {
			before := begins.Load()
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, edgeURL+fmt.Sprintf("/gizclaw/v1/device/lua-app/install?content_length=%d&sha256=%s", len(data)+1, digest), strings.NewReader("unread"))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+apiKey.APIKey)
			request.Header.Set("Content-Type", "application/octet-stream")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusRequestEntityTooLarge || !bytes.Contains(body, []byte("LUA_APP_PACKAGE_TOO_LARGE")) || begins.Load() != before {
				t.Fatalf("oversize=%d %s, begins=%d", response.StatusCode, body, begins.Load()-before)
			}
		})
	})
}
