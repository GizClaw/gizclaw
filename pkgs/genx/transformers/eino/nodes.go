package eino

import "time"

// Binding reads one Graph input, History, Memory, or state value.
type Binding struct {
	From string
}

// NodeDefinition is a closed union of supported node definitions.
type NodeDefinition struct {
	ID      string
	Inputs  map[string]Binding
	Outputs map[string]string

	Prompt      *PromptNode
	ChatModel   *ChatModelNode
	Transform   *TransformNode
	Script      *ScriptNode
	Lambda      *LambdaRefNode
	Race        *RaceNode
	Batch       *BatchNode
	Passthrough *PassthroughNode
	Retriever   *RetrieverNode
	Subgraph    *SubgraphNode
}

// ChatModelNode invokes one resolved Eino chat model.
type ChatModelNode struct {
	Model       string
	Temperature *float32
	MaxTokens   *int
}

// PromptNode formats ordered Eino messages.
type PromptNode struct {
	Format   PromptFormat
	Messages []PromptMessage
}

// PromptFormat selects Eino's supported template syntax.
type PromptFormat string

const (
	PromptFString    PromptFormat = "f_string"
	PromptGoTemplate PromptFormat = "go_template"
	PromptJinja2     PromptFormat = "jinja2"
)

// PromptMessage is a typed message template or message placeholder.
type PromptMessage struct {
	Role        PromptRole
	Template    string
	Placeholder string
	Optional    bool
}

// PromptRole identifies one supported conversational role.
type PromptRole string

const (
	PromptSystem    PromptRole = "system"
	PromptUser      PromptRole = "user"
	PromptAssistant PromptRole = "assistant"
)

// TransformNode performs one deterministic built-in transformation.
type TransformNode struct {
	Operation      TransformOperation
	Order          []string
	Separator      string
	Messages       []TransformMessage
	MaxInputBytes  int
	MaxOutputBytes int
}

// TransformOperation is one closed built-in operation.
type TransformOperation string

const (
	TransformSelect        TransformOperation = "select"
	TransformConcatText    TransformOperation = "concat_text"
	TransformDecodeJSON    TransformOperation = "decode_json"
	TransformBuildMessages TransformOperation = "build_messages"
)

// TransformMessage builds one message from a bound input or literal text.
type TransformMessage struct {
	Role  PromptRole
	Input string
	Text  string
}

// ScriptNode runs one bounded Starlark entrypoint.
type ScriptNode struct {
	Language   ScriptLanguage
	Entrypoint string
	Source     string
	Limits     ScriptLimits
}

// ScriptLanguage is the supported sandbox language.
type ScriptLanguage string

const ScriptStarlark ScriptLanguage = "starlark"

// ScriptLimits bound one Starlark evaluation.
type ScriptLimits struct {
	MaxExecutionSteps uint64
	Timeout           time.Duration
	MaxInputBytes     int
	MaxOutputBytes    int
}

// LambdaRefNode invokes one resolved named Eino Lambda.
type LambdaRefNode struct {
	Lambda string
}

// RaceNode runs isolated nested Graphs and selects one winner.
type RaceNode struct {
	Branches       []RaceBranch
	Winner         RaceWinnerDefinition
	MaxConcurrency int
}

// RaceBranch identifies one nested Graph.
type RaceBranch struct {
	ID    string
	Graph GraphDefinition
}

// RaceWinnerDefinition selects the first output, success, or predicate match.
type RaceWinnerDefinition struct {
	Mode RaceWinnerMode
	When *Predicate
}

// RaceWinnerMode controls winner selection.
type RaceWinnerMode string

const (
	RaceFirstOutput  RaceWinnerMode = "first_output"
	RaceFirstSuccess RaceWinnerMode = "first_success"
	RacePredicate    RaceWinnerMode = "predicate"
)

// BatchNode runs a nested Graph over a list with bounded concurrency.
type BatchNode struct {
	Items          Binding
	Graph          GraphDefinition
	MaxConcurrency int
}

// PassthroughNode forwards one value.
type PassthroughNode struct{}

// RetrieverNode invokes one resolved Eino retriever.
type RetrieverNode struct {
	Retriever string
	Query     Binding
	TopK      int
}

// SubgraphNode runs one nested Graph.
type SubgraphNode struct {
	Graph GraphDefinition
}

func (node NodeDefinition) kindCount() int {
	count := 0
	for _, present := range []bool{
		node.Prompt != nil, node.ChatModel != nil, node.Transform != nil,
		node.Script != nil, node.Lambda != nil, node.Race != nil,
		node.Batch != nil, node.Passthrough != nil, node.Retriever != nil,
		node.Subgraph != nil,
	} {
		if present {
			count++
		}
	}
	return count
}
