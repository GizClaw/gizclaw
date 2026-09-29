package pcm

import "encoding/binary"

const wavHeaderSize = 44

// WAV wraps little-endian signed PCM data in format f as a canonical
// RIFF/WAVE file with a 44-byte header.
func (f Format) WAV(data []byte) []byte {
	channels := f.Channels()
	sampleRate := f.SampleRate()
	depth := f.Depth()
	blockAlign := channels * depth / 8
	out := make([]byte, wavHeaderSize, wavHeaderSize+len(data))
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(wavHeaderSize-8+len(data)))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], 1)
	binary.LittleEndian.PutUint16(out[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(out[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(sampleRate*blockAlign))
	binary.LittleEndian.PutUint16(out[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(out[34:36], uint16(depth))
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(len(data)))
	return append(out, data...)
}
