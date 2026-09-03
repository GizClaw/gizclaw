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
	doc, err := LoadDocument(writeTestDocument(t, validDocument), nil)
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
	}
	if doc.Version != "gizclaw.test/v1alpha1" || doc.Steps[0].RPC.Method != "all.ping" {
		t.Fatalf("document = %#v", doc)
	}
}

func TestLoadDocumentRejectsReadBeforeAssignment(t *testing.T) {
	content := strings.Replace(validDocument, "request: {}", "request:\n        value: ${server_time}", 1)
	_, err := LoadDocument(writeTestDocument(t, content), nil)
	if err == nil || !strings.Contains(err.Error(), "unavailable variable") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadDocumentRejectsMissingUserStory(t *testing.T) {
	_, err := LoadDocument(writeTestDocument(t, strings.SplitN(validDocument, "version:", 2)[1]), nil)
	if err == nil || !strings.Contains(err.Error(), "User Story") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadDocumentRejectsUnknownReportRedaction(t *testing.T) {
	content := strings.Replace(validDocument, "steps:\n", "report:\n  redact: [missing]\nsteps:\n", 1)
	_, err := LoadDocument(writeTestDocument(t, content), nil)
	if err == nil || !strings.Contains(err.Error(), "report redact references unknown variable") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadDocumentRejectsInvalidStepDurationOffline(t *testing.T) {
	content := strings.Replace(validDocument, "    client: peer\n", "    client: peer\n    timeout: later\n", 1)
	_, err := LoadDocument(writeTestDocument(t, content), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid timeout") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadDocumentValidatesPeerStreamIdleTimeoutOffline(t *testing.T) {
	peerStreamStep := func(idleTimeout string) string {
		return validDocument + "  - id: turn\n    client: peer\n    peer_stream:\n      mode: text\n      input: hello\n      idle_timeout: " + idleTimeout + "\n"
	}
	doc, err := LoadDocument(writeTestDocument(t, peerStreamStep("15s")), nil)
	if err != nil {
		t.Fatalf("idle_timeout 15s rejected: %v", err)
	}
	if got := doc.Steps[len(doc.Steps)-1].PeerStream.IdleTimeout; got != "15s" {
		t.Fatalf("idle_timeout = %q", got)
	}
	for _, invalid := range []string{"0s", "soon"} {
		_, err := LoadDocument(writeTestDocument(t, peerStreamStep(invalid)), nil)
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
	doc, err := LoadDocument(writeTestDocument(t, peerStreamStep("      completion: first_response\n      first_text_timeout: 2s\n      first_audio_timeout: 3s\n")), nil)
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
			if _, err := LoadDocument(writeTestDocument(t, peerStreamStep(extra)), nil); err != nil {
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
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadDocument(writeTestDocument(t, peerStreamStep(extra)), nil); err == nil {
				t.Fatal("invalid first_response completion accepted")
			}
		})
	}
	_, err = LoadDocument(writeTestDocument(t, peerStreamStep("      completion: first_response\n      require_text: false\n      require_audio: false\n")), nil)
	if err == nil || !strings.Contains(err.Error(), "first_response requires text or audio output") {
		t.Fatalf("no-modality error = %v", err)
	}
}

func TestLoadDocumentValidatesPersistentPeerStream(t *testing.T) {
	peerStreamStep := func(extra string) string {
		return validDocument + `  - id: turn
    client: peer
    peer_stream:
      mode: realtime
      input: audio-fixture
` + extra
	}
	doc, err := LoadDocument(writeTestDocument(t, peerStreamStep("      session: microphone\n      keep_open: true\n")), nil)
	if err != nil {
		t.Fatalf("persistent peer_stream rejected: %v", err)
	}
	op := doc.Steps[len(doc.Steps)-1].PeerStream
	if op.Session != "microphone" || !op.KeepOpen {
		t.Fatalf("peer_stream operation = %#v", op)
	}
	if _, err := LoadDocument(writeTestDocument(t, peerStreamStep("      session: microphone\n      await_rearm: INPUT_ROUTE_RELOADED\n      keep_open: true\n")), nil); err != nil {
		t.Fatalf("await_rearm peer_stream rejected: %v", err)
	}
	for name, extra := range map[string]string{
		"missing session":  "      keep_open: true\n",
		"unused session":   "      session: microphone\n",
		"invalid session":  "      session: BadName\n      keep_open: true\n",
		"unsupported code": "      session: microphone\n      await_rearm: SOMETHING_ELSE\n",
		"interrupt":        "      session: microphone\n      keep_open: true\n      interrupt_after: 1s\n",
		"retry":            "      session: microphone\n      keep_open: true\n    retry:\n      attempts: 2\n",
		"non-realtime":     "      session: microphone\n      keep_open: true\n      mode: text\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadDocument(writeTestDocument(t, peerStreamStep(extra)), nil); err == nil {
				t.Fatal("invalid persistent peer_stream accepted")
			}
		})
	}
	finalizer := validDocument + `finally:
  - client: peer
    peer_stream:
      mode: realtime
      input: audio-fixture
      session: microphone
      keep_open: true
`
	if _, err := LoadDocument(writeTestDocument(t, finalizer), nil); err == nil || !strings.Contains(err.Error(), "not allowed in finally") {
		t.Fatalf("persistent finalizer error = %v", err)
	}
}

