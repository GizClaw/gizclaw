package rpcgen

import (
	"bytes"
	"fmt"
)

func emitEncodeH(model Model) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(cHeader())
	b.WriteString("#ifndef GZC_RPC_ENCODE_H\n#define GZC_RPC_ENCODE_H\n\n")
	b.WriteString("#include \"gzc_rpc_types.h\"\n\n")
	b.WriteString("#ifdef __cplusplus\nextern \"C\" {\n#endif\n\n")
	for _, schema := range uniqueSchemas(model, func(m Method) Schema { return m.Request }) {
		fmt.Fprintf(&b, "int %s(const gzc_platform_t *platform, const %s *value, gzc_buf_t *out_json);\n", encodeFuncName(model.Package, schema.Name), typeName(model.Package, schema.Name))
	}
	b.WriteString("\n#ifdef __cplusplus\n}\n#endif\n\n")
	b.WriteString("#endif\n")
	return b.Bytes(), nil
}

func emitEncodeC(model Model) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(cHeader())
	b.WriteString("#include \"gzc_rpc_encode.h\"\n\n")
	b.WriteString("#include \"gzc_rpc_methods.h\"\n\n")
	b.WriteString(emitMethodsTable(model))
	b.WriteString("\n")
	for _, schema := range uniqueSchemas(model, func(m Method) Schema { return m.Request }) {
		fmt.Fprintf(&b, "int %s(const gzc_platform_t *platform, const %s *value, gzc_buf_t *out_json) {\n", encodeFuncName(model.Package, schema.Name), typeName(model.Package, schema.Name))
		b.WriteString("  if (value == NULL || out_json == NULL) {\n    return GZC_ERR_INVALID_ARGUMENT;\n  }\n")
		b.WriteString("  gzc_json_writer_t writer;\n  gzc_json_writer_init(&writer, platform, out_json);\n")
		b.WriteString("  int rc = gzc_json_object_begin(&writer);\n  if (rc != GZC_OK) { return rc; }\n")
		for _, field := range schema.Fields {
			cond := "true"
			if !field.Required {
				cond = "value->has_" + field.CName
			}
			fmt.Fprintf(&b, "  if (%s) {\n", cond)
			fmt.Fprintf(&b, "    rc = %s;\n", encodeFieldExpr(field))
			b.WriteString("    if (rc != GZC_OK) { return rc; }\n")
			b.WriteString("  }\n")
		}
		b.WriteString("  return gzc_json_object_end(&writer);\n")
		b.WriteString("}\n\n")
	}
	return b.Bytes(), nil
}

func encodeFieldExpr(field Field) string {
	switch field.Type.Kind {
	case CTypeString:
		return fmt.Sprintf("gzc_json_field_str(&writer, \"%s\", value->%s)", field.JSONName, field.CName)
	case CTypeBool:
		return fmt.Sprintf("gzc_json_field_bool(&writer, \"%s\", value->%s)", field.JSONName, field.CName)
	case CTypeI32:
		return fmt.Sprintf("gzc_json_field_i32(&writer, \"%s\", value->%s)", field.JSONName, field.CName)
	case CTypeI64:
		return fmt.Sprintf("gzc_json_field_i64(&writer, \"%s\", value->%s)", field.JSONName, field.CName)
	case CTypeF64:
		return fmt.Sprintf("gzc_json_field_f64(&writer, \"%s\", value->%s)", field.JSONName, field.CName)
	default:
		return fmt.Sprintf("gzc_json_field_raw(&writer, \"%s\", value->%s.raw)", field.JSONName, field.CName)
	}
}
