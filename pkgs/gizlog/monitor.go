package gizlog

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// MonitorEntry is a bounded recent process log, not a firmware log.
type MonitorEntry struct {
	ID            uint64    `json:"id"`
	Time          time.Time `json:"time"`
	Level         string    `json:"level"`
	Error         string    `json:"error,omitempty"`
	Message       string    `json:"message"`
	PeerPublicKey string    `json:"peer_public_key,omitempty"`
}

var monitorLogs struct {
	sync.Mutex
	sequence uint64
	entries  [500]MonitorEntry
}

type monitorHandler struct {
	attrs []slog.Attr
	level slog.Level
}

func (h *monitorHandler) Enabled(_ context.Context, level slog.Level) bool { return level >= h.level }
func (h *monitorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	copyH := *h
	copyH.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &copyH
}
func (h *monitorHandler) WithGroup(_ string) slog.Handler { return h }
func (h *monitorHandler) Handle(ctx context.Context, r slog.Record) error {
	entry := MonitorEntry{Time: r.Time, Level: r.Level.String(), Message: r.Message, PeerPublicKey: PeerPublicKey(ctx)}
	if len(entry.Message) > 4096 {
		entry.Message = entry.Message[:4096]
	}
	// Identity is taken only from the trusted logging context, never caller attributes.
	read := func(a slog.Attr) {
		if a.Key == "error" {
			entry.Error = a.Value.Resolve().String()
			if len(entry.Error) > 4096 {
				entry.Error = entry.Error[:4096]
			}
		}
	}
	for _, a := range h.attrs {
		read(a)
	}
	r.Attrs(func(a slog.Attr) bool { read(a); return true })

	monitorLogs.Lock()
	monitorLogs.sequence++
	entry.ID = monitorLogs.sequence
	monitorLogs.entries[(entry.ID-1)%500] = entry
	monitorLogs.Unlock()
	return nil
}

// ReadMonitorLogs returns at most 500 recent records, optionally scoped to a peer.
func ReadMonitorLogs(peer string) []MonitorEntry {
	monitorLogs.Lock()
	defer monitorLogs.Unlock()
	result := make([]MonitorEntry, 0, 500)
	start := uint64(1)
	if monitorLogs.sequence > 500 {
		start = monitorLogs.sequence - 499
	}
	for id := start; id <= monitorLogs.sequence; id++ {
		e := monitorLogs.entries[(id-1)%500]
		if peer == "" || e.PeerPublicKey == peer {
			e.Message = strings.ToValidUTF8(e.Message, "�")
			result = append(result, e)
		}
	}
	return result
}
