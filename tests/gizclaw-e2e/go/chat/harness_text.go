//go:build gizclaw_e2e

package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func (d *personaDriver) runTextRoundtrip(ctx context.Context) (roundStats, error) {
	stat := roundStats{Index: 1}
	d.useRoundtripUtterances()
	userText, err := d.nextUtterance(ctx, 1)
	if err != nil {
		return stat, err
	}
	stat.UserText = userText
	stat.Transcript = userText
	if d.transport == nil {
		if err := d.resetTransport(); err != nil {
			return stat, fmt.Errorf("open text transport: %w", err)
		}
	}
	if d.reloadAgent != nil {
		if err := d.reloadAgent(ctx); err != nil && !isAgentAlreadyRunning(err) {
			return stat, fmt.Errorf("reload text workspace: %w", err)
		}
	}
	d.transport.drain()

	streamID := "workspacetest-text-" + genx.NewStreamID()
	started := time.Now()
	if err := d.transport.sendTextTurn(ctx, streamID, userText); err != nil {
		return stat, fmt.Errorf("send text turn: %w", err)
	}
	stat.UplinkSend = time.Since(started)
	var assistant strings.Builder
	var trace roundEventTrace
	deadline := time.NewTimer(d.roundResponseTimeout())
	defer deadline.Stop()
	for stat.AssistantTextDone == 0 {
		select {
		case <-ctx.Done():
			return stat, fmt.Errorf("wait text response: %w; recent events: %s", ctx.Err(), trace.String())
		case <-deadline.C:
			return stat, fmt.Errorf("text response timeout; recent events: %s", trace.String())
		case err := <-d.transport.errs:
			return stat, fmt.Errorf("text transport: %w; recent events: %s", err, trace.String())
		case received := <-d.transport.events:
			event := received.event
			label := eventLabel(event)
			trace.add("event stream=%s label=%s type=%s text=%q error=%s", eventStreamID(event), label, event.Type, eventText(event), eventError(event))
			if message, ok := peerEventError(event); ok {
				return stat, fmt.Errorf("text peer event error: %s; recent events: %s", message, trace.String())
			}
			if label != "assistant" {
				continue
			}
			stat.EventCount++
			if event.Text != nil && strings.TrimSpace(*event.Text) != "" {
				if stat.FirstAssistantTextChunk == 0 {
					stat.FirstAssistantTextChunk = received.since(started)
					stat.FirstAssistantText = *event.Text
				}
				assistant.WriteString(*event.Text)
			}
			if isAssistantTextDoneEvent(event) {
				stat.AssistantTextDone = received.since(started)
			}
		}
	}
	stat.AssistantText = strings.TrimSpace(assistant.String())
	stat.ResponseTotal = time.Since(started)
	stat.WorkspaceTotal = stat.ResponseTotal
	if stat.AssistantText == "" {
		return stat, fmt.Errorf("text response is empty; recent events: %s", trace.String())
	}
	if err := validateAssistantOutputText(stat.AssistantText); err != nil {
		return stat, fmt.Errorf("text response: %w", err)
	}
	fmt.Printf("workspace_progress event=text_round_done workspace=%s assistant_chars=%d total=%s\n",
		d.cfg.Workspace, runeCount(stat.AssistantText), stat.WorkspaceTotal.Truncate(time.Millisecond))
	return stat, nil
}

func (t *chatTransport) sendTextTurn(ctx context.Context, streamID, text string) error {
	label := "workspacetest"
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, BeginOfStream: true}},
		{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label}},
		{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, EndOfStream: true}},
	} {
		if err := t.stream.Push(ctx, chunk); err != nil {
			return err
		}
	}
	return nil
}
