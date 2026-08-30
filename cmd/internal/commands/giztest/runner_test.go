package giztest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestRunTaskRunsFinalizersAfterClientSetupFailure(t *testing.T) {
	var output bytes.Buffer
	doc := &Document{
		Name: "setup-failure", Path: "setup-failure.giztest.yaml", Repeat: 1,
		Variables: map[string]VariableSpec{
			"endpoint": {Direction: "input", Type: "string", Value: "http://invalid"},
			"evidence": {Direction: "input", Type: "string", Value: "cleanup-ran"},
		},
		Clients: map[string]ClientSpec{"peer": {Identity: "ephemeral", Connection: "webrtc", AccessPoint: "${endpoint}"}},
		Finally: []Step{{ID: "emit_cleanup", Output: &OutputOperation{Variable: "evidence"}}},
	}
	result := runTask(context.Background(), task{doc: doc}, runOptions{in: nil, out: &output})
	if result.Status != "failed" || len(result.Cleanup) != 1 || result.Cleanup[0].Status != "passed" {
		t.Fatalf("result = %#v", result)
	}
	if output.String() != "evidence=cleanup-ran\n" {
		t.Fatalf("cleanup output = %q", output.String())
	}
}

func TestPeerStreamSessionsAreTaskAndClientScoped(t *testing.T) {
	first := newPeerStreamSessions()
	second := newPeerStreamSessions()
	firstStream := &countingPeerStream{}
	secondStream := &countingPeerStream{}
	if err := first.add("microphone", newPeerStreamSession("peer-a", firstStream)); err != nil {
		t.Fatal(err)
	}
	if err := second.add("microphone", newPeerStreamSession("peer-a", secondStream)); err != nil {
		t.Fatalf("same session name in another task was rejected: %v", err)
	}
	if _, err := first.take("microphone", "peer-b"); err == nil || !strings.Contains(err.Error(), "belongs to client") {
		t.Fatalf("cross-client take error = %v", err)
	}
	if _, err := first.take("microphone", "peer-a"); err != nil {
		t.Fatalf("owning client could not consume session: %v", err)
	}
	if _, err := first.take("microphone", "peer-a"); err == nil || !strings.Contains(err.Error(), "is not open") {
		t.Fatalf("consumed session take error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if firstStream.closeCalls != 0 || secondStream.closeCalls != 1 {
		t.Fatalf("close calls first/second = %d/%d, want 0/1", firstStream.closeCalls, secondStream.closeCalls)
	}
}

func TestPeerStreamSessionsCloseExactlyOnceBeforeFinalizers(t *testing.T) {
	stream := &countingPeerStream{
		pushes: make(chan *genx.MessageChunk, 256),
		in:     make(chan *genx.MessageChunk, 4),
	}
	writer := &closeAwareWriter{stream: stream}
	requireAudio := false
	doc := &Document{
		Name: "session-cleanup", Path: "session.giztest.yaml", Repeat: 1,
		Variables: map[string]VariableSpec{
			"cleanup": {Direction: "input", Type: "string", Value: "ran"},
		},
		Clients: map[string]ClientSpec{"peer": {}},
		Steps: []Step{{ID: "turn", Client: "peer", PeerStream: &PeerStreamOperation{
			Mode: "realtime", Input: []byte{1}, Session: "microphone", KeepOpen: true,
			Completion: "first_response", FirstTextTimeout: "1s", RequireAudio: &requireAudio,
		}}},
		Finally: []Step{{ID: "cleanup", Output: &OutputOperation{Variable: "cleanup"}}},
	}
	go func() {
		for range 202 {
			<-stream.pushes
		}
		stream.in <- assistantText("assistant", "ready", false)
	}()
	result := runTask(context.Background(), task{doc: doc}, runOptions{
		out: writer,
		connectClients: func(context.Context, map[string]ClientSpec, []Step, *variables) (*clientSet, error) {
			return &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}, nil
		},
		openPeerStream: func(*gizcli.Client) peerStreamOpener {
			return func() (peerStream, error) { return stream, nil }
		},
	})
	if result.Status != "passed" || len(result.Cleanup) != 1 || result.Cleanup[0].Status != "passed" {
		t.Fatalf("result = %#v", result)
	}
	if stream.closeCalls != 1 {
		t.Fatalf("retained stream close calls = %d, want 1", stream.closeCalls)
	}
	if !writer.observedClosed {
		t.Fatal("finalizer ran before retained peer_stream session closed")
	}
}

type countingPeerStream struct {
	closeCalls int
	pushes     chan *genx.MessageChunk
	in         chan *genx.MessageChunk
}

