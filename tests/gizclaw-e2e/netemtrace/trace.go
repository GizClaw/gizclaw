// Package netemtrace observes the isolated audio-netem build overlay only.
// Release builds never import this package. Payloads are identified by SHA-256;
// no identifiers are added to audio or to the production wire protocol.
package netemtrace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"time"
)

var writeMu sync.Mutex

// Record emits one bounded observation, without retaining or printing payloads.
func Record(stage string, data []byte, detail string, sequence, timestamp int64) {
	if os.Getenv("NETEM_NODE") == "" {
		return
	}
	now := time.Now().UnixNano()
	hash := ""
	if len(data) > 0 {
		digest := sha256.Sum256(data)
		hash = hex.EncodeToString(digest[:])
	}
	record, err := json.Marshal(map[string]any{
		"ns": now, "node": os.Getenv("NETEM_NODE"), "stage": stage,
		"hash": hash, "bytes": len(data), "detail": detail,
		"sequence": sequence, "timestamp": timestamp,
	})
	if err != nil {
		return
	}
	line := append([]byte("NETEM_TRACE "), record...)
	line = append(line, '\n')
	// Only output serialization is locked; no transport state is held here.
	writeMu.Lock()
	defer writeMu.Unlock()
	_, _ = os.Stderr.Write(line)
}
