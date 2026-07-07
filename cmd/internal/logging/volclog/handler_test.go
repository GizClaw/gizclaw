package volclog

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/volcengine/volc-sdk-golang/service/tls/pb"
	"github.com/volcengine/volc-sdk-golang/service/tls/producer"
)

func TestHandlerSendsSlogRecord(t *testing.T) {
	fake := &fakeProducer{}
	handler := NewHandlerWithProducer(Config{
		TopicID:          "topic",
		Level:            slog.LevelDebug,
		EnableNanosecond: true,
	}, fake).WithAttrs([]slog.Attr{
		slog.String("service", "server"),
	}).WithGroup("request")

	record := slog.NewRecord(time.Date(2026, 7, 7, 1, 2, 3, 456789123, time.UTC), slog.LevelInfo, "served", 0)
	record.AddAttrs(
		slog.Int("status", 200),
		slog.Group("http", slog.String("method", "GET")),
		slog.Any("error", errors.New("boom")),
	)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatal(err)
	}

	if !fake.started {
		t.Fatal("producer was not started")
	}
	if fake.topic != "topic" || fake.source != "gizclaw" || fake.filename != "slog" || fake.shardHash != "" {
		t.Fatalf("send target = topic=%q source=%q filename=%q shard=%q", fake.topic, fake.source, fake.filename, fake.shardHash)
	}
	if fake.log.Time != record.Time.UnixMilli() {
		t.Fatalf("log time = %d, want %d", fake.log.Time, record.Time.UnixMilli())
	}
	if got := fake.log.GetTimeNs(); got != 789123 {
		t.Fatalf("log time ns = %d", got)
	}
	contents := logContents(fake.log)
	for key, want := range map[string]string{
		"level":               "INFO",
		"msg":                 "served",
		"service":             "server",
		"request.status":      "200",
		"request.http.method": "GET",
		"request.error":       "boom",
	} {
		if contents[key] != want {
			t.Fatalf("content[%s] = %q, want %q; all=%v", key, contents[key], want, contents)
		}
	}
}

func TestHandlerEnabledAndClose(t *testing.T) {
	fake := &fakeProducer{}
	handler := NewHandlerWithProducer(Config{TopicID: "topic", Level: slog.LevelWarn}, fake)
	if handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("info should be disabled")
	}
	if !handler.Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("warn should be enabled")
	}
	if err := handler.Close(); err != nil {
		t.Fatal(err)
	}
	if !fake.closed {
		t.Fatal("producer was not closed")
	}
	handler.ForceClose()
	if !fake.forceClosed {
		t.Fatal("producer was not force closed")
	}
}

