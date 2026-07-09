package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

type enumDef struct {
	Name   string
	Values []enumValue
}

type enumValue struct {
	Name   string
	Number string
}

type messageDef struct {
	Name   string
	Fields []fieldDef
	Oneof  bool
}

type fieldDef struct {
	Name     string
	JSONName string
	Type     string
	Optional bool
	Repeated bool
	Map      bool
	MapValue string
	Oneof    bool
}

type rpcMethod struct {
	Name     string
	Request  string
	Response string
}

type protoDoc struct {
	Enums    map[string]enumDef
	Messages map[string]messageDef
	Order    []string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "gzc-rpcapi-gen: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var peerPath string
	var commonPath string
	var payloadPath string
	var outPath string
	flags := flag.NewFlagSet("gzc-rpcapi-gen", flag.ContinueOnError)
	flags.StringVar(&peerPath, "peer", "api/rpc/peer.proto", "Peer RPC protobuf schema")
	flags.StringVar(&commonPath, "common", "api/rpc/common.proto", "Common RPC protobuf schema")
	flags.StringVar(&payloadPath, "payload", "api/rpc/payload.proto", "Peer RPC payload protobuf schema")
	flags.StringVar(&outPath, "out", "pkgs/gizclaw/api/rpcapi/generated.go", "Generated Go rpcapi DTO output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if outPath == "" {
		return errors.New("-out is required")
	}
	peer, err := parseProto(peerPath)
	if err != nil {
		return err
	}
	common, err := parseProto(commonPath)
	if err != nil {
		return err
	}
	payload, err := parseProto(payloadPath)
	if err != nil {
		return err
	}
	methods, err := loadRPCMethods(peerPath)
	if err != nil {
		return err
	}
	out, err := emit(peer, common, payload, methods)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, out, 0644)
}

func parseProto(path string) (protoDoc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return protoDoc{}, err
	}
	enumRe := regexp.MustCompile(`^\s*enum\s+([A-Za-z_]\w*)\s*\{\s*$`)
	enumValueRe := regexp.MustCompile(`^\s*([A-Z][A-Z0-9_]*)\s*=\s*(-?\d+)\s*;`)
	messageRe := regexp.MustCompile(`^\s*message\s+([A-Za-z_]\w*)\s*\{\s*$`)
	fieldRe := regexp.MustCompile(`^\s*(optional\s+|repeated\s+)?(map<([^,>]+),\s*([^>]+)>|[A-Za-z0-9_.]+)\s+([A-Za-z_]\w*)\s*=\s*\d+\s*(?:\[([^\]]*)\])?\s*;`)
	out := protoDoc{Enums: map[string]enumDef{}, Messages: map[string]messageDef{}}
	var current string
	var kind string
	inOneof := false
	depth := 0
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if current == "" {
			if match := enumRe.FindStringSubmatch(line); match != nil {
				current = match[1]
				kind = "enum"
				depth = 1
				out.Enums[current] = enumDef{Name: current}
				continue
			}
			if match := messageRe.FindStringSubmatch(line); match != nil {
				current = match[1]
				kind = "message"
				depth = 1
				out.Messages[current] = messageDef{Name: current}
				out.Order = append(out.Order, current)
				continue
			}
			continue
		}
		if kind == "enum" {
			if match := enumValueRe.FindStringSubmatch(line); match != nil {
				def := out.Enums[current]
				def.Values = append(def.Values, enumValue{Name: match[1], Number: match[2]})
				out.Enums[current] = def
			}
			depth += braceDelta(line)
			if depth <= 0 {
				current = ""
				kind = ""
			}
			continue
		}
		if trimmed == "oneof value {" {
			inOneof = true
			msg := out.Messages[current]
			msg.Oneof = true
			out.Messages[current] = msg
		} else if match := fieldRe.FindStringSubmatch(line); match != nil {
			prefix := strings.TrimSpace(match[1])
			field := fieldDef{
				Name:     match[5],
				JSONName: protoOptionJSONName(match[6]),
				Type:     match[2],
				Optional: prefix == "optional",
				Repeated: prefix == "repeated",
				Map:      strings.HasPrefix(match[2], "map<"),
				MapValue: strings.TrimSpace(match[4]),
				Oneof:    inOneof,
			}
			msg := out.Messages[current]
			msg.Fields = append(msg.Fields, field)
			out.Messages[current] = msg
		}
		depth += braceDelta(line)
		if inOneof && strings.Contains(line, "}") {
			inOneof = false
		}
		if depth <= 0 {
			current = ""
			kind = ""
			inOneof = false
			depth = 0
		}
	}
	return out, nil
}

