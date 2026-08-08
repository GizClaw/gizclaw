package pendingdeletion

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"math"
	"time"
)

func newPendingDeletionToken() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func retryDelay(initial, maximum time.Duration, failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	power := min(failures-1, 30)
	delay := float64(initial) * math.Pow(2, float64(power))
	if delay > float64(maximum) {
		delay = float64(maximum)
	}
	// Jitter stays in [75%, 100%] so retries remain bounded by retry_max.
	var random [8]byte
	factor := 1.0
	if _, err := rand.Read(random[:]); err == nil {
		factor = 0.75 + float64(binary.LittleEndian.Uint64(random[:])%2501)/10000
	}
	return time.Duration(delay * factor)
}
