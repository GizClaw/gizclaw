package volctlsfixture

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/scanner"
	"unicode"
)

type predicate func(map[string]any) bool

type parser struct {
	scanner scanner.Scanner
	token   rune
	text    string
	err     error
}

func parseQuery(expression string) (predicate, error) {
	p := &parser{}
	p.scanner.Init(strings.NewReader(expression))
	p.scanner.Mode = scanner.ScanIdents | scanner.ScanStrings
	p.scanner.IsIdentRune = func(c rune, i int) bool {
		return unicode.IsLetter(c) || c == '_' || i > 0 && (unicode.IsDigit(c) || c == '.' || c == '-')
	}
	p.scanner.Error = func(_ *scanner.Scanner, message string) { p.err = fmt.Errorf("invalid query: %s", message) }
	p.next()
	result := p.or()
	if p.err != nil {
		return nil, p.err
	}
	if p.token != scanner.EOF {
		return nil, fmt.Errorf("unsupported query token %q", p.text)
	}
	return result, nil
}

func (p *parser) next() { p.token = p.scanner.Scan(); p.text = p.scanner.TokenText() }

func (p *parser) or() predicate {
	left := p.and()
	for p.text == "OR" {
		p.next()
		previous, right := left, p.and()
		left = func(row map[string]any) bool { return previous(row) || right(row) }
	}
	return left
}

func (p *parser) and() predicate {
	left := p.atom()
	for p.text == "AND" {
		p.next()
		previous, right := left, p.atom()
		left = func(row map[string]any) bool { return previous(row) && right(row) }
	}
	return left
}

func (p *parser) atom() predicate {
	if p.text == "NOT" {
		p.next()
		child := p.atom()
		return func(row map[string]any) bool { return !child(row) }
	}
	if p.token == '(' {
		p.next()
		child := p.or()
		if p.token != ')' {
			p.err = fmt.Errorf("missing query parenthesis")
		} else {
			p.next()
		}
		return child
	}
	if p.token == '*' {
		p.next()
		return func(map[string]any) bool { return true }
	}
	if p.token != scanner.Ident {
		p.err = fmt.Errorf("expected query field, got %q", p.text)
		return func(map[string]any) bool { return false }
	}
	field := p.text
	p.next()
	if p.token != ':' {
		p.err = fmt.Errorf("missing query colon")
		return func(map[string]any) bool { return false }
	}
	p.next()
	phrase := p.token == '#'
	if phrase {
		p.next()
	}
	if p.token == '*' {
		p.next()
		return func(row map[string]any) bool { _, ok := fieldValue(row, field); return ok }
	}
	if p.token != scanner.String {
		p.err = fmt.Errorf("expected quoted query value")
		return func(map[string]any) bool { return false }
	}
	value, err := strconv.Unquote(p.text)
	if err != nil {
		p.err = err
	}
	p.next()
	return func(row map[string]any) bool {
		got, ok := fieldValue(row, field)
		if !ok {
			return false
		}
		if phrase {
			return strings.Contains(got, value)
		}
		return got == value
	}
}

func fieldValue(row map[string]any, field string) (string, bool) {
	if strings.HasPrefix(field, "attributes.") {
		var nested any
		if text, ok := row["attributes"].(string); !ok || json.Unmarshal([]byte(text), &nested) != nil {
			return "", false
		}
		for part := range strings.SplitSeq(strings.TrimPrefix(field, "attributes."), ".") {
			object, ok := nested.(map[string]any)
			if !ok {
				return "", false
			}
			nested, ok = object[part]
			if !ok {
				return "", false
			}
		}
		value, ok := nested.(string)
		return value, ok
	}
	value, ok := row[field].(string)
	return value, ok
}
