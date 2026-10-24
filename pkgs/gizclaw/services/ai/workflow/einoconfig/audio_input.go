package einoconfig

import (
	"errors"
	"strings"

	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
)

// AudioInputModels validates the native input shape and lists every chat Model,
// including nested Graphs, whose audio capability must be checked by the host.
func AudioInputModels(graph genxeino.GraphDefinition) ([]string, error) {
	var aliases []string
	messages := false
	var visit func(genxeino.GraphDefinition)
	visit = func(graph genxeino.GraphDefinition) {
		for _, node := range graph.Nodes {
			for _, binding := range node.Inputs {
				messages = messages || strings.TrimSpace(binding.From) == "input.messages"
			}
			if node.ChatModel != nil {
				aliases = append(aliases, node.ChatModel.Model)
			}
			if node.Subgraph != nil {
				visit(node.Subgraph.Graph)
			}
			if node.Batch != nil {
				visit(node.Batch.Graph)
			}
			if node.Race != nil {
				for _, branch := range node.Race.Branches {
					visit(branch.Graph)
				}
			}
		}
	}
	visit(graph)
	if len(aliases) == 0 || !messages {
		return nil, errors.New("audio_input model requires a chat Model and an input.messages binding")
	}
	return aliases, nil
}
