package graphstate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/cloudwego/eino/compose"
)

type checkpointReceipt struct {
	expected string
	snapshot genxeino.StateSnapshot
}

// Delegate every accepted write to the production SQL CAS store.
type checkpointProbe struct {
	genxeino.StateStore
	attempts atomic.Int32
	receipts chan checkpointReceipt
	before   func(context.Context, string, string, map[string]any) error
}

func (p *checkpointProbe) CompareAndSwap(ctx context.Context, scope, version string, fields map[string]any) (genxeino.StateSnapshot, error) {
	p.attempts.Add(1)
	if p.before != nil {
		if err := p.before(ctx, scope, version, fields); err != nil {
			return genxeino.StateSnapshot{}, err
		}
	}
	snapshot, err := p.StateStore.CompareAndSwap(ctx, scope, version, fields)
	if err == nil {
		p.receipts <- checkpointReceipt{expected: version, snapshot: snapshot}
	}
	return snapshot, err
}

type checkpointLambdas map[string]genxeino.ResolvedLambda

func (l checkpointLambdas) ResolveLambda(_ context.Context, name string) (genxeino.ResolvedLambda, error) {
	value, ok := l[name]
	if !ok {
		return genxeino.ResolvedLambda{}, fmt.Errorf("unknown checkpoint Lambda %q", name)
	}
	return value, nil
}

type continuationFixture struct {
	state   *Store
	probe   *checkpointProbe
	history *workspace.HistoryStore
	config  genxeino.Config
}

