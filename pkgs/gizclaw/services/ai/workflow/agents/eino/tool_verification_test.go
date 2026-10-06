package eino

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolcatalog"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

type verificationGenerator struct {
	result string
	err    error
	seen   string
	schema json.RawMessage
}

func (*verificationGenerator) GenerateStream(context.Context, string, genx.ModelContext) (genx.Stream, error) {
	return nil, errors.New("not a primary model call")
}

func (g *verificationGenerator) Invoke(_ context.Context, _ string, input genx.ModelContext, tool *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	g.schema, _ = json.Marshal(tool.Argument)
	for message := range input.Messages() {
		if parts, ok := message.Payload.(genx.Contents); ok {
			for _, part := range parts {
				if text, ok := part.(genx.Text); ok {
					g.seen += string(text)
				}
			}
		}
	}
	return genx.Usage{}, tool.NewFuncCall(g.result), g.err
}

func TestToolVerifierFailsClosedAndDoesNotChangeCandidate(t *testing.T) {
	for _, row := range []struct {
		name, response string
		allow          bool
		invalid        bool
	}{
		{"allow", `{"approved":true,"reason":"approved"}`, true, false},
		{"wrong target", `{"approved":false,"reason":"wrong_target"}`, false, false},
		{"cancelled", `{"approved":false,"reason":"cancelled"}`, false, false},
		{"unknown reason", `{"approved":false,"reason":"invented"}`, false, true},
		{"inconsistent", `{"approved":true,"reason":"cancelled"}`, false, true},
		{"missing approval", `{"reason":"approved"}`, false, true},
		{"duplicate approval", `{"approved":false,"approved":true,"reason":"approved"}`, false, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			generator := &verificationGenerator{result: row.response}
			candidate := toolcatalog.Tool{Alias: "lamp", Source: "mhs", Target: map[string]any{"id": "led.status"}}
			arguments := json.RawMessage(`{"brightness_percent":30}`)
			resolve := func(context.Context) ([]toolcatalog.Tool, error) { return []toolcatalog.Tool{candidate}, nil }
			reason, err := runtimeToolVerifier(generator, "model/checker", resolve)(t.Context(), candidate, arguments, genx.ToolConversation{CurrentUser: "screen 30%"})
			if row.invalid != (err != nil) || (!row.invalid && row.allow != (reason == "")) {
				t.Fatalf("reason=%q error=%v", reason, err)
			}
			if candidate.Target["id"] != "led.status" || string(arguments) != `{"brightness_percent":30}` || generator.seen == "" {
				t.Fatal("candidate changed or actual conversation was omitted")
			}
			var schema struct {
				Properties map[string]struct{ Enum []string }
			}
			if err := json.Unmarshal(generator.schema, &schema); err != nil || len(schema.Properties["reason"].Enum) != 10 {
				t.Fatalf("structured Invoke omitted the finite decision schema: %s", generator.schema)
			}
		})
	}
}

func TestToolVerifierReadsOtherTargetContextWithoutPrivateExecutors(t *testing.T) {
	candidate := toolcatalog.Tool{Alias: "lamp", Source: "mhs", Target: map[string]any{"id": "led.status"}}
	focus := "screen is the configured focus"
	resolve := func(context.Context) ([]toolcatalog.Tool, error) {
		return []toolcatalog.Tool{candidate, {Alias: "screen", Description: focus, Target: map[string]any{"id": "display.main"}, HTTP: &toolkit.Tool{HTTP: &toolkit.HTTPRequest{Headers: map[string]string{"private": "must-not-reach-verifier"}}}}}, nil
	}
	generator := &verificationGenerator{result: `{"approved":false,"reason":"wrong_target"}`}
	verify := runtimeToolVerifier(generator, "model/checker", resolve)
	for _, current := range []string{"screen is the configured focus", "lamp is the configured focus"} {
		focus = current
		generator.seen = ""
		if reason, err := verify(t.Context(), candidate, json.RawMessage(`{"brightness_percent":60}`), genx.ToolConversation{CurrentUser: "increase brightness"}); err != nil || reason == "" {
			t.Fatalf("decision: %q, %v", reason, err)
		}
		if !strings.Contains(generator.seen, current) || !strings.Contains(generator.seen, "display.main") || strings.Contains(generator.seen, "must-not-reach-verifier") {
			t.Fatal("current cross-target metadata missing or private executor leaked")
		}
	}
}

