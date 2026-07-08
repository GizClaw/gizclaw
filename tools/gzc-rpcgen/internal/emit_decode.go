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
		fmt.Fprintf(&b, "int %s(gzc_str_t payload, %s *out_value);\n", decodeFuncName(model.Package, schema.Name), typeName(model.Package, schema.Name))
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
	b.WriteString(protoDecodeHelpers(
		modelUsesResponseType(model, CTypeString) || modelUsesResponseType(model, CTypeJSON),
		modelUsesResponseType(model, CTypeBool),
		modelUsesResponseType(model, CTypeI32),
		modelUsesResponseType(model, CTypeI64),
		modelUsesResponseType(model, CTypeF64),
	))
	b.WriteString("\n")
	for _, schema := range uniqueSchemas(model, func(m Method) Schema { return m.Response }) {
		fmt.Fprintf(&b, "int %s(gzc_str_t payload, %s *out_value) {\n", decodeFuncName(model.Package, schema.Name), typeName(model.Package, schema.Name))
		b.WriteString("  if (out_value == NULL) {\n    return GZC_ERR_INVALID_ARGUMENT;\n  }\n")
		b.WriteString("  memset(out_value, 0, sizeof(*out_value));\n")
		if len(schema.Fields) == 0 {
			b.WriteString("  (void)payload;\n")
			b.WriteString("  return GZC_OK;\n")
			b.WriteString("}\n\n")
			continue
		}
		b.WriteString("  if (payload.data == NULL && payload.len != 0) {\n    return GZC_ERR_INVALID_ARGUMENT;\n  }\n")
		b.WriteString("  size_t offset = 0;\n")
		b.WriteString("  while (offset < payload.len) {\n")
		b.WriteString("    uint64_t key = 0;\n")
		b.WriteString("    int rc = gzc_rpc_proto_read_varint((const uint8_t *)payload.data, payload.len, &offset, &key);\n")
		b.WriteString("    if (rc != GZC_OK) { return rc; }\n")
		b.WriteString("    uint32_t field_number = (uint32_t)(key >> 3u);\n")
		b.WriteString("    uint32_t wire_type = (uint32_t)(key & 0x07u);\n")
		b.WriteString("    switch (field_number) {\n")
		for _, field := range schema.Fields {
			fmt.Fprintf(&b, "    case %d:\n", field.Number)
			if !field.Required {
				fmt.Fprintf(&b, "      out_value->has_%s = true;\n", field.CName)
			}
			fmt.Fprintf(&b, "      rc = %s;\n", decodeFieldExpr(field))
			b.WriteString("      if (rc != GZC_OK) { return rc; }\n")
			b.WriteString("      break;\n")
		}
		b.WriteString("    default:\n")
		b.WriteString("      rc = gzc_rpc_proto_skip((const uint8_t *)payload.data, payload.len, &offset, wire_type);\n")
		b.WriteString("      if (rc != GZC_OK) { return rc; }\n")
		b.WriteString("      break;\n")
		b.WriteString("    }\n")
		b.WriteString("  }\n")
		b.WriteString("  return GZC_OK;\n")
		b.WriteString("}\n\n")
	}
	return b.Bytes(), nil
}

func decodeFieldExpr(field Field) string {
	switch field.Type.Kind {
	case CTypeString:
		return fmt.Sprintf("gzc_rpc_proto_read_str(payload, wire_type, &offset, &out_value->%s)", field.CName)
	case CTypeBool:
		return fmt.Sprintf("gzc_rpc_proto_read_bool(payload, wire_type, &offset, &out_value->%s)", field.CName)
	case CTypeI32:
		return fmt.Sprintf("gzc_rpc_proto_read_i32(payload, wire_type, &offset, &out_value->%s)", field.CName)
	case CTypeI64:
		return fmt.Sprintf("gzc_rpc_proto_read_i64(payload, wire_type, &offset, &out_value->%s)", field.CName)
	case CTypeF64:
		return fmt.Sprintf("gzc_rpc_proto_read_double(payload, wire_type, &offset, &out_value->%s)", field.CName)
	default:
		if field.Repeated || field.Map {
			return fmt.Sprintf("gzc_rpc_proto_read_repeated_payload(payload, %d, wire_type, &offset, &out_value->%s)", field.Number, field.CName)
		}
		return fmt.Sprintf("gzc_rpc_proto_read_str(payload, wire_type, &offset, &out_value->%s.raw)", field.CName)
	}
}

func modelUsesResponseType(model Model, kind CTypeKind) bool {
	for _, schema := range uniqueSchemas(model, func(m Method) Schema { return m.Response }) {
		for _, field := range schema.Fields {
			if field.Type.Kind == kind {
				return true
			}
		}
	}
	return false
}

