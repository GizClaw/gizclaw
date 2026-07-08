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
		fmt.Fprintf(&b, "int %s(const gzc_platform_t *platform, const %s *value, gzc_buf_t *out_payload);\n", encodeFuncName(model.Package, schema.Name), typeName(model.Package, schema.Name))
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
	b.WriteString("#include <string.h>\n\n")
	b.WriteString(protoEncodeHelpers(modelUsesRequestType(model, CTypeF64)))
	b.WriteString("\n")
	b.WriteString(emitMethodsTable(model))
	b.WriteString("\n")
	for _, schema := range uniqueSchemas(model, func(m Method) Schema { return m.Request }) {
		fmt.Fprintf(&b, "int %s(const gzc_platform_t *platform, const %s *value, gzc_buf_t *out_payload) {\n", encodeFuncName(model.Package, schema.Name), typeName(model.Package, schema.Name))
		b.WriteString("  if (value == NULL || out_payload == NULL) {\n    return GZC_ERR_INVALID_ARGUMENT;\n  }\n")
		b.WriteString("  gzc_buf_reset(out_payload);\n")
		if len(schema.Fields) == 0 {
			b.WriteString("  (void)platform;\n")
			b.WriteString("  return GZC_OK;\n")
			b.WriteString("}\n\n")
			continue
		}
		b.WriteString("  int rc;\n")
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
		b.WriteString("  return GZC_OK;\n")
		b.WriteString("}\n\n")
	}
	return b.Bytes(), nil
}

func encodeFieldExpr(field Field) string {
	switch field.Type.Kind {
	case CTypeString:
		return fmt.Sprintf("gzc_rpc_proto_append_bytes(platform, out_payload, %d, (const uint8_t *)value->%s.data, value->%s.len)", field.Number, field.CName, field.CName)
	case CTypeBool:
		return fmt.Sprintf("gzc_rpc_proto_append_varint(platform, out_payload, %d, value->%s ? 1u : 0u)", field.Number, field.CName)
	case CTypeI32:
		return fmt.Sprintf("gzc_rpc_proto_append_varint(platform, out_payload, %d, (uint64_t)(uint32_t)value->%s)", field.Number, field.CName)
	case CTypeI64:
		return fmt.Sprintf("gzc_rpc_proto_append_varint(platform, out_payload, %d, (uint64_t)value->%s)", field.Number, field.CName)
	case CTypeF64:
		return fmt.Sprintf("gzc_rpc_proto_append_double(platform, out_payload, %d, value->%s)", field.Number, field.CName)
	default:
		return fmt.Sprintf("gzc_rpc_proto_append_bytes(platform, out_payload, %d, (const uint8_t *)value->%s.raw.data, value->%s.raw.len)", field.Number, field.CName, field.CName)
	}
}

func modelUsesRequestType(model Model, kind CTypeKind) bool {
	for _, schema := range uniqueSchemas(model, func(m Method) Schema { return m.Request }) {
		for _, field := range schema.Fields {
			if field.Type.Kind == kind {
				return true
			}
		}
	}
	return false
}

func protoEncodeHelpers(includeDouble bool) string {
	out := `static int gzc_rpc_proto_append_raw_varint(const gzc_platform_t *platform, gzc_buf_t *out, uint64_t value) {
  uint8_t buf[10];
  size_t n = 0;
  do {
    uint8_t byte = (uint8_t)(value & 0x7fu);
    value >>= 7u;
    if (value != 0) {
      byte |= 0x80u;
    }
    buf[n++] = byte;
  } while (value != 0 && n < sizeof(buf));
  return gzc_buf_append(out, platform, buf, n);
}

static int gzc_rpc_proto_append_key(const gzc_platform_t *platform, gzc_buf_t *out, uint32_t number, uint32_t wire_type) {
  return gzc_rpc_proto_append_raw_varint(platform, out, ((uint64_t)number << 3u) | wire_type);
}

static int gzc_rpc_proto_append_varint(const gzc_platform_t *platform, gzc_buf_t *out, uint32_t number, uint64_t value) {
  int rc = gzc_rpc_proto_append_key(platform, out, number, 0u);
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_rpc_proto_append_raw_varint(platform, out, value);
}

static int gzc_rpc_proto_append_bytes(const gzc_platform_t *platform, gzc_buf_t *out, uint32_t number, const uint8_t *data, size_t len) {
  if (data == NULL && len != 0) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  int rc = gzc_rpc_proto_append_key(platform, out, number, 2u);
  if (rc != GZC_OK) {
    return rc;
  }
  rc = gzc_rpc_proto_append_raw_varint(platform, out, (uint64_t)len);
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_buf_append(out, platform, data, len);
}
`
	if includeDouble {
		out += `
static int gzc_rpc_proto_append_double(const gzc_platform_t *platform, gzc_buf_t *out, uint32_t number, double value) {
  uint64_t bits = 0;
  uint8_t buf[8];
  memcpy(&bits, &value, sizeof(bits));
  for (size_t i = 0; i < sizeof(buf); i++) {
    buf[i] = (uint8_t)((bits >> (i * 8u)) & 0xffu);
  }
  int rc = gzc_rpc_proto_append_key(platform, out, number, 1u);
  if (rc != GZC_OK) {
    return rc;
  }
  return gzc_buf_append(out, platform, buf, sizeof(buf));
}
`
	}
	return out
}
