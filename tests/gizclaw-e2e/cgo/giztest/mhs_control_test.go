package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// mhsControlTransport replaces only the network boundary. Calls still use the
// C control SDK and the cgo HTTP backend.
type mhsControlTransport func(*http.Request) (*http.Response, error)

func (f mhsControlTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestMhsControlRoutes(t *testing.T) {
	control, err := openControl()
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	previous := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = previous })
	cases := []struct {
		name, method, route, request, response string
		status, kind                           int
		decodeError                            bool
	}{
		{"manifest", "GET", "manifest", "", `{"devices":[{"id":"led.left","hwd":"led"},{"id":"led.right","hwd":"led"}]}`, 200, 0, false},
		{"empty manifest", "GET", "manifest", "", `{"devices":[]}`, 200, 0, false},
		{"wifi", "POST", "read", `{"id":"wifi.main","hwd":"wifi"}`, `{"id":"wifi.main","hwd":"wifi","value":{"connected":true,"ssid":"home"}}`, 200, 0, false},
		{"ble", "POST", "read", `{"id":"ble.main","hwd":"ble"}`, `{"id":"ble.main","hwd":"ble","value":{"powered":true}}`, 200, 0, false},
		{"modem", "POST", "read", `{"id":"modem.main","hwd":"modem"}`, `{"id":"modem.main","hwd":"modem","value":{"registered":true}}`, 200, 0, false},
		{"battery", "POST", "read", `{"id":"battery.main","hwd":"battery"}`, `{"id":"battery.main","hwd":"battery","value":{"percent":80}}`, 200, 0, false},
		{"mic", "POST", "read", `{"id":"mic.main","hwd":"mic"}`, `{"id":"mic.main","hwd":"mic","value":{"available":true}}`, 200, 0, false},
		{"display", "POST", "read", `{"id":"display.main","hwd":"display"}`, `{"id":"display.main","hwd":"display","value":{"brightness_percent":40}}`, 200, 0, false},
		{"led", "POST", "read", `{"id":"led.left","hwd":"led"}`, `{"id":"led.left","hwd":"led","value":{"enabled":false}}`, 200, 0, false},
		{"speaker", "POST", "read", `{"id":"speaker.main","hwd":"speaker"}`, `{"id":"speaker.main","hwd":"speaker","value":{"volume_percent":30}}`, 200, 0, false},
		{"display write", "POST", "write", `{"id":"display.main","hwd":"display","value":{"brightness_percent":50}}`, `{"id":"display.main","hwd":"display","value":{"brightness_percent":45}}`, 200, 0, false},
		{"led write", "POST", "write", `{"id":"led.left","hwd":"led","value":{"enabled":false}}`, `{"id":"led.left","hwd":"led","value":{"enabled":false}}`, 200, 0, false},
		{"speaker write", "POST", "write", `{"id":"speaker.main","hwd":"speaker","value":{"volume_percent":50}}`, `{"id":"speaker.main","hwd":"speaker","value":{"volume_percent":45}}`, 200, 0, false},
		{"read-only write reaches Server", "POST", "write", `{"id":"battery.main","hwd":"battery","value":{}}`, `{"error":{"code":"INVALID_REQUEST"}}`, 400, 10, false},
		{"unknown HWD reaches Server", "POST", "read", `{"id":"device.main","hwd":"device"}`, `{"error":{"code":"INVALID_REQUEST"}}`, 400, 10, false},
		{"missing HWD", "POST", "read", `{"id":"battery.main","hwd":"battery"}`, `{"error":{"code":"MHS_HWD_NOT_FOUND"}}`, 404, 3, false},
		{"bad response type", "POST", "read", `{"id":"battery.main","hwd":"battery"}`, `{"id":"battery.main","hwd":"wifi","value":{"connected":true}}`, 200, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			http.DefaultClient = &http.Client{Transport: mhsControlTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Method != tc.method || req.URL.Path != "/gizclaw/v1/device/mhs/v0/"+tc.route || req.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("unexpected request: %s %s authorization=%q", req.Method, req.URL.Path, req.Header.Get("Authorization"))
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Error(err)
				}
				if tc.request == "" {
					if len(body) != 0 {
						t.Errorf("GET body=%s", body)
					}
				} else {
					var got, want any
					if err := json.Unmarshal(body, &got); err != nil {
						t.Error(err)
					}
					if err := json.Unmarshal([]byte(tc.request), &want); err != nil {
						t.Error(err)
					}
					gotJSON, _ := json.Marshal(got)
					wantJSON, _ := json.Marshal(want)
					if string(gotJSON) != string(wantJSON) {
						t.Errorf("request=%s want=%s", gotJSON, wantJSON)
					}
				}
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.response)), Request: req}, nil
			})}
			result, err := control.Request("https://mhs.invalid", "Bearer test-key", tc.method, "/gizclaw/v1/device/mhs/v0/"+tc.route, tc.request, 1000)
			if tc.decodeError {
				if err == nil || !strings.Contains(err.Error(), "MHS control encode/decode") {
					t.Fatalf("decode error=%v", err)
				}
			} else if err != nil || result.status != tc.status || result.kind != tc.kind || string(result.body) != tc.response {
				t.Fatalf("result=%+v body=%s err=%v", result, result.body, err)
			}
			if calls != 1 {
				t.Fatalf("transport calls=%d", calls)
			}
		})
	}
}