func (s *countingPeerStream) Push(_ context.Context, chunk *genx.MessageChunk) error {
	s.pushes <- chunk
	return nil
}
func (s *countingPeerStream) Next() (*genx.MessageChunk, error) {
	if s.in == nil {
		return nil, io.EOF
	}
	return <-s.in, nil
}
func (s *countingPeerStream) Close() error {
	s.closeCalls++
	return nil
}

type closeAwareWriter struct {
	stream         *countingPeerStream
	observedClosed bool
}

func (w *closeAwareWriter) Write(p []byte) (int, error) {
	w.observedClosed = w.stream.closeCalls == 1
	return len(p), nil
}

func TestRunStepKeepsOperationEvidenceOnFailure(t *testing.T) {
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		stream.in <- assistantText("s1", "hello", false)
	}()
	vars, err := newVariables(map[string]VariableSpec{})
	if err != nil {
		t.Fatal(err)
	}
	clients := &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}
	step := Step{ID: "turn", Client: "peer", PeerStream: &PeerStreamOperation{Mode: "text", Input: "hello", IdleTimeout: "40ms"}}
	opts := runOptions{out: io.Discard, openPeerStream: func(*gizcli.Client) peerStreamOpener {
		return func() (peerStream, error) { return stream, nil }
	}}
	report, err := runStep(context.Background(), "idle.giztest.yaml", step, clients, vars, nil, opts, nil)
	if err == nil || report.Status != "failed" || !strings.Contains(report.Error, "peer_stream idle timeout exceeded") {
		t.Fatalf("report = %#v err = %v", report, err)
	}
	if report.Evidence["deadline"] != "idle_timeout" || report.Evidence["events"] != 1 {
		t.Fatalf("failed step evidence = %#v", report.Evidence)
	}
}

func TestRunStepSelectsWorkspaceRelayEvidenceOnAssertionFailure(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(fmt.Sprintf("full=%t", full), func(t *testing.T) {
			tester, candidate := newFakeRelayStream(), newFakeRelayStream()
			go func() {
				drainPushes(tester, 3)
				tester.in <- assistantText("t1", "question", true)
				drainPushes(candidate, 3)
				candidate.in <- assistantText("c1", "FAIL: actual verdict", true)
			}()
			vars, err := newVariables(map[string]VariableSpec{})
			if err != nil {
				t.Fatal(err)
			}
			step := Step{
				ID: "relay",
				WorkspaceRelay: &WorkspaceRelayOperation{
					FirstClient: "tester", SecondClient: "candidate", Input: "brief", Media: "text", MaxTurns: 2, TerminalClient: "candidate",
				},
				Expect: map[string]Expectation{"/terminal/text": {Pattern: "^PASS$"}},
			}
			opts := runOptions{out: io.Discard, fullEvidence: full, openRelayStreams: func() (relayStream, relayStream, error) {
				return tester, candidate, nil
			}}
			report, err := runStep(context.Background(), "relay.giztest.yaml", step, &clientSet{}, vars, nil, opts, nil)
			if err == nil || report.Status != "failed" || !strings.Contains(report.Error, "pattern failed") {
				t.Fatalf("report = %#v err = %v", report, err)
			}
			data, _ := json.Marshal(report.Evidence)
			if full {
				if !strings.Contains(string(data), "FAIL: actual verdict") {
					t.Fatalf("full evidence = %s", data)
				}
			} else if strings.Contains(string(data), "actual verdict") {
				t.Fatalf("redacted evidence = %s", data)
			}
		})
	}
}

