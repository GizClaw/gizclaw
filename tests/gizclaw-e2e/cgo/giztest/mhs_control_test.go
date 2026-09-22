package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// mhsControlTransport replaces only the network boundary. Requests still pass
// through bridge.c, the typed C control SDK and the real cgo HTTP backend.
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
	const manifest = `{"devices":[{"id":"display.main","kind":"display","tags":["test"],"states":[{"name":"brightness","type":"int","access":"read_write","min":0,"max":100,"step":1},{"name":"mode","type":"enum","access":"read","enum_values":["auto","off"]}]}]}`
	const read = `{"states":[{"device_id":"display.main","state":"brightness"},{"device_id":"led.status","state":"enabled"}]}`
	const readResponse = `{"states":[{"device_id":"display.main","state":"brightness","value":35},{"device_id":"led.status","state":"enabled","value":false}]}`
	const write = `{"states":[{"device_id":"display.main","state":"brightness","value":50},{"device_id":"led.status","state":"enabled","value":true}]}`
	const applied = `{"states":[{"device_id":"display.main","state":"brightness","value":40},{"device_id":"led.status","state":"enabled","value":true}]}`
	largeStates := make([]map[string]any, 32)
	for i := range largeStates {
		largeStates[i] = map[string]any{"device_id": "x", "state": fmt.Sprintf("value-%d", i), "value": strings.Repeat("\x01", 256)}
	}
	largeBatch, err := json.Marshal(map[string]any{"states": largeStates})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, method, route, request, response string
		status, kind                           int
		decodeError                            bool
	}{
		{"manifest", "GET", "manifest", "", manifest, 200, 0, false},
		{"empty manifest", "GET", "manifest", "", `{"devices":[]}`, 200, 0, false},
		{"read", "POST", "read", read, readResponse, 200, 0, false},
		{"write applied", "PATCH", "states", write, applied, 200, 0, false},
		{"maximum escaped batch", "PATCH", "states", string(largeBatch), string(largeBatch), 200, 0, false},
		{"missing hardware", "POST", "read", `{"states":[{"device_id":"battery.main","state":"level"}]}`, `{"error":{"code":"MHS_STATE_NOT_FOUND"}}`, 404, 3, false},
		{"invalid batch", "PATCH", "states", `{"states":[{"device_id":"battery.main","state":"level","value":80}]}`, `{"error":{"code":"INVALID_REQUEST"}}`, 400, 10, false},
		{"duplicate batch reaches server", "POST", "read", `{"states":[{"device_id":"display.main","state":"brightness"},{"device_id":"display.main","state":"brightness"}]}`, `{"error":{"code":"INVALID_REQUEST"}}`, 400, 10, false},
		{"exact integers", "PATCH", "states", `{"states":[{"device_id":"x","state":"value","value":9007199254740991},{"device_id":"x","state":"negative","value":-9007199254740991}]}`, `{"states":[{"device_id":"x","state":"value","value":9007199254740991}]}`, 200, 0, false},
		{"escaped string", "PATCH", "states", `{"states":[{"device_id":"x","state":"label","value":"二\n\"\\😀"}]}`, `{"states":[{"device_id":"x","state":"label","value":"二\n\"\\😀"}]}`, 200, 0, false},
		{"malformed response fails runner", "POST", "read", read, `{"states":[{"device_id":"x","state":"value","value":null}]}`, 200, 0, true},
		{"malformed nested manifest fails runner", "GET", "manifest", "", `{"devices":[{"id":"x","kind":"x","states":[{"name":"x","type":"invalid","access":"read"}]}]}`, 200, 0, true},
	} {
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
					// Compact without converting numbers to float64.
					var got, want strings.Builder
					for _, pair := range []struct {
						raw string
						dst *strings.Builder
					}{{string(body), &got}, {tc.request, &want}} {
						decoder := json.NewDecoder(strings.NewReader(pair.raw))
						decoder.UseNumber()
						var value any
						if err := decoder.Decode(&value); err != nil {
							t.Error(err)
						}
						encoded, err := json.Marshal(value)
						if err != nil {
							t.Error(err)
						}
						pair.dst.Write(encoded)
					}
					if got.String() != want.String() {
						t.Errorf("request=%s want=%s", got.String(), want.String())
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