func newContinuationFixture(t *testing.T, total int64) *continuationFixture {
	t.Helper()
	db := testDB(t)
	state, err := OpenScope(t.Context(), db, "owner", "workspace", "agent", map[string]any{
		"cursor": int64(0), "queue": []any{"one", "two", "three", "four", "five"}, "played": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	records, err := memory.NewSQLHistoryStore(t.Context(), db, "continuation_history", 0)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	objects, err := objectstore.NewRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	history := workspace.NewHistoryStore(records, objects, "context")
	probe := &checkpointProbe{StateStore: state, receipts: make(chan checkpointReceipt, 16)}
	f := &continuationFixture{state: state, probe: probe, history: history}
	f.config = genxeino.Config{
		Agent: genxeino.AgentConfig{ID: "agent", ContextID: "context"}, ContinueFrom: "continue",
		State:   &genxeino.StatePersistenceConfig{Store: probe, Scope: "context", Fields: []string{"cursor", "queue", "played"}},
		History: &genxeino.HistoryConfig{Store: history, Scope: "context", Limit: 50},
		Graph: genxeino.GraphDefinition{
			Name: "checkpoint-flow",
			State: genxeino.StateDefinition{Fields: []genxeino.StateField{
				{Name: "cursor", Type: genxeino.StateInteger, Merge: genxeino.MergeReplace},
				{Name: "queue", Type: genxeino.StateList, Merge: genxeino.MergeReplace},
				{Name: "played", Type: genxeino.StateList, Merge: genxeino.MergeReplace},
				{Name: "transient", Type: genxeino.StateString, Merge: genxeino.MergeReplace},
				{Name: "answer", Type: genxeino.StateString, Merge: genxeino.MergeReplace},
				{Name: "continue", Type: genxeino.StateBoolean, Merge: genxeino.MergeReplace},
			}},
			Nodes: []genxeino.NodeDefinition{
				{ID: "advance", Lambda: &genxeino.LambdaRefNode{Lambda: "advance"},
					Inputs:  map[string]genxeino.Binding{"command": {From: "input.text"}, "cursor": {From: "cursor"}, "queue": {From: "queue"}, "played": {From: "played"}},
					Outputs: map[string]string{"cursor": "cursor", "queue": "queue", "played": "played", "continue": "continue", "transient": "transient"}},
				{ID: "narrate", Lambda: &genxeino.LambdaRefNode{Lambda: "narrate"},
					Inputs:  map[string]genxeino.Binding{"command": {From: "input.text"}, "cursor": {From: "cursor"}},
					Outputs: map[string]string{"text": "answer"}},
			},
			Edges:   []genxeino.EdgeDefinition{{From: "start", To: "advance"}, {From: "advance", To: "narrate"}, {From: "narrate", To: "end"}},
			Outputs: []genxeino.OutputDefinition{{Node: "narrate", Field: "answer", Name: "answer", MIMEType: "text/plain", Primary: true}},
		},
		Lambdas: checkpointLambdas{
			"advance": {
				Lambda: compose.InvokableLambda(func(_ context.Context, input map[string]any) (map[string]any, error) {
					cursor := input["cursor"].(int64)
					played := slices.Clone(input["played"].([]any))
					pause := input["command"] == "pause"
					if !pause {
						cursor++
						played = append(played, cursor)
					}
					return map[string]any{"cursor": cursor, "queue": input["queue"], "played": played, "continue": !pause && cursor < total, "transient": "not selected"}, nil
				}),
				Inputs:  map[string]genxeino.StateType{"command": genxeino.StateString, "cursor": genxeino.StateInteger, "queue": genxeino.StateList, "played": genxeino.StateList},
				Outputs: map[string]genxeino.StateType{"cursor": genxeino.StateInteger, "queue": genxeino.StateList, "played": genxeino.StateList, "continue": genxeino.StateBoolean, "transient": genxeino.StateString},
			},
			"narrate": {
				Lambda: compose.InvokableLambda(func(_ context.Context, input map[string]any) (map[string]any, error) {
					text := fmt.Sprintf("section %d", input["cursor"])
					if input["command"] == "pause" {
						text = "Paused."
					}
					return map[string]any{"text": text}, nil
				}),
				Inputs: map[string]genxeino.StateType{"command": genxeino.StateString, "cursor": genxeino.StateInteger}, Outputs: map[string]genxeino.StateType{"text": genxeino.StateString},
			},
		},
	}
	return f
}

type checkpointObserver interface {
	DeferOutputObservation()
	ObserveOutput(*genx.MessageChunk)
	AbandonOutputObservation(*genx.MessageChunk)
}

func checkpointInput(t *testing.T, builder *genx.StreamBuilder, text string) {
	t.Helper()
	id := genx.NewStreamID()
	if err := builder.Add(genx.NewBeginOfStream(id), &genx.MessageChunk{
		Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: id},
	}, &genx.MessageChunk{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}}); err != nil {
		t.Fatal(err)
	}
}

type checkpointResolver struct{ history *workspace.HistoryStore }

func (r checkpointResolver) Resolve(context.Context, string) (agenthost.Spec, error) {
	return agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "context"}, AgentType: "eino",
		Runtime: workspace.Runtime{History: r.history},
	}, nil
}

func checkpointOutput(t *testing.T, f *continuationFixture, input genx.Stream) (genx.Stream, checkpointObserver) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	// The Host is the sole History writer; Eino reads that same SQL History.
	host := agenthost.New(checkpointResolver{history: f.history})
	if err := host.Register("eino", agenthost.FactoryFunc(func(ctx context.Context, _ agenthost.Spec) (genx.Transformer, error) {
		return genxeino.New(ctx, f.config)
	})); err != nil {
		t.Fatal(err)
	}
	agent, release, err := host.OpenAgent(ctx, "context")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	output, err := agent.Transform(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = output.Close() })
	observer := output.(checkpointObserver)
	observer.DeferOutputObservation()
	return output, observer
}

func checkpointHistory(ctx context.Context, history memory.HistoryReader) ([]memory.HistoryEntry, error) {
	page, err := history.Query(ctx, memory.HistoryQuery{ConversationID: "context", Limit: 50})
	return page.Entries, err
}

