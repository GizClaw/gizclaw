// Command scriptqualityresources prepares native text-only screenplay resources
// for the isolated quality lane, preserving their graph and memory bindings.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/goccy/go-yaml"
	"os"
	"path/filepath"
)

func main() {
	root := flag.String("resources", "tests/gizclaw-e2e/testdata/resources/04-workflows", "native workflow directory")
	out := flag.String("output", "", "output ResourceList JSON path")
	flag.Parse()
	var items []map[string]any
	for _, name := range []string{"08-eino-journey.yaml", "10-eino-multi-role-storyteller.yaml", "11-eino-murder-mystery.yaml", "12-eino-poetry-adventure-li-bai.yaml", "13-eino-werewolf.yaml"} {
		raw, err := os.ReadFile(filepath.Join(*root, name))
		if err != nil {
			fail(err)
		}
		var resource map[string]any
		if err := yaml.Unmarshal(raw, &resource); err != nil {
			fail(err)
		}
		spec, ok := resource["spec"].(map[string]any)
		if !ok {
			fail(fmt.Errorf("%s is missing spec", name))
		}
		eino, ok := spec["eino"].(map[string]any)
		if !ok || eino["graph"] == nil {
			fail(fmt.Errorf("%s is missing native Eino graph", name))
		}
		before, err := json.Marshal(eino["graph"])
		if err != nil {
			fail(err)
		}
		delete(eino, "voice_adapter")
		after, err := json.Marshal(eino["graph"])
		if err != nil {
			fail(err)
		}
		if !bytes.Equal(before, after) {
			fail(fmt.Errorf("%s graph changed", name))
		}
		encoded, err := json.Marshal(spec)
		if err != nil {
			fail(err)
		}
		var typed apitypes.WorkflowSpec
		if err := json.Unmarshal(encoded, &typed); err != nil || typed.Driver != apitypes.WorkflowDriverEino {
			fail(fmt.Errorf("%s is not a native Eino resource", name))
		}
		items = append(items, resource)
	}
	data, err := json.MarshalIndent(map[string]any{"apiVersion": "gizclaw.admin/v1alpha1", "kind": "ResourceList", "spec": map[string]any{"items": items}}, "", "  ")
	if err != nil {
		fail(err)
	}
	if *out == "" {
		fail(fmt.Errorf("output path is required"))
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0600); err != nil {
		fail(err)
	}
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
