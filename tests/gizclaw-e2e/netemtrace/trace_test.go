package netemtrace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/pion/rtp"
)

func capture(t *testing.T, observe func()) map[string]any {
	t.Helper()
	t.Setenv("NETEM_NODE", "client")
	path := filepath.Join(t.TempDir(), "trace")
	output, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = output
	defer func() { os.Stderr = previous; _ = output.Close() }()
	observe()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(string(data), "NETEM_TRACE ")), &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestTunnelRTPObserver(t *testing.T) {
	payload := []byte{0xf8, 0xff, 0xfe}
	media := &rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: 65535, Timestamp: 0xfffffff0, SSRC: 9,
		CSRC: []uint32{4, 5}}, Payload: payload}
	if err := media.SetExtension(1, []byte{7, 8}); err != nil {
		t.Fatal(err)
	}
	encoded, err := media.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, 17)
	frame[0], frame[1] = 3, 1
	frame = append(frame, encoded...)
	record := capture(t, func() { RecordTunnel("tunnel_wire_read", frame) })
	if record["error"] != nil || record["hash"] != digest(payload) || record["wire_hash"] != digest(frame) ||
		record["sequence"] != float64(65535) || record["timestamp"] != float64(0xfffffff0) ||
		record["ssrc"] != float64(9) || record["bytes"] != float64(len(payload)) || record["duration48"] != float64(960) {
		t.Fatalf("RTP envelope was not parsed independently of its payload: %v", record)
	}
	media.SequenceNumber = 0
	next, err := media.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	nextFrame := append(append([]byte(nil), frame[:17]...), next...)
	nextRecord := capture(t, func() { RecordTunnel("tunnel_wire_read", nextFrame) })
	if nextRecord["hash"] != record["hash"] || nextRecord["wire_hash"] == record["wire_hash"] {
		t.Fatal("repeated codec payloads must retain distinct wire RTP identities")
	}
}

func TestTunnelLegacyAndMalformedObserver(t *testing.T) {
	frame := make([]byte, 17)
	frame[0], frame[1] = 2, 1
	frame = append(frame, 0xf8, 0xff, 0xfe)
	record := capture(t, func() { RecordTunnel("wire", frame) })
	if record["error"] != nil || record["sequence"] != float64(-1) || record["hash"] != digest(frame[17:]) {
		t.Fatalf("legacy packet observation: %v", record)
	}
	for _, invalid := range [][]byte{nil, frame[:17], append([]byte{3}, frame[1:]...), append([]byte{4}, frame[1:]...)} {
		record := capture(t, func() { RecordTunnel("wire", invalid) })
		if record["error"] == nil {
			t.Fatalf("malformed envelope silently accepted: %v", record)
		}
	}
}

func TestRecoveredObserverDoesNotInventEncodedPLC(t *testing.T) {
	pcm, original := make([]byte, 640), []byte{0xf8, 0xff, 0xfe}
	info := &packet.OpusInfo{SourceID: "session/ssrc", Header: rtp.Header{Version: 2, SequenceNumber: 7, Timestamp: 960}, Duration48: 960}
	real := capture(t, func() { RecordFrame("media_recovered", pcm, original, false, info) })
	if real["error"] != nil || real["hash"] != digest(original) || real["pcm_hash"] != digest(pcm) || real["pcm_bytes"] != float64(640) {
		t.Fatalf("real original/PCM observation: %v", real)
	}
	plc := capture(t, func() { RecordFrame("media_recovered", pcm, nil, true, info) })
	if plc["error"] != nil || plc["hash"] != "" || plc["bytes"] != float64(0) || plc["concealed"] != true {
		t.Fatalf("PLC must contain PCM only: %v", plc)
	}
	invalid := capture(t, func() { RecordFrame("media_recovered", pcm, original, true, info) })
	if invalid["error"] == nil {
		t.Fatal("fabricated encoded PLC did not fail closed")
	}
}

func TestBlobObserverUsesRealOriginal(t *testing.T) {
	blob := &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: make([]byte, 640),
		OriginalAudio: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}}
	record := capture(t, func() { RecordBlob("client_recovered", blob) })
	if record["hash"] != digest(blob.OriginalAudio.Data) || record["pcm_hash"] != digest(blob.Data) || record["error"] != nil {
		t.Fatalf("blob observer confused decoded and encoded audio: %v", record)
	}
}
