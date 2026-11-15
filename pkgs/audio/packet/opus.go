// Package packet describes encoded audio packet timing independently of a
// transport, decoder implementation, or product session lifecycle.
package packet

import "github.com/pion/rtp"

// OpusClockRate is the RTP media clock rate for Opus, including when PCM is
// decoded at another sample rate.
const OpusClockRate = 48000

// OpusInfo carries the timing of one received Opus packet or a known gap.
// It is process-local metadata; Header retains the original RTP identity and
// timing rather than replacing them with packet arrival times.
type OpusInfo struct {
	// SourceID distinguishes decoder histories, including across connections.
	SourceID string
	Header   rtp.Header
	// Duration48 is the duration in 48 kHz samples per channel. Missing packets
	// require an exact duration inferred from the media timeline.
	Duration48 uint32
	// Missing requests concealment for a known gap and requires an empty payload.
	Missing bool
	// Discontinuity ends the preceding decoder history without inventing audio
	// for an unknown or excessively long gap.
	Discontinuity bool
}

// OpusTiming is an opaque, process-local handoff of encoded media timing.
// Stream consumers carry it unchanged; media adapters interpret the RTP clock.
// It owns its metadata and never owns a decoder or packet buffer.
type OpusTiming struct {
	info *OpusInfo
}

// NewOpusTiming snapshots timing for a real encoded packet. Nil stays untimed.
func NewOpusTiming(info *OpusInfo) *OpusTiming {
	if info == nil {
		return nil
	}
	return &OpusTiming{info: info.Clone()}
}

// Clone returns an independent handoff, including all retained header storage.
func (t *OpusTiming) Clone() *OpusTiming {
	if t == nil {
		return nil
	}
	return NewOpusTiming(t.info)
}

// MediaInfo returns an owned snapshot for media-layer adapters only.
func (t *OpusTiming) MediaInfo() *OpusInfo {
	if t == nil {
		return nil
	}
	return t.info.Clone()
}

// Clone returns timing metadata with independent RTP header storage.
func (i *OpusInfo) Clone() *OpusInfo {
	if i == nil {
		return nil
	}
	cloned := *i
	cloned.Header = i.Header.Clone()
	return &cloned
}
