package doubaotts

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

func TestSeedV2RejectsSuccessfulStreamWithoutAudio(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = fmt.Fprintln(w, `{"reqid":"req-final-only","trace_id":"trace-final-only","log_id":"log-final-only","code":20000000,"message":"ok","data":null}`)
	}))
	defer server.Close()

	transformer, err := NewSeedV2(SeedV2Config{
		Client: doubaospeech.NewClient(
			"app-test",
			doubaospeech.WithAPIKey("key-test"),
			doubaospeech.WithBaseURL(server.URL),
		),
		Speaker: "voice-test",
	})
	if err != nil {
		t.Fatalf("NewSeedV2() error = %v", err)
	}

	emittedBytes := 0
	err = transformer.synthesize(t.Context(), "readable text", streamkit.TTSMeta{}, transformer.mimeType(), func(audio []byte) error {
		emittedBytes += len(audio)
		return nil
	})
	if !errors.Is(err, errSeedV2EmptyAudio) {
		t.Fatalf("synthesize() error = %v, want empty-audio error", err)
	}
	wantError := "doubaotts: seed v2 completed without audio (request_id=req-final-only, trace_id=trace-final-only, log_id=log-final-only)"
	if err.Error() != wantError {
		t.Fatalf("synthesize() error = %q, want %q", err, wantError)
	}
	if emittedBytes != 0 {
		t.Fatalf("synthesize() emitted %d bytes, want 0", emittedBytes)
	}
	if got := requests.Load(); got != seedV2MaxAttempts {
		t.Fatalf("provider requests = %d, want %d", got, seedV2MaxAttempts)
	}
}

func TestSeedV2RetriesEmptyAudioUntilSuccess(t *testing.T) {
	wantAudio := []byte("pcm audio")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := requests.Add(1)
		if attempt < seedV2MaxAttempts {
			_, _ = fmt.Fprintln(w, `{"reqid":"req-empty","code":20000000,"message":"ok","data":null}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"reqid":"req-audio","code":0,"message":"","data":"%s"}`+"\n", base64.StdEncoding.EncodeToString(wantAudio))
		_, _ = fmt.Fprintln(w, `{"reqid":"req-audio","code":20000000,"message":"ok","data":null}`)
	}))
	defer server.Close()

	transformer := newSeedV2ForTest(t, server.URL, "pcm")
	var gotAudio []byte
	err := transformer.synthesize(t.Context(), "readable text", streamkit.TTSMeta{}, transformer.mimeType(), func(audio []byte) error {
		gotAudio = append(gotAudio, audio...)
		return nil
	})
	if err != nil {
		t.Fatalf("synthesize() error = %v", err)
	}
	if !bytes.Equal(gotAudio, wantAudio) {
		t.Fatalf("synthesize() audio = %q, want %q", gotAudio, wantAudio)
	}
	if got := requests.Load(); got != seedV2MaxAttempts {
		t.Fatalf("provider requests = %d, want %d", got, seedV2MaxAttempts)
	}
}

