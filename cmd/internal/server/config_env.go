package server

import (
	"os"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
)

// parseExpandedConfig parses a Server config document and expands environment
// references in every scalar value exactly once, before shape validation and
// decoding. Mapping keys, anchor names, and aliases are never expanded. It
// returns nil for an empty document.
func parseExpandedConfig(data []byte) (ast.Node, error) {
	file, err := parser.ParseBytes(data, 0)
	if err != nil {
		return nil, err
	}
	if len(file.Docs) == 0 || file.Docs[0] == nil {
		return nil, nil
	}
	return expandConfigNode(file.Docs[0].Body), nil
}

// decodeConfigNode decodes an expanded config document; a nil document leaves
// v unchanged, matching yaml.Unmarshal on empty input.
func decodeConfigNode(node ast.Node, v any, opts ...yaml.DecodeOption) error {
	if node == nil {
		return nil
	}
	if _, comment := node.(*ast.CommentGroupNode); comment {
		return nil
	}
	return yaml.NodeToValue(node, v, opts...)
}

func expandConfigNode(node ast.Node) ast.Node {
	switch n := node.(type) {
	case *ast.MappingNode:
		for _, value := range n.Values {
			value.Value = expandConfigNode(value.Value)
		}
	case *ast.MappingValueNode:
		n.Value = expandConfigNode(n.Value)
	case *ast.SequenceNode:
		for index, value := range n.Values {
			n.Values[index] = expandConfigNode(value)
			if index < len(n.Entries) && n.Entries[index] != nil {
				n.Entries[index].Value = n.Values[index]
			}
		}
	case *ast.AnchorNode:
		n.Value = expandConfigNode(n.Value)
	case *ast.TagNode:
		// An explicit tag fixes the scalar type, so only the text is expanded.
		if value, ok := n.Value.(*ast.StringNode); ok {
			expandConfigString(value, false)
		} else {
			n.Value = expandConfigNode(n.Value)
		}
	case *ast.LiteralNode:
		// Block scalars decode through their value; their multi-line source
		// text is left as written.
		if n.Value != nil {
			n.Value.Value = expandConfigEnv(n.Value.Value)
			if n.Value.Token != nil {
				n.Value.Token.Value = n.Value.Value
			}
		}
	case *ast.StringNode:
		return expandConfigString(n, true)
	}
	return node
}

// expandConfigString expands one string scalar in place, rewriting its token
// so the node reads as if the expanded text had been written as a
// double-quoted string. With retype, an unquoted reference that expands to a
// YAML boolean becomes a boolean so `enabled: ${FLAG}` decodes into bool
// fields. Other values stay strings: the decoder converts them for numeric
// fields, and string fields keep text such as "007" verbatim.
func expandConfigString(node *ast.StringNode, retype bool) ast.Node {
	expanded := expandConfigEnv(node.Value)
	if expanded == node.Value || node.Token == nil {
		return node
	}
	tk := node.Token
	plain := tk.Type == token.StringType
	leading, trailing := originPadding(tk.Origin)
	tk.Value = expanded
	node.Value = expanded
	if retype && plain && token.New(expanded, expanded, tk.Position).Type == token.BoolType {
		tk.Type = token.BoolType
		tk.Origin = leading + expanded + trailing
		boolean := ast.Bool(tk)
		boolean.BaseNode = node.BaseNode
		return boolean
	}
	tk.Type = token.DoubleQuoteType
	tk.CharacterType = token.CharacterTypeIndicator
	tk.Indicator = token.QuotedScalarIndicator
	tk.Origin = leading + strconv.Quote(expanded) + trailing
	return node
}

// originPadding returns the whitespace around a scalar token's source text.
func originPadding(origin string) (string, string) {
	const space = " \t\r\n"
	trimmed := strings.TrimLeft(origin, space)
	leading := origin[:len(origin)-len(trimmed)]
	body := strings.TrimRight(trimmed, space)
	return leading, trimmed[len(body):]
}

// expandConfigEnv applies os.ExpandEnv semantics, except that "$$" produces a
// literal "$" so values such as passwords can carry dollar signs.
func expandConfigEnv(value string) string {
	return os.Expand(value, func(name string) string {
		if name == "$" {
			return "$"
		}
		return os.Getenv(name)
	})
}
