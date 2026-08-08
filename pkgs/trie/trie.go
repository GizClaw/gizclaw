// Package trie provides a generic trie data structure for efficient path-based
// storage and retrieval. It supports MQTT-style topic patterns with wildcards:
//   - "/a/b/c" - exact path match
//   - "/a/+/c" - single-level wildcard (matches any single segment)
//   - "/a/#"   - multi-level wildcard (matches any remaining path segments)
package trie

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrInvalidPattern is returned when the path pattern is invalid.
var ErrInvalidPattern = errors.New("invalid path pattern, path should be /a/b/c or /a/+/c or /a/#")

// Trie is a generic trie data structure that supports path-based storage with
// wildcard matching.
type Trie[T any] struct {
	children map[string]*Trie[T]
	matchAny *Trie[T]
	matchAll *Trie[T]
	set      bool
	value    T
}

// New creates a new empty Trie.
func New[T any]() *Trie[T] {
	return &Trie[T]{}
}

func (t *Trie[T]) setFunc(fn func(ptr *T, existed bool) error) error {
	if err := fn(&t.value, t.set); err != nil {
		return err
	}
	t.set = true
	return nil
}

// Set stores a value at the specified path using the provided setFunc.
func (t *Trie[T]) Set(path string, setFunc func(ptr *T, existed bool) error) error {
	if len(path) == 0 {
		return t.setFunc(setFunc)
	}

	var first, subseq string
	if before, after, ok := strings.Cut(path, "/"); !ok {
		first = path
	} else {
		first = before
		subseq = after
	}
	if t.children != nil {
		if ch, ok := t.children[first]; ok {
			return ch.Set(subseq, setFunc)
		}
	}

	switch first {
	case "+":
		if t.matchAny == nil {
			t.matchAny = &Trie[T]{}
		}
		return t.matchAny.Set(subseq, setFunc)
	case "#":
		if len(subseq) != 0 {
			return ErrInvalidPattern
		}
		if t.matchAll == nil {
			t.matchAll = &Trie[T]{}
		}
		return t.matchAll.setFunc(setFunc)
	default:
		if t.children == nil {
			t.children = make(map[string]*Trie[T])
		}
		ch := &Trie[T]{}
		t.children[first] = ch
		return ch.Set(subseq, setFunc)
	}
}

// SetValue is a convenience method that stores a value at the specified path.
func (t *Trie[T]) SetValue(path string, value T) error {
	return t.Set(path, func(ptr *T, _ bool) error {
		*ptr = value
		return nil
	})
}

// Get retrieves the value stored at the specified path.
func (t *Trie[T]) Get(path string) (*T, bool) {
	_, val, ok := t.match("", path)
	return val, ok
}

// GetValue retrieves the value stored at the specified path.
func (t *Trie[T]) GetValue(path string) (T, bool) {
	ptr, ok := t.Get(path)
	if !ok {
		var zero T
		return zero, false
	}
	return *ptr, true
}

// Match returns the matched route pattern and value for the given path.
func (t *Trie[T]) Match(path string) (route string, value *T, ok bool) {
	return t.match("", path)
}

func (t *Trie[T]) match(matched, path string) (string, *T, bool) {
	if len(path) == 0 {
		return matched, &t.value, t.set
	}
	var first, subseq string
	before, after, ok := strings.Cut(path, "/")
	if !ok {
		first = path
	} else {
		first = before
		subseq = after
	}
	if t.children != nil {
		if ch, ok := t.children[first]; ok {
			if route, handlers, ok := ch.match(matched+"/"+first, subseq); ok {
				return route, handlers, true
			}
		}
	}
	if t.matchAny != nil {
		if route, handlers, ok := t.matchAny.match(matched+"/+", subseq); ok {
			return route, handlers, true
		}
	}
	if t.matchAll != nil {
		if route, handlers, ok := t.matchAll.match(matched+"/#", ""); ok {
			return route, handlers, true
		}
	}
	return "", nil, false
}

// Walk calls the given function for each node in the trie.
func (t *Trie[T]) Walk(f func(path string, value T, set bool)) {
	t.walkWithPath(nil, func(path []string, node *Trie[T]) {
		f(strings.Join(path, "/"), node.value, node.set)
	})
}

func (t *Trie[T]) walkWithPath(path []string, f func([]string, *Trie[T])) {
	if t.children != nil {
		for seg, ch := range t.children {
			ch.walkWithPath(append(path, seg), f)
		}
	}
	if t.matchAny != nil {
		t.matchAny.walkWithPath(append(path, "+"), f)
	}
	if t.matchAll != nil {
		t.matchAll.walkWithPath(append(path, "#"), f)
	}
	f(path, t)
}

// String returns a string representation of the trie structure.
func (t *Trie[T]) String() string {
	var lines []string
	t.walkWithPath(nil, func(path []string, node *Trie[T]) {
		if node.set {
			lines = append(lines, fmt.Sprintf("%s: %v", strings.Join(path, "/"), node.value))
		}
	})
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// Len returns the number of values stored in the trie.
func (t *Trie[T]) Len() int {
	count := 0
	t.Walk(func(_ string, _ T, set bool) {
		if set {
			count++
		}
	})
	return count
}
