package rpcapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/structpb"
)

const rpcPayloadProtoPackage = "gizclaw.rpc.v1."

func encodeRPCRequestPayload(method RPCMethod, params *RPCRequest_Params) ([]byte, error) {
	if params == nil {
		return nil, nil
	}
	messageName, ok := rpcRequestPayloadMessages[method]
	if !ok {
		return nil, fmt.Errorf("rpc: request payload schema not found for method %s", method)
	}
	return encodeRPCPayloadMessage(messageName, params.union)
}

func decodeRPCRequestPayload(method RPCMethod, payload []byte) (*RPCRequest_Params, error) {
	messageName, ok := rpcRequestPayloadMessages[method]
	if !ok {
		return nil, fmt.Errorf("rpc: request payload schema not found for method %s", method)
	}
	data, err := decodeRPCPayloadMessage(messageName, payload)
	if err != nil {
		return nil, err
	}
	return &RPCRequest_Params{union: data}, nil
}

func encodeRPCResponsePayload(method RPCMethod, result *RPCResponse_Result) ([]byte, error) {
	if result == nil {
		return nil, nil
	}
	messageName, ok := rpcResponsePayloadMessages[method]
	if !ok {
		return nil, fmt.Errorf("rpc: response payload schema not found for method %s", method)
	}
	return encodeRPCPayloadMessage(messageName, result.union)
}

func decodeRPCResponsePayload(method RPCMethod, payload []byte) (*RPCResponse_Result, error) {
	messageName, ok := rpcResponsePayloadMessages[method]
	if !ok {
		return nil, fmt.Errorf("rpc: response payload schema not found for method %s", method)
	}
	data, err := decodeRPCPayloadMessage(messageName, payload)
	if err != nil {
		return nil, err
	}
	return &RPCResponse_Result{union: data}, nil
}

// EncodeRPCRequestPayloadJSON converts JSON-shaped request params into the
// method-specific protobuf payload used on the Peer RPC wire.
func EncodeRPCRequestPayloadJSON(method RPCMethod, jsonPayload []byte) ([]byte, error) {
	params := &RPCRequest_Params{union: append([]byte(nil), jsonPayload...)}
	return encodeRPCRequestPayload(method, params)
}

// DecodeRPCResponsePayloadJSON converts a method-specific protobuf response
// payload into the JSON-shaped result used by test and CLI harnesses.
func DecodeRPCResponsePayloadJSON(method RPCMethod, payload []byte) ([]byte, error) {
	result, err := decodeRPCResponsePayload(method, payload)
	if err != nil {
		return nil, err
	}
	return result.MarshalJSON()
}

func encodeRPCPayloadMessage(messageName string, jsonPayload []byte) ([]byte, error) {
	msg, err := newRPCPayloadMessage(messageName)
	if err != nil {
		return nil, err
	}
	value, err := decodeJSONObject(jsonPayload)
	if err != nil {
		return nil, fmt.Errorf("rpc: decode %s payload JSON: %w", messageName, err)
	}
	if err := fillProtoMessage(msg, value); err != nil {
		return nil, fmt.Errorf("rpc: encode %s payload: %w", messageName, err)
	}
	out, err := proto.Marshal(msg.Interface())
	if err != nil {
		return nil, fmt.Errorf("rpc: marshal %s payload: %w", messageName, err)
	}
	return out, nil
}

func decodeRPCPayloadMessage(messageName string, payload []byte) ([]byte, error) {
	msg, err := newRPCPayloadMessage(messageName)
	if err != nil {
		return nil, err
	}
	if len(payload) > 0 {
		if err := proto.Unmarshal(payload, msg.Interface()); err != nil {
			return nil, fmt.Errorf("rpc: unmarshal %s payload: %w", messageName, err)
		}
	}
	value, err := protoMessageJSONValue(msg)
	if err != nil {
		return nil, fmt.Errorf("rpc: decode %s payload: %w", messageName, err)
	}
	out, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("rpc: marshal %s payload JSON: %w", messageName, err)
	}
	return out, nil
}

