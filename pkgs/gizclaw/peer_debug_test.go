package gizclaw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestDeviceDebugAccessModes(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	for _, tc := range []struct {
		mode, method, path, body string
		want                     int
	}{
		{"off", "GET", "/device", "", 403},
		{"readonly", "GET", "/device", "", 200},
		{"readonly", "POST", "/device/actions/reboot", "{}", 403},
		{"readonly", "POST", "/contacts", `{"name":"debug-contact","phone":"123"}`, 403},
		{"fullcontrol", "GET", "/device", "", 200},
		{"fullcontrol", "POST", "/contacts", `{"name":"debug-contact","display_name":"Debug"}`, 201},
		{"fullcontrol", "POST", "/device/actions/reboot", "{}", 409},
		{"fullcontrol", "GET", "/api-keys", "", 401},
		{"fullcontrol", "POST", "/api-keys", `{"display_name":"escape"}`, 401},
		{"off", "GET", "/device", "", 403},
	} {
		t.Run(tc.mode+tc.method+tc.path, func(t *testing.T) {
			if _, err := f.peers.PutSelfInfo(context.Background(), f.owner, apitypes.DeviceInfo{DebugMode: &tc.mode}); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(tc.method, "/gizclaw/v1"+tc.path+"?public_key="+f.owner.String(), strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			res := httptest.NewRecorder()
			f.handler.ServeHTTP(res, req)
			if res.Code == http.StatusOK && res.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("debug response is cacheable")
			}
			if res.Code != tc.want {
				t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
			}
		})
	}
	// A supplied API key retains its own owner even with an invalid selector.
	if res := f.do(t, "GET", "/gizclaw/v1/device?public_key=not-a-key", ""); res.Code != 200 {
		t.Fatalf("API key owner changed: %d %s", res.Code, res.Body.String())
	}
	// An invalid bearer must never fall back to debug access.
	mode := "fullcontrol"
	if _, err := f.peers.PutSelfInfo(context.Background(), f.owner, apitypes.DeviceInfo{DebugMode: &mode}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/gizclaw/v1/device?public_key="+f.owner.String(), nil)
	req.Header.Set("Authorization", "Bearer invalid")
	res := httptest.NewRecorder()
	f.handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("invalid bearer status=%d", res.Code)
	}
}

func TestAnonymousDeviceDirectoryIncludesDebugOff(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	record, err := f.peers.LoadPeer(context.Background(), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	record.Device.Identifiers = &apitypes.DeviceIdentifiers{Sn: new("shared"), Imeis: &[]apitypes.PeerIMEI{{Tac: "123", Serial: "456"}}}
	if _, err = f.peers.SavePeer(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/peers/@findBySn/shared", "/peers/@findByImei/123/456"} {
		res := httptest.NewRecorder()
		f.handler.ServeHTTP(res, httptest.NewRequest("GET", "/gizclaw/v1"+path, nil))
		if res.Code != 200 {
			t.Fatalf("%s: %d %s", path, res.Code, res.Body.String())
		}
		var result struct {
			PublicKeys []string `json:"public_keys"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.PublicKeys) != 1 || result.PublicKeys[0] != f.owner.String() {
			t.Fatalf("%s: %s", path, res.Body.String())
		}
	}
}
