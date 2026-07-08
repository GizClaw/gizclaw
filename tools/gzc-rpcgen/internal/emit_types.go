package rpcgen

import (
	"bytes"
	"fmt"
)

func emitTypesH(model Model) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(cHeader())
	b.WriteString("#ifndef GZC_RPC_TYPES_H\n#define GZC_RPC_TYPES_H\n\n")
	b.WriteString("#include \"gzc_platform.h\"\n\n")
	b.WriteString("#ifdef __cplusplus\nextern \"C\" {\n#endif\n\n")
	b.WriteString("typedef struct {\n  gzc_str_t raw;\n} gzc_rpc_payload_t;\n\n")
	schemas := append(uniqueSchemas(model, func(m Method) Schema { return m.Request }), uniqueSchemas(model, func(m Method) Schema { return m.Response })...)
	seen := map[string]bool{}
	for _, schema := range schemas {
		if seen[schema.Name] {
			continue
		}
		seen[schema.Name] = true
		fmt.Fprintf(&b, "typedef struct {\n")
		if len(schema.Fields) == 0 {
			b.WriteString("  int _empty;\n")
		}
		for _, field := range schema.Fields {
			if !field.Required {
				fmt.Fprintf(&b, "  bool has_%s;\n", field.CName)
			}
			fmt.Fprintf(&b, "  %s %s;\n", field.Type.Name, field.CName)
		}
		fmt.Fprintf(&b, "} %s;\n\n", typeName(model.Package, schema.Name))
	}
	b.WriteString("#ifdef __cplusplus\n}\n#endif\n\n")
	b.WriteString("#endif\n")
	return b.Bytes(), nil
}
