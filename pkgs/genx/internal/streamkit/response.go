package streamkit

import (
	"sort"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const textMIME = "text/plain"

// ResponseConfig declares the stable metadata for one logical response route.
// An empty StreamID is replaced with a fresh ID. Role, Name, and Label are
// copied to synthesized terminal chunks and fill missing metadata on emitted
// chunks without imposing model- or agent-specific defaults.
type ResponseConfig struct {
	StreamID string
	Role     genx.Role
	Name     string
	Label    string
}

// Response tracks MIME-route completion for one logical StreamID. It is safe
// for concurrent producer and interruption paths.
type Response struct {
	mu       sync.Mutex
	config   ResponseConfig
	routes   map[string]bool
	terminal bool
}

// NewResponse starts a response with caller-supplied metadata.
func NewResponse(config ResponseConfig) *Response {
	config.StreamID = strings.TrimSpace(config.StreamID)
	if config.StreamID == "" {
		config.StreamID = genx.NewStreamID()
	}
	return &Response{config: config, routes: make(map[string]bool)}
}

// StreamID returns this response's immutable route identity.
func (r *Response) StreamID() string {
	if r == nil {
		return ""
	}
	return r.config.StreamID
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
		if streamID != "" && streamID != r.config.StreamID {
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
func (r *Response) End(errorText string) []*genx.MessageChunk {
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
		return []*genx.MessageChunk{r.controlEOS(errorText)}
	}
	mimeTypes := make([]string, 0, len(r.routes))
	for mimeType, done := range r.routes {
		if !done {
			mimeTypes = append(mimeTypes, mimeType)
		}
	}
	if len(mimeTypes) == 0 && strings.TrimSpace(errorText) != "" {
		return []*genx.MessageChunk{r.controlEOS(errorText)}
	}
	sort.Strings(mimeTypes)
	chunks := make([]*genx.MessageChunk, 0, len(mimeTypes))
	for _, mimeType := range mimeTypes {
		r.routes[mimeType] = true
		chunks = append(chunks, r.routeEOS(mimeType, errorText))
	}
	return chunks
}

func (r *Response) endAfterDiscard(errorText string, discarded []*genx.MessageChunk) []*genx.MessageChunk {
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
		return []*genx.MessageChunk{r.controlEOS(errorText)}
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
		chunks = append(chunks, r.routeEOS(mimeType, errorText))
	}
	return chunks
}

func (r *Response) applyMetadata(chunk *genx.MessageChunk) *genx.MessageChunk {
	copyChunk := *chunk
	if copyChunk.Role == "" {
		copyChunk.Role = r.config.Role
	}
	if copyChunk.Name == "" {
		copyChunk.Name = r.config.Name
	}
	copyCtrl := genx.StreamCtrl{StreamID: r.config.StreamID, Label: r.config.Label}
	if chunk.Ctrl != nil {
		copyCtrl = *chunk.Ctrl
		if strings.TrimSpace(copyCtrl.StreamID) == "" {
			copyCtrl.StreamID = r.config.StreamID
		}
		if copyCtrl.Label == "" {
			copyCtrl.Label = r.config.Label
		}
	}
	copyChunk.Ctrl = &copyCtrl
	return &copyChunk
}

func (r *Response) controlEOS(errorText string) *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: r.config.Role,
		Name: r.config.Name,
		Ctrl: &genx.StreamCtrl{
			StreamID:    r.config.StreamID,
			Label:       r.config.Label,
			Error:       errorText,
			EndOfStream: true,
		},
	}
}

func (r *Response) routeEOS(mimeType, errorText string) *genx.MessageChunk {
	chunk := r.controlEOS(errorText)
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
