package eino

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"
)

func TestScriptRegexFindBoundsNativeResults(t *testing.T) {
	for _, test := range []struct {
		name, text, pattern string
		byteLimit           int
	}{
		{name: "zero-width match count", text: strings.Repeat("x", 100000), pattern: "", byteLimit: 1 << 20},
		{name: "result byte limit", text: strings.Repeat("x", 100), pattern: "x+", byteLimit: 16},
	} {
		t.Run(test.name, func(t *testing.T) {
			thread := &starlark.Thread{}
			thread.SetLocal(scriptNativeOutputLimitKey, test.byteLimit)
			_, err := scriptRegexFind(thread, nil, starlark.Tuple{
				starlark.String(test.text), starlark.Tuple{starlark.String(test.pattern), starlark.String("g")},
			}, nil)
			if err == nil || !strings.Contains(err.Error(), "limit") {
				t.Fatalf("unbounded regex result accepted: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	thread := &starlark.Thread{}
	thread.SetLocal(scriptNativeContextKey, ctx)
	if _, err := scriptRegexFind(thread, nil, starlark.Tuple{
		starlark.String("text"), starlark.Tuple{starlark.String(""), starlark.String("g")},
	}, nil); err != context.Canceled {
		t.Fatalf("canceled regex = %v", err)
	}
}

func TestScriptJSONAndRegexRespectInputOwnership(t *testing.T) {
	source := `def run(input):
    values = json.decode(json.encode(input["values"]))
    values["score"] += 100
    matched = regex_find(input["text"], ("badge=([a-z]+)", "i"))
    rendered = regex_replace(input["text"], ("badge", "gi"), "award")
    return {"answer": json.encode({"score": values["score"], "badge": matched[1], "rendered": rendered})}
`
	script, err := compileScript(t.Context(), ScriptNode{Language: ScriptStarlark, Source: source, Entrypoint: "run",
		Limits: ScriptLimits{MaxExecutionSteps: 10000, Timeout: time.Second, MaxInputBytes: 65536, MaxOutputBytes: 65536}})
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]any{"score": int64(0)}
	result, err := script.run(t.Context(), map[string]any{"values": values, "text": "BADGE=moon badge=river"}, map[string]StateType{"answer": StateString})
	if err != nil {
		t.Fatal(err)
	}
	answer := result["answer"].(string)
	if values["score"] != int64(0) || !strings.Contains(answer, `"score":100`) ||
		!strings.Contains(answer, `"badge":"moon"`) || !strings.Contains(answer, "award=moon award=river") {
		t.Fatalf("answer=%s, caller values=%v", answer, values)
	}
}

func TestScriptRegexRejectsUnsupportedPatternsAndFlags(t *testing.T) {
	for _, pattern := range []string{`("(?=secret)", "")`, `("badge", "y")`} {
		script, err := compileScript(t.Context(), ScriptNode{Language: ScriptStarlark,
			Source: "def run(input):\n    return {\"answer\": regex_find(\"text\", " + pattern + ")}\n",
			Limits: ScriptLimits{MaxExecutionSteps: 10000, Timeout: time.Second, MaxInputBytes: 65536, MaxOutputBytes: 65536}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := script.run(t.Context(), map[string]any{}, map[string]StateType{"answer": StateString}); err == nil {
			t.Fatalf("unsupported regex accepted: %s", pattern)
		}
	}
}

func TestScriptRegexReplaceBoundsNativeExpansion(t *testing.T) {
	for _, test := range []struct {
		name, text, pattern, flags, replacement string
		limit                                   int
	}{
		{"global expansion", "aaa", "a", "g", strings.Repeat("x", 100000), 1024},
		{"single capture expansion", strings.Repeat("x", 1000), "(x+)", "", strings.Repeat("$1", 1000), 1024},
		{"unmatched suffix", "abc", "b", "", "X", 2},
		{"zero-width matches", strings.Repeat("x", 100000), "", "g", "", 1 << 20},
	} {
		t.Run(test.name, func(t *testing.T) {
			thread := &starlark.Thread{}
			thread.SetLocal(scriptNativeOutputLimitKey, test.limit)
			_, err := scriptRegexReplace(thread, nil, starlark.Tuple{starlark.String(test.text), starlark.Tuple{starlark.String(test.pattern), starlark.String(test.flags)}, starlark.String(test.replacement)}, nil)
			if err == nil || !strings.Contains(err.Error(), "limit") {
				t.Fatalf("unbounded replacement accepted: %v", err)
			}
		})
	}
}

func TestScriptRegexReplaceMatchesRE2Expansion(t *testing.T) {
	for _, test := range []struct{ text, pattern, replacement string }{
		{"a ab aaa", "a*", "<$0>"},
		{"ab ab", "(?P<word>a)(b)?", "${word}:$2:$$:$missing:$1x:$?:${}"},
		{"b a", "(?P<same>a)|(?P<same>b)", "${same}"},
		{"中文", "", "-"},
		{"foo", "(foo)", "$01:$1:$1234567890:${1}:$"},
	} {
		for _, all := range []bool{false, true} {
			thread := &starlark.Thread{}
			flags := ""
			if all {
				flags = "g"
			}
			result, err := scriptRegexReplace(thread, nil, starlark.Tuple{starlark.String(test.text), starlark.Tuple{starlark.String(test.pattern), starlark.String(flags)}, starlark.String(test.replacement)}, nil)
			if err != nil {
				t.Fatal(err)
			}
			compiled := regexp.MustCompile(test.pattern)
			want := compiled.ReplaceAllString(test.text, test.replacement)
			if !all {
				match := compiled.FindStringSubmatchIndex(test.text)
				want = test.text
				if match != nil {
					want = test.text[:match[0]] + string(compiled.ExpandString(nil, test.replacement, test.text, match)) + test.text[match[1]:]
				}
			}
			got, _ := starlark.AsString(result)
			if got != want {
				t.Fatalf("text=%q pattern=%q replacement=%q all=%v got=%q want=%q", test.text, test.pattern, test.replacement, all, got, want)
			}
		}
	}
}

type cancelRegexAfterChecks struct {
	context.Context
	checks int
}

func (c *cancelRegexAfterChecks) Err() error {
	c.checks++
	if c.checks >= 8 {
		return context.Canceled
	}
	return nil
}

func TestScriptRegexReplaceChecksCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, nativeCtx := range []context.Context{ctx, &cancelRegexAfterChecks{Context: t.Context()}} {
		thread := &starlark.Thread{}
		thread.SetLocal(scriptNativeContextKey, nativeCtx)
		_, err := scriptRegexReplace(thread, nil, starlark.Tuple{starlark.String("abc abc abc"), starlark.Tuple{starlark.String("(abc)"), starlark.String("g")}, starlark.String("$1-$1")}, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("replacement did not observe cancellation: %v", err)
		}
	}
}
