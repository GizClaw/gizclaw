package match

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"strconv"
	"strings"
	"text/template"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/generators"

	_ "embed"
)

//go:embed default.gotmpl
var defaultPromptTpl string

type Option func(*compileConfig)

type compileConfig struct {
	tpl string
}

// WithTpl sets a custom prompt template.
func WithTpl(tpl string) Option {
	return func(c *compileConfig) {
		c.tpl = tpl
	}
}

// Matcher is a compiled matcher built from rules.
type Matcher struct {
	systemPrompt string
	specs        map[string]map[string]Var
}

// SystemPrompt returns the rendered system prompt for debugging.
func (m *Matcher) SystemPrompt() string {
	return m.systemPrompt
}

// Arg holds a matched argument's value along with its definition.
type Arg struct {
	Value    any
	Var      Var
	HasValue bool
}

// Result is the structured output from a single match.
type Result struct {
	Rule    string
	Args    map[string]Arg
	RawText string
}

type MatchOption func(*matchConfig)

type matchConfig struct {
	gen genx.Generator
}

// WithGenerator sets a custom generator for Match.
func WithGenerator(gen genx.Generator) MatchOption {
	return func(c *matchConfig) {
		c.gen = gen
	}
}

// Match executes the matcher against user input and returns streaming results.
func (m *Matcher) Match(ctx context.Context, pattern string, mc genx.ModelContext, opts ...MatchOption) iter.Seq2[Result, error] {
	cfg := &matchConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	mcb := &genx.ModelContextBuilder{}
	mcb.PromptText("", m.systemPrompt)
	internal := mcb.Build()

	combined := genx.ModelContexts(mc, internal)

	return func(yield func(Result, error) bool) {
		stream, err := generateMatchStream(ctx, pattern, combined, cfg.gen)
		if err != nil {
			yield(Result{}, fmt.Errorf("generate: %w", err))
			return
		}
		if stream == nil {
			yield(Result{}, fmt.Errorf("generate: returned nil stream"))
			return
		}
		defer stream.Close()

		chunks := func(yieldChunk func(string, error) bool) {
			for {
				chunk, nextErr := stream.Next()
				if nextErr != nil {
					if errors.Is(nextErr, genx.ErrDone) || errors.Is(nextErr, io.EOF) {
						return
					}
					yieldChunk("", nextErr)
					return
				}
				if chunk == nil || chunk.Part == nil {
					continue
				}
				text, ok := chunk.Part.(genx.Text)
				if ok && !yieldChunk(string(text), nil) {
					return
				}
			}
		}
		for result, parseErr := range m.Parse(chunks) {
			if !yield(result, parseErr) {
				return
			}
		}
	}
}

func generateMatchStream(
	ctx context.Context,
	pattern string,
	modelContext genx.ModelContext,
	generator genx.Generator,
) (genx.Stream, error) {
	if generator != nil {
		return generator.GenerateStream(ctx, pattern, modelContext)
	}
	return generators.GenerateStream(ctx, pattern, modelContext)
}

// Parse converts arbitrary text chunks into ordered Match results. Chunk
// boundaries do not affect line framing.
func (m *Matcher) Parse(chunks iter.Seq2[string, error]) iter.Seq2[Result, error] {
	return func(yield func(Result, error) bool) {
		pending := ""
		for chunk, err := range chunks {
			if err != nil {
				yield(Result{}, err)
				return
			}
			text := pending + chunk
			for {
				index := strings.IndexByte(text, '\n')
				if index < 0 {
					pending = text
					break
				}
				if !m.yieldLine(text[:index], yield) {
					return
				}
				text = text[index+1:]
			}
		}
		m.yieldLine(pending, yield)
	}
}

func (m *Matcher) yieldLine(line string, yield func(Result, error) bool) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	result, ok := m.parseLine(line)
	return !ok || yield(result, nil)
}

