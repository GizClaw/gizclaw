package genx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
// Transformer is configured with a ToolInvoker and no explicit positive limit.
const DefaultMaxToolCalls = 32

// ToolDefinition describes one function that a model may call. The schema is
// owned by the caller of ToolInvoker.ResolveTools and may be mutated.
type ToolDefinition struct {
	Name        string
	Description string
	Argument    *jsonschema.Schema
}

// ToolInvoker resolves the function declarations available to a model and
// executes one function by name. Implementations own resource resolution,
// authorization, argument validation, and executor dispatch.
type ToolInvoker interface {
	ResolveTools(context.Context) ([]ToolDefinition, error)
	InvokeTool(context.Context, string, json.RawMessage) (json.RawMessage, error)
}

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

// ResolveTools returns defensive declaration snapshots in construction order.
func (t *Toolkit) ResolveTools(ctx context.Context) ([]ToolDefinition, error) {
	if t == nil {
		return nil, fmt.Errorf("%w: Toolkit is nil", ErrInvalidToolkit)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("genx: resolve Toolkit tools: %w", err)
	}
	tools := make([]ToolDefinition, 0, len(t.ordered))
	for index := range t.ordered {
		definition, err := cloneToolkitDefinition(&t.ordered[index])
		if err != nil {
			return nil, fmt.Errorf("genx: clone Toolkit tool %q: %w", t.ordered[index].tool.Name, err)
		}
		tools = append(tools, definition)
	}
	return tools, nil
}

// InvokeTool validates and executes one function call. Provider call
// identifiers remain owned by the Transformer invocation.
func (t *Toolkit) InvokeTool(
	ctx context.Context,
	name string,
	arguments json.RawMessage,
) (json.RawMessage, error) {
	if t == nil {
		return nil, fmt.Errorf("%w: Toolkit is nil", ErrInvalidToolkit)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: function name is required", ErrInvalidToolkit)
	}
	index, ok := t.byName[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrToolkitToolNotFound, name)
	}
	entry := &t.ordered[index]
	instance, err := decodeToolkitArguments(arguments)
	if err != nil {
		return nil, fmt.Errorf("%w: tool %q arguments: %w", ErrInvalidToolkit, name, err)
	}
	if err := entry.resolved.Validate(instance); err != nil {
		return nil, fmt.Errorf("%w: arguments do not match tool %q: %w", ErrInvalidToolkit, name, err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("genx: invoke Toolkit tool %q: %w", name, err)
	}
	funcCall := &FuncCall{Name: name, Arguments: string(arguments)}
	result, err := entry.tool.Invoke(ctx, funcCall, string(arguments))
	if err != nil {
		return nil, fmt.Errorf("genx: invoke Toolkit tool %q: %w", name, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("genx: discard late Toolkit tool %q result: %w", name, err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("genx: encode Toolkit tool %q result: %w", name, err)
	}
	return encoded, nil
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

func cloneToolkitDefinition(entry *toolkitEntry) (ToolDefinition, error) {
	var schemaClone jsonschema.Schema
	if err := json.Unmarshal(entry.schemaJSON, &schemaClone); err != nil {
		return ToolDefinition{}, err
	}
	return ToolDefinition{
		Name:        entry.tool.Name,
		Description: entry.tool.Description,
		Argument:    &schemaClone,
	}, nil
}

func decodeToolkitArguments(arguments json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(arguments))
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
