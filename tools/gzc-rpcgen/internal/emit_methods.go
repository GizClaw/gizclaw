package rpcgen

import (
	"bytes"
	"fmt"
)

func emitMethodsH(model Model) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(cHeader())
	b.WriteString("#ifndef GZC_RPC_METHODS_H\n#define GZC_RPC_METHODS_H\n\n")
	b.WriteString("#include <stddef.h>\n\n")
	b.WriteString("#ifdef __cplusplus\nextern \"C\" {\n#endif\n\n")
	for _, method := range model.Methods {
		fmt.Fprintf(&b, "#define %s \"%s\"\n", method.ConstName, method.Value)
	}
	b.WriteString("\n")
	b.WriteString("typedef enum {\n")
	b.WriteString("  GZC_RPC_METHOD_KIND_UNARY = 0,\n")
	b.WriteString("  GZC_RPC_METHOD_KIND_BINARY_STREAM = 1,\n")
	b.WriteString("  GZC_RPC_METHOD_KIND_BINARY_DOWNLOAD = 2\n")
	b.WriteString("} gzc_rpc_method_kind_t;\n\n")
	b.WriteString("typedef struct {\n")
	b.WriteString("  const char *method;\n")
	b.WriteString("  unsigned method_id;\n")
	b.WriteString("  const char *request_type;\n")
	b.WriteString("  const char *response_type;\n")
	b.WriteString("  gzc_rpc_method_kind_t kind;\n")
	b.WriteString("} gzc_rpc_method_info_t;\n\n")
	b.WriteString("extern const gzc_rpc_method_info_t gzc_rpc_methods[];\n")
	fmt.Fprintf(&b, "#define GZC_RPC_METHOD_COUNT %d\n\n", len(model.Methods))
	b.WriteString("#ifdef __cplusplus\n}\n#endif\n\n")
	b.WriteString("#endif\n")
	return b.Bytes(), nil
}

func emitMethodsTable(model Model) string {
	var b bytes.Buffer
	b.WriteString("const gzc_rpc_method_info_t gzc_rpc_methods[] = {\n")
	for _, method := range model.Methods {
		fmt.Fprintf(&b, "  {%s, %du, \"%s\", \"%s\", %s},\n", method.ConstName, method.ID, method.RequestName, method.ResponseName, method.Kind)
	}
	b.WriteString("};\n")
	return b.String()
}
