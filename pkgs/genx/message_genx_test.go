package genx

import (
	"context"
	"strings"
	"testing"
)

type customPart struct{}

func (customPart) clone() Part { return customPart{} }
func (customPart) isPart()     {}

type customTool struct{}

func (*customTool) isTool() {}

func TestMessageChunkConstructorsAndClone(t *testing.T) {
	bos := NewBeginOfStream("s1")
	if !bos.IsBeginOfStream() || bos.IsEndOfStream() {
		t.Fatalf("unexpected bos chunk: %#v", bos)
	}

	eos := NewEndOfStream("audio/opus")
	if !eos.IsEndOfStream() || eos.Part.(*Blob).MIMEType != "audio/opus" {
		t.Fatalf("unexpected eos chunk: %#v", eos)
	}

	textEOS := NewTextEndOfStream()
	if !textEOS.IsEndOfStream() {
		t.Fatalf("unexpected text eos chunk: %#v", textEOS)
	}

	orig := &MessageChunk{
		Role:     RoleUser,
		Name:     "u",
		Part:     &Blob{MIMEType: "image/png", Data: []byte{1, 2, 3}},
		ToolCall: &ToolCall{ID: "id", FuncCall: &FuncCall{Name: "f", Arguments: "{}"}},
		Ctrl:     &StreamCtrl{BeginOfStream: true, StreamID: "s"},
	}
	clone := orig.Clone()
	if clone == orig {
		t.Fatal("clone should return different pointer")
	}
	cloneBlob := clone.Part.(*Blob)
	cloneBlob.Data[0] = 9
	if orig.Part.(*Blob).Data[0] == 9 {
		t.Fatal("clone should deep-copy blob data")
	}
}

func TestMessageChunkMIMEType(t *testing.T) {
	for _, tc := range []struct {
		name     string
		chunk    *MessageChunk
		mimeType string
		ok       bool
	}{
		{name: "text", chunk: &MessageChunk{Part: Text("hello")}, mimeType: "text/plain", ok: true},
		{name: "text eos", chunk: NewTextEndOfStream(), mimeType: "text/plain", ok: true},
		{name: "blob", chunk: &MessageChunk{Part: &Blob{MIMEType: " Audio/OGG; CODECS=opus "}}, mimeType: "audio/ogg; codecs=opus", ok: true},
		{name: "parameter order", chunk: &MessageChunk{Part: &Blob{MIMEType: "audio/L16; rate=16000; channels=1"}}, mimeType: "audio/l16; channels=1; rate=16000", ok: true},
		{name: "blob eos", chunk: NewEndOfStream("audio/opus"), mimeType: "audio/opus", ok: true},
		{name: "control eos", chunk: &MessageChunk{Ctrl: &StreamCtrl{EndOfStream: true}}},
		{name: "empty blob mime", chunk: &MessageChunk{Part: &Blob{}}},
		{name: "invalid blob mime", chunk: &MessageChunk{Part: &Blob{MIMEType: "audio/ogg; codecs"}}},
		{name: "nil blob", chunk: &MessageChunk{Part: (*Blob)(nil)}},
		{name: "nil chunk"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mimeType, ok := tc.chunk.MIMEType()
			if mimeType != tc.mimeType || ok != tc.ok {
				t.Fatalf("MIMEType() = %q, %t, want %q, %t", mimeType, ok, tc.mimeType, tc.ok)
			}
		})
	}
}

func TestFuncCallAndToolCallInvoke(t *testing.T) {
	call := &FuncCall{Name: "orphan"}
	if _, err := call.Invoke(context.Background()); err == nil {
		t.Fatal("expected missing tool error")
	}

	tool, err := NewFuncTool[struct {
		V int `json:"v"`
	}]("plus", "plus")
	if err != nil {
		t.Fatalf("new tool failed: %v", err)
	}
	tool.Invoke = nil
	if _, err := tool.NewFuncCall(`{"v":1}`).Invoke(context.Background()); err == nil {
		t.Fatal("expected missing invoke function error")
	}

	tool2 := MustNewFuncTool[struct {
		V int `json:"v"`
	}]("plus", "plus", InvokeFunc[struct {
		V int `json:"v"`
	}](func(_ context.Context, _ *FuncCall, arg struct {
		V int `json:"v"`
	}) (any, error) {
		return arg.V + 1, nil
	}))

	tc := &ToolCall{ID: "id-1", FuncCall: tool2.NewFuncCall(`{"v":2}`)}
	v, err := tc.Invoke(context.Background())
	if err != nil {
		t.Fatalf("tool call invoke failed: %v", err)
	}
	if v.(int) != 3 {
		t.Fatalf("unexpected invoke result: %v", v)
	}

	if _, err := (&ToolCall{ID: "id-2"}).Invoke(context.Background()); err == nil {
		t.Fatal("expected nil func call error")
	}
}

func TestInspectHelpersAndUsageString(t *testing.T) {
	if got := InspectMessage(nil); got != "" {
		t.Fatalf("expected empty inspect for nil message, got: %q", got)
	}

	msg := &Message{Role: RoleModel, Name: "assistant", Payload: Contents{Text("hello"), &Blob{MIMEType: "audio/wav", Data: []byte{1}}, customPart{}}}
	inspected := InspectMessage(msg)
	if !strings.Contains(inspected, "model") || !strings.Contains(inspected, "hello") || !strings.Contains(inspected, "audio/wav") {
		t.Fatalf("unexpected inspected message: %s", inspected)
	}

	if got := InspectTool(&SearchWebTool{}); got != "### SearchWebTool" {
		t.Fatalf("unexpected search web tool inspect: %q", got)
	}

	fTool := MustNewFuncTool[struct {
		A string `json:"a"`
	}]("do", "desc")
	if got := InspectTool(fTool); !strings.Contains(got, "### do") {
		t.Fatalf("unexpected func tool inspect: %q", got)
	}

	if got := InspectTool(&customTool{}); got != "" {
		t.Fatalf("unexpected custom tool inspect: %q", got)
	}

	var mcb ModelContextBuilder
	mcb.PromptText("sys", "prompt")
	mcb.UserText("u", "hello")
	mcb.AddTool(fTool)
	text, err := InspectModelContext(mcb.Build())
	if err != nil {
		t.Fatalf("inspect model context failed: %v", err)
	}
	if !strings.Contains(text, "prompt") || !strings.Contains(text, "hello") || !strings.Contains(text, "do") {
		t.Fatalf("unexpected inspected model context: %s", text)
	}

	usageText := (Usage{PromptTokenCount: 1, CachedContentTokenCount: 2, GeneratedTokenCount: 3}).String()
	if !strings.Contains(usageText, "Prompt") || !strings.Contains(usageText, "Generated") {
		t.Fatalf("unexpected usage string: %s", usageText)
	}

	if RoleUser.String() != "user" || RoleModel.String() != "model" || RoleTool.String() != "tool" {
		t.Fatalf("unexpected role string values")
	}
}
