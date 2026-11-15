// Package netemtrace observes the isolated audio-netem build overlay only.
// Release builds never import this package. Payloads are identified by SHA-256;
// no identifiers are added to audio or to the production wire protocol.
package netemtrace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/pion/rtp"
)

var writeMu sync.Mutex

func digest(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func observation(stage string, data []byte, detail string, sequence, timestamp int64) map[string]any {
	return map[string]any{
		"stage": stage, "hash": digest(data), "bytes": len(data), "detail": detail,
		"sequence": sequence, "timestamp": timestamp,
	}
}

func emit(record map[string]any) {
	if os.Getenv("NETEM_NODE") == "" {
		return
	}
	record["node"] = os.Getenv("NETEM_NODE")
	record["ns"] = time.Now().UnixNano()
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	line := append([]byte("NETEM_TRACE "), data...)
	line = append(line, '\n')
	// Only output serialization is locked; no transport state is held here.
	writeMu.Lock()
	defer writeMu.Unlock()
	_, _ = os.Stderr.Write(line)
}

// Record emits one bounded observation, without retaining or printing payloads.
func Record(stage string, data []byte, detail string, sequence, timestamp int64) {
	emit(observation(stage, data, detail, sequence, timestamp))
}

func withInfo(record map[string]any, info *packet.OpusInfo) {
	if info == nil {
		return
	}
	record["sequence"] = int64(info.Header.SequenceNumber)
	record["timestamp"] = int64(info.Header.Timestamp)
	record["ssrc"] = info.Header.SSRC
	record["source"] = info.SourceID
	record["duration48"] = info.Duration48
}

// RecordOpus observes an encoded packet without creating timing metadata.
func RecordOpus(stage string, data []byte, detail string, info *packet.OpusInfo) {
	record := observation(stage, data, detail, -1, -1)
	withInfo(record, info)
	emit(record)
}

// RecordTunnel parses the actual v3 RTP envelope or the legacy v2 raw payload.
// Invalid envelopes emit an explicit observer error; they never disappear from
// the evidence or masquerade as received media.
func RecordTunnel(stage string, frame []byte) {
	record := observation(stage, nil, "", -1, -1)
	record["wire_hash"] = digest(frame)
	if len(frame) <= 17 {
		record["error"] = "short tunnel media frame"
		emit(record)
		return
	}
	record["wire_version"] = int(frame[0])
	record["detail"] = hex.EncodeToString(frame[1:17])
	switch frame[0] {
	case 2:
		record["hash"], record["bytes"] = digest(frame[17:]), len(frame[17:])
	case 3:
		var media rtp.Packet
		if err := media.Unmarshal(frame[17:]); err != nil || media.Version != 2 || len(media.Payload) == 0 {
			record["error"] = "invalid tunnel RTP packet"
			break
		}
		record["hash"], record["bytes"] = digest(media.Payload), len(media.Payload)
		withInfo(record, &packet.OpusInfo{Header: media.Header, Duration48: packet.OpusDuration48(media.Payload)})
	default:
		record["error"] = fmt.Sprintf("unknown tunnel media version %d", frame[0])
	}
	emit(record)
}

// RecordFrame keeps PCM/PLC observations separate from real encoded originals.
// A concealed frame has an empty encoded hash and cannot satisfy packet proof.
func RecordFrame(stage string, pcm, original []byte, concealed bool, info *packet.OpusInfo) {
	record := observation(stage, original, "", -1, -1)
	record["pcm_hash"], record["pcm_bytes"] = digest(pcm), len(pcm)
	record["concealed"] = concealed
	withInfo(record, info)
	if len(pcm) == 0 || len(pcm)%2 != 0 || concealed == (len(original) > 0) {
		record["error"] = "invalid recovered PCM/original provenance"
	}
	emit(record)
}

// RecordBlob observes the actual application boundary, including the legacy
// encoded fallback. PCM is never hashed or counted as an encoded Opus packet.
func RecordBlob(stage string, blob *genx.Blob) {
	if blob == nil || len(blob.Data) == 0 {
		return
	}
	if blob.MIMEType == "audio/pcm;rate=16000;channels=1" {
		var original []byte
		if blob.OriginalAudio != nil && blob.OriginalAudio.MIMEType == "audio/opus" {
			original = blob.OriginalAudio.Data
		}
		RecordFrame(stage, blob.Data, original, blob.ConcealedAudio, nil)
	} else if strings.HasPrefix(blob.MIMEType, "audio/opus") {
		Record(stage, blob.Data, blob.MIMEType, -1, -1)
	} else {
		record := observation(stage, nil, blob.MIMEType, -1, -1)
		record["error"] = "unsupported observed audio format"
		emit(record)
	}
}
