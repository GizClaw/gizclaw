package streamkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/streamlog"
)

func TestStageLogsPreserveDrainAndDeferredAcknowledgement(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	ctx := streamlog.StartStage(t.Context(), "test_transformer")
	input := &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("question"), Ctrl: &genx.StreamCtrl{StreamID: "input", BeginOfStream: true, EndOfStream: true}}
	streamlog.ObserveInputRead(ctx, input, io.EOF)
	acknowledgements := 0
	output := NewOutput(OutputConfig{LogContext: ctx, Observe: func(*genx.MessageChunk) { acknowledgements++ }})
	output.DeferOutputObservation()
	epoch := genx.NewResponseEpoch("input")
	var chunks []*genx.MessageChunk
	for _, text := range []string{"H", "i", ".", " tail"} {
		chunk := &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "reply", ResponseEpoch: epoch}}
		chunks = append(chunks, chunk)
		if err := output.Push(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	// CloseWithError is a no-op once production closed normally; logging must
	// not discard the recorder for the still-readable queue.
	if err := output.CloseWithError(context.Canceled); err != nil {
		t.Fatal(err)
	}
	for _, expected := range chunks {
		got, err := output.Next()
		if err != nil || got != expected {
			t.Fatalf("changed delivery: %p %v", got, err)
		}
	}
	if acknowledgements != 0 {
		t.Fatal("logging acknowledged undelivered output")
	}
	output.ObserveOutput(chunks[0])
	output.ObserveOutput(chunks[0])
	if acknowledgements != 1 {
		t.Fatal("changed acknowledgement semantics")
	}
	if _, err := output.Next(); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	first, ended := 0, 0
	var sentences []string
	for line := range strings.SplitSeq(strings.TrimSpace(logs.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record["transformer"] != "test_transformer" {
			t.Fatal(record)
		}
		if record["boundary"] != "transformer_output" {
			continue
		}
		switch record["event"] {
		case "first_text":
			first++
			if record["input_stream_id"] != "input" || record["input_elapsed_ms"] == nil {
				t.Fatal(record)
			}
		case "text":
			sentences = append(sentences, record["content"].(string))
		case "stream_end":
			ended++
			if record["result"] != "completed" {
				t.Fatal(record)
			}
		}
	}
	if first != 1 || ended != 1 || strings.Join(sentences, "") != "Hi. tail" || len(sentences) != 2 {
		t.Fatalf("first=%d ended=%d sentences=%v", first, ended, sentences)
	}
	// An explicit abort flushes an incomplete fragment without requiring another pull.
	aborted := NewOutput(OutputConfig{LogContext: streamlog.StartStage(ctx, "aborted")})
	if err := aborted.Push(chunks[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := aborted.Next(); err != nil {
		t.Fatal(err)
	}
	if err := aborted.CloseWithError(context.Canceled); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), `"result":"canceled"`) {
		t.Fatal("abort was not logged")
	}
}
