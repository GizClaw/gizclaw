package peerquota

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/quota"
)

type testReporter struct{}

func (testReporter) QuotaReport(_ context.Context, peer giznet.PublicKey) (quota.QuotaRequest, error) {
	return quota.QuotaRequest{PeerPublicKey: peer.String(), Identifiers: &apitypes.DeviceIdentifiers{Sn: new("device-sn"), Imeis: new([]apitypes.PeerIMEI{{Tac: "12345678", Serial: "123456"}}), Labels: new([]apitypes.PeerLabel{{Key: "hardware", Value: "test"}})}, Usage: []quota.QuotaUsage{{ModelId: "billing-model", Hour: time.Now().UTC().Truncate(time.Hour), Quantity: 12}}}, nil
}

func testService(t *testing.T, handler http.HandlerFunc) (*Service, apitypes.RuntimeProfileQuota) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
			return
		}
		request.Body.Close()
		request.Body = io.NopCloser(bytes.NewReader(body))
		handler(w, request)
	}))
	service := New(testReporter{})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := service.Close(ctx); err != nil {
			t.Error(err)
		}
		server.Close()
	})
	return service, apitypes.RuntimeProfileQuota{Endpoint: server.URL + "/custom/quota", ApiKey: "test-key"}
}

func sendDecision(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("write decision: %v", err)
	}
}

func TestQuotaResponseStatesAndCompleteReport(t *testing.T) {
	for _, name := range []string{"omitted", "null", "allowed", "denied", "missing-validity", "null-validity", "elapsed-validity", "malformed", "http-error"} {
		t.Run(name, func(t *testing.T) {
			svc, policy := testService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/custom/quota" || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("endpoint or authentication changed")
				}
				var report quota.QuotaRequest
				if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
					t.Error(err)
				}
				if report.Identifiers == nil || report.Identifiers.Sn == nil || *report.Identifiers.Sn != "device-sn" || len(*report.Identifiers.Imeis) != 1 || len(*report.Identifiers.Labels) != 1 || len(report.Usage) != 1 || report.Usage[0].Quantity != 12 || report.Usage[0].ModelId != "billing-model" {
					t.Errorf("incomplete report: %+v", report)
				}
				body := map[string]any{"valid_until": time.Now().Add(time.Minute)}
				switch name {
				case "null":
					body["expires_at"] = nil
				case "allowed":
					body["expires_at"] = time.Now().Add(time.Minute)
				case "denied":
					body["expires_at"] = time.Now().Add(-time.Hour)
				case "missing-validity":
					delete(body, "valid_until")
				case "null-validity":
					body["valid_until"] = nil
				case "elapsed-validity":
					body["valid_until"] = time.Now().Add(-time.Hour)
				case "malformed":
					body["expires_at"] = "not-a-date"
				case "http-error":
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				sendDecision(t, w, body)
			})
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			call, release, err := svc.Authorize(ctx, giznet.PublicKey{1}, policy)
			if name == "denied" {
				if !errors.Is(err, ErrDenied) {
					t.Fatalf("error = %v", err)
				}
			} else if name == "omitted" || name == "null" || name == "allowed" {
				if err != nil {
					t.Fatal(err)
				}
				if call.Err() != nil {
					t.Fatal(call.Err())
				}
				release()
			} else if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestUsableExpiryCancelsDuringBlockedRefresh(t *testing.T) {
	var requests atomic.Int32
	refreshStarted := make(chan struct{})
	svc, policy := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			sendDecision(t, w, quota.QuotaResponse{ExpiresAt: new(time.Now().Add(200 * time.Millisecond)), ValidUntil: time.Now().Add(300 * time.Millisecond)})
			return
		}
		close(refreshStarted)
		<-r.Context().Done()
	})
	call, release, err := svc.Authorize(t.Context(), giznet.PublicKey{2}, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start before expiry")
	}
	select {
	case <-call.Done():
	case <-time.After(time.Second):
		t.Fatal("blocked HTTP delayed quota expiry")
	}
	if !errors.Is(context.Cause(call), ErrDenied) {
		t.Fatalf("cause = %v", context.Cause(call))
	}
}

func TestUnlimitedGrantStopsWhenItsResultExpires(t *testing.T) {
	var requests atomic.Int32
	svc, policy := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			sendDecision(t, w, quota.QuotaResponse{ValidUntil: time.Now().Add(200 * time.Millisecond)})
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	call, release, err := svc.Authorize(t.Context(), giznet.PublicKey{3}, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	select {
	case <-call.Done():
	case <-time.After(time.Second):
		t.Fatal("stale unlimited authorization remained usable")
	}
	if !errors.Is(context.Cause(call), ErrUnavailable) {
		t.Fatalf("cause = %v", context.Cause(call))
	}
}

func TestRefreshExtendsActiveGrantAndDeniedGrantRecovers(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(map[bool]string{false: "extension", true: "recovery"}[denied], func(t *testing.T) {
			var requests atomic.Int32
			refreshed := make(chan struct{})
			svc, policy := testService(t, func(w http.ResponseWriter, r *http.Request) {
				if requests.Add(1) == 1 {
					expiry := time.Now().Add(200 * time.Millisecond)
					if denied {
						expiry = time.Now().Add(-time.Hour)
					}
					sendDecision(t, w, quota.QuotaResponse{ExpiresAt: &expiry, ValidUntil: time.Now().Add(150 * time.Millisecond)})
					return
				}
				sendDecision(t, w, quota.QuotaResponse{ExpiresAt: new(time.Now().Add(time.Minute)), ValidUntil: time.Now().Add(time.Minute)})
				close(refreshed)
			})
			call, release, err := svc.Authorize(t.Context(), giznet.PublicKey{4}, policy)
			if denied {
				if !errors.Is(err, ErrDenied) {
					t.Fatalf("error=%v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				defer release()
			}
			select {
			case <-refreshed:
			case <-time.After(time.Second):
				t.Fatal("refresh did not run")
			}
			if denied {
				until := time.Now().Add(time.Second)
				for {
					call, release, err = svc.Authorize(t.Context(), giznet.PublicKey{4}, policy)
					if err == nil {
						break
					}
					if !errors.Is(err, ErrDenied) || time.Now().After(until) {
						t.Fatal(err)
					}
					time.Sleep(time.Millisecond)
				}
				defer release()
			}
			select {
			case <-call.Done():
				t.Fatalf("active grant was not extended: %v", context.Cause(call))
			case <-time.After(250 * time.Millisecond):
			}
		})
	}
}

func TestUnrelatedPeerProgressAndCloseJoin(t *testing.T) {
	blocked := make(chan struct{})
	svc, policy := testService(t, func(w http.ResponseWriter, r *http.Request) {
		var report quota.QuotaRequest
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			t.Error(err)
			return
		}
		if report.PeerPublicKey == (giznet.PublicKey{5}).String() {
			close(blocked)
			<-r.Context().Done()
			return
		}
		sendDecision(t, w, quota.QuotaResponse{ValidUntil: time.Now().Add(time.Minute)})
	})
	finished := make(chan error, 1)
	go func() { _, _, err := svc.Authorize(t.Context(), giznet.PublicKey{5}, policy); finished <- err }()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("blocked peer did not start")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	call, release, err := svc.Authorize(ctx, giznet.PublicKey{6}, policy)
	if err != nil {
		t.Fatalf("unrelated Peer blocked: %v", err)
	}
	defer release()
	if err := svc.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-call.Done():
	default:
		t.Fatal("close retained an active call")
	}
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("close failed to join pending acquisition")
	}
}
