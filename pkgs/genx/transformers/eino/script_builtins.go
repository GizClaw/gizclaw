package eino

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	starlarkjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
)

// Script builtins have no filesystem, network, provider or product access.
// JSON and RE2 processing remain bounded by the Script input/output limits.
var scriptBuiltins = starlark.StringDict{
	"json":          starlarkjson.Module,
	"regex_find":    starlark.NewBuiltin("regex_find", scriptRegexFind),
	"regex_replace": starlark.NewBuiltin("regex_replace", scriptRegexReplace),
	"now_millis": starlark.NewBuiltin("now_millis", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if err := starlark.UnpackArgs("now_millis", args, kwargs); err != nil {
			return nil, err
		}
		return starlark.MakeInt64(time.Now().UnixMilli()), nil
	}),
}

func scriptRegexp(pattern starlark.Tuple) (*regexp.Regexp, bool, error) {
	if len(pattern) != 2 {
		return nil, false, fmt.Errorf("regex pattern must be a (source, flags) tuple")
	}
	source, sourceOK := starlark.AsString(pattern[0])
	flags, flagsOK := starlark.AsString(pattern[1])
	if !sourceOK || !flagsOK {
		return nil, false, fmt.Errorf("regex source and flags must be strings")
	}
	for _, flag := range flags {
		switch flag {
		case 'i', 'm', 's':
			source = "(?" + string(flag) + ")" + source
		case 'g', 'u':
			// RE2 processes UTF-8; g selects all matches below.
		default:
			return nil, false, fmt.Errorf("unsupported regex flag %q", flag)
		}
	}
	compiled, err := regexp.Compile(source)
	return compiled, strings.Contains(flags, "g"), err
}

func scriptRegexFind(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var text string
	var pattern starlark.Tuple
	if err := starlark.UnpackArgs("regex_find", args, kwargs, "text", &text, "pattern", &pattern); err != nil {
		return nil, err
	}
	compiled, all, err := scriptRegexp(pattern)
	if err != nil {
		return nil, err
	}
	matches := compiled.FindStringSubmatch(text)
	if all {
		matches = compiled.FindAllString(text, -1)
	}
	if matches == nil {
		return starlark.None, nil
	}
	values := make([]starlark.Value, len(matches))
	for index, value := range matches {
		values[index] = starlark.String(value)
	}
	return starlark.NewList(values), nil
}

func scriptRegexReplace(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var text, replacement string
	var pattern starlark.Tuple
	if err := starlark.UnpackArgs("regex_replace", args, kwargs, "text", &text, "pattern", &pattern, "replacement", &replacement); err != nil {
		return nil, err
	}
	compiled, all, err := scriptRegexp(pattern)
	if err != nil {
		return nil, err
	}
	if all {
		return starlark.String(compiled.ReplaceAllString(text, replacement)), nil
	}
	indices := compiled.FindStringSubmatchIndex(text)
	if indices == nil {
		return starlark.String(text), nil
	}
	result := compiled.ExpandString(nil, replacement, text, indices)
	return starlark.String(text[:indices[0]] + string(result) + text[indices[1]:]), nil
}
