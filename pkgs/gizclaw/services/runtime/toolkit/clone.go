package toolkit

import (
	"encoding/json"
	"maps"
)

func cloneTool(in Tool) Tool {
	out := in
	out.Description = cloneStringPtr(in.Description)
	out.Version = cloneStringPtr(in.Version)
	out.InputSchema = *in.InputSchema.CloneSchemas()
	out.Metadata = cloneRaw(in.Metadata)
	out.Triggers = cloneTriggers(in.Triggers)
	out.HTTP = cloneHTTPRequest(in.HTTP)
	return out
}

func cloneHTTPRequest(in *HTTPRequest) *HTTPRequest {
	if in == nil {
		return nil
	}
	out := *in
	out.Headers = make(map[string]string, len(in.Headers))
	maps.Copy(out.Headers, in.Headers)
	out.Query = append([]HTTPArgumentBinding(nil), in.Query...)
	out.Body = append([]HTTPArgumentBinding(nil), in.Body...)
	out.ResponsePointer = cloneStringPtr(in.ResponsePointer)
	out.SuccessStatusCodes = append([]int(nil), in.SuccessStatusCodes...)
	out.Auth.BearerToken = cloneStringPtr(in.Auth.BearerToken)
	out.Auth.Header = cloneStringPtr(in.Auth.Header)
	out.Auth.APIKey = cloneStringPtr(in.Auth.APIKey)
	out.Auth.Credential = cloneStringPtr(in.Auth.Credential)
	out.Auth.Region = cloneStringPtr(in.Auth.Region)
	out.Auth.Service = cloneStringPtr(in.Auth.Service)
	out.Auth.Action = cloneStringPtr(in.Auth.Action)
	out.Auth.Version = cloneStringPtr(in.Auth.Version)
	return &out
}

func cloneTools(in []Tool) []Tool {
	out := make([]Tool, len(in))
	for i := range in {
		out[i] = cloneTool(in[i])
	}
	return out
}

func cloneTriggers(in []ToolTrigger) []ToolTrigger {
	if in == nil {
		return nil
	}
	out := make([]ToolTrigger, len(in))
	for i, trigger := range in {
		out[i] = trigger
		out[i].Description = cloneStringPtr(trigger.Description)
		out[i].Patterns = append([]string(nil), trigger.Patterns...)
		out[i].Metadata = cloneRaw(trigger.Metadata)
		out[i].Examples = make([]ToolTriggerExample, len(trigger.Examples))
		for j, example := range trigger.Examples {
			out[i].Examples[j] = example
			out[i].Examples[j].Args = cloneRaw(example.Args)
			out[i].Examples[j].Output = cloneStringPtr(example.Output)
		}
	}
	return out
}

func cloneRaw(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	return append(json.RawMessage(nil), in...)
}

func cloneStringPtr(in *string) *string {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
