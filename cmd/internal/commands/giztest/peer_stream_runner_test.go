package giztestcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

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
	doc := &giztest.Document{
		Name: "session-cleanup", Path: "session.giztest.yaml", Repeat: 1,
		Variables: map[string]giztest.VariableSpec{
			"cleanup": {Direction: "input", Type: "string", Value: "ran"},
		},
		Clients: map[string]giztest.ClientSpec{"peer": {}},
		Steps: []giztest.Step{{ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{
			Mode: "realtime", Input: []byte{1}, Session: "microphone", KeepOpen: true,
			Completion: "first_response", FirstTextTimeout: "1s", RequireAudio: &requireAudio,
		}}},
		Finally: []giztest.Step{{ID: "cleanup", Output: &giztest.OutputOperation{Variable: "cleanup"}}},
	}
	go func() {
		for range 202 {
			<-stream.pushes
		}
		stream.in <- assistantText("assistant", "ready", false)
	}()
	testDriver := &driver{
		speechCache: newSpeechFixtureCache(),
		connectClients: func(context.Context, map[string]giztest.ClientSpec, []giztest.Step, *giztest.Variables) (*clientSet, error) {
			return &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}, nil
		},
		openPeerStream: func(*gizcli.Client) peerStreamOpener {
			return func() (peerStream, error) { return stream, nil }
		},
	}
	report := giztest.Run(context.Background(), []*giztest.Document{doc}, giztest.Options{
		Driver: testDriver, Parallel: 1, Out: writer,
	})
	if len(report.Tasks) != 1 {
		t.Fatalf("report = %#v", report)
	}
	result := report.Tasks[0]
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
	vars, err := giztest.NewVariables(map[string]giztest.VariableSpec{})
	if err != nil {
		t.Fatal(err)
	}
	clients := &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}
	step := giztest.Step{ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "text", Input: "hello", IdleTimeout: "40ms"}}
	testDriver := &driver{speechCache: newSpeechFixtureCache(), openPeerStream: func(*gizcli.Client) peerStreamOpener {
		return func() (peerStream, error) { return stream, nil }
	}}
	report, err := giztest.RunStep(context.Background(), "idle.giztest.yaml", step, testDriverSession(testDriver, clients), vars, nil, giztest.Options{Driver: testDriver, Out: io.Discard}, nil)
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
			vars, err := giztest.NewVariables(map[string]giztest.VariableSpec{})
			if err != nil {
				t.Fatal(err)
			}
			step := giztest.Step{
				ID: "relay",
				WorkspaceRelay: &giztest.WorkspaceRelayOperation{
					FirstClient: "tester", SecondClient: "candidate", Input: "brief", Media: "text", MaxTurns: 2, TerminalClient: "candidate",
				},
				Expect: map[string]giztest.Expectation{"/terminal/text": {Pattern: "^PASS$"}},
			}
			testDriver := &driver{speechCache: newSpeechFixtureCache(), fullEvidence: full, openRelayStreams: func() (relayStream, relayStream, error) {
				return tester, candidate, nil
			}}
			report, err := giztest.RunStep(context.Background(), "relay.giztest.yaml", step, testDriverSession(testDriver, &clientSet{}), vars, nil, giztest.Options{Driver: testDriver, Out: io.Discard}, nil)
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
	vars, err := giztest.NewVariables(map[string]giztest.VariableSpec{})
	if err != nil {
		t.Fatal(err)
	}
	step := giztest.Step{ID: "relay", WorkspaceRelay: &giztest.WorkspaceRelayOperation{
		FirstClient: "tester", SecondClient: "candidate", Input: "brief", Media: "text",
		IdleTimeout: "40ms", MaxTurns: 2, TerminalClient: "candidate",
	}}
	testDriver := &driver{speechCache: newSpeechFixtureCache(), openRelayStreams: func() (relayStream, relayStream, error) {
		return tester, candidate, nil
	}}
	type outcome struct {
		report giztest.StepReport
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		report, err := giztest.RunStep(context.Background(), "relay.giztest.yaml", step, testDriverSession(testDriver, &clientSet{}), vars, nil, giztest.Options{Driver: testDriver, Out: io.Discard}, nil)
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
		t.Fatalf("giztest.RunStep returned before relay readers exited: %#v", got)
	default:
	}
	close(readerExit)
	select {
	case got := <-done:
		if got.err == nil || got.report.Evidence["deadline"] != "idle_timeout" {
			t.Fatalf("report = %#v err = %v", got.report, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("giztest.RunStep did not return after relay readers exited")
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
			vars, _ := giztest.NewVariables(map[string]giztest.VariableSpec{})
			step := giztest.Step{
				ID: "relay",
				WorkspaceRelay: &giztest.WorkspaceRelayOperation{
					FirstClient: "tester", SecondClient: "candidate", Input: "brief", Media: "text", MaxTurns: 2, TerminalClient: "candidate",
				},
				Expect: map[string]giztest.Expectation{"/terminal/text": {Pattern: "^PASS$"}},
				Retry:  &giztest.RetrySpec{Attempts: 2, On: []string{"assertion"}},
			}
			testDriver := &driver{speechCache: newSpeechFixtureCache(), fullEvidence: full, openRelayStreams: func() (relayStream, relayStream, error) {
				pair := pairs[opened]
				opened++
				return pair[0], pair[1], nil
			}}
			report, err := giztest.RunStep(context.Background(), "relay.giztest.yaml", step, testDriverSession(testDriver, &clientSet{}), vars, nil, giztest.Options{Driver: testDriver, Out: io.Discard}, nil)
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
	vars, err := giztest.NewVariables(map[string]giztest.VariableSpec{"result": {Direction: "output", Type: "object"}})
	if err != nil {
		t.Fatal(err)
	}
	clients := &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}
	opened := 0
	step := giztest.Step{
		ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "text", Input: "hello"},
		SaveAs: "result", Expect: map[string]giztest.Expectation{"/text": {Contains: "PASS"}},
		Retry: &giztest.RetrySpec{Attempts: 2, On: []string{"assertion"}},
	}
	testDriver := &driver{speechCache: newSpeechFixtureCache(), openPeerStream: func(*gizcli.Client) peerStreamOpener {
		stream := streams[opened]
		opened++
		return func() (peerStream, error) { return stream, nil }
	}}
	report, err := giztest.RunStep(context.Background(), "retry.giztest.yaml", step, testDriverSession(testDriver, clients), vars, nil, giztest.Options{Driver: testDriver, Out: io.Discard}, nil)
	if err != nil || report.Status != "passed" || opened != 2 {
		t.Fatalf("report = %#v, opened = %d, err = %v", report, opened, err)
	}
	if len(report.Attempts) != 2 || report.Attempts[0].FailureKind != "assertion" || report.Attempts[1].Status != "passed" {
		t.Fatalf("attempts = %#v", report.Attempts)
	}
	if strings.Contains(report.Attempts[0].Error, "FAIL") || strings.Contains(report.Attempts[0].Error, "PASS") {
		t.Fatalf("attempt error leaks matcher content: %q", report.Attempts[0].Error)
	}
	stored, _ := vars.Value("result")
	result, _ := stored.(map[string]any)
	if text, _ := giztest.StringTarget(result["text"]); text != "PASS" {
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
	vars, _ := giztest.NewVariables(map[string]giztest.VariableSpec{})
	step := giztest.Step{ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "text", Input: "hello"}, ExpectError: &giztest.ErrorExpectation{Code: 7}, Retry: &giztest.RetrySpec{Attempts: 2, On: []string{"assertion"}}}
	testDriver := &driver{speechCache: newSpeechFixtureCache(), openPeerStream: func(*gizcli.Client) peerStreamOpener {
		stream := streams[opened]
		opened++
		return func() (peerStream, error) { return stream, nil }
	}}
	report, err := giztest.RunStep(context.Background(), "retry.giztest.yaml", step, testDriverSession(testDriver, clients), vars, nil, giztest.Options{Driver: testDriver, Out: io.Discard}, nil)
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
	vars, _ := giztest.NewVariables(map[string]giztest.VariableSpec{})
	step := giztest.Step{ID: "turn", Client: "peer", Timeout: "30ms", PeerStream: &giztest.PeerStreamOperation{Mode: "text", Input: "hello"}, Retry: &giztest.RetrySpec{Attempts: 2}}
	testDriver := &driver{speechCache: newSpeechFixtureCache(), openPeerStream: func(*gizcli.Client) peerStreamOpener {
		stream := streams[opened]
		opened++
		return func() (peerStream, error) { return stream, nil }
	}}
	report, err := giztest.RunStep(context.Background(), "retry.giztest.yaml", step, testDriverSession(testDriver, clients), vars, nil, giztest.Options{Driver: testDriver, Out: io.Discard}, nil)
	if err != nil || report.Status != "passed" || opened != 2 || report.Attempts[0].FailureKind != "timeout" {
		t.Fatalf("report = %#v, opened = %d, err = %v", report, opened, err)
	}
}