func TestRunStepClosesWorkspaceRelayStreamsOnIdleTimeout(t *testing.T) {
	tester, candidate := newFakeRelayStream(), newFakeRelayStream()
	readerExit := make(chan struct{})
	tester.nextExitGate = readerExit
	candidate.nextExitGate = readerExit
	go drainPushes(tester, 3)
	vars, err := newVariables(map[string]VariableSpec{})
	if err != nil {
		t.Fatal(err)
	}
	step := Step{ID: "relay", WorkspaceRelay: &WorkspaceRelayOperation{
		FirstClient: "tester", SecondClient: "candidate", Input: "brief", Media: "text",
		IdleTimeout: "40ms", MaxTurns: 2, TerminalClient: "candidate",
	}}
	opts := runOptions{out: io.Discard, openRelayStreams: func() (relayStream, relayStream, error) {
		return tester, candidate, nil
	}}
	type outcome struct {
		report StepReport
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		report, err := runStep(context.Background(), "relay.giztest.yaml", step, &clientSet{}, vars, nil, opts, nil)
		done <- outcome{report: report, err: err}
	}()
	for name, stream := range map[string]*fakeRelayStream{"tester": tester, "candidate": candidate} {
		select {
		case <-stream.closed:
		case <-time.After(time.Second):
			t.Fatalf("%s stream was not closed", name)
		}
	}
	select {
	case got := <-done:
		t.Fatalf("runStep returned before relay readers exited: %#v", got)
	default:
	}
	close(readerExit)
	select {
	case got := <-done:
		if got.err == nil || got.report.Evidence["deadline"] != "idle_timeout" {
			t.Fatalf("report = %#v err = %v", got.report, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("runStep did not return after relay readers exited")
	}
	for name, stream := range map[string]*fakeRelayStream{"tester": tester, "candidate": candidate} {
		select {
		case <-stream.closed:
		default:
			t.Fatalf("%s stream was not closed", name)
		}
	}
}

func TestRunStepRetryPreservesSelectedWorkspaceRelayEvidence(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(fmt.Sprintf("full=%t", full), func(t *testing.T) {
			pairs := [][2]*fakeRelayStream{{newFakeRelayStream(), newFakeRelayStream()}, {newFakeRelayStream(), newFakeRelayStream()}}
			for i, pair := range pairs {
				verdict := "FAIL first attempt"
				if i == 1 {
					verdict = "PASS"
				}
				go func(tester, candidate *fakeRelayStream, terminal string) {
					drainPushes(tester, 3)
					tester.in <- assistantText("t1", "question", true)
					drainPushes(candidate, 3)
					candidate.in <- assistantText("c1", terminal, true)
				}(pair[0], pair[1], verdict)
			}
			opened := 0
			vars, _ := newVariables(map[string]VariableSpec{})
			step := Step{
				ID: "relay",
				WorkspaceRelay: &WorkspaceRelayOperation{
					FirstClient: "tester", SecondClient: "candidate", Input: "brief", Media: "text", MaxTurns: 2, TerminalClient: "candidate",
				},
				Expect: map[string]Expectation{"/terminal/text": {Pattern: "^PASS$"}},
				Retry:  &RetrySpec{Attempts: 2, On: []string{"assertion"}},
			}
			opts := runOptions{out: io.Discard, fullEvidence: full, openRelayStreams: func() (relayStream, relayStream, error) {
				pair := pairs[opened]
				opened++
				return pair[0], pair[1], nil
			}}
			report, err := runStep(context.Background(), "relay.giztest.yaml", step, &clientSet{}, vars, nil, opts, nil)
			if err != nil || report.Status != "passed" || len(report.Attempts) != 2 {
				t.Fatalf("report = %#v err = %v", report, err)
			}
			first, _ := json.Marshal(report.Attempts[0].Evidence)
			if full && !strings.Contains(string(first), "FAIL first attempt") {
				t.Fatalf("full retry evidence = %s", first)
			}
			if !full && strings.Contains(string(first), "FAIL first attempt") {
				t.Fatalf("redacted retry evidence = %s", first)
			}
		})
	}
}

func TestRunDocumentsAdmitsBarrierGroupAtomically(t *testing.T) {
	block := make(chan struct{})
	started := make(chan string, 3)
	var once sync.Once
	docBefore := &Document{Name: "before", Path: "before.giztest.yaml", Repeat: 1, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "review", ReviewOp: &ReviewOperation{Message: "before"}}}}
	docGroup := &Document{Name: "group", Path: "group.giztest.yaml", Repeat: 2, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "sync", Barrier: &BarrierOperation{Participants: 2}}}}
	in := &blockingReviewReader{block: block, started: started, once: &once}
	done := make(chan Report, 1)
	go func() {
		done <- runDocuments(context.Background(), []*Document{docBefore, docGroup}, runOptions{parallel: 2, in: in, out: io.Discard})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("leading task did not start")
	}
	select {
	case report := <-done:
		t.Fatalf("barrier group started before two slots were available: %#v", report)
	case <-time.After(50 * time.Millisecond):
	}
	close(block)
	select {
	case report := <-done:
		if report.Status != "passed" {
			t.Fatalf("report = %#v", report)
		}
		for _, result := range report.Tasks {
			if result.Name == "group" && result.DurationMS >= 25 {
				t.Fatalf("barrier task was partially admitted for %dms", result.DurationMS)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("run did not finish")
	}
}

type blockingReviewReader struct {
	block   <-chan struct{}
	started chan<- string
	once    *sync.Once
}

func (r *blockingReviewReader) Read(p []byte) (int, error) {
	r.once.Do(func() { r.started <- "started" })
	<-r.block
	copy(p, "pass\n")
	return 5, nil
}

func TestRunDocumentsRejectsInsufficientBarrierCapacityOffline(t *testing.T) {
	doc := &Document{Name: "benchmark.case", Path: "case.giztest.yaml", Repeat: 2, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "sync", Barrier: &BarrierOperation{Participants: 2}}}}
	report := runDocuments(context.Background(), []*Document{doc}, runOptions{parallel: 1, in: nil, out: io.Discard})
	if report.Status != "failed" || len(report.Tasks) != 2 {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunDocumentsReportsEveryCancelledTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	doc := &Document{Name: "cancelled", Path: "cancelled.giztest.yaml", Repeat: 3, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "sync", Barrier: &BarrierOperation{Participants: 3}}}}
	report := runDocuments(ctx, []*Document{doc}, runOptions{parallel: 3, out: io.Discard})
	if len(report.Tasks) != 3 || report.Status != "failed" {
		t.Fatalf("report = %#v", report)
	}
}

