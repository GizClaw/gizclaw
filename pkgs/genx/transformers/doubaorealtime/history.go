package doubaorealtime

import (
	"fmt"
	"strings"

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
	if next.Ctrl == nil {
		next.Ctrl = &genx.StreamCtrl{}
	}
	next.Ctrl.StreamID = streamID
	next.Ctrl.Label = genx.HistoryUserAudioLabel
	return next
}

func historyUserAudioEOSChunk(streamID, mimeType string) *genx.MessageChunk {
	if strings.TrimSpace(streamID) == "" {
		streamID = "audio"
	}
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "audio/pcm"
	}
	return &genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: &genx.Blob{MIMEType: mimeType}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: genx.HistoryUserAudioLabel, EndOfStream: true}}
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
