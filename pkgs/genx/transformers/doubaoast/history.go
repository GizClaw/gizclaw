package doubaoast

import (
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

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
	// This is a newly generated history route. Source audio boundaries belong
	// to the input route and must not leak into the generated lifecycle.
	next.Ctrl.BeginOfStream = false
	next.Ctrl.EndOfStream = false
	next.Ctrl.Error = ""
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

func historyUserAudioBOSChunk(streamID, mimeType string) *genx.MessageChunk {
	chunk := historyUserAudioEOSChunk(streamID, mimeType)
	chunk.Ctrl.BeginOfStream = true
	chunk.Ctrl.EndOfStream = false
	return chunk
}

type timestampedHistoryAudioBlock struct {
	chunk *genx.MessageChunk
	start time.Duration
	end   time.Duration
}

type timestampedHistoryAudioBuffer struct {
	blocks  []timestampedHistoryAudioBlock
	cursor  time.Duration
	flushed time.Duration
}

func (b *timestampedHistoryAudioBuffer) reset() {
	if b == nil {
		return
	}
	b.blocks = b.blocks[:0]
	b.cursor = 0
	b.flushed = 0
}

func (b *timestampedHistoryAudioBuffer) append(chunk *genx.MessageChunk, streamID string, start, duration time.Duration) {
	if b == nil || chunk == nil {
		return
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob == nil || len(blob.Data) == 0 || baseAudioMIME(blob.MIMEType) != "audio/opus" {
		return
	}
	// Subtitle times use the decoded PCM sent to the provider. Incoming chunk
	// timestamps describe arrival and can contain gaps or bursts absent from
	// that audio. Keep sub-millisecond samples and any converted audio prefix.
	end := start + duration
	b.cursor = end
	b.blocks = append(b.blocks, timestampedHistoryAudioBlock{chunk: historyUserAudioChunk(chunk, streamID), start: start, end: end})
}

func (b *timestampedHistoryAudioBuffer) segment(startMS, endMS int) []*genx.MessageChunk {
	if b == nil {
		return nil
	}
	start, end := time.Duration(startMS)*time.Millisecond, time.Duration(endMS)*time.Millisecond
	useFlushCursor := startMS <= 0 && endMS <= 0
	if useFlushCursor {
		start, end = b.flushed, b.cursor
	}
	if end <= start {
		return nil
	}
	var out []*genx.MessageChunk
	flushed := b.flushed
	for _, block := range b.blocks {
		if block.end <= start || block.start >= end {
			continue
		}
		out = append(out, block.chunk.Clone())
		flushed = max(flushed, block.end)
	}
	if len(out) > 0 && useFlushCursor {
		b.flushed = flushed
	}
	return out
}

func pushHistoryAudioSegment(output interface {
	Push(*genx.MessageChunk) error
}, streamID string, chunks []*genx.MessageChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	mimeType := "audio/opus"
	for _, chunk := range chunks {
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