func TestToolVerifierDoesNotCallModelWithoutCurrentCatalog(t *testing.T) {
	for _, resolve := range []func(context.Context) ([]toolcatalog.Tool, error){nil, func(context.Context) ([]toolcatalog.Tool, error) { return nil, errors.New("private lookup failure") }} {
		generator := &verificationGenerator{result: `{"approved":true,"reason":"approved"}`}
		_, err := runtimeToolVerifier(generator, "model/checker", resolve)(t.Context(), toolcatalog.Tool{}, json.RawMessage(`{}`), genx.ToolConversation{CurrentUser: "change brightness"})
		if err == nil || strings.Contains(err.Error(), "private") || generator.seen != "" {
			t.Fatalf("catalog failure reached model or leaked: %v", err)
		}
	}
}

func TestToolResponseVerifierUsesCurrentCatalogAndFiniteDecision(t *testing.T) {
	resolve := func(context.Context) ([]toolcatalog.Tool, error) {
		return []toolcatalog.Tool{{Alias: "screen.read", Source: "mhs", Available: true, Target: map[string]any{"id": "display.main", "operation": "read"}, HTTP: &toolkit.Tool{HTTP: &toolkit.HTTPRequest{Headers: map[string]string{"private": "must-not-reach-verifier"}}}}}, nil
	}
	for _, response := range []string{`{"approved":true,"reason":"approved"}`, `{"approved":false,"reason":"missing_operation"}`, `{"approved":false,"reason":"false_completion"}`, `{"approved":false,"reason":"invented"}`} {
		generator := &verificationGenerator{result: response}
		feedback, err := runtimeToolResponseVerifier(generator, "model/checker", resolve)(t.Context(), genx.ToolConversation{CurrentUser: "screen 30%", ContinuationStart: 1, Messages: []genx.ToolConversationMessage{{Role: "user", Content: "screen 30%"}}}, "set without proof")
		if response == `{"approved":true,"reason":"approved"}` {
			if err != nil || feedback != "" {
				t.Fatalf("approved: %q %v", feedback, err)
			}
		} else if response == `{"approved":false,"reason":"invented"}` {
			if err == nil {
				t.Fatal("unknown decision accepted")
			}
		} else if err != nil || feedback == "" {
			t.Fatalf("rejected: %q %v", feedback, err)
		}
		if !strings.Contains(generator.seen, `"operation":"read"`) || strings.Contains(generator.seen, "must-not-reach-verifier") {
			t.Fatal("fixed operation omitted or private executor data exposed")
		}
	}
}

func TestEmptyToolReplyRequestsCorrectionWithoutModelOrCatalogCall(t *testing.T) {
	resolve := func(context.Context) ([]toolcatalog.Tool, error) {
		t.Error("empty reply loaded catalog")
		return nil, nil
	}
	generator := &verificationGenerator{}
	feedback, err := runtimeToolResponseVerifier(generator, "model/checker", resolve)(t.Context(), genx.ToolConversation{}, " \n")
	if feedback == "" || err != nil || generator.seen != "" {
		t.Fatalf("empty reply: feedback=%q err=%v", feedback, err)
	}
}

func TestUnavailableCatalogCannotDemandExecutionButStillRejectsFalseCompletion(t *testing.T) {
	resolve := func(context.Context) ([]toolcatalog.Tool, error) {
		return []toolcatalog.Tool{{Alias: "screen", Availability: toolcatalog.Availability{Reason: "DEVICE_OFFLINE"}}}, nil
	}
	for _, response := range []string{`{"approved":true,"reason":"approved"}`, `{"approved":false,"reason":"false_completion"}`, `{"approved":false,"reason":"missing_operation"}`} {
		generator := &verificationGenerator{result: response}
		feedback, err := runtimeToolResponseVerifier(generator, "model/checker", resolve)(t.Context(), genx.ToolConversation{CurrentUser: "screen 40%"}, "device unavailable; not completed")
		if strings.Contains(string(generator.schema), "missing_operation") || strings.Contains(string(generator.schema), "unnecessary_clarification") {
			t.Fatal("unavailable catalog admitted impossible classifications")
		}
		if strings.Contains(response, "missing_operation") {
			if err == nil {
				t.Fatal("impossible missing-operation decision accepted")
			}
		} else if strings.Contains(response, "false_completion") {
			if err != nil || feedback == "" {
				t.Fatalf("false claim escaped: %q %v", feedback, err)
			}
		} else if err != nil || feedback != "" {
			t.Fatalf("honest unavailable reply rejected: %q %v", feedback, err)
		}
	}
}