func drainCheckpointOutput(t *testing.T, output genx.Stream, observer checkpointObserver) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := output.Next()
		if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
			return chunks
		}
		if err != nil {
			t.Fatal(err)
		}
		chunks = append(chunks, chunk)
		observer.ObserveOutput(chunk)
	}
}

func requireCheckpointCursor(t *testing.T, f *continuationFixture, cursor int64) genxeino.StateSnapshot {
	t.Helper()
	snapshot, err := f.state.Load(t.Context(), "context")
	if err != nil || snapshot.Fields["cursor"] != cursor {
		t.Fatalf("checkpoint=%+v error=%v, want cursor %d", snapshot, err, cursor)
	}
	if len(snapshot.Fields) != 3 || !reflect.DeepEqual(snapshot.Fields["queue"], []any{"one", "two", "three", "four", "five"}) {
		t.Fatalf("selected fields/cache changed: %+v", snapshot.Fields)
	}
	return snapshot
}

func TestContinuationCheckpointsFollowDeliveredHistory(t *testing.T) {
	for _, continuous := range []bool{false, true} {
		t.Run(fmt.Sprintf("continuous=%t", continuous), func(t *testing.T) {
			f := newContinuationFixture(t, 3)
			want := int64(3)
			if !continuous {
				f.config.ContinueFrom = ""
				want = 1
			}
			var acknowledged atomic.Int64
			f.probe.before = func(ctx context.Context, _ string, _ string, fields map[string]any) error {
				// Continuous messages flush at the delivered generation boundary;
				// ordinary routes retain their existing final-EOS recording policy.
				if !continuous {
					return nil
				}
				cursor := fields["cursor"].(int64)
				records, err := checkpointHistory(ctx, f.history)
				if err != nil {
					return err
				}
				if len(records) != int(cursor)+1 || continuous && acknowledged.Load() < cursor {
					return fmt.Errorf("State preceded delivered History: cursor=%d records=%d boundaries=%d", cursor, len(records), acknowledged.Load())
				}
				return nil
			}
			input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
			checkpointInput(t, input, "news")
			if err := input.Done(genx.Usage{}); err != nil {
				t.Fatal(err)
			}
			output, observer := checkpointOutput(t, f, input.Stream())
			streamID, begins, ends := "", 0, 0
			for {
				chunk, err := output.Next()
				if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if chunk.Ctrl.Label == "workspace.history.updated" {
					observer.ObserveOutput(chunk)
					continue
				}
				if chunk.Ctrl.Error != "" {
					t.Fatal(chunk.Ctrl.Error)
				}
				if streamID == "" {
					streamID = chunk.Ctrl.StreamID
				}
				if streamID != chunk.Ctrl.StreamID {
					t.Fatal("checkpoint split the output route")
				}
				if chunk.IsBeginOfStream() {
					begins++
				}
				if chunk.IsEndOfStream() {
					ends++
				}
				if chunk.Ctrl.MessageEnd {
					acknowledged.Add(1)
				}
				observer.ObserveOutput(chunk)
			}
			requireCheckpointCursor(t, f, want)
			records, err := checkpointHistory(t.Context(), f.history)
			if err != nil || len(records) != int(want)+1 {
				t.Fatalf("authoritative History=%+v err=%v, want user plus %d assistant messages", records, err, want)
			}
			if f.probe.attempts.Load() != int32(want) || len(f.probe.receipts) != int(want) || begins != 1 || ends != 1 {
				t.Fatalf("CAS attempts=%d commits=%d BOS/EOS=%d/%d, want %d writes", f.probe.attempts.Load(), len(f.probe.receipts), begins, ends, want)
			}
			version := ""
			for cursor := int64(1); cursor <= want; cursor++ {
				receipt := <-f.probe.receipts
				if receipt.expected != version || receipt.snapshot.Version == version || receipt.snapshot.Fields["cursor"] != cursor {
					t.Fatalf("CAS did not advance from %q at cursor %d: %+v", version, cursor, receipt)
				}
				version = receipt.snapshot.Version
			}
		})
	}
}

