package giztestcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

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
	s := &session{driver: d, clients: &clientSet{clients: map[string]*gizcli.Client{"judge": {}, "player": {}, "candidate": {}}}, streams: newPeerStreamSessions()}
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
	pingStarted := make(chan struct{})
	var pingActive atomic.Int32
	var pingOnce sync.Once
	d := newDriver(false, nil)
	d.pingClient = func(ctx context.Context, _ *gizcli.Client) error {
		pingActive.Add(1)
		defer pingActive.Add(-1)
		pingOnce.Do(func() { close(pingStarted) })
		<-ctx.Done()
		return ctx.Err()
	}
	d.openRelayStreams = func() (relayStream, relayStream, error) { return player, candidate, nil }
	d.openPeerStream = func(*gizcli.Client) peerStreamOpener {
		return func() (peerStream, error) {
			if pingActive.Load() != 0 {
				t.Error("keepalive RPC still active when judge starts")
			}
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
	s := &session{driver: d, clients: &clientSet{clients: map[string]*gizcli.Client{"judge": {}, "player": {}, "candidate": {}}}, streams: newPeerStreamSessions()}
	go func() {
		for {
			chunk := <-player.pushes
			if chunk.Ctrl != nil && chunk.Ctrl.EndOfStream {
				break
			}
		}
		<-pingStarted
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
	s := &session{driver: d, clients: &clientSet{clients: map[string]*gizcli.Client{"judge": {}, "player": {}, "candidate": {}}}, streams: newPeerStreamSessions()}
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

func TestQualityJudgeKeepaliveRepeatsAndDrainsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	pings := make(chan struct{}, 4)
	done := keepQualityJudgeAlive(ctx, 10*time.Millisecond, func(pingCtx context.Context) error {
		deadline, ok := pingCtx.Deadline()
		if !ok || time.Until(deadline) > 10*time.Second {
			t.Error("keepalive RPC has no bounded deadline")
		}
		select {
		case pings <- struct{}{}:
			return nil
		case <-pingCtx.Done():
			return pingCtx.Err()
		}
	}, cancel)
	for range 3 {
		select {
		case <-pings:
		case <-time.After(time.Second):
			t.Fatal("inactive judge was not kept alive")
		}
	}
	cancel(nil)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("keepalive did not drain on cancellation")
	}
}

func TestQualityJudgeKeepaliveFailureCancelsRelayWithoutLeakingError(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	done := keepQualityJudgeAlive(ctx, time.Minute, func(context.Context) error {
		return errors.New("sensitive provider detail")
	}, cancel)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("failed keepalive did not stop")
	}
	if context.Cause(ctx) != errQualityJudgeKeepalive {
		t.Fatalf("relay cause = %v", context.Cause(ctx))
	}
	if t.Context().Err() != nil {
		t.Fatal("keepalive canceled the task owner")
	}
}

func TestQualityQuoteIDsResolveOnlyExactCandidateEvidence(t *testing.T) {
	turns := []qualityTurn{{Turn: 1, Role: "player", Text: "player evidence"}, {Turn: 2, Role: "candidate", Text: "**李白**：先找到线索，再作结论。"}}
	good := `{"criteria":[{"id":"progression","score":2,"reason":"Needs more progress","evidence":[{"turn":2,"quote_id":"t2-q1"}]}]}`
	for _, test := range []struct {
		name, text string
		valid      bool
	}{
		{"catalog reference", good, true},
		{"unknown reference", strings.Replace(good, "t2-q1", "t2-q99", 1), false},
		{"player reference", strings.Replace(good, "t2-q1", "t1-q1", 1), false},
		{"different turn", strings.Replace(good, `"turn":2`, `"turn":4`, 1), false},
		{"ambiguous text and ID", strings.Replace(good, `"quote_id":`, `"quote":"altered text","quote_id":`, 1), false},
		{"empty text plus ID", strings.Replace(good, `"quote_id":`, `"quote":"","quote_id":`, 1), false},
		{"null text plus ID", strings.Replace(good, `"quote_id":`, `"quote":null,"quote_id":`, 1), false},
		{"literal text plus empty ID", strings.Replace(good, `"quote_id":"t2-q1"`, `"quote":"**李白**","quote_id":""`, 1), false},
		{"literal text plus null ID", strings.Replace(good, `"quote_id":"t2-q1"`, `"quote":"**李白**","quote_id":null`, 1), false},
		{"unknown evidence field", strings.Replace(good, `"quote_id":`, `"unknown":true,"quote_id":`, 1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, report, err := validateQualityResponse(test.text, qualityTestSpec(), turns, true)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
			if err != nil {
				return
			}
			if result["passed"] != false {
				t.Fatal("a valid citation changed the low-score verdict")
			}
			encoded, _ := json.Marshal(report)
			if !strings.Contains(string(encoded), turns[1].Text) || strings.Contains(string(encoded), "quote_id") {
				t.Fatalf("report did not resolve original evidence: %s", encoded)
			}
		})
	}
}

