package peerquota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/quota"
)

func TestHTTPPoolReusesConnectionsAcrossPeersAndHookFailures(t *testing.T) {
	for _, protocol := range []string{"http1", "http2"} {
		t.Run(protocol, func(t *testing.T) {
			var requests, opened atomic.Int32
			closed := make(chan struct{}, 8)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				index := requests.Add(1)
				var report quota.QuotaRequest
				if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
					t.Error(err)
					return
				}
				peer := giznet.PublicKey{1}
				credential := "first-key"
				if index == 4 {
					peer = giznet.PublicKey{2}
					credential = "second-key"
				}
				if report.PeerPublicKey != peer.String() || r.Header.Get("Authorization") != "Bearer "+credential || r.URL.Path != "/hook/quota" || r.URL.Query().Get("version") != "1" {
					t.Error("pooled request lost its Peer, credential, or exact endpoint")
				}
				if protocol == "http2" && r.ProtoMajor != 2 {
					t.Errorf("protocol = %s, want HTTP/2", r.Proto)
				}
				switch index {
				case 2:
					http.Error(w, "temporary failure", http.StatusServiceUnavailable)
				case 3:
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"valid_until":`)
				default:
					sendDecision(t, w, quota.QuotaResponse{ValidUntil: time.Now().Add(time.Minute)})
				}
			}))
			server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				switch state {
				case http.StateNew:
					opened.Add(1)
				case http.StateClosed:
					closed <- struct{}{}
				}
			}
			if protocol == "http2" {
				server.EnableHTTP2 = true
				server.StartTLS()
			} else {
				server.Start()
			}
			defer server.Close()
			service := New(testReporter{})
			if protocol == "http2" {
				service.client.Transport.(*http.Transport).TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
			}
			t.Cleanup(func() { _ = service.Close(t.Context()) })
			policy := apitypes.RuntimeProfileQuotaCustom{Type: apitypes.RuntimeProfileQuotaCustomTypeCustom, Endpoint: server.URL + "/hook/quota?version=1", ApiKey: "first-key"}
			first := &entry{service: service, key: entryKey{peer: giznet.PublicKey{1}, policy: policy}, policy: policy}
			policy.ApiKey = "second-key"
			second := &entry{service: service, key: entryKey{peer: giznet.PublicKey{2}, policy: policy}, policy: policy}
			reused := make(chan bool, 8)
			ctx := httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused <- info.Reused }})
			for index, worker := range []*entry{first, first, first, second, first} {
				_, err := worker.fetch(ctx)
				if index == 1 || index == 2 {
					if err == nil {
						t.Fatal("hook failure unexpectedly succeeded")
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if got := <-reused; got != (index > 0) {
					t.Fatalf("request %d reused=%v, want %v", index+1, got, index > 0)
				}
			}
			if opened.Load() != 1 {
				t.Fatalf("opened %d connections for 5 quota requests", opened.Load())
			}
			closing, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			if err := service.Close(closing); err != nil {
				t.Fatal(err)
			}
			select {
			case <-closed:
			case <-closing.Done():
				t.Fatal("service shutdown retained its idle HTTP connection")
			}
		})
	}
}

func TestPooledHookDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	service := New(testReporter{})
	defer func() { _ = service.Close(t.Context()) }()
	policy := apitypes.RuntimeProfileQuotaCustom{Type: apitypes.RuntimeProfileQuotaCustomTypeCustom, Endpoint: server.URL, ApiKey: "private-hook-key"}
	worker := &entry{service: service, key: entryKey{peer: giznet.PublicKey{1}, policy: policy}, policy: policy}
	_, err := worker.fetch(t.Context())
	if err == nil || err.Error() != "quota HTTP status 307" || errors.Is(err, ErrDenied) || redirected.Load() != 0 {
		t.Fatalf("redirect handling: err=%v forwarded requests=%d", err, redirected.Load())
	}
}
