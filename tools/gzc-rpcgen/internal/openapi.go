package rpcgen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type loader struct {
	docs        map[string]map[string]any
	includeDirs []string
}

func Run(cfg Config) error {
	if cfg.Package == "" {
		cfg.Package = "gzc"
	}
	if cfg.SchemaPath == "" || cfg.OutDir == "" {
		return fmt.Errorf("-schema and -out are required")
	}
	l := &loader{docs: map[string]map[string]any{}}
	for _, dir := range cfg.IncludeDirs {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		l.includeDirs = append(l.includeDirs, abs)
	}
	model, err := l.loadModel(cfg)
	if err != nil {
		return err
	}
	files, err := emitAll(model)
	if err != nil {
		return err
	}
	return writeFiles(cfg, files)
}

func (l *loader) loadModel(cfg Config) (Model, error) {
	root, err := l.loadDoc(cfg.SchemaPath)
	if err != nil {
		return Model{}, err
	}
	schemas := componentSchemas(root)
	methodSchema, ok := schemas["RPCMethod"].(map[string]any)
	if !ok {
		return Model{}, fmt.Errorf("RPCMethod schema not found")
	}
	methodValues, err := stringArray(methodSchema["enum"])
	if err != nil {
		return Model{}, fmt.Errorf("RPCMethod enum: %w", err)
	}
	reqRefs, err := oneOfRefs(schemas, "RPCRequest", "params")
	if err != nil {
		return Model{}, err
	}
	respRefs, err := oneOfRefs(schemas, "RPCResponse", "result")
	if err != nil {
		return Model{}, err
	}
	if len(methodValues) != len(reqRefs) || len(methodValues) != len(respRefs) {
		return Model{}, fmt.Errorf("method/request/response count mismatch: methods=%d requests=%d responses=%d", len(methodValues), len(reqRefs), len(respRefs))
	}
	model := Model{Package: cfg.Package}
	for i, method := range methodValues {
		reqName, reqSchema, err := l.resolveSchema(cfg.SchemaPath, reqRefs[i])
		if err != nil {
			return Model{}, fmt.Errorf("resolve request %s: %w", reqRefs[i], err)
		}
		respName, respSchema, err := l.resolveSchema(cfg.SchemaPath, respRefs[i])
		if err != nil {
			return Model{}, fmt.Errorf("resolve response %s: %w", respRefs[i], err)
		}
		model.Methods = append(model.Methods, Method{
			Index:        i,
			Value:        method,
			ConstName:    methodConstName(cfg.Package, method),
			RequestName:  reqName,
			ResponseName: respName,
			Request:      schemaFromMap(reqName, reqSchema),
			Response:     schemaFromMap(respName, respSchema),
		})
	}
	return model, nil
}

func componentSchemas(doc map[string]any) map[string]any {
	components, _ := doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	return schemas
}

func oneOfRefs(schemas map[string]any, schemaName, propertyName string) ([]string, error) {
	schema, ok := schemas[schemaName].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s schema not found", schemaName)
	}
	properties, _ := schema["properties"].(map[string]any)
	prop, ok := properties[propertyName].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s.%s schema not found", schemaName, propertyName)
	}
	oneOf, ok := prop["oneOf"].([]any)
	if !ok {
		return nil, fmt.Errorf("%s.%s oneOf not found", schemaName, propertyName)
	}
	refs := make([]string, 0, len(oneOf))
	for _, item := range oneOf {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.%s oneOf item is not object", schemaName, propertyName)
		}
		ref, ok := m["$ref"].(string)
		if !ok {
			return nil, fmt.Errorf("%s.%s oneOf item missing $ref", schemaName, propertyName)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func (l *loader) loadDoc(path string) (map[string]any, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if doc, ok := l.docs[abs]; ok {
		return doc, nil
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	l.docs[abs] = doc
	return doc, nil
}

func (l *loader) resolveSchema(fromFile, ref string) (string, map[string]any, error) {
	filePart, pointer, ok := strings.Cut(ref, "#")
	if !ok || pointer == "" {
		return "", nil, fmt.Errorf("unsupported ref %q", ref)
	}
	targetFile := fromFile
	if filePart != "" {
		targetFile = filepath.Join(filepath.Dir(fromFile), filePart)
	}
	if filePart != "" {
		resolvedTarget, err := l.resolveRefFile(targetFile, filePart)
		if err != nil {
			return "", nil, err
		}
		targetFile = resolvedTarget
	}
	doc, err := l.loadDoc(targetFile)
	if err != nil {
		return "", nil, err
	}
	parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	if len(parts) != 3 || parts[0] != "components" || parts[1] != "schemas" {
		return "", nil, fmt.Errorf("unsupported schema pointer %q", pointer)
	}
	schemas := componentSchemas(doc)
	schema, ok := schemas[parts[2]].(map[string]any)
	if !ok {
		return "", nil, fmt.Errorf("schema %s not found in %s", parts[2], targetFile)
	}
	if nested, ok := schema["$ref"].(string); ok {
		_, resolved, err := l.resolveSchema(targetFile, nested)
		if err != nil {
			return "", nil, err
		}
		return parts[2], resolved, nil
	}
	return parts[2], schema, nil
}

func (l *loader) resolveRefFile(targetFile, filePart string) (string, error) {
	if _, err := os.Stat(targetFile); err == nil {
		return targetFile, nil
	}
	cleanFilePart := strings.TrimPrefix(filepath.Clean(filePart), string(filepath.Separator))
	cleanFilePart = strings.TrimPrefix(cleanFilePart, "."+string(filepath.Separator))
	for _, dir := range l.includeDirs {
		candidate := filepath.Join(dir, cleanFilePart)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("ref file %s not found", filePart)
}

func schemaFromMap(name string, schema map[string]any) Schema {
	out := Schema{Name: name, Original: schema}
	required := map[string]bool{}
	if req, ok := schema["required"].([]any); ok {
		for _, item := range req {
			if s, ok := item.(string); ok {
				required[s] = true
			}
		}
	}
	properties, _ := schema["properties"].(map[string]any)
	for propName, propSchemaAny := range properties {
		propSchema, _ := propSchemaAny.(map[string]any)
		out.Fields = append(out.Fields, Field{
			JSONName: propName,
			CName:    snakeIdent(propName),
			Type:     ctypeFor(propSchema),
			Required: required[propName],
		})
	}
	sortFields(out.Fields)
	return out
}

func stringArray(value any) ([]string, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("not an array")
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("item is not string")
		}
		out = append(out, s)
	}
	return out, nil
}