func newRPCPayloadMessage(messageName string) (*dynamicpb.Message, error) {
	fullName := protoreflect.FullName(rpcPayloadProtoPackage + messageName)
	mt, err := protoregistry.GlobalTypes.FindMessageByName(fullName)
	if err != nil {
		return nil, fmt.Errorf("rpc: protobuf message %s not registered: %w", fullName, err)
	}
	return dynamicpb.NewMessage(mt.Descriptor()), nil
}

func decodeJSONObject(data []byte) (any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	var value any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func fillProtoMessage(msg protoreflect.Message, value any) error {
	desc := msg.Descriptor()
	if fd := singleValueField(desc); fd != nil {
		return setProtoField(msg, fd, value, nil)
	}
	if isOneofValueWrapper(desc) {
		return setOneofWrapper(msg, value, nil)
	}
	obj, ok := value.(map[string]any)
	if !ok {
		if value == nil {
			return nil
		}
		return fmt.Errorf("expected object for %s, got %T", desc.FullName(), value)
	}
	fields := desc.Fields()
	for name, fieldValue := range obj {
		if fieldValue == nil {
			continue
		}
		fd := fields.ByJSONName(name)
		if fd == nil {
			fd = fields.ByName(protoreflect.Name(name))
		}
		if fd == nil {
			continue
		}
		if err := setProtoField(msg, fd, fieldValue, obj); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func singleValueField(desc protoreflect.MessageDescriptor) protoreflect.FieldDescriptor {
	if desc.Fields().Len() != 1 {
		return nil
	}
	fd := desc.Fields().Get(0)
	if fd.Name() != "value" || fd.ContainingOneof() != nil {
		return nil
	}
	return fd
}

func isOneofValueWrapper(desc protoreflect.MessageDescriptor) bool {
	if desc.Oneofs().Len() != 1 || desc.Oneofs().Get(0).Name() != "value" {
		return false
	}
	for i := 0; i < desc.Fields().Len(); i++ {
		if desc.Fields().Get(i).ContainingOneof() == nil {
			return false
		}
	}
	return desc.Fields().Len() > 0
}

func setOneofWrapper(msg protoreflect.Message, value any, parent map[string]any) error {
	obj, _ := value.(map[string]any)
	if field := discriminatorOneofField(msg.Descriptor(), obj, parent); field != nil {
		return setProtoField(msg, field, value, nil)
	}
	var best protoreflect.FieldDescriptor
	bestScore := -1
	fields := msg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		score := 0
		if obj != nil && fd.Message() != nil {
			childFields := fd.Message().Fields()
			for key := range obj {
				if childFields.ByJSONName(key) != nil || childFields.ByName(protoreflect.Name(key)) != nil {
					score++
				}
			}
		}
		if score > bestScore {
			best = fd
			bestScore = score
		}
	}
	if best == nil {
		return fmt.Errorf("no oneof payload candidate for %s", msg.Descriptor().FullName())
	}
	return setProtoField(msg, best, value, nil)
}

func discriminatorOneofField(desc protoreflect.MessageDescriptor, value, parent map[string]any) protoreflect.FieldDescriptor {
	var discriminator string
	switch desc.Name() {
	case "CredentialBody":
		if parent != nil {
			discriminator, _ = parent["provider"].(string)
		}
	case "ModelProviderData", "VoiceProviderData":
		if parent != nil {
			provider, _ := parent["provider"].(map[string]any)
			discriminator, _ = provider["kind"].(string)
		}
	case "WorkspaceParameters":
		if value != nil {
			discriminator, _ = value["agent_type"].(string)
		}
		if discriminator == "" && parent != nil {
			discriminator, _ = parent["agent_type"].(string)
		}
	}
	if discriminator == "" {
		return nil
	}
	fieldName := oneofDiscriminatorFieldName(desc.Name(), discriminator)
	if fieldName == "" {
		return nil
	}
	return desc.Fields().ByName(protoreflect.Name(fieldName))
}

func oneofDiscriminatorFieldName(desc protoreflect.Name, discriminator string) string {
	switch desc {
	case "CredentialBody":
		switch discriminator {
		case "openai":
			return "open_aicredential_body"
		case "gemini":
			return "gemini_credential_body"
		case "dashscope":
			return "dash_scope_credential_body"
		case "minimax":
			return "mini_max_credential_body"
		case "volc":
			return "volc_credential_body"
		}
	case "ModelProviderData":
		switch discriminator {
		case "gemini-tenant":
			return "gemini_tenant_model_provider_data"
		case "dashscope-tenant":
			return "dash_scope_tenant_model_provider_data"
		case "openai-tenant":
			return "open_aitenant_model_provider_data"
		case "volc-tenant":
			return "volc_tenant_model_provider_data"
		}
	case "VoiceProviderData":
		switch discriminator {
		case "gemini-tenant":
			return "gemini_tenant_voice_provider_data"
		case "dashscope-tenant":
			return "dash_scope_tenant_voice_provider_data"
		case "openai-tenant":
			return "open_aitenant_voice_provider_data"
		case "minimax-tenant":
			return "mini_max_tenant_voice_provider_data"
		case "volc-tenant":
			return "volc_tenant_voice_provider_data"
		}
	case "WorkspaceParameters":
		switch discriminator {
		case "flowcraft":
			return "flowcraft_workspace_parameters"
		case "doubao-realtime":
			return "doubao_realtime_workspace_parameters"
		case "ast-translate":
			return "asttranslate_workspace_parameters"
		case "chatroom":
			return "chat_room_workspace_parameters"
		}
	}
	return ""
}

func setProtoField(msg protoreflect.Message, fd protoreflect.FieldDescriptor, value any, parent map[string]any) error {
	if fd.IsMap() {
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("expected map, got %T", value)
		}
		m := msg.Mutable(fd).Map()
		for key, item := range obj {
			mapKey, err := protoMapKey(fd.MapKey(), key)
			if err != nil {
				return err
			}
			mapValue, err := protoFieldValue(fd.MapValue(), item, nil)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			m.Set(mapKey, mapValue)
		}
		return nil
	}
	if fd.IsList() {
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("expected list, got %T", value)
		}
		list := msg.Mutable(fd).List()
		for i, item := range items {
			fieldValue, err := protoFieldValue(fd, item, nil)
			if err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
			list.Append(fieldValue)
		}
		return nil
	}
	fieldValue, err := protoFieldValue(fd, value, parent)
	if err != nil {
		return err
	}
	msg.Set(fd, fieldValue)
	return nil
}

