package logging

import (
	"reflect"
	"testing"
)

func TestPrepareConfigDefaults(t *testing.T) {
	got, err := PrepareConfig(Config{})
	if err != nil {
		t.Fatalf("PrepareConfig() error = %v", err)
	}
	if got.Level != "info" || len(got.Sinks) != 1 || got.Sinks[0].Kind != SinkStderr || got.Sinks[0].Level != "info" {
		t.Fatalf("default config = %+v", got)
	}
}

func TestPrepareConfigSinks(t *testing.T) {
	got, err := PrepareConfig(Config{
		Level:      "debug",
		QueryStore: "logs",
		Sinks: []SinkConfig{
			{Kind: SinkStderr},
			{Kind: SinkStore, Store: "logs", Level: "warn"},
		},
	})
	if err != nil {
		t.Fatalf("PrepareConfig() error = %v", err)
	}
	if got.Sinks[0].Level != "debug" || got.Sinks[1].Level != "warn" {
		t.Fatalf("prepared levels = %+v", got.Sinks)
	}
}

func TestPrepareConfigRejectsInvalidShapes(t *testing.T) {
	tests := []Config{
		{Level: "verbose"},
		{Sinks: []SinkConfig{}},
		{Sinks: []SinkConfig{{Kind: "file"}}},
		{Sinks: []SinkConfig{{Kind: SinkStderr, Store: "logs"}}},
		{Sinks: []SinkConfig{{Kind: SinkStore}}},
		{Sinks: []SinkConfig{{Kind: SinkStderr}, {Kind: SinkStderr}}},
		{Sinks: []SinkConfig{{Kind: SinkStore, Store: "logs"}, {Kind: SinkStore, Store: "logs", Level: "warn"}}},
		{QueryStore: "logs", Sinks: []SinkConfig{{Kind: SinkStderr}}},
	}
	for index, cfg := range tests {
		if _, err := PrepareConfig(cfg); err == nil {
			t.Fatalf("case %d: PrepareConfig() error = nil", index)
		}
	}
}

func TestPrepareConfigExpandsEnvironment(t *testing.T) {
	t.Setenv("GIZCLAW_TEST_LOG_LEVEL", "debug")
	t.Setenv("GIZCLAW_TEST_LOG_STORE", "logs")
	got, err := PrepareConfig(Config{
		Level:      "$GIZCLAW_TEST_LOG_LEVEL",
		QueryStore: "$GIZCLAW_TEST_LOG_STORE",
		Sinks:      []SinkConfig{{Kind: SinkStore, Store: "$GIZCLAW_TEST_LOG_STORE"}},
	})
	if err != nil {
		t.Fatalf("PrepareConfig() error = %v", err)
	}
	if got.Level != "debug" || got.QueryStore != "logs" || got.Sinks[0].Store != "logs" {
		t.Fatalf("expanded config = %+v", got)
	}
}

func TestConfigSurfaceStaysMinimal(t *testing.T) {
	got := yamlFields(reflect.TypeFor[Config](), "")
	want := map[string]bool{"level": true, "query_store": true, "sinks": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("config fields = %v, want %v", got, want)
	}
	sinkFields := yamlFields(reflect.TypeFor[SinkConfig](), "")
	wantSink := map[string]bool{"kind": true, "store": true, "level": true}
	if !reflect.DeepEqual(sinkFields, wantSink) {
		t.Fatalf("sink fields = %v, want %v", sinkFields, wantSink)
	}
}

func yamlFields(typ reflect.Type, prefix string) map[string]bool {
	fields := map[string]bool{}
	for field := range typ.Fields() {
		name := field.Tag.Get("yaml")
		if comma := len(name); comma > 0 {
			for j, r := range name {
				if r == ',' {
					comma = j
					break
				}
			}
			name = name[:comma]
		}
		if name != "" && name != "-" {
			fields[name] = true
		}
	}
	return fields
}
