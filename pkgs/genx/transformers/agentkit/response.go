package agentkit

import (
	"sort"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const textMIME = "text/plain"

// Response tracks MIME-route completion for one assistant response StreamID.
// It is safe for concurrent provider-reader and interruption paths.
type Response struct {
	mu       sync.Mutex
	streamID string
	routes   map[string]bool
	terminal bool
}

// NewResponse starts a response. An empty ID is replaced with a fresh StreamID.
func NewResponse(streamID string) *Response {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = genx.NewStreamID()
	}
	return &Response{streamID: streamID, routes: make(map[string]bool)}
}

// StreamID returns this response's immutable route identity.
func (r *Response) StreamID() string {
	if r == nil {
		return ""
	}
	return r.streamID
}

// Declare marks a canonical MIME channel as open.
func (r *Response) Declare(mimeType string) bool {
	if r == nil {
		return false
	}
	mimeType = canonicalMIME(mimeType)
	if mimeType == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.routes[mimeType]; r.terminal || exists {
		return false
	}
	r.routes[mimeType] = false
	return true
}

// Accept reports whether a chunk can still enter this response. Data declares
// its MIME route; a route EOS closes only that MIME; a control-only EOS closes
// the complete response.
func (r *Response) Accept(chunk *genx.MessageChunk) bool {
	if r == nil || chunk == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.terminal {
		return false
	}
	if chunk.Ctrl != nil {
		streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
		if streamID != "" && streamID != r.streamID {
			return false
		}
		if chunk.IsEndOfStream() && chunk.Part == nil {
			r.terminal = true
			return true
		}
	}
	mimeType, ok := chunk.MIMEType()
	if !ok {
		return true
	}
	if done, exists := r.routes[mimeType]; exists && done {
		return false
	}
	if _, exists := r.routes[mimeType]; !exists {
		r.routes[mimeType] = false
	}
	if chunk.IsEndOfStream() {
		r.routes[mimeType] = true
	}
	return true
}

// End closes every still-open MIME route and returns one EOS chunk per route.
// The returned chunks retain the response StreamID and supplied label/error.
func (r *Response) End(label, errorText string) []*genx.MessageChunk {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.terminal {
		return nil
	}
	r.terminal = true
	if len(r.routes) == 0 {
		return []*genx.MessageChunk{{
			Role: genx.RoleModel,
			Ctrl: &genx.StreamCtrl{
				StreamID:    r.streamID,
				Label:       label,
				Error:       errorText,
				EndOfStream: true,
			},
		}}
	}
	mimeTypes := make([]string, 0, len(r.routes))
	for mimeType, done := range r.routes {
		if done {
			continue
		}
		mimeTypes = append(mimeTypes, mimeType)
	}
	sort.Strings(mimeTypes)
	chunks := make([]*genx.MessageChunk, 0, len(mimeTypes))
	for _, mimeType := range mimeTypes {
		r.routes[mimeType] = true
		chunks = append(chunks, routeEOS(r.streamID, label, mimeType, errorText))
	}
	return chunks
}

// endAfterDiscard closes the response after buffered chunks were discarded.
// A provider EOS that was discarded was never visible to the caller, so it is
// replaced with the supplied terminal EOS. Routes whose EOS was already pulled
// are not emitted a second time.
func (r *Response) endAfterDiscard(label, errorText string, discarded []*genx.MessageChunk) []*genx.MessageChunk {
	if r == nil {
		return nil
	}
	discardedRoutes := make(map[string]bool)
	discardedControlEOS := false
	for _, chunk := range discarded {
		if chunk == nil || !chunk.IsEndOfStream() {
			continue
		}
		if mimeType, ok := chunk.MIMEType(); ok {
			discardedRoutes[mimeType] = true
		} else {
			discardedControlEOS = true
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	wasTerminal := r.terminal
	r.terminal = true
	if wasTerminal && !discardedControlEOS {
		return nil
	}
	if len(r.routes) == 0 {
		return []*genx.MessageChunk{{
			Role: genx.RoleModel,
			Ctrl: &genx.StreamCtrl{
				StreamID:    r.streamID,
				Label:       label,
				Error:       errorText,
				EndOfStream: true,
			},
		}}
	}
	mimeTypes := make([]string, 0, len(r.routes))
	for mimeType, done := range r.routes {
		if !done || discardedRoutes[mimeType] {
			mimeTypes = append(mimeTypes, mimeType)
		}
		r.routes[mimeType] = true
	}
	sort.Strings(mimeTypes)
	chunks := make([]*genx.MessageChunk, 0, len(mimeTypes))
	for _, mimeType := range mimeTypes {
		chunks = append(chunks, routeEOS(r.streamID, label, mimeType, errorText))
	}
	return chunks
}

func routeEOS(streamID, label, mimeType, errorText string) *genx.MessageChunk {
	chunk := &genx.MessageChunk{Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{
		StreamID:    streamID,
		Label:       label,
		Error:       errorText,
		EndOfStream: true,
	}}
	if mimeType == textMIME {
		chunk.Part = genx.Text("")
	} else {
		chunk.Part = &genx.Blob{MIMEType: mimeType}
	}
	return chunk
}

func canonicalMIME(mimeType string) string {
	chunk := &genx.MessageChunk{Part: &genx.Blob{MIMEType: strings.TrimSpace(mimeType)}}
	canonical, ok := chunk.MIMEType()
	if !ok {
		return ""
	}
	return canonical
}
