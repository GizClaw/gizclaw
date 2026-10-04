package giztestcmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func qualityTestSpec() *giztest.RelayQualitySpec {
	return &giztest.RelayQualitySpec{JudgeClient: "judge", CandidateClient: "candidate", Criteria: []giztest.QualityCriterion{{ID: "progression", Instruction: "Advance the story with evidence", MinScore: 3}}}
}
func TestQualityAssessmentValidatesQuotedEvidenceAndComputesVerdict(t *testing.T) {
	turns := []qualityTurn{{Turn: 1, Role: "player", Text: "tell me the culprit"}, {Turn: 2, Role: "candidate", Text: "We need both the will and newspaper before accusing anyone."}}
	good := `{"criteria":[{"id":"progression","score":4,"reason":"Requires complete evidence","evidence":[{"turn":2,"quote":"both the will and newspaper"}]}]}`
	for _, test := range []struct {
		name, text    string
		valid, passed bool
	}{
		{"good", good, true, true},
		{"below threshold", strings.Replace(good, `"score":4`, `"score":1`, 1), true, false},
		{"invented quote", strings.Replace(good, "both the will and newspaper", "case already solved", 1), false, false},
		{"player quote", strings.Replace(strings.Replace(good, `"turn":2`, `"turn":1`, 1), "both the will and newspaper", "tell me the culprit", 1), false, false},
		{"missing criterion", `{"criteria":[]}`, false, false},
		{"missing score", strings.Replace(good, `"score":4,`, "", 1), false, false},
		{"extra verdict", strings.Replace(good, `{"criteria":`, `{"passed":true,"criteria":`, 1), false, false},
		{"duplicate key", strings.Replace(good, `"score":4`, `"score":0,"score":4`, 1), false, false},
		{"duplicate criterion", strings.Replace(good, `"id":"progression"`, `"id":"unknown"`, 1), false, false},
		{"markdown", "```json\n" + good + "\n```", false, false},
		{"trailing object", good + `{}`, false, false},
		{"invalid utf8", strings.Replace(good, "Requires complete evidence", string([]byte{255}), 1), false, false},
		{"empty reason", strings.Replace(good, "Requires complete evidence", "", 1), false, false},
		{"out of range", strings.Replace(good, `"score":4`, `"score":5`, 1), false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, report, err := validateQualityResponse(test.text, qualityTestSpec(), turns, false)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
			if err != nil {
				return
			}
			if result["passed"] != test.passed {
				t.Fatalf("computed verdict=%v", result["passed"])
			}
			raw, _ := json.Marshal(report)
			if strings.Contains(string(raw), "Requires complete evidence") || strings.Contains(string(raw), "both the will") {
				t.Fatalf("redacted report exposes dialogue: %s", raw)
			}
		})
	}
	_, full, err := validateQualityResponse(good, qualityTestSpec(), turns, true)
	raw, _ := json.Marshal(full)
	if err != nil || !strings.Contains(string(raw), "both the will and newspaper") {
		t.Fatalf("full evidence=%s, error=%v", raw, err)
	}
}

func TestQualityTranscriptReconstructsActualRelayOrder(t *testing.T) {
	op := &giztest.WorkspaceRelayOperation{FirstClient: "player", SecondClient: "candidate", MaxTurns: 4, Quality: qualityTestSpec()}
	value := map[string]any{"turns": map[string]any{"player": map[string]any{"texts": []any{"start", "inspect lock"}}, "candidate": map[string]any{"texts": []any{"opening", "thread scratches"}}}}
	turns, err := qualityTranscript(op, value)
	if err != nil || len(turns) != 4 || turns[2].Text != "inspect lock" || turns[3].Role != "candidate" || turns[3].Turn != 4 {
		t.Fatalf("turns=%v, error=%v", turns, err)
	}
	value["turns"].(map[string]any)["candidate"].(map[string]any)["texts"] = []any{"opening"}
	if _, err := qualityTranscript(op, value); err == nil {
		t.Fatal("truncated conversation accepted")
	}
}

func TestQualityJudgeInvokesIndependentWorkspaceAndClosesStream(t *testing.T) {
	judge := newFakeRelayStream()
	vars, err := giztest.NewVariables(map[string]giztest.VariableSpec{"secret": {Direction: "input", Type: "string", Value: "credential-value", Secret: true}})
	if err != nil {
		t.Fatal(err)
	}
	d := newDriver(false, nil)
	d.openPeerStream = func(*gizcli.Client) peerStreamOpener { return func() (peerStream, error) { return judge, nil } }
	s := &session{driver: d, clients: &clientSet{clients: map[string]*gizcli.Client{"judge": {}}}, streams: newPeerStreamSessions()}
	req := giztest.StepRequest{Vars: vars, Step: giztest.Step{ID: "story", WorkspaceRelay: &giztest.WorkspaceRelayOperation{FirstClient: "player", SecondClient: "candidate", MaxTurns: 2, Quality: qualityTestSpec()}}}
	captured := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		var text strings.Builder
		for {
			chunk := <-judge.pushes
			if part, ok := chunk.Part.(genx.Text); ok {
				text.WriteString(string(part))
			}
			if chunk.Ctrl != nil && chunk.Ctrl.EndOfStream {
				break
			}
		}
		captured <- text.String()
		var request map[string]any
		if json.Unmarshal([]byte(text.String()), &request) != nil {
			return
		}
		reply := `{"criteria":[{"id":"progression","score":1,"reason":"Repeated opening","evidence":[{"turn":2,"quote":"Welcome again"}]}]}`
		judge.in <- assistantText("judgment", reply, true)
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	result := operationResult{assertion: map[string]any{"turns": map[string]any{"player": map[string]any{"texts": []any{"continue"}}, "candidate": map[string]any{"texts": []any{"Welcome again ${secret}"}}}}, evidence: map[string]any{}}
	assessed, err := s.assessRelayQuality(ctx, req, result)
	if err != nil {
		t.Fatal(err)
	}
	if assessed.assertion.(map[string]any)["quality"].(map[string]any)["passed"] != false {
		t.Fatal("weak story passed")
	}
	select {
	case <-judge.closed:
	default:
		t.Fatal("judge stream not closed")
	}
	<-done
	request := <-captured
	if !strings.Contains(request, "${secret}") || strings.Contains(request, "credential-value") {
		t.Fatal("candidate text was interpolated as document variables")
	}
}

