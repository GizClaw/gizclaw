package memory

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Role identifies the conversational role of a turn.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Turn is one ordered conversational input to memory extraction.
type Turn struct {
	ID         string
	Role       Role
	Speaker    string
	Text       string
	ObservedAt time.Time
	Attributes map[string]any
}

// Observation is raw material submitted for fact extraction. Text, Turns, or
// both may be set. Context is extraction input and is not required to be copied
// into resulting fact attributes.
type Observation struct {
	ID         string
	Text       string
	Turns      []Turn
	Context    map[string]any
	ObservedAt time.Time
}

// SourceRef connects an extracted fact to its source observation and turns.
type SourceRef struct {
	ObservationID string
	TurnIDs       []string
}

// Fact is one provider-neutral, recallable memory record. Revision is an
// opaque provider token used only for optimistic update or delete requests.
type Fact struct {
	ID         string
	Revision   string
	Text       string
	Attributes map[string]any
	Sources    []SourceRef
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// FilterOperator defines one structured recall-filter operation.
type FilterOperator string

const (
	FilterEqual        FilterOperator = "eq"
	FilterNotEqual     FilterOperator = "ne"
	FilterIn           FilterOperator = "in"
	FilterNotIn        FilterOperator = "not_in"
	FilterExists       FilterOperator = "exists"
	FilterGreaterThan  FilterOperator = "gt"
	FilterGreaterEqual FilterOperator = "gte"
	FilterLessThan     FilterOperator = "lt"
	FilterLessEqual    FilterOperator = "lte"
)

// Filter is one backend-neutral field predicate. Providers must return
// ErrUnsupported when they cannot preserve the requested predicate.
type Filter struct {
	Field    string
	Operator FilterOperator
	Value    any
}

// Query selects facts relevant to Text. Limit must be positive. Matches are
// returned in descending relevance order.
type Query struct {
	Text    string
	Limit   int
	Filters []Filter
}

// Match is a recalled fact and its provider-normalized relevance score.
type Match struct {
	Fact  Fact
	Score float64
}

// RecallResult contains ordered recall matches.
type RecallResult struct {
	Matches []Match
}

// OperationStatus is the state of asynchronous observation processing.
type OperationStatus string

const (
	OperationPending   OperationStatus = "pending"
	OperationSucceeded OperationStatus = "succeeded"
	OperationFailed    OperationStatus = "failed"
)

// Operation identifies asynchronous provider work.
type Operation struct {
	ID     string
	Status OperationStatus
	Error  string
}

// ObserveResult reports facts materialized by Observe or Wait. Operation is
// non-nil when the provider exposes asynchronous processing state.
type ObserveResult struct {
	Facts     []Fact
	Operation *Operation
}

// AttributePatch applies explicit attribute additions/replacements and
// deletions. A key cannot appear in both Set and Delete.
type AttributePatch struct {
	Set    map[string]any
	Delete []string
}

// UpdateRequest changes a logical fact. A nil Text preserves current text.
// ExpectedRevision is optional; providers without conditional writes return
// ErrUnsupported when it is supplied.
type UpdateRequest struct {
	ID               string
	ExpectedRevision string
	Text             *string
	Attributes       AttributePatch
}

// DeleteRequest removes or retires a logical fact. ExpectedRevision is
// optional; providers without conditional deletes return ErrUnsupported when
// it is supplied.
type DeleteRequest struct {
	ID               string
	ExpectedRevision string
}

func validateObservation(observation Observation) error {
	if strings.TrimSpace(observation.Text) == "" && len(observation.Turns) == 0 {
		return fmt.Errorf("%w: observation requires text or turns", ErrInvalidInput)
	}
	for i, turn := range observation.Turns {
		if !validRole(turn.Role) {
			return fmt.Errorf("%w: turn %d has invalid role %q", ErrInvalidInput, i, turn.Role)
		}
		if strings.TrimSpace(turn.Text) == "" {
			return fmt.Errorf("%w: turn %d has empty text", ErrInvalidInput, i)
		}
		if len(turn.Attributes) > 0 {
			return fmt.Errorf("%w: turn %d attributes are not supported", ErrUnsupported, i)
		}
	}
	return nil
}

func validateQuery(query Query) error {
	if strings.TrimSpace(query.Text) == "" {
		return fmt.Errorf("%w: query text is required", ErrInvalidInput)
	}
	if query.Limit <= 0 {
		return fmt.Errorf("%w: query limit must be positive", ErrInvalidInput)
	}
	for i, filter := range query.Filters {
		if err := validateFilter(filter); err != nil {
			return fmt.Errorf("filter %d: %w", i, err)
		}
	}
	return nil
}

func validateFilter(filter Filter) error {
	if strings.TrimSpace(filter.Field) == "" {
		return fmt.Errorf("%w: filter field is required", ErrInvalidInput)
	}
	switch filter.Operator {
	case FilterEqual, FilterNotEqual, FilterGreaterThan, FilterGreaterEqual, FilterLessThan, FilterLessEqual:
		if filter.Value == nil {
			return fmt.Errorf("%w: filter %q requires a value", ErrInvalidInput, filter.Operator)
		}
	case FilterIn, FilterNotIn:
		value := reflect.ValueOf(filter.Value)
		if !value.IsValid() || (value.Kind() != reflect.Array && value.Kind() != reflect.Slice) || value.Len() == 0 {
			return fmt.Errorf("%w: filter %q requires a non-empty array value", ErrInvalidInput, filter.Operator)
		}
	case FilterExists:
		if _, ok := filter.Value.(bool); !ok {
			return fmt.Errorf("%w: filter %q requires a boolean value", ErrInvalidInput, filter.Operator)
		}
	default:
		return fmt.Errorf("%w: unknown filter operator %q", ErrInvalidInput, filter.Operator)
	}
	return nil
}

func validateUpdate(request UpdateRequest) error {
	if strings.TrimSpace(request.ID) == "" {
		return fmt.Errorf("%w: update fact id is required", ErrInvalidInput)
	}
	if request.Text == nil && len(request.Attributes.Set) == 0 && len(request.Attributes.Delete) == 0 {
		return fmt.Errorf("%w: update has no changes", ErrInvalidInput)
	}
	deleted := make(map[string]struct{}, len(request.Attributes.Delete))
	for _, key := range request.Attributes.Delete {
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("%w: update contains an empty attribute key", ErrInvalidInput)
		}
		if _, duplicate := deleted[key]; duplicate {
			return fmt.Errorf("%w: update deletes attribute %q more than once", ErrInvalidInput, key)
		}
		deleted[key] = struct{}{}
	}
	for key := range request.Attributes.Set {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("%w: update contains an empty attribute key", ErrInvalidInput)
		}
		if _, conflict := deleted[key]; conflict {
			return fmt.Errorf("%w: update both sets and deletes attribute %q", ErrInvalidInput, key)
		}
	}
	return nil
}

