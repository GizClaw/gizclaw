package giztestcmd

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"mime"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// peerAudioIntegrity observes raw audio channel boundaries and payload, before
// response filtering. It intentionally does not infer missing BOS/EOS. A caller
// may retain it across turns to catch late packets from a completed response.
// The stream reader owns observe; summary is called after that reader's delivery.
type peerAudioIntegrity struct {
	active                         map[string]bool
	ended                          map[string]bool
	streams, maxActive, violations int
	violationReasons               map[string]int
	digest                         hash.Hash
}

func (p *peerAudioIntegrity) observe(chunk *genx.MessageChunk) {
	if chunk == nil {
		return
	}
	if chunk.Part == nil && chunk.IsEndOfStream() && chunk.Ctrl != nil {
		prefix := chunk.Ctrl.StreamID + "\x00"
		for id := range p.active {
			if strings.HasPrefix(id, prefix) {
				delete(p.active, id)
				p.ended[id] = true
			}
		}
		return
	}
	mediaType, _ := chunk.MIMEType()
	if base, params, err := mime.ParseMediaType(mediaType); err == nil {
		mediaType = mime.FormatMediaType(base, params)
	}
	audio := strings.HasPrefix(mediaType, "audio/") || strings.HasPrefix(mediaType, "application/ogg")
	if !audio {
		return
	}
	if p.active == nil {
		p.active = make(map[string]bool)
		p.ended = make(map[string]bool)
	}
	if chunk.Ctrl == nil || chunk.Ctrl.StreamID == "" {
		p.violation("missing_stream_id")
		return
	}
	id := chunk.Ctrl.StreamID + "\x00" + mediaType
	if chunk.IsBeginOfStream() {
		if p.ended[id] {
			p.violation("begin_after_end")
		} else if p.active[id] {
			p.violation("duplicate_begin")
		}
		p.active[id] = true
		p.streams++
		p.maxActive = max(p.maxActive, len(p.active))
	}
	if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
		if p.ended[id] {
			p.violation("payload_after_end")
		} else if !p.active[id] {
			p.violation("payload_without_begin")
		}
		if p.digest == nil {
			p.digest = sha256.New()
		}
		_, _ = p.digest.Write(blob.Data)
	}
	if chunk.IsEndOfStream() {
		if !p.active[id] {
			if p.ended[id] {
				p.violation("duplicate_end")
			} else {
				p.violation("end_without_begin")
			}
		}
		delete(p.active, id)
		p.ended[id] = true
	}
}

func (p *peerAudioIntegrity) violation(reason string) {
	p.violations++
	if p.violationReasons == nil {
		p.violationReasons = make(map[string]int)
	}
	p.violationReasons[reason]++
}

func (p *peerAudioIntegrity) summary() map[string]any {
	result := map[string]any{"streams": p.streams, "max_active": p.maxActive, "open": len(p.active), "violations": p.violations}
	if len(p.violationReasons) > 0 {
		result["violation_reasons"] = p.violationReasons
	}
	if p.digest != nil {
		result["sha256"] = hex.EncodeToString(p.digest.Sum(nil))
	}
	return result
}