func TestAssertValueExtendedMatchers(t *testing.T) {
	value := map[string]any{
		"text":          []string{"乌龟把种", "子交给小鸟，", "小鸟种下了种子。"},
		"joined":        "PASS",
		"fragments_any": []any{"hello ", "world"},
		"first_text_ms": int64(1200),
		"ratio":         2.5,
		"server_time":   "1755763200123",
		"nan_string":    "NaN",
		"inf_string":    "+Inf",
		"nan_float":     math.NaN(),
	}
	minInt := func(v int) *int { return &v }
	minFloat := func(v float64) *float64 { return &v }
	pass := map[string]map[string]Expectation{
		"contains on joined fragments":  {"/text": {Contains: "小鸟种下"}},
		"contains_all across fragments": {"/text": {ContainsAll: []string{"乌龟", "小鸟", "种子"}}},
		"contains_any single hit":       {"/text": {ContainsAny: []string{"发芽", "种子"}}},
		"not_contains clean":            {"/text": {NotContains: []any{"###", "```"}}},
		"not_contains scalar operand":   {"/joined": {NotContains: "FAIL"}},
		"pattern on plain string":       {"/joined": {Pattern: "^PASS$"}},
		"rune window":                   {"/text": {MinLength: minInt(10), MaxLength: minInt(20)}},
		"maximum on int64":              {"/first_text_ms": {Maximum: minFloat(6000)}},
		"minimum on float":              {"/ratio": {Minimum: minFloat(1)}},
		"bounds on protojson int64":     {"/server_time": {Minimum: minFloat(1), Maximum: minFloat(1e15)}},
		"combined matchers":             {"/joined": {Contains: "PAS", Pattern: "^[A-Z]+$", MaxLength: minInt(4)}},
		"any-typed fragment join":       {"/fragments_any": {Contains: "hello world"}},
	}
	for name, assertions := range pass {
		t.Run("pass "+name, func(t *testing.T) {
			if err := assertValue(assertions, value); err != nil {
				t.Fatalf("assertValue() error = %v", err)
			}
		})
	}
	fail := map[string]map[string]Expectation{
		"contains missing":           {"/text": {Contains: "恐龙"}},
		"contains_all partial":       {"/text": {ContainsAll: []string{"乌龟", "恐龙"}}},
		"contains_any all missing":   {"/text": {ContainsAny: []string{"恐龙", "大象"}}},
		"not_contains hit":           {"/text": {NotContains: []any{"种子"}}},
		"pattern mismatch":           {"/joined": {Pattern: "^FAIL$"}},
		"min_length short":           {"/joined": {MinLength: minInt(10)}},
		"max_length long":            {"/text": {MaxLength: minInt(3)}},
		"maximum exceeded":           {"/first_text_ms": {Maximum: minFloat(1000)}},
		"minimum unmet":              {"/ratio": {Minimum: minFloat(3)}},
		"string matcher on number":   {"/first_text_ms": {Contains: "1"}},
		"numeric matcher on string":  {"/joined": {Maximum: minFloat(1)}},
		"numeric string above bound": {"/server_time": {Maximum: minFloat(10)}},
		"NaN string never satisfies": {"/nan_string": {Minimum: minFloat(0), Maximum: minFloat(1)}},
		"Inf string never satisfies": {"/inf_string": {Minimum: minFloat(0)}},
		"NaN float never satisfies":  {"/nan_float": {Maximum: minFloat(1)}},
	}
	for name, assertions := range fail {
		t.Run("fail "+name, func(t *testing.T) {
			if err := assertValue(assertions, value); err == nil {
				t.Fatal("assertValue() accepted a failing matcher")
			}
		})
	}
}

