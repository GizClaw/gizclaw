package pcm

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFormatWAV(t *testing.T) {
	data := []byte{1, 0, 2, 0, 3, 0, 4, 0}
	wav := L16Stereo24K.WAV(data)
	if len(wav) != wavHeaderSize+len(data) {
		t.Fatalf("len = %d, want %d", len(wav), wavHeaderSize+len(data))
	}
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" || string(wav[12:16]) != "fmt " || string(wav[36:40]) != "data" {
		t.Fatalf("unexpected chunk ids in header % x", wav[:wavHeaderSize])
	}
	for _, check := range []struct {
		name string
		got  uint32
		want uint32
	}{
		{"riff size", binary.LittleEndian.Uint32(wav[4:8]), uint32(36 + len(data))},
		{"format", uint32(binary.LittleEndian.Uint16(wav[20:22])), 1},
		{"channels", uint32(binary.LittleEndian.Uint16(wav[22:24])), 2},
		{"sample rate", binary.LittleEndian.Uint32(wav[24:28]), 24000},
		{"byte rate", binary.LittleEndian.Uint32(wav[28:32]), 24000 * 4},
		{"block align", uint32(binary.LittleEndian.Uint16(wav[32:34])), 4},
		{"bits", uint32(binary.LittleEndian.Uint16(wav[34:36])), 16},
		{"data size", binary.LittleEndian.Uint32(wav[40:44]), uint32(len(data))},
	} {
		if check.got != check.want {
			t.Errorf("%s = %d, want %d", check.name, check.got, check.want)
		}
	}
	if !bytes.Equal(wav[wavHeaderSize:], data) {
		t.Fatalf("payload = % x, want % x", wav[wavHeaderSize:], data)
	}
}