func TestQualityQuoteCatalogPreservesUnicodeAndCandidateBoundaries(t *testing.T) {
	source := strings.Repeat("李白🌙，", 180)
	turns := []qualityTurn{{Turn: 1, Role: "player", Text: "do not cite me"}, {Turn: 2, Role: "candidate", Text: source}}
	quotes := qualityEvidenceQuotes(turns)
	var restored strings.Builder
	for index, quote := range quotes {
		if quote.Turn != 2 || len(quote.Quote) > 512 || !utf8.ValidString(quote.Quote) || !strings.Contains(source, quote.Quote) {
			t.Fatalf("catalog contains invalid candidate evidence at %d", index)
		}
		restored.WriteString(quote.Quote)
	}
	if len(quotes) < 2 || restored.String() != source {
		t.Fatal("catalog dropped or rewrote source text")
	}
}

func TestQualityCitationRetryIsBoundedAndDoesNotSeekPassingScores(t *testing.T) {
	for _, repaired := range []bool{false, true} {
		t.Run(fmt.Sprint(repaired), func(t *testing.T) {
			d := newDriver(false, nil)
			var opened []*fakeRelayStream
			d.openPeerStream = func(*gizcli.Client) peerStreamOpener {
				return func() (peerStream, error) {
					stream := newFakeRelayStream()
					opened = append(opened, stream)
					attempt := len(opened)
					if attempt > 1 {
						select {
						case <-opened[attempt-2].closed:
						default:
							t.Error("previous judge stream remains open")
						}
					}
					go func() {
						var input strings.Builder
						for {
							c := <-stream.pushes
							if part, ok := c.Part.(genx.Text); ok {
								input.WriteString(string(part))
							}
							if c.IsEndOfStream() {
								break
							}
						}
						if attempt > 1 && !strings.Contains(input.String(), "citation_feedback") {
							t.Error("retry lacks feedback")
						}
						id := "invented"
						if repaired && attempt == 2 {
							id = "t2-q1"
						}
						stream.in <- assistantText("j", fmt.Sprintf(`{"criteria":[{"id":"progression","score":1,"reason":"No progress","evidence":[{"turn":2,"quote_id":%q}]}]}`, id), true)
					}()
					return stream, nil
				}
			}
			s := &session{driver: d, clients: &clientSet{clients: map[string]*gizcli.Client{"judge": {}}}}
			req := giztest.StepRequest{Vars: mustVariables(t, nil), Step: giztest.Step{WorkspaceRelay: &giztest.WorkspaceRelayOperation{FirstClient: "player", SecondClient: "candidate", MaxTurns: 2, Quality: qualityTestSpec()}}}
			result := operationResult{assertion: map[string]any{"turns": map[string]any{"player": map[string]any{"texts": []any{"continue"}}, "candidate": map[string]any{"texts": []any{"Welcome again"}}}}, evidence: map[string]any{}}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			got, err := s.assessRelayQuality(ctx, req, result)
			if repaired {
				if err != nil || len(opened) != 2 || got.assertion.(map[string]any)["quality"].(map[string]any)["passed"] != false {
					t.Fatalf("repair changed verdict or failed: %v", err)
				}
			} else if err == nil || len(opened) != 3 {
				t.Fatalf("unbounded/accepted invalid evidence: attempts=%d err=%v", len(opened), err)
			}
			for _, stream := range opened {
				select {
				case <-stream.closed:
				default:
					t.Error("judge attempt not closed")
				}
			}
			if _, exists := got.evidence["quality_judge_invalid_response"]; exists {
				t.Error("redacted evidence contains an invalid model response")
			}
		})
	}
}

func TestQualityJudgeLargeInputKeepsExactUTF8AndOneTerminal(t *testing.T) {
	stream := newFakeRelayStream()
	limited := &qualityLimitedStream{peerStream: stream}
	text := strings.Repeat("原文🌙", 9000)
	chunk := &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "same", BeginOfStream: true, EndOfStream: true}}
	var reconstructed strings.Builder
	done := make(chan struct{})
	var begins, ends int
	go func() {
		defer close(done)
		for {
			p := <-stream.pushes
			v := string(p.Part.(genx.Text))
			if len(v) > 8<<10 || !utf8.ValidString(v) {
				t.Error("invalid transport-sized chunk")
			}
			reconstructed.WriteString(v)
			if p.IsBeginOfStream() {
				begins++
			}
			if p.IsEndOfStream() {
				ends++
				break
			}
		}
	}()
	if err := limited.Push(t.Context(), chunk); err != nil {
		t.Fatal(err)
	}
	<-done
	if reconstructed.String() != text || begins != 1 || ends != 1 || !chunk.IsBeginOfStream() || !chunk.IsEndOfStream() {
		t.Fatal("chunking altered content or controls")
	}
}
