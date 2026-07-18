package ogg

import (
	"fmt"
	"strings"
)

// PacketDecoder incrementally reconstructs Ogg packets from arbitrarily chunked input.
type PacketDecoder struct {
	pending               []byte
	packet                []byte
	expectingContinuation bool
	currentPacketBOS      bool
}

// Write consumes Ogg bytes and returns every complete logical packet now available.
func (d *PacketDecoder) Write(data []byte) ([]Packet, error) {
	if d == nil {
		return nil, fmt.Errorf("ogg: packet decoder is nil")
	}
	if len(data) == 0 {
		return nil, nil
	}
	d.pending = append(d.pending, data...)
	var packets []Packet
	for {
		page, ok, err := d.nextPage()
		if err != nil {
			return nil, err
		}
		if !ok {
			return packets, nil
		}
		pagePackets, err := d.consumePage(page)
		if err != nil {
			return nil, err
		}
		packets = append(packets, pagePackets...)
	}
}

// Close validates that the input ended at a complete Ogg packet boundary.
func (d *PacketDecoder) Close() error {
	if d == nil {
		return nil
	}
	if len(d.pending) != 0 {
		return fmt.Errorf("ogg: truncated page: %d pending bytes", len(d.pending))
	}
	if d.expectingContinuation || len(d.packet) != 0 {
		return fmt.Errorf("ogg: stream ended with unterminated packet")
	}
	return nil
}

func (d *PacketDecoder) nextPage() (*Page, bool, error) {
	if len(d.pending) == 0 {
		return nil, false, nil
	}
	if len(d.pending) < pageHeaderSize {
		if len(d.pending) < len(CapturePattern) && !strings.HasPrefix(CapturePattern, string(d.pending)) {
			return nil, false, fmt.Errorf("ogg: invalid capture pattern prefix %q", d.pending)
		}
		if len(d.pending) >= len(CapturePattern) && string(d.pending[:len(CapturePattern)]) != CapturePattern {
			return nil, false, fmt.Errorf("ogg: invalid capture pattern prefix %q", d.pending)
		}
		return nil, false, nil
	}
	if string(d.pending[:len(CapturePattern)]) != CapturePattern {
		return nil, false, fmt.Errorf("ogg: invalid capture pattern %q", d.pending[:len(CapturePattern)])
	}
	segmentCount := int(d.pending[26])
	headerLen := pageHeaderSize + segmentCount
	if len(d.pending) < headerLen {
		return nil, false, nil
	}
	payloadLen := 0
	for _, segment := range d.pending[pageHeaderSize:headerLen] {
		payloadLen += int(segment)
	}
	pageLen := headerLen + payloadLen
	if len(d.pending) < pageLen {
		return nil, false, nil
	}
	page, err := ParsePage(d.pending[:pageLen])
	if err != nil {
		return nil, false, err
	}
	d.pending = d.pending[pageLen:]
	return page, true, nil
}

func (d *PacketDecoder) consumePage(page *Page) ([]Packet, error) {
	if page == nil {
		return nil, fmt.Errorf("ogg: page is nil")
	}
	if page.HasContinuation() {
		if !d.expectingContinuation {
			return nil, fmt.Errorf("ogg: unexpected continuation page")
		}
	} else if d.expectingContinuation {
		return nil, fmt.Errorf("ogg: missing continuation page")
	}

	var packets []Packet
	payloadOffset := 0
	for segmentIndex, segment := range page.Segments {
		if !d.expectingContinuation && len(d.packet) == 0 {
			d.currentPacketBOS = page.HasBOS() && segmentIndex == 0
		}
		chunkLen := int(segment)
		if payloadOffset+chunkLen > len(page.Payload) {
			return nil, fmt.Errorf("ogg: segment overflows payload")
		}
		if chunkLen > 0 {
			d.packet = append(d.packet, page.Payload[payloadOffset:payloadOffset+chunkLen]...)
		}
		payloadOffset += chunkLen
		if segment == maxSegmentSize {
			d.expectingContinuation = true
			continue
		}
		packets = append(packets, Packet{
			Data:            append([]byte(nil), d.packet...),
			GranulePosition: page.GranulePosition,
			BOS:             d.currentPacketBOS,
			EOS:             page.HasEOS() && segmentIndex == len(page.Segments)-1,
		})
		d.packet = d.packet[:0]
		d.expectingContinuation = false
		d.currentPacketBOS = false
	}
	if payloadOffset != len(page.Payload) {
		return nil, fmt.Errorf("ogg: page has trailing payload")
	}
	return packets, nil
}
