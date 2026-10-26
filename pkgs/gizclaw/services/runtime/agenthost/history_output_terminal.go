package agenthost

import "github.com/GizClaw/gizclaw-go/pkgs/genx"

// historyTerminalBody separates readable data from its durable completion
// boundary. Only EOS waits for persistence; the next route's data keeps flowing.
// The original upstream delivery acknowledgement stays on the terminal.
func historyTerminalBody(terminal *genx.MessageChunk) *genx.MessageChunk {
	var body *genx.MessageChunk
	switch part := terminal.Part.(type) {
	case genx.Text:
		if part == "" {
			return nil
		}
		body = terminal.Clone()
		terminal.Part = genx.Text("")
	case *genx.Blob:
		if part == nil || len(part.Data) == 0 {
			return nil
		}
		body = terminal.Clone()
		terminal.Part = &genx.Blob{MIMEType: part.MIMEType}
	default:
		return nil
	}
	terminal.Ctrl.BeginOfStream = false
	body.Ctrl.MessageEnd = false
	body.Ctrl.EndOfStream = false
	body.Ctrl.ResponseEpochEnd = false
	genx.SetStreamError(body.Ctrl, nil)
	body.Ctrl.ErrorCode = ""
	body.Ctrl.ErrorRetryable = false
	body.Ctrl.FailureClass = ""
	return body
}
