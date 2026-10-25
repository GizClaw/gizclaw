package giztestcmd

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const peerStreamResponseHistoryLimit = 256

// A retained reader remembers recent response IDs even between operations.
// New input IDs are fresh, so a response already observed by that reader cannot
// be the reply to input that this operation has not submitted yet.
type peerStreamResponseHistory struct {
	mu    sync.Mutex
	ids   []string
	known map[string]time.Time
}

func peerStreamAssistantResponse(chunk *genx.MessageChunk) (string, bool) {
	if chunk == nil {
		return "", false
	}
	id, label := "", ""
	if chunk.Ctrl != nil {
		id = strings.TrimSpace(chunk.Ctrl.StreamID)
		label = strings.TrimSpace(chunk.Ctrl.Label)
	}
	if label == "" {
		switch chunk.Part.(type) {
		case genx.Text, *genx.Blob:
			label = "assistant"
		}
	}
	return id, label == "assistant"
}

func (h *peerStreamResponseHistory) observe(chunk *genx.MessageChunk, receivedAt time.Time) {
	if h == nil {
		return
	}
	id, assistant := peerStreamAssistantResponse(chunk)
	if !assistant || id == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if first, exists := h.known[id]; exists {
		if receivedAt.Before(first) {
			h.known[id] = receivedAt
		}
		return
	}
	if h.known == nil {
		h.known = make(map[string]time.Time)
	}
	if len(h.ids) == peerStreamResponseHistoryLimit {
		delete(h.known, h.ids[0])
		copy(h.ids, h.ids[1:])
		h.ids[len(h.ids)-1] = id
	} else {
		h.ids = append(h.ids, id)
	}
	h.known[id] = receivedAt
}

func (h *peerStreamResponseHistory) snapshot() map[string]bool {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	known := make(map[string]bool, len(h.known))
	for id := range h.known {
		known[id] = true
	}
	return known
}

func (h *peerStreamResponseHistory) firstSeen(id string, fallback time.Time) time.Time {
	if h == nil || id == "" {
		return fallback
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if first, ok := h.known[id]; ok && first.Before(fallback) {
		return first
	}
	return fallback
}

// The same filter owns eligibility for both consumption and deadline rescue.
// It is installed before reading input, including a retained reader's backlog.
// Once an ID is earlier, its later packets and terminal events stay earlier.
type peerStreamResponseFilter struct {
	history    *peerStreamResponseHistory
	allowEarly bool
	mu         sync.Mutex
	commitAt   time.Time
	earlier    map[string]bool
	overflow   bool
}

func (f *peerStreamResponseFilter) begin(known map[string]bool) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commitAt = time.Time{}
	f.earlier = known
	if f.earlier == nil {
		f.earlier = make(map[string]bool)
	}
}

func (f *peerStreamResponseFilter) commit(at time.Time) {
	if f == nil {
		return
	}
	f.mu.Lock()
	if f.commitAt.IsZero() {
		f.commitAt = at
	}
	f.mu.Unlock()
}

func (f *peerStreamResponseFilter) accepts(chunk *genx.MessageChunk, receivedAt time.Time) bool {
	if f == nil {
		return true
	}
	id, assistant := peerStreamAssistantResponse(chunk)
	if !assistant {
		return true
	}
	firstSeen := receivedAt
	if !f.allowEarly {
		firstSeen = f.history.firstSeen(id, receivedAt)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.allowEarly {
		return !f.earlier[id]
	}
	if f.commitAt.IsZero() || firstSeen.Before(f.commitAt) {
		if id != "" && !f.earlier[id] {
			if len(f.earlier) == 2*peerStreamResponseHistoryLimit {
				f.overflow = true
			} else {
				f.earlier[id] = true
			}
		}
		return false
	}
	return !f.overflow && !f.earlier[id]
}

func (f *peerStreamResponseFilter) err() error {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.overflow {
		return errors.New("peer_stream earlier response tracking limit exceeded")
	}
	return nil
}
