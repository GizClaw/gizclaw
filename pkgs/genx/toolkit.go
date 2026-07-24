package genx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

var (
	// ErrInvalidToolkit reports an invalid Toolkit declaration or invocation.
	ErrInvalidToolkit = errors.New("genx: invalid Toolkit")
	// ErrToolkitToolNotFound reports a call to a function outside the Toolkit.
	ErrToolkitToolNotFound = errors.New("genx: Toolkit tool not found")
)

// DefaultMaxToolCalls is the per-invocation ToolCall limit used when a
// Transformer is configured with a Toolkit and no explicit positive limit.
const DefaultMaxToolCalls = 32

type toolkitEntry struct {
	tool       FuncTool
	schemaJSON []byte
	resolved   *jsonschema.Resolved
}

// Toolkit is an immutable ordered collection of executable function tools.
// It is safe for concurrent use when the supplied executors are safe for
// concurrent use.
type Toolkit struct {
	ordered []toolkitEntry
	byName  map[string]int
}

// NewToolkit validates and snapshots executable function tools.
func NewToolkit(tools ...*FuncTool) (*Toolkit, error) {
	toolkit := &Toolkit{
		ordered: make([]toolkitEntry, 0, len(tools)),
		byName:  make(map[string]int, len(tools)),
	}
	for index, source := range tools {
		entry, err := snapshotToolkitEntry(source)
		if err != nil {
			return nil, fmt.Errorf("%w: tool %d: %w", ErrInvalidToolkit, index, err)
		}
		if _, duplicate := toolkit.byName[entry.tool.Name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate tool name %q", ErrInvalidToolkit, entry.tool.Name)
		}
		toolkit.byName[entry.tool.Name] = len(toolkit.ordered)
		toolkit.ordered = append(toolkit.ordered, entry)
	}
	return toolkit, nil
}

// Tools yields defensive snapshots in construction order.
func (t *Toolkit) Tools() iter.Seq[*FuncTool] {
	return func(yield func(*FuncTool) bool) {
		if t == nil {
			return
		}
		for index := range t.ordered {
			entry := &t.ordered[index]
			tool, err := cloneToolkitTool(entry)
			if err != nil || !yield(tool) {
				return
			}
		}
	}
}

// Invoke validates and executes one function call. Call identifiers are not
// tracked globally because their uniqueness belongs to a Transformer
// invocation or provider session.
func (t *Toolkit) Invoke(ctx context.Context, call ToolCall) (ToolResult, error) {
	if t == nil {
		return ToolResult{}, fmt.Errorf("%w: Toolkit is nil", ErrInvalidToolkit)
	}
	call.ID = strings.TrimSpace(call.ID)
	if call.ID == "" {
		return ToolResult{}, fmt.Errorf("%w: call ID is required", ErrInvalidToolkit)
	}
	if call.FuncCall == nil {
		return ToolResult{}, fmt.Errorf("%w: call %q has no function", ErrInvalidToolkit, call.ID)
	}
	name := strings.TrimSpace(call.FuncCall.Name)
	if name == "" {
		return ToolResult{}, fmt.Errorf("%w: call %q function name is required", ErrInvalidToolkit, call.ID)
	}
	index, ok := t.byName[name]
	if !ok {
		return ToolResult{}, fmt.Errorf("%w: %s", ErrToolkitToolNotFound, name)
	}
	entry := &t.ordered[index]
	instance, err := decodeToolkitArguments(call.FuncCall.Arguments)
	if err != nil {
		return ToolResult{}, fmt.Errorf("%w: call %q arguments: %w", ErrInvalidToolkit, call.ID, err)
	}
	if err := entry.resolved.Validate(instance); err != nil {
		return ToolResult{}, fmt.Errorf("%w: call %q arguments do not match %q: %w", ErrInvalidToolkit, call.ID, name, err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ToolResult{}, fmt.Errorf("genx: invoke Toolkit tool %q for call %q: %w", name, call.ID, err)
	}
	funcCall := &FuncCall{Name: name, Arguments: call.FuncCall.Arguments}
	result, err := entry.tool.Invoke(ctx, funcCall, call.FuncCall.Arguments)
	if err != nil {
		return ToolResult{}, fmt.Errorf("genx: invoke Toolkit tool %q for call %q: %w", name, call.ID, err)
	}
	if err := ctx.Err(); err != nil {
		return ToolResult{}, fmt.Errorf("genx: discard late Toolkit tool %q result for call %q: %w", name, call.ID, err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return ToolResult{}, fmt.Errorf("genx: encode Toolkit tool %q result for call %q: %w", name, call.ID, err)
	}
	return ToolResult{ID: call.ID, Result: string(encoded)}, nil
}

func snapshotToolkitEntry(source *FuncTool) (toolkitEntry, error) {
	if source == nil {
		return toolkitEntry{}, errors.New("tool is nil")
	}
	name := strings.TrimSpace(source.Name)
	if name == "" {
		return toolkitEntry{}, errors.New("tool name is required")
	}
	if source.Argument == nil {
		return toolkitEntry{}, fmt.Errorf("tool %q argument schema is required", name)
	}
	if source.Invoke == nil {
		return toolkitEntry{}, fmt.Errorf("tool %q executor is required", name)
	}
	schemaJSON, err := json.Marshal(source.Argument)
	if err != nil {
		return toolkitEntry{}, fmt.Errorf("encode tool %q argument schema: %w", name, err)
	}
	var schemaClone jsonschema.Schema
	if err := json.Unmarshal(schemaJSON, &schemaClone); err != nil {
		return toolkitEntry{}, fmt.Errorf("clone tool %q argument schema: %w", name, err)
	}
	resolved, err := schemaClone.Resolve(nil)
	if err != nil {
		return toolkitEntry{}, fmt.Errorf("resolve tool %q argument schema: %w", name, err)
	}
	return toolkitEntry{
		tool: FuncTool{
			Name:        name,
			Description: strings.TrimSpace(source.Description),
			Argument:    &schemaClone,
			Invoke:      source.Invoke,
		},
		schemaJSON: schemaJSON,
		resolved:   resolved,
	}, nil
}

func cloneToolkitTool(entry *toolkitEntry) (*FuncTool, error) {
	var schemaClone jsonschema.Schema
	if err := json.Unmarshal(entry.schemaJSON, &schemaClone); err != nil {
		return nil, err
	}
	return &FuncTool{
		Name:        entry.tool.Name,
		Description: entry.tool.Description,
		Argument:    &schemaClone,
		Invoke:      entry.tool.Invoke,
	}, nil
}

func decodeToolkitArguments(arguments string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(arguments))
	var instance any
	if err := decoder.Decode(&instance); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	if !bytes.Equal(bytes.TrimSpace([]byte(arguments)), []byte("null")) && instance == nil {
		return nil, errors.New("arguments are empty")
	}
	return instance, nil
}
