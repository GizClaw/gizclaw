package eino

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	starlarkjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
)

const (
	scriptNativeContextKey     = "eino.native.context"
	scriptNativeOutputLimitKey = "eino.native.output_limit"
	maxScriptRegexMatches      = 4096
)

func scriptNativeLimits(thread *starlark.Thread) (context.Context, int) {
	ctx, _ := thread.Local(scriptNativeContextKey).(context.Context)
	if ctx == nil {
		ctx = context.Background()
	}
	limit, _ := thread.Local(scriptNativeOutputLimitKey).(int)
	if limit <= 0 {
		limit = 1 << 20
	}
	return ctx, limit
}

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

func scriptRegexFind(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx, byteLimit := scriptNativeLimits(thread)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var text string
	var pattern starlark.Tuple
	if err := starlark.UnpackArgs("regex_find", args, kwargs, "text", &text, "pattern", &pattern); err != nil {
		return nil, err
	}
	compiled, all, err := scriptRegexp(pattern)
	if err != nil {
		return nil, err
	}
	var matches []string
	if all {
		// The extra match detects overflow without allocating for every input
		// position, including zero-width matches on caller-controlled text.
		limit := min(maxScriptRegexMatches, byteLimit/2)
		matches = compiled.FindAllString(text, limit+1)
		if len(matches) > limit {
			return nil, fmt.Errorf("regex match count exceeds limit %d", limit)
		}
	} else {
		if compiled.NumSubexp()+1 > maxScriptRegexMatches {
			return nil, fmt.Errorf("regex capture count exceeds limit %d", maxScriptRegexMatches)
		}
		matches = compiled.FindStringSubmatch(text)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if matches == nil {
		return starlark.None, nil
	}
	values := make([]starlark.Value, len(matches))
	resultBytes := 0
	for index, value := range matches {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resultBytes += len(value) + 2
		if resultBytes > byteLimit {
			return nil, fmt.Errorf("regex result exceeds output byte limit %d", byteLimit)
		}
		values[index] = starlark.String(value)
	}
	return starlark.NewList(values), nil
}

func scriptRegexReplace(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx, byteLimit := scriptNativeLimits(thread)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var text, replacement string
	var pattern starlark.Tuple
	if err := starlark.UnpackArgs("regex_replace", args, kwargs, "text", &text, "pattern", &pattern, "replacement", &replacement); err != nil {
		return nil, err
	}
	compiled, all, err := scriptRegexp(pattern)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	captures := compiled.NumSubexp() + 1
	// Bound both dimensions of native match-index storage before searching.
	if captures > min(maxScriptRegexMatches, max(1, byteLimit/16)) {
		return nil, fmt.Errorf("regex capture count exceeds output byte limit %d", byteLimit)
	}
	matchLimit := min(maxScriptRegexMatches, max(1, byteLimit/(16*captures)))
	count := 1
	if all {
		count = matchLimit + 1
	}
	matches := compiled.FindAllStringSubmatchIndex(text, count)
	if len(matches) > matchLimit {
		return nil, fmt.Errorf("regex match count exceeds limit %d", matchLimit)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var result strings.Builder
	appendText := func(value string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(value) > byteLimit-result.Len() {
			return fmt.Errorf("regex result exceeds output byte limit %d", byteLimit)
		}
		result.WriteString(value)
		return nil
	}
	last := 0
	names := compiled.SubexpNames()
	for _, match := range matches {
		if err := appendText(text[last:match[0]]); err != nil {
			return nil, err
		}
		template := replacement
		for template != "" {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			before, after, found := strings.Cut(template, "$")
			if err := appendText(before); err != nil {
				return nil, err
			}
			if !found {
				break
			}
			template = after
			if strings.HasPrefix(template, "$") {
				if err := appendText("$"); err != nil {
					return nil, err
				}
				template = template[1:]
				continue
			}
			name, rest, ok := scriptRegexReference(template)
			if !ok {
				if err := appendText("$"); err != nil {
					return nil, err
				}
				continue
			}
			template = rest
			capture := -1
			number, numericErr := strconv.Atoi(name)
			if numericErr == nil && len(name) <= 9 && (len(name) == 1 || name[0] != '0') {
				if number < len(match)/2 && match[2*number] >= 0 {
					capture = number
				}
			} else {
				for index, candidate := range names {
					if candidate == name && match[2*index] >= 0 {
						capture = index
						break
					}
				}
			}
			if capture >= 0 {
				if err := appendText(text[match[2*capture]:match[2*capture+1]]); err != nil {
					return nil, err
				}
			}
		}
		last = match[1]
	}
	if err := appendText(text[last:]); err != nil {
		return nil, err
	}
	return starlark.String(result.String()), nil
}

// scriptRegexReference parses the name syntax accepted by regexp.ExpandString.
func scriptRegexReference(value string) (name, rest string, ok bool) {
	braced := strings.HasPrefix(value, "{")
	if braced {
		value = value[1:]
	}
	end := 0
	for end < len(value) {
		character, size := utf8.DecodeRuneInString(value[end:])
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '_' {
			break
		}
		end += size
	}
	if end == 0 {
		return "", "", false
	}
	name = value[:end]
	if braced {
		if end == len(value) || value[end] != '}' {
			return "", "", false
		}
		end++
	}
	return name, value[end:], true
}
