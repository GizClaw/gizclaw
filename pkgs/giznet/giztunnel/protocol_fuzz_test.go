package giztunnel

import "testing"

func FuzzParseTunnelLabel(f *testing.F) {
	f.Add("giznet/v2/tunnel/00112233445566778899aabbccddeeff/packet")
	f.Add("")
	f.Fuzz(func(t *testing.T, label string) {
		_, _ = parseLabel(label)
	})
}

func FuzzDecodeSessionResult(f *testing.F) {
	f.Add([]byte{'G', 'Z', 'T', '2', 0, 0, 0})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, payload []byte) {
		_, _, _ = decodeSessionResult(payload)
	})
}
