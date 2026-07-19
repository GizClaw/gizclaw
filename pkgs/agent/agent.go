// Package agent defines the common runtime contract for AI agents.
package agent

import (
	"errors"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/buffer"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// Agent is an AI runtime that consumes and produces GenX streams. Unlike an
// ordinary Transformer, an Agent owns model turns and completes any ToolCall
// loop before publishing the final model response.
type Agent interface {
	genx.Transformer
}

// IsStreamEnd reports whether err is a successful stream terminal state.
func IsStreamEnd(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, buffer.ErrIteratorDone) || errors.Is(err, genx.ErrDone) {
		return true
	}
	var state *genx.State
	return errors.As(err, &state) && state.Status() == genx.StatusDone
}
