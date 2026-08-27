package giztest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validDocument = `# User Story:
# As a registered device,
# I want to ping the target server,
# So that I can verify Peer RPC connectivity.
version: gizclaw.test/v1alpha1
name: ping-connectivity
clients:
  peer:
    identity: ephemeral
    connection: webrtc
    access_point: ${endpoint}
variables:
  endpoint:
    direction: input
    type: string
    value: 127.0.0.1:8080
  server_time:
    direction: output
    type: string
steps:
  - id: ping
    client: peer
    rpc:
      method: all.ping
      request: {}
    capture:
      server_time: /server_time
    expect:
      /server_time:
        present: true
`

func writeTestDocument(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "case.giztest.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDocumentCanonicalContract(t *testing.T) {
	doc, err := loadDocument(writeTestDocument(t, validDocument))
	if err != nil {
		t.Fatalf("loadDocument() error = %v", err)
	}
	if doc.Version != "gizclaw.test/v1alpha1" || doc.Steps[0].RPC.Method != "all.ping" {
		t.Fatalf("document = %#v", doc)
	}
}

func TestLoadDocumentRejectsReadBeforeAssignment(t *testing.T) {
	content := strings.Replace(validDocument, "request: {}", "request:\n        value: ${server_time}", 1)
	_, err := loadDocument(writeTestDocument(t, content))
	if err == nil || !strings.Contains(err.Error(), "unavailable variable") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadDocumentRejectsMissingUserStory(t *testing.T) {
	_, err := loadDocument(writeTestDocument(t, strings.SplitN(validDocument, "version:", 2)[1]))
	if err == nil || !strings.Contains(err.Error(), "User Story") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadDocumentRejectsUnknownReportRedaction(t *testing.T) {
	content := strings.Replace(validDocument, "steps:\n", "report:\n  redact: [missing]\nsteps:\n", 1)
	_, err := loadDocument(writeTestDocument(t, content))
	if err == nil || !strings.Contains(err.Error(), "report redact references unknown variable") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadDocumentRejectsInvalidStepDurationOffline(t *testing.T) {
	content := strings.Replace(validDocument, "    client: peer\n", "    client: peer\n    timeout: later\n", 1)
	_, err := loadDocument(writeTestDocument(t, content))
	if err == nil || !strings.Contains(err.Error(), "invalid timeout") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadDocumentValidatesPeerStreamIdleTimeoutOffline(t *testing.T) {
	peerStreamStep := func(idleTimeout string) string {
		return validDocument + "  - id: turn\n    client: peer\n    peer_stream:\n      mode: text\n      input: hello\n      idle_timeout: " + idleTimeout + "\n"
	}
	doc, err := loadDocument(writeTestDocument(t, peerStreamStep("15s")))
	if err != nil {
		t.Fatalf("idle_timeout 15s rejected: %v", err)
	}
	if got := doc.Steps[len(doc.Steps)-1].PeerStream.IdleTimeout; got != "15s" {
		t.Fatalf("idle_timeout = %q", got)
	}
	for _, invalid := range []string{"0s", "soon"} {
		_, err := loadDocument(writeTestDocument(t, peerStreamStep(invalid)))
		if err == nil || !strings.Contains(err.Error(), "step turn has invalid idle_timeout") {
			t.Fatalf("idle_timeout %q error = %v", invalid, err)
		}
	}
}

func TestLoadDocumentValidatesPeerStreamFirstResponseCompletion(t *testing.T) {
	peerStreamStep := func(extra string) string {
		return validDocument + `  - id: turn
    client: peer
    peer_stream:
      mode: text
      input: hello
` + extra
	}
	doc, err := loadDocument(writeTestDocument(t, peerStreamStep("      completion: first_response\n      first_text_timeout: 2s\n      first_audio_timeout: 3s\n")))
	if err != nil {
		t.Fatalf("first_response completion rejected: %v", err)
	}
	op := doc.Steps[len(doc.Steps)-1].PeerStream
	if op.Completion != "first_response" || op.FirstTextTimeout != "2s" || op.FirstAudioTimeout != "3s" {
		t.Fatalf("peer_stream operation = %#v", op)
	}
	for name, extra := range map[string]string{
		"text only":  "      completion: first_response\n      first_text_timeout: 2s\n      require_audio: false\n",
		"audio only": "      completion: first_response\n      first_audio_timeout: 3s\n      require_text: false\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadDocument(writeTestDocument(t, peerStreamStep(extra))); err != nil {
				t.Fatalf("modality-selective first_response rejected: %v", err)
			}
		})
	}
	for name, extra := range map[string]string{
		"missing text deadline":   "      completion: first_response\n      first_audio_timeout: 3s\n",
		"invalid audio deadline":  "      completion: first_response\n      first_text_timeout: 2s\n      first_audio_timeout: soon\n",
		"deadline without mode":   "      first_text_timeout: 2s\n",
		"terminal dependency":     "      completion: first_response\n      first_text_timeout: 2s\n      first_audio_timeout: 3s\n      wait_for_history: true\n",
		"disabled audio deadline": "      completion: first_response\n      first_text_timeout: 2s\n      first_audio_timeout: 3s\n      require_audio: false\n",
		"no modality":             "      completion: first_response\n      require_text: false\n      require_audio: false\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadDocument(writeTestDocument(t, peerStreamStep(extra))); err == nil {
				t.Fatal("invalid first_response completion accepted")
			}
		})
	}
}