func protoMapKey(fd protoreflect.FieldDescriptor, value string) (protoreflect.MapKey, error) {
	switch fd.Kind() {
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(value).MapKey(), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		n, err := strconv.ParseInt(value, 10, 32)
		return protoreflect.ValueOfInt32(int32(n)).MapKey(), err
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		n, err := strconv.ParseInt(value, 10, 64)
		return protoreflect.ValueOfInt64(n).MapKey(), err
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		n, err := strconv.ParseUint(value, 10, 32)
		return protoreflect.ValueOfUint32(uint32(n)).MapKey(), err
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		n, err := strconv.ParseUint(value, 10, 64)
		return protoreflect.ValueOfUint64(n).MapKey(), err
	case protoreflect.BoolKind:
		b, err := strconv.ParseBool(value)
		return protoreflect.ValueOfBool(b).MapKey(), err
	default:
		return protoreflect.MapKey{}, fmt.Errorf("unsupported map key kind %s", fd.Kind())
	}
}

func protoFieldValue(fd protoreflect.FieldDescriptor, value any, parent map[string]any) (protoreflect.Value, error) {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		b, ok := value.(bool)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected bool, got %T", value)
		}
		return protoreflect.ValueOfBool(b), nil
	case protoreflect.EnumKind:
		number, err := protoEnumNumber(fd.Enum(), value)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfEnum(number), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		n, err := jsonInt(value, 32)
		return protoreflect.ValueOfInt32(int32(n)), err
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		n, err := jsonInt(value, 64)
		return protoreflect.ValueOfInt64(n), err
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		n, err := jsonUint(value, 32)
		return protoreflect.ValueOfUint32(uint32(n)), err
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		n, err := jsonUint(value, 64)
		return protoreflect.ValueOfUint64(n), err
	case protoreflect.FloatKind:
		n, err := jsonFloat(value, 32)
		return protoreflect.ValueOfFloat32(float32(n)), err
	case protoreflect.DoubleKind:
		n, err := jsonFloat(value, 64)
		return protoreflect.ValueOfFloat64(n), err
	case protoreflect.StringKind:
		s, ok := value.(string)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected string, got %T", value)
		}
		return protoreflect.ValueOfString(s), nil
	case protoreflect.BytesKind:
		s, ok := value.(string)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected base64 string, got %T", value)
		}
		data, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfBytes(data), nil
	case protoreflect.MessageKind, protoreflect.GroupKind:
		if fd.Message().FullName() == "google.protobuf.Struct" {
			obj, ok := value.(map[string]any)
			if !ok {
				return protoreflect.Value{}, fmt.Errorf("expected object for google.protobuf.Struct, got %T", value)
			}
			normalized, err := normalizeStructMap(obj)
			if err != nil {
				return protoreflect.Value{}, err
			}
			st, err := structpb.NewStruct(normalized)
			if err != nil {
				return protoreflect.Value{}, err
			}
			return protoreflect.ValueOfMessage(st.ProtoReflect()), nil
		}
		child := dynamicpb.NewMessage(fd.Message())
		if isOneofValueWrapper(child.Descriptor()) {
			if err := setOneofWrapper(child, value, parent); err != nil {
				return protoreflect.Value{}, err
			}
			return protoreflect.ValueOfMessage(child), nil
		}
		if err := fillProtoMessage(child, value); err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfMessage(child), nil
	default:
		return protoreflect.Value{}, fmt.Errorf("unsupported protobuf kind %s", fd.Kind())
	}
}

