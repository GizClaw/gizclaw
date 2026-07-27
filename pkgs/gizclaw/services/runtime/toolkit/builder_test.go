package toolkit

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestBuilderSelectsCanonicalNamesAndAppliesPolicy(t *testing.T) {
	t.Parallel()
	server := &Server{Store: kv.NewMemory(nil)}
	for _, tool := range []Tool{testClientTool("volume_set"), testHTTPTool("get_weather")} {
		if _, err := server.PutTool(context.Background(), tool); err != nil {
			t.Fatalf("PutTool(%q): %v", tool.Name, err)
		}
	}
	kit, err := (&Builder{Tools: server}).Build(context.Background(), BuildRequest{
		ProfileTools:  []string{"get_weather", "volume_set", "get_weather"},
		AllowedTools:  []string{"volume_set"},
		RestrictTools: true,
	})
	if err != nil {
		t.Fatalf("Build(): %v", err)
	}
	if len(kit.Tools) != 1 || kit.Tools[0].Name != "volume_set" {
		t.Fatalf("Build() tools = %#v", kit.Tools)
	}
	if _, ok := kit.Find("get_weather"); ok {
		t.Fatal("policy-excluded Tool was returned")
	}
}

func TestBuilderSkipsDisabledAndRejectsDanglingTools(t *testing.T) {
	t.Parallel()
	server := &Server{Store: kv.NewMemory(nil)}
	disabled := testClientTool("volume_set")
	disabled.Enabled = false
	if _, err := server.PutTool(context.Background(), disabled); err != nil {
		t.Fatalf("PutTool(): %v", err)
	}
	kit, err := (&Builder{Tools: server}).Build(context.Background(), BuildRequest{
		ProfileTools: []string{"volume_set"},
	})
	if err != nil {
		t.Fatalf("Build(): %v", err)
	}
	if len(kit.Tools) != 0 {
		t.Fatalf("Build() returned disabled tools: %#v", kit.Tools)
	}
	if _, err := (&Builder{Tools: server}).Build(context.Background(), BuildRequest{
		ProfileTools: []string{"does_not_exist"},
	}); err == nil {
		t.Fatal("Build() accepted a dangling RuntimeProfile Tool binding")
	}
}

func TestBuilderReturnsDefensiveSnapshots(t *testing.T) {
	t.Parallel()
	server := &Server{Store: kv.NewMemory(nil)}
	tool := testClientTool("volume_set")
	tool.Metadata = []byte(`{"category":"device"}`)
	if _, err := server.PutTool(context.Background(), tool); err != nil {
		t.Fatalf("PutTool(): %v", err)
	}
	builder := &Builder{Tools: server}
	first, err := builder.Build(context.Background(), BuildRequest{ProfileTools: []string{"volume_set"}})
	if err != nil {
		t.Fatalf("Build(): %v", err)
	}
	first.Tools[0].Metadata[0] = '['
	second, err := builder.Build(context.Background(), BuildRequest{ProfileTools: []string{"volume_set"}})
	if err != nil {
		t.Fatalf("Build() second: %v", err)
	}
	if string(second.Tools[0].Metadata) != `{"category":"device"}` {
		t.Fatalf("stored metadata mutated: %s", second.Tools[0].Metadata)
	}
}
