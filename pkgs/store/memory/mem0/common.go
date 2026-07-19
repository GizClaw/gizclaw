package mem0

import (
	"reflect"

	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

type (
	attributePatch          = memorystore.AttributePatch
	deleteRequest           = memorystore.DeleteRequest
	fact                    = memorystore.Fact
	filter                  = memorystore.Filter
	filterOperator          = memorystore.FilterOperator
	match                   = memorystore.Match
	observation             = memorystore.Observation
	observeResult           = memorystore.ObserveResult
	operation               = memorystore.Operation
	query                   = memorystore.Query
	recallResult            = memorystore.RecallResult
	role                    = memorystore.Role
	scope                   = memorystore.Scope
	sourceRef               = memorystore.SourceRef
	storeContract           = memorystore.Store
	operationWaiterContract = memorystore.OperationWaiter
	operationWaiter         = memorystore.OperationWaiter
	turn                    = memorystore.Turn
	updateRequest           = memorystore.UpdateRequest
)

const (
	filterEqual        = memorystore.FilterEqual
	filterNotEqual     = memorystore.FilterNotEqual
	filterIn           = memorystore.FilterIn
	filterNotIn        = memorystore.FilterNotIn
	filterExists       = memorystore.FilterExists
	filterGreaterThan  = memorystore.FilterGreaterThan
	filterGreaterEqual = memorystore.FilterGreaterEqual
	filterLessThan     = memorystore.FilterLessThan
	filterLessEqual    = memorystore.FilterLessEqual
	operationPending   = memorystore.OperationPending
	operationSucceeded = memorystore.OperationSucceeded
	operationFailed    = memorystore.OperationFailed
	roleUser           = memorystore.RoleUser
	roleAssistant      = memorystore.RoleAssistant
)

var (
	errConflict     = memorystore.ErrConflict
	errInvalidInput = memorystore.ErrInvalidInput
	errNotFound     = memorystore.ErrNotFound
	errUnavailable  = memorystore.ErrUnavailable
	errUnsupported  = memorystore.ErrUnsupported
)

func validateObservation(observation observation) error {
	return memorystore.ValidateObservation(observation)
}
func validateQuery(query query) error            { return memorystore.ValidateQuery(query) }
func validateUpdate(request updateRequest) error { return memorystore.ValidateUpdate(request) }
func validateDelete(request deleteRequest) error { return memorystore.ValidateDelete(request) }

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