func loadRPCMethods(path string) ([]rpcMethod, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	commentRe := regexp.MustCompile(`^\s*//\s*rpc:\s+(\S+)\s+request=(\w+)\s+response=(\w+)\s*$`)
	entryRe := regexp.MustCompile(`^\s*RPC_METHOD_[A-Z0-9_]+\s*=\s*(-?\d+)\s*;`)
	var out []rpcMethod
	var pending *rpcMethod
	for _, line := range strings.Split(string(data), "\n") {
		if match := commentRe.FindStringSubmatch(line); match != nil {
			pending = &rpcMethod{Name: match[1], Request: match[2], Response: match[3]}
			continue
		}
		if entryRe.FindStringSubmatch(line) == nil {
			continue
		}
		if pending == nil {
			continue
		}
		out = append(out, *pending)
		pending = nil
	}
	return out, nil
}

func emit(peer, common, payload protoDoc, methods []rpcMethod) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("// Package rpcapi provides primitives to interact with the peer RPC API.\n\n")
	buf.WriteString("// Code generated by tools/gzc-rpcapi-gen. DO NOT EDIT.\n")
	buf.WriteString("package rpcapi\n\n")
	buf.WriteString("import (\n")
	buf.WriteString("\t\"encoding/json\"\n")
	buf.WriteString("\t\"errors\"\n")
	buf.WriteString("\t\"time\"\n")
	buf.WriteString(")\n\n")

	for _, name := range sortedEnumNames(payload.Enums) {
		emitStringEnum(&buf, payload.Enums[name])
	}
	emitRPCErrorCode(&buf, common.Enums["RpcErrorCode"])
	emitRPCMethods(&buf, methods)
	emitRPCVersion(&buf)

	for _, name := range payload.Order {
		msg := payload.Messages[name]
		emitMessageType(&buf, payload, msg)
	}
	emitRPCEnvelopeTypes(&buf)
	for _, name := range payload.Order {
		msg := payload.Messages[name]
		if msg.Oneof {
			emitUnionHelpers(&buf, msg)
		}
	}
	emitPayloadUnionHelpers(&buf, "RPCRequest_Params", requestPayloads(methods))
	emitPayloadUnionHelpers(&buf, "RPCResponse_Result", responsePayloads(methods))
	emitWorkspaceDiscriminator(&buf)
	emitJSONMerge(&buf)
	return buf.Bytes(), nil
}

func emitStringEnum(buf *bytes.Buffer, enum enumDef) {
	values := enumStringValues(enum)
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(buf, "// Defines values for %s.\n", enum.Name)
	buf.WriteString("const (\n")
	for _, value := range values {
		fmt.Fprintf(buf, "\t%s%s %s = %q\n", enum.Name, upperCamel(value), enum.Name, value)
	}
	buf.WriteString(")\n\n")
	fmt.Fprintf(buf, "// %s defines model for %s.\n", enum.Name, enum.Name)
	fmt.Fprintf(buf, "type %s string\n\n", enum.Name)
	emitValidMethod(buf, enum.Name, values)
}

