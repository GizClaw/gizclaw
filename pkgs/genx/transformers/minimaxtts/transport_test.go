package minimaxtts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/minimax-go"
)

func TestDefaultHTTPAndExplicitTransports(t *testing.T) {
	for _, transport := range []Transport{"", TransportHTTP, TransportWebSocket} {
		name := string(transport)
		if transport == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			requests := make(chan map[string]any, 1)
			server := newMiniMaxTTSTestServer(t, requests)
			defer server.Close()
			client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			transformer, err := New(Config{Client: client, VoiceID: "voice", Model: "speech-model", Transport: transport})
			if err != nil {
				t.Fatal(err)
			}
			want := transport
			if want == "" {
				want = TransportHTTP
			}
			if transformer.transport != want {
				t.Fatalf("transport = %q, want %q", transformer.transport, want)
			}
			if err := transformer.synthesize(t.Context(), "hello", streamkit.TTSMeta{}, "audio/mpeg", func([]byte) error { return nil }); err != nil {
				t.Fatal(err)
			}
			request := <-requests
			if want == TransportHTTP && request["stream"] != true || want == TransportWebSocket && request["event"] != "task_start" {
				t.Fatalf("wrong protocol: %v", request)
			}
			if _, err := New(Config{Client: client, VoiceID: "voice", Model: "speech-model", Transport: "unsupported"}); err == nil {
				t.Fatal("accepted an unsupported transport")
			}
		})
	}
}

func TestHTTPReusesConnectionPreservesTraceAndDoesNotReplayAggregate(t *testing.T) {
	t.Setenv("GIZCLAW_TTS_DEBUG", "1")
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(old)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"data\":{\"audio\":\"0102\",\"status\":1}}\n\n")
		// A compatible proxy can ignore exclusion. The full copy must still
		// be kept separate by the SDK rather than emitted a second time.
		fmt.Fprint(w, "data: {\"data\":{\"audio\":\"0102\",\"status\":2},\"extra_info\":{\"usage_characters\":5}}\n\n")
	}))
	defer server.Close()
	client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, APIKey: "private-api-key", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	transformer, err := New(Config{Transport: TransportHTTP, Client: client, VoiceID: "voice", Model: "speech-model", Format: "pcm"})
	if err != nil {
		t.Fatal(err)
	}
	var reused []bool
	ctx := httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = append(reused, info.Reused) }})
	for index := range 2 {
		var audio []byte
		err := transformer.synthesize(ctx, "private speech text", streamkit.TTSMeta{StreamID: "route", SegmentIndex: index + 1}, "audio/pcm", func(data []byte) error { audio = append(audio, data...); return nil })
		if err != nil || !bytes.Equal(audio, []byte{1, 2}) {
			t.Fatalf("audio=%v err=%v", audio, err)
		}
	}
	if len(reused) != 2 || reused[0] || !reused[1] {
		t.Fatalf("caller trace / connection reuse: %v", reused)
	}
	output := logs.String()
	if strings.Count(output, `"msg":"minimax tts: first audio timing"`) != 2 || !strings.Contains(output, `"connection_reused":true`) {
		t.Fatalf("missing first-audio timing: %s", output)
	}
	if strings.Contains(output, "private speech text") || strings.Contains(output, "private-api-key") {
		t.Fatal("timing log leaked request contents")
	}
}

func TestHTTPCancellationDoesNotCancelConcurrentSynthesis(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"data\":{\"audio\":\"0102\",\"status\":1}}\n\n")
		w.(http.Flusher).Flush()
		started <- body.Text
		select {
		case <-r.Context().Done():
			return
		case <-release:
			fmt.Fprint(w, "data: {\"data\":{\"audio\":\"0304\",\"status\":1}}\n\ndata: {\"data\":{\"status\":2}}\n\n")
		}
	}))
	t.Cleanup(func() {
		once.Do(func() { close(release) })
		server.Close()
	})
	client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	transformer, err := New(Config{Transport: TransportHTTP, Client: client, VoiceID: "voice", Model: "speech-model", Format: "pcm"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	firstCtx, firstCancel := context.WithCancel(ctx)
	defer firstCancel()
	firstDone, secondDone := make(chan error, 1), make(chan error, 1)
	go func() {
		firstDone <- transformer.synthesize(firstCtx, "first", streamkit.TTSMeta{}, "audio/pcm", func([]byte) error { return nil })
	}()
	var second []byte
	go func() {
		secondDone <- transformer.synthesize(ctx, "second", streamkit.TTSMeta{}, "audio/pcm", func(data []byte) error { second = append(second, data...); return nil })
	}()
	for range 2 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("requests did not start")
		}
	}
	firstCancel()
	select {
	case err := <-firstDone:
		if err == nil {
			t.Fatal("cancelled synthesis completed normally")
		}
	case <-ctx.Done():
		t.Fatal("cancelled HTTP body stayed blocked")
	}
	once.Do(func() { close(release) })
	select {
	case err := <-secondDone:
		if err != nil || !bytes.Equal(second, []byte{1, 2, 3, 4}) {
			t.Fatalf("independent synthesis: audio=%v err=%v", second, err)
		}
	case <-ctx.Done():
		t.Fatal("independent synthesis stayed blocked")
	}
}

func TestHTTPTruncatedAudioIsAnErrorWithoutReplayingPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"data\":{\"audio\":\"0102\",\"status\":1}}\n\n")
	}))
	defer server.Close()
	client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	transformer, err := New(Config{Transport: TransportHTTP, Client: client, VoiceID: "voice", Model: "speech-model", Format: "pcm"})
	if err != nil {
		t.Fatal(err)
	}
	var audio []byte
	err = transformer.synthesize(t.Context(), "hello", streamkit.TTSMeta{}, "audio/pcm", func(data []byte) error { audio = append(audio, data...); return nil })
	if err == nil || !strings.Contains(err.Error(), "without a terminal") || !bytes.Equal(audio, []byte{1, 2}) {
		t.Fatalf("truncated stream: audio=%v err=%v", audio, err)
	}
}
