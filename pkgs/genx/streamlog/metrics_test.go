package streamlog

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

func TestSpeechMetricsUseKnownContentAndEOSOrigins(t *testing.T) {
	store := metrics.NewMemoryStore()
	stop, err := gizmetrics.InstallStore(store, gizmetrics.WithFlushInterval(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stop(context.Background()) })
	ctx := StartStage(t.Context(), "test-asr", Model{Provider: "test", Model: "asr-model", Kind: "asr"})
	r := New(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), "transformer_output")
	now := time.Now()
	r.inputs["input"] = inputTiming{contentMIME: "audio/pcm", started: now.Add(-10 * time.Second), contentStarted: now.Add(-2 * time.Second), ended: now.Add(-200 * time.Millisecond)}
	for _, text := range []string{"", "hello", " world"} {
		r.Observe(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "input"}})
	}
	eos := &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "input", EndOfStream: true}}
	r.Observe(eos)
	r.Observe(eos)
	r.Close(nil)
	// Whitespace is not a successful transcript.
	r = New(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), "transformer_output")
	r.inputs["empty"] = inputTiming{contentMIME: "audio/pcm", started: now, contentStarted: now, ended: now}
	r.Observe(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(" "), Ctrl: &genx.StreamCtrl{StreamID: "empty", EndOfStream: true}})
	// Text passed through an ASR stage is not an ASR model invocation.
	r.inputs["pass"] = inputTiming{contentMIME: "text/plain", started: now, contentStarted: now, ended: now}
	r.Observe(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("pass through"), Ctrl: &genx.StreamCtrl{StreamID: "pass", EndOfStream: true}})
	// Failed and unowned routes must not fabricate successful final latency.
	r = New(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), "transformer_output")
	r.Observe(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "unknown", EndOfStream: true, Error: "failed"}})
	if err := stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]float64{"genx_asr_first_text_seconds": 2, "genx_asr_final_result_seconds": .2} {
		for suffix := range 2 {
			metric := name + "_count"
			expected := 1.0
			if suffix == 1 {
				metric = name + "_sum"
				expected = want
			}
			series, err := store.Latest(t.Context(), metrics.LatestQuery{Selector: metrics.Selector{Name: metric}, At: time.Now(), Lookback: time.Minute})
			if err != nil || len(series) != 1 {
				t.Fatalf("%s series=%v err=%v", metric, series, err)
			}
			value := series[0].Points[0].Value
			if value < expected || value > expected+.1 {
				t.Fatalf("%s=%f want approximately %f", metric, value, expected)
			}
			if len(series[0].Labels) != 3 || series[0].Labels["model"] != "asr-model" {
				t.Fatalf("labels=%v", series[0].Labels)
			}
		}
	}
}

func TestInputTimingKeepsFirstEOS(t *testing.T) {
	r := New(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)), "transformer_output")
	chunk := &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}}
	r.ObserveInput(chunk)
	first := r.inputs["audio"]
	r.ObserveInput(chunk)
	if got := r.inputs["audio"]; got.ended != first.ended || got.contentStarted != first.contentStarted || got.contentMIME != "audio/pcm" {
		t.Fatalf("duplicate EOS changed input origin: first=%v got=%v", first, got)
	}
}
