package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// The fixture resolves the device's real API key owner and forwards requests
// through authenticated Admin HTTP over WebRTC. It never synthesizes a limit error.
func TestIdentifierLimitGiztestsGo(t *testing.T) {
	for _, kind := range []string{"combined", "sn", "imei"} {
		t.Run(kind, func(t *testing.T) {
			scenario := filepath.Join("..", "testdata", "identifier-limits", kind+".giztest.yaml")
			runPeerControlGiztest(t, scenario, 5, 1, func(ctx context.Context, file, report string) ([]byte, error) {
				command := giztestcmd.NewCmd()
				var output bytes.Buffer
				command.SetOut(&output)
				command.SetErr(&output)
				command.SetArgs([]string{"run", file, "--parallel", "1", "--output", report})
				err := command.ExecuteContext(ctx)
				return output.Bytes(), err
			}, func(srv *gizclaw.Server, admin *gizcli.Client) {
				if kind != "combined" {
					seed := &peer.Server{Store: kv.Prefixed(srv.PeerStore, kv.Key{"records"})}
					for i := 1; i <= peer.IdentifierIndexPeerLimit; i++ {
						sn := "giztest-shared-sn"
						identifiers := &apitypes.DeviceIdentifiers{Sn: &sn}
						if kind == "imei" {
							sn = fmt.Sprintf("giztest-seed-%d", i)
							identifiers.Imeis = &[]apitypes.PeerIMEI{{Tac: "12345678", Serial: "123456"}}
						}
						if _, err := seed.SavePeer(t.Context(), apitypes.Peer{PublicKey: giznet.PublicKey{byte(i)}.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive, Device: apitypes.DeviceInfo{Identifiers: identifiers}}); err != nil {
							t.Fatal(err)
						}
					}
				}
				keys := apikey.NewServer(srv.APIKeyStore)
				transport := admin.HTTPClient(gizcli.ServiceAdminHTTP)
				proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
					principal, err := keys.Authenticate(request.Context(), strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
					if err != nil {
						http.Error(w, "fixture owner authentication failed", http.StatusUnauthorized)
						return
					}
					path := "/peers/" + principal.Key.Owner
					if request.Method == http.MethodPost {
						path += "/@refresh"
					}
					forward := request.Clone(request.Context())
					forward.URL = &url.URL{Scheme: "http", Host: "gizclaw", Path: path}
					forward.RequestURI = ""
					response, err := transport.Do(forward)
					if err != nil {
						http.Error(w, "fixture Admin forwarding failed", http.StatusBadGateway)
						return
					}
					defer response.Body.Close()
					w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
					w.WriteHeader(response.StatusCode)
					_, _ = io.Copy(w, response.Body)
				}))
				t.Cleanup(proxy.Close)
				t.Setenv("GIZCLAW_TEST_IDENTIFIER_ADMIN_ENDPOINT", proxy.URL)
			})
		})
	}
}
