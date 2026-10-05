package einoconfig

import (
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

// VisitModelAliases visits Model bindings in the root and nested Eino Graphs.
func VisitModelAliases(path string, graph apitypes.EinoGraph, requireModel func(string, string, apitypes.ModelKind) error) error {
	for index, raw := range graph.Nodes {
		discriminator, err := raw.Discriminator()
		if err != nil {
			return fmt.Errorf("%s.graph.nodes[%d]: %w", path, index, err)
		}
		nodePath := fmt.Sprintf("graph.nodes[%d]", index)
		switch discriminator {
		case "chat_model":
			node, err := raw.AsEinoChatModelNode()
			if err != nil {
				return fmt.Errorf("%s.%s: %w", path, nodePath, err)
			}
			if err := requireModel(nodePath+".model", node.Model, apitypes.ModelKindLlm); err != nil {
				return err
			}
		case "batch":
			node, err := raw.AsEinoBatchNode()
			if err != nil {
				return fmt.Errorf("%s.%s: %w", path, nodePath, err)
			}
			if err := VisitModelAliases(path+"."+nodePath+".graph", node.Graph, requireModel); err != nil {
				return err
			}
		case "subgraph":
			node, err := raw.AsEinoSubgraphNode()
			if err != nil {
				return fmt.Errorf("%s.%s: %w", path, nodePath, err)
			}
			if err := VisitModelAliases(path+"."+nodePath+".graph", node.Graph, requireModel); err != nil {
				return err
			}
		case "race":
			node, err := raw.AsEinoRaceNode()
			if err != nil {
				return fmt.Errorf("%s.%s: %w", path, nodePath, err)
			}
			for branchIndex, branch := range node.Branches {
				branchPath := fmt.Sprintf("%s.%s.branches[%d].graph", path, nodePath, branchIndex)
				if err := VisitModelAliases(branchPath, branch.Graph, requireModel); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
