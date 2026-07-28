package eino

import (
	"context"
	"fmt"

	genxmatch "github.com/GizClaw/gizclaw-go/pkgs/genx/match"
	"github.com/cloudwego/eino/schema"
)

func compileMatchNode(
	ctx context.Context,
	config *normalizedConfig,
	node NodeDefinition,
) (func(context.Context, *runState) (map[string]any, map[string]bool, error), error) {
	matcher := node.Match.matcher
	if matcher == nil {
		return nil, fmt.Errorf("eino: Match node %q was not compiled", node.ID)
	}
	component, err := config.Components.ResolveChatModel(ctx, node.Match.Model)
	if err != nil {
		return nil, fmt.Errorf("eino: resolve Match model %q: %w", node.Match.Model, err)
	}
	if component == nil {
		return nil, fmt.Errorf("eino: Match model %q resolved nil", node.Match.Model)
	}
	return func(ctx context.Context, state *runState) (map[string]any, map[string]bool, error) {
		inputs, err := state.nodeInputs(node.Inputs)
		if err != nil {
			return nil, nil, err
		}
		text, ok := inputs["text"].(string)
		if !ok {
			return nil, nil, fmt.Errorf("eino: Match input \"text\" is not string")
		}
		reader, err := component.Stream(ctx, []*schema.Message{
			schema.SystemMessage(matcher.SystemPrompt()),
			schema.UserMessage(text),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("eino: stream Match model: %w", err)
		}
		if reader == nil {
			return nil, nil, fmt.Errorf("eino: stream Match model: returned nil reader")
		}
		defer reader.Close()
		chunks := func(yield func(string, error) bool) {
			for {
				message, receiveErr := reader.Recv()
				if receiveErr != nil {
					if isStreamEnd(receiveErr) {
						return
					}
					yield("", receiveErr)
					return
				}
				if message != nil && message.Content != "" &&
					!yield(message.Content, nil) {
					return
				}
			}
		}
		matches, err := genxmatch.Collect(matcher.Parse(chunks))
		if err != nil {
			return nil, nil, fmt.Errorf("eino: parse Match model stream: %w", err)
		}
		return map[string]any{"matches": genxmatch.Project(matches)}, nil, nil
	}, nil
}
