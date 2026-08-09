package doubaorealtime

import (
	"fmt"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const historyFallbackOpusPacketDurationMS = 20

func historyUserAudioChunk(chunk *genx.MessageChunk, streamID string) *genx.MessageChunk {
	if strings.TrimSpace(streamID) == "" {
		streamID = "audio"
	}
	next := chunk.Clone()
	next.Role = genx.RoleUser
	next.Name = "transcript"
	if blob, ok := next.Part.(*genx.Blob); ok && blob != nil {
		blob.MIMEType = canonicalHistoryAudioMIME(blob.MIMEType)
	}
	if next.Ctrl == nil {
		next.Ctrl = &genx.StreamCtrl{}
	}
	next.Ctrl.StreamID = streamID
	next.Ctrl.Label = genx.HistoryUserAudioLabel
	next.Ctrl.BeginOfStream = false
	next.Ctrl.EndOfStream = false
	next.Ctrl.Error = ""
	return next
}

func historyUserAudioBOSChunk(streamID, mimeType string) *genx.MessageChunk {
	streamID, mimeType = normalizeHistoryUserAudioRoute(streamID, mimeType)
	return &genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: &genx.Blob{MIMEType: mimeType}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: genx.HistoryUserAudioLabel, BeginOfStream: true}}
}

func historyUserAudioEOSChunk(streamID, mimeType string) *genx.MessageChunk {
	streamID, mimeType = normalizeHistoryUserAudioRoute(streamID, mimeType)
	return &genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: &genx.Blob{MIMEType: mimeType}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: genx.HistoryUserAudioLabel, EndOfStream: true}}
}

func normalizeHistoryUserAudioRoute(streamID, mimeType string) (string, string) {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = "audio"
	}
	mimeType = canonicalHistoryAudioMIME(mimeType)
	return streamID, mimeType
}

func canonicalHistoryAudioMIME(mimeType string) string {
	chunk := &genx.MessageChunk{Part: &genx.Blob{MIMEType: strings.TrimSpace(mimeType)}}
	if canonical, ok := chunk.MIMEType(); ok {
		return canonical
	}
	return "audio/pcm"
}

type doubaoRealtimeHistoryRoute struct {
	streamID string
	mimeType string
}

type doubaoRealtimeHistoryRoutes struct {
	mu   sync.Mutex
	open map[doubaoRealtimeHistoryRoute]struct{}
}

func newDoubaoRealtimeHistoryRoutes() *doubaoRealtimeHistoryRoutes {
	return &doubaoRealtimeHistoryRoutes{open: make(map[doubaoRealtimeHistoryRoute]struct{})}
}

func (r *doubaoRealtimeHistoryRoutes) push(output realtimeChunkOutput, chunk *genx.MessageChunk, streamID string) error {
	if r == nil || chunk == nil {
		return nil
	}
	mimeType, _ := chunk.MIMEType()
	streamID, mimeType = normalizeHistoryUserAudioRoute(streamID, mimeType)
	route := doubaoRealtimeHistoryRoute{streamID: streamID, mimeType: mimeType}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.open[route]; !ok {
		if err := output.Push(historyUserAudioBOSChunk(streamID, mimeType)); err != nil {
			return fmt.Errorf("push history user audio bos: %w", err)
		}
		r.open[route] = struct{}{}
	}
	if err := output.Push(historyUserAudioChunk(chunk, streamID)); err != nil {
		return fmt.Errorf("push history user audio data: %w", err)
	}
	return nil
}

