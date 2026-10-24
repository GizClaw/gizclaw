package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"

	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"
)

// Exercise the CLI through real Server/Edge/WebRTC and the production ASR,
// model and TTS adapters. Only the remote HTTP/WebSocket protocols are local.
func TestAudioTurnOwnershipGiztest(t *testing.T) {
	for _, tc := range []struct {
		mode string
		keep bool
	}{{mode: "push-to-talk"}, {mode: "realtime"}, {mode: "realtime", keep: true}} {
		t.Run(fmt.Sprintf("%s/retained=%t", tc.mode, tc.keep), func(t *testing.T) {
			fixture := &audioTurnProtocol{realtime: tc.mode == "realtime", openingAudioInput: tc.keep, release: make(chan struct{}), tail: make(chan struct{})}
			provider := httptest.NewServer(http.HandlerFunc(fixture.serve))
			t.Cleanup(provider.Close)
			original := websocket.DefaultDialer
			dialer := *original
			dialer.Proxy = nil
			dialer.NetDialTLSContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				if address != "openspeech.bytedance.com:443" {
					return nil, fmt.Errorf("unexpected speech endpoint %q", address)
				}
				return (&net.Dialer{}).DialContext(ctx, network, provider.Listener.Addr().String())
			}
			websocket.DefaultDialer = &dialer
			t.Cleanup(func() { websocket.DefaultDialer = original })
			withPeerControlServer(t, 8, func(ctx context.Context, _ *gizclaw.Server, admin *gizcli.Client, _, edgeURL string) {
				root := filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "testdata")
				data, err := os.ReadFile(filepath.Join(root, "native-audio-history", "resources.json"))
				if err != nil {
					t.Fatal(err)
				}
				var resources map[string]any
				if err := json.Unmarshal(bytes.ReplaceAll(data, []byte("FIXTURE_ENDPOINT"), []byte(provider.URL)), &resources); err != nil {
					t.Fatal(err)
				}
				for _, item := range resources["spec"].(map[string]any)["items"].([]any) {
					resource := item.(map[string]any)
					if resource["kind"] == "Model" && resource["metadata"].(map[string]any)["id"] == "native-history-audio" {
						resource["spec"].(map[string]any)["provider_data"].(map[string]any)["support_text_only"] = true
					}
					if resource["kind"] != "Workflow" {
						continue
					}
					eino := resource["spec"].(map[string]any)["eino"].(map[string]any)
					delete(eino, "conversation")
					graph := eino["graph"].(map[string]any)
					graph["nodes"] = graph["nodes"].([]any)[:1]
					graph["edges"] = []any{map[string]any{"from": "start", "to": "reply"}, map[string]any{"from": "reply", "to": "end"}}
				}
				data, err = json.Marshal(resources)
				if err != nil {
					t.Fatal(err)
				}
				var resource apitypes.Resource
				if err := json.Unmarshal(data, &resource); err != nil {
					t.Fatal(err)
				}
				api, err := admin.ServerAdminClient()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := adminresource.NewClient(api, nil).ApplyResource(ctx, resource); err != nil {
					t.Fatal(err)
				}
				data, err = os.ReadFile(filepath.Join(root, "audio-turn-ownership", "rounds.giztest.yaml"))
				if err != nil {
					t.Fatal(err)
				}
				var doc giztest.Document
				story, _, _ := strings.Cut(string(data), "version:")
				if err := yaml.Unmarshal(data, &doc); err != nil {
					t.Fatal(err)
				}
				doc.Name = fmt.Sprintf("audio-turn-ownership.%s.retained-%t", tc.mode, tc.keep)
				for i := range doc.Steps {
					step := &doc.Steps[i]
					if step.ID == "create" && tc.mode == "realtime" {
						step.RPC.Request.(map[string]any)["parameters"].(map[string]any)["eino_workspace_parameters"].(map[string]any)["input"] = "WORKSPACE_INPUT_MODE_REALTIME"
					}
					if step.PeerStream == nil {
						continue
					}
					if step.ID != "opening" || tc.keep {
						step.PeerStream.Mode = tc.mode
						step.PeerStream.Pacing = "20ms"
					}
					if tc.keep {
						step.PeerStream.Input = "${input_audio}"
						step.PeerStream.Session, step.PeerStream.KeepOpen = "microphone", true
					}
				}
				t.Setenv("GIZCLAW_TEST_ENDPOINT", edgeURL)
				t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "local-peer-sync-test-token")
				t.Setenv("GIZCLAW_TURN_FIXTURE", provider.URL)
				directory := t.TempDir()
				if base := os.Getenv("GIZCLAW_TURN_OWNERSHIP_EVIDENCE"); base != "" {
					directory = filepath.Join(base, tc.mode, fmt.Sprintf("retained-%t", tc.keep))
					if err := os.MkdirAll(directory, 0700); err != nil {
						t.Fatal(err)
					}
				}
				data, err = yaml.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				data = append([]byte(story), data...)
				path := filepath.Join(directory, "rounds.giztest.yaml")
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				command := giztestcmd.NewCmd()
				var output bytes.Buffer
				command.SetOut(&output)
				command.SetErr(&output)
				reportPath := filepath.Join(directory, "giztest.json")
				command.SetArgs([]string{"run", path, "--parallel", "1", "--report", reportPath})
				runErr := command.ExecuteContext(ctx)
				if err := os.WriteFile(filepath.Join(directory, "runner.log"), output.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
				fixture.mu.Lock()
				requests := slices.Clone(fixture.requests)
				inputs := fixture.inputs
				fixture.mu.Unlock()
				if runErr != nil {
					t.Fatalf("real transport turn ownership: %v\n%s", runErr, output.String())
				}
				if !slices.Equal(requests, []string{"opening", "first question", "second question"}) {
					t.Fatalf("model input order=%v", requests)
				}
				wantInputs := 2
				if tc.keep {
					wantInputs++
				}
				if inputs != wantInputs {
					t.Fatalf("ASR received %d inputs, want %d", inputs, wantInputs)
				}
				data, err = os.ReadFile(reportPath)
				if err != nil {
					t.Fatal(err)
				}
				var report giztest.Report
				if err := json.Unmarshal(data, &report); err != nil {
					t.Fatal(err)
				}
				if report.Status != "passed" || len(report.Tasks) != 1 || len(report.Tasks[0].Cleanup) != 3 {
					t.Fatalf("incomplete transport report: %s", data)
				}
				for _, step := range append(report.Tasks[0].Steps, report.Tasks[0].Cleanup...) {
					if step.Status != "passed" {
						t.Fatalf("step %s status=%s", step.ID, step.Status)
					}
					if tc.keep && (step.ID == "first_turn" || step.ID == "second_turn") && step.Evidence["session_connection_reused"] != true {
						t.Fatalf("step %s did not reuse its session", step.ID)
					}
				}
			})
		})
	}
}

