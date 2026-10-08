package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestRuntimeToolSystemPromptDoesNotInterpolateHistory(t *testing.T) {
	data, err := os.ReadFile("../../testdata/runtime-tools/workflow.json")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Eino struct {
			Graph struct {
				Nodes []struct {
					Type     string `json:"type"`
					Messages []struct {
						Role     string `json:"role"`
						Template string `json:"template"`
					} `json:"messages"`
				} `json:"nodes"`
			} `json:"graph"`
		} `json:"eino"`
	}
	if err := json.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, node := range workflow.Eino.Graph.Nodes {
		if node.Type != "prompt" {
			continue
		}
		for _, entry := range node.Messages {
			if entry.Role != "system" {
				continue
			}
			checked++
			formatted, err := schema.SystemMessage(entry.Template).Format(t.Context(), map[string]any{
				"history": []*schema.Message{schema.UserMessage("untrusted-history-marker")},
			}, schema.FString)
			if err != nil {
				t.Fatal(err)
			}
			if len(formatted) != 1 || formatted[0].Content != entry.Template || strings.Contains(formatted[0].Content, "untrusted-history-marker") {
				t.Fatal("static business rules interpolated user history")
			}
		}
	}
	if checked == 0 {
		t.Fatal("no system business rules were checked")
	}
}
