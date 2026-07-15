package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	entrySpecRel = "api/http/shared.json"
	outDirRel    = "pkgs/gizclaw/api/apitypes"

	resolvedTypesFile = "types_resolved.json"
	configFile        = "codegen_config.yaml"
	codegenFile       = "codegen.go"

	resolveArg = "-resolve"
)

type generator struct {
	root string

	visitedFiles map[string]bool
	schemas      map[string]any
	schemaSource map[string]string
	entryAliases map[string]string
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == resolveArg {
		if err := resolveTypes(); err != nil {
			fmt.Fprintf(os.Stderr, "apitypesgen: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 {
		fmt.Fprintf(os.Stderr, "apitypesgen: unknown argument %q\n", os.Args[1])
		os.Exit(1)
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "apitypesgen: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := findRepoRoot()
	if err != nil {
		return err
	}

	g := &generator{
		root:         root,
		visitedFiles: map[string]bool{},
		schemas:      map[string]any{},
		schemaSource: map[string]string{},
		entryAliases: map[string]string{},
	}

	entrySpec := filepath.Join(root, entrySpecRel)
	if err := g.processEntrySpec(entrySpec); err != nil {
		return err
	}

	outDir := filepath.Join(root, outDirRel)
	if err := writeIfChanged(filepath.Join(outDir, configFile), []byte(codegenConfig())); err != nil {
		return err
	}
	if err := writeIfChanged(filepath.Join(outDir, codegenFile), []byte(codegenGo())); err != nil {
		return err
	}

	return nil
}

func resolveTypes() error {
	root, err := findRepoRoot()
	if err != nil {
		return err
	}

	g := &generator{
		root:         root,
		visitedFiles: map[string]bool{},
		schemas:      map[string]any{},
		schemaSource: map[string]string{},
		entryAliases: map[string]string{},
	}
	if err := g.processEntrySpec(filepath.Join(root, entrySpecRel)); err != nil {
		return err
	}

	bundle, err := g.bundleSpec()
	if err != nil {
		return err
	}

	return writeIfChanged(filepath.Join(root, outDirRel, resolvedTypesFile), bundle)
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, entrySpecRel)) {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find repository root")
		}
		dir = parent
	}
}

func (g *generator) processEntrySpec(path string) error {
	doc, err := readJSON(path)
	if err != nil {
		return err
	}

	schemas, err := componentSchemas(doc)
	if err != nil {
		return fmt.Errorf("%s: %w", rel(g.root, path), err)
	}

	for _, name := range sortedKeys(schemas) {
		schema := schemas[name]
		ref, ok := topLevelRef(schema)
		if !ok {
			rewritten, err := g.rewriteRefs(path, schema)
			if err != nil {
				return err
			}
			if err := g.addSchema(name, rewritten, path); err != nil {
				return err
			}
			continue
		}

		targetFile, targetName, err := g.resolveComponentRef(path, ref)
		if err != nil {
			return fmt.Errorf("%s schema %q: %w", rel(g.root, path), name, err)
		}
		if err := g.processSchemaFile(targetFile); err != nil {
			return err
		}

		if targetName != name {
			g.entryAliases[name] = targetName
		}
	}

	return nil
}

func (g *generator) processSchemaFile(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if g.visitedFiles[abs] {
		return nil
	}
	g.visitedFiles[abs] = true

	doc, err := readJSON(abs)
	if err != nil {
		return err
	}

	schemas, err := componentSchemas(doc)
	if err != nil {
		return fmt.Errorf("%s: %w", rel(g.root, abs), err)
	}

	for _, name := range sortedKeys(schemas) {
		rewritten, err := g.rewriteRefs(abs, schemas[name])
		if err != nil {
			return err
		}
		if err := g.addSchema(name, rewritten, abs); err != nil {
			return err
		}
	}

	return nil
}

func (g *generator) addSchema(name string, schema any, source string) error {
	if existing, ok := g.schemas[name]; ok {
		if jsonEqual(existing, schema) {
			return nil
		}
		return fmt.Errorf("schema %q is defined differently in %s and %s", name, rel(g.root, g.schemaSource[name]), rel(g.root, source))
	}

	g.schemas[name] = schema
	g.schemaSource[name] = source
	return nil
}

func (g *generator) rewriteRefs(currentFile string, value any) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			if key == "$ref" {
				ref, ok := child.(string)
				if !ok {
					return nil, fmt.Errorf("%s contains a non-string $ref", rel(g.root, currentFile))
				}
				rewritten, err := g.rewriteRef(currentFile, ref)
				if err != nil {
					return nil, err
				}
				out[key] = rewritten
				continue
			}

			rewritten, err := g.rewriteRefs(currentFile, child)
			if err != nil {
				return nil, err
			}
			out[key] = rewritten
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			rewritten, err := g.rewriteRefs(currentFile, child)
			if err != nil {
				return nil, err
			}
			out[i] = rewritten
		}
		return out, nil
	default:
		return value, nil
	}
}

