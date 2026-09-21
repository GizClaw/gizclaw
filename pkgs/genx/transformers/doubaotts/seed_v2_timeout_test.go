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
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
)

func TestSeedV2ContinuesAfterHTTPClientTimeout(t *testing.T) {
	for _, test := range []struct {
		name, failed, want string
		partial            bool
	}{
		{name: "first", failed: "1", want: "audio2audio3"},
		{name: "middle", failed: "2", want: "audio1audio3"},
		{name: "last", failed: "3", want: "audio1audio2"},
		{name: "all", failed: "123"},
		{name: "body", failed: "2", partial: true, want: "audio1partial2audio3"},
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
				_, _ = io.Copy(io.Discard, r.Body)
				index := strings.Index("123", request.Params.Text[len(request.Params.Text)-2:len(request.Params.Text)-1])
				if index < 0 {
					t.Errorf("unexpected segment %q", request.Params.Text)
					return
				}
				requests[index].Add(1)
				segment := fmt.Sprint(index + 1)
				if strings.Contains(test.failed, segment) {
					if test.partial {
						_, _ = fmt.Fprintf(w, "{\"code\":0,\"data\":%q}\n", base64.StdEncoding.EncodeToString([]byte("partial"+segment)))
						w.(http.Flusher).Flush()
					}
					<-r.Context().Done()
					return
				}
				_, _ = fmt.Fprintf(w, "{\"code\":0,\"data\":%q}\n{\"code\":20000000}\n", base64.StdEncoding.EncodeToString([]byte("audio"+segment)))
			}))
			defer server.Close()
			logs := installPeerLogStore(t)
			ctx := gizlog.WithPeerPublicKey(t.Context(), "peer-test")
			transformer, err := NewSeedV2(SeedV2Config{
				Client:  doubaospeech.NewClient("test", doubaospeech.WithBaseURL(server.URL), doubaospeech.WithTimeout(250*time.Millisecond)),
				Speaker: "test-speaker", Format: "pcm",
			})
			if err != nil {
				t.Fatal(err)
			}
			output, err := transformer.Transform(ctx, &seedV2TestStream{chunks: []*genx.MessageChunk{
				{Part: genx.Text("Sentence number 1!"), Ctrl: &genx.StreamCtrl{StreamID: "reply", BeginOfStream: true}},
				{Part: genx.Text("Sentence number 2!"), Ctrl: &genx.StreamCtrl{StreamID: "reply"}},
				{Part: genx.Text("Sentence number 3!"), Ctrl: &genx.StreamCtrl{StreamID: "reply", EndOfStream: true}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			var audio strings.Builder
			var terminal *genx.MessageChunk
			var endErr error
			for {
				chunk, err := output.Next()
				if err != nil {
					endErr = err
					break
				}
				audio.Write(chunk.Part.(*genx.Blob).Data)
				if chunk.IsEndOfStream() {
					terminal = chunk
				}
			}
			if audio.String() != test.want {
				t.Errorf("audio = %q, want %q", audio.String(), test.want)
			}
			for i := range requests {
				if n := requests[i].Load(); n != 1 {
					t.Errorf("segment %d requests = %d, want 1", i+1, n)
				}
			}
			if ctx.Err() != nil {
				t.Fatalf("caller context unexpectedly ended: %v", ctx.Err())
			}
			if terminal == nil {
				t.Fatal("missing audio EOS")
			}
			if test.want == "" {
				if !errors.Is(endErr, context.DeadlineExceeded) || terminal.Ctrl.Error == "" {
					t.Errorf("all-failed terminal = %+v, error = %v", terminal.Ctrl, endErr)
				}
			} else if terminal.Ctrl.Error != "" || (!errors.Is(endErr, io.EOF) && !errors.Is(endErr, genx.ErrDone)) {
				t.Errorf("partial reply must finish normally: EOS=%+v, error=%v", terminal.Ctrl, endErr)
			}
			seen := map[string]bool{}
			for range len(test.failed) {
				record := logs.waitFor(t, "doubao tts: segment failed")
				index := record.Attributes["segment_index"]
				if index == "" || !strings.Contains(test.failed, index) || seen[index] {
					t.Fatalf("unexpected failure log: %+v", record)
				}
				seen[index] = true
				if record.Severity != "ERROR" || record.Attributes["stream_id"] != "reply" || !strings.Contains(record.Attributes["error"], "Client.Timeout") {
					t.Errorf("timeout log = %+v", record)
				}
			}
		})
	}
}

func TestSeedV2SegmentFailureUsesCallerContext(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			logs := installPeerLogStore(t)
			providerErr := fmt.Errorf("provider request: %w", cause)
			err := seedV2SegmentFailure(t.Context(), streamkit.TTSMeta{StreamID: "reply", SegmentIndex: 2}, providerErr, nil)
			var segmentErr *streamkit.TTSSegmentError
			if !errors.As(err, &segmentErr) || !errors.Is(err, cause) {
				t.Errorf("live caller: error = %T(%v), want segment failure", err, err)
			} else {
				logs.waitFor(t, "doubao tts: segment failed")
			}
			var ctx context.Context
			var cancel context.CancelFunc
			if cause == context.Canceled {
				ctx, cancel = context.WithCancel(t.Context())
				cancel()
			} else {
				ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			}
			defer cancel()
			err = seedV2SegmentFailure(ctx, streamkit.TTSMeta{}, providerErr, nil)
			segmentErr = nil
			if !errors.Is(err, cause) || errors.As(err, &segmentErr) {
				t.Errorf("ended caller: error = %T(%v), want terminal %v", err, err, cause)
			}
		})
	}
}

func TestSeedV2DefaultHTTPStreamBeyondThirtySeconds(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"headers", "body"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if phase == "body" {
					_, _ = io.WriteString(w, "{\"code\":0,\"data\":\"Zmlyc3Q=\"}\n")
					w.(http.Flusher).Flush()
				}
				select {
				case <-time.After(31 * time.Second):
				case <-r.Context().Done():
					return
				}
				_, _ = io.WriteString(w, "{\"code\":0,\"data\":\"bGFzdA==\"}\n{\"code\":20000000}\n")
			}))
			defer server.Close()
			output, err := newSeedV2ForTest(t, server.URL, "pcm").Transform(t.Context(), &seedV2TestStream{chunks: []*genx.MessageChunk{
				{Part: genx.Text("A long synthesis request!"), Ctrl: &genx.StreamCtrl{StreamID: "reply", EndOfStream: true}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			var audio strings.Builder
			var terminal *genx.MessageChunk
			for {
				chunk, err := output.Next()
				if err != nil {
					if !errors.Is(err, io.EOF) && !errors.Is(err, genx.ErrDone) {
						t.Errorf("stream failed: %v", err)
					}
					break
				}
				audio.Write(chunk.Part.(*genx.Blob).Data)
				if chunk.IsEndOfStream() {
					terminal = chunk
				}
			}
			want := "last"
			if phase == "body" {
				want = "firstlast"
			}
			if audio.String() != want || terminal == nil || terminal.Ctrl.Error != "" {
				t.Fatalf("audio=%q, EOS=%+v; want %q and successful EOS", audio.String(), terminal, want)
			}
		})
	}
}