func TestVerificationCapabilityFactsKeepReadAndWriteIndependent(t *testing.T) {
	resolve := func(context.Context) ([]toolcatalog.Tool, error) {
		return []toolcatalog.Tool{
			{Alias: "screen.read", Source: "mhs", Available: true, Target: map[string]any{"id": "display.main", "hwd": "display", "operation": "read"}},
			{Alias: "screen.revoked", Source: "mhs", Target: map[string]any{"id": "display.main", "hwd": "display", "operation": "write", "fields": []string{"brightness_percent"}}},
			{Alias: "lamp.write", Source: "mhs", Available: true, Target: map[string]any{"id": "led.status", "hwd": "led", "operation": "write", "fields": []string{"brightness_percent"}}},
			{Alias: "screen.external", Source: "http_request", Available: true, Target: map[string]any{"id": "display.main", "hwd": "display", "operation": "write"}},
		}, nil
	}
	generator := &verificationGenerator{result: `{"approved":true,"reason":"approved"}`}
	conversation := genx.ToolConversation{CurrentUser: "40%", ContinuationStart: 3, Messages: []genx.ToolConversationMessage{
		{Role: "user", Content: "set screen brightness"},
		{Role: "assistant", Content: "proposal to change the lamp"},
		{Role: "user", Content: "40%"},
	}}
	if feedback, err := runtimeToolResponseVerifier(generator, "model/checker", resolve)(t.Context(), conversation, "screen write unavailable; not changed"); err != nil || feedback != "" {
		t.Fatalf("verification: %q %v", feedback, err)
	}
	var input struct {
		Users []string `json:"user_turns"`
		MHS   []struct {
			ID, HWD string
			Read    bool     `json:"can_read"`
			Write   bool     `json:"can_write"`
			Fields  []string `json:"writable_fields"`
		} `json:"mhs_capabilities"`
	}
	if err := json.Unmarshal([]byte(generator.seen), &input); err != nil {
		// The fake records the system prompt before the raw proposal JSON.
		start := strings.Index(generator.seen, `{"conversation":`)
		if start < 0 || json.Unmarshal([]byte(generator.seen[start:]), &input) != nil {
			t.Fatalf("missing structured verification input: %v", err)
		}
	}
	if len(input.Users) != 2 || input.Users[0] != "set screen brightness" || input.Users[1] != "40%" {
		t.Fatalf("assistant proposal became user authority: %v", input.Users)
	}
	if len(input.MHS) != 2 || input.MHS[0].ID != "display.main" || !input.MHS[0].Read || input.MHS[0].Write || len(input.MHS[0].Fields) != 0 {
		t.Fatalf("read, revoked or external capability created screen write authority: %+v", input.MHS)
	}
	if input.MHS[1].ID != "led.status" || input.MHS[1].Read || !input.MHS[1].Write || len(input.MHS[1].Fields) != 1 || input.MHS[1].Fields[0] != "brightness_percent" {
		t.Fatalf("lamp capability lost or borrowed: %+v", input.MHS)
	}
}

func TestVerificationFeedbackCarriesSafeContextForPrimaryCorrection(t *testing.T) {
	candidate := toolcatalog.Tool{Alias: "screen.read", Source: "mhs", Target: map[string]any{"id": "display.main", "hwd": "display", "operation": "read"}}
	resolve := func(context.Context) ([]toolcatalog.Tool, error) {
		return []toolcatalog.Tool{candidate, {Alias: "lamp.read", Source: "mhs", Description: "configured lamp focus", Target: map[string]any{"id": "led.status", "hwd": "led", "operation": "read"}, HTTP: &toolkit.Tool{HTTP: &toolkit.HTTPRequest{Headers: map[string]string{"private": "must-not-reach-primary"}}}}}, nil
	}
	generator := &verificationGenerator{result: `{"approved":false,"reason":"wrong_target"}`}
	feedback, err := runtimeToolVerifier(generator, "model/checker", resolve)(t.Context(), candidate, json.RawMessage(`{}`), genx.ToolConversation{CurrentUser: "increase brightness"})
	if err != nil || !strings.Contains(feedback, "configured lamp focus") || !strings.Contains(feedback, "display.main") || !strings.Contains(feedback, "led.status") || strings.Contains(feedback, "must-not-reach-primary") {
		t.Fatalf("missing safe correction context: %q, %v", feedback, err)
	}
	if candidate.Target["id"] != "display.main" || !strings.Contains(feedback, "do not authorize a new action") {
		t.Fatal("feedback changed candidate or granted independent authority")
	}
}

func TestVerificationUserTurnsPreserveActualAudioTranscription(t *testing.T) {
	for _, currentWireText := range []string{"", "turn light up"} {
		users := toolVerificationUserTurns(genx.ToolConversation{CurrentUser: "turn light up", Messages: []genx.ToolConversationMessage{
			{Role: "user", Content: "previous user turn"},
			{Role: "assistant", Content: "proposed unrelated change"},
			{Role: "user", Content: currentWireText},
		}})
		if len(users) != 2 || users[0] != "previous user turn" || users[1] != "turn light up" {
			t.Fatalf("transcription omitted, duplicated or replaced by assistant: %v", users)
		}
	}
}
