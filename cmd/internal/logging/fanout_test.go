package logging

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestFanoutHandlerForwardsEnabledChildren(t *testing.T) {
	infoState := &recordingState{}
	warnState := &recordingState{}
	info := &recordingHandler{min: slog.LevelInfo, state: infoState}
	warn := &recordingHandler{min: slog.LevelWarn, state: warnState}
	handler := NewFanoutHandler(info, warn).WithAttrs([]slog.Attr{slog.String("service", "server")}).WithGroup("request")

	if !handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("fanout should be enabled when any child accepts the level")
	}
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "served", 0)
	record.AddAttrs(slog.Int("status", 200))
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(infoState.records) != 1 {
		t.Fatalf("info records = %d, want 1", len(infoState.records))
	}
	if len(warnState.records) != 0 {
		t.Fatalf("warn records = %d, want 0", len(warnState.records))
	}
	if got := infoState.attrs["service"]; got != "server" {
		t.Fatalf("attr service = %q", got)
	}
	if got := infoState.attrs["request.status"]; got != "200" {
		t.Fatalf("group attr status = %q", got)
	}
}

func TestFanoutHandlerJoinsErrors(t *testing.T) {
	errA := errors.New("a")
	errB := errors.New("b")
	handler := NewFanoutHandler(&recordingHandler{err: errA}, &recordingHandler{err: errB})
	err := handler.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0))
	if !errors.Is(err, errA) || !errors.Is(err, errB) {
		t.Fatalf("Handle() err = %v, want joined errors", err)
	}
}

type recordingHandler struct {
	min    slog.Level
	state  *recordingState
	groups []string
	err    error
}

type recordingState struct {
	attrs   map[string]string
	records []slog.Record
}

func (h *recordingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.min
}

func (h *recordingHandler) Handle(_ context.Context, record slog.Record) error {
	if h.state == nil {
		h.state = &recordingState{}
	}
	if h.state.attrs == nil {
		h.state.attrs = map[string]string{}
	}
	record.Attrs(func(attr slog.Attr) bool {
		h.state.attrs[joinTestKey(h.groups, attr.Key)] = attr.Value.String()
		return true
	})
	h.state.records = append(h.state.records, record.Clone())
	return h.err
}

func (h *recordingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	if next.state == nil {
		next.state = &recordingState{}
	}
	if next.state.attrs == nil {
		next.state.attrs = map[string]string{}
	}
	for _, attr := range attrs {
		next.state.attrs[joinTestKey(next.groups, attr.Key)] = attr.Value.String()
	}
	return &next
}

func (h *recordingHandler) WithGroup(name string) slog.Handler {
	next := *h
	next.groups = append(append([]string(nil), h.groups...), name)
	return &next
}

func joinTestKey(groups []string, key string) string {
	out := ""
	for _, group := range groups {
		if out != "" {
			out += "."
		}
		out += group
	}
	if out != "" {
		out += "."
	}
	return out + key
}
