package audiostream

import (
	"bytes"
	"testing"
)

func TestNormalizerPassesThroughFormatsWithoutHandling(t *testing.T) {
	for _, mimeType := range []string{"", "audio/ogg", "audio/pcm", "application/octet-stream"} {
		normalizer := NewNormalizer(mimeType)
		data := []byte("audio-ID3-data")
		if got := normalizer.Normalize(data); !bytes.Equal(got, data) {
			t.Errorf("NewNormalizer(%q).Normalize() = %q, want passthrough", mimeType, got)
		}
		if got := normalizer.Flush(); len(got) != 0 {
			t.Errorf("NewNormalizer(%q).Flush() = %q, want empty", mimeType, got)
		}
	}
}

func TestNormalizerRemovesMP3ID3AcrossChunkBoundaries(t *testing.T) {
	for _, mimeType := range []string{"audio/mpeg", "audio/mp3", "audio/x-mpeg", " Audio/MPEG ; bitrate=128"} {
		t.Run(mimeType, func(t *testing.T) {
			normalizer := NewNormalizer(mimeType)
			var got []byte
			first := append(fakeID3v2Tag([]byte("tag-a")), []byte("frame-a")...)
			second := append(fakeID3v2Tag([]byte("tag-b")), []byte("frame-b")...)
			for _, chunk := range [][]byte{
				first[:2],
				first[2:10],
				first[10:13],
				append(first[13:], second[:2]...),
				second[2:],
			} {
				got = append(got, normalizer.Normalize(chunk)...)
			}
			got = append(got, normalizer.Flush()...)
			if bytes.Contains(got, []byte("ID3")) {
				t.Fatalf("normalized MP3 still contains ID3 metadata: %q", got)
			}
			if string(got) != "frame-aframe-b" {
				t.Fatalf("normalized MP3 = %q, want frame-aframe-b", got)
			}
		})
	}
}

func TestNormalizerPreservesInvalidMP3ID3Bytes(t *testing.T) {
	normalizer := NewNormalizer("audio/mpeg")
	data := []byte{'I', 'D', '3', 4, 0, 0, 0x80, 0, 0, 0, 'f'}
	got := append(normalizer.Normalize(data), normalizer.Flush()...)
	if !bytes.Equal(got, data) {
		t.Fatalf("normalized MP3 = %v, want invalid ID3 bytes preserved", got)
	}
}

func TestNormalizerRemovesTrailingMP3ID3v1Tag(t *testing.T) {
	normalizer := NewNormalizer("audio/mpeg")
	tag := make([]byte, 128)
	copy(tag, "TAG")
	copy(tag[20:], "ID3 inside metadata")
	data := append([]byte("frame-a"), tag...)
	var got []byte
	for _, chunk := range [][]byte{data[:5], data[5:70], data[70:]} {
		got = append(got, normalizer.Normalize(chunk)...)
	}
	got = append(got, normalizer.Flush()...)
	if string(got) != "frame-a" {
		t.Fatalf("normalized MP3 = %q, want frame-a", got)
	}
}

func fakeID3v2Tag(payload []byte) []byte {
	header := []byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0}
	size := len(payload)
	header[6] = byte((size >> 21) & 0x7f)
	header[7] = byte((size >> 14) & 0x7f)
	header[8] = byte((size >> 7) & 0x7f)
	header[9] = byte(size & 0x7f)
	return append(header, payload...)
}
