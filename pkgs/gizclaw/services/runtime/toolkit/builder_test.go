package toolkit

import (
	"context"
	"testing"
)

func TestBuilderResolvesCanonicalIDsAndAppliesPolicy(t *testing.T) {
	t.Parallel()
	server := &Server{DB: newTestDatabase(t)}
	toolIDs := make(map[string]string)
	for _, tool := range []Tool{testCatalogTool("volume_set"), testHTTPTool("get_weather")} {
		created, err := server.CreateTool(context.Background(), tool)
		if err != nil {
			t.Fatalf("PutTool(%q): %v", tool.InvokeName, err)
		}
		toolIDs[tool.InvokeName] = created.ID
	}
	kit, err := (&Builder{Tools: server}).Build(context.Background(), BuildRequest{
		ProfileTools: []string{toolIDs["get_weather"], toolIDs["volume_set"], toolIDs["get_weather"]},
		AllowedTools: []string{toolIDs["volume_set"], "unbound-tool"},
	})
	if err != nil {
		t.Fatalf("Build(): %v", err)
	}
	if len(kit.Tools) != 1 || kit.Tools[0].InvokeName != "volume_set" {
		t.Fatalf("Build() tools = %#v", kit.Tools)
	}
	if _, ok := kit.Find("get_weather"); ok {
		t.Fatal("policy-excluded Tool was returned")
	}
}

func TestBuilderExposesNoToolsWithoutAllowedTools(t *testing.T) {
	t.Parallel()
	server := &Server{DB: newTestDatabase(t)}
	created, err := server.CreateTool(context.Background(), testHTTPTool("get_weather"))
	if err != nil {
		t.Fatalf("PutTool(): %v", err)
	}
	for _, allowed := range [][]string{nil, {}} {
		kit, err := (&Builder{Tools: server}).Build(context.Background(), BuildRequest{
			ProfileTools: []string{created.ID},
			AllowedTools: allowed,
		})
		if err != nil {
			t.Fatalf("Build(%#v): %v", allowed, err)
		}
		if len(kit.Tools) != 0 {
			t.Fatalf("Build(%#v) inherited RuntimeProfile tools: %#v", allowed, kit.Tools)
		}
	}
}

func TestBuilderSkipsDisabledAndRejectsDanglingTools(t *testing.T) {
	t.Parallel()
	server := &Server{DB: newTestDatabase(t)}
	disabled := testCatalogTool("volume_set")
	disabled.Enabled = false
	created, err := server.CreateTool(context.Background(), disabled)
	if err != nil {
		t.Fatalf("PutTool(): %v", err)
	}
	kit, err := (&Builder{Tools: server}).Build(context.Background(), BuildRequest{
		ProfileTools: []string{created.ID},
		AllowedTools: []string{created.ID},
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
	server := &Server{DB: newTestDatabase(t)}
	tool := testCatalogTool("volume_set")
	tool.Metadata = []byte(`{"category":"device"}`)
	created, err := server.CreateTool(context.Background(), tool)
	if err != nil {
		t.Fatalf("PutTool(): %v", err)
	}
	builder := &Builder{Tools: server}
	request := BuildRequest{ProfileTools: []string{created.ID}, AllowedTools: []string{created.ID}}
	first, err := builder.Build(context.Background(), request)
	if err != nil {
		t.Fatalf("Build(): %v", err)
	}
	first.Tools[0].Metadata[0] = '['
	second, err := builder.Build(context.Background(), request)
	if err != nil {
		t.Fatalf("Build() second: %v", err)
	}
	if string(second.Tools[0].Metadata) != `{"category":"device"}` {
		t.Fatalf("stored metadata mutated: %s", second.Tools[0].Metadata)
	}
}