type audioTurnProtocol struct {
	realtime, openingAudioInput bool
	release, tail               chan struct{}
	once                        sync.Once
	mu                          sync.Mutex
	inputs                      int
	requests                    []string
}

func (f *audioTurnProtocol) serve(w http.ResponseWriter, r *http.Request) {
	if websocket.IsWebSocketUpgrade(r) {
		f.asr(w, r)
		return
	}
	if r.URL.Path == "/opening-tail" {
		f.once.Do(func() { close(f.release) })
		select {
		case <-f.tail:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"released":true}`)
		case <-r.Context().Done():
		}
		return
	}
	if r.URL.Path == "/v1/t2a_v2" {
		new(podcastProtocolFixture).speech(w, r)
		return
	}
	var request nativeHistoryRequest
	if r.URL.Path != "/api/v3/chat/completions" || json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&request) != nil {
		http.Error(w, "unexpected provider request", http.StatusBadRequest)
		return
	}
	var input string
	for _, message := range request.Messages {
		if message.Role == "user" {
			_ = json.Unmarshal(message.Content, &input)
		}
	}
	f.mu.Lock()
	f.requests = append(f.requests, input)
	f.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	emit := func(text string) error {
		data, err := json.Marshal(map[string]any{"id": "turn-fixture", "object": "chat.completion.chunk", "model": "protocol-fixture", "choices": []any{map[string]any{"delta": map[string]any{"role": "assistant", "content": text}}}})
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return err
		}
		w.(http.Flusher).Flush()
		return nil
	}
	finish := func() error {
		if _, err := io.WriteString(w, "data: {\"id\":\"turn-fixture\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"); err != nil {
			return err
		}
		w.(http.Flusher).Flush()
		return nil
	}
	if input == "opening" {
		if err := emit("slow-opening."); err != nil {
			return
		}
		select {
		case <-f.release:
			if err := emit(" old-opening-tail."); err != nil {
				return
			}
			if err := finish(); err != nil {
				return
			}
			close(f.tail)
		case <-r.Context().Done():
			return
		}
		return
	}
	// The controlled reply is later than the end of the 200 ms input clip.
	// This makes accepting any queued old text/audio observable as 0 ms.
	timer := time.NewTimer(600 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-r.Context().Done():
		return
	}
	if err := emit(strings.Replace(input, "question", "reply", 1)); err != nil {
		return
	}
	if err := finish(); err != nil {
		return
	}
}

func (f *audioTurnProtocol) asr(w http.ResponseWriter, r *http.Request) {
	conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	text, sent := "", false
	received, peak := 0, 0
	for {
		_, frame, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if len(frame) < 8 || frame[1]>>4 != 2 {
			continue
		}
		pcm := frame[8:]
		if frame[2]&15 == 1 {
			reader, err := gzip.NewReader(bytes.NewReader(pcm))
			if err != nil {
				return
			}
			pcm, err = io.ReadAll(io.LimitReader(reader, 1<<20))
			_ = reader.Close()
			if err != nil {
				return
			}
		}
		received += len(pcm)
		for i := 0; i+1 < len(pcm); i += 2 {
			value := int(int16(binary.LittleEndian.Uint16(pcm[i:])))
			peak = max(peak, value, -value)
		}
		if received == 0 || peak < 500 {
			continue
		}
		if text == "" {
			f.mu.Lock()
			f.inputs++
			turn := f.inputs
			f.mu.Unlock()
			if f.openingAudioInput {
				turn--
			}
			text = map[int]string{0: "opening", 1: "first question", 2: "second question"}[turn]
		}
		last := frame[1]&2 != 0
		// Wait for 500 ms of PCM in realtime. The 200 ms speech clip has
		// ended before this point, even when its paced sender is delayed.
		if !last && (!f.realtime || sent || received < 16000) {
			continue
		}
		payload, err := json.Marshal(map[string]any{"audio_info": map[string]any{"duration": 200}, "result": map[string]any{"text": text, "utterances": []any{map[string]any{"text": text, "start_time": 0, "end_time": 200, "definite": true}}}})
		if err != nil {
			return
		}
		response := []byte{0x11, 0x91, 0x10, 0}
		seq := uint32(1)
		if last {
			response[1], seq = 0x93, ^uint32(0)
		}
		response = binary.BigEndian.AppendUint32(response, seq)
		response = binary.BigEndian.AppendUint32(response, uint32(len(payload)))
		if err := conn.WriteMessage(websocket.BinaryMessage, append(response, payload...)); err != nil {
			return
		}
		sent = true
		if last {
			return
		}
	}
}
