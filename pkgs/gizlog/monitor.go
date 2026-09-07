package gizlog

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// MonitorEntry is a bounded recent process log, not a firmware log. Fields
// carries the record's structured attributes so one request can be followed
// across records by request_id, peer key, stream identifier and the rest.
type MonitorEntry struct {
	ID            uint64            `json:"id"`
	Time          time.Time         `json:"time"`
	Level         string            `json:"level"`
	Error         string            `json:"error,omitempty"`
	Message       string            `json:"message"`
	PeerPublicKey string            `json:"peer_public_key,omitempty"`
	Fields        map[string]string `json:"fields,omitempty"`
}

// Structured attributes are bounded per record so a chatty caller cannot grow
// the ring beyond its memory budget.
const (
	monitorMaxFields   = 24
	monitorMaxFieldKey = 64
	monitorMaxFieldLen = 512
)

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
	// Identity is taken only from the trusted logging context, never caller
	// attributes: a caller-supplied peer_public_key stays an ordinary field.
	read := func(a slog.Attr) {
		if a.Key == "error" {
			entry.Error = a.Value.Resolve().String()
			if len(entry.Error) > 4096 {
				entry.Error = entry.Error[:4096]
			}
			return
		}
		if a.Key == "" || len(a.Key) > monitorMaxFieldKey {
			return
		}
		value := a.Value.Resolve().String()
		if value == "" {
			return
		}
		if len(value) > monitorMaxFieldLen {
			value = value[:monitorMaxFieldLen]
		}
		if entry.Fields == nil {
			entry.Fields = make(map[string]string, 8)
		}
		if _, ok := entry.Fields[a.Key]; !ok && len(entry.Fields) >= monitorMaxFields {
			return
		}
		entry.Fields[a.Key] = strings.ToValidUTF8(value, "\uFFFD")
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
	result := make([]MonitorEntry, 0, 500)
	start := uint64(1)
	if monitorLogs.sequence > 500 {
		start = monitorLogs.sequence - 499
	}
	for id := start; id <= monitorLogs.sequence; id++ {
		e := monitorLogs.entries[(id-1)%500]
		if peer == "" || e.PeerPublicKey == peer {
			result = append(result, e)
		}
	}
	monitorLogs.Unlock()
	// A retained record's field map is never written after publication, so the
	// defensive copy that keeps callers from editing it happens outside the
	// critical section.
	for i := range result {
		result[i].Message = strings.ToValidUTF8(result[i].Message, "�")
		if len(result[i].Fields) == 0 {
			continue
		}
		fields := make(map[string]string, len(result[i].Fields))
		for key, value := range result[i].Fields {
			fields[key] = value
		}
		result[i].Fields = fields
	}
	return result
}