func (r *doubaoRealtimeHistoryRoutes) close(output realtimeChunkOutput, streamID, mimeType string) error {
	if r == nil {
		return nil
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = "audio"
	}
	mimeType = strings.TrimSpace(mimeType)
	if mimeType != "" {
		mimeType = canonicalHistoryAudioMIME(mimeType)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	var routes []doubaoRealtimeHistoryRoute
	for route := range r.open {
		if route.streamID == streamID && (mimeType == "" || route.mimeType == mimeType) {
			routes = append(routes, route)
		}
	}
	if len(routes) == 0 {
		streamID, mimeType = normalizeHistoryUserAudioRoute(streamID, mimeType)
		route := doubaoRealtimeHistoryRoute{streamID: streamID, mimeType: mimeType}
		if err := output.Push(historyUserAudioBOSChunk(streamID, mimeType)); err != nil {
			return fmt.Errorf("push history user audio bos: %w", err)
		}
		r.open[route] = struct{}{}
		routes = append(routes, route)
	}
	for _, route := range routes {
		if err := output.Push(historyUserAudioEOSChunk(route.streamID, route.mimeType)); err != nil {
			return fmt.Errorf("push history user audio eos: %w", err)
		}
		delete(r.open, route)
	}
	return nil
}

type timestampedHistoryAudioBlock struct {
	chunk   *genx.MessageChunk
	startMS int
	endMS   int
}

type timestampedHistoryAudioBuffer struct {
	blocks    []timestampedHistoryAudioBlock
	baseTS    int64
	haveTS    bool
	cursorMS  int
	flushedMS int
}

func (b *timestampedHistoryAudioBuffer) reset() {
	if b == nil {
		return
	}
	b.blocks = b.blocks[:0]
	b.baseTS = 0
	b.haveTS = false
	b.cursorMS = 0
	b.flushedMS = 0
}

func (b *timestampedHistoryAudioBuffer) append(chunk *genx.MessageChunk, streamID string) {
	if b == nil || chunk == nil {
		return
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob == nil || len(blob.Data) == 0 || realtimeBaseMIME(blob.MIMEType) != "audio/opus" {
		return
	}
	next := historyUserAudioChunk(chunk, streamID)
	next.Ctrl.BeginOfStream = false
	next.Ctrl.EndOfStream = false
	next.Ctrl.Error = ""
	startMS := b.cursorMS
	if next.Ctrl.Timestamp > 0 {
		if !b.haveTS {
			b.baseTS = next.Ctrl.Timestamp
			b.haveTS = true
		}
		startMS = max(int(next.Ctrl.Timestamp-b.baseTS), 0)
	}
	if n := len(b.blocks); n > 0 && b.blocks[n-1].endMS <= startMS {
		b.blocks[n-1].endMS = startMS
	}
	endMS := startMS + historyOpusPacketDurationMS(blob.Data)
	if endMS <= startMS {
		endMS = startMS + historyFallbackOpusPacketDurationMS
	}
	b.cursorMS = endMS
	b.blocks = append(b.blocks, timestampedHistoryAudioBlock{chunk: next, startMS: startMS, endMS: endMS})
}

func (b *timestampedHistoryAudioBuffer) segment(startMS, endMS int) []*genx.MessageChunk {
	if b == nil {
		return nil
	}
	useFlushCursor := startMS <= 0 && endMS <= 0
	if useFlushCursor {
		startMS, endMS = b.flushedMS, b.cursorMS
	}
	if endMS <= startMS {
		return nil
	}
	var out []*genx.MessageChunk
	flushed := b.flushedMS
	for _, block := range b.blocks {
		if block.endMS <= startMS || block.startMS >= endMS {
			continue
		}
		out = append(out, block.chunk.Clone())
		flushed = max(flushed, block.endMS)
	}
	if len(out) > 0 && useFlushCursor {
		b.flushedMS = flushed
	}
	return out
}

func historyOpusPacketDurationMS(packet []byte) int {
	ticks := codecconv.OpusPacketRTPTicks(packet)
	if ticks == 0 {
		return historyFallbackOpusPacketDurationMS
	}
	ms := int(ticks / 48)
	if ms <= 0 {
		return historyFallbackOpusPacketDurationMS
	}
	return ms
}

func pushHistoryAudioSegment(output interface {
	Push(*genx.MessageChunk) error
}, streamID string, chunks []*genx.MessageChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	mimeType := "audio/opus"
	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && strings.TrimSpace(blob.MIMEType) != "" {
			mimeType = blob.MIMEType
			break
		}
	}
	if err := output.Push(historyUserAudioBOSChunk(streamID, mimeType)); err != nil {
		return fmt.Errorf("push history user audio bos: %w", err)
	}
	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		if err := output.Push(chunk); err != nil {
			return err
		}
	}
	if err := output.Push(historyUserAudioEOSChunk(streamID, mimeType)); err != nil {
		return fmt.Errorf("push history user audio eos: %w", err)
	}
	return nil
}