func normalizeStructMap(obj map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(obj))
	for key, value := range obj {
		normalized, err := normalizeStructValue(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out[key] = normalized
	}
	return out, nil
}

func normalizeStructValue(value any) (any, error) {
	switch v := value.(type) {
	case json.Number:
		n, err := strconv.ParseFloat(v.String(), 64)
		if err != nil {
			return nil, err
		}
		return n, nil
	case map[string]any:
		return normalizeStructMap(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			normalized, err := normalizeStructValue(item)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out[i] = normalized
		}
		return out, nil
	default:
		return value, nil
	}
}

func protoEnumNumber(desc protoreflect.EnumDescriptor, value any) (protoreflect.EnumNumber, error) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return 0, nil
		}
		want := strings.ToUpper(strings.ReplaceAll(v, "-", "_"))
		wantCompact := strings.ReplaceAll(want, "_", "")
		values := desc.Values()
		for i := 0; i < values.Len(); i++ {
			ev := values.Get(i)
			name := enumJSONName(desc, ev)
			if name == want || strings.ReplaceAll(name, "_", "") == wantCompact {
				return ev.Number(), nil
			}
		}
		return 0, fmt.Errorf("unknown enum value %q for %s", v, desc.FullName())
	case json.Number:
		n, err := v.Int64()
		return protoreflect.EnumNumber(n), err
	case float64:
		if math.Trunc(v) != v {
			return 0, fmt.Errorf("expected integer enum number, got %v", v)
		}
		return protoreflect.EnumNumber(v), nil
	default:
		return 0, fmt.Errorf("expected enum string or number, got %T", value)
	}
}

