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

// holdForwardTerminal keeps a successful EOS cancellable while its History
// durability fence is pending. A later interruption owns the live terminal.
func (o *historyOutput) holdForwardTerminal(chunk *genx.MessageChunk) {
	route, ok := historyForwardChunkRoute(chunk)
	if !ok {
		return
	}
	mimeType, _ := historyForwardChunkMIME(chunk)
	o.forwardMu.Lock()
	defer o.forwardMu.Unlock()
	if o.pendingTerminalForward == nil {
		o.pendingTerminalForward = make(map[*genx.MessageChunk]historyForwardChunkKey)
	}
	o.pendingTerminalForward[chunk] = historyForwardChunkKey{historyForwardRouteKey: route.key(), mimeType: mimeType}
}

func (o *historyOutput) cancelPendingForwardTerminals(interrupt *genx.MessageChunk) {
	route, ok := historyForwardChunkRoute(interrupt)
	if !ok {
		return
	}
	mimeType, hasMIME := historyForwardChunkMIME(interrupt)
	var discarded []*genx.MessageChunk
	o.forwardMu.Lock()
	for chunk, key := range o.pendingTerminalForward {
		if key.streamID == route.streamID && (!hasMIME || (key.historyForwardRouteKey == route.key() && key.mimeType == mimeType)) {
			delete(o.pendingTerminalForward, chunk)
			discarded = append(discarded, chunk)
		}
	}
	o.forwardMu.Unlock()
	for _, chunk := range discarded {
		o.abandonPendingObservation(chunk)
	}
}

// addForwardTerminal serializes the nonblocking buffer insertion with
// cancellation. The interruption can then be inserted only after this EOS,
// or remove this EOS before either reaches the output buffer.
func (o *historyOutput) addForwardTerminal(chunk *genx.MessageChunk) (bool, error) {
	o.forwardMu.Lock()
	defer o.forwardMu.Unlock()
	if _, pending := o.pendingTerminalForward[chunk]; !pending {
		return false, nil
	}
	delete(o.pendingTerminalForward, chunk)
	return true, o.output.Add(chunk)
}

func (o *historyOutput) discardForwardTerminal(chunk *genx.MessageChunk) {
	o.forwardMu.Lock()
	delete(o.pendingTerminalForward, chunk)
	o.forwardMu.Unlock()
	o.abandonPendingObservation(chunk)
}
