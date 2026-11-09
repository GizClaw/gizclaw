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

// Clone returns timing metadata with independent RTP header storage.
func (i *OpusInfo) Clone() *OpusInfo {
	if i == nil {
		return nil
	}
	cloned := *i
	cloned.Header = i.Header.Clone()
	return &cloned
}