func TestLoadDocumentAssignsStableFinalizerID(t *testing.T) {
	content := validDocument + `finally:
  - client: peer
    rpc:
      method: server.run.stop
      request: {}
`
	doc, err := loadDocument(writeTestDocument(t, content))
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Finally[0].ID; got != "finally_1" {
		t.Fatalf("finalizer id = %q", got)
	}
}

func TestDocumentRejectsSpeechCacheOutsideSavedSynthesis(t *testing.T) {
	doc := &Document{
		Version: "gizclaw.test/v1alpha1",
		Name:    "cached-speech",
		Repeat:  1,
		Clients: map[string]ClientSpec{"peer": {Identity: "ephemeral", Connection: "webrtc", AccessPoint: "127.0.0.1:8080"}},
		Variables: map[string]VariableSpec{
			"audio": {Direction: "output", Type: "audio", MaxBytes: 1024, MediaType: "audio/ogg", Codec: "opus"},
		},
		Steps: []Step{{
			ID: "speech", Client: "peer", SaveAs: "audio",
			Speech: &SpeechOperation{Method: "server.speech.synthesize", Request: map[string]any{}, Cache: "run"},
		}},
	}
	if err := doc.validateSemantics(); err != nil {
		t.Fatalf("cached synthesis rejected: %v", err)
	}
	doc.Steps[0].Speech.Method = "server.speech.transcribe"
	doc.Steps[0].Speech.Input = []byte("audio")
	if err := doc.validateSemantics(); err == nil || !strings.Contains(err.Error(), "only supported for synthesis") {
		t.Fatalf("transcription cache error = %v", err)
	}
}

func TestLoadDocumentAcceptsCombinedMatchers(t *testing.T) {
	content := strings.Replace(validDocument, "      /server_time:\n        present: true\n", "      /server_time:\n        pattern: \"^[0-9TZ:.-]+$\"\n        min_length: 4\n        max_length: 64\n        not_contains: [ERROR]\n", 1)
	if _, err := loadDocument(writeTestDocument(t, content)); err != nil {
		t.Fatalf("loadDocument() error = %v", err)
	}
}

