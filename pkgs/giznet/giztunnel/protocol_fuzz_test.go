package giztunnel

import (
	"bytes"
	"testing"
)

func FuzzReadFrame(f *testing.F) {
	f.Add([]byte("GZT1\x02\x00\x00\x00\x00"))
	f.Add([]byte("bad"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = readFrame(bytes.NewReader(data), defaultMaxFrameSize)
	})
}

func FuzzDecodeOpenRequest(f *testing.F) {
	f.Add([]byte(`{"version":1}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = decodeOpenRequest(data)
	})
}
