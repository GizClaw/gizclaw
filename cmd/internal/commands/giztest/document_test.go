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
