package eino

import (
	"fmt"
	"reflect"
	"strings"
)

func evaluatePredicate(predicate Predicate, state map[string]any) (bool, error) {
	if predicate.All != nil {
		for _, child := range predicate.All {
			matched, err := evaluatePredicate(child, state)
			if err != nil || !matched {
				return matched, err
			}
		}
		return true, nil
	}
	if predicate.Any != nil {
		for _, child := range predicate.Any {
			matched, err := evaluatePredicate(child, state)
			if err != nil {
				return false, err
			}
			if matched {
				return true, nil
			}
		}
		return false, nil
	}
	if predicate.Not != nil {
		matched, err := evaluatePredicate(*predicate.Not, state)
		return !matched, err
	}
	value, exists := state[predicate.Field]
	switch predicate.Op {
	case PredicateExists:
		return exists, nil
	case PredicateNotExists:
		return !exists, nil
	}
	if !exists {
		return false, nil
	}
	switch predicate.Op {
	case PredicateEqual, PredicateNotEqual:
		matched := reflect.DeepEqual(canonicalComparable(value), canonicalComparable(predicate.Value))
		if predicate.Op == PredicateNotEqual {
			matched = !matched
		}
		return matched, nil
	case PredicateContains, PredicateNotContains:
		matched, err := containsValue(value, predicate.Value)
		if predicate.Op == PredicateNotContains {
			matched = !matched
		}
		return matched, err
	case PredicateLess, PredicateLessEqual, PredicateGreater, PredicateGreaterEqual:
		left, err := normalizeNumber(value)
		if err != nil {
			return false, err
		}
		right, err := normalizeNumber(predicate.Value)
		if err != nil {
			return false, err
		}
		switch predicate.Op {
		case PredicateLess:
			return left < right, nil
		case PredicateLessEqual:
			return left <= right, nil
		case PredicateGreater:
			return left > right, nil
		default:
			return left >= right, nil
		}
	default:
		return false, fmt.Errorf("eino: unsupported predicate operator %q", predicate.Op)
	}
}

func canonicalComparable(value any) any {
	if number, err := normalizeNumber(value); err == nil {
		return number
	}
	return value
}

func containsValue(container, needle any) (bool, error) {
	switch value := container.(type) {
	case string:
		text, ok := needle.(string)
		return ok && strings.Contains(value, text), nil
	case []any:
		for _, item := range value {
			if reflect.DeepEqual(canonicalComparable(item), canonicalComparable(needle)) {
				return true, nil
			}
		}
		return false, nil
	case map[string]any:
		key, ok := needle.(string)
		if !ok {
			return false, nil
		}
		_, exists := value[key]
		return exists, nil
	default:
		return false, fmt.Errorf("eino: cannot apply contains to %T", container)
	}
}
