package streamlog

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

func readRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

func TestRecorderAggregatesDeltasAndKeepsAudioIndependent(t *testing.T) {
	var buf bytes.Buffer
	r := New(t.Context(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{AddSource: true})), "model_output")
	r.ObserveInput(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input", BeginOfStream: true}})
	epoch := genx.NewResponseEpoch("input")
	for _, text := range []string{"你", "好", "。", "再", "见"} {
		r.Observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "reply", ResponseEpoch: epoch}})
	}
	r.Observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "reply", ResponseEpoch: epoch, EndOfStream: true}})
	for range 3 {
		r.Observe(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}, Ctrl: &genx.StreamCtrl{StreamID: "reply", ResponseEpoch: epoch}})
	}
	r.Close(nil)
	r.Close(nil)
	counts := map[string]int{}
	var sentences []string
	for _, record := range readRecords(t, &buf) {
		event := record["event"].(string)
		counts[event]++
		if record["source"] == nil {
			t.Fatal("missing source")
		}
		if record["stream_id"] != "reply" || record["input_stream_id"] != "input" || record["role"] != "assistant" {
			t.Fatalf("identity: %v", record)
		}
		if event == "first_text" || event == "first_audio" {
			if _, ok := record["input_elapsed_ms"]; !ok {
				t.Fatal("missing input latency")
			}
		}
		if event == "text" {
			sentences = append(sentences, record["content"].(string))
		}
	}
	if counts["first_text"] != 1 || counts["first_audio"] != 1 || counts["stream_end"] != 2 || strings.Join(sentences, "") != "你好。再见" || len(sentences) != 2 {
		t.Fatalf("counts=%v sentences=%v", counts, sentences)
	}
}

func TestRecorderDoesNotAttachUnownedOutputToLatestInput(t *testing.T) {
	var buf bytes.Buffer
	r := New(t.Context(), slog.New(slog.NewJSONHandler(&buf, nil)), "model_output")
	r.ObserveInput(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "latest"}})
	r.Observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("unowned"), Ctrl: &genx.StreamCtrl{StreamID: "other"}})
	r.Close(context.Canceled)
	for _, record := range readRecords(t, &buf) {
		if _, ok := record["input_elapsed_ms"]; ok {
			t.Fatal("invented input ownership")
		}
	}
}

func TestRecorderBoundsTextAndFlushesControlEOS(t *testing.T) {
	var buf bytes.Buffer
	r := New(t.Context(), slog.New(slog.NewJSONHandler(&buf, nil)), "model_output")
	text := strings.Repeat("语", maxTextBytes)
	r.Observe(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "transcript"}})
	r.Observe(&genx.MessageChunk{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "transcript", EndOfStream: true}})
	r.Close(nil)
	var joined strings.Builder
	for _, record := range readRecords(t, &buf) {
		if record["event"] == "text" {
			content := record["content"].(string)
			if len(content) > maxTextBytes || !utf8.ValidString(content) {
				t.Fatal("invalid bounded text")
			}
			joined.WriteString(content)
		}
	}
	if joined.String() != text {
		t.Fatal("truncated transcript")
	}
}

func TestRecorderConcurrentShutdown(t *testing.T) {
	var buf bytes.Buffer
	r := New(t.Context(), slog.New(slog.NewJSONHandler(&buf, nil)), "model_output")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				r.Observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("a"), Ctrl: &genx.StreamCtrl{StreamID: "reply"}})
			}
		})
	}
	wg.Go(func() { r.Close(context.Canceled) })
	wg.Wait()
	r.Close(nil)
	ends := 0
	for _, record := range readRecords(t, &buf) {
		if record["event"] == "stream_end" {
			ends++
		}
	}
	if ends > 1 {
		t.Fatalf("duplicate terminal records: %d", ends)
	}
}

func TestRecorderFlushKeepsReplacementRuntimeObservable(t *testing.T) {
	var buf bytes.Buffer
	r := New(t.Context(), slog.New(slog.NewJSONHandler(&buf, nil)), "model_output")
	for _, id := range []string{"old", "new"} {
		r.Observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(id), Ctrl: &genx.StreamCtrl{StreamID: id}})
		r.Flush(context.Canceled)
	}
	r.Close(nil)
	var content []string
	for _, record := range readRecords(t, &buf) {
		if record["event"] == "text" {
			content = append(content, record["content"].(string))
		}
	}
	if strings.Join(content, ",") != "old,new" {
		t.Fatalf("replacement lost: %v", content)
	}
}

