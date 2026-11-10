package genx

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/pion/rtp"
)

func TestBlobAudioTimingIsOwnedClonedAndPrivate(t *testing.T) {
	info := &packet.OpusInfo{SourceID: "private-track", Header: rtp.Header{Version: 2, SequenceNumber: 12, Timestamp: 3840, CSRC: []uint32{7}}, Duration48: 960}
	chunk := &MessageChunk{Part: &Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}, AudioTiming: packet.NewOpusTiming(info)}}
	clone := chunk.Clone().Part.(*Blob)
	info.Header.CSRC[0] = 99
	timing := clone.AudioTiming.MediaInfo()
	timing.Header.CSRC[0] = 55
	original := chunk.Part.(*Blob)
	if original.AudioTiming == clone.AudioTiming || clone.AudioTiming.MediaInfo().Header.CSRC[0] != 7 || original.AudioTiming.MediaInfo().Header.CSRC[0] != 7 {
		t.Fatal("media handoff shares caller or clone metadata")
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("AudioTiming")) || bytes.Contains(encoded, []byte("private-track")) || bytes.Contains(encoded, []byte("3840")) {
		t.Fatalf("media metadata escaped local stream: %s", encoded)
	}
	if (&Blob{}).clone().(*Blob).AudioTiming != nil {
		t.Fatal("untimed clone gained timing")
	}
}