func emitValidMethod(buf *bytes.Buffer, typeName string, values []string) {
	fmt.Fprintf(buf, "// Valid indicates whether the value is a known member of the %s enum.\n", typeName)
	fmt.Fprintf(buf, "func (e %s) Valid() bool {\n", typeName)
	buf.WriteString("\tswitch e {\n")
	for _, value := range values {
		fmt.Fprintf(buf, "\tcase %s%s:\n\t\treturn true\n", typeName, upperCamel(value))
	}
	buf.WriteString("\tdefault:\n\t\treturn false\n\t}\n}\n\n")
}

func emitRPCErrorCode(buf *bytes.Buffer, enum enumDef) {
	buf.WriteString("// Defines values for RPCErrorCode.\n")
	buf.WriteString("const (\n")
	for _, value := range enum.Values {
		if strings.HasSuffix(value.Name, "_UNSPECIFIED") {
			continue
		}
		name := strings.TrimPrefix(value.Name, "RPC_ERROR_CODE_")
		fmt.Fprintf(buf, "\tRPCErrorCode%s RPCErrorCode = %s\n", upperCamel(strings.ToLower(name)), value.Number)
	}
	buf.WriteString(")\n\n")
	values := make([]string, 0, len(enum.Values))
	for _, value := range enum.Values {
		if strings.HasSuffix(value.Name, "_UNSPECIFIED") {
			continue
		}
		values = append(values, strings.ToLower(strings.TrimPrefix(value.Name, "RPC_ERROR_CODE_")))
	}
	emitValidMethod(buf, "RPCErrorCode", values)
}

func emitRPCMethods(buf *bytes.Buffer, methods []rpcMethod) {
	buf.WriteString("// Defines values for RPCMethod.\n")
	buf.WriteString("const (\n")
	for _, method := range methods {
		fmt.Fprintf(buf, "\t%s RPCMethod = %q\n", goRPCMethodConst(method.Name), method.Name)
	}
	buf.WriteString(")\n\n")
	values := make([]string, 0, len(methods))
	for _, method := range methods {
		values = append(values, method.Name)
	}
	emitValidMethod(buf, "RPCMethod", values)
}

func emitRPCVersion(buf *bytes.Buffer) {
	buf.WriteString("// Defines values for RPCVersion.\n")
	buf.WriteString("const (\n\tRPCVersionV1 RPCVersion = 1\n)\n\n")
	buf.WriteString("// Valid indicates whether the value is a known member of the RPCVersion enum.\n")
	buf.WriteString("func (e RPCVersion) Valid() bool {\n\tswitch e {\n\tcase RPCVersionV1:\n\t\treturn true\n\tdefault:\n\t\treturn false\n\t}\n}\n\n")
}

func emitMessageType(buf *bytes.Buffer, doc protoDoc, msg messageDef) {
	if msg.Oneof {
		fmt.Fprintf(buf, "// %s defines model for %s.\n", msg.Name, msg.Name)
		fmt.Fprintf(buf, "type %s struct {\n\tunion json.RawMessage\n}\n\n", msg.Name)
		return
	}
	if len(msg.Fields) == 0 {
		fmt.Fprintf(buf, "// %s defines model for %s.\n", msg.Name, msg.Name)
		fmt.Fprintf(buf, "type %s = map[string]interface{}\n\n", msg.Name)
		return
	}
	if len(msg.Fields) == 1 && msg.Fields[0].Name == "value" {
		fmt.Fprintf(buf, "// %s defines model for %s.\n", msg.Name, msg.Name)
		fmt.Fprintf(buf, "type %s = %s\n\n", msg.Name, goFieldType(doc, msg.Fields[0], false))
		return
	}
	if len(msg.Fields) == 1 && msg.Fields[0].Name == "fields" && msg.Fields[0].Type == "google.protobuf.Struct" {
		fmt.Fprintf(buf, "// %s defines model for %s.\n", msg.Name, msg.Name)
		fmt.Fprintf(buf, "type %s map[string]interface{}\n\n", msg.Name)
		return
	}
	fmt.Fprintf(buf, "// %s defines model for %s.\n", msg.Name, msg.Name)
	fmt.Fprintf(buf, "type %s struct {\n", msg.Name)
	for _, field := range msg.Fields {
		jsonName := fieldJSONName(field)
		fieldName := upperCamel(jsonName)
		fieldType := goFieldType(doc, field, true)
		tag := jsonName
		if field.Optional || strings.HasPrefix(fieldType, "*") {
			tag += ",omitempty"
		}
		fmt.Fprintf(buf, "\t%s %s `json:\"%s\"`\n", fieldName, fieldType, tag)
	}
	buf.WriteString("}\n\n")
}

