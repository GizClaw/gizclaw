// Package admissiontest runs the same admission documents against in-process
// and Docker Servers. Only administration differs between the two hosts.
package admissiontest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

// Host supplies real administrative operations, either Admin HTTP or the CLI.
// Run executes documents using one SDK's native giztest runner.
type Host struct {
	// Public APIs use Edge ingress or an authenticated Peer HTTP transport.
	PublicEndpoint  string
	PublicTransport http.RoundTripper
	Endpoint        string
	Documents       string
	Run             func(context.Context, string, string) ([]byte, error)
	CreateProfile   func(context.Context, adminhttp.RuntimeProfileUpsert) error
	CreateToken     func(context.Context, adminhttp.RegistrationTokenUpsert) error
	GetToken        func(context.Context, string) (apitypes.RegistrationToken, error)
	Block           func(context.Context, string) error
	Approve         func(context.Context, string) error
	GetPeer         func(context.Context, string) (apitypes.Registration, error)
	Runtime         func(context.Context, string) (apitypes.Runtime, error)
}

// Run verifies token lifecycle, structured refusals and online block/approve.
// Each invocation owns fresh token IDs so languages cannot consume each other's slots.
func Run(t *testing.T, host Host) {
	t.Helper()
	ctx := t.Context()
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	prefix := "admission-" + key.Public.ShortString()
	if err := host.CreateProfile(ctx, adminhttp.RuntimeProfileUpsert{Id: prefix, Spec: apitypes.RuntimeProfileSpec{AppConfig: new(apitypes.RuntimeProfileAppConfig{"admission.marker": "accepted"})}}); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name    string
		enabled *bool
		expires *time.Time
		limit   *int64
	}{
		{name: "valid"}, {name: "one-slot", limit: new(int64(1))}, {name: "disabled", enabled: new(false)}, {name: "expired", expires: new(time.Now().Add(-time.Hour))},
	} {
		id := prefix + "-" + fixture.name
		if err := host.CreateToken(ctx, adminhttp.RegistrationTokenUpsert{Id: id, Token: id, RuntimeProfileId: prefix, Enabled: fixture.enabled, ExpiresAt: fixture.expires, MaxActivations: fixture.limit}); err != nil {
			t.Fatal(err)
		}
	}
	for _, scenario := range []struct {
		name, file, token                      string
		accepted, block, approve, disconnected bool
	}{
		{name: "valid", file: "valid", token: "valid", accepted: true},
		{name: "last-slot and credential-free reconnect", file: "valid", token: "one-slot", accepted: true},
		{name: "missing", file: "missing", token: "valid"},
		{name: "wrong", file: "wrong", token: "valid"},
		{name: "unknown type", file: "unknown-type", token: "valid"},
		{name: "unknown version", file: "unknown-version", token: "valid"},
		{name: "legacy type", file: "legacy-type", token: "valid"},
		{name: "disabled", file: "valid", token: "disabled"},
		{name: "expired", file: "valid", token: "expired"},
		{name: "exhausted new device", file: "valid", token: "one-slot"},
		{name: "block disconnects existing connection", file: "disconnected", token: "valid", block: true, disconnected: true},
		{name: "block online and reject reconnect", file: "blocked", token: "valid", block: true},
		{name: "block then approve and reconnect", file: "blocked", token: "valid", block: true, approve: true, accepted: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			target, err := url.Parse(host.Endpoint)
			if err != nil {
				t.Fatal(err)
			}
			proxy := httputil.NewSingleHostReverseProxy(target)
			publicTarget := target
			if host.PublicEndpoint != "" {
				publicTarget, err = url.Parse(host.PublicEndpoint)
				if err != nil {
					t.Fatal(err)
				}
			}
			proxy.Transport = roundTripper(func(request *http.Request) (*http.Response, error) {
				transport := http.DefaultTransport
				if strings.HasPrefix(request.URL.Path, "/gizclaw/v1/") {
					request.URL.Scheme, request.URL.Host = publicTarget.Scheme, publicTarget.Host
					if host.PublicTransport != nil {
						transport = host.PublicTransport
					}
				}
				response, err := transport.RoundTrip(request)
				if response != nil {
					response.Request = request
				}
				return response, err
			})
			var denied, offers, hooks, offersBeforeBlock atomic.Int64
			var peer atomic.Pointer[string]
			var failure atomic.Pointer[error]
			recordError := func(err error) { failure.CompareAndSwap(nil, &err) }
			proxy.ModifyResponse = func(resp *http.Response) error {
				ctx := resp.Request.Context()
				if resp.Request.URL.Path == gizwebrtc.SignalingPath {
					pk := resp.Request.Header.Get("X-Giznet-Public-Key")
					if previous := peer.Load(); previous == nil {
						peer.Store(&pk)
					} else if *previous != pk {
						return fmt.Errorf("reconnect changed Peer identity")
					}
					offers.Add(1)
					if resp.StatusCode == http.StatusForbidden {
						data, err := io.ReadAll(resp.Body)
						_ = resp.Body.Close()
						if err != nil {
							return err
						}
						resp.Body = io.NopCloser(bytes.NewReader(data))
						var body struct {
							Error string `json:"error"`
						}
						if err := json.Unmarshal(data, &body); err != nil {
							return err
						}
						if body.Error == "peer_forbidden" {
							denied.Add(1)
						}
					}
				}
				if scenario.block && resp.Request.URL.Path == "/gizclaw/v1/api-keys/self" && resp.StatusCode == http.StatusOK {
					// The real response proves registration, RPC and Public HTTP work before
					// block. Hold it while administering the still-connected device.
					pk := peer.Load()
					if pk == nil {
						return fmt.Errorf("HTTP checkpoint preceded signaling")
					}
					if err := checkRuntime(ctx, host, *pk, true); err != nil {
						return err
					}
					offersBeforeBlock.Store(offers.Load())
					if err := host.Block(ctx, *pk); err != nil {
						return err
					}
					registration, err := host.GetPeer(ctx, *pk)
					if err != nil {
						return err
					}
					if registration.Status != apitypes.PeerRegistrationStatusBlocked {
						return fmt.Errorf("block did not persist blocked status")
					}
					if err := checkRuntime(ctx, host, *pk, false); err != nil {
						return err
					}
					if scenario.approve {
						if err := host.Approve(ctx, *pk); err != nil {
							return err
						}
						registration, err = host.GetPeer(ctx, *pk)
						if err != nil {
							return err
						}
						if registration.Status != apitypes.PeerRegistrationStatusActive {
							return fmt.Errorf("approve did not persist active status")
						}
					}
					hooks.Add(1)
				}
				return nil
			}
			proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
				recordError(err)
				http.Error(w, "admission observer failed", http.StatusBadGateway)
			}
			observer := httptest.NewServer(proxy)
			defer observer.Close()
			t.Setenv("GIZCLAW_TEST_ENDPOINT", observer.URL)
			t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", prefix+"-"+scenario.token)
			reportPath := filepath.Join(t.TempDir(), "report.json")
			output, runErr := host.Run(ctx, filepath.Join(host.Documents, scenario.file+".giztest.yaml"), reportPath)
			data, readErr := os.ReadFile(reportPath)
			if readErr != nil {
				t.Fatalf("runner report: %v; runner=%v\n%s", readErr, runErr, output)
			}
			if err := failure.Load(); err != nil {
				t.Fatalf("observer: %v", *err)
			}
			wantDenied := int64(1)
			wantStatus := "failed"
			if scenario.disconnected {
				wantDenied = 0
			}
			if scenario.accepted {
				wantDenied = 0
				wantStatus = "passed"
				if runErr != nil {
					t.Fatalf("runner: %v\n%s\n%s", runErr, output, data)
				}
			} else if runErr == nil {
				t.Fatal("negative document unexpectedly passed")
			}
			if denied.Load() != wantDenied {
				t.Fatalf("Server peer_forbidden responses = %d, want %d; runner=%v\n%s", denied.Load(), wantDenied, runErr, output)
			}

			var report struct {
				Status string `json:"status"`
				Tasks  []struct {
					Status string `json:"status"`
					Steps  []struct {
						ID     string `json:"id"`
						Error  string `json:"error"`
						Status string `json:"status"`
					} `json:"steps"`
				} `json:"tasks"`
			}
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			if report.Status != wantStatus || len(report.Tasks) != 1 || report.Tasks[0].Status != wantStatus {
				t.Fatalf("unexpected runner report: %s", data)
			}
			if !scenario.accepted && !scenario.block && (offers.Load() != 1 || len(report.Tasks[0].Steps) != 0) {
				t.Fatalf("expected refusal during the first handshake: offers=%d report=%s", offers.Load(), data)
			}
			if scenario.accepted && !scenario.block {
				steps := report.Tasks[0].Steps
				expected := []string{"read_profile", "repeat_register", "reconnect_without_credential", "register_after_reconnect", "read_after_reconnect"}
				// Go/C can retry a failed ICE establishment with a fresh offer.
				// The report proves both connections worked; refusals never retry.
				if offers.Load() < 2 || len(steps) != len(expected) {
					t.Fatalf("expected initial admission and same-key reconnect: offers=%d report=%s", offers.Load(), data)
				}
				for i, id := range expected {
					if steps[i].ID != id || steps[i].Status != "passed" {
						t.Fatalf("admitted step %s must pass: %s", id, data)
					}
				}
			}
			if scenario.block {
				wantAfterBlock := int64(1)
				if scenario.disconnected {
					wantAfterBlock = 0
				}
				beforeBlock := offersBeforeBlock.Load()
				afterBlock := offers.Load() - beforeBlock
				if beforeBlock < 1 || hooks.Load() != 1 || (!scenario.accepted && afterBlock != wantAfterBlock) || (scenario.accepted && afterBlock < 1) {
					t.Fatalf("block sequence: offers before=%d after=%d hooks=%d; runner=%v\n%s\n%s", beforeBlock, afterBlock, hooks.Load(), runErr, output, data)
				}
				steps := report.Tasks[0].Steps
				expected := []string{"read_profile", "create_key", "online_checkpoint", "reconnect_after_admin"}
				if scenario.disconnected {
					expected[3] = "connection_after_block"
				}
				if scenario.accepted {
					expected = append(expected, "register_after_reconnect", "read_after_reconnect")
				}
				if len(steps) != len(expected) {
					t.Fatalf("block steps: %s", data)
				}
				for i, id := range expected {
					status := "passed"
					if !scenario.accepted && i == 3 {
						status = "failed"
					}
					if steps[i].ID != id || steps[i].Status != status {
						t.Fatalf("block step %s must be %s: %s", id, status, data)
					}
				}
				if scenario.disconnected {
					message := strings.ToLower(steps[3].Error)
					if strings.Contains(message, "timeout") || strings.Contains(message, "deadline") || !(strings.Contains(message, "closed") || strings.Contains(message, "not connected") || strings.Contains(message, "disconnected") || strings.Contains(message, "not open")) {
						t.Fatalf("expected a connection-closed RPC error, got %q", steps[3].Error)
					}
				}
			}
			t.Logf("report=%s; Server peer_forbidden=%d; signaling requests=%d; admin checkpoints=%d", wantStatus, denied.Load(), offers.Load(), hooks.Load())
		})
	}

	for _, token := range []struct {
		name        string
		activations int64
	}{{"valid", 4}, {"one-slot", 1}, {"disabled", 0}, {"expired", 0}} {
		item, err := host.GetToken(ctx, prefix+"-"+token.name)
		if err != nil || item.ActivationCount != token.activations {
			t.Errorf("%s activation count=%d, want %d; error=%v", token.name, item.ActivationCount, token.activations, err)
		}
	}
}

func checkRuntime(ctx context.Context, host Host, publicKey string, online bool) error {
	runtime, err := host.Runtime(ctx, publicKey)
	if err != nil {
		return err
	}
	if runtime.Online != online {
		return fmt.Errorf("Peer online=%v, want %v", runtime.Online, online)
	}
	return nil
}

// roundTripper selects the deployed Public HTTP surface without changing payloads.
type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