func enumJSONName(desc protoreflect.EnumDescriptor, value protoreflect.EnumValueDescriptor) string {
	name := string(value.Name())
	prefix := enumValuePrefix(desc)
	if strings.HasPrefix(name, prefix) {
		return strings.TrimPrefix(name, prefix)
	}
	return name
}

func enumValuePrefix(desc protoreflect.EnumDescriptor) string {
	values := desc.Values()
	if values.Len() == 0 {
		return ""
	}
	prefix := string(values.Get(0).Name())
	for i := 1; i < values.Len(); i++ {
		name := string(values.Get(i).Name())
		for !strings.HasPrefix(name, prefix) && prefix != "" {
			prefix = prefix[:len(prefix)-1]
		}
	}
	if idx := strings.LastIndex(prefix, "_"); idx >= 0 {
		return prefix[:idx+1]
	}
	return ""
}

func jsonInt(value any, bitSize int) (int64, error) {
	switch v := value.(type) {
	case json.Number:
		return v.Int64()
	case float64:
		if math.Trunc(v) != v {
			return 0, fmt.Errorf("expected integer, got %v", v)
		}
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, bitSize)
	default:
		return 0, fmt.Errorf("expected integer, got %T", value)
	}
}

func jsonUint(value any, bitSize int) (uint64, error) {
	switch v := value.(type) {
	case json.Number:
		return strconv.ParseUint(v.String(), 10, bitSize)
	case float64:
		if math.Trunc(v) != v {
			return 0, fmt.Errorf("expected unsigned integer, got %v", v)
		}
		return uint64(v), nil
	case string:
		return strconv.ParseUint(v, 10, bitSize)
	default:
		return 0, fmt.Errorf("expected unsigned integer, got %T", value)
	}
}

func jsonFloat(value any, bitSize int) (float64, error) {
	switch v := value.(type) {
	case json.Number:
		return strconv.ParseFloat(v.String(), bitSize)
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(v, bitSize)
	default:
		return 0, fmt.Errorf("expected number, got %T", value)
	}
}

func protoMessageJSONValue(msg protoreflect.Message) (any, error) {
	desc := msg.Descriptor()
	if fd := singleValueField(desc); fd != nil {
		return protoFieldJSONValue(msg, fd)
	}
	if isOneofValueWrapper(desc) {
		fields := desc.Fields()
		for i := 0; i < fields.Len(); i++ {
			fd := fields.Get(i)
			if msg.Has(fd) {
				return protoFieldJSONValue(msg, fd)
			}
		}
		return map[string]any{}, nil
	}
	out := make(map[string]any)
	fields := desc.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if !protoFieldPresent(msg, fd) {
			continue
		}
		value, err := protoFieldJSONValue(msg, fd)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fd.JSONName(), err)
		}
		out[protoJSONFieldName(fd)] = value
	}
	return out, nil
}

func protoFieldPresent(msg protoreflect.Message, fd protoreflect.FieldDescriptor) bool {
	if fd.IsList() {
		return msg.Get(fd).List().Len() > 0
	}
	if fd.IsMap() {
		return msg.Get(fd).Map().Len() > 0
	}
	if fd.HasPresence() {
		return msg.Has(fd)
	}
	return !protoValueIsZero(fd, msg.Get(fd))
}

func protoFieldJSONValue(msg protoreflect.Message, fd protoreflect.FieldDescriptor) (any, error) {
	value := msg.Get(fd)
	if fd.IsMap() {
		out := make(map[string]any)
		var err error
		value.Map().Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
			var item any
			item, err = protoScalarJSONValue(fd.MapValue(), v)
			if err != nil {
				return false
			}
			out[fmt.Sprint(k.Interface())] = item
			return true
		})
		return out, err
	}
	if fd.IsList() {
		list := value.List()
		out := make([]any, 0, list.Len())
		for i := 0; i < list.Len(); i++ {
			item, err := protoScalarJSONValue(fd, list.Get(i))
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, nil
	}
	return protoScalarJSONValue(fd, value)
}

