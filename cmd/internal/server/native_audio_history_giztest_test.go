package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"

	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"
)

type nativeHistoryRequest struct {
	raw      json.RawMessage
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

type nativeHistoryProtocol struct {
	mu        sync.Mutex
	requests  []nativeHistoryRequest
	histories []apitypes.PeerRunHistoryListResponse
	audio     [][32]byte
	sections  int
}

// Run the committed Giztest with real Server, Edge, WebRTC, provider adapters,
// Eino and SQL History. Only remote model/speech protocols are local fixtures.
func TestNativeAudioHistoryGiztest(t *testing.T) {
	fixture := &nativeHistoryProtocol{}
	provider := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(provider.Close)
	withPeerControlServer(t, 8, func(ctx context.Context, _ *gizclaw.Server, admin *gizcli.Client, _, edgeURL string) {
		root := filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "testdata", "native-audio-history")
		data, err := os.ReadFile(filepath.Join(root, "resources.json"))
		if err != nil {
			t.Fatal(err)
		}
		var resource apitypes.Resource
		if err := json.Unmarshal(bytes.ReplaceAll(data, []byte("FIXTURE_ENDPOINT"), []byte(provider.URL)), &resource); err != nil {
			t.Fatal(err)
		}
		api, err := admin.ServerAdminClient()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := adminresource.NewClient(api, nil).ApplyResource(ctx, resource); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GIZCLAW_TEST_ENDPOINT", edgeURL)
		t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "local-peer-sync-test-token")
		t.Setenv("GIZCLAW_NATIVE_HISTORY_FIXTURE", provider.URL)
		directory := os.Getenv("GIZCLAW_NATIVE_HISTORY_EVIDENCE")
		if directory == "" {
			directory = t.TempDir()
		}
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		reportPath := filepath.Join(directory, "giztest.json")
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", filepath.Join(root, "rounds.giztest.yaml"), "--parallel", "1", "--evidence", "full", "--output", filepath.Join(directory, "replay"), "--report", reportPath})
		runErr := command.ExecuteContext(ctx)
		if err := os.WriteFile(filepath.Join(directory, "runner.log"), output.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		fixture.mu.Lock()
		requests, histories, audio := slices.Clone(fixture.requests), slices.Clone(fixture.histories), slices.Clone(fixture.audio)
		fixture.mu.Unlock()
		rawRequests := make([]json.RawMessage, len(requests))
		for index, request := range requests {
			rawRequests[index] = request.raw
		}
		for name, evidence := range map[string]any{"provider-requests.json": rawRequests, "history-snapshots.json": histories} {
			data, err := json.MarshalIndent(evidence, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, name), append(data, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
		}
		data, readErr := os.ReadFile(reportPath)
		if runErr != nil || readErr != nil {
			t.Fatalf("native History Giztest failed: %v, report: %v\n%s\n%s", runErr, readErr, output.String(), data)
		}
		var report giztest.Report
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatal(err)
		}
		if report.Status != "passed" || len(report.Tasks) != 1 || len(report.Tasks[0].Cleanup) != 3 {
			t.Fatalf("incomplete native History Giztest: %s", data)
		}
		for _, step := range append(report.Tasks[0].Steps, report.Tasks[0].Cleanup...) {
			if step.Status != "passed" {
				t.Fatalf("step %s was %s", step.ID, step.Status)
			}
		}
		if len(audio) != 2 || audio[0] == audio[1] || len(histories) != 2 || len(requests) != 6 {
			t.Fatalf("missing distinct audio, snapshots or exact model calls: audio=%x histories=%d calls=%d", audio, len(histories), len(requests))
		}
		var conversations [][]string
		for _, request := range requests {
			var dialogue []string
			for _, message := range request.Messages {
				var text string
				if (message.Role == "user" || message.Role == "assistant") && json.Unmarshal(message.Content, &text) == nil {
					dialogue = append(dialogue, message.Role+":"+text)
				}
			}
			if len(dialogue) != 0 {
				conversations = append(conversations, dialogue)
			}
		}
		// The provider adapter combines adjacent assistant sections in its
		// wire message. History must still retain each section separately.
		want := [][]string{
			{"user:first spoken question"},
			{"user:first spoken question", "assistant:first reply 1. "},
			{"user:first spoken question", "assistant:first reply 1. first reply 2. ", "user:second spoken question"},
			{"user:first spoken question", "assistant:first reply 1. first reply 2. ", "user:second spoken question", "assistant:second reply 1. "},
		}
		if !reflect.DeepEqual(conversations, want) {
			t.Fatalf("raw model requests reordered or concatenated turns: got %q, want %q", conversations, want)
		}
		t.Logf("native PTT: two distinct audio inputs, two separate replies per turn, ordered raw History and next model context; evidence: %s", directory)
	})
}

func (f *nativeHistoryProtocol) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/v1/t2a_v2" {
		new(podcastProtocolFixture).speech(w, r)
		return
	}
	if r.URL.Path == "/history" {
		var snapshot apitypes.PeerRunHistoryListResponse
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&snapshot); err != nil {
			http.Error(w, "invalid snapshot", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.histories = append(f.histories, snapshot)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"retained": true, "snapshot": snapshot})
		return
	}
	if r.URL.Path != "/api/v3/chat/completions" {
		http.NotFound(w, r)
		return
	}
	var request nativeHistoryRequest
	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil || json.Unmarshal(payload, &request) != nil {
		http.Error(w, "invalid model request", http.StatusBadRequest)
		return
	}
	request.raw = payload
	audio := []byte(nil)
	for _, message := range request.Messages {
		var parts []struct {
			Type       string `json:"type"`
			InputAudio struct {
				Data string `json:"data"`
			} `json:"input_audio"`
		}
		if message.Role != "user" || json.Unmarshal(message.Content, &parts) != nil {
			continue
		}
		for _, part := range parts {
			if part.Type != "input_audio" {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(part.InputAudio.Data)
			if err != nil || len(data) < 44 {
				http.Error(w, "invalid source audio", http.StatusBadRequest)
				return
			}
			audio = append(audio, data...)
		}
	}
	f.mu.Lock()
	f.requests = append(f.requests, request)
	if len(audio) != 0 {
		f.audio = append(f.audio, sha256.Sum256(audio))
		f.sections = 0
	} else {
		f.sections++
	}
	turn := len(f.audio)
	section := f.sections
	f.mu.Unlock()
	text := fmt.Sprintf("first reply %d. ", section)
	if turn > 1 {
		text = fmt.Sprintf("second reply %d. ", section)
	}
	if len(audio) != 0 {
		text = `{"transcript":"first spoken question"}`
		if turn > 1 {
			text = `{"transcript":"second spoken question"}`
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for _, part := range []map[string]any{{"delta": map[string]any{"role": "assistant", "content": text}, "finish_reason": nil}, {"delta": map[string]any{}, "finish_reason": "stop"}} {
		data, err := json.Marshal(map[string]any{"id": "native-history", "object": "chat.completion.chunk", "model": "doubao-seed-2-1-lite-260915", "choices": []any{part}})
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return
		}
		w.(http.Flusher).Flush()
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}