func validateDelete(request DeleteRequest) error {
	if strings.TrimSpace(request.ID) == "" {
		return fmt.Errorf("%w: delete fact id is required", ErrInvalidInput)
	}
	return nil
}

func validRole(role Role) bool {
	switch role {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		return true
	default:
		return false
	}
}

func cloneFact(fact Fact) Fact {
	fact.Attributes = cloneMap(fact.Attributes)
	fact.Sources = cloneSources(fact.Sources)
	return fact
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = cloneValue(value)
	}
	return output
}

func cloneValue(value any) any {
	if value == nil {
		return nil
	}
	return cloneReflectValue(reflect.ValueOf(value)).Interface()
}

func cloneReflectValue(value reflect.Value) reflect.Value {
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneReflectValue(value.Elem())
		output := reflect.New(value.Type()).Elem()
		output.Set(cloned)
		return output
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		output := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			output.SetMapIndex(iterator.Key(), cloneReflectValue(iterator.Value()))
		}
		return output
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		output := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := range value.Len() {
			output.Index(index).Set(cloneReflectValue(value.Index(index)))
		}
		return output
	case reflect.Array:
		output := reflect.New(value.Type()).Elem()
		for index := range value.Len() {
			output.Index(index).Set(cloneReflectValue(value.Index(index)))
		}
		return output
	default:
		return value
	}
}

func cloneSources(input []SourceRef) []SourceRef {
	if input == nil {
		return nil
	}
	output := make([]SourceRef, len(input))
	for i, source := range input {
		output[i] = SourceRef{
			ObservationID: source.ObservationID,
			TurnIDs:       append([]string(nil), source.TurnIDs...),
		}
	}
	return output
}