func TestContinuationInterruptResumesFromLastSQLCheckpoint(t *testing.T) {
	for _, holdBoundary := range []bool{false, true} {
		t.Run(fmt.Sprintf("hold-boundary=%t", holdBoundary), func(t *testing.T) {
			f := newContinuationFixture(t, 5)
			input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
			output, observer := checkpointOutput(t, f, input.Stream())
			checkpointInput(t, input, "news")
			boundaries := 0
			var held *genx.MessageChunk
			for {
				chunk, err := output.Next()
				if err != nil {
					t.Fatal(err)
				}
				if holdBoundary && chunk.Ctrl.MessageEnd {
					boundaries++
					if boundaries == 3 {
						held = chunk
						break
					}
				}
				if !holdBoundary && chunk.Part == genx.Text("section 3") {
					held = chunk
					break
				}
				observer.ObserveOutput(chunk)
			}
			before := requireCheckpointCursor(t, f, 2)
			if f.probe.attempts.Load() != 2 {
				t.Fatalf("unacknowledged generation reached CAS: %d attempts", f.probe.attempts.Load())
			}
			checkpointInput(t, input, "pause")
			// The final consumer releases its superseded claim without falsely
			// acknowledging delivery, matching MixerOutput's interruption path.
			observer.AbandonOutputObservation(held)
			if err := input.Done(genx.Usage{}); err != nil {
				t.Fatal(err)
			}
			chunks := drainCheckpointOutput(t, output, observer)
			interrupted := false
			for _, chunk := range chunks {
				if chunk.IsEndOfStream() && chunk.Ctrl.Error == "interrupted" {
					interrupted = true
				}
			}
			afterPause := requireCheckpointCursor(t, f, 2)
			if !interrupted || f.probe.attempts.Load() != 3 || afterPause.Version == before.Version {
				t.Fatalf("pause: interrupted=%t attempts=%d snapshot=%+v", interrupted, f.probe.attempts.Load(), afterPause)
			}
			records, err := checkpointHistory(t.Context(), f.history)
			if err != nil {
				t.Fatal(err)
			}
			prefixes := 0
			for _, record := range records {
				if record.Interrupted {
					prefixes++
				}
			}
			if holdBoundary && prefixes != 1 || !holdBoundary && prefixes != 0 {
				t.Fatalf("interrupted History entries=%d", prefixes)
			}
			// Rebuild the Transformer against the same real SQL State/History.
			resume := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
			checkpointInput(t, resume, "resume")
			if err := resume.Done(genx.Usage{}); err != nil {
				t.Fatal(err)
			}
			resumed, resumedObserver := checkpointOutput(t, f, resume.Stream())
			var text strings.Builder
			for _, chunk := range drainCheckpointOutput(t, resumed, resumedObserver) {
				if chunk.Ctrl.Error != "" {
					t.Fatal(chunk.Ctrl.Error)
				}
				if part, ok := chunk.Part.(genx.Text); ok {
					text.WriteString(string(part))
				}
			}
			final := requireCheckpointCursor(t, f, 5)
			if text.String() != "section 3section 4section 5" || f.probe.attempts.Load() != 6 || !reflect.DeepEqual(final.Fields["played"], []any{int64(1), int64(2), int64(3), int64(4), int64(5)}) {
				t.Fatalf("resume text=%q attempts=%d snapshot=%+v", text.String(), f.probe.attempts.Load(), final)
			}
		})
	}
}

type rejectingCheckpointHistoryReader struct{}

func (rejectingCheckpointHistoryReader) Query(context.Context, memory.HistoryQuery) (memory.HistoryPage, error) {
	return memory.HistoryPage{}, errors.New("History read failed")
}