func (m *Matcher) parseLine(line string) (Result, bool) {
	name, kv, hasColon := strings.Cut(line, ":")
	name = strings.TrimSpace(name)

	if name == "" {
		return Result{RawText: line}, true
	}

	vars, ok := m.specs[name]
	if !ok {
		return Result{RawText: line}, true
	}

	var args map[string]Arg
	if hasColon {
		args = m.parseKVToArgs(strings.TrimSpace(kv), vars)
	} else {
		args = m.parseKVToArgs("", vars)
	}
	return Result{Rule: name, Args: args}, true
}

func (m *Matcher) parseKVToArgs(kv string, vars map[string]Var) map[string]Arg {
	args := make(map[string]Arg)

	for name, v := range vars {
		args[name] = Arg{Value: nil, Var: v, HasValue: false}
	}

	if strings.TrimSpace(kv) == "" {
		return args
	}

	for part := range strings.SplitSeq(kv, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" {
			continue
		}

		varDef, exists := vars[k]
		if !exists {
			continue
		}

		var typedValue any = v
		switch varDef.Type {
		case "int":
			if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
				typedValue = parsed
			}
		case "float":
			if parsed, err := strconv.ParseFloat(v, 64); err == nil {
				typedValue = parsed
			}
		case "bool":
			if parsed, err := strconv.ParseBool(v); err == nil {
				typedValue = parsed
			}
		}

		args[k] = Arg{Value: typedValue, Var: varDef, HasValue: true}
	}

	return args
}

// Collect consumes a streaming sequence into a slice.
func Collect(seq iter.Seq2[Result, error]) ([]Result, error) {
	var out []Result
	for r, err := range seq {
		if err != nil {
			return out, err
		}
		out = append(out, r)
	}
	return out, nil
}

// Project returns a JSON-compatible, defensively owned representation of
// ordered Match results.
func Project(results []Result) []any {
	projected := make([]any, len(results))
	for index, result := range results {
		args := make(map[string]any, len(result.Args))
		for name, argument := range result.Args {
			args[name] = map[string]any{
				"value": argument.Value,
				"var": map[string]any{
					"label": argument.Var.Label,
					"type":  argument.Var.Type,
				},
				"has_value": argument.HasValue,
			}
		}
		projected[index] = map[string]any{
			"rule":     result.Rule,
			"args":     args,
			"raw_text": result.RawText,
		}
	}
	return projected
}

// Compile compiles rules into a reusable Matcher.
func Compile(rules []*Rule, opts ...Option) (*Matcher, error) {
	if len(rules) == 0 {
		return nil, fmt.Errorf("match: rules are required")
	}
	cfg := &compileConfig{tpl: defaultPromptTpl}
	for _, opt := range opts {
		opt(cfg)
	}

	data, err := buildPromptData(rules)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New("prompt").Funcs(template.FuncMap{
		"inc": func(i int) int { return i + 1 },
	}).Parse(cfg.tpl)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}

	specs := make(map[string]map[string]Var, len(rules))
	for _, r := range rules {
		if _, exists := specs[r.Name]; exists {
			return nil, fmt.Errorf("match: duplicate rule name %q", r.Name)
		}
		specs[r.Name] = maps.Clone(r.Vars)
	}

	return &Matcher{systemPrompt: buf.String(), specs: specs}, nil
}

type promptData struct {
	References map[string]string
	Rules      []ruleData
}

type ruleData struct {
	Name     string
	Patterns []patternData
	Examples []Example
}

type patternData struct {
	Input  string
	Output string
}

func buildPromptData(rules []*Rule) (*promptData, error) {
	data := &promptData{References: make(map[string]string)}
	seen := make(map[string]struct{}, len(rules))
	for index, r := range rules {
		if r == nil {
			return nil, fmt.Errorf("match: rule[%d] is nil", index)
		}
		if _, duplicate := seen[r.Name]; duplicate {
			return nil, fmt.Errorf("match: duplicate rule name %q", r.Name)
		}
		seen[r.Name] = struct{}{}
		if err := r.compileTo(data); err != nil {
			return nil, err
		}
	}
	return data, nil
}
