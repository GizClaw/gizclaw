package codecconv

import (
	"bytes"
	"testing"
)

func TestNormalizeTTSAudioStripsRepeatedID3Tags(t *testing.T) {
	data := append(fakeID3Header(), []byte("frame-a")...)
	data = append(data, fakeID3Header()...)
	data = append(data, []byte("frame-b")...)

	got := normalizeTTSAudio("audio/mpeg", data)
	if bytes.Contains(got, []byte("ID3")) {
		t.Fatalf("expected ID3 tags to be stripped from %q", got)
	}
	if string(got) != "frame-aframe-b" {
		t.Fatalf("normalizeTTSAudio() = %q, want frame-aframe-b", got)
	}
}

func TestTTSAudioNormalizerStripsID3AcrossChunkBoundaries(t *testing.T) {
	normalizer := newTTSAudioNormalizer("audio/mpeg")
	var got []byte
	first := append(fakeID3Tag([]byte("tag-a")), []byte("frame-a")...)
	second := append(fakeID3Tag([]byte("tag-b")), []byte("frame-b")...)
	for _, chunk := range [][]byte{
		first[:2],
		first[2:10],
		first[10:13],
		append(first[13:], second[:2]...),
		second[2:],
	} {
		got = append(got, normalizer.Write(chunk)...)
	}
	got = append(got, normalizer.Flush()...)
	if bytes.Contains(got, []byte("ID3")) {
		t.Fatalf("expected split ID3 tags to be stripped from %q", got)
	}
	if string(got) != "frame-aframe-b" {
		t.Fatalf("normalizer output = %q, want frame-aframe-b", got)
	}
}

func fakeID3Header() []byte {
	return []byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0}
}

func fakeID3Tag(payload []byte) []byte {
	header := fakeID3Header()
	size := len(payload)
	header[6] = byte((size >> 21) & 0x7f)
	header[7] = byte((size >> 14) & 0x7f)
	header[8] = byte((size >> 7) & 0x7f)
	header[9] = byte(size & 0x7f)
	return append(header, payload...)
}