func TestSeedV2RejectsStreamNormalizedToEmptyAudio(t *testing.T) {
	metadataOnlyMP3 := []byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"reqid":"req-metadata","code":0,"message":"","data":"%s"}`+"\n", base64.StdEncoding.EncodeToString(metadataOnlyMP3))
		_, _ = fmt.Fprintln(w, `{"reqid":"req-metadata","code":20000000,"message":"ok","data":null}`)
	}))
	defer server.Close()

	transformer := newSeedV2ForTest(t, server.URL, "mp3")
	emittedBytes := 0
	err := transformer.synthesize(t.Context(), "readable text", streamkit.TTSMeta{}, transformer.mimeType(), func(audio []byte) error {
		emittedBytes += len(audio)
		return nil
	})
	if !errors.Is(err, errSeedV2EmptyAudio) {
		t.Fatalf("synthesize() error = %v, want empty-audio error", err)
	}
	if emittedBytes != 0 {
		t.Fatalf("synthesize() emitted %d normalized bytes, want 0", emittedBytes)
	}
}

func TestSeedV2ReturnsEmptyAudioFailureOnStreamRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, `{"reqid":"req-final-only","code":20000000,"message":"ok","data":null}`)
	}))
	defer server.Close()

	transformer := newSeedV2ForTest(t, server.URL, "pcm")
	output, err := transformer.Transform(t.Context(), &seedV2TestStream{chunks: []*genx.MessageChunk{
		{
			Role: genx.RoleModel,
			Name: "answer",
			Part: genx.Text("readable text"),
			Ctrl: &genx.StreamCtrl{StreamID: "response", Label: "assistant", EndOfStream: true},
		},
	}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()

	var chunks []*genx.MessageChunk
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			// The route reports the failure on its EOS and the stream itself then
			// surfaces the same cause instead of a clean end.
			if !errors.Is(nextErr, errSeedV2EmptyAudio) {
				t.Fatalf("output.Next() error = %v, want %v", nextErr, errSeedV2EmptyAudio)
			}
			break
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 2 {
		t.Fatalf("output chunk count = %d, want BOS/error EOS", len(chunks))
	}
	if !chunks[0].IsBeginOfStream() {
		t.Fatalf("initial chunk = %#v, want BOS", chunks[0])
	}
	terminal := chunks[1]
	wantError := "doubaotts: seed v2 completed without audio (request_id=req-final-only)"
	if !terminal.IsEndOfStream() || terminal.Ctrl.Error != wantError {
		t.Fatalf("terminal chunk = %#v, want empty-audio error EOS", terminal)
	}
	if blob, ok := terminal.Part.(*genx.Blob); !ok || len(blob.Data) != 0 {
		t.Fatalf("terminal part = %#v, want empty audio Blob", terminal.Part)
	}
}

func TestDoubaoTTSTransformersEmitOwnedAudioLifecycle(t *testing.T) {
	for _, test := range []struct {
		name string
		new  func(*testing.T, string) genx.Transformer
	}{
		{
			name: "Seed V2",
			new: func(t *testing.T, baseURL string) genx.Transformer {
				return newSeedV2ForTest(t, baseURL, "pcm")
			},
		},
		{
			name: "ICL V2",
			new: func(t *testing.T, baseURL string) genx.Transformer {
				transformer, err := NewICLV2(ICLV2Config{
					Client: doubaospeech.NewClient(
						"app-test",
						doubaospeech.WithAPIKey("key-test"),
						doubaospeech.WithBaseURL(baseURL),
					),
					Speaker: "S_voice-test",
					Format:  "pcm",
				})
				if err != nil {
					t.Fatalf("NewICLV2() error = %v", err)
				}
				return transformer
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			audio := []byte("pcm audio")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprintf(w, `{"reqid":"req-audio","code":0,"message":"","data":"%s"}`+"\n", base64.StdEncoding.EncodeToString(audio))
				_, _ = fmt.Fprintln(w, `{"reqid":"req-audio","code":20000000,"message":"ok","data":null}`)
			}))
			defer server.Close()

			output, err := test.new(t, server.URL).Transform(t.Context(), &seedV2TestStream{chunks: []*genx.MessageChunk{
				{Role: genx.RoleModel, Name: "answer", Part: genx.Text("readable text"), Ctrl: &genx.StreamCtrl{StreamID: "response", Label: "assistant"}},
				{Role: genx.RoleModel, Name: "answer", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "response", Label: "assistant", EndOfStream: true}},
			}})
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			defer output.Close()
			chunks := collectDoubaoTTSChunks(t, output)
			if len(chunks) != 3 || !chunks[0].IsBeginOfStream() || chunks[1].IsBeginOfStream() || chunks[1].IsEndOfStream() || !chunks[2].IsEndOfStream() {
				t.Fatalf("lifecycle = %#v, want BOS/data/EOS", chunks)
			}
			for index, chunk := range chunks {
				if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "response" || chunk.Ctrl.Label != "assistant" {
					t.Fatalf("chunk %d route = %#v", index, chunk.Ctrl)
				}
				blob, ok := chunk.Part.(*genx.Blob)
				if !ok || blob.MIMEType != "audio/pcm" {
					t.Fatalf("chunk %d part = %#v", index, chunk.Part)
				}
			}
			if blob := chunks[1].Part.(*genx.Blob); !bytes.Equal(blob.Data, audio) {
				t.Fatalf("audio data = %q, want %q", blob.Data, audio)
			}
		})
	}
}

func TestSeedV2EmitsNormalizedAudio(t *testing.T) {
	wantAudio := []byte("pcm audio")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"reqid":"req-audio","code":0,"message":"","data":"%s"}`+"\n", base64.StdEncoding.EncodeToString(wantAudio))
		_, _ = fmt.Fprintln(w, `{"reqid":"req-audio","code":20000000,"message":"ok","data":null}`)
	}))
	defer server.Close()

	transformer := newSeedV2ForTest(t, server.URL, "pcm")
	var gotAudio []byte
	err := transformer.synthesize(t.Context(), "readable text", streamkit.TTSMeta{}, transformer.mimeType(), func(audio []byte) error {
		gotAudio = append(gotAudio, audio...)
		return nil
	})
	if err != nil {
		t.Fatalf("synthesize() error = %v", err)
	}
	if !bytes.Equal(gotAudio, wantAudio) {
		t.Fatalf("synthesize() audio = %q, want %q", gotAudio, wantAudio)
	}
}