func TestContinuationCheckpointRejectsFailedGeneration(t *testing.T) {
	for _, failure := range []string{"graph", "history", "conflict", "empty"} {
		t.Run(failure, func(t *testing.T) {
			f := newContinuationFixture(t, 2)
			wantCursor, wantAttempts, wantError := int64(1), int32(1), "generation failed"
			switch failure {
			case "graph", "empty":
				lambda := f.config.Lambdas.(checkpointLambdas)["narrate"]
				lambda.Lambda = compose.InvokableLambda(func(_ context.Context, input map[string]any) (map[string]any, error) {
					if failure == "empty" {
						return map[string]any{"text": ""}, nil
					}
					if input["cursor"] == int64(2) {
						return nil, errors.New("generation failed")
					}
					return map[string]any{"text": "section 1"}, nil
				})
				f.config.Lambdas.(checkpointLambdas)["narrate"] = lambda
				if failure == "empty" {
					wantCursor, wantAttempts, wantError = 0, 0, "narration"
				}
			case "history":
				f.config.History.Store = rejectingCheckpointHistoryReader{}
				wantCursor, wantAttempts, wantError = 0, 0, "History read failed"
			case "conflict":
				wantCursor, wantAttempts, wantError = 99, 2, "revision conflict"
				f.probe.before = func(ctx context.Context, scope, version string, fields map[string]any) error {
					if fields["cursor"] == int64(2) {
						_, err := f.state.CompareAndSwap(ctx, scope, version, map[string]any{"cursor": int64(99), "queue": []any{"one", "two", "three", "four", "five"}, "played": []any{int64(99)}})
						return err
					}
					return nil
				}
			}
			input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
			checkpointInput(t, input, "news")
			if err := input.Done(genx.Usage{}); err != nil {
				t.Fatal(err)
			}
			output, observer := checkpointOutput(t, f, input.Stream())
			var terminal string
			for _, chunk := range drainCheckpointOutput(t, output, observer) {
				if chunk.IsEndOfStream() {
					terminal = chunk.Ctrl.Error
				}
			}
			requireCheckpointCursor(t, f, wantCursor)
			if !strings.Contains(terminal, wantError) || f.probe.attempts.Load() != wantAttempts {
				t.Fatalf("failure=%s terminal=%q CAS attempts=%d, want %q/%d", failure, terminal, f.probe.attempts.Load(), wantError, wantAttempts)
			}
		})
	}
}

func TestContinuationCheckpointRejectsEmptyTerminalGeneration(t *testing.T) {
	for _, text := range []string{"", " \n\t"} {
		t.Run(fmt.Sprintf("text=%q", text), func(t *testing.T) {
			f := newContinuationFixture(t, 2)
			lambda := f.config.Lambdas.(checkpointLambdas)["narrate"]
			lambda.Lambda = compose.InvokableLambda(func(_ context.Context, input map[string]any) (map[string]any, error) {
				if input["cursor"] == int64(2) {
					return map[string]any{"text": text}, nil
				}
				return map[string]any{"text": "section 1"}, nil
			})
			f.config.Lambdas.(checkpointLambdas)["narrate"] = lambda
			input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
			checkpointInput(t, input, "news")
			if err := input.Done(genx.Usage{}); err != nil {
				t.Fatal(err)
			}
			output, observer := checkpointOutput(t, f, input.Stream())
			var terminal string
			for _, chunk := range drainCheckpointOutput(t, output, observer) {
				if chunk.IsEndOfStream() {
					terminal = chunk.Ctrl.Error
				}
			}
			if !strings.Contains(terminal, "narration") {
				t.Fatalf("empty terminal generation reported success: EOS=%q", terminal)
			}
			snapshot := requireCheckpointCursor(t, f, 1)
			if f.probe.attempts.Load() != 1 || !reflect.DeepEqual(snapshot.Fields["played"], []any{int64(1)}) {
				t.Fatalf("empty terminal generation advanced checkpoint: attempts=%d snapshot=%+v", f.probe.attempts.Load(), snapshot)
			}
		})
	}
}