func protoDecodeHelpers(includeString, includeBool, includeI32, includeI64, includeDouble bool) string {
	out := `static int gzc_rpc_proto_read_varint(const uint8_t *data, size_t len, size_t *offset, uint64_t *out) {
  if ((data == NULL && len != 0) || offset == NULL || out == NULL) {
    return GZC_ERR_INVALID_ARGUMENT;
  }
  uint64_t value = 0;
  unsigned shift = 0;
  while (*offset < len && shift < 64u) {
    uint8_t byte = data[*offset];
    *offset += 1;
    value |= ((uint64_t)(byte & 0x7fu)) << shift;
    if ((byte & 0x80u) == 0) {
      *out = value;
      return GZC_OK;
    }
    shift += 7u;
  }
  return GZC_ERR_RPC;
}

static int gzc_rpc_proto_read_len(const uint8_t *data, size_t len, size_t *offset, gzc_str_t *out) {
  uint64_t size = 0;
  int rc = gzc_rpc_proto_read_varint(data, len, offset, &size);
  if (rc != GZC_OK) {
    return rc;
  }
  if (size > len - *offset) {
    return GZC_ERR_RPC;
  }
  out->data = (const char *)(data + *offset);
  out->len = (size_t)size;
  *offset += (size_t)size;
  return GZC_OK;
}
`
	if includeString {
		out += `
static int gzc_rpc_proto_read_str(gzc_str_t payload, uint32_t wire_type, size_t *offset, gzc_str_t *out) {
  if (wire_type != 2u || out == NULL) {
    return GZC_ERR_RPC;
  }
  return gzc_rpc_proto_read_len((const uint8_t *)payload.data, payload.len, offset, out);
}

static int gzc_rpc_proto_read_repeated_payload(gzc_str_t payload, uint32_t field_number, uint32_t wire_type, size_t *offset, gzc_rpc_payload_t *out) {
  gzc_str_t ignored = {0};
  if (wire_type != 2u || out == NULL) {
    return GZC_ERR_RPC;
  }
  if (out->count == 0u) {
    out->raw = payload;
    out->field_number = field_number;
  }
  out->count += 1u;
  return gzc_rpc_proto_read_len((const uint8_t *)payload.data, payload.len, offset, &ignored);
}
`
	}
	if includeBool {
		out += `
static int gzc_rpc_proto_read_bool(gzc_str_t payload, uint32_t wire_type, size_t *offset, bool *out) {
  uint64_t value = 0;
  if (wire_type != 0u || out == NULL) {
    return GZC_ERR_RPC;
  }
  int rc = gzc_rpc_proto_read_varint((const uint8_t *)payload.data, payload.len, offset, &value);
  if (rc != GZC_OK) {
    return rc;
  }
  *out = value != 0;
  return GZC_OK;
}
`
	}
	if includeI32 {
		out += `
static int gzc_rpc_proto_read_i32(gzc_str_t payload, uint32_t wire_type, size_t *offset, int32_t *out) {
  uint64_t value = 0;
  if (wire_type != 0u || out == NULL) {
    return GZC_ERR_RPC;
  }
  int rc = gzc_rpc_proto_read_varint((const uint8_t *)payload.data, payload.len, offset, &value);
  if (rc != GZC_OK) {
    return rc;
  }
  *out = (int32_t)value;
  return GZC_OK;
}
`
	}
	if includeI64 {
		out += `
static int gzc_rpc_proto_read_i64(gzc_str_t payload, uint32_t wire_type, size_t *offset, int64_t *out) {
  uint64_t value = 0;
  if (wire_type != 0u || out == NULL) {
    return GZC_ERR_RPC;
  }
  int rc = gzc_rpc_proto_read_varint((const uint8_t *)payload.data, payload.len, offset, &value);
  if (rc != GZC_OK) {
    return rc;
  }
  *out = (int64_t)value;
  return GZC_OK;
}
`
	}
	if includeDouble {
		out += `
static int gzc_rpc_proto_read_double(gzc_str_t payload, uint32_t wire_type, size_t *offset, double *out) {
  if (wire_type != 1u || out == NULL || payload.len - *offset < 8u) {
    return GZC_ERR_RPC;
  }
  uint64_t bits = 0;
  const uint8_t *data = (const uint8_t *)payload.data + *offset;
  for (size_t i = 0; i < 8u; i++) {
    bits |= ((uint64_t)data[i]) << (i * 8u);
  }
  memcpy(out, &bits, sizeof(bits));
  *offset += 8u;
  return GZC_OK;
}
`
	}
	out += `
static int gzc_rpc_proto_skip(const uint8_t *data, size_t len, size_t *offset, uint32_t wire_type) {
  uint64_t value = 0;
  switch (wire_type) {
  case 0u:
    return gzc_rpc_proto_read_varint(data, len, offset, &value);
  case 1u:
    if (len - *offset < 8u) {
      return GZC_ERR_RPC;
    }
    *offset += 8u;
    return GZC_OK;
  case 2u:
    return gzc_rpc_proto_read_len(data, len, offset, &(gzc_str_t){0});
  case 5u:
    if (len - *offset < 4u) {
      return GZC_ERR_RPC;
    }
    *offset += 4u;
    return GZC_OK;
  default:
    return GZC_ERR_RPC;
  }
}
`
	return out
}