func emitRPCEnvelopeTypes(buf *bytes.Buffer) {
	buf.WriteString("// RPCError defines model for RPCError.\n")
	buf.WriteString("type RPCError struct {\n\tCode RPCErrorCode `json:\"code\"`\n\tMessage string `json:\"message\"`\n}\n\n")
	buf.WriteString("// RPCErrorCode defines model for RPCErrorCode.\n")
	buf.WriteString("type RPCErrorCode int\n\n")
	buf.WriteString("// RPCMethod defines model for RPCMethod.\n")
	buf.WriteString("type RPCMethod string\n\n")
	buf.WriteString("// RPCRequest defines model for RPCRequest.\n")
	buf.WriteString("type RPCRequest struct {\n\tId string `json:\"id\"`\n\tMethod RPCMethod `json:\"method\"`\n\tParams *RPCRequest_Params `json:\"params,omitempty\"`\n\tV RPCVersion `json:\"v\"`\n}\n\n")
	buf.WriteString("// RPCRequest_Params defines model for RPCRequest.Params.\n")
	buf.WriteString("type RPCRequest_Params struct {\n\tunion json.RawMessage\n}\n\n")
	buf.WriteString("// RPCResponse defines model for RPCResponse.\n")
	buf.WriteString("type RPCResponse struct {\n\tError *RPCError `json:\"error,omitempty\"`\n\tId string `json:\"id\"`\n\tResult *RPCResponse_Result `json:\"result,omitempty\"`\n\tV RPCVersion `json:\"v\"`\n}\n\n")
	buf.WriteString("// RPCResponse_Result defines model for RPCResponse.Result.\n")
	buf.WriteString("type RPCResponse_Result struct {\n\tunion json.RawMessage\n}\n\n")
	buf.WriteString("// RPCVersion defines model for RPCVersion.\n")
	buf.WriteString("type RPCVersion int\n\n")
}

func emitUnionHelpers(buf *bytes.Buffer, msg messageDef) {
	types := make([]string, 0, len(msg.Fields))
	for _, field := range msg.Fields {
		types = append(types, field.Type)
	}
	emitPayloadUnionHelpers(buf, msg.Name, types)
	if msg.Name == "WorkspaceParameters" {
		return
	}
}

func emitPayloadUnionHelpers(buf *bytes.Buffer, unionName string, types []string) {
	seen := map[string]bool{}
	for _, typ := range types {
		if seen[typ] {
			continue
		}
		seen[typ] = true
		fmt.Fprintf(buf, "// As%s returns the union data inside the %s as a %s\n", typ, unionName, typ)
		fmt.Fprintf(buf, "func (t %s) As%s() (%s, error) {\n\tvar body %s\n\terr := json.Unmarshal(t.union, &body)\n\treturn body, err\n}\n\n", unionName, typ, typ, typ)
		fmt.Fprintf(buf, "// From%s overwrites any union data inside the %s as the provided %s\n", typ, unionName, typ)
		fmt.Fprintf(buf, "func (t *%s) From%s(v %s) error {\n\tb, err := json.Marshal(v)\n\tt.union = b\n\treturn err\n}\n\n", unionName, typ, typ)
		fmt.Fprintf(buf, "// Merge%s performs a merge with any union data inside the %s, using the provided %s\n", typ, unionName, typ)
		fmt.Fprintf(buf, "func (t *%s) Merge%s(v %s) error {\n\tb, err := json.Marshal(v)\n\tif err != nil {\n\t\treturn err\n\t}\n\tmerged, err := jsonMerge(t.union, b)\n\tt.union = merged\n\treturn err\n}\n\n", unionName, typ, typ)
	}
	emitUnionJSON(buf, unionName)
}