func protoScalarJSONValue(fd protoreflect.FieldDescriptor, value protoreflect.Value) (any, error) {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return value.Bool(), nil
	case protoreflect.EnumKind:
		ev := fd.Enum().Values().ByNumber(value.Enum())
		if ev == nil || ev.Number() == 0 {
			return "", nil
		}
		return enumValueJSONString(fd.Enum(), ev), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return int(value.Int()), nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return value.Int(), nil
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return uint(value.Uint()), nil
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return value.Uint(), nil
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return value.Float(), nil
	case protoreflect.StringKind:
		return value.String(), nil
	case protoreflect.BytesKind:
		return base64.StdEncoding.EncodeToString(value.Bytes()), nil
	case protoreflect.MessageKind, protoreflect.GroupKind:
		if fd.Message().FullName() == "google.protobuf.Struct" {
			data, err := proto.Marshal(value.Message().Interface())
			if err != nil {
				return nil, err
			}
			var st structpb.Struct
			if err := proto.Unmarshal(data, &st); err != nil {
				return nil, err
			}
			return st.AsMap(), nil
		}
		return protoMessageJSONValue(value.Message())
	default:
		return nil, fmt.Errorf("unsupported protobuf kind %s", fd.Kind())
	}
}

func protoJSONFieldName(fd protoreflect.FieldDescriptor) string {
	if name, ok := protoJSONFieldNameOverrides[fd.FullName()]; ok {
		return name
	}
	name := string(fd.Name())
	if fd.JSONName() != defaultProtoJSONName(name) {
		return fd.JSONName()
	}
	return name
}

var protoJSONFieldNameOverrides = map[protoreflect.FullName]string{
	"gizclaw.rpc.v1.DoubaoRealtimeJSONSchema.additional_properties": "additionalProperties",
	"gizclaw.rpc.v1.DoubaoRealtimeJSONSchema.any_of":                "anyOf",
	"gizclaw.rpc.v1.DoubaoRealtimeJSONSchema.max_length":            "maxLength",
	"gizclaw.rpc.v1.DoubaoRealtimeJSONSchema.min_length":            "minLength",
}

func defaultProtoJSONName(name string) string {
	var out strings.Builder
	upperNext := false
	for _, r := range name {
		if r == '_' {
			upperNext = true
			continue
		}
		if upperNext && r >= 'a' && r <= 'z' {
			out.WriteRune(r - ('a' - 'A'))
		} else {
			out.WriteRune(r)
		}
		upperNext = false
	}
	return out.String()
}

func enumValueJSONString(desc protoreflect.EnumDescriptor, value protoreflect.EnumValueDescriptor) string {
	name := enumJSONName(desc, value)
	if mapped, ok := enumJSONValueOverrides[name]; ok {
		return mapped
	}
	return strings.ToLower(name)
}

var enumJSONValueOverrides = map[string]string{
	"AST_TRANSLATE":     "ast-translate",
	"DASHSCOPE_TENANT":  "dashscope-tenant",
	"DASH_SCOPE_TENANT": "dashscope-tenant",
	"DOUBAO_REALTIME":   "doubao-realtime",
	"GEMINI_TENANT":     "gemini-tenant",
	"MINI_MAX":          "minimax",
	"MINIMAX_TENANT":    "minimax-tenant",
	"OPENAI_TENANT":     "openai-tenant",
	"PUSH_TO_TALK":      "push-to-talk",
	"VOLC_TENANT":       "volc-tenant",
}

func protoValueIsZero(fd protoreflect.FieldDescriptor, value protoreflect.Value) bool {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return !value.Bool()
	case protoreflect.EnumKind:
		return value.Enum() == 0
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return value.Int() == 0
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return value.Uint() == 0
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return value.Float() == 0
	case protoreflect.StringKind:
		return value.String() == ""
	case protoreflect.BytesKind:
		return len(value.Bytes()) == 0
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return !value.Message().IsValid()
	default:
		return false
	}
}
