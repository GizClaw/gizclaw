package rpcgen

import (
	"strings"
	"unicode"
)

func snakeIdent(s string) string {
	var out []rune
	prevUnderscore := false
	for i, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if unicode.IsUpper(r) && i > 0 && !prevUnderscore {
				out = append(out, '_')
			}
			out = append(out, unicode.ToLower(r))
			prevUnderscore = false
			continue
		}
		if !prevUnderscore {
			out = append(out, '_')
			prevUnderscore = true
		}
	}
	got := strings.Trim(string(out), "_")
	if got == "" {
		return "value"
	}
	return got
}

func upperIdent(s string) string {
	return strings.ToUpper(snakeIdent(s))
}

func exportedToSnake(s string) string {
	return snakeIdent(s)
}

func methodConstName(pkg, method string) string {
	return strings.ToUpper(pkg) + "_RPC_METHOD_" + upperIdent(method)
}

func typeName(pkg, schemaName string) string {
	return pkg + "_" + exportedToSnake(schemaName) + "_t"
}

func encodeFuncName(pkg, schemaName string) string {
	return pkg + "_" + exportedToSnake(schemaName) + "_encode_json"
}

func decodeFuncName(pkg, schemaName string) string {
	return pkg + "_" + exportedToSnake(schemaName) + "_decode_json"
}
