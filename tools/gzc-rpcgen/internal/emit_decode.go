package rpcgen

import (
	"bytes"
	"fmt"
)

func emitDecodeH(model Model) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(cHeader())
	b.WriteString("#ifndef GZC_RPC_DECODE_H\n#define GZC_RPC_DECODE_H\n\n")
	b.WriteString("#include \"gzc_rpc_types.h\"\n\n")
	b.WriteString("#ifdef __cplusplus\nextern \"C\" {\n#endif\n\n")
	for _, schema := range uniqueSchemas(model, func(m Method) Schema { return m.Response }) {
		fmt.Fprintf(&b, "int %s(gzc_str_t json, %s *out_value);\n", decodeFuncName(model.Package, schema.Name), typeName(model.Package, schema.Name))
	}
	b.WriteString("\n#ifdef __cplusplus\n}\n#endif\n\n")
	b.WriteString("#endif\n")
	return b.Bytes(), nil
}

func emitDecodeC(model Model) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(cHeader())
	b.WriteString("#include \"gzc_rpc_decode.h\"\n\n")
	b.WriteString("#include <string.h>\n\n")
	for _, schema := range uniqueSchemas(model, func(m Method) Schema { return m.Response }) {
		fmt.Fprintf(&b, "int %s(gzc_str_t json, %s *out_value) {\n", decodeFuncName(model.Package, schema.Name), typeName(model.Package, schema.Name))
		b.WriteString("  if (out_value == NULL) {\n    return GZC_ERR_INVALID_ARGUMENT;\n  }\n")
		b.WriteString("  memset(out_value, 0, sizeof(*out_value));\n")
		if len(schema.Fields) == 0 {
			b.WriteString("  (void)json;\n")
			b.WriteString("  return GZC_OK;\n")
			b.WriteString("}\n\n")
			continue
		}
		b.WriteString("  gzc_str_t raw;\n  int rc;\n")
		for _, field := range schema.Fields {
			fmt.Fprintf(&b, "  rc = gzc_json_find_field(json, \"%s\", &raw);\n", field.JSONName)
			if field.Required {
				b.WriteString("  if (rc != GZC_OK) { return rc; }\n")
			} else {
				b.WriteString("  if (rc == GZC_OK) {\n")
			}
			prefix := "  "
			if !field.Required {
				prefix = "    "
				fmt.Fprintf(&b, "%sout_value->has_%s = true;\n", prefix, field.CName)
			}
			fmt.Fprintf(&b, "%src = %s;\n", prefix, decodeFieldExpr(field))
			fmt.Fprintf(&b, "%sif (rc != GZC_OK) { return rc; }\n", prefix)
			if !field.Required {
				b.WriteString("  }\n")
			}
		}
		b.WriteString("  return GZC_OK;\n")
		b.WriteString("}\n\n")
	}
	return b.Bytes(), nil
}

func decodeFieldExpr(field Field) string {
	switch field.Type.Kind {
	case CTypeString:
		return fmt.Sprintf("gzc_json_parse_string(raw, &out_value->%s)", field.CName)
	case CTypeBool:
		return fmt.Sprintf("gzc_json_parse_bool(raw, &out_value->%s)", field.CName)
	case CTypeI32:
		return fmt.Sprintf("gzc_json_parse_i32(raw, &out_value->%s)", field.CName)
	case CTypeI64:
		return fmt.Sprintf("gzc_json_parse_i64(raw, &out_value->%s)", field.CName)
	case CTypeF64:
		return fmt.Sprintf("gzc_json_parse_f64(raw, &out_value->%s)", field.CName)
	default:
		return fmt.Sprintf("(out_value->%s.raw = raw, GZC_OK)", field.CName)
	}
}
