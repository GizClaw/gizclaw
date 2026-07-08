package rpcgen

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type protoMethod struct {
	ID       int
	Name     string
	Request  string
	Response string
}

type protoMessage struct {
	Fields []protoField
}

type protoPayload struct {
	Messages map[string]protoMessage
	Enums    map[string]bool
}

type protoField struct {
	Name     string
	Number   int
	Type     string
	Optional bool
	Repeated bool
	Map      bool
}

func loadProtoModel(cfg Config) (Model, error) {
	if cfg.ProtoPath == "" || cfg.PayloadProtoPath == "" {
		return Model{}, fmt.Errorf("-proto and -payload-proto are required when -schema is empty")
	}
	methods, err := loadProtoMethods(cfg.ProtoPath)
	if err != nil {
		return Model{}, err
	}
	payload, err := loadPayloadProto(cfg.PayloadProtoPath)
	if err != nil {
		return Model{}, err
	}
	model := Model{Package: cfg.Package}
	for i, method := range methods {
		req, ok := payload.Messages[method.Request]
		if !ok {
			return Model{}, fmt.Errorf("%s request message %s not found in %s", method.Name, method.Request, cfg.PayloadProtoPath)
		}
		resp, ok := payload.Messages[method.Response]
		if !ok {
			return Model{}, fmt.Errorf("%s response message %s not found in %s", method.Name, method.Response, cfg.PayloadProtoPath)
		}
		model.Methods = append(model.Methods, Method{
			Index:        i,
			ID:           method.ID,
			Value:        method.Name,
			ConstName:    methodConstName(cfg.Package, method.Name),
			RequestName:  method.Request,
			ResponseName: method.Response,
			Kind:         methodKind(method.Name),
			Request:      schemaFromProtoMessage(method.Request, req, payload.Enums),
			Response:     schemaFromProtoMessage(method.Response, resp, payload.Enums),
		})
	}
	return model, nil
}

func loadProtoMethods(path string) ([]protoMethod, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	commentRe := regexp.MustCompile(`^\s*//\s*rpc:\s+(\S+)\s+request=(\w+)\s+response=(\w+)\s*$`)
	entryRe := regexp.MustCompile(`^\s*RPC_METHOD_[A-Z0-9_]+\s*=\s*(\d+)\s*;`)
	var out []protoMethod
	var pending *protoMethod
	seenNames := map[string]bool{}
	seenIDs := map[int]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if match := commentRe.FindStringSubmatch(line); match != nil {
			pending = &protoMethod{Name: match[1], Request: match[2], Response: match[3]}
			continue
		}
		match := entryRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		if pending == nil {
			if strings.Contains(line, "RPC_METHOD_UNSPECIFIED") {
				continue
			}
			return nil, fmt.Errorf("%s: RpcMethod entry missing rpc comment: %s", path, strings.TrimSpace(line))
		}
		id, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("%s: invalid RpcMethod id %q: %w", path, match[1], err)
		}
		if seenNames[pending.Name] {
			return nil, fmt.Errorf("%s: duplicate rpc method %q", path, pending.Name)
		}
		if seenIDs[id] {
			return nil, fmt.Errorf("%s: duplicate rpc method id %d", path, id)
		}
		pending.ID = id
		out = append(out, *pending)
		seenNames[pending.Name] = true
		seenIDs[id] = true
		pending = nil
	}
	return out, nil
}

func loadPayloadProto(path string) (protoPayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return protoPayload{}, err
	}
	messageRe := regexp.MustCompile(`^\s*message\s+([A-Za-z0-9_]+)\s+\{\s*$`)
	enumRe := regexp.MustCompile(`^\s*enum\s+([A-Za-z0-9_]+)\s+\{\s*$`)
	fieldRe := regexp.MustCompile(`^\s*(optional\s+|repeated\s+)?(map<[^>]+>|[A-Za-z0-9_.]+)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(\d+)\s*;`)
	out := protoPayload{Messages: map[string]protoMessage{}, Enums: map[string]bool{}}
	var current string
	var currentKind string
	depth := 0
	for _, line := range strings.Split(string(data), "\n") {
		if current == "" {
			if match := messageRe.FindStringSubmatch(line); match != nil {
				current = match[1]
				currentKind = "message"
				depth = 1
				out.Messages[current] = protoMessage{}
				continue
			}
			if match := enumRe.FindStringSubmatch(line); match != nil {
				current = match[1]
				currentKind = "enum"
				depth = 1
				out.Enums[current] = true
			}
			continue
		}
		depth += strings.Count(line, "{")
		depth -= strings.Count(line, "}")
		if currentKind == "message" {
			if match := fieldRe.FindStringSubmatch(line); match != nil {
				n, err := strconv.Atoi(match[4])
				if err != nil {
					return protoPayload{}, fmt.Errorf("%s: invalid field number %q in %s.%s: %w", path, match[4], current, match[3], err)
				}
				msg := out.Messages[current]
				msg.Fields = append(msg.Fields, protoField{
					Name:     match[3],
					Number:   n,
					Type:     match[2],
					Optional: strings.TrimSpace(match[1]) == "optional",
					Repeated: strings.TrimSpace(match[1]) == "repeated",
					Map:      strings.HasPrefix(match[2], "map<"),
				})
				out.Messages[current] = msg
			}
		}
		if depth <= 0 {
			current = ""
			currentKind = ""
			depth = 0
		}
	}
	return out, nil
}

func schemaFromProtoMessage(name string, msg protoMessage, enums map[string]bool) Schema {
	out := Schema{Name: name}
	for _, field := range msg.Fields {
		out.Fields = append(out.Fields, Field{
			JSONName: field.Name,
			CName:    snakeIdent(field.Name),
			Number:   field.Number,
			Type:     ctypeForProtoField(field, enums),
			Required: !field.Optional,
		})
	}
	sortFields(out.Fields)
	return out
}

func ctypeForProtoField(field protoField, enums map[string]bool) CType {
	if field.Repeated || field.Map {
		return CType{Kind: CTypeJSON, Name: "gzc_json_t"}
	}
	switch field.Type {
	case "string", "bytes":
		return CType{Kind: CTypeString, Name: "gzc_str_t"}
	case "bool":
		return CType{Kind: CTypeBool, Name: "bool"}
	case "int32", "uint32":
		return CType{Kind: CTypeI32, Name: "int32_t"}
	case "int64", "uint64":
		return CType{Kind: CTypeI64, Name: "int64_t"}
	case "double":
		return CType{Kind: CTypeF64, Name: "double"}
	default:
		if enums[field.Type] {
			return CType{Kind: CTypeI32, Name: "int32_t"}
		}
		return CType{Kind: CTypeJSON, Name: "gzc_json_t"}
	}
}