func TestLoadDocumentRejectsInvalidExpectationsOffline(t *testing.T) {
	cases := map[string]struct {
		expect string
		want   string
	}{
		"pattern must compile":          {"{pattern: \"[\"}", "pattern does not compile"},
		"length bounds must cohere":     {"{min_length: 10, max_length: 2}", "min_length exceeds max_length"},
		"numeric bounds must cohere":    {"{minimum: 10, maximum: 2}", "minimum exceeds maximum"},
		"absent excludes value matcher": {"{present: false, contains: x}", "cannot combine with value matchers"},
		"needle list must be non-empty": {"{contains_all: []}", "schema"},
		"needles must be non-empty":     {"{contains: \"\"}", "schema"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			content := strings.Replace(validDocument, "      /server_time:\n        present: true\n", "      /server_time: "+tc.expect+"\n", 1)
			_, err := loadDocument(writeTestDocument(t, content))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestExpectationValidateRejectsMalformedOperandsInGo(t *testing.T) {
	tooMany := make([]string, 17)
	for i := range tooMany {
		tooMany[i] = "x"
	}
	cases := map[string]Expectation{
		"oversize needle list":     {ContainsAll: tooMany},
		"empty needle":             {ContainsAny: []string{""}},
		"oversize needle":          {Contains: strings.Repeat("字", 257)},
		"not_contains wrong type":  {NotContains: 7},
		"not_contains empty list":  {NotContains: []any{}},
		"not_contains wrong entry": {NotContains: []any{1}},
	}
	for name, expectation := range cases {
		t.Run(name, func(t *testing.T) {
			if err := expectation.validate(); err == nil {
				t.Fatalf("validate() accepted %#v", expectation)
			}
		})
	}
}

func TestExpectationValidateDoesNotEchoInvalidPattern(t *testing.T) {
	err := Expectation{Pattern: "secret-operand(["}.validate()
	if err == nil || !strings.Contains(err.Error(), "pattern does not compile") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "secret-operand") {
		t.Fatalf("validation error leaks the pattern operand: %v", err)
	}
}

func TestLoadDocumentAcceptsNormalizationAndRetry(t *testing.T) {
	content := strings.Replace(validDocument, "    client: peer\n", "    client: peer\n    retry:\n      attempts: 3\n      on: [timeout, assertion]\n      delay: 5s\n", 1)
	content = strings.Replace(content, "        present: true\n", "        equals: １２三\n        normalize: [digits, case]\n", 1)
	doc, err := loadDocument(writeTestDocument(t, content))
	if err != nil {
		t.Fatalf("loadDocument() error = %v", err)
	}
	if doc.Steps[0].Retry.Attempts != 3 || len(doc.Steps[0].Expect["/server_time"].Normalize) != 2 {
		t.Fatalf("document = %#v", doc)
	}
}

func TestLoadDocumentRejectsInvalidNormalizationOffline(t *testing.T) {
	cases := map[string]string{
		"empty options":             "{contains: x, normalize: []}",
		"duplicate options":         "{contains: x, normalize: [case, case]}",
		"unknown option":            "{contains: x, normalize: [accent]}",
		"no affected matcher":       "{pattern: x, normalize: [case]}",
		"non-string equals":         "{equals: 7, normalize: [case]}",
		"normalized empty contains": "{contains: \"。\", normalize: [punctuation]}",
	}
	for name, expectation := range cases {
		t.Run(name, func(t *testing.T) {
			content := strings.Replace(validDocument, "      /server_time:\n        present: true\n", "      /server_time: "+expectation+"\n", 1)
			if _, err := loadDocument(writeTestDocument(t, content)); err == nil {
				t.Fatal("loadDocument() accepted invalid normalization")
			}
		})
	}
}

func TestLoadDocumentRejectsInvalidRetryOffline(t *testing.T) {
	cases := map[string]string{
		"too few attempts":  "{attempts: 1}",
		"too many attempts": "{attempts: 11}",
		"empty on":          "{attempts: 2, on: []}",
		"duplicate on":      "{attempts: 2, on: [timeout, timeout]}",
		"unknown kind":      "{attempts: 2, on: [provider]}",
		"zero delay":        "{attempts: 2, delay: 0s}",
		"oversize delay":    "{attempts: 2, delay: 6m}",
	}
	for name, retry := range cases {
		t.Run(name, func(t *testing.T) {
			content := strings.Replace(validDocument, "    client: peer\n", "    client: peer\n    retry: "+retry+"\n", 1)
			if _, err := loadDocument(writeTestDocument(t, content)); err == nil {
				t.Fatal("loadDocument() accepted invalid retry")
			}
		})
	}
}

func TestValidateRetryRejectsLocalAndFinalizerOperations(t *testing.T) {
	for name, step := range map[string]Step{
		"output":     {ID: "emit", Output: &OutputOperation{Variable: "value"}, Retry: &RetrySpec{Attempts: 2}},
		"review":     {ID: "review", ReviewOp: &ReviewOperation{Message: "check"}, Retry: &RetrySpec{Attempts: 2}},
		"barrier":    {ID: "sync", Barrier: &BarrierOperation{}, Retry: &RetrySpec{Attempts: 2}},
		"client_rpc": {ID: "callback", ClientRPC: &ClientRPCOperation{Method: "client.info.get"}, Retry: &RetrySpec{Attempts: 2}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateRetry(step, false); err == nil {
				t.Fatal("validateRetry() accepted local operation")
			}
		})
	}
	step := Step{ID: "cleanup", RPC: &RPCOperation{Method: "server.run.stop"}, Retry: &RetrySpec{Attempts: 2}}
	if err := validateRetry(step, true); err == nil {
		t.Fatal("validateRetry() accepted finalizer")
	}
}

const relayDocument = `# User Story:
# As a Workflow catalog maintainer,
# I want a tester Workspace to exercise a candidate Workspace,
# So that the dialogue stays declarative and bounded.
version: gizclaw.test/v1alpha1
name: relay-case
clients:
  tester:
    identity: ephemeral
    connection: webrtc
    access_point: ${endpoint}
  candidate:
    identity: ephemeral
    connection: webrtc
    access_point: ${endpoint}
variables:
  endpoint:
    direction: input
    type: string
    value: 127.0.0.1:8080
  verdict:
    direction: output
    type: string
steps:
  - id: select_tester
    client: tester
    rpc:
      method: server.run.workspace.set
      request: {workspace_name: t}
  - id: select_candidate
    client: candidate
    rpc:
      method: server.run.workspace.set
      request: {workspace_name: c}
  - id: relay
    workspace_relay:
      first_client: tester
      second_client: candidate
      input: brief
      media: text
      max_turns: 15
      terminal_client: tester
    capture:
      verdict: /terminal/text
    expect:
      /terminal/client: {equals: tester}
      /completed_turns: {equals: 15}
      /turns/candidate/first_text_ms/max: {maximum: 6000}
      /turns/candidate/text_runes/min: {minimum: 1}
`

func TestLoadDocumentAcceptsWorkspaceRelay(t *testing.T) {
	doc, err := loadDocument(writeTestDocument(t, relayDocument))
	if err != nil {
		t.Fatalf("loadDocument() error = %v", err)
	}
	if doc.Steps[2].WorkspaceRelay.MaxTurns != 15 || doc.Steps[2].operation() != "workspace_relay" {
		t.Fatalf("relay step = %#v", doc.Steps[2])
	}
}

func TestLoadDocumentAcceptsMultimodalWorkspaceRelay(t *testing.T) {
	content := strings.Replace(relayDocument, "      media: text\n", "      media: text\n      terminal_media: audio\n      idle_timeout: 45s\n", 1)
	doc, err := loadDocument(writeTestDocument(t, content))
	if err != nil {
		t.Fatal(err)
	}
	op := doc.Steps[2].WorkspaceRelay
	if op.TerminalMedia != "audio" || op.IdleTimeout != "45s" {
		t.Fatalf("workspace_relay = %#v", op)
	}
}

func TestLoadDocumentAllowsTerminalTextCaptureForAudioRelay(t *testing.T) {
	content := strings.Replace(relayDocument, "      media: text\n", "      media: audio\n", 1)
	if _, err := loadDocument(writeTestDocument(t, content)); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDocumentRejectsInvalidWorkspaceRelay(t *testing.T) {
	cases := map[string]struct {
		mutate func(string) string
		want   string
	}{
		"same clients": {func(s string) string {
			return strings.Replace(s, "second_client: candidate", "second_client: tester", 1)
		}, "two distinct clients"},
		"unknown client": {func(s string) string {
			return strings.Replace(s, "second_client: candidate", "second_client: stranger", 1)
		}, "unknown client"},
		"missing selection": {func(s string) string {
			return strings.Replace(s, "      method: server.run.workspace.set\n      request: {workspace_name: c}", "      method: all.ping\n      request: {}", 1)
		}, "preceding server.run.workspace.set"},
		"terminal parity": {func(s string) string {
			return strings.Replace(s, "terminal_client: tester", "terminal_client: candidate", 1)
		}, "does not match"},
		"turns below bound": {func(s string) string {
			return strings.Replace(s, "max_turns: 15", "max_turns: 1", 1)
		}, "schema"},
		"unsupported media": {func(s string) string {
			return strings.Replace(s, "media: text", "media: video", 1)
		}, "schema"},
		"unsupported terminal media": {func(s string) string {
			return strings.Replace(s, "media: text", "media: text\n      terminal_media: video", 1)
		}, "schema"},
		"audio terminates on text": {func(s string) string {
			return strings.Replace(s, "media: text", "media: audio\n      terminal_media: text", 1)
		}, "requires terminal_media audio"},
		"zero idle timeout": {func(s string) string {
			return strings.Replace(s, "media: text", "media: text\n      idle_timeout: 0s", 1)
		}, "invalid idle_timeout"},
		"invalid idle timeout": {func(s string) string {
			return strings.Replace(s, "media: text", "media: text\n      idle_timeout: soon", 1)
		}, "invalid idle_timeout"},
		"unsafe capture": {func(s string) string {
			return strings.Replace(s, "verdict: /terminal/text", "verdict: /turns", 1)
		}, "capturing only the terminal"},
		"audio capture for text relay": {func(s string) string {
			s = strings.Replace(s, "    type: string\nsteps:", "    type: audio\n    media_type: audio/ogg\n    codec: opus\n    max_bytes: 1024\nsteps:", 1)
			return strings.Replace(s, "verdict: /terminal/text", "verdict: /terminal/audio", 1)
		}, "capturing only the terminal"},
		"unknown field": {func(s string) string {
			return strings.Replace(s, "media: text", "media: text\n      pacing: 20ms", 1)
		}, "schema"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := loadDocument(writeTestDocument(t, tc.mutate(relayDocument)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateSemanticsRejectsRelayInFinally(t *testing.T) {
	doc := &Document{
		Version: "gizclaw.test/v1alpha1",
		Name:    "relay-finally",
		Repeat:  1,
		Clients: map[string]ClientSpec{
			"a": {Identity: "ephemeral", Connection: "webrtc", AccessPoint: "127.0.0.1:8080"},
			"b": {Identity: "ephemeral", Connection: "webrtc", AccessPoint: "127.0.0.1:8080"},
		},
		Variables: map[string]VariableSpec{},
		Steps:     []Step{{ID: "ping", Client: "a", RPC: &RPCOperation{Method: "all.ping", Request: map[string]any{}}}},
		Finally: []Step{{ID: "relay", WorkspaceRelay: &WorkspaceRelayOperation{
			FirstClient: "a", SecondClient: "b", Input: "x", Media: "text", MaxTurns: 2, TerminalClient: "b",
		}}},
	}
	if err := doc.validateSemantics(); err == nil || !strings.Contains(err.Error(), "not allowed in finally") {
		t.Fatalf("error = %v", err)
	}
}
