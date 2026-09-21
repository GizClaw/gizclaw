package doubaotts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
)

func TestSeedV2ContinuesAfterSegmentFailure(t *testing.T) {
	for _, test := range []struct {
		name    string
		failed  string
		partial bool
		want    string
	}{
		{name: "middle", failed: "2", want: "audio1audio3"},
		{name: "first", failed: "1", want: "audio2audio3"},
		{name: "last", failed: "3", want: "audio1audio2"},
		{name: "all", failed: "123", want: ""},
		{name: "partial_middle", failed: "2", partial: true, want: "audio1partial2audio3"},
		{name: "all_partial", failed: "123", partial: true, want: "partial1partial2partial3"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests [3]atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Params struct {
						Text string `json:"text"`
					} `json:"req_params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				text := strings.TrimSpace(request.Params.Text)
				index := strings.Index("123", text[len(text)-2:len(text)-1])
				if index < 0 {
					t.Errorf("unexpected segment %q", text)
					return
				}
				requests[index].Add(1)
				segment := fmt.Sprint(index + 1)
				failed := strings.Contains(test.failed, segment)
				audio := "audio" + segment
				if failed {
					w.Header().Set("Content-Length", "4096")
					audio = ""
					if test.partial {
						audio = "partial" + segment
					}
				}
				_, _ = fmt.Fprintf(w, `{"reqid":"req-%s","trace_id":"trace-%s","log_id":"log-%s","code":0,"data":"%s"}`+"\n", segment, segment, segment, base64.StdEncoding.EncodeToString([]byte(audio)))
				if !failed {
					_, _ = io.WriteString(w, "{\"code\":20000000}\n")
				}
			}))
			defer server.Close()
			logs := installPeerLogStore(t)
			ctx, cancel := context.WithTimeout(gizlog.WithPeerPublicKey(t.Context(), "peer-test"), 5*time.Second)
			defer cancel()
			output, err := newSeedV2ForTest(t, server.URL, "pcm").Transform(ctx, &seedV2TestStream{chunks: []*genx.MessageChunk{
				{Role: genx.RoleModel, Part: genx.Text("Sentence number 1!"), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant", BeginOfStream: true}},
				{Role: genx.RoleModel, Part: genx.Text("Sentence number 2!"), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant"}},
				{Role: genx.RoleModel, Part: genx.Text("Sentence number 3!"), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant", EndOfStream: true}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			var audio strings.Builder
			var terminal *genx.MessageChunk
			var endErr error
			bos, eos := 0, 0
			for {
				chunk, err := output.Next()
				if err != nil {
					endErr = err
					break
				}
				if chunk.Ctrl.StreamID != "reply" {
					t.Errorf("stream ID = %q", chunk.Ctrl.StreamID)
				}
				if chunk.IsBeginOfStream() {
					bos++
				}
				if chunk.IsEndOfStream() {
					eos++
					terminal = chunk
				}
				audio.Write(chunk.Part.(*genx.Blob).Data)
			}
			if audio.String() != test.want {
				t.Errorf("audio = %q, want %q", audio.String(), test.want)
			}
			for i := range requests {
				if got := requests[i].Load(); got != 1 {
					t.Errorf("segment %d requests = %d, want 1 (no retry)", i+1, got)
				}
			}
			if bos != 1 || eos != 1 {
				t.Fatalf("boundaries BOS=%d EOS=%d, want 1 each", bos, eos)
			}
			if test.want == "" {
				apiErr, ok := doubaospeech.AsError(endErr)
				if !ok || apiErr.ReqID != "req-1" || terminal.Ctrl.Error != endErr.Error() || terminal.Ctrl.FailureClass != genx.FailureClassProvider {
					t.Errorf("all-failed terminal = %+v, stream error = %v", terminal.Ctrl, endErr)
				}
			} else if terminal.Ctrl.Error != "" || (!errors.Is(endErr, io.EOF) && !errors.Is(endErr, genx.ErrDone)) {
				t.Errorf("partial reply must terminate normally: EOS=%+v, error=%v", terminal.Ctrl, endErr)
			}
			seen := map[string]bool{}
			for range len(test.failed) {
				record := logs.waitFor(t, "doubao tts: segment failed")
				index := record.Attributes["segment_index"]
				if !strings.Contains(test.failed, index) || index == "" || seen[index] {
					t.Fatalf("unexpected segment failure log: %+v", record)
				}
				seen[index] = true
				if record.Severity != "ERROR" {
					t.Errorf("severity = %q, want ERROR", record.Severity)
				}
				for key, want := range map[string]string{
					"stream_id": "reply", "peer_public_key": "peer-test", "code": "3005",
					"message": "tts stream truncated before final frame", "request_id": "req-" + index,
					"trace_id": "trace-" + index, "log_id": "log-" + index,
				} {
					if record.Attributes[key] != want {
						t.Errorf("log %s = %q, want %q", key, record.Attributes[key], want)
					}
				}
				for _, key := range []string{"text", "authorization", "api_key"} {
					if _, ok := record.Attributes[key]; ok {
						t.Errorf("failure log contains %s", key)
					}
				}
			}
		})
	}
}

func TestSeedV2EmitsAudioBeforeProviderFinishes(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "{\"code\":0,\"data\":\"Zmlyc3Q=\"}\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "{\"code\":0,\"data\":\"c2Vjb25k\"}\n{\"code\":20000000}\n")
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	output, err := newSeedV2ForTest(t, server.URL, "pcm").Transform(ctx, &seedV2TestStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("An incremental sentence."), Ctrl: &genx.StreamCtrl{StreamID: "reply", EndOfStream: true}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	chunk, err := output.Next()
	if err != nil || !chunk.IsBeginOfStream() {
		t.Fatalf("BOS = %v, %v", chunk, err)
	}
	chunk, err = output.Next()
	if err != nil || string(chunk.Part.(*genx.Blob).Data) != "first" {
		t.Fatalf("audio before releasing provider = %v, %v", chunk, err)
	}
	close(release)
	chunks := collectDoubaoTTSChunks(t, output)
	if len(chunks) != 2 || string(chunks[0].Part.(*genx.Blob).Data) != "second" || !chunks[1].IsEndOfStream() {
		t.Fatalf("remaining output = %+v", chunks)
	}
}
