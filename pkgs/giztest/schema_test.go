package giztest

import (
	"strings"
	"testing"
)

func TestEmbeddedSchemaCompiles(t *testing.T) {
	if _, err := schemaOnce(); err != nil {
		t.Fatal(err)
	}
}

func TestExpectedTerminalErrorContract(t *testing.T) {
	valid := validDocument + `  - id: missing_metadata
    client: peer
    peer_stream:
      mode: text
      input: hello
      expect_terminal_error:
        code: MODEL_REQUEST_BINDING_MISSING
        message: Model request metadata could not be resolved.
        label: assistant
        retryable: false
`
	if _, err := LoadDocument(writeTestDocument(t, valid), nil); err != nil {
		t.Fatal(err)
	}
	for name, document := range map[string]string{
		"listen":             strings.Replace(valid, "mode: text", "mode: listen\n      duration: 1s", 1),
		"success completion": valid + "      completion: first_response\n",
		"requires text":      valid + "      require_text: true\n",
		"requires audio":     valid + "      require_audio: true\n",
		"interruption":       valid + "      interrupt_after: 1s\n",
		"history":            valid + "      wait_for_history: true\n",
		"reply observation":  valid + "      reply_observation: 1s\n",
		"missing retryable":  strings.Replace(valid, "        retryable: false\n", "", 1),
		"blank code":         strings.Replace(valid, "code: MODEL_REQUEST_BINDING_MISSING", "code: ' '", 1),
		"reference":          strings.Replace(valid, "label: assistant", "label: '${peer}'", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadDocument(writeTestDocument(t, document), nil); err == nil {
				t.Fatal("accepted an ambiguous terminal error expectation")
			}
		})
	}
}

func TestListenAllowedTerminalErrorContract(t *testing.T) {
	valid := validDocument + `  - id: removed_listen
    client: peer
    peer_stream:
      mode: listen
      duration: 1s
      allowed_terminal_error:
        code: AGENT_OUTPUT_ERROR
        message: "sfu: workspace access revoked"
        stream_id: agent-output-error
        label: agent
        retryable: true
`
	if _, err := LoadDocument(writeTestDocument(t, valid), nil); err != nil {
		t.Fatal(err)
	}
	for name, document := range map[string]string{
		"input mode":        strings.Replace(valid, "mode: listen", "mode: text\n      input: hello", 1),
		"missing stream":    strings.Replace(valid, "        stream_id: agent-output-error\n", "", 1),
		"missing retryable": strings.Replace(valid, "        retryable: true\n", "", 1),
		"blank code":        strings.Replace(valid, "code: AGENT_OUTPUT_ERROR", "code: ' '", 1),
		"reference":         strings.Replace(valid, "label: agent", "label: '${peer}'", 1),
		"unknown matcher":   valid + "        pattern: '.*'\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadDocument(writeTestDocument(t, document), nil); err == nil {
				t.Fatal("accepted an incomplete or non-literal terminal expectation")
			}
		})
	}
}

func TestFirstPacketTimingInputContract(t *testing.T) {
	valid := validDocument + "  - id: send\n    client: peer\n    peer_stream:\n      mode: push-to-talk\n      input: audio\n      completion: input_sent\n      measure_first_packet: true\n"
	if _, err := LoadDocument(writeTestDocument(t, valid), nil); err != nil {
		t.Fatal(err)
	}
	for name, document := range map[string]string{
		"text":      strings.Replace(valid, "mode: push-to-talk", "mode: text", 1),
		"realtime":  strings.Replace(valid, "mode: push-to-talk", "mode: realtime", 1),
		"terminal":  strings.Replace(valid, "completion: input_sent", "completion: terminal", 1),
		"overlap":   valid + "      overlap_input: true\n",
		"interrupt": valid + "      interrupt_after: 1s\n",
		"empty":     strings.Replace(valid, "      input: audio\n", "      empty_input: true\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadDocument(writeTestDocument(t, document), nil); err == nil {
				t.Fatal("accepted unsupported first-packet probe")
			}
		})
	}
}
func TestSchemaRejectsUnknownField(t *testing.T) {
	_, err := LoadDocument(writeTestDocument(t, strings.Replace(validDocument, "name: ping-connectivity", "name: ping-connectivity\nunknown: true", 1)), nil)
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("error = %v", err)
	}
}

func TestEmbeddedSchemaValidatesPersistentPeerStreamFields(t *testing.T) {
	valid := persistentPeerStreamDocument("      session: microphone\n      keep_open: true\n")
	if _, err := LoadDocument(writeTestDocument(t, valid), nil); err != nil {
		t.Fatalf("persistent peer_stream schema rejected valid document: %v", err)
	}
	for name, fields := range map[string]string{
		"keep_open type":   "      session: microphone\n      keep_open: retained\n",
		"await_rearm type": "      session: microphone\n      await_rearm: 42\n",
		"unknown field":    "      session: microphone\n      keep_open: true\n      retain_forever: true\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadDocument(writeTestDocument(t, persistentPeerStreamDocument(fields)), nil); err == nil || !strings.Contains(err.Error(), "schema") {
				t.Fatalf("schema error = %v", err)
			}
		})
	}
}

func persistentPeerStreamDocument(fields string) string {
	return validDocument + "  - id: turn\n    client: peer\n    peer_stream:\n      mode: realtime\n      input: hello\n" + fields
}

func TestOverlapInputContract(t *testing.T) {
	for _, mode := range []string{"push-to-talk", "realtime"} {
		valid := validDocument + "  - id: overlap\n    client: peer\n    peer_stream:\n      mode: " + mode + "\n      input: audio\n      overlap_input: true\n"
		if _, err := LoadDocument(writeTestDocument(t, valid), nil); err != nil {
			t.Fatal(err)
		}
		for _, extra := range []string{"interrupt_after: 1s", "completion: first_response", "keep_open: true", "require_audio: false", "idle_timeout: 1s", "empty_input: true"} {
			if _, err := LoadDocument(writeTestDocument(t, valid+"      "+extra+"\n"), nil); err == nil {
				t.Fatalf("accepted overlap with %s", extra)
			}
		}
	}
	for _, mode := range []string{"text", "listen"} {
		if err := validatePeerStreamStep(Step{ID: "overlap", PeerStream: &PeerStreamOperation{Mode: mode, Input: "audio", OverlapInput: true}}, false); err == nil {
			t.Fatalf("accepted %s overlap", mode)
		}
	}
}