func (g *generator) rewriteRef(currentFile, ref string) (string, error) {
	if strings.HasPrefix(ref, "#/") {
		return ref, nil
	}
	if strings.Contains(ref, "://") {
		return "", fmt.Errorf("unsupported remote $ref %q in %s", ref, rel(g.root, currentFile))
	}

	targetFile, targetName, err := g.resolveComponentRef(currentFile, ref)
	if err != nil {
		return "", err
	}
	if err := g.processSchemaFile(targetFile); err != nil {
		return "", err
	}

	return "#/components/schemas/" + targetName, nil
}

func (g *generator) resolveComponentRef(currentFile, ref string) (string, string, error) {
	filePart, fragment, ok := strings.Cut(ref, "#")
	if !ok {
		return "", "", fmt.Errorf("unsupported $ref without fragment %q in %s", ref, rel(g.root, currentFile))
	}

	targetName, err := componentSchemaName(fragment)
	if err != nil {
		return "", "", fmt.Errorf("%s in %s: %w", ref, rel(g.root, currentFile), err)
	}

	targetFile := currentFile
	if filePart != "" {
		targetFile = filepath.Join(filepath.Dir(currentFile), filepath.FromSlash(filePart))
	}
	targetFile, err = filepath.Abs(targetFile)
	if err != nil {
		return "", "", err
	}

	return targetFile, targetName, nil
}

func (g *generator) bundleSpec() ([]byte, error) {
	schemas := make(map[string]any, len(g.schemas)+len(g.entryAliases))
	for name, schema := range g.schemas {
		schemas[name] = schema
	}
	for alias, target := range g.entryAliases {
		if _, ok := schemas[target]; !ok {
			return nil, fmt.Errorf("entry alias %q points at missing schema %q", alias, target)
		}
		schemas[alias] = map[string]any{
			"$ref": "#/components/schemas/" + target,
		}
	}

	bundle := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "Shared API Types Codegen Bundle",
			"description": "Generated by go run ./internal/apitypesgen from api/http/shared.json. DO NOT EDIT.",
			"version":     "1.0.0",
		},
		"paths": map[string]any{},
		"components": map[string]any{
			"schemas": schemas,
		},
	}

	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func componentSchemas(doc map[string]any) (map[string]any, error) {
	components, ok := doc["components"].(map[string]any)
	if !ok {
		return nil, errors.New("missing components object")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		return nil, errors.New("missing components.schemas object")
	}
	return schemas, nil
}

func componentSchemaName(fragment string) (string, error) {
	const prefix = "/components/schemas/"
	if !strings.HasPrefix(fragment, prefix) {
		return "", fmt.Errorf("fragment must point to components.schemas")
	}
	name := strings.TrimPrefix(fragment, prefix)
	if name == "" || strings.Contains(name, "/") {
		return "", fmt.Errorf("invalid schema fragment")
	}
	return name, nil
}

func topLevelRef(schema any) (string, bool) {
	object, ok := schema.(map[string]any)
	if !ok || len(object) != 1 {
		return "", false
	}
	ref, ok := object["$ref"].(string)
	return ref, ok
}

func readJSON(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc, nil
}

func writeIfChanged(path string, data []byte) error {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, data) {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", path)
	return nil
}

func codegenConfig() string {
	return `# Code generated by go run ./pkgs/gizclaw/api/apitypes/internal/apitypesgen; DO NOT EDIT.
# Source: api/http/shared.json and every JSON schema reachable from its $ref graph.
# To refresh:
#   1. go run ./pkgs/gizclaw/api/apitypes/internal/apitypesgen
#   2. go generate ./pkgs/gizclaw/api/apitypes
package: apitypes
generate:
  models: true
output-options:
  skip-prune: true
compatibility:
  always-prefix-enum-values: true
`
}

func codegenGo() string {
	return `package apitypes

// Code generated by go run ./internal/apitypesgen; DO NOT EDIT.

` + "//go:generate go run ./internal/apitypesgen -resolve\n" +
		"//go:generate go tool oapi-codegen -config=codegen_config.yaml -o generated.go types_resolved.json\n"
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func jsonEqual(a, b any) bool {
	aJSON, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bJSON, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return bytes.Equal(aJSON, bJSON)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func rel(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}
