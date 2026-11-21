package eino

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Controllable provider whose response contains text and an unsolicited call.
// The stream outlives the foreground turn until released or the owner closes.
type discardProbe struct {
	fakeChatModel
	started chan []*schema.Message
	release chan struct{}
	drained chan bool
	fail    bool
}

func (p *discardProbe) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if len(model.GetCommonOptions(nil, opts...).Tools) != 0 {
		return nil, errors.New("discard provider received tools")
	}
	p.started <- messages
	r, w := schema.Pipe[*schema.Message](0)
	go func() {
		defer w.Close()
		select {
		case <-ctx.Done():
			p.drained <- false
			return
		case <-p.release:
		}
		if p.fail {
			w.Send(nil, errors.New("JIUTIAN_SENTINEL_ERROR"))
			p.drained <- true
			return
		}
		closed := w.Send(&schema.Message{Role: schema.Assistant, Content: "JIUTIAN_SENTINEL", ToolCalls: []schema.ToolCall{{ID: "bogus", Type: "function", Function: schema.FunctionCall{Name: "lookup", Arguments: `{"value":"JIUTIAN_SENTINEL"}`}}}}, nil)
		p.drained <- !closed
	}()
	return r, nil
}

func TestDiscardResponseIsolatedFromToolTurn(t *testing.T) {
	for _, mode := range []string{"drain", "provider error", "generation close"} {
		t.Run(mode, func(t *testing.T) {
			p := &discardProbe{started: make(chan []*schema.Message, 1), release: make(chan struct{}), drained: make(chan bool, 1), fail: mode == "provider error"}
			seed := &scriptedChatModel{rounds: [][]*schema.Message{
				{{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "seed-call", Type: "function", Function: schema.FunctionCall{Name: "lookup", Arguments: `{"value":"requested"}`}}}}},
				{schema.AssistantMessage("Seed reply", nil)},
			}}
			cfg := chatConfig(&namedComponentResolver{chat: map[string]model.BaseChatModel{"chat": seed, "carrier": p}})
			cfg.Graph.Nodes = append(cfg.Graph.Nodes, NodeDefinition{ID: "observe", Inputs: map[string]Binding{"messages": {From: "messages"}}, ChatModel: &ChatModelNode{Model: "carrier", DiscardResponse: true}})
			cfg.Graph.Edges = []EdgeDefinition{{From: "start", To: "prompt"}, {From: "prompt", To: "observe"}, {From: "observe", To: "model"}, {From: "model", To: "end"}}
			var calls []string
			cfg.ToolInvoker = einoTestToolInvoker(func(value string) (any, error) {
				calls = append(calls, value)
				return map[string]string{"found": value}, nil
			})
			tr, err := New(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer tr.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			out, err := tr.Transform(ctx, textInput("requested"))
			if err != nil {
				t.Fatal(err)
			}
			if got := joinedText(drain(t, out)); got != "Seed reply" {
				t.Fatalf("response while carrier is blocked = %q", got)
			}
			select {
			case submitted := <-p.started:
				if len(submitted) != 2 || submitted[1].Content != "requested" {
					t.Fatalf("carrier input = %#v", submitted)
				}
			case <-ctx.Done():
				t.Fatal("carrier request was not sent")
			}
			if mode == "generation close" {
				tr.Close()
			} else {
				close(p.release)
			}
			select {
			case consumed := <-p.drained:
				if consumed != (mode != "generation close") {
					t.Fatal("unexpected discard stream lifecycle")
				}
			case <-ctx.Done():
				t.Fatal("discard stream leaked")
			}
			tr.Close()
			if len(calls) != 1 || calls[0] != "requested" || len(seed.inputs) != 2 {
				t.Fatalf("calls=%v seed rounds=%d", calls, len(seed.inputs))
			}
			data, err := json.Marshal(seed.inputs)
			if err != nil || strings.Contains(string(data), "JIUTIAN_SENTINEL") || !strings.Contains(string(data), `found`) {
				t.Fatalf("Seed continuation: %s", data)
			}
		})
	}
}

func TestDiscardResponseRejectsPublishedOutput(t *testing.T) {
	cfg := chatConfig(&componentMapResolver{chat: &fakeChatModel{}})
	cfg.Graph.Nodes[1].ChatModel.DiscardResponse = true
	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "discard_response") {
		t.Fatalf("configuration error = %v", err)
	}
}
