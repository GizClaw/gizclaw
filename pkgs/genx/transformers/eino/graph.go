package eino

import "fmt"

// GraphDefinition is one declarative Eino Graph.
type GraphDefinition struct {
	Name     string
	Compile  GraphCompileConfig
	State    StateDefinition
	Nodes    []NodeDefinition
	Edges    []EdgeDefinition
	Branches []BranchDefinition
	Outputs  []OutputDefinition
}

// GraphCompileConfig controls Eino Graph compilation.
type GraphCompileConfig struct {
	MaxRunSteps     int
	NodeTriggerMode NodeTriggerMode
	FanIn           map[string]FanInConfig
}

// NodeTriggerMode controls whether a node runs after any or all predecessors.
type NodeTriggerMode string

const (
	// NodeTriggerAnyPredecessor maps to Eino's Pregel scheduler.
	NodeTriggerAnyPredecessor NodeTriggerMode = "any_predecessor"
	// NodeTriggerAllPredecessor maps to Eino's DAG scheduler.
	NodeTriggerAllPredecessor NodeTriggerMode = "all_predecessor"
)

// FanInConfig is the supported Eino fan-in stream configuration.
type FanInConfig struct {
	StreamMergeWithSourceEOF bool
}

// StateDefinition declares the typed state visible to nodes.
type StateDefinition struct {
	Fields []StateField
}

// StateField declares one state value and its sequential merge behavior.
type StateField struct {
	Name     string
	Type     StateType
	Required bool
	Merge    MergePolicy
}

// StateType is one supported configuration value type.
type StateType string

const (
	StateString    StateType = "string"
	StateBoolean   StateType = "boolean"
	StateInteger   StateType = "integer"
	StateNumber    StateType = "number"
	StateObject    StateType = "object"
	StateList      StateType = "list"
	StateMessages  StateType = "messages"
	StateDocuments StateType = "documents"
	StateBlob      StateType = "blob"
)

// MergePolicy controls sequential updates to one state field.
type MergePolicy string

const (
	MergeReplace MergePolicy = "replace"
	MergeAppend  MergePolicy = "append"
	MergeObject  MergePolicy = "object_merge"
)

// EdgeDefinition is one Eino Graph edge.
type EdgeDefinition struct {
	From string
	To   string
}

// BranchDefinition routes one node output to one or more declared targets.
type BranchDefinition struct {
	From    string
	Mode    BranchMode
	Routes  []BranchRoute
	Default string
}

// BranchMode controls whether the first or every matching route is selected.
type BranchMode string

const (
	BranchFirstMatch BranchMode = "first_match"
	BranchAllMatch   BranchMode = "all_match"
)

// BranchRoute is one predicate and destination.
type BranchRoute struct {
	When Predicate
	To   string
}

// Predicate is a closed recursive predicate over declared state.
type Predicate struct {
	All   []Predicate
	Any   []Predicate
	Not   *Predicate
	Field string
	Op    PredicateOperator
	Value any
}

// PredicateOperator is one supported leaf comparison.
type PredicateOperator string

const (
	PredicateEqual        PredicateOperator = "eq"
	PredicateNotEqual     PredicateOperator = "ne"
	PredicateExists       PredicateOperator = "exists"
	PredicateNotExists    PredicateOperator = "not_exists"
	PredicateContains     PredicateOperator = "contains"
	PredicateNotContains  PredicateOperator = "not_contains"
	PredicateLess         PredicateOperator = "lt"
	PredicateLessEqual    PredicateOperator = "lte"
	PredicateGreater      PredicateOperator = "gt"
	PredicateGreaterEqual PredicateOperator = "gte"
)

// OutputDefinition publishes one declared node state field as a GenX route.
type OutputDefinition struct {
	Node     string
	Field    string
	Name     string
	MIMEType string
	Primary  bool
}

func validStateType(value StateType) bool {
	switch value {
	case StateString, StateBoolean, StateInteger, StateNumber, StateObject,
		StateList, StateMessages, StateDocuments, StateBlob:
		return true
	default:
		return false
	}
}

func validMerge(stateType StateType, merge MergePolicy) bool {
	switch merge {
	case MergeReplace:
		return true
	case MergeAppend:
		return stateType == StateString || stateType == StateList ||
			stateType == StateMessages || stateType == StateDocuments
	case MergeObject:
		return stateType == StateObject
	default:
		return false
	}
}

func validatePredicate(predicate Predicate, fields map[string]StateType) error {
	forms := 0
	if predicate.All != nil {
		forms++
	}
	if predicate.Any != nil {
		forms++
	}
	if predicate.Not != nil {
		forms++
	}
	if predicate.Field != "" || predicate.Op != "" {
		forms++
	}
	if forms != 1 {
		return fmt.Errorf("predicate requires exactly one form")
	}
	if predicate.All != nil || predicate.Any != nil {
		group := predicate.All
		if predicate.Any != nil {
			group = predicate.Any
		}
		if len(group) == 0 {
			return fmt.Errorf("predicate group cannot be empty")
		}
		for index, child := range group {
			if err := validatePredicate(child, fields); err != nil {
				return fmt.Errorf("predicate child %d: %w", index, err)
			}
		}
		return nil
	}
	if predicate.Not != nil {
		return validatePredicate(*predicate.Not, fields)
	}
	stateType, ok := fields[predicate.Field]
	if !ok {
		return fmt.Errorf("predicate field %q is not declared", predicate.Field)
	}
	switch predicate.Op {
	case PredicateExists, PredicateNotExists:
		return nil
	case PredicateEqual, PredicateNotEqual:
		if !valueMatchesStateType(predicate.Value, stateType) {
			return fmt.Errorf("predicate value is incompatible with field %q", predicate.Field)
		}
	case PredicateContains, PredicateNotContains:
		if stateType != StateString && stateType != StateList && stateType != StateObject {
			return fmt.Errorf("predicate %q requires string, list, or object", predicate.Op)
		}
	case PredicateLess, PredicateLessEqual, PredicateGreater, PredicateGreaterEqual:
		if stateType != StateInteger && stateType != StateNumber {
			return fmt.Errorf("predicate %q requires integer or number", predicate.Op)
		}
	default:
		return fmt.Errorf("unsupported predicate operator %q", predicate.Op)
	}
	return nil
}