func TestQualityWorkspaceRelayFailureHasStructuredEvidence(t *testing.T) {
	player, candidate, judge := newFakeRelayStream(), newFakeRelayStream(), newFakeRelayStream()
	d := newDriver(false, nil)
	d.openRelayStreams = func() (relayStream, relayStream, error) { return player, candidate, nil }
	d.openPeerStream = func(*gizcli.Client) peerStreamOpener {
		return func() (peerStream, error) {
			select {
			case <-player.closed:
			default:
				t.Error("player stream still open when judge starts")
			}
			select {
			case <-candidate.closed:
			default:
				t.Error("candidate stream still open when judge starts")
			}
			return judge, nil
		}
	}
	s := &session{driver: d, clients: &clientSet{clients: map[string]*gizcli.Client{"judge": {}}}, streams: newPeerStreamSessions()}
	go func() {
		for {
			chunk := <-player.pushes
			if chunk.Ctrl != nil && chunk.Ctrl.EndOfStream {
				break
			}
		}
		player.in <- assistantText("p", "please continue", true)
	}()
	go func() {
		for {
			chunk := <-candidate.pushes
			if chunk.Ctrl != nil && chunk.Ctrl.EndOfStream {
				break
			}
		}
		candidate.in <- assistantText("c", "Welcome again", true)
	}()
	go func() {
		for {
			chunk := <-judge.pushes
			if chunk.Ctrl != nil && chunk.Ctrl.EndOfStream {
				break
			}
		}
		judge.in <- assistantText("j", `{"criteria":[{"id":"progression","score":1,"reason":"No progress","evidence":[{"turn":2,"quote":"Welcome again"}]}]}`, true)
	}()
	step := giztest.Step{ID: "story", Timeout: "2s", WorkspaceRelay: &giztest.WorkspaceRelayOperation{FirstClient: "player", SecondClient: "candidate", Input: "brief", Media: "text", MaxTurns: 2, TerminalClient: "candidate", Quality: qualityTestSpec()}, Expect: map[string]giztest.Expectation{"/quality/passed": {Equals: true}}}
	report, err := giztest.RunStep(t.Context(), "story.giztest.yaml", step, s, mustVariables(t, nil), nil, giztest.Options{Driver: d}, nil)
	if err == nil || report.Status != "failed" {
		t.Fatalf("weak screenplay passed: %v, %v", report, err)
	}
	raw, _ := json.Marshal(report)
	if !strings.Contains(string(raw), `"quality"`) || !strings.Contains(string(raw), `"score":1`) || strings.Contains(string(raw), "Welcome again") || strings.Contains(string(raw), "No progress") {
		t.Fatalf("wrong evidence or privacy: %s", raw)
	}
}

func TestQualityJudgeCancellationClosesItsStream(t *testing.T) {
	judge := newFakeRelayStream()
	d := newDriver(false, nil)
	d.openPeerStream = func(*gizcli.Client) peerStreamOpener { return func() (peerStream, error) { return judge, nil } }
	s := &session{driver: d, clients: &clientSet{clients: map[string]*gizcli.Client{"judge": {}}}, streams: newPeerStreamSessions()}
	req := giztest.StepRequest{Vars: mustVariables(t, nil), Step: giztest.Step{WorkspaceRelay: &giztest.WorkspaceRelayOperation{FirstClient: "player", SecondClient: "candidate", MaxTurns: 2, Quality: qualityTestSpec()}}}
	result := operationResult{assertion: map[string]any{"turns": map[string]any{"player": map[string]any{"texts": []any{"start"}}, "candidate": map[string]any{"texts": []any{"hello"}}}}, evidence: map[string]any{}}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, err := s.assessRelayQuality(ctx, req, result); done <- err }()
	select {
	case <-judge.pushes:
	case <-time.After(time.Second):
		t.Fatal("judge did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled judge passed")
		}
	case <-time.After(time.Second):
		t.Fatal("judge did not stop")
	}
	select {
	case <-judge.closed:
	default:
		t.Fatal("judge stream not closed")
	}
}

func TestQualityJudgeEnforcesLimitBeforeCollectingFullResponse(t *testing.T) {
	for _, test := range []struct {
		name  string
		chunk *genx.MessageChunk
		count int
	}{
		{"single oversized chunk", assistantText("j", strings.Repeat("a", maxQualityResponseBytes+1), false), 1},
		{"aggregate chunks", assistantText("j", strings.Repeat("a", maxQualityResponseBytes/2+1), false), 2},
		{"event flood", assistantText("j", "", false), relayMaxTurnEvents + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream := newFakeRelayStream()
			limited := &qualityLimitedStream{peerStream: stream}
			for index := range test.count {
				stream.in <- test.chunk
				_, err := limited.Next()
				if index == test.count-1 {
					if err != errQualityResponseLimit {
						t.Fatalf("limit error=%v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			}
			select {
			case <-stream.closed:
			default:
				t.Fatal("bounded stream did not close its transport")
			}
		})
	}
}