func TestSeedV2PreservesProviderError(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = fmt.Fprintln(w, `{"reqid":"req-provider","code":55000000,"message":"provider failed"}`)
	}))
	defer server.Close()

	transformer := newSeedV2ForTest(t, server.URL, "pcm")
	err := transformer.synthesize(t.Context(), "readable text", streamkit.TTSMeta{}, transformer.mimeType(), func([]byte) error {
		return nil
	})
	apiErr, ok := doubaospeech.AsError(err)
	if !ok {
		t.Fatalf("synthesize() error = %T(%v), want provider error", err, err)
	}
	if apiErr.Code != 55000000 || apiErr.ReqID != "req-provider" {
		t.Fatalf("provider error = %#v", apiErr)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("provider requests = %d, want 1", got)
	}
}

func TestSeedV2PreservesCanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	transformer := newSeedV2ForTest(t, server.URL, "pcm")
	err := transformer.synthesize(ctx, "readable text", streamkit.TTSMeta{}, transformer.mimeType(), func([]byte) error {
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("synthesize() error = %v, want context.Canceled", err)
	}
}

func TestSeedV2PreservesEmitError(t *testing.T) {
	audio := base64.StdEncoding.EncodeToString([]byte("pcm audio"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"reqid":"req-emit","code":0,"message":"","data":"%s"}`+"\n", audio)
		_, _ = fmt.Fprintln(w, `{"reqid":"req-emit","code":20000000,"message":"ok","data":null}`)
	}))
	defer server.Close()

	wantErr := errors.New("emit failed")
	transformer := newSeedV2ForTest(t, server.URL, "pcm")
	err := transformer.synthesize(t.Context(), "readable text", streamkit.TTSMeta{}, transformer.mimeType(), func([]byte) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("synthesize() error = %v, want %v", err, wantErr)
	}
}

func newSeedV2ForTest(t *testing.T, baseURL, format string) *SeedV2 {
	t.Helper()
	transformer, err := NewSeedV2(SeedV2Config{
		Client: doubaospeech.NewClient(
			"app-test",
			doubaospeech.WithAPIKey("key-test"),
			doubaospeech.WithBaseURL(baseURL),
		),
		Speaker: "voice-test",
		Format:  format,
	})
	if err != nil {
		t.Fatalf("NewSeedV2() error = %v", err)
	}
	return transformer
}

type seedV2TestStream struct {
	chunks []*genx.MessageChunk
	index  int
}

func (s *seedV2TestStream) Next() (*genx.MessageChunk, error) {
	if s.index == len(s.chunks) {
		return nil, io.EOF
	}
	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}

func (*seedV2TestStream) Close() error               { return nil }
func (*seedV2TestStream) CloseWithError(error) error { return nil }

func collectDoubaoTTSChunks(t *testing.T, stream genx.Stream) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
			return chunks
		}
		if err != nil {
			t.Fatalf("stream.Next() error = %v", err)
		}
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
}
