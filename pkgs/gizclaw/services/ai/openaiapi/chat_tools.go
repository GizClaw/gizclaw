package openaiapi

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// chatFunctionName is the OpenAI Chat Completions function name contract.
var chatFunctionName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// chatTools converts client-declared function tools. The caller executes
// every call it receives; GizClaw only forwards declarations and calls.
func chatTools(body *chatCompletionRequest) ([]*genx.FuncTool, error) {
	if body.ToolChoice != nil && string(body.ToolChoice) != `"auto"` {
		return nil, invalid("unsupported_option", "tool_choice", "Only the auto tool_choice is supported by GizClaw.")
	}
	switch string(body.ParallelToolCalls) {
	case "", "true":
	case "false":
		return nil, invalid("unsupported_option", "parallel_tool_calls", "Disabling parallel tool calls is not supported by GizClaw.")
	default:
		return nil, invalid("invalid_request", "parallel_tool_calls", "parallel_tool_calls must be a boolean.")
	}
	tools := make([]*genx.FuncTool, 0, len(body.Tools))
	names := make(map[string]struct{}, len(body.Tools))
	for _, declaration := range body.Tools {
		tool, err := chatTool(declaration)
		if err != nil {
			return nil, err
		}
		if _, ok := names[tool.Name]; ok {
			return nil, invalid("invalid_tools", "tools", "Tool function names must be unique.")
		}
		names[tool.Name] = struct{}{}
		tools = append(tools, tool)
	}
	return tools, nil
}

func chatTool(declaration map[string]any) (*genx.FuncTool, error) {
	if err := rejectUnknownFields(declaration, "tools", "type", "function"); err != nil {
		return nil, err
	}
	if declaration["type"] != "function" {
		return nil, invalid("unsupported_option", "tools", "Only function tools are supported by GizClaw.")
	}
	function, ok := declaration["function"].(map[string]any)
	if !ok {
		return nil, invalid("invalid_tools", "tools", "A function tool requires a function object.")
	}
	if err := rejectUnknownFields(function, "tools", "name", "description", "parameters", "strict"); err != nil {
		return nil, err
	}
	name, _ := function["name"].(string)
	if !chatFunctionName.MatchString(name) {
		return nil, invalid("invalid_tools", "tools", "A tool function name must match ^[A-Za-z0-9_-]{1,64}$.")
	}
	tool := &genx.FuncTool{Name: name}
	if value, ok := function["description"]; ok {
		if tool.Description, ok = value.(string); !ok {
			return nil, invalid("invalid_tools", "tools", "A tool description must be a string.")
		}
	}
	if value, ok := function["parameters"]; ok {
		schema, ok := value.(map[string]any)
		if !ok {
			return nil, invalid("invalid_tools", "tools", "Tool parameters must be a JSON Schema object.")
		}
		encoded, err := json.Marshal(schema)
		if err != nil {
			return nil, invalid("invalid_tools", "tools", "Tool parameters must be a JSON Schema object.")
		}
		tool.Parameters = encoded
	}
	if value, ok := function["strict"]; ok {
		if tool.Strict, ok = value.(bool); !ok {
			return nil, invalid("invalid_tools", "tools", "Tool strict must be a boolean.")
		}
	}
	return tool, nil
}

// chatStreamOptions validates stream_options and reports include_usage.
func chatStreamOptions(raw json.RawMessage, streaming bool) (bool, error) {
	if raw == nil {
		return false, nil
	}
	if !streaming {
		return false, invalid("invalid_stream_options", "stream_options", "stream_options requires stream to be true.")
	}
	var options map[string]any
	if err := json.Unmarshal(raw, &options); err != nil || options == nil {
		return false, invalid("invalid_stream_options", "stream_options", "stream_options must be an object.")
	}
	if err := rejectUnknownFields(options, "stream_options", "include_usage"); err != nil {
		return false, err
	}
	value, ok := options["include_usage"]
	if !ok {
		return false, nil
	}
	includeUsage, ok := value.(bool)
	if !ok {
		return false, invalid("invalid_stream_options", "stream_options", "include_usage must be a boolean.")
	}
	return includeUsage, nil
}

func parseAssistantToolCalls(value any) ([]*genx.ToolCall, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, invalid("invalid_messages", "messages.tool_calls", "tool_calls must be an array.")
	}
	calls := make([]*genx.ToolCall, 0, len(items))
	for _, item := range items {
		call, ok := item.(map[string]any)
		if !ok {
			return nil, invalid("invalid_messages", "messages.tool_calls", "A tool call must be an object.")
		}
		if err := rejectUnknownFields(call, "messages.tool_calls", "id", "type", "function"); err != nil {
			return nil, err
		}
		function, _ := call["function"].(map[string]any)
		if err := rejectUnknownFields(function, "messages.tool_calls", "name", "arguments"); err != nil {
			return nil, err
		}
		id, _ := call["id"].(string)
		name, _ := function["name"].(string)
		arguments, argumentsOK := function["arguments"].(string)
		if call["type"] != "function" || strings.TrimSpace(id) == "" || name == "" || !argumentsOK {
			return nil, invalid("invalid_messages", "messages.tool_calls", "A tool call requires id, type function, function.name, and function.arguments.")
		}
		calls = append(calls, &genx.ToolCall{ID: id, FuncCall: &genx.FuncCall{Name: name, Arguments: arguments}})
	}
	return calls, nil
}

// chatToolCall renders one GenX call as a Chat Completions tool call.
func chatToolCall(call *genx.ToolCall) map[string]any {
	return map[string]any{
		"id": call.ID, "type": "function",
		"function": map[string]any{"name": call.FuncCall.Name, "arguments": call.FuncCall.Arguments},
	}
}

type chatOutput struct {
	text      string
	toolCalls []map[string]any
}

func readChatStream(stream genx.Stream) (chatOutput, error) {
	defer stream.Close()
	var (
		text   strings.Builder
		output chatOutput
	)
	for {
		chunk, err := stream.Next()
		if streamDone(err) {
			output.text = text.String()
			return output, nil
		}
		if err != nil {
			return chatOutput{}, err
		}
		if chunk == nil {
			continue
		}
		if call := chunk.ToolCall; call != nil && call.FuncCall != nil {
			output.toolCalls = append(output.toolCalls, chatToolCall(call))
		}
		if chunk.IsEndOfStream() {
			continue
		}
		if value, ok := chunk.Part.(genx.Text); ok {
			text.WriteString(string(value))
		}
	}
}

// chatUsage returns the upstream usage carried by a completed stream, if any.
func chatUsage(err error) (map[string]any, bool) {
	state, ok := errors.AsType[*genx.State](err)
	if !ok {
		return nil, false
	}
	usage := state.Usage()
	if usage.PromptTokenCount == 0 && usage.GeneratedTokenCount == 0 {
		return nil, false
	}
	return map[string]any{
		"prompt_tokens":     usage.PromptTokenCount,
		"completion_tokens": usage.GeneratedTokenCount,
		"total_tokens":      usage.PromptTokenCount + usage.GeneratedTokenCount,
	}, true
}
