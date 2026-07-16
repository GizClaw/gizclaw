package logstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// SlogHandler projects structured slog records into an Appender.
type SlogHandler struct {
	appender Appender
	stream   string
	kind     string
	level    slog.Leveler
	attrs    []slogField
	groups   []string
}

type slogField struct {
	key   string
	value string
}

// NewSlogHandler creates a synchronous slog adapter for one fixed stream and kind.
func NewSlogHandler(appender Appender, stream, kind string, level slog.Leveler) (*SlogHandler, error) {
	if appender == nil {
		return nil, fmt.Errorf("logstore: slog appender is nil")
	}
	if strings.TrimSpace(stream) == "" || strings.TrimSpace(kind) == "" {
		return nil, fmt.Errorf("logstore: slog stream and kind are required")
	}
	if level == nil {
		level = slog.LevelInfo
	}
	return &SlogHandler{appender: appender, stream: stream, kind: kind, level: level}, nil
}

// Enabled reports whether level passes the adapter threshold.
func (h *SlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h != nil && h.appender != nil && level >= h.level.Level()
}

// Handle appends one projected record synchronously.
func (h *SlogHandler) Handle(ctx context.Context, record slog.Record) error {
	if h == nil || h.appender == nil {
		return nil
	}
	id, err := randomRecordID()
	if err != nil {
		return fmt.Errorf("logstore: create slog record id: %w", err)
	}
	observedAt := record.Time
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	attributes := make(map[string]string, len(h.attrs)+record.NumAttrs()+2)
	for _, attr := range h.attrs {
		setSlogAttribute(attributes, attr.key, attr.value)
	}
	record.Attrs(func(attr slog.Attr) bool {
		appendSlogAttr(attributes, h.groups, attr)
		return true
	})
	setSlogAttribute(attributes, "source", "gizclaw")
	setSlogAttribute(attributes, "path", "slog")
	item := Record{
		ID:         id,
		Time:       observedAt.UTC(),
		Stream:     h.stream,
		Kind:       h.kind,
		Severity:   record.Level.String(),
		Message:    record.Message,
		Attributes: attributes,
	}
	if err := ValidateRecord(item); err != nil {
		return err
	}
	return h.appender.Append(ctx, []Record{item})
}

// WithAttrs returns an adapter with additional handler-owned attributes.
func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if h == nil {
		return (*SlogHandler)(nil)
	}
	next := *h
	next.attrs = append([]slogField(nil), h.attrs...)
	for _, attr := range attrs {
		next.attrs = appendSlogFields(next.attrs, h.groups, attr)
	}
	next.groups = append([]string(nil), h.groups...)
	return &next
}

// WithGroup returns an adapter with an additional group prefix.
func (h *SlogHandler) WithGroup(name string) slog.Handler {
	if h == nil {
		return (*SlogHandler)(nil)
	}
	if strings.TrimSpace(name) == "" {
		return h
	}
	next := *h
	next.attrs = append([]slogField(nil), h.attrs...)
	next.groups = append(append([]string(nil), h.groups...), name)
	return &next
}

func randomRecordID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func appendSlogAttr(attributes map[string]string, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	if attr.Value.Kind() == slog.KindGroup {
		next := groups
		if name := strings.TrimSpace(attr.Key); name != "" {
			next = append(append([]string(nil), groups...), name)
		}
		for _, child := range attr.Value.Group() {
			appendSlogAttr(attributes, next, child)
		}
		return
	}
	key := slogAttributeKey(groups, attr.Key)
	if key == "" {
		return
	}
	setSlogAttribute(attributes, key, slogValueString(attr.Value))
}

func appendSlogFields(fields []slogField, groups []string, attr slog.Attr) []slogField {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return fields
	}
	if attr.Value.Kind() == slog.KindGroup {
		next := groups
		if name := strings.TrimSpace(attr.Key); name != "" {
			next = append(append([]string(nil), groups...), name)
		}
		for _, child := range attr.Value.Group() {
			fields = appendSlogFields(fields, next, child)
		}
		return fields
	}
	key := slogAttributeKey(groups, attr.Key)
	if key == "" {
		return fields
	}
	return append(fields, slogField{key: key, value: slogValueString(attr.Value)})
}

func slogAttributeKey(groups []string, key string) string {
	parts := make([]string, 0, len(groups)+1)
	for _, group := range groups {
		if group = strings.TrimSpace(group); group != "" {
			parts = append(parts, group)
		}
	}
	if key = strings.TrimSpace(key); key != "" {
		parts = append(parts, key)
	}
	return strings.Join(parts, ".")
}

func setSlogAttribute(attributes map[string]string, key, value string) {
	for existing := range attributes {
		if existing == key || strings.HasPrefix(existing, key+".") || strings.HasPrefix(key, existing+".") {
			delete(attributes, existing)
		}
	}
	attributes[key] = value
}

func slogValueString(value slog.Value) string {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindBool:
		return strconv.FormatBool(value.Bool())
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindFloat64:
		return strconv.FormatFloat(value.Float64(), 'g', -1, 64)
	case slog.KindInt64:
		return strconv.FormatInt(value.Int64(), 10)
	case slog.KindTime:
		return value.Time().Format(time.RFC3339Nano)
	case slog.KindUint64:
		return strconv.FormatUint(value.Uint64(), 10)
	case slog.KindGroup:
		return ""
	case slog.KindAny:
		if err, ok := value.Any().(error); ok {
			return err.Error()
		}
		if data, err := json.Marshal(value.Any()); err == nil {
			return string(data)
		}
		return fmt.Sprint(value.Any())
	case slog.KindLogValuer:
		return slogValueString(value.Resolve())
	default:
		return value.String()
	}
}
