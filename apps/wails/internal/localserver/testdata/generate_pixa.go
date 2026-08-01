package main

import (
	"encoding/binary"
	"os"
)

const (
	width       = 96
	height      = 104
	headerSize  = 40
	paletteSize = 4
	clipSize    = 56
	frameSize   = 16
)

func main() {
	indices := make([]byte, 0, width*height)
	for y := range height {
		for x := range width {
			index := byte(1)
			if x == 0 || x == width-1 || y == 0 || y == height-1 {
				index = 0
			}
			indices = append(indices, index)
		}
	}
	payload := make([]byte, 0, height*6)
	for start := 0; start < len(indices); {
		end := start + 1
		for end < len(indices) && end-start < 255 && indices[end] == indices[start] {
			end++
		}
		payload = append(payload, byte(end-start), indices[start])
		start = end
	}

	paletteOffset := headerSize
	clipOffset := paletteOffset + paletteSize
	frameOffset := clipOffset + clipSize
	payloadOffset := frameOffset + frameSize
	data := make([]byte, payloadOffset+len(payload))
	copy(data, "PIXA")
	binary.LittleEndian.PutUint16(data[4:6], 1)
	binary.LittleEndian.PutUint16(data[6:8], headerSize)
	binary.LittleEndian.PutUint16(data[8:10], width)
	binary.LittleEndian.PutUint16(data[10:12], height)
	binary.LittleEndian.PutUint16(data[12:14], 2)
	binary.LittleEndian.PutUint16(data[14:16], 1)
	binary.LittleEndian.PutUint32(data[16:20], 1)
	binary.LittleEndian.PutUint32(data[20:24], uint32(paletteOffset))
	binary.LittleEndian.PutUint32(data[24:28], uint32(clipOffset))
	binary.LittleEndian.PutUint32(data[28:32], uint32(frameOffset))
	binary.LittleEndian.PutUint32(data[32:36], uint32(payloadOffset))
	binary.LittleEndian.PutUint32(data[36:40], uint32(len(payload)))

	binary.LittleEndian.PutUint16(data[paletteOffset+2:paletteOffset+4], 0x07e0)
	copy(data[clipOffset:clipOffset+32], "idle")
	binary.LittleEndian.PutUint32(data[clipOffset+36:clipOffset+40], 0)
	binary.LittleEndian.PutUint32(data[clipOffset+40:clipOffset+44], 1)
	binary.LittleEndian.PutUint32(data[clipOffset+44:clipOffset+48], 100)
	binary.LittleEndian.PutUint16(data[clipOffset+48:clipOffset+50], 1)

	binary.LittleEndian.PutUint16(data[frameOffset:frameOffset+2], 100)
	data[frameOffset+2] = 0
	data[frameOffset+3] = 1
	binary.LittleEndian.PutUint32(data[frameOffset+4:frameOffset+8], 0)
	binary.LittleEndian.PutUint32(data[frameOffset+8:frameOffset+12], uint32(len(payload)))
	copy(data[payloadOffset:], payload)

	if err := os.WriteFile("transparent-96x104.pixa", data, 0o644); err != nil {
		panic(err)
	}
}
