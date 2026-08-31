package flowcraft

import (
	"reflect"
	"strings"
	"testing"

	memoryhook "github.com/GizClaw/flowcraft/core/memory/hook"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestNormalizeConfigRejectsAdversarialContracts(t *testing.T) {
	t.Parallel()
	valid := testConfig(&echoGenerator{})
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name: "negative iteration limit",
			mutate: func(config *Config) {
				config.MaxIterations = -1
			},
			wantErr: "cannot be negative",
		},
		{
			name: "negative tool call limit",
			mutate: func(config *Config) {
				config.MaxToolCalls = -1
			},
			wantErr: "MaxToolCalls cannot be negative",
		},
		{
			name: "tool call limit without ToolInvoker",
			mutate: func(config *Config) {
				config.MaxToolCalls = 1
			},
			wantErr: "MaxToolCalls requires ToolInvoker",
		},
		{
			name: "unknown initiative",
			mutate: func(config *Config) {
				config.Initiative = "whenever"
			},
			wantErr: "unsupported Initiative",
		},
		{
			name: "empty model alias",
			mutate: func(config *Config) {
				config.Graph.Nodes[0].Config = testInferenceConfig(" ")
			},
			wantErr: "requires model.id",
		},
		{
			name: "empty script",
			mutate: func(config *Config) {
				config.Graph.Nodes[0].Type = "script"
				config.Graph.Nodes[0].Config = testNodeConfig(map[string]any{"runtime": "js", "source": " "})
			},
			wantErr: "requires inline source",
		},
		{
			name: "configured passthrough",
			mutate: func(config *Config) {
				config.Graph.Nodes[0].Type = "passthrough"
				config.Graph.Nodes[0].Config = testNodeConfig(map[string]any{"unexpected": true})
			},
			wantErr: "does not accept config",
		},
		{
			name: "memory setting without store",
			mutate: func(config *Config) {
				config.MemoryScope = memory.Scope{AppID: "app"}
			},
			wantErr: "settings require Memory",
		},
		{
			name: "memory without scope",
			mutate: func(config *Config) {
				config.Memory = &waitingMemoryStore{}
			},
			wantErr: "MemoryScope is required",
		},
		{
			name: "unsupported recent memory hook",
			mutate: func(config *Config) {
				config.Memory = &waitingMemoryStore{}
				config.MemoryScope = memory.Scope{AppID: "app"}
				config.MemoryContext = &memoryhook.ContextSettings{
					Query: memoryhook.QuerySettings{RecentOnly: true}, Output: "memory_items",
				}
			},
			wantErr: "recent_only is not supported",
		},
		{
			name: "unsupported memory datasets",
			mutate: func(config *Config) {
				config.Memory = &waitingMemoryStore{}
				config.MemoryScope = memory.Scope{AppID: "app"}
				config.MemoryContext = &memoryhook.ContextSettings{
					Query: memoryhook.QuerySettings{CurrentMessage: true}, DatasetIDs: []string{"docs"}, Output: "memory_items",
				}
			},
			wantErr: "dataset_ids are not supported",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			config.Graph = cloneTestGraph(valid.Graph)
			test.mutate(&config)
			if _, err := normalizeConfig(config); err == nil ||
				!strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("normalizeConfig() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}

	config := valid
	config.Graph = cloneTestGraph(valid.Graph)
	config.PublishNodes = []string{" chat ", "chat"}
	normalized, err := normalizeConfig(config)
	if err != nil {
		t.Fatalf("normalizeConfig(duplicate publisher) error = %v", err)
	}
	if !reflect.DeepEqual(normalized.PublishNodes, []string{"chat"}) {
		t.Fatalf("normalized PublishNodes = %#v", normalized.PublishNodes)
	}
}

func TestCloneConfigValueOwnsEveryCompositeShape(t *testing.T) {
	t.Parallel()
	if cloned, err := cloneConfigValue(nil); err != nil || cloned != nil {
		t.Fatalf("cloneConfigValue(nil) = %#v, %v", cloned, err)
	}
	if _, err := cloneConfigValue(make(chan int)); err == nil {
		t.Fatal("cloneConfigValue(channel) succeeded")
	}
	number := 7
	source := map[string]any{
		"pointer":  &number,
		"slice":    []any{map[string]int{"nested": 1}},
		"array":    [2]string{"left", "right"},
		"nilMap":   map[string]int(nil),
		"nilSlice": []string(nil),
		"filter": memory.Filter{
			Field: "kind", Operator: memory.FilterEqual, Value: []string{"one"},
		},
	}
	clonedValue, err := cloneConfigValue(source)
	if err != nil {
		t.Fatalf("cloneConfigValue() error = %v", err)
	}
	cloned := clonedValue.(map[string]any)
	*cloned["pointer"].(*int) = 9
	cloned["slice"].([]any)[0].(map[string]int)["nested"] = 2
	if number != 7 || source["slice"].([]any)[0].(map[string]int)["nested"] != 1 {
		t.Fatalf("clone mutated source: %#v", source)
	}
	if cloned["nilMap"].(map[string]int) != nil || cloned["nilSlice"].([]string) != nil {
		t.Fatalf("typed nil values changed: %#v", cloned)
	}

	var interfaceValue any = []string{"owned"}
	clonedReflect, err := cloneConfigReflect(reflect.ValueOf(&interfaceValue).Elem())
	if err != nil {
		t.Fatalf("cloneConfigReflect(interface) error = %v", err)
	}
	clonedReflect.Interface().([]string)[0] = "changed"
	if interfaceValue.([]string)[0] != "owned" {
		t.Fatal("interface clone leaked mutation")
	}

	for _, unsupported := range []any{make(chan int), func() {}} {
		value := reflect.ValueOf(unsupported)
		if _, err := cloneConfigReflect(value); err == nil {
			t.Fatalf("cloneConfigReflect(%s) succeeded", value.Kind())
		}
	}
}

type memoryOnlyStore struct {
	memory.Store
}