func emitUnionJSON(buf *bytes.Buffer, unionName string) {
	fmt.Fprintf(buf, "func (t %s) MarshalJSON() ([]byte, error) {\n\tb, err := t.union.MarshalJSON()\n\treturn b, err\n}\n\n", unionName)
	fmt.Fprintf(buf, "func (t *%s) UnmarshalJSON(b []byte) error {\n\terr := t.union.UnmarshalJSON(b)\n\treturn err\n}\n\n", unionName)
}

func emitWorkspaceDiscriminator(buf *bytes.Buffer) {
	buf.WriteString("func (t WorkspaceParameters) Discriminator() (string, error) {\n")
	buf.WriteString("\tvar discriminator struct {\n\t\tDiscriminator string `json:\"agent_type\"`\n\t}\n")
	buf.WriteString("\terr := json.Unmarshal(t.union, &discriminator)\n\treturn discriminator.Discriminator, err\n}\n\n")
	buf.WriteString("func (t WorkspaceParameters) ValueByDiscriminator() (interface{}, error) {\n")
	buf.WriteString("\tdiscriminator, err := t.Discriminator()\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	buf.WriteString("\tswitch discriminator {\n")
	buf.WriteString("\tcase \"ast-translate\":\n\t\treturn t.AsASTTranslateWorkspaceParameters()\n")
	buf.WriteString("\tcase \"chatroom\":\n\t\treturn t.AsChatRoomWorkspaceParameters()\n")
	buf.WriteString("\tcase \"doubao-realtime\":\n\t\treturn t.AsDoubaoRealtimeWorkspaceParameters()\n")
	buf.WriteString("\tcase \"flowcraft\":\n\t\treturn t.AsFlowcraftWorkspaceParameters()\n")
	buf.WriteString("\tdefault:\n\t\treturn nil, errors.New(\"unknown discriminator value: \" + discriminator)\n\t}\n}\n\n")
}

func emitJSONMerge(buf *bytes.Buffer) {
	buf.WriteString("func jsonMerge(a, b []byte) ([]byte, error) {\n")
	buf.WriteString("\tif len(a) == 0 {\n\t\treturn b, nil\n\t}\n\tif len(b) == 0 {\n\t\treturn a, nil\n\t}\n")
	buf.WriteString("\tvar left map[string]interface{}\n\tvar right map[string]interface{}\n")
	buf.WriteString("\tif err := json.Unmarshal(a, &left); err != nil {\n\t\treturn nil, err\n\t}\n")
	buf.WriteString("\tif err := json.Unmarshal(b, &right); err != nil {\n\t\treturn nil, err\n\t}\n")
	buf.WriteString("\tfor key, value := range right {\n\t\tleft[key] = value\n\t}\n")
	buf.WriteString("\treturn json.Marshal(left)\n}\n")
}

func goFieldType(doc protoDoc, field fieldDef, allowPointer bool) string {
	if field.Map {
		return "map[string]" + goScalarType(doc, field.MapValue, field.Name)
	}
	if field.Repeated {
		typ := "[]" + goScalarType(doc, field.Type, field.Name)
		if allowPointer && optionalRepeatedField(field.Name) {
			return "*" + typ
		}
		return typ
	}
	typ := goScalarType(doc, field.Type, field.Name)
	if allowPointer && field.Optional {
		return "*" + typ
	}
	if allowPointer && field.Type == "google.protobuf.Struct" {
		return "*" + typ
	}
	return typ
}

func goScalarType(doc protoDoc, protoType, fieldName string) string {
	switch protoType {
	case "string":
		if strings.HasSuffix(fieldName, "_at") || fieldName == "mod_time" {
			return "time.Time"
		}
		return "string"
	case "bytes":
		return "[]byte"
	case "bool":
		return "bool"
	case "int32":
		return "int32"
	case "uint32":
		return "uint32"
	case "int64":
		if intField(fieldName) {
			return "int"
		}
		return "int64"
	case "uint64":
		return "uint64"
	case "double", "float":
		return "float64"
	case "google.protobuf.Struct":
		return "map[string]interface{}"
	case "google.protobuf.Value":
		return "interface{}"
	default:
		typ := strings.TrimPrefix(protoType, ".")
		if _, ok := doc.Enums[typ]; ok {
			return typ
		}
		return typ
	}
}

func optionalRepeatedField(name string) bool {
	switch name {
	case "imeis", "labels":
		return true
	default:
		return false
	}
}

func intField(name string) bool {
	switch name {
	case "limit", "ttl_seconds", "speech_rate", "minimum", "maximum", "min_items", "max_items", "level", "exp", "progress", "battery_percent":
		return true
	default:
		return false
	}
}

func requestPayloads(methods []rpcMethod) []string {
	out := make([]string, 0, len(methods))
	for _, method := range methods {
		out = append(out, method.Request)
	}
	return out
}

func responsePayloads(methods []rpcMethod) []string {
	out := make([]string, 0, len(methods))
	for _, method := range methods {
		out = append(out, method.Response)
	}
	return out
}

func enumStringValues(enum enumDef) []string {
	prefix := strings.ToUpper(lowerSnake(enum.Name))
	out := make([]string, 0, len(enum.Values))
	for _, value := range enum.Values {
		if strings.HasSuffix(value.Name, "_UNSPECIFIED") {
			continue
		}
		item := strings.TrimPrefix(value.Name, prefix+"_")
		item = strings.ToLower(item)
		sep := "_"
		switch {
		case strings.Contains(enum.Name, "AgentType"), strings.Contains(enum.Name, "ProviderKind"), enum.Name == "WorkflowDriver", enum.Name == "WorkspaceInputMode":
			sep = "-"
		}
		out = append(out, strings.ReplaceAll(item, "_", sep))
	}
	sort.Strings(out)
	return out
}

func goRPCMethodConst(method string) string {
	return "RPCMethod" + upperCamel(method)
}

func sortedEnumNames(m map[string]enumDef) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func upperCamel(value string) string {
	parts := regexp.MustCompile(`[^A-Za-z0-9]+`).Split(value, -1)
	var out strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		out.WriteString(strings.ToUpper(part[:1]))
		if len(part) > 1 {
			out.WriteString(part[1:])
		}
	}
	if out.Len() == 0 {
		return "Value"
	}
	return out.String()
}

func lowerSnake(value string) string {
	var out strings.Builder
	prevLower := false
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
			if prevLower {
				out.WriteByte('_')
			}
			out.WriteRune(r + ('a' - 'A'))
			prevLower = false
		case r >= 'a' && r <= 'z':
			out.WriteRune(r)
			prevLower = true
		case r >= '0' && r <= '9':
			out.WriteRune(r)
			prevLower = false
		default:
			if out.Len() > 0 && !strings.HasSuffix(out.String(), "_") {
				out.WriteByte('_')
			}
			prevLower = false
		}
	}
	return strings.Trim(out.String(), "_")
}

func fieldJSONName(field fieldDef) string {
	if field.JSONName != "" {
		return field.JSONName
	}
	return field.Name
}

func protoOptionJSONName(options string) string {
	match := regexp.MustCompile(`(?:^|,)\s*json_name\s*=\s*"([^"]+)"`).FindStringSubmatch(options)
	if match == nil {
		return ""
	}
	return match[1]
}

func braceDelta(line string) int {
	return strings.Count(line, "{") - strings.Count(line, "}")
}
