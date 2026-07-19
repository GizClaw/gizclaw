package toolkit

import "encoding/json"

func cloneTool(in Tool) Tool {
	out := in
	out.Name = cloneStringPtr(in.Name)
	out.Description = cloneStringPtr(in.Description)
	out.OwnerPeer = cloneStringPtr(in.OwnerPeer)
	out.OwnerPublicKey = cloneStringPtr(in.OwnerPublicKey)
	out.Version = cloneStringPtr(in.Version)
	out.InputSchema = *in.InputSchema.CloneSchemas()
	if in.OutputSchema != nil {
		out.OutputSchema = in.OutputSchema.CloneSchemas()
	}
	out.Metadata = cloneRaw(in.Metadata)
	out.Executor.Name = cloneStringPtr(in.Executor.Name)
	out.Executor.Method = cloneStringPtr(in.Executor.Method)
	out.Executor.PeerID = cloneStringPtr(in.Executor.PeerID)
	out.Executor.Config = cloneRaw(in.Executor.Config)
	out.Triggers = cloneTriggers(in.Triggers)
	return out
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
