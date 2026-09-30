package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestToolInvokeControlBodies proves that a tool/v0 invoke body the typed C
// control API cannot express reaches the Server unchanged, while a valid body
// still goes through the typed call.
func TestToolInvokeControlBodies(t *testing.T) {
	control, err := openControl()
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	previous := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = previous })
	cases := []struct {
		name, request, sent, response string
		status, kind                  int
	}{
		{"typed find", `{"tool":"device.find","args":{"duration_ms":1500}}`, `{"tool":"device.find","args":{"duration_ms":1500}}`, `{"result":{}}`, 200, 0},
		{"non-object args reach Server", `{"tool":"device.find","args":"not-an-object"}`, `{"tool":"device.find","args":"not-an-object"}`, `{"error":{"code":"INVALID_REQUEST"}}`, 400, 10},
		{"missing args reach Server", `{"tool":"device.find"}`, `{"tool":"device.find"}`, `{"error":{"code":"INVALID_REQUEST"}}`, 400, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			http.DefaultClient = &http.Client{Transport: mhsControlTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Method != http.MethodPost || req.URL.Path != "/gizclaw/v1/device/tool/v0/invoke" || req.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("unexpected request: %s %s authorization=%q", req.Method, req.URL.Path, req.Header.Get("Authorization"))
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Error(err)
				}
				var got, want any
				if err := json.Unmarshal(body, &got); err != nil {
					t.Errorf("request body %q: %v", body, err)
				}
				if err := json.Unmarshal([]byte(tc.sent), &want); err != nil {
					t.Error(err)
				}
				gotJSON, _ := json.Marshal(got)
				wantJSON, _ := json.Marshal(want)
				if string(gotJSON) != string(wantJSON) {
					t.Errorf("request=%s want=%s", gotJSON, wantJSON)
				}
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.response)), Request: req}, nil
			})}
			result, err := control.Request("https://tool.invalid", "Bearer test-key", http.MethodPost, "/gizclaw/v1/device/tool/v0/invoke", tc.request, 1000)
			if err != nil || result.status != tc.status || result.kind != tc.kind || string(result.body) != tc.response {
				t.Fatalf("result=%+v body=%s err=%v", result, result.body, err)
			}
			if calls != 1 {
				t.Fatalf("transport calls=%d", calls)
			}
		})
	}
}