func TestAssertValueContentMatcherFailuresAreContentFree(t *testing.T) {
	value := map[string]any{"text": []string{"secret story about 乌龟"}}
	assertions := map[string]Expectation{"/text": {NotContains: []any{"乌龟"}, Contains: "乌龟"}}
	err := assertValue(assertions, value)
	if err == nil {
		t.Fatal("assertValue() accepted a failing matcher")
	}
	for _, banned := range []string{"secret", "乌龟"} {
		if strings.Contains(err.Error(), banned) {
			t.Fatalf("failure message leaks content: %v", err)
		}
	}
}

func TestAssertValueNormalizedMatchers(t *testing.T) {
	value := map[string]any{
		"text":    []any{"下午 四点，", "G"},
		"route":   "今天的观点？",
		"verdict": "ＰＡＳＳ。",
	}
	all := []string{"whitespace", "punctuation", "case", "digits"}
	pass := map[string]Expectation{
		"/text":    {ContainsAll: []string{"四点", "g"}, ContainsAny: []string{"missing", "四 点"}, NotContains: []any{"五点"}, Normalize: all},
		"/route":   {Contains: "今天的观点?", Normalize: []string{"punctuation"}, MinLength: new(6)},
		"/verdict": {Equals: "ｐａｓｓ", Normalize: []string{"case", "punctuation"}},
	}
	if err := assertValue(pass, value); err != nil {
		t.Fatalf("normalized assertion failed: %v", err)
	}
	if err := assertValue(map[string]Expectation{"/route": {Equals: "今天的观点?"}}, value); err == nil {
		t.Fatal("byte-exact equals unexpectedly normalized punctuation")
	}
	if err := assertValue(map[string]Expectation{"/text": {MaxLength: new(5), Contains: "四点", Normalize: []string{"whitespace", "punctuation"}}}, value); err == nil || !strings.Contains(err.Error(), "max_length") {
		t.Fatalf("raw length matcher did not observe original text: %v", err)
	}
	for name, test := range map[string]struct {
		option string
		want   string
	}{
		"whitespace only":  {option: "whitespace", want: "A？１"},
		"punctuation only": {option: "punctuation", want: " A１"},
		"case only":        {option: "case", want: " a？１"},
		"digits only":      {option: "digits", want: " A？1"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := normalizeString(" A？１", []string{test.option}); got != test.want {
				t.Fatalf("normalizeString() = %q, want %q", got, test.want)
			}
		})
	}
	for _, options := range [][]string{{"digits", "case"}, {"case", "digits"}} {
		if got := normalizeString("Ａ１２三", options); got != "ａ123" {
			t.Fatalf("normalizeString() = %q", got)
		}
	}
}

func TestFailureKind(t *testing.T) {
	tests := map[string]struct {
		err  error
		want string
	}{
		"none":              {want: ""},
		"assertion":         {err: &assertionFailure{err: context.DeadlineExceeded}, want: "assertion"},
		"wrapped timeout":   {err: fmt.Errorf("step timed out: %w", context.DeadlineExceeded), want: "timeout"},
		"wrapped cancelled": {err: fmt.Errorf("step cancelled: %w", context.Canceled), want: "cancelled"},
		"operation":         {err: errors.New("provider rejected request"), want: "operation"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := failureKind(test.err); got != test.want {
				t.Fatalf("failureKind() = %q, want %q", got, test.want)
			}
		})
	}
}

func finishRetryTextTurn(stream *fakeRelayStream, text string) {
	stream.in <- assistantText("turn", text, false)
	stream.in <- assistantBlob("turn", []byte{1}, false)
	stream.in <- assistantText("turn", "", true)
	stream.in <- assistantBlob("turn", nil, true)
}

