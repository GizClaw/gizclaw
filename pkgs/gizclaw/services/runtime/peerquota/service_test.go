package peerquota

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/quota"
)

type testReporter struct{}

func (testReporter) QuotaReport(_ context.Context, peer giznet.PublicKey) (quota.QuotaRequest, error) {
	return quota.QuotaRequest{PeerPublicKey: peer.String(), Identifiers: &apitypes.DeviceIdentifiers{Sn: new("device-sn"), Imeis: new([]apitypes.PeerIMEI{{Tac: "12345678", Serial: "123456"}}), Labels: new([]apitypes.PeerLabel{{Key: "hardware", Value: "test"}})}, Usage: []quota.QuotaUsage{{ModelId: "billing-model", Hour: time.Now().UTC().Truncate(time.Hour), Quantity: 12}}}, nil
}

func testService(t *testing.T, handler http.HandlerFunc) (*Service, apitypes.RuntimeProfileQuotaCustom) {
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
	return service, apitypes.RuntimeProfileQuotaCustom{Type: apitypes.RuntimeProfileQuotaCustomTypeCustom, Endpoint: server.URL + "/custom/quota", ApiKey: "test-key"}
}

func sendDecision(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("write decision: %v", err)
	}
}

func waitQuotaDenied(t *testing.T, svc *Service, peer giznet.PublicKey, policy apitypes.RuntimeProfileQuotaCustom) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	for {
		_, release, err := svc.Authorize(ctx, peer, policy)
		if errors.Is(err, ErrDenied) {
			return
		}
		if err != nil {
			t.Fatalf("waiting for explicit denial: %v", err)
		}
		release()
		select {
		case <-ctx.Done():
			t.Fatal("explicit quota denial was not applied")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestQuotaResponseStatesAndCompleteReport(t *testing.T) {
	for _, name := range []string{"omitted", "null", "allowed", "denied", "missing-validity", "null-validity", "elapsed-validity", "malformed", "http-error"} {
		t.Run(name, func(t *testing.T) {
			respond := make(chan struct{})
			finished := make(chan struct{})
			svc, policy := testService(t, func(w http.ResponseWriter, r *http.Request) {
				defer close(finished)
				select {
				case <-respond:
				case <-r.Context().Done():
					return
				}
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
			if err != nil {
				t.Fatalf("cold authorization waited for the hook: %v", err)
			}
			defer release()
			close(respond)
			<-finished
			if name == "denied" {
				select {
				case <-call.Done():
				case <-ctx.Done():
					t.Fatal("explicit denial did not cancel the active call")
				}
				if !errors.Is(context.Cause(call), ErrDenied) {
					t.Fatalf("cause = %v", context.Cause(call))
				}
				waitQuotaDenied(t, svc, giznet.PublicKey{1}, policy)
			} else {
				select {
				case <-call.Done():
					t.Fatalf("hook response canceled allowed call: %v", context.Cause(call))
				case <-time.After(30 * time.Millisecond):
				}
			}
		})
	}
}

func TestQuotaPolicyIsolation(t *testing.T) {
	svc, original := testService(t, func(w http.ResponseWriter, r *http.Request) {
		decision := quota.QuotaResponse{ValidUntil: time.Now().Add(time.Minute)}
		if r.Header.Get("Authorization") != "Bearer rotated-key" || r.URL.Path != "/custom/quota" {
			decision.ExpiresAt = new(time.Now().Add(-time.Hour))
		}
		sendDecision(t, w, decision)
	})
	rotated := original
	rotated.ApiKey = "rotated-key"
	otherEndpoint := rotated
	otherEndpoint.Endpoint += "/denied"
	for _, tc := range []struct {
		name   string
		policy apitypes.RuntimeProfileQuotaCustom
		denied bool
	}{
		{name: "original denied", policy: original, denied: true},
		{name: "rotated allowed", policy: rotated},
		{name: "other endpoint denied", policy: otherEndpoint, denied: true},
		{name: "original still denied", policy: original, denied: true},
		{name: "rotated still allowed", policy: rotated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.denied {
				waitQuotaDenied(t, svc, giznet.PublicKey{7}, tc.policy)
				return
			}
			_, release, err := svc.Authorize(t.Context(), giznet.PublicKey{7}, tc.policy)
			if err != nil {
				t.Fatal(err)
			}
			release()
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

func TestStaleGrantSurvivesFailedRefresh(t *testing.T) {
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
		t.Fatalf("stale authorization was canceled: %v", context.Cause(call))
	case <-time.After(350 * time.Millisecond):
	}
	if requests.Load() < 2 {
		t.Fatal("background refresh was not attempted")
	}
	next, nextRelease, err := svc.Authorize(t.Context(), giznet.PublicKey{3}, policy)
	if err != nil {
		t.Fatalf("stale authorization rejected a new call: %v", err)
	}
	defer nextRelease()
	if next.Err() != nil {
		t.Fatalf("new call has canceled authorization: %v", context.Cause(next))
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
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				waitQuotaDenied(t, svc, giznet.PublicKey{4}, policy)
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
	go func() {
		_, release, err := svc.Authorize(t.Context(), giznet.PublicKey{5}, policy)
		if release != nil {
			release()
		}
		finished <- err
	}()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("blocked peer did not start")
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("blocked hook rejected cold authorization: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("cold authorization waited for the blocked hook")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	blockedCall, blockedRelease, err := svc.Authorize(ctx, giznet.PublicKey{5}, policy)
	if err != nil {
		t.Fatalf("same-Peer call waited for blocked refresh: %v", err)
	}
	defer blockedRelease()
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
	case <-ctx.Done():
		t.Fatal("close retained an active call")
	}
	select {
	case <-blockedCall.Done():
	case <-ctx.Done():
		t.Fatal("close retained the same-Peer call")
	}
}

type warningRecorder struct {
	slog.Handler
	mu      sync.Mutex
	records []slog.Record
	peers   []string
}

func (h *warningRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (h *warningRecorder) Handle(ctx context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, record.Clone())
	h.peers = append(h.peers, gizlog.PeerPublicKey(ctx))
	return nil
}

func TestHookFailuresWarnWithoutRevokingCallsOrLeakingSecrets(t *testing.T) {
	handler := &warningRecorder{}
	previous := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(previous) })
	var requests atomic.Int32
	svc, policy := testService(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "credential secret response body", http.StatusUnauthorized)
	})
	call, release, err := svc.Authorize(t.Context(), giznet.PublicKey{9}, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	deadline := time.After(2 * time.Second)
	for requests.Load() < 2 {
		select {
		case <-call.Done():
			t.Fatalf("hook failure canceled active call: %v", context.Cause(call))
		case <-deadline:
			t.Fatal("failed hook was not retried")
		case <-time.After(time.Millisecond):
		}
	}
	handler.mu.Lock()
	records := append([]slog.Record(nil), handler.records...)
	peers := append([]string(nil), handler.peers...)
	handler.mu.Unlock()
	if len(records) != 1 || records[0].Level != slog.LevelWarn {
		t.Fatalf("want one rate-limited warning, got %+v", records)
	}
	attributes := map[string]string{}
	records[0].Attrs(func(attr slog.Attr) bool {
		attributes[attr.Key] = attr.Value.String()
		return true
	})
	if peers[0] != (giznet.PublicKey{9}).String() || attributes["reason"] != "quota HTTP status 401" {
		t.Fatalf("warning lacks safe failure context: %+v", attributes)
	}
	for _, value := range attributes {
		if strings.Contains(value, policy.ApiKey) || strings.Contains(value, policy.Endpoint) || strings.Contains(value, "secret response body") {
			t.Fatalf("warning leaked private hook data: %+v", attributes)
		}
	}
}

func TestExplicitDenialSurvivesStaleResultAndHookFailure(t *testing.T) {
	var requests atomic.Int32
	respond := make(chan struct{})
	svc, policy := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			select {
			case <-respond:
			case <-r.Context().Done():
				return
			}
			sendDecision(t, w, quota.QuotaResponse{ExpiresAt: new(time.Now().Add(-time.Hour)), ValidUntil: time.Now().Add(100 * time.Millisecond)})
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	call, release, err := svc.Authorize(t.Context(), giznet.PublicKey{10}, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	close(respond)
	waitQuotaDenied(t, svc, giznet.PublicKey{10}, policy)
	select {
	case <-call.Done():
		if !errors.Is(context.Cause(call), ErrDenied) {
			t.Fatalf("cause = %v", context.Cause(call))
		}
	case <-time.After(time.Second):
		t.Fatal("explicit denial did not cancel the active call")
	}
	time.Sleep(150 * time.Millisecond)
	if requests.Load() < 2 {
		t.Fatal("denied decision was not refreshed")
	}
	_, _, err = svc.Authorize(t.Context(), giznet.PublicKey{10}, policy)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("hook failure forgot explicit denial: %v", err)
	}
}
