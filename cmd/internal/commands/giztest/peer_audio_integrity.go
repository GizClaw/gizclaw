package giztestcmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"mime"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// peerAudioIntegrity observes raw audio channel boundaries and payload, before
// response filtering. It intentionally does not infer missing BOS/EOS. Payload
// for a known-ended stream is ignored without changing the delivered digest.
// The stream reader owns observe; summary is called after that reader's delivery.
type peerAudioIntegrity struct {
	active                         map[string]bool
	ended                          map[string]bool
	streams, maxActive, violations int
	ignoredPostEOSPackets          uint64
	violationReasons               map[string]int
	digest                         hash.Hash
}

func (p *peerAudioIntegrity) observe(chunk *genx.MessageChunk) bool {
	if chunk == nil {
		return true
	}
	if chunk.Part == nil && chunk.IsEndOfStream() && chunk.Ctrl != nil {
		prefix := chunk.Ctrl.StreamID + "\x00"
		for id := range p.active {
			if strings.HasPrefix(id, prefix) {
				delete(p.active, id)
				p.ended[id] = true
			}
		}
		return true
	}
	mediaType, _ := chunk.MIMEType()
	if base, params, err := mime.ParseMediaType(mediaType); err == nil {
		mediaType = mime.FormatMediaType(base, params)
	}
	audio := strings.HasPrefix(mediaType, "audio/") || strings.HasPrefix(mediaType, "application/ogg")
	if !audio {
		return true
	}
	if p.active == nil {
		p.active = make(map[string]bool)
		p.ended = make(map[string]bool)
	}
	if chunk.Ctrl == nil || chunk.Ctrl.StreamID == "" {
		p.violation("missing_stream_id")
		return true
	}
	id := chunk.Ctrl.StreamID + "\x00" + mediaType
	if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 && p.ended[id] && !chunk.IsBeginOfStream() && !chunk.IsEndOfStream() && chunk.Ctrl.Error == "" && chunk.Ctrl.ErrorCode == "" {
		p.ignoredPostEOSPackets++
		return false
	}
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
	return true
}

func (p *peerAudioIntegrity) violation(reason string) {
	p.violations++
	if p.violationReasons == nil {
		p.violationReasons = make(map[string]int)
	}
	p.violationReasons[reason]++
}

func (p *peerAudioIntegrity) summary() map[string]any {
	result := map[string]any{"streams": p.streams, "max_active": p.maxActive, "open": len(p.active), "violations": p.violations, "ignored_post_eos_packets": json.Number(strconv.FormatUint(p.ignoredPostEOSPackets, 10))}
	if len(p.violationReasons) > 0 {
		result["violation_reasons"] = p.violationReasons
	}
	if p.digest != nil {
		result["sha256"] = hex.EncodeToString(p.digest.Sum(nil))
	}
	return result
}