func TestRunStepRetriesAssertionAndCommitsOnlyWinningOutput(t *testing.T) {
	streams := []*fakeRelayStream{newFakeRelayStream(), newFakeRelayStream()}
	for index, stream := range streams {
		text := "FAIL"
		if index == 1 {
			text = "PASS"
		}
		go func() {
			drainPushes(stream, 3)
			finishRetryTextTurn(stream, text)
		}()
	}
	vars, err := newVariables(map[string]VariableSpec{"result": {Direction: "output", Type: "object"}})
	if err != nil {
		t.Fatal(err)
	}
	clients := &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}
	opened := 0
	step := Step{
		ID: "turn", Client: "peer", PeerStream: &PeerStreamOperation{Mode: "text", Input: "hello"},
		SaveAs: "result", Expect: map[string]Expectation{"/text": {Contains: "PASS"}},
		Retry: &RetrySpec{Attempts: 2, On: []string{"assertion"}},
	}
	opts := runOptions{out: io.Discard, openPeerStream: func(*gizcli.Client) peerStreamOpener {
		stream := streams[opened]
		opened++
		return func() (peerStream, error) { return stream, nil }
	}}
	report, err := runStep(context.Background(), "retry.giztest.yaml", step, clients, vars, nil, opts, nil)
	if err != nil || report.Status != "passed" || opened != 2 {
		t.Fatalf("report = %#v, opened = %d, err = %v", report, opened, err)
	}
	if len(report.Attempts) != 2 || report.Attempts[0].FailureKind != "assertion" || report.Attempts[1].Status != "passed" {
		t.Fatalf("attempts = %#v", report.Attempts)
	}
	if strings.Contains(report.Attempts[0].Error, "FAIL") || strings.Contains(report.Attempts[0].Error, "PASS") {
		t.Fatalf("attempt error leaks matcher content: %q", report.Attempts[0].Error)
	}
	result, _ := vars.values["result"].data.(map[string]any)
	if text, _ := stringTarget(result["text"]); text != "PASS" {
		t.Fatalf("winning output = %#v", result)
	}
}

func TestRunStepRetryReportsExhaustedAttempts(t *testing.T) {
	streams := []*fakeRelayStream{newFakeRelayStream(), newFakeRelayStream()}
	for _, stream := range streams {
		go func() {
			drainPushes(stream, 3)
			finishRetryTextTurn(stream, "FAIL")
		}()
	}
	opened := 0
	clients := &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}
	vars, _ := newVariables(map[string]VariableSpec{})
	step := Step{ID: "turn", Client: "peer", PeerStream: &PeerStreamOperation{Mode: "text", Input: "hello"}, ExpectError: &ErrorExpectation{Code: 7}, Retry: &RetrySpec{Attempts: 2, On: []string{"assertion"}}}
	opts := runOptions{out: io.Discard, openPeerStream: func(*gizcli.Client) peerStreamOpener {
		stream := streams[opened]
		opened++
		return func() (peerStream, error) { return stream, nil }
	}}
	report, err := runStep(context.Background(), "retry.giztest.yaml", step, clients, vars, nil, opts, nil)
	if err == nil || report.Status != "failed" || opened != 2 || len(report.Attempts) != 2 {
		t.Fatalf("report = %#v, opened = %d, err = %v", report, opened, err)
	}
	for _, attempt := range report.Attempts {
		if attempt.FailureKind != "assertion" || attempt.Status != "failed" {
			t.Fatalf("attempt = %#v", attempt)
		}
	}
}

func TestRunStepRetriesTimeoutWithFreshStepDeadline(t *testing.T) {
	first := newFakeRelayStream()
	second := newFakeRelayStream()
	go func() {
		drainPushes(first, 3)
	}()
	go func() {
		drainPushes(second, 3)
		finishRetryTextTurn(second, "PASS")
	}()
	streams := []*fakeRelayStream{first, second}
	opened := 0
	clients := &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}
	vars, _ := newVariables(map[string]VariableSpec{})
	step := Step{ID: "turn", Client: "peer", Timeout: "30ms", PeerStream: &PeerStreamOperation{Mode: "text", Input: "hello"}, Retry: &RetrySpec{Attempts: 2}}
	opts := runOptions{out: io.Discard, openPeerStream: func(*gizcli.Client) peerStreamOpener {
		stream := streams[opened]
		opened++
		return func() (peerStream, error) { return stream, nil }
	}}
	report, err := runStep(context.Background(), "retry.giztest.yaml", step, clients, vars, nil, opts, nil)
	if err != nil || report.Status != "passed" || opened != 2 || report.Attempts[0].FailureKind != "timeout" {
		t.Fatalf("report = %#v, opened = %d, err = %v", report, opened, err)
	}
}

func TestRunStepRetryStopsOnOperationFailure(t *testing.T) {
	clients := &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}
	vars, _ := newVariables(map[string]VariableSpec{})
	opened := 0
	step := Step{ID: "turn", Client: "missing", PeerStream: &PeerStreamOperation{Mode: "text", Input: "hello"}, ExpectError: &ErrorExpectation{Code: 7}, Retry: &RetrySpec{Attempts: 3, On: []string{"assertion"}}}
	opts := runOptions{out: io.Discard, openPeerStream: func(*gizcli.Client) peerStreamOpener {
		opened++
		return func() (peerStream, error) { return nil, errors.New("provider unavailable") }
	}}
	report, err := runStep(context.Background(), "retry.giztest.yaml", step, clients, vars, nil, opts, nil)
	if err == nil || opened != 0 || len(report.Attempts) != 1 || report.Attempts[0].FailureKind != "operation" {
		t.Fatalf("report = %#v, opened = %d, err = %v", report, opened, err)
	}
}

