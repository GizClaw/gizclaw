package giztunnel

import (
	"bytes"
	"net"
	"testing"
)

type recordingBuffersWriter struct {
	bytes.Buffer
	calls int
}

func (w *recordingBuffersWriter) WriteBuffers(buffers net.Buffers) (int64, error) {
	w.calls++
	return buffers.WriteTo(&w.Buffer)
}

func TestWriteFrameCoalescesHeaderAndPayloadWhenSupported(t *testing.T) {
	w := &recordingBuffersWriter{}
	payload := bytes.Repeat([]byte{0x42}, 32*1024)
	if err := writeFrame(w, frameStreamData, payload, defaultMaxFrameSize); err != nil {
		t.Fatal(err)
	}
	if w.calls != 1 {
		t.Fatalf("WriteBuffers calls = %d, want 1", w.calls)
	}
	typ, got, err := readFrame(bytes.NewReader(w.Bytes()), defaultMaxFrameSize)
	if err != nil {
		t.Fatal(err)
	}
	if typ != frameStreamData || !bytes.Equal(got, payload) {
		t.Fatalf("decoded frame type=%d payload=%d bytes", typ, len(got))
	}
}