func TestFirstOutputMetricsPersistWithoutIdentityLabels(t *testing.T) {
	store := metrics.NewMemoryStore()
	shutdown, err := gizmetrics.InstallStore(store, gizmetrics.WithFlushInterval(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
	ctx := gizlog.WithPeerPublicKey(gizlog.WithRequestID(t.Context(), "request-secret"), "peer-secret")
	r := New(ctx, slog.New(slog.NewJSONHandler(io.Discard, nil)), "model_output")
	r.ObserveInput(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-secret"}})
	r.Observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "output-secret", ResponseEpoch: genx.NewResponseEpoch("input-secret")}})
	r.Close(nil)
	if err := shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	series, err := store.Latest(t.Context(), metrics.LatestQuery{Selector: metrics.Selector{Name: "genx_input_to_first_output_seconds_count"}, At: time.Now(), Lookback: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || len(series[0].Points) != 1 || series[0].Points[0].Value != 1 {
		t.Fatalf("missing first-output metric: %v", series)
	}
	if len(series[0].Labels) != 3 || series[0].Labels["boundary"] != "model_output" || series[0].Labels["event"] != "first_text" || series[0].Labels["role"] != "assistant" {
		t.Fatalf("unexpected metric labels: %v", series[0].Labels)
	}
}

type blockingHandler struct {
	entered, release chan struct{}
	once             sync.Once
}

func (*blockingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *blockingHandler) WithAttrs([]slog.Attr) slog.Handler     { return h }
func (h *blockingHandler) WithGroup(string) slog.Handler          { return h }
func (h *blockingHandler) Handle(context.Context, slog.Record) error {
	h.once.Do(func() { close(h.entered); <-h.release })
	return nil
}

func TestRecorderDoesNotHoldStateLockDuringSinkIO(t *testing.T) {
	handler := &blockingHandler{entered: make(chan struct{}), release: make(chan struct{})}
	r := New(t.Context(), slog.New(handler), "model_output")
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("x"), Ctrl: &genx.StreamCtrl{StreamID: "output"}})
	}()
	<-handler.entered
	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		r.ObserveInput(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input"}})
	}()
	select {
	case <-inputDone:
	case <-time.After(time.Second):
		close(handler.release)
		<-done
		t.Fatal("log sink blocked input state")
	}
	close(handler.release)
	<-done
	r.Close(nil)
}

func TestRecorderTerminalOutcomeIncludesCodeOnlyFailures(t *testing.T) {
	for _, test := range []struct{ message, code, result string }{{"", "PROVIDER_TIMEOUT", "error"}, {"interrupted", "", "interrupted"}, {"context canceled", "", "canceled"}, {"", "", "completed"}} {
		var buf bytes.Buffer
		r := New(t.Context(), slog.New(slog.NewJSONHandler(&buf, nil)), "model_output")
		r.Observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("partial"), Ctrl: &genx.StreamCtrl{StreamID: "reply", EndOfStream: true, Error: test.message, ErrorCode: test.code}})
		records := readRecords(t, &buf)
		terminal := records[len(records)-1]
		if terminal["result"] != test.result || (test.code != "" && terminal["error_code"] != test.code) {
			t.Fatalf("terminal = %v, want %+v", terminal, test)
		}
	}
}

func TestRecorderExplicitOutputLinkSurvivesLaterInput(t *testing.T) {
	var buf bytes.Buffer
	r := New(t.Context(), slog.New(slog.NewJSONHandler(&buf, nil)), "transformer_output")
	r.ObserveInput(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "original", EndOfStream: true}})
	r.LinkOutput("original", "reply")
	r.ObserveInput(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "later"}})
	chunk := &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "reply", EndOfStream: true}}
	r.Observe(chunk)
	if chunk.Ctrl.ResponseEpoch != nil {
		t.Fatal("logging changed ownership")
	}
	for _, record := range readRecords(t, &buf) {
		if record["input_stream_id"] != "original" || record["input_elapsed_ms"] == nil || record["after_input_end_ms"] == nil {
			t.Fatalf("missing exact input association: %v", record)
		}
	}
}

func TestRecorderRemappedSourceRequiresExactKnownInput(t *testing.T) {
	for _, source := range []string{"input", "unknown"} {
		var buf bytes.Buffer
		r := New(t.Context(), slog.New(slog.NewJSONHandler(&buf, nil)), "peer_delivery")
		r.ObserveInput(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input", EndOfStream: true}})
		r.Observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("reply"), Ctrl: &genx.StreamCtrl{StreamID: "remapped", SourceStreamID: source, EndOfStream: true}})
		for _, record := range readRecords(t, &buf) {
			_, timed := record["input_elapsed_ms"]
			if timed != (source == "input") || record["source_stream_id"] != source {
				t.Fatalf("invalid source association: %v", record)
			}
		}
	}
}