func TestNewHandlerBuildsAccessKeyProducerConfig(t *testing.T) {
	var got *producer.Config
	old := newProducer
	newProducer = func(cfg *producer.Config) Producer {
		got = cfg
		return &fakeProducer{}
	}
	t.Cleanup(func() { newProducer = old })

	handler, err := NewHandler(Config{
		Endpoint:        "https://tls-cn-beijing.volces.com",
		Region:          "cn-beijing",
		TopicID:         "topic",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	t.Cleanup(func() { _ = handler.Close() })
	if got == nil {
		t.Fatal("producer config was not captured")
	}
	if got.ClientConfig.Endpoint != "https://tls-cn-beijing.volces.com" || got.ClientConfig.Region != "cn-beijing" || got.ClientConfig.AccessKeyID != "ak" || got.ClientConfig.AccessKeySecret != "sk" {
		t.Fatalf("client config = %+v", got.ClientConfig)
	}
}

func TestNewHandlerValidatesRequiredConfig(t *testing.T) {
	base := Config{
		Endpoint:        "https://tls-cn-beijing.volces.com",
		Region:          "cn-beijing",
		TopicID:         "topic",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
	}
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{name: "endpoint", edit: func(c *Config) { c.Endpoint = "" }, want: "endpoint"},
		{name: "region", edit: func(c *Config) { c.Region = "" }, want: "region"},
		{name: "topic", edit: func(c *Config) { c.TopicID = "" }, want: "topic"},
		{name: "access key id", edit: func(c *Config) { c.AccessKeyID = "" }, want: "access key id"},
		{name: "access key secret", edit: func(c *Config) { c.AccessKeySecret = "" }, want: "access key secret"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.edit(&cfg)
			if _, err := NewHandler(cfg); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewHandler() err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestHandlerPropagatesProducerError(t *testing.T) {
	want := errors.New("send failed")
	handler := NewHandlerWithProducer(Config{TopicID: "topic"}, &fakeProducer{err: want})
	err := handler.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0))
	if !errors.Is(err, want) {
		t.Fatalf("Handle err = %v, want %v", err, want)
	}
}

func TestNilHandlerMethods(t *testing.T) {
	var handler *Handler
	if handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("nil handler should be disabled")
	}
	if err := handler.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)); err != nil {
		t.Fatal(err)
	}
	if got := handler.WithAttrs([]slog.Attr{slog.String("x", "y")}); got == nil {
		t.Fatal("nil WithAttrs should return a typed nil handler")
	}
	if got := handler.WithGroup("request"); got == nil {
		t.Fatal("nil WithGroup should return a typed nil handler")
	}
	if err := handler.Close(); err != nil {
		t.Fatal(err)
	}
	handler.ForceClose()
}

func TestHandlerEmptyGroupReturnsSameHandler(t *testing.T) {
	handler := NewHandlerWithProducer(Config{TopicID: "topic"}, &fakeProducer{})
	if got := handler.WithGroup("   "); got != handler {
		t.Fatal("empty group should return the original handler")
	}
}

func TestValueString(t *testing.T) {
	type sample struct {
		Name string `json:"name"`
	}
	tests := map[string]slog.Value{
		"string":   slog.StringValue("hello"),
		"bool":     slog.BoolValue(true),
		"duration": slog.DurationValue(2 * time.Second),
		"float":    slog.Float64Value(1.25),
		"int":      slog.Int64Value(-7),
		"time":     slog.TimeValue(time.Date(2026, 7, 7, 1, 2, 3, 4, time.UTC)),
		"uint":     slog.Uint64Value(9),
		"json":     slog.AnyValue(sample{Name: "gizclaw"}),
		"fallback": slog.AnyValue(func() {}),
	}
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			if got := valueString(value); got == "" {
				t.Fatal("value string is empty")
			}
		})
	}
	if got := valueString(slog.IntValue(42)); got != "42" {
		t.Fatalf("int value = %q", got)
	}
	if got := valueString(slog.AnyValue(errors.New("boom"))); got != "boom" {
		t.Fatalf("error value = %q", got)
	}
	if got := valueString(slog.AnyValue(strconv.NumError{Func: "Atoi", Num: "x", Err: strconv.ErrSyntax})); !strings.Contains(got, "Atoi") {
		t.Fatalf("fallback value = %q", got)
	}
}

type fakeProducer struct {
	mu          sync.Mutex
	started     bool
	closed      bool
	forceClosed bool
	shardHash   string
	topic       string
	source      string
	filename    string
	log         *pb.Log
	err         error
}

func (p *fakeProducer) SendLog(shardHash, topic, source, filename string, log *pb.Log, _ producer.CallBack) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shardHash = shardHash
	p.topic = topic
	p.source = source
	p.filename = filename
	p.log = log
	return p.err
}

func (p *fakeProducer) Start() {
	p.started = true
}

func (p *fakeProducer) Close() {
	p.closed = true
}

func (p *fakeProducer) ForceClose() {
	p.forceClosed = true
}

func logContents(log *pb.Log) map[string]string {
	out := map[string]string{}
	for _, content := range log.GetContents() {
		out[content.Key] = content.Value
	}
	return out
}
