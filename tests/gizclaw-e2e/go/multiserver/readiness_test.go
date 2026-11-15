//go:build gizclaw_e2e

package multiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// waitSharedRead retries only a successful read that has not observed the expected
// shared state. Each caller supplies a one-minute deadline; read errors stop it.
func waitSharedRead(ctx context.Context, read func(context.Context) (bool, error)) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		ready, err := read(ctx)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func apiKeyVisible(ctx context.Context, base, key string) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/gizclaw/v1/api-keys/self", nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		_, err := io.Copy(io.Discard, response.Body)
		return err == nil, err
	}
	var denied apitypes.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&denied); err != nil {
		return false, fmt.Errorf("API key readiness HTTP %d: %w", response.StatusCode, err)
	}
	if response.StatusCode == http.StatusUnauthorized && denied.Error.Code == "INVALID_API_KEY" {
		return false, nil
	}
	return false, fmt.Errorf("API key readiness HTTP %d code=%s", response.StatusCode, denied.Error.Code)
}

func waitAPIKeyVisible(t *testing.T, ctx context.Context, base, key string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := waitSharedRead(ctx, func(ctx context.Context) (bool, error) {
		return apiKeyVisible(ctx, base, key)
	}); err != nil {
		t.Fatalf("wait for API key visibility through %s: %v", base, err)
	}
}

func waitRemoteFriend(t *testing.T, ctx context.Context, client *gizcli.Client, peerKey, workspace string) *rpcapi.FriendListResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	var friends *rpcapi.FriendListResponse
	var attempt int
	if err := waitSharedRead(ctx, func(ctx context.Context) (bool, error) {
		attempt++
		var err error
		friends, err = client.ListFriends(ctx, fmt.Sprintf("remote-friend-visible-%d", attempt), rpcapi.FriendListRequest{})
		if err != nil {
			return false, err
		}
		if len(friends.Items) == 0 {
			return false, nil
		}
		if len(friends.Items) != 1 {
			return false, fmt.Errorf("unexpected remote Friend count: %d", len(friends.Items))
		}
		item := friends.Items[0]
		if item.PeerPublicKey == nil || item.WorkspaceName == nil {
			return false, nil
		}
		if *item.PeerPublicKey != peerKey || *item.WorkspaceName != workspace {
			return false, fmt.Errorf("unexpected remote Friend identity or Workspace: %+v", item)
		}
		return true, nil
	}); err != nil {
		t.Fatalf("wait for remote Friend %s in Workspace %s: %v", peerKey, workspace, err)
	}
	return friends
}

func TestSharedReadReadiness(t *testing.T) {
	t.Run("absence is read again until visible", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		calls := 0
		err := waitSharedRead(ctx, func(context.Context) (bool, error) {
			calls++
			return calls == 2, nil
		})
		if err != nil || calls != 2 {
			t.Fatalf("error=%v calls=%d, want success on the second read", err, calls)
		}
	})
	t.Run("read errors are not retried", func(t *testing.T) {
		failure := errors.New("read failed")
		calls := 0
		err := waitSharedRead(t.Context(), func(context.Context) (bool, error) {
			calls++
			return false, failure
		})
		if !errors.Is(err, failure) || calls != 1 {
			t.Fatalf("error=%v calls=%d, want original failure after one read", err, calls)
		}
	})
	t.Run("absence respects the caller deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		calls := 0
		err := waitSharedRead(ctx, func(context.Context) (bool, error) {
			calls++
			return false, nil
		})
		if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
			t.Fatalf("error=%v calls=%d, want deadline after one read", err, calls)
		}
	})
	t.Run("only missing API keys may wait", func(t *testing.T) {
		for _, tc := range []struct {
			status    int
			code      string
			ready     bool
			wantError bool
		}{
			{http.StatusOK, "", true, false},
			{http.StatusUnauthorized, "INVALID_API_KEY", false, false},
			{http.StatusUnauthorized, "OTHER_AUTH_FAILURE", false, true},
			{http.StatusInternalServerError, "INTERNAL_ERROR", false, true},
		} {
			t.Run(fmt.Sprintf("%d/%s", tc.status, tc.code), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodGet || r.URL.Path != "/gizclaw/v1/api-keys/self" {
						t.Errorf("unexpected readiness request: %s %s", r.Method, r.URL.Path)
					}
					w.WriteHeader(tc.status)
					_, _ = fmt.Fprintf(w, `{"error":{"code":%q}}`, tc.code)
				}))
				defer server.Close()
				ready, err := apiKeyVisible(t.Context(), server.URL, "test-key")
				if ready != tc.ready || (err != nil) != tc.wantError {
					t.Fatalf("ready=%v error=%v, want ready=%v error=%v", ready, err, tc.ready, tc.wantError)
				}
			})
		}
	})
}
