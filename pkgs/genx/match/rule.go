package match

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"strings"
)

// Var defines a variable to be extracted from user input.
type Var struct {
	Label string `json:"label" yaml:"label"`
	Type  string `json:"type" yaml:"type"`
}

// Example is a structured grounding example for the prompt.
type Example struct {
	Subject     string `json:"-"`
	UserText    string `json:"-"`
	FormattedTo string `json:"-"`
}

func (e Example) MarshalJSON() ([]byte, error) {
	if e.FormattedTo != "" {
		if e.UserText == "" {
			return nil, fmt.Errorf("invalid example: FormattedTo is set but UserText is empty")
		}
		return json.Marshal([]string{e.Subject, e.UserText, e.FormattedTo})
	}
	if e.UserText != "" {
		return json.Marshal([]string{e.Subject, e.UserText})
	}
	return json.Marshal([]string{e.Subject})
}

func (e *Example) UnmarshalJSON(b []byte) error {
	if e == nil {
		return fmt.Errorf("example is nil")
	}
	var arr []string
	if err := json.Unmarshal(b, &arr); err != nil {
		return fmt.Errorf("example must be an array: %w", err)
	}
	if len(arr) < 1 || len(arr) > 3 {
		return fmt.Errorf("invalid example array length: %d (expected 1-3)", len(arr))
	}
	e.Subject = arr[0]
	if len(arr) >= 2 {
		e.UserText = arr[1]
	}
	if len(arr) == 3 {
		e.FormattedTo = arr[2]
	}
	return nil
}

// Pattern is a single input/output pattern example for one rule.
type Pattern struct {
	Input  string `json:"-"`
	Output string `json:"-"`
}

func (p Pattern) MarshalJSON() ([]byte, error) {
	if p.Output == "" {
		return json.Marshal(p.Input)
	}
	return json.Marshal([]string{p.Input, p.Output})
}

func (p *Pattern) UnmarshalJSON(b []byte) error {
	if p == nil {
		return fmt.Errorf("pattern is nil")
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		p.Input = s
		p.Output = ""
		return nil
	}
	var arr []string
	if err := json.Unmarshal(b, &arr); err == nil {
		if len(arr) < 1 || len(arr) > 2 {
			return fmt.Errorf("invalid pattern array length: %d", len(arr))
		}
		p.Input = arr[0]
		if len(arr) == 2 {
			p.Output = arr[1]
		} else {
			p.Output = ""
		}
		return nil
	}
	return fmt.Errorf("unsupported pattern json")
}

// Rule is a schema-driven rule definition.
type Rule struct {
	Name       string            `json:"name" yaml:"name"`
	References map[string]string `json:"references,omitempty" yaml:"references,omitempty"`
	Vars       map[string]Var    `json:"vars,omitempty" yaml:"vars,omitempty"`
	Patterns   []Pattern         `json:"patterns,omitempty" yaml:"patterns,omitempty"`
	Examples   []Example         `json:"examples,omitempty" yaml:"examples,omitempty"`
}

var validVarTypes = map[string]struct{}{
	"string": {},
	"int":    {},
	"float":  {},
	"bool":   {},
}

var (
	placeholderRe = regexp.MustCompile(`\[([A-Za-z_][A-Za-z0-9_]*)\]`)
	varNameRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func (r *Rule) compileTo(data *promptData) error {
	if data == nil {
		return fmt.Errorf("rule: prompt data is nil")
	}
	if r == nil {
		return fmt.Errorf("rule is nil")
	}
	if r.Name == "" || strings.TrimSpace(r.Name) != r.Name ||
		strings.ContainsAny(r.Name, ":\r\n") {
		return fmt.Errorf("rule name %q is invalid", r.Name)
	}

	for name, v := range r.Vars {
		if !varNameRe.MatchString(name) {
			return fmt.Errorf("rule %q: var name %q is invalid", r.Name, name)
		}
		if v.Type != "" {
			if _, ok := validVarTypes[v.Type]; !ok {
				return fmt.Errorf("rule %q: var %q has invalid type %q (expected string|int|float|bool)", r.Name, name, v.Type)
			}
		}
		if strings.TrimSpace(v.Label) != v.Label {
			return fmt.Errorf("rule %q: var %q label must be trimmed", r.Name, name)
		}
		if strings.ContainsAny(v.Label, "[]") {
			return fmt.Errorf("rule %q: var %q label must not contain '[' or ']'", r.Name, name)
		}
	}

	for i, p := range r.Patterns {
		if strings.ContainsAny(p.Input, "\r\n") {
			return fmt.Errorf("rule %q: pattern[%d] input contains newline", r.Name, i)
		}
		if strings.ContainsAny(p.Output, "\r\n") {
			return fmt.Errorf("rule %q: pattern[%d] output contains newline", r.Name, i)
		}
		matches := placeholderRe.FindAllStringSubmatch(p.Input, -1)
		if strings.ContainsAny(placeholderRe.ReplaceAllString(p.Input, ""), "[]") {
			return fmt.Errorf("rule %q: pattern[%d] has malformed placeholder", r.Name, i)
		}
		for _, m := range matches {
			varName := m[1]
			if _, ok := r.Vars[varName]; !ok {
				return fmt.Errorf("rule %q: pattern[%d] has placeholder [%s] not defined in vars", r.Name, i, varName)
			}
		}
	}
	for i, example := range r.Examples {
		if strings.TrimSpace(example.Subject) == "" {
			return fmt.Errorf("rule %q: example[%d] requires subject", r.Name, i)
		}
		if strings.ContainsAny(example.Subject, "\r\n") ||
			strings.ContainsAny(example.UserText, "\r\n") ||
			strings.ContainsAny(example.FormattedTo, "\r\n") {
			return fmt.Errorf("rule %q: example[%d] contains newline", r.Name, i)
		}
		if example.FormattedTo != "" && example.UserText == "" {
			return fmt.Errorf("rule %q: example[%d] has output without user text", r.Name, i)
		}
	}
	for name, value := range r.References {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name ||
			strings.TrimSpace(value) == "" || strings.ContainsAny(name+value, "\r\n") {
			return fmt.Errorf("rule %q: reference %q is invalid", r.Name, name)
		}
	}

	maps.Copy(data.References, r.References)

	rd := ruleData{Name: r.Name, Examples: r.Examples}

	for _, p := range r.Patterns {
		pd := patternData{Input: p.Input, Output: p.Output}
		if pd.Output == "" {
			pd.Input, pd.Output = expandPattern(r.Name, p.Input, r.Vars)
		}
		rd.Patterns = append(rd.Patterns, pd)
	}

	data.Rules = append(data.Rules, rd)
	return nil
}

func expandPattern(ruleName, input string, vars map[string]Var) (string, string) {
	if input == "" {
		return "", ruleName
	}

	var outParts []string

	expanded := placeholderRe.ReplaceAllStringFunc(input, func(match string) string {
		varName := match[1 : len(match)-1]
		v, ok := vars[varName]
		if !ok || v.Label == "" {
			return match
		}
		label := "[" + v.Label + "]"
		outParts = append(outParts, varName+"="+label)
		return label
	})

	if len(outParts) == 0 {
		return expanded, ruleName
	}
	return expanded, ruleName + ": " + strings.Join(outParts, ", ")
}
