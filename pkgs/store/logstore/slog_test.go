package logstore

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

type captureAppender struct {
	records []Record
	err     error
}

func (a *captureAppender) Append(_ context.Context, records []Record) error {
	for _, record := range records {
		a.records = append(a.records, cloneRecord(record))
	}
	return a.err
}

func TestSlogHandlerProjectionAndCollisions(t *testing.T) {
	appender := &captureAppender{}
	handler, err := NewSlogHandler(appender, "system", "log", slog.LevelWarn)
	if err != nil {
		t.Fatalf("NewSlogHandler() error = %v", err)
	}
	if handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("info should be disabled")
	}
	handler = handler.WithAttrs([]slog.Attr{slog.String("request", "scalar"), slog.String("source", "caller")}).(*SlogHandler)
	record := slog.NewRecord(time.Unix(10, 123), slog.LevelWarn, "message", 0)
	record.AddAttrs(slog.Group("request", slog.String("method", "GET")), slog.String("path", "caller"))
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	got := appender.records[0]
	if got.ID == "" || got.Stream != "system" || got.Kind != "log" || got.Severity != "WARN" || got.Message != "message" {
		t.Fatalf("record = %+v", got)
	}
	if got.Attributes["request.method"] != "GET" || got.Attributes["source"] != "gizclaw" || got.Attributes["path"] != "slog" {
		t.Fatalf("attributes = %+v", got.Attributes)
	}
	if _, exists := got.Attributes["request"]; exists {
		t.Fatal("later descendant did not remove scalar collision")
	}
}

func TestSlogHandlerPropagatesAppendError(t *testing.T) {
	want := errors.New("append")
	handler, _ := NewSlogHandler(&captureAppender{err: want}, "system", "log", slog.LevelInfo)
	if err := handler.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "x", 0)); !errors.Is(err, want) {
		t.Fatalf("Handle() error = %v, want append error", err)
	}
}

func TestSlogHandlerWithAttrsKeepsOriginalGroup(t *testing.T) {
	appender := &captureAppender{}
	handler, _ := NewSlogHandler(appender, "system", "log", slog.LevelInfo)
	handler = handler.WithAttrs([]slog.Attr{slog.String("root", "value")}).(*SlogHandler)
	handler = handler.WithGroup("request").(*SlogHandler)
	handler = handler.WithAttrs([]slog.Attr{slog.String("method", "GET")}).(*SlogHandler)
	handler = handler.WithGroup("nested").(*SlogHandler)
	record := slog.NewRecord(time.Time{}, slog.LevelInfo, "message", 0)
	record.AddAttrs(slog.String("id", "1"))
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	got := appender.records[0]
	if got.Time.IsZero() || got.Time.Location() != time.UTC {
		t.Fatalf("record time = %v", got.Time)
	}
	if got.Attributes["root"] != "value" || got.Attributes["request.method"] != "GET" || got.Attributes["request.nested.id"] != "1" {
		t.Fatalf("attributes = %+v", got.Attributes)
	}
}
