package giztestcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

const maxQualityResponseBytes = 64 << 10

var errQualityResponseLimit = errors.New("quality judge response exceeds its stream limit")

var errQualityJudgeKeepalive = errors.New("quality judge keepalive failed")

// keepQualityJudgeAlive preserves an otherwise inactive judge's Edge session
// while the two other clients play. Cancellation drains its bounded RPC before
// the judge is invoked or the task starts its finalizers.
func keepQualityJudgeAlive(ctx context.Context, interval time.Duration, ping func(context.Context) error, cancel context.CancelCauseFunc) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for ctx.Err() == nil {
			pingCtx, stop := context.WithTimeout(ctx, 10*time.Second)
			err := ping(pingCtx)
			stop()
			if err != nil {
				if ctx.Err() == nil {
					cancel(errQualityJudgeKeepalive)
				}
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}

// qualityLimitedStream rejects excessive text/events before the general
// PeerStream collector retains them. Non-text media is unsupported by a judge.
type qualityLimitedStream struct {
	peerStream
	bytes  int
	events int
}

// Push keeps a valid bounded judge request below the transport's 64 KiB
// message limit. Chunk boundaries never change the original JSON text.
func (s *qualityLimitedStream) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	text, ok := chunk.Part.(genx.Text)
	if !ok || len(text) <= 8<<10 {
		return s.peerStream.Push(ctx, chunk)
	}
	if !utf8.ValidString(string(text)) {
		return errors.New("quality request is not valid UTF-8")
	}
	for start := 0; start < len(text); {
		end := min(start+(8<<10), len(text))
		for end < len(text) && !utf8.RuneStart(text[end]) {
			end--
		}
		part := chunk.Clone()
		part.Part = genx.Text(text[start:end])
		if chunk.Ctrl != nil {
			ctrl := *chunk.Ctrl
			ctrl.BeginOfStream = chunk.Ctrl.BeginOfStream && start == 0
			ctrl.EndOfStream = chunk.Ctrl.EndOfStream && end == len(text)
			ctrl.ResponseEpochEnd = chunk.Ctrl.ResponseEpochEnd && end == len(text)
			part.Ctrl = &ctrl
		}
		if err := s.peerStream.Push(ctx, part); err != nil {
			return err
		}
		start = end
	}
	return nil
}

func (s *qualityLimitedStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.peerStream.Next()
	if err != nil || chunk == nil {
		return chunk, err
	}
	s.events++
	if text, ok := chunk.Part.(genx.Text); ok {
		s.bytes += len(text)
	}
	if _, audio := chunk.Part.(*genx.Blob); audio {
		_ = s.peerStream.Close()
		return nil, errQualityResponseLimit
	}
	if s.bytes > maxQualityResponseBytes || s.events > relayMaxTurnEvents {
		_ = s.peerStream.Close()
		return nil, errQualityResponseLimit
	}
	return chunk, nil
}

type qualityTurn struct {
	Turn int    `json:"turn"`
	Role string `json:"role"`
	Text string `json:"text"`
}
type qualityQuote struct {
	Turn           int    `json:"turn"`
	Quote          string `json:"quote,omitempty"`
	QuoteID        string `json:"quote_id,omitempty"`
	quotePresent   bool
	quoteIDPresent bool
}