func TestRunStepRetryStopsOnOperationFailure(t *testing.T) {
	clients := &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}
	vars, _ := giztest.NewVariables(map[string]giztest.VariableSpec{})
	opened := 0
	step := giztest.Step{ID: "turn", Client: "missing", PeerStream: &giztest.PeerStreamOperation{Mode: "text", Input: "hello"}, ExpectError: &giztest.ErrorExpectation{Code: 7}, Retry: &giztest.RetrySpec{Attempts: 3, On: []string{"assertion"}}}
	testDriver := &driver{speechCache: newSpeechFixtureCache(), openPeerStream: func(*gizcli.Client) peerStreamOpener {
		opened++
		return func() (peerStream, error) { return nil, errors.New("provider unavailable") }
	}}
	report, err := giztest.RunStep(context.Background(), "retry.giztest.yaml", step, testDriverSession(testDriver, clients), vars, nil, giztest.Options{Driver: testDriver, Out: io.Discard}, nil)
	if err == nil || opened != 0 || len(report.Attempts) != 1 || report.Attempts[0].FailureKind != "operation" {
		t.Fatalf("report = %#v, opened = %d, err = %v", report, opened, err)
	}
}

func TestRunStepRetryDelayHonorsCancellation(t *testing.T) {
	stream := newFakeRelayStream()
	go func() {
		drainPushes(stream, 3)
		finishRetryTextTurn(stream, "FAIL")
	}()
	clients := &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}
	vars, _ := giztest.NewVariables(map[string]giztest.VariableSpec{})
	opened := 0
	step := giztest.Step{ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "text", Input: "hello"}, Expect: map[string]giztest.Expectation{"/text": {Contains: "PASS"}}, Retry: &giztest.RetrySpec{Attempts: 2, On: []string{"assertion"}, Delay: "1s"}}
	testDriver := &driver{speechCache: newSpeechFixtureCache(), openPeerStream: func(*gizcli.Client) peerStreamOpener {
		opened++
		return func() (peerStream, error) { return stream, nil }
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	report, err := giztest.RunStep(ctx, "retry.giztest.yaml", step, testDriverSession(testDriver, clients), vars, nil, giztest.Options{Driver: testDriver, Out: io.Discard}, nil)
	if !errors.Is(err, context.DeadlineExceeded) || opened != 1 || len(report.Attempts) != 1 {
		t.Fatalf("report = %#v, opened = %d, err = %v", report, opened, err)
	}
	if report.Error != report.Attempts[0].Error || report.Evidence["events"] != report.Attempts[0].Evidence["events"] {
		t.Fatalf("top-level report no longer reflects the last actual attempt: %#v", report)
	}
}