func TestRunStepRetriesOperationAfterReconnect(t *testing.T) {
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		finishRetryTextTurn(stream, "PASS")
	}()
	clients := &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}
	vars, _ := newVariables(map[string]VariableSpec{})
	opened := 0
	reconnected := 0
	step := Step{ID: "turn", Client: "peer", PeerStream: &PeerStreamOperation{Mode: "text", Input: "hello"}, Retry: &RetrySpec{Attempts: 2, On: []string{"operation"}}}
	opts := runOptions{
		out: io.Discard,
		openPeerStream: func(*gizcli.Client) peerStreamOpener {
			opened++
			if opened == 1 {
				return func() (peerStream, error) { return nil, io.EOF }
			}
			return func() (peerStream, error) { return stream, nil }
		},
		reconnectClient: func(context.Context, string) error {
			reconnected++
			return nil
		},
	}
	report, err := runStep(context.Background(), "retry.giztest.yaml", step, clients, vars, nil, opts, nil)
	if err != nil || report.Status != "passed" || opened != 2 || reconnected != 1 || report.Attempts[0].FailureKind != "operation" {
		t.Fatalf("report = %#v, opened = %d, reconnected = %d, err = %v", report, opened, reconnected, err)
	}
}

func TestRunStepRetriesProviderOperationWithoutReconnect(t *testing.T) {
	streams := []*fakeRelayStream{newFakeRelayStream(), newFakeRelayStream()}
	go func() {
		drainPushes(streams[0], 3)
		streams[0].in <- &genx.MessageChunk{Ctrl: &genx.StreamCtrl{
			StreamID: "turn", Label: "assistant", EndOfStream: true,
			Error: "provider connection closed without audio",
		}}
	}()
	go func() {
		drainPushes(streams[1], 3)
		finishRetryTextTurn(streams[1], "PASS")
	}()
	clients := &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}
	vars, _ := newVariables(map[string]VariableSpec{})
	opened := 0
	reconnected := 0
	step := Step{ID: "turn", Client: "peer", PeerStream: &PeerStreamOperation{Mode: "text", Input: "hello"}, Retry: &RetrySpec{Attempts: 2, On: []string{"operation"}}}
	opts := runOptions{
		out: io.Discard,
		openPeerStream: func(*gizcli.Client) peerStreamOpener {
			stream := streams[opened]
			opened++
			return func() (peerStream, error) { return stream, nil }
		},
		reconnectClient: func(context.Context, string) error {
			reconnected++
			return nil
		},
	}
	report, err := runStep(context.Background(), "retry.giztest.yaml", step, clients, vars, nil, opts, nil)
	if err != nil || report.Status != "passed" || opened != 2 || reconnected != 0 || report.Attempts[0].FailureKind != "operation" {
		t.Fatalf("report = %#v, opened = %d, reconnected = %d, err = %v", report, opened, reconnected, err)
	}
}

func TestRunStepRetryDelayHonorsCancellation(t *testing.T) {
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		finishRetryTextTurn(stream, "FAIL")
	}()
	clients := &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}
	vars, _ := newVariables(map[string]VariableSpec{})
	opened := 0
	step := Step{ID: "turn", Client: "peer", PeerStream: &PeerStreamOperation{Mode: "text", Input: "hello"}, Expect: map[string]Expectation{"/text": {Contains: "PASS"}}, Retry: &RetrySpec{Attempts: 2, On: []string{"assertion"}, Delay: "1s"}}
	opts := runOptions{out: io.Discard, openPeerStream: func(*gizcli.Client) peerStreamOpener {
		opened++
		return func() (peerStream, error) { return stream, nil }
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	report, err := runStep(ctx, "retry.giztest.yaml", step, clients, vars, nil, opts, nil)
	if !errors.Is(err, context.DeadlineExceeded) || opened != 1 || len(report.Attempts) != 1 {
		t.Fatalf("report = %#v, opened = %d, err = %v", report, opened, err)
	}
	if report.Error != report.Attempts[0].Error || report.Evidence["events"] != report.Attempts[0].Evidence["events"] {
		t.Fatalf("top-level report no longer reflects the last actual attempt: %#v", report)
	}
}

func TestRunStepRetryDelayPrecedesTransportReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	clients := &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}
	vars, _ := newVariables(map[string]VariableSpec{})
	reconnected := 0
	step := Step{
		ID: "turn", Client: "peer",
		PeerStream: &PeerStreamOperation{Mode: "text", Input: "hello"},
		Retry:      &RetrySpec{Attempts: 2, On: []string{"operation"}, Delay: "1s"},
	}
	opts := runOptions{
		out: io.Discard,
		openPeerStream: func(*gizcli.Client) peerStreamOpener {
			return func() (peerStream, error) { return nil, io.EOF }
		},
		reconnectClient: func(context.Context, string) error {
			reconnected++
			return nil
		},
	}
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	report, err := runStep(ctx, "retry.giztest.yaml", step, clients, vars, nil, opts, nil)
	if !errors.Is(err, context.Canceled) || reconnected != 0 || len(report.Attempts) != 1 {
		t.Fatalf("report = %#v, reconnected = %d, err = %v", report, reconnected, err)
	}
}

