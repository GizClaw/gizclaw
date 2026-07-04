package rpcgen

type CTypeKind int

const (
	CTypeString CTypeKind = iota
	CTypeBool
	CTypeI32
	CTypeI64
	CTypeF64
	CTypeJSON
)

type CType struct {
	Kind CTypeKind
	Name string
}

func ctypeFor(schema map[string]any) CType {
	if schema == nil {
		return CType{Kind: CTypeJSON, Name: "gzc_json_t"}
	}
	if _, ok := schema["$ref"].(string); ok {
		return CType{Kind: CTypeJSON, Name: "gzc_json_t"}
	}
	t, _ := schema["type"].(string)
	switch t {
	case "string":
		return CType{Kind: CTypeString, Name: "gzc_str_t"}
	case "boolean":
		return CType{Kind: CTypeBool, Name: "bool"}
	case "integer":
		if format, _ := schema["format"].(string); format == "int64" {
			return CType{Kind: CTypeI64, Name: "int64_t"}
		}
		return CType{Kind: CTypeI32, Name: "int32_t"}
	case "number":
		return CType{Kind: CTypeF64, Name: "double"}
	default:
		return CType{Kind: CTypeJSON, Name: "gzc_json_t"}
	}
}