func TestLoadDocumentAssignsStableFinalizerID(t *testing.T) {
	content := validDocument + `finally:
  - client: peer
    rpc:
      method: server.run.stop
      request: {}
`
	doc, err := LoadDocument(writeTestDocument(t, content), nil)
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
	if _, err := LoadDocument(writeTestDocument(t, content), nil); err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
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
			_, err := LoadDocument(writeTestDocument(t, content), nil)
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
	doc, err := LoadDocument(writeTestDocument(t, content), nil)
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
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
			if _, err := LoadDocument(writeTestDocument(t, content), nil); err == nil {
				t.Fatal("LoadDocument() accepted invalid normalization")
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
			if _, err := LoadDocument(writeTestDocument(t, content), nil); err == nil {
				t.Fatal("LoadDocument() accepted invalid retry")
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
	doc, err := LoadDocument(writeTestDocument(t, relayDocument), nil)
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
	}
	if doc.Steps[2].WorkspaceRelay.MaxTurns != 15 || doc.Steps[2].Operation() != "workspace_relay" {
		t.Fatalf("relay step = %#v", doc.Steps[2])
	}
}

func TestLoadDocumentAcceptsMultimodalWorkspaceRelay(t *testing.T) {
	content := strings.Replace(relayDocument, "      media: text\n", "      media: text\n      terminal_media: audio\n      idle_timeout: 45s\n", 1)
	doc, err := LoadDocument(writeTestDocument(t, content), nil)
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
	if _, err := LoadDocument(writeTestDocument(t, content), nil); err != nil {
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
			_, err := LoadDocument(writeTestDocument(t, tc.mutate(relayDocument)), nil)
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

func TestLoadDocumentValidatesListenPeerStream(t *testing.T) {
	listenStep := func(extra string) string {
		return validDocument + "  - id: listen\n    client: peer\n    peer_stream:\n      mode: listen\n" + extra
	}
	doc, err := LoadDocument(writeTestDocument(t, listenStep("      duration: 2s\n")), nil)
	if err != nil {
		t.Fatalf("listen rejected: %v", err)
	}
	if got := doc.Steps[len(doc.Steps)-1].PeerStream; got.Mode != "listen" || got.Duration != "2s" || got.Input != nil {
		t.Fatalf("listen operation = %#v", got)
	}
	for name, tc := range map[string]struct {
		extra string
		want  string
	}{
		"missing duration":        {extra: "", want: "schema validation"},
		"zero duration":           {extra: "      duration: 0s\n", want: "listen requires a duration"},
		"unparsable duration":     {extra: "      duration: soon\n", want: "listen requires a duration"},
		"duration above bound":    {extra: "      duration: 6m\n", want: "listen requires a duration"},
		"input":                   {extra: "      duration: 2s\n      input: hello\n", want: "schema validation"},
		"pacing":                  {extra: "      duration: 2s\n      pacing: 20ms\n", want: "cannot set pacing"},
		"interrupt_after":         {extra: "      duration: 2s\n      interrupt_after: 1s\n", want: "cannot set interrupt_after"},
		"idle_timeout":            {extra: "      duration: 2s\n      idle_timeout: 1s\n", want: "cannot set idle_timeout"},
		"completion":              {extra: "      duration: 2s\n      completion: terminal\n", want: "cannot set completion"},
		"terminal_label":          {extra: "      duration: 2s\n      terminal_label: transcript\n", want: "cannot set terminal_label"},
		"require_text":            {extra: "      duration: 2s\n      require_text: false\n", want: "cannot set require_text"},
		"wait_for_history":        {extra: "      duration: 2s\n      wait_for_history: true\n", want: "cannot set wait_for_history"},
		"session":                 {extra: "      duration: 2s\n      session: mic\n      keep_open: true\n", want: "cannot set session"},
		"await_rearm":             {extra: "      duration: 2s\n      await_rearm: INPUT_ROUTE_RELOADED\n", want: "cannot set"},
		"duration outside listen": {extra: "", want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			body := listenStep(tc.extra)
			if name == "duration outside listen" {
				body = validDocument + "  - id: turn\n    client: peer\n    peer_stream:\n      mode: text\n      input: hello\n      duration: 2s\n"
				tc.want = "only valid for listen"
			}
			_, err := LoadDocument(writeTestDocument(t, body), nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLoadDocumentValidatesInputSentCompletion(t *testing.T) {
	turn := func(mode, extra string) string {
		return validDocument + "  - id: turn\n    client: peer\n    peer_stream:\n      mode: " + mode + "\n      input: ${endpoint}\n      completion: input_sent\n" + extra
	}
	for _, mode := range []string{"push-to-talk", "realtime"} {
		doc, err := LoadDocument(writeTestDocument(t, turn(mode, "")), nil)
		if err != nil {
			t.Fatalf("input_sent %s rejected: %v", mode, err)
		}
		if got := doc.Steps[len(doc.Steps)-1].PeerStream.Completion; got != "input_sent" {
			t.Fatalf("completion = %q", got)
		}
	}
	if _, err := LoadDocument(writeTestDocument(t, turn("push-to-talk", "      pacing: 20ms\n      idle_timeout: 5s\n")), nil); err != nil {
		t.Fatalf("input_sent with pacing rejected: %v", err)
	}
	for name, tc := range map[string]struct {
		mode  string
		extra string
		want  string
	}{
		"text mode":           {mode: "text", want: "requires push-to-talk or realtime"},
		"first_text_timeout":  {mode: "push-to-talk", extra: "      first_text_timeout: 1s\n", want: "cannot wait for output"},
		"first_audio_timeout": {mode: "realtime", extra: "      first_audio_timeout: 1s\n", want: "cannot wait for output"},
		"wait_for_history":    {mode: "push-to-talk", extra: "      wait_for_history: true\n", want: "cannot wait for output"},
		"require_text":        {mode: "push-to-talk", extra: "      require_text: false\n", want: "cannot wait for output"},
		"require_audio":       {mode: "push-to-talk", extra: "      require_audio: true\n", want: "cannot wait for output"},
		"interrupt_after":     {mode: "push-to-talk", extra: "      interrupt_after: 1s\n", want: "cannot wait for output"},
		"terminal_label":      {mode: "push-to-talk", extra: "      terminal_label: transcript\n", want: "cannot wait for output"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadDocument(writeTestDocument(t, turn(tc.mode, tc.extra)), nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLoadDocumentAcceptsBackgroundAwaitListen(t *testing.T) {
	doc, err := LoadDocument(writeTestDocument(t, validDocument+`  - id: listen
    client: peer
    background: true
    peer_stream:
      mode: listen
      duration: 3s
  - id: wait
    await: listen
    timeout: 10s
    expect:
      /audio_bytes:
        equals: 0
`), nil)
	if err != nil {
		t.Fatalf("background listen document rejected: %v", err)
	}
	listen, wait := doc.Steps[1], doc.Steps[2]
	if !listen.Background || listen.PeerStream.Mode != "listen" || listen.PeerStream.Duration != "3s" {
		t.Fatalf("listen step = %#v", listen)
	}
	if wait.Await != "listen" || wait.Operation() != "await" || operationNeedsClient(wait.Operation()) {
		t.Fatalf("await step = %#v", wait)
	}
}

func TestLoadDocumentRejectsInvalidBackgroundAwait(t *testing.T) {
	listen := func(extra string) string {
		return "  - id: listen\n    client: peer\n    background: true\n    peer_stream:\n      mode: listen\n      duration: 3s\n" + extra
	}
	for name, tc := range map[string]struct {
		body string
		want string
	}{
		"background never awaited":  {body: listen(""), want: "must be awaited exactly once"},
		"await unknown step":        {body: listen("  - id: wait\n    await: other\n"), want: "not an earlier background step"},
		"await before background":   {body: "  - id: wait\n    await: listen\n" + listen(""), want: "not an earlier background step"},
		"await twice":               {body: listen("  - id: wait\n    await: listen\n  - id: again\n    await: listen\n"), want: "already awaited"},
		"await with client":         {body: listen("  - id: wait\n    client: peer\n    await: listen\n"), want: "takes its client"},
		"await with operation":      {body: listen("  - id: wait\n    await: listen\n    rpc:\n      method: all.ping\n      request: {}\n"), want: "schema validation"},
		"await with retry":          {body: listen("  - id: wait\n    await: listen\n    retry:\n      attempts: 2\n"), want: "does not support retry"},
		"background rpc":            {body: "  - id: bg\n    client: peer\n    background: true\n    rpc:\n      method: all.ping\n      request: {}\n  - id: wait\n    await: bg\n", want: "requires a peer_stream"},
		"background expect":         {body: "  - id: listen\n    client: peer\n    background: true\n    peer_stream:\n      mode: listen\n      duration: 3s\n    expect:\n      /audio_bytes:\n        equals: 0\n  - id: wait\n    await: listen\n", want: "cannot capture, expect, or save_as"},
		"background retry":          {body: "  - id: listen\n    client: peer\n    background: true\n    retry:\n      attempts: 2\n    peer_stream:\n      mode: listen\n      duration: 3s\n  - id: wait\n    await: listen\n", want: "cannot retry"},
		"background session":        {body: "  - id: turn\n    client: peer\n    background: true\n    peer_stream:\n      mode: realtime\n      input: ${endpoint}\n      session: mic\n      keep_open: true\n  - id: wait\n    await: turn\n", want: "cannot use a session"},
		"background in finally":     {body: listen("  - id: wait\n    await: listen\n") + "finally:\n  - id: late\n    client: peer\n    background: true\n    peer_stream:\n      mode: listen\n      duration: 1s\n", want: "schema validation"},
		"background and await":      {body: listen("  - id: wait\n    await: listen\n    background: true\n"), want: "cannot be both background and await"},
		"await in finally":          {body: listen("  - id: wait\n    await: listen\n") + "finally:\n  - id: late\n    await: listen\n", want: "schema validation"},
		"await before finally only": {body: listen("") + "finally:\n  - id: cleanup\n    client: peer\n    rpc:\n      method: all.ping\n      request: {}\n", want: "must be awaited exactly once"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadDocument(writeTestDocument(t, validDocument+tc.body), nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

// A driver that cannot run steps in the background does not list "await", so
// a document with background steps is rejected by validate and skipped by
// LoadSupportedDocuments instead of failing once a task is running.
func TestLoadDocumentGatesBackgroundStepsOnDriverAwaitSupport(t *testing.T) {
	path := writeTestDocument(t, validDocument+`  - id: listen
    client: peer
    background: true
    peer_stream:
      mode: listen
      duration: 3s
  - id: wait
    await: listen
`)
	if _, err := LoadDocument(path, &stubDriver{operations: []string{"rpc", "peer_stream"}}); err == nil || !strings.Contains(err.Error(), "operation await is not supported") {
		t.Fatalf("driver without await accepted background steps: %v", err)
	}
	if _, err := LoadDocument(path, &stubDriver{operations: []string{"rpc", "peer_stream", "await"}}); err != nil {
		t.Fatalf("driver with await rejected background steps: %v", err)
	}
	documents, skipped, err := LoadSupportedDocuments([]string{path}, &stubDriver{operations: []string{"rpc", "peer_stream"}})
	if err != nil || len(documents) != 0 || len(skipped) != 1 || !strings.Contains(skipped[0].Reason, "await") {
		t.Fatalf("documents = %#v skipped = %#v err = %v", documents, skipped, err)
	}
}
