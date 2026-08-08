package ogg

import (
	"bytes"
	"fmt"
	"math"
)

// Packet 是逻辑包（可跨多个 page）。
type Packet struct {
	Data            []byte
	GranulePosition uint64
	BOS             bool
	EOS             bool
}

func packetLacing(packetLen int) ([]byte, error) {
	if packetLen < 0 {
		return nil, fmt.Errorf("ogg: packet lacing: negative packet length %d", packetLen)
	}
	segs := make([]byte, 0, packetLen/maxSegmentSize+1)
	remain := packetLen
	for remain >= maxSegmentSize {
		segs = append(segs, maxSegmentSize)
		remain -= maxSegmentSize
	}
	segs = append(segs, byte(remain))
	return segs, nil
}

// BuildPacketPages 把一个逻辑包拆成一个或多个 page。
func BuildPacketPages(serial, sequence uint32, packet []byte, granulePos uint64, bos, eos bool) ([]*Page, error) {
	segs, err := packetLacing(len(packet))
	if err != nil {
		return nil, err
	}

	pageCount := (len(segs) + maxPageSegments - 1) / maxPageSegments
	if pageCount <= 0 {
		return nil, fmt.Errorf("ogg: build packet pages: internal invalid page count %d", pageCount)
	}
	if uint64(sequence)+uint64(pageCount)-1 > math.MaxUint32 {
		return nil, fmt.Errorf("ogg: build packet pages: page sequence overflow: start=%d pageCount=%d", sequence, pageCount)
	}

	pages := make([]*Page, 0, pageCount)
	payloadOffset := 0
	for idx, segOffset := 0, 0; segOffset < len(segs); idx, segOffset = idx+1, segOffset+maxPageSegments {
		end := min(segOffset+maxPageSegments, len(segs))
		chunkSegs := append([]byte(nil), segs[segOffset:end]...)

		chunkPayloadLen := 0
		for _, seg := range chunkSegs {
			chunkPayloadLen += int(seg)
		}
		if payloadOffset+chunkPayloadLen > len(packet) {
			return nil, fmt.Errorf("ogg: build packet pages: payload overflow: offset=%d chunk=%d total=%d", payloadOffset, chunkPayloadLen, len(packet))
		}
		payload := append([]byte(nil), packet[payloadOffset:payloadOffset+chunkPayloadLen]...)
		payloadOffset += chunkPayloadLen

		headerType := byte(0)
		if idx > 0 {
			headerType |= HeaderTypeContinued
		}
		if idx == 0 && bos {
			headerType |= HeaderTypeBOS
		}
		isLast := end == len(segs)
		if isLast && eos {
			headerType |= HeaderTypeEOS
		}

		pageGranule := GranulePositionUnknown
		if isLast {
			pageGranule = granulePos
		}

		p := &Page{
			Version:         0,
			HeaderType:      headerType,
			GranulePosition: pageGranule,
			BitstreamSerial: serial,
			PageSequence:    sequence + uint32(idx),
			Segments:        chunkSegs,
			Payload:         payload,
		}
		if err := p.Validate(); err != nil {
			return nil, fmt.Errorf("ogg: build packet pages: %w", err)
		}
		pages = append(pages, p)
	}

	if payloadOffset != len(packet) {
		return nil, fmt.Errorf("ogg: build packet pages: payload not fully consumed: consumed=%d total=%d", payloadOffset, len(packet))
	}

	return pages, nil
}

// MarshalPages 顺序序列化多个 page 为连续字节流。
func MarshalPages(pages []*Page) ([]byte, error) {
	var out bytes.Buffer
	for idx, p := range pages {
		if p == nil {
			return nil, fmt.Errorf("ogg: marshal pages: page[%d] is nil", idx)
		}
		raw, err := p.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("ogg: marshal pages: page[%d]: %w", idx, err)
		}
		if _, err := out.Write(raw); err != nil {
			return nil, fmt.Errorf("ogg: marshal pages: page[%d] write buffer failed: %w", idx, err)
		}
	}
	return out.Bytes(), nil
}

// ExtractPackets 从按序 page 列表中重建逻辑包。
func ExtractPackets(pages []*Page) ([]Packet, error) {
	packets := make([]Packet, 0, len(pages))

	var buf []byte
	expectingContinuation := false
	currentPacketBOS := false

	for pageIdx, page := range pages {
		if page == nil {
			return nil, fmt.Errorf("ogg: extract packets: page[%d] is nil", pageIdx)
		}
		if err := page.Validate(); err != nil {
			return nil, fmt.Errorf("ogg: extract packets: page[%d]: %w", pageIdx, err)
		}

		if page.HasContinuation() {
			if !expectingContinuation {
				return nil, fmt.Errorf("ogg: extract packets: unexpected continuation on page %d", pageIdx)
			}
		} else if expectingContinuation {
			return nil, fmt.Errorf("ogg: extract packets: missing continuation before page %d", pageIdx)
		}

		payloadOffset := 0
		for segIdx, seg := range page.Segments {
			if !expectingContinuation && len(buf) == 0 {
				currentPacketBOS = page.HasBOS() && segIdx == 0
			}

			chunkLen := int(seg)
			if payloadOffset+chunkLen > len(page.Payload) {
				return nil, fmt.Errorf(
					"ogg: extract packets: page %d segment %d overflows payload: offset=%d chunk=%d payload=%d",
					pageIdx,
					segIdx,
					payloadOffset,
					chunkLen,
					len(page.Payload),
				)
			}
			if chunkLen > 0 {
				buf = append(buf, page.Payload[payloadOffset:payloadOffset+chunkLen]...)
			}
			payloadOffset += chunkLen

			if seg < maxSegmentSize {
				pktData := append([]byte(nil), buf...)
				pkt := Packet{
					Data:            pktData,
					GranulePosition: page.GranulePosition,
					BOS:             currentPacketBOS,
					EOS:             page.HasEOS() && segIdx == len(page.Segments)-1,
				}
				packets = append(packets, pkt)

				buf = buf[:0]
				expectingContinuation = false
				currentPacketBOS = false
			} else {
				expectingContinuation = true
			}
		}

		if payloadOffset != len(page.Payload) {
			return nil, fmt.Errorf("ogg: extract packets: page %d has trailing payload: consumed=%d total=%d", pageIdx, payloadOffset, len(page.Payload))
		}
	}

	if expectingContinuation {
		return nil, fmt.Errorf("ogg: extract packets: stream ended with unterminated packet")
	}

	return packets, nil
}
