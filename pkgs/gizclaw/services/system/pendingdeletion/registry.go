package pendingdeletion

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var sourceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type registration struct {
	source   Source
	handlers map[Kind]Handler
}

type sourceValidator interface {
	Validate() error
}

// Registry validates source and handler ownership before workers start.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]registration
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]registration)}
}

// Register adds one source only when every advertised kind has one handler.
func (r *Registry) Register(source Source, handlers ...Handler) error {
	if r == nil {
		return errors.New("pending deletion: nil registry")
	}
	if isNilInterface(source) {
		return errors.New("pending deletion: nil source")
	}
	if validator, ok := source.(sourceValidator); ok {
		if err := validator.Validate(); err != nil {
			return fmt.Errorf("pending deletion: validate source: %w", err)
		}
	}
	name := source.Name()
	if !sourceNamePattern.MatchString(name) || name != strings.TrimSpace(name) {
		return fmt.Errorf("pending deletion: invalid source name %q", name)
	}
	owned := make(map[Kind]bool)
	for _, kind := range source.Kinds() {
		if !kind.valid() || owned[kind] {
			return fmt.Errorf("pending deletion: source %q has invalid or duplicate kind %q", name, kind)
		}
		owned[kind] = true
	}
	if len(owned) == 0 {
		return fmt.Errorf("pending deletion: source %q has no kinds", name)
	}
	byKind := make(map[Kind]Handler, len(handlers))
	for _, handler := range handlers {
		if isNilInterface(handler) {
			return fmt.Errorf("pending deletion: source %q has nil handler", name)
		}
		kind := handler.Kind()
		if !owned[kind] {
			return fmt.Errorf("pending deletion: source %q handler owns unadvertised kind %q", name, kind)
		}
		if _, exists := byKind[kind]; exists {
			return fmt.Errorf("pending deletion: source %q has duplicate handler for %q", name, kind)
		}
		byKind[kind] = handler
	}
	for kind := range owned {
		if byKind[kind] == nil {
			return fmt.Errorf("pending deletion: source %q has no handler for %q", name, kind)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[name]; exists {
		return fmt.Errorf("pending deletion: duplicate source %q", name)
	}
	r.entries[name] = registration{source: source, handlers: byKind}
	return nil
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (r *Registry) lookup(source string, kind Kind) (Source, Handler, bool) {
	if r == nil {
		return nil, nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.entries[source]
	if !ok {
		return nil, nil, false
	}
	handler, ok := entry.handlers[kind]
	return entry.source, handler, ok
}

func (r *Registry) sources() []Source {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.entries))
	for name := range r.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]Source, 0, len(names))
	for _, name := range names {
		result = append(result, r.entries[name].source)
	}
	return result
}

func (r *Registry) source(name string) (Source, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.entries[name]
	return entry.source, ok
}