func TestAttemptVariableCopiesClearFailedBytes(t *testing.T) {
	input := []byte{9, 8}
	vars, _ := newVariables(map[string]VariableSpec{
		"input": {Direction: "input", Type: "audio", Value: input, MaxBytes: 4, MediaType: "audio/ogg", Codec: "opus"},
		"audio": {Direction: "output", Type: "audio", MaxBytes: 4, MediaType: "audio/ogg", Codec: "opus"},
	})
	attempt := cloneVariables(vars)
	buffer := []byte{1, 2, 3}
	if err := attempt.assign("audio", buffer); err != nil {
		t.Fatal(err)
	}
	releaseAttemptVariables(vars, attempt)
	if buffer[0] != 0 || vars.values["audio"].data != nil || input[0] != 9 {
		t.Fatalf("failed buffer = %v, committed = %#v, input = %v", buffer, vars.values["audio"].data, input)
	}
}

func TestAttemptVariableCopiesClearNestedFailedBytes(t *testing.T) {
	vars, _ := newVariables(map[string]VariableSpec{
		"result": {Direction: "output", Type: "object"},
	})
	attempt := cloneVariables(vars)
	buffer := []byte{1, 2, 3}
	object := map[string]any{"parts": []any{buffer}}
	object["cycle"] = object
	if err := attempt.assign("result", object); err != nil {
		t.Fatal(err)
	}
	releaseAttemptVariables(vars, attempt)
	if buffer[0] != 0 || object["parts"] != nil || object["cycle"] != nil || vars.values["result"].data != nil {
		t.Fatalf("discarded object was not cleared: buffer = %v, object = %#v", buffer, object)
	}
}

func TestAttemptVariableCopiesPreserveAliasedInputBytes(t *testing.T) {
	buffer := []byte{1, 2, 3}
	input := map[string]any{"parts": []any{buffer}}
	vars, _ := newVariables(map[string]VariableSpec{
		"input":  {Direction: "input", Type: "object", Value: input},
		"result": {Direction: "output", Type: "object"},
	})
	attempt := cloneVariables(vars)
	if err := attempt.assign("result", input); err != nil {
		t.Fatal(err)
	}
	releaseAttemptVariables(vars, attempt)
	if buffer[0] != 1 || input["parts"] == nil || vars.values["result"].data != nil {
		t.Fatalf("input alias was cleared: buffer = %v, input = %#v", buffer, input)
	}
}

func TestAttemptVariableCommitTransfersNestedBytes(t *testing.T) {
	vars, _ := newVariables(map[string]VariableSpec{
		"result": {Direction: "output", Type: "object"},
	})
	attempt := cloneVariables(vars)
	buffer := []byte{1, 2, 3}
	object := map[string]any{"parts": []any{buffer}}
	if err := attempt.assign("result", object); err != nil {
		t.Fatal(err)
	}
	if err := commitAttemptVariables(vars, attempt); err != nil {
		t.Fatal(err)
	}
	releaseAttemptVariables(vars, attempt)
	if buffer[0] != 1 || vars.values["result"].data == nil {
		t.Fatalf("committed object was cleared: buffer = %v, committed = %#v", buffer, vars.values["result"].data)
	}
}

func TestCommitAttemptVariablesValidatesBeforePublishing(t *testing.T) {
	vars, _ := newVariables(map[string]VariableSpec{
		"first":  {Direction: "output", Type: "string"},
		"second": {Direction: "output", Type: "integer"},
	})
	attempt := cloneVariables(vars)
	first := attempt.values["first"]
	first.data = "ready"
	attempt.values["first"] = first
	second := attempt.values["second"]
	second.data = "wrong type"
	attempt.values["second"] = second
	if err := commitAttemptVariables(vars, attempt); err == nil {
		t.Fatal("commitAttemptVariables() accepted an invalid pending value")
	}
	if vars.values["first"].data != nil || vars.values["second"].data != nil {
		t.Fatalf("partial outputs published: %#v", vars.values)
	}
}