func (q *qualityQuote) UnmarshalJSON(raw []byte) error {
	var fields struct {
		Turn    int             `json:"turn"`
		Quote   json.RawMessage `json:"quote"`
		QuoteID json.RawMessage `json:"quote_id"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fields); err != nil {
		return err
	}
	q.Turn, q.quotePresent, q.quoteIDPresent = fields.Turn, fields.Quote != nil, fields.QuoteID != nil
	if q.quotePresent && q.quoteIDPresent {
		return errors.New("quality evidence must use exactly one quote format")
	}
	if q.quotePresent {
		return json.Unmarshal(fields.Quote, &q.Quote)
	}
	if q.quoteIDPresent {
		return json.Unmarshal(fields.QuoteID, &q.QuoteID)
	}
	return nil
}

type qualityCitationError struct{ criterion string }

func (e *qualityCitationError) Error() string {
	return fmt.Sprintf("quality criterion %s cites invalid candidate evidence", e.criterion)
}

type qualityEvidenceQuote struct {
	ID    string `json:"id"`
	Turn  int    `json:"turn"`
	Quote string `json:"quote"`
}

// qualityEvidenceQuotes lets the judge select exact original text without
// reconstructing punctuation or Markdown. Every entry remains a candidate
// substring and is resolved by the runner, never supplied by the model.
func qualityEvidenceQuotes(turns []qualityTurn) []qualityEvidenceQuote {
	var quotes []qualityEvidenceQuote
	for _, turn := range turns {
		if turn.Role != "candidate" || !utf8.ValidString(turn.Text) {
			continue
		}
		part := 0
		for start := 0; start < len(turn.Text); {
			end := min(start+512, len(turn.Text))
			for end < len(turn.Text) && !utf8.RuneStart(turn.Text[end]) {
				end--
			}
			quote := strings.TrimSpace(turn.Text[start:end])
			if quote != "" {
				part++
				quotes = append(quotes, qualityEvidenceQuote{ID: fmt.Sprintf("t%d-q%d", turn.Turn, part), Turn: turn.Turn, Quote: quote})
			}
			start = end
		}
	}
	return quotes
}

type qualityRating struct {
	ID       string         `json:"id"`
	Score    *int           `json:"score"`
	Reason   string         `json:"reason"`
	Evidence []qualityQuote `json:"evidence"`
}
type qualityResponse struct {
	Criteria []qualityRating `json:"criteria"`
}

// assessRelayQuality uses a separate selected Workspace after the relay's
// streams/readers are closed. Thresholds and pass/fail stay outside the model.
func (s *session) assessRelayQuality(ctx context.Context, req giztest.StepRequest, result operationResult) (operationResult, error) {
	quality := req.Step.WorkspaceRelay.Quality
	if quality == nil {
		return result, nil
	}
	turns, err := qualityTranscript(req.Step.WorkspaceRelay, result.assertion)
	if err != nil {
		return result, err
	}
	criteria := make([]map[string]any, 0, len(quality.Criteria))
	for _, criterion := range quality.Criteria {
		instruction, err := req.Vars.Resolve(criterion.Instruction)
		if err != nil {
			return result, err
		}
		criteria = append(criteria, map[string]any{"id": criterion.ID, "instruction": instruction})
	}
	reference, err := req.Vars.Resolve(quality.Reference)
	if err != nil {
		return result, err
	}
	payload, err := json.Marshal(map[string]any{"reference": reference, "criteria": criteria, "dialogue": turns, "evidence_quotes": qualityEvidenceQuotes(turns)})
	if err != nil || len(payload) > relayMaxTextBytes {
		return result, errors.New("quality assessment request exceeds its text limit")
	}
	required, optional := true, false
	step := giztest.Step{ID: req.Step.ID, Client: quality.JudgeClient, PeerStream: &giztest.PeerStreamOperation{Mode: "text", Input: string(payload), RequireText: &required, RequireAudio: &optional, IdleTimeout: "30s"}}
	// Dialogue is untrusted data. Do not interpolate its ${...} text through
	// the document Variables a second time after JSON encoding.
	client, err := s.clients.get(quality.JudgeClient)
	if err != nil {
		return result, err
	}
	open := openClientPeerStream(client)
	if s.driver.openPeerStream != nil {
		open = s.driver.openPeerStream(client)
	}
	boundedOpen := func() (peerStream, error) {
		stream, err := open()
		if err != nil {
			return nil, err
		}
		return &qualityLimitedStream{peerStream: stream}, nil
	}
	invocation := peerStreamInvocation{client: client, open: boundedOpen, step: step, input: string(payload)}
	judgeCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	for attempt := 1; attempt <= 3; attempt++ {
		judged, err := invocation.run(judgeCtx, nil)
		if err != nil {
			if errors.Is(err, errQualityResponseLimit) {
				return result, errQualityResponseLimit
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return result, fmt.Errorf("quality judge timed out: %w", context.DeadlineExceeded)
			}
			if errors.Is(err, context.Canceled) {
				return result, fmt.Errorf("quality judge canceled: %w", context.Canceled)
			}
			return result, errors.New("quality judge operation failed")
		}
		object, ok := judged.assertion.(map[string]any)
		if !ok {
			return result, errors.New("quality judge returned no response")
		}
		text, ok := object["reply"].(string)
		if !ok {
			return result, errors.New("quality judge returned no assistant text")
		}
		assertion, evidence, err := validateQualityResponse(text, quality, turns, s.driver.fullEvidence)
		if err == nil {
			evidence["judge_attempts"] = attempt
			result.assertion.(map[string]any)["quality"] = assertion
			result.evidence["quality"] = evidence
			return result, nil
		}
		if s.driver.fullEvidence {
			result.evidence["quality_judge_invalid_response"] = text
		}
		if attempt == 3 {
			return result, err
		}
		// Invalid evidence cannot establish a score. Request another assessment
		// against the same original data; every response receives the full strict
		// validator and the shared one-minute deadline still bounds all attempts.
		corrected, err := json.Marshal(map[string]any{"reference": reference, "criteria": criteria, "dialogue": turns, "evidence_quotes": qualityEvidenceQuotes(turns), "citation_feedback": "Previous structured assessment was invalid: " + err.Error() + ". Return every configured criterion exactly once with a 0-4 integer score, reason, and 1-3 existing evidence_quotes IDs, each keeping its entry turn. Do not invent IDs or copy literal text."})
		if err != nil || len(corrected) > relayMaxTextBytes {
			return result, errors.New("quality assessment request exceeds its text limit")
		}
		invocation.input = string(corrected)
	}
	return result, errors.New("quality judge did not return valid evidence")
}

func qualityTranscript(op *giztest.WorkspaceRelayOperation, value any) ([]qualityTurn, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("quality assessment requires completed relay evidence")
	}
	sides, ok := object["turns"].(map[string]any)
	if !ok {
		return nil, errors.New("quality assessment is missing conversation turns")
	}
	texts := map[string][]string{}
	for _, name := range []string{op.FirstClient, op.SecondClient} {
		side, ok := sides[name].(map[string]any)
		if !ok {
			return nil, errors.New("quality assessment is missing a relay participant")
		}
		entries, ok := side["texts"].([]any)
		if !ok {
			return nil, errors.New("quality assessment requires textual conversation turns")
		}
		for _, entry := range entries {
			text, ok := entry.(string)
			if !ok {
				return nil, errors.New("quality assessment has a non-text conversation turn")
			}
			texts[name] = append(texts[name], text)
		}
	}
	offsets := map[string]int{}
	turns := make([]qualityTurn, 0, op.MaxTurns)
	for index := range op.MaxTurns {
		name := op.FirstClient
		if index%2 != 0 {
			name = op.SecondClient
		}
		offset := offsets[name]
		if offset >= len(texts[name]) || strings.TrimSpace(texts[name][offset]) == "" || !utf8.ValidString(texts[name][offset]) {
			return nil, errors.New("quality assessment cannot grade an incomplete or empty conversation")
		}
		role := "player"
		if name == op.Quality.CandidateClient {
			role = "candidate"
		}
		turns = append(turns, qualityTurn{Turn: index + 1, Role: role, Text: texts[name][offset]})
		offsets[name]++
	}
	return turns, nil
}

func validateQualityResponse(text string, spec *giztest.RelayQualitySpec, turns []qualityTurn, full bool) (map[string]any, map[string]any, error) {
	raw := []byte(text)
	malformed := errors.New("quality judge returned malformed structured JSON")
	if len(raw) == 0 || len(raw) > maxQualityResponseBytes || !utf8.Valid(raw) || uniqueJSONKeys(raw) != nil {
		return nil, nil, malformed
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var response qualityResponse
	if decoder.Decode(&response) != nil {
		return nil, nil, malformed
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, nil, malformed
	}
	if len(response.Criteria) != len(spec.Criteria) {
		return nil, nil, errors.New("quality judge must assess every configured criterion exactly once")
	}
	expected := map[string]giztest.QualityCriterion{}
	for _, criterion := range spec.Criteria {
		expected[criterion.ID] = criterion
	}
	candidates := map[int]string{}
	for _, turn := range turns {
		if turn.Role == "candidate" {
			candidates[turn.Turn] = turn.Text
		}
	}
	quoteCatalog := map[string]qualityEvidenceQuote{}
	for _, quote := range qualityEvidenceQuotes(turns) {
		quoteCatalog[quote.ID] = quote
	}
	supplied := map[string]qualityRating{}
	for index, rating := range response.Criteria {
		if _, exists := expected[rating.ID]; !exists {
			return nil, nil, fmt.Errorf("quality judge has an unknown or duplicate criterion at index %d", index)
		}
		delete(expected, rating.ID)
		supplied[rating.ID] = rating
	}
	allPassed := true
	ratings, reports := make([]any, 0, len(response.Criteria)), make([]any, 0, len(response.Criteria))
	for _, criterion := range spec.Criteria {
		rating := supplied[criterion.ID]
		if rating.Score == nil || *rating.Score < 0 || *rating.Score > 4 || strings.TrimSpace(rating.Reason) == "" || len(rating.Reason) > 2048 || len(rating.Evidence) == 0 || len(rating.Evidence) > 3 {
			return nil, nil, fmt.Errorf("quality criterion %s has an invalid score, reason or evidence", criterion.ID)
		}
		references := make([]int, 0, len(rating.Evidence))
		for index, quote := range rating.Evidence {
			if quote.quoteIDPresent {
				selected, exists := quoteCatalog[quote.QuoteID]
				if !exists || selected.Turn != quote.Turn || quote.quotePresent {
					return nil, nil, &qualityCitationError{criterion: criterion.ID}
				}
				quote.Quote, quote.QuoteID = selected.Quote, ""
				rating.Evidence[index] = quote
			}
			source, exists := candidates[quote.Turn]
			if !exists || strings.TrimSpace(quote.Quote) == "" || len(quote.Quote) > 512 || !strings.Contains(source, quote.Quote) {
				return nil, nil, &qualityCitationError{criterion: criterion.ID}
			}
			references = append(references, quote.Turn)
		}
		passed := *rating.Score >= criterion.MinScore
		allPassed = allPassed && passed
		detail := map[string]any{"id": criterion.ID, "score": *rating.Score, "min_score": criterion.MinScore, "passed": passed, "reason": rating.Reason, "evidence": rating.Evidence}
		report := map[string]any{"id": criterion.ID, "score": *rating.Score, "min_score": criterion.MinScore, "passed": passed, "evidence_turns": references}
		if full {
			report["reason"], report["evidence"] = rating.Reason, rating.Evidence
		}
		ratings, reports = append(ratings, detail), append(reports, report)
	}
	return map[string]any{"passed": allPassed, "criteria": ratings}, map[string]any{"passed": allPassed, "criteria": reports}, nil
}

// uniqueJSONKeys rejects ambiguous judge objects before typed decoding. Depth
// and byte limits bound adversarial output; errors never echo model content.
func uniqueJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value func(int) error
	value = func(depth int) error {
		if depth > 8 {
			return errors.New("JSON depth exceeded")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, compound := token.(json.Delim)
		if !compound {
			return nil
		}
		if delimiter != '{' && delimiter != '[' {
			return errors.New("unexpected delimiter")
		}
		keys := map[string]bool{}
		for decoder.More() {
			if delimiter == '{' {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || keys[name] {
					return errors.New("duplicate or invalid key")
				}
				keys[name] = true
			}
			if err := value(depth + 1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	return value(0)
}
