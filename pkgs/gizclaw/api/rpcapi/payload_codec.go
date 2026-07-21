package rpcapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/structpb"
)

const rpcPayloadProtoPackage = "gizclaw.rpc.v1."

type decodeRPCPayloadOptions struct {
	emitDefaults bool
}

func encodeRPCRequestPayload(method RPCMethod, params *RPCPayload) ([]byte, error) {
	if params == nil {
		return nil, nil
	}
	messageName, ok := rpcRequestPayloadMessages[method]
	if !ok {
		return nil, fmt.Errorf("rpc: request payload schema not found for method %s", method)
	}
	return params.bytesForMessage(messageName)
}

func decodeRPCRequestPayload(method RPCMethod, payload []byte) (*RPCPayload, error) {
	messageName, ok := rpcRequestPayloadMessages[method]
	if !ok {
		return nil, fmt.Errorf("rpc: request payload schema not found for method %s", method)
	}
	return newRPCPayload(messageName, payload, false), nil
}

func encodeRPCResponsePayload(method RPCMethod, result *RPCPayload) ([]byte, error) {
	if result == nil {
		return nil, nil
	}
	messageName, ok := rpcResponsePayloadMessages[method]
	if !ok {
		return nil, fmt.Errorf("rpc: response payload schema not found for method %s", method)
	}
	return result.bytesForMessage(messageName)
}

func decodeRPCResponsePayload(method RPCMethod, payload []byte) (*RPCPayload, error) {
	messageName, ok := rpcResponsePayloadMessages[method]
	if !ok {
		return nil, fmt.Errorf("rpc: response payload schema not found for method %s", method)
	}
	return newRPCPayload(messageName, payload, true), nil
}

func newRPCPayload(messageName string, payload []byte, emitDefaults bool) *RPCPayload {
	return &RPCPayload{
		payload:      append([]byte(nil), payload...),
		messageName:  messageName,
		emitDefaults: emitDefaults,
	}
}

func (t RPCPayload) bytesForMessage(messageName string) ([]byte, error) {
	if t.messageName == "" {
		return nil, fmt.Errorf("rpc: payload message name is required")
	}
	if t.messageName != "" && t.messageName != messageName {
		return nil, fmt.Errorf("rpc: payload contains %s, want %s", t.messageName, messageName)
	}
	return append([]byte(nil), t.payload...), nil
}

func (t *RPCPayload) encode(messageName string, value any) error {
	if protoValue, ok, err := rpcPayloadProtoMessage(messageName, value); ok || err != nil {
		if err != nil {
			return err
		}
		payload, err := proto.Marshal(protoValue)
		if err != nil {
			return fmt.Errorf("rpc: marshal %s payload: %w", messageName, err)
		}
		t.payload = payload
		t.messageName = messageName
		t.emitDefaults = false
		return nil
	}
	msg, err := newRPCPayloadMessage(messageName)
	if err != nil {
		return err
	}
	if err := fillProtoMessageFromGo(msg, reflect.ValueOf(value), reflect.Value{}); err != nil {
		return fmt.Errorf("rpc: encode %s payload: %w", messageName, err)
	}
	payload, err := proto.Marshal(msg.Interface())
	if err != nil {
		return fmt.Errorf("rpc: marshal %s payload: %w", messageName, err)
	}
	t.payload = payload
	t.messageName = messageName
	t.emitDefaults = false
	return nil
}

func (t RPCPayload) decode(messageName string, out any) error {
	if t.messageName == "" {
		return fmt.Errorf("rpc: payload message name is required")
	}
	if t.messageName != "" && t.messageName != messageName {
		return fmt.Errorf("rpc: payload contains %s, want %s", t.messageName, messageName)
	}
	if protoOut, ok, err := rpcPayloadProtoMessage(messageName, out); ok || err != nil {
		if err != nil {
			return err
		}
		if len(t.payload) > 0 {
			if err := proto.Unmarshal(t.payload, protoOut); err != nil {
				return fmt.Errorf("rpc: unmarshal %s payload: %w", messageName, err)
			}
		}
		return nil
	}
	msg, err := newRPCPayloadMessage(messageName)
	if err != nil {
		return err
	}
	if len(t.payload) > 0 {
		if err := proto.Unmarshal(t.payload, msg.Interface()); err != nil {
			return fmt.Errorf("rpc: unmarshal %s payload: %w", messageName, err)
		}
	}
	if err := fillGoValueFromProto(reflect.ValueOf(out), msg, decodeRPCPayloadOptions{emitDefaults: t.emitDefaults}); err != nil {
		return fmt.Errorf("rpc: decode %s payload: %w", messageName, err)
	}
	return nil
}

func (t *RPCPayload) merge(messageName string, value any) error {
	msg, err := newRPCPayloadMessage(messageName)
	if err != nil {
		return err
	}
	if len(t.payload) > 0 {
		if err := proto.Unmarshal(t.payload, msg.Interface()); err != nil {
			return fmt.Errorf("rpc: unmarshal %s payload: %w", messageName, err)
		}
	}
	if protoValue, ok, err := rpcPayloadProtoMessage(messageName, value); ok || err != nil {
		if err != nil {
			return err
		}
		patch, err := newRPCPayloadMessage(messageName)
		if err != nil {
			return err
		}
		payload, err := proto.Marshal(protoValue)
		if err != nil {
			return fmt.Errorf("rpc: marshal %s payload: %w", messageName, err)
		}
		if err := proto.Unmarshal(payload, patch.Interface()); err != nil {
			return fmt.Errorf("rpc: merge %s payload: %w", messageName, err)
		}
		proto.Merge(msg.Interface(), patch.Interface())
		payload, err = proto.Marshal(msg.Interface())
		if err != nil {
			return fmt.Errorf("rpc: marshal %s payload: %w", messageName, err)
		}
		t.payload = payload
		t.messageName = messageName
		return nil
	}
	patch, err := newRPCPayloadMessage(messageName)
	if err != nil {
		return err
	}
	if err := fillProtoMessageFromGo(patch, reflect.ValueOf(value), reflect.Value{}); err != nil {
		return fmt.Errorf("rpc: merge %s payload: %w", messageName, err)
	}
	proto.Merge(msg.Interface(), patch.Interface())
	payload, err := proto.Marshal(msg.Interface())
	if err != nil {
		return fmt.Errorf("rpc: marshal %s payload: %w", messageName, err)
	}
	t.payload = payload
	t.messageName = messageName
	return nil
}

func rpcPayloadProtoMessage(messageName string, value any) (proto.Message, bool, error) {
	msg, ok := value.(proto.Message)
	if !ok {
		return nil, false, nil
	}
	if value := reflect.ValueOf(msg); value.Kind() == reflect.Pointer && value.IsNil() {
		return nil, true, fmt.Errorf("rpc: %s payload proto message is nil", messageName)
	}
	if msg == nil || !msg.ProtoReflect().IsValid() {
		return nil, true, fmt.Errorf("rpc: %s payload proto message is nil", messageName)
	}
	fullName := protoreflect.FullName(rpcPayloadProtoPackage + messageName)
	if got := msg.ProtoReflect().Descriptor().FullName(); got != fullName {
		return nil, true, fmt.Errorf("rpc: payload contains %s, want %s", got, fullName)
	}
	return msg, true, nil
}

func newRPCPayloadMessage(messageName string) (*dynamicpb.Message, error) {
	fullName := protoreflect.FullName(rpcPayloadProtoPackage + messageName)
	mt, err := protoregistry.GlobalTypes.FindMessageByName(fullName)
	if err != nil {
		return nil, fmt.Errorf("rpc: protobuf message %s not registered: %w", fullName, err)
	}
	return dynamicpb.NewMessage(mt.Descriptor()), nil
}

var timeType = reflect.TypeOf(time.Time{})

var rpcPayloadUnionValueTypes = map[protoreflect.Name]reflect.Type{
	"ASTTranslateInternalSpeakerParameters": reflect.TypeOf(ASTTranslateInternalSpeakerParameters{}),
	"ASTTranslateExternalVoiceParameters":   reflect.TypeOf(ASTTranslateExternalVoiceParameters{}),
	"OpenAICredentialBody":                  reflect.TypeOf(OpenAICredentialBody{}),
	"GeminiCredentialBody":                  reflect.TypeOf(GeminiCredentialBody{}),
	"DashScopeCredentialBody":               reflect.TypeOf(DashScopeCredentialBody{}),
	"DeepSeekCredentialBody":                reflect.TypeOf(DeepSeekCredentialBody{}),
	"MiniMaxCredentialBody":                 reflect.TypeOf(MiniMaxCredentialBody{}),
	"VolcCredentialBody":                    reflect.TypeOf(VolcCredentialBody{}),
	"GeminiTenantModelProviderData":         reflect.TypeOf(GeminiTenantModelProviderData{}),
	"DashScopeTenantModelProviderData":      reflect.TypeOf(DashScopeTenantModelProviderData{}),
	"DeepSeekTenantModelProviderData":       reflect.TypeOf(DeepSeekTenantModelProviderData{}),
	"OpenAITenantModelProviderData":         reflect.TypeOf(OpenAITenantModelProviderData{}),
	"MiniMaxTenantModelProviderData":        reflect.TypeOf(MiniMaxTenantModelProviderData{}),
	"VolcTenantModelProviderData":           reflect.TypeOf(VolcTenantModelProviderData{}),
	"GeminiTenantVoiceProviderData":         reflect.TypeOf(GeminiTenantVoiceProviderData{}),
	"DashScopeTenantVoiceProviderData":      reflect.TypeOf(DashScopeTenantVoiceProviderData{}),
	"OpenAITenantVoiceProviderData":         reflect.TypeOf(OpenAITenantVoiceProviderData{}),
	"MiniMaxTenantVoiceProviderData":        reflect.TypeOf(MiniMaxTenantVoiceProviderData{}),
	"VolcTenantVoiceProviderData":           reflect.TypeOf(VolcTenantVoiceProviderData{}),
	"FlowcraftWorkspaceParameters":          reflect.TypeOf(FlowcraftWorkspaceParameters{}),
	"DoubaoRealtimeWorkspaceParameters":     reflect.TypeOf(DoubaoRealtimeWorkspaceParameters{}),
	"ASTTranslateWorkspaceParameters":       reflect.TypeOf(ASTTranslateWorkspaceParameters{}),
	"ChatRoomWorkspaceParameters":           reflect.TypeOf(ChatRoomWorkspaceParameters{}),
	"PetWorkspaceParameters":                reflect.TypeOf(PetWorkspaceParameters{}),
}

func fillProtoMessageFromGo(msg protoreflect.Message, value reflect.Value, parent reflect.Value) error {
	value = indirectGoValue(value)
	if !value.IsValid() {
		return nil
	}
	desc := msg.Descriptor()
	if isOneofValueWrapper(desc) {
		stored := goUnionStoredValue(value)
		if !stored.IsValid() {
			return nil
		}
		return setOneofWrapperFromGo(msg, stored, parent)
	}
	if fd := singleValueField(desc); fd != nil && !goStructHasJSONField(value, "value") {
		return setProtoFieldFromGo(msg, fd, value, parent)
	}
	if value.Kind() == reflect.Map {
		fields := desc.Fields()
		iter := value.MapRange()
		for iter.Next() {
			key := iter.Key()
			if key.Kind() != reflect.String {
				return fmt.Errorf("expected string map key for %s, got %s", desc.FullName(), key.Kind())
			}
			name := key.String()
			fd := fields.ByJSONName(name)
			if fd == nil {
				fd = fields.ByName(protoreflect.Name(name))
			}
			if fd == nil {
				continue
			}
			if err := setProtoFieldFromGo(msg, fd, iter.Value(), value); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		return nil
	}
	if value.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct for %s, got %s", desc.FullName(), value.Kind())
	}
	if err := validateGoOneofFields(desc, value); err != nil {
		return err
	}
	fields := desc.Fields()
	valueType := value.Type()
	for i := 0; i < value.NumField(); i++ {
		sf := valueType.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		name, ok := goJSONFieldName(sf)
		if !ok {
			continue
		}
		fd := fields.ByJSONName(name)
		if fd == nil {
			fd = fields.ByName(protoreflect.Name(name))
		}
		if fd == nil {
			continue
		}
		fieldValue := value.Field(i)
		if goValueAbsent(fieldValue) {
			continue
		}
		if err := setProtoFieldFromGo(msg, fd, fieldValue, value); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func validateGoOneofFields(desc protoreflect.MessageDescriptor, value reflect.Value) error {
	for i := 0; i < desc.Oneofs().Len(); i++ {
		oneof := desc.Oneofs().Get(i)
		var selected []string
		for j := 0; j < oneof.Fields().Len(); j++ {
			field := oneof.Fields().Get(j)
			candidate := goFieldByJSONName(value, field.JSONName())
			if !candidate.IsValid() {
				candidate = goFieldByJSONName(value, string(field.Name()))
			}
			if candidate.IsValid() && !goValueAbsent(candidate) {
				selected = append(selected, field.JSONName())
			}
		}
		if len(selected) > 1 {
			return fmt.Errorf("oneof %s has multiple values: %s", oneof.FullName(), strings.Join(selected, ", "))
		}
	}
	return nil
}

func setProtoFieldFromGo(msg protoreflect.Message, fd protoreflect.FieldDescriptor, value reflect.Value, parent reflect.Value) error {
	value = indirectGoValue(value)
	if !value.IsValid() {
		return nil
	}
	if fd.IsMap() {
		if value.Kind() != reflect.Map {
			return fmt.Errorf("expected map, got %s", value.Kind())
		}
		m := msg.Mutable(fd).Map()
		iter := value.MapRange()
		for iter.Next() {
			mapKey, err := protoMapKeyFromGo(fd.MapKey(), iter.Key())
			if err != nil {
				return err
			}
			mapValue, err := protoFieldValueFromGo(fd.MapValue(), iter.Value(), reflect.Value{})
			if err != nil {
				return fmt.Errorf("%v: %w", iter.Key().Interface(), err)
			}
			m.Set(mapKey, mapValue)
		}
		return nil
	}
	if fd.IsList() {
		if value.Kind() != reflect.Slice && value.Kind() != reflect.Array {
			return fmt.Errorf("expected list, got %s", value.Kind())
		}
		list := msg.Mutable(fd).List()
		for i := 0; i < value.Len(); i++ {
			item, err := protoFieldValueFromGo(fd, value.Index(i), reflect.Value{})
			if err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
			list.Append(item)
		}
		return nil
	}
	fieldValue, err := protoFieldValueFromGo(fd, value, parent)
	if err != nil {
		return err
	}
	msg.Set(fd, fieldValue)
	return nil
}

func setOneofWrapperFromGo(msg protoreflect.Message, value reflect.Value, parent reflect.Value) error {
	if field := discriminatorOneofFieldFromGo(msg.Descriptor(), value, parent); field != nil {
		return setProtoFieldFromGo(msg, field, value, reflect.Value{})
	}
	fields := msg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.Message() != nil && goValueLooksLikeProtoMessage(value, fd.Message()) {
			return setProtoFieldFromGo(msg, fd, value, reflect.Value{})
		}
	}
	return fmt.Errorf("no oneof payload candidate for %s", msg.Descriptor().FullName())
}

func discriminatorOneofFieldFromGo(desc protoreflect.MessageDescriptor, value reflect.Value, parent reflect.Value) protoreflect.FieldDescriptor {
	var discriminator string
	switch desc.Name() {
	case "CredentialBody":
		discriminator = goStringField(parent, "provider")
	case "ModelProviderData", "VoiceProviderData":
		provider := goFieldByJSONName(parent, "provider")
		discriminator = goStringField(provider, "kind")
	case "WorkspaceParameters":
		discriminator = goStringField(value, "agent_type")
		if discriminator == "" {
			discriminator = goStringField(parent, "agent_type")
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

func protoFieldValueFromGo(fd protoreflect.FieldDescriptor, value reflect.Value, parent reflect.Value) (protoreflect.Value, error) {
	value = indirectGoValue(value)
	if !value.IsValid() {
		return protoreflect.Value{}, nil
	}
	if value.Type() == timeType && fd.Kind() == protoreflect.StringKind {
		return protoreflect.ValueOfString(value.Interface().(time.Time).Format(time.RFC3339Nano)), nil
	}
	switch fd.Kind() {
	case protoreflect.BoolKind:
		if value.Kind() != reflect.Bool {
			return protoreflect.Value{}, fmt.Errorf("expected bool, got %s", value.Kind())
		}
		return protoreflect.ValueOfBool(value.Bool()), nil
	case protoreflect.EnumKind:
		number, err := protoEnumNumberFromGo(fd.Enum(), value)
		return protoreflect.ValueOfEnum(number), err
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		n, err := goInt(value, 32)
		return protoreflect.ValueOfInt32(int32(n)), err
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		n, err := goInt(value, 64)
		return protoreflect.ValueOfInt64(n), err
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		n, err := goUint(value, 32)
		return protoreflect.ValueOfUint32(uint32(n)), err
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		n, err := goUint(value, 64)
		return protoreflect.ValueOfUint64(n), err
	case protoreflect.FloatKind:
		n, err := goFloat(value, 32)
		return protoreflect.ValueOfFloat32(float32(n)), err
	case protoreflect.DoubleKind:
		n, err := goFloat(value, 64)
		return protoreflect.ValueOfFloat64(n), err
	case protoreflect.StringKind:
		if value.Kind() != reflect.String {
			return protoreflect.Value{}, fmt.Errorf("expected string, got %s", value.Kind())
		}
		return protoreflect.ValueOfString(value.String()), nil
	case protoreflect.BytesKind:
		if value.Kind() == reflect.Slice && value.Type().Elem().Kind() == reflect.Uint8 {
			return protoreflect.ValueOfBytes(value.Bytes()), nil
		}
		if value.Kind() == reflect.String {
			data, err := base64.StdEncoding.DecodeString(value.String())
			if err != nil {
				return protoreflect.Value{}, err
			}
			return protoreflect.ValueOfBytes(data), nil
		}
		return protoreflect.Value{}, fmt.Errorf("expected bytes, got %s", value.Kind())
	case protoreflect.MessageKind, protoreflect.GroupKind:
		if fd.Message().FullName() == "google.protobuf.Struct" {
			st, err := structFromGoValue(value)
			if err != nil {
				return protoreflect.Value{}, err
			}
			return protoreflect.ValueOfMessage(st.ProtoReflect()), nil
		}
		child := dynamicpb.NewMessage(fd.Message())
		if err := fillProtoMessageFromGo(child, value, parent); err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfMessage(child), nil
	default:
		return protoreflect.Value{}, fmt.Errorf("unsupported protobuf kind %s", fd.Kind())
	}
}

func fillGoValueFromProto(target reflect.Value, msg protoreflect.Message, opts decodeRPCPayloadOptions) error {
	if target.Kind() != reflect.Pointer || target.IsNil() {
		return fmt.Errorf("target must be a non-nil pointer")
	}
	target = indirectGoValue(target)
	desc := msg.Descriptor()
	if fd := singleValueField(desc); fd != nil && !goStructHasJSONField(target, "value") {
		return setGoValueFromProto(target, fd, msg.Get(fd), opts)
	}
	if isOneofValueWrapper(desc) {
		return setGoUnionFromProto(target, msg, opts)
	}
	if target.Kind() == reflect.Map {
		target.Set(reflect.MakeMap(target.Type()))
		fields := desc.Fields()
		for i := 0; i < fields.Len(); i++ {
			fd := fields.Get(i)
			if !protoFieldPresent(msg, fd, opts) {
				continue
			}
			item, err := protoFieldGoInterface(fd, msg.Get(fd), opts)
			if err != nil {
				return fmt.Errorf("%s: %w", protoJSONFieldName(fd), err)
			}
			target.SetMapIndex(reflect.ValueOf(protoJSONFieldName(fd)), reflect.ValueOf(item))
		}
		return nil
	}
	if target.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct target for %s, got %s", desc.FullName(), target.Kind())
	}
	fields := desc.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if !protoFieldPresent(msg, fd, opts) {
			continue
		}
		targetField := goFieldByJSONName(target, protoJSONFieldName(fd))
		if !targetField.IsValid() || !targetField.CanSet() {
			continue
		}
		if err := setGoValueFromProto(targetField, fd, msg.Get(fd), opts); err != nil {
			return fmt.Errorf("%s: %w", protoJSONFieldName(fd), err)
		}
	}
	return nil
}

func setGoValueFromProto(target reflect.Value, fd protoreflect.FieldDescriptor, value protoreflect.Value, opts decodeRPCPayloadOptions) error {
	if target.Kind() == reflect.Pointer {
		if target.IsNil() {
			target.Set(reflect.New(target.Type().Elem()))
		}
		return setGoValueFromProto(target.Elem(), fd, value, opts)
	}
	if fd.IsList() {
		if target.Kind() != reflect.Slice {
			return fmt.Errorf("expected slice target, got %s", target.Kind())
		}
		list := value.List()
		out := reflect.MakeSlice(target.Type(), 0, list.Len())
		for i := 0; i < list.Len(); i++ {
			elem := reflect.New(target.Type().Elem()).Elem()
			if err := setGoSingularValueFromProto(elem, fd, list.Get(i), opts); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
			out = reflect.Append(out, elem)
		}
		target.Set(out)
		return nil
	}
	if fd.IsMap() {
		if target.Kind() != reflect.Map {
			return fmt.Errorf("expected map target, got %s", target.Kind())
		}
		out := reflect.MakeMapWithSize(target.Type(), value.Map().Len())
		var err error
		value.Map().Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
			key := reflect.New(target.Type().Key()).Elem()
			if err = setGoMapKeyFromProto(key, fd.MapKey(), k); err != nil {
				return false
			}
			elem := reflect.New(target.Type().Elem()).Elem()
			if err = setGoSingularValueFromProto(elem, fd.MapValue(), v, opts); err != nil {
				return false
			}
			out.SetMapIndex(key, elem)
			return true
		})
		if err != nil {
			return err
		}
		target.Set(out)
		return nil
	}
	return setGoSingularValueFromProto(target, fd, value, opts)
}

func setGoSingularValueFromProto(target reflect.Value, fd protoreflect.FieldDescriptor, value protoreflect.Value, opts decodeRPCPayloadOptions) error {
	if target.Kind() == reflect.Interface {
		item, err := protoFieldGoValueFromProto(fd, value, opts)
		if err != nil {
			return err
		}
		target.Set(reflect.ValueOf(item))
		return nil
	}
	if target.Type() == timeType && fd.Kind() == protoreflect.StringKind {
		if value.String() == "" {
			target.Set(reflect.Zero(target.Type()))
			return nil
		}
		parsed, err := time.Parse(time.RFC3339Nano, value.String())
		if err != nil {
			return err
		}
		target.Set(reflect.ValueOf(parsed))
		return nil
	}
	switch fd.Kind() {
	case protoreflect.BoolKind:
		target.SetBool(value.Bool())
	case protoreflect.EnumKind:
		enumValue := ""
		if ev := fd.Enum().Values().ByNumber(value.Enum()); ev != nil && ev.Number() != 0 {
			enumValue = enumValueJSONString(fd.Enum(), ev)
		}
		target.SetString(enumValue)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		target.SetInt(value.Int())
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind, protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		target.SetUint(value.Uint())
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		target.SetFloat(value.Float())
	case protoreflect.StringKind:
		target.SetString(value.String())
	case protoreflect.BytesKind:
		if target.Kind() == reflect.Slice && target.Type().Elem().Kind() == reflect.Uint8 {
			target.SetBytes(append([]byte(nil), value.Bytes()...))
		} else if target.Kind() == reflect.String {
			target.SetString(base64.StdEncoding.EncodeToString(value.Bytes()))
		} else {
			return fmt.Errorf("expected bytes target, got %s", target.Kind())
		}
	case protoreflect.MessageKind, protoreflect.GroupKind:
		if fd.Message().FullName() == "google.protobuf.Struct" {
			return setGoStructValue(target, value.Message())
		}
		return fillGoValueFromProto(target.Addr(), value.Message(), opts)
	default:
		return fmt.Errorf("unsupported protobuf kind %s", fd.Kind())
	}
	return nil
}

func protoFieldGoInterface(fd protoreflect.FieldDescriptor, value protoreflect.Value, opts decodeRPCPayloadOptions) (any, error) {
	return protoFieldGoValueFromProto(fd, value, opts)
}

func protoFieldGoValueFromProto(fd protoreflect.FieldDescriptor, value protoreflect.Value, opts decodeRPCPayloadOptions) (any, error) {
	if fd.IsMap() {
		out := make(map[string]any)
		var err error
		value.Map().Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
			var item any
			item, err = protoScalarGoValue(fd.MapValue(), v, opts)
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
			item, err := protoScalarGoValue(fd, list.Get(i), opts)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, nil
	}
	return protoScalarGoValue(fd, value, opts)
}

func setGoUnionFromProto(target reflect.Value, msg protoreflect.Message, opts decodeRPCPayloadOptions) error {
	target = indirectGoValue(target)
	fields := msg.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if !msg.Has(fd) {
			continue
		}
		valueField := target.FieldByName("Value")
		if valueField.IsValid() && valueField.CanSet() && valueField.Kind() == reflect.Interface {
			item, err := goUnionValueFromProto(fd, msg.Get(fd), opts)
			if err != nil {
				return err
			}
			valueField.Set(reflect.ValueOf(item))
			return nil
		}
		targetField := goFieldByProtoMessageName(target, fd.Message().Name())
		if !targetField.IsValid() || !targetField.CanSet() {
			return fmt.Errorf("no Go union target for %s", fd.Message().FullName())
		}
		return setGoValueFromProto(targetField, fd, msg.Get(fd), opts)
	}
	return nil
}

func goUnionValueFromProto(fd protoreflect.FieldDescriptor, value protoreflect.Value, opts decodeRPCPayloadOptions) (any, error) {
	valueType, ok := rpcPayloadUnionValueTypes[fd.Message().Name()]
	if !ok {
		return nil, fmt.Errorf("no Go union value type for %s", fd.Message().FullName())
	}
	target := reflect.New(valueType).Elem()
	if err := setGoValueFromProto(target, fd, value, opts); err != nil {
		return nil, err
	}
	return target.Interface(), nil
}

func indirectGoValue(value reflect.Value) reflect.Value {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	return value
}

func setGoMapKeyFromProto(target reflect.Value, fd protoreflect.FieldDescriptor, value protoreflect.MapKey) error {
	switch fd.Kind() {
	case protoreflect.StringKind:
		target.SetString(value.String())
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		target.SetInt(value.Int())
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind, protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		target.SetUint(value.Uint())
	case protoreflect.BoolKind:
		target.SetBool(value.Bool())
	default:
		return fmt.Errorf("unsupported map key kind %s", fd.Kind())
	}
	return nil
}

func goValueAbsent(value reflect.Value) bool {
	if !value.IsValid() {
		return true
	}
	if goUnionEmpty(value) {
		return true
	}
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func goUnionStoredValue(value reflect.Value) reflect.Value {
	value = indirectGoValue(value)
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	field := value.FieldByName("Value")
	if !field.IsValid() || field.Kind() != reflect.Interface || field.IsNil() {
		return reflect.Value{}
	}
	return field.Elem()
}

func goUnionEmpty(value reflect.Value) bool {
	value = indirectGoValue(value)
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return false
	}
	field := value.FieldByName("Value")
	return field.IsValid() && field.Kind() == reflect.Interface && field.IsNil()
}

func goJSONFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	if idx := strings.IndexByte(tag, ','); idx >= 0 {
		tag = tag[:idx]
	}
	if tag == "" {
		tag = field.Name
	}
	return tag, true
}

func goStructHasJSONField(value reflect.Value, name string) bool {
	value = indirectGoValue(value)
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return false
	}
	valueType := value.Type()
	for i := 0; i < valueType.NumField(); i++ {
		fieldName, ok := goJSONFieldName(valueType.Field(i))
		if ok && fieldName == name {
			return true
		}
	}
	return false
}

func goFieldByJSONName(value reflect.Value, name string) reflect.Value {
	value = indirectGoValue(value)
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	valueType := value.Type()
	for i := 0; i < value.NumField(); i++ {
		sf := valueType.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		fieldName, ok := goJSONFieldName(sf)
		if ok && fieldName == name {
			return value.Field(i)
		}
	}
	return reflect.Value{}
}

func goFieldByProtoMessageName(value reflect.Value, name protoreflect.Name) reflect.Value {
	value = indirectGoValue(value)
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	want := strings.ToLower(strings.ReplaceAll(string(name), "_", ""))
	valueType := value.Type()
	for i := 0; i < value.NumField(); i++ {
		sf := valueType.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		if strings.ToLower(sf.Name) == want {
			return value.Field(i)
		}
	}
	return reflect.Value{}
}

func goStringField(value reflect.Value, name string) string {
	field := goFieldByJSONName(value, name)
	field = indirectGoValue(field)
	if field.IsValid() && field.Kind() == reflect.String {
		return field.String()
	}
	return ""
}

func goValueLooksLikeProtoMessage(value reflect.Value, desc protoreflect.MessageDescriptor) bool {
	value = indirectGoValue(value)
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return false
	}
	valueName := strings.ToLower(value.Type().Name())
	descName := strings.ToLower(strings.ReplaceAll(string(desc.Name()), "_", ""))
	return valueName == descName
}

func protoMapKeyFromGo(fd protoreflect.FieldDescriptor, value reflect.Value) (protoreflect.MapKey, error) {
	value = indirectGoValue(value)
	switch fd.Kind() {
	case protoreflect.StringKind:
		if value.Kind() != reflect.String {
			return protoreflect.MapKey{}, fmt.Errorf("expected string map key, got %s", value.Kind())
		}
		return protoreflect.ValueOfString(value.String()).MapKey(), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		n, err := goInt(value, 32)
		return protoreflect.ValueOfInt32(int32(n)).MapKey(), err
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		n, err := goInt(value, 64)
		return protoreflect.ValueOfInt64(n).MapKey(), err
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		n, err := goUint(value, 32)
		return protoreflect.ValueOfUint32(uint32(n)).MapKey(), err
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		n, err := goUint(value, 64)
		return protoreflect.ValueOfUint64(n).MapKey(), err
	case protoreflect.BoolKind:
		if value.Kind() != reflect.Bool {
			return protoreflect.MapKey{}, fmt.Errorf("expected bool map key, got %s", value.Kind())
		}
		return protoreflect.ValueOfBool(value.Bool()).MapKey(), nil
	default:
		return protoreflect.MapKey{}, fmt.Errorf("unsupported map key kind %s", fd.Kind())
	}
}

func protoEnumNumberFromGo(desc protoreflect.EnumDescriptor, value reflect.Value) (protoreflect.EnumNumber, error) {
	value = indirectGoValue(value)
	switch value.Kind() {
	case reflect.String:
		return protoEnumNumber(desc, value.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return protoreflect.EnumNumber(value.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return protoreflect.EnumNumber(value.Uint()), nil
	default:
		return 0, fmt.Errorf("expected enum string or number, got %s", value.Kind())
	}
}

func goInt(value reflect.Value, bitSize int) (int64, error) {
	value = indirectGoValue(value)
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := value.Int()
		if strconv.IntSize >= bitSize || bitSize == 64 {
			return n, nil
		}
		if n < -(1<<(bitSize-1)) || n >= 1<<(bitSize-1) {
			return 0, fmt.Errorf("integer overflows %d bits", bitSize)
		}
		return n, nil
	case reflect.String:
		return strconv.ParseInt(value.String(), 10, bitSize)
	default:
		return 0, fmt.Errorf("expected integer, got %s", value.Kind())
	}
}

func goUint(value reflect.Value, bitSize int) (uint64, error) {
	value = indirectGoValue(value)
	switch value.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n := value.Uint()
		if bitSize < 64 && n >= 1<<bitSize {
			return 0, fmt.Errorf("unsigned integer overflows %d bits", bitSize)
		}
		return n, nil
	case reflect.String:
		return strconv.ParseUint(value.String(), 10, bitSize)
	default:
		return 0, fmt.Errorf("expected unsigned integer, got %s", value.Kind())
	}
}

func goFloat(value reflect.Value, bitSize int) (float64, error) {
	value = indirectGoValue(value)
	switch value.Kind() {
	case reflect.Float32, reflect.Float64:
		return value.Float(), nil
	case reflect.String:
		return strconv.ParseFloat(value.String(), bitSize)
	default:
		return 0, fmt.Errorf("expected number, got %s", value.Kind())
	}
}

func structFromGoValue(value reflect.Value) (*structpb.Struct, error) {
	value = indirectGoValue(value)
	if !value.IsValid() {
		return structpb.NewStruct(map[string]any{})
	}
	if value.Type() == reflect.TypeOf(structpb.Struct{}) {
		st := value.Interface().(structpb.Struct)
		return &st, nil
	}
	if value.CanInterface() {
		if _, ok := value.Interface().(json.Marshaler); ok {
			data, err := json.Marshal(value.Interface())
			if err != nil {
				return nil, err
			}
			var out map[string]any
			if err := json.Unmarshal(data, &out); err != nil {
				return nil, err
			}
			return structpb.NewStruct(out)
		}
	}
	if value.Kind() == reflect.Map {
		out := make(map[string]any, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			key := iter.Key()
			if key.Kind() != reflect.String {
				return nil, fmt.Errorf("google.protobuf.Struct map key must be string, got %s", key.Kind())
			}
			item, err := goValueInterface(iter.Value())
			if err != nil {
				return nil, err
			}
			out[key.String()] = item
		}
		return structpb.NewStruct(out)
	}
	item, err := goValueInterface(value)
	if err != nil {
		return nil, err
	}
	obj, ok := item.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected object for google.protobuf.Struct, got %T", item)
	}
	return structpb.NewStruct(obj)
}

func goValueInterface(value reflect.Value) (any, error) {
	value = indirectGoValue(value)
	if !value.IsValid() {
		return nil, nil
	}
	switch value.Kind() {
	case reflect.Bool:
		return value.Bool(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value.Uint(), nil
	case reflect.Float32, reflect.Float64:
		return value.Float(), nil
	case reflect.String:
		return value.String(), nil
	case reflect.Slice, reflect.Array:
		out := make([]any, value.Len())
		for i := 0; i < value.Len(); i++ {
			item, err := goValueInterface(value.Index(i))
			if err != nil {
				return nil, err
			}
			out[i] = item
		}
		return out, nil
	case reflect.Map:
		out := make(map[string]any, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			if iter.Key().Kind() != reflect.String {
				return nil, fmt.Errorf("map key must be string, got %s", iter.Key().Kind())
			}
			item, err := goValueInterface(iter.Value())
			if err != nil {
				return nil, err
			}
			out[iter.Key().String()] = item
		}
		return out, nil
	case reflect.Struct:
		out := make(map[string]any)
		valueType := value.Type()
		for i := 0; i < value.NumField(); i++ {
			sf := valueType.Field(i)
			if sf.PkgPath != "" {
				continue
			}
			name, ok := goJSONFieldName(sf)
			if !ok || goValueAbsent(value.Field(i)) {
				continue
			}
			item, err := goValueInterface(value.Field(i))
			if err != nil {
				return nil, err
			}
			out[name] = item
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported struct value kind %s", value.Kind())
	}
}

func setGoStructValue(target reflect.Value, msg protoreflect.Message) error {
	st, ok := msg.Interface().(*structpb.Struct)
	if !ok {
		data, err := proto.Marshal(msg.Interface())
		if err != nil {
			return err
		}
		var decoded structpb.Struct
		if err := proto.Unmarshal(data, &decoded); err != nil {
			return err
		}
		st = &decoded
	}
	if target.Kind() == reflect.Map {
		values := st.AsMap()
		target.Set(reflect.MakeMapWithSize(target.Type(), len(values)))
		for key, item := range values {
			itemValue := reflect.Zero(target.Type().Elem())
			if item != nil {
				itemValue = reflect.ValueOf(item)
			}
			target.SetMapIndex(reflect.ValueOf(key), itemValue)
		}
		return nil
	}
	if target.Type() == reflect.TypeOf(structpb.Struct{}) {
		target.Set(reflect.ValueOf(*st))
		return nil
	}
	if target.CanAddr() && target.Addr().CanInterface() {
		if _, ok := target.Addr().Interface().(json.Unmarshaler); !ok {
			return fmt.Errorf("unsupported google.protobuf.Struct target %s", target.Type())
		}
		data, err := json.Marshal(st.AsMap())
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, target.Addr().Interface()); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("unsupported google.protobuf.Struct target %s", target.Type())
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
		case "deepseek":
			return "deep_seek_credential_body"
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
		case "deepseek-tenant":
			return "deep_seek_tenant_model_provider_data"
		case "openai-tenant":
			return "open_aitenant_model_provider_data"
		case "minimax-tenant":
			return "mini_max_tenant_model_provider_data"
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

func protoEnumNumber(desc protoreflect.EnumDescriptor, value string) (protoreflect.EnumNumber, error) {
	if value == "" {
		return 0, nil
	}
	want := strings.ToUpper(strings.ReplaceAll(value, "-", "_"))
	wantCompact := strings.ReplaceAll(want, "_", "")
	values := desc.Values()
	for i := 0; i < values.Len(); i++ {
		ev := values.Get(i)
		name := enumJSONName(desc, ev)
		if name == want || strings.ReplaceAll(name, "_", "") == wantCompact || enumValueJSONString(desc, ev) == value {
			return ev.Number(), nil
		}
	}
	return 0, fmt.Errorf("unknown enum value %q for %s", value, desc.FullName())
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

func protoMessageGoValue(msg protoreflect.Message, opts decodeRPCPayloadOptions) (any, error) {
	desc := msg.Descriptor()
	if fd := singleValueField(desc); fd != nil {
		return protoFieldGoValue(msg, fd, opts)
	}
	if isOneofValueWrapper(desc) {
		fields := desc.Fields()
		for i := 0; i < fields.Len(); i++ {
			fd := fields.Get(i)
			if msg.Has(fd) {
				return protoFieldGoValue(msg, fd, opts)
			}
		}
		return map[string]any{}, nil
	}
	out := make(map[string]any)
	fields := desc.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if !protoFieldPresent(msg, fd, opts) {
			continue
		}
		value, err := protoFieldGoValue(msg, fd, opts)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fd.JSONName(), err)
		}
		out[protoJSONFieldName(fd)] = value
	}
	return out, nil
}

func protoFieldPresent(msg protoreflect.Message, fd protoreflect.FieldDescriptor, opts decodeRPCPayloadOptions) bool {
	if fd.IsList() {
		if protoOptionalRepeatedField(fd) && msg.Get(fd).List().Len() == 0 {
			return false
		}
		return opts.emitDefaults || msg.Get(fd).List().Len() > 0
	}
	if fd.IsMap() {
		return opts.emitDefaults || msg.Get(fd).Map().Len() > 0
	}
	if fd.HasPresence() {
		return msg.Has(fd)
	}
	if opts.emitDefaults {
		return true
	}
	return !protoValueIsZero(fd, msg.Get(fd))
}

func protoOptionalRepeatedField(fd protoreflect.FieldDescriptor) bool {
	switch fd.FullName() {
	case "gizclaw.rpc.v1.DoubaoRealtimeJSONSchema.any_of",
		"gizclaw.rpc.v1.DoubaoRealtimeJSONSchema.enum_values",
		"gizclaw.rpc.v1.DoubaoRealtimeJSONSchema.required":
		return true
	default:
		return false
	}
}

func protoFieldGoValue(msg protoreflect.Message, fd protoreflect.FieldDescriptor, opts decodeRPCPayloadOptions) (any, error) {
	value := msg.Get(fd)
	if fd.IsMap() {
		out := make(map[string]any)
		var err error
		value.Map().Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
			var item any
			item, err = protoScalarGoValue(fd.MapValue(), v, opts)
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
			item, err := protoScalarGoValue(fd, list.Get(i), opts)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, nil
	}
	return protoScalarGoValue(fd, value, opts)
}

func protoScalarGoValue(fd protoreflect.FieldDescriptor, value protoreflect.Value, opts decodeRPCPayloadOptions) (any, error) {
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
		return protoMessageGoValue(value.Message(), opts)
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
	"DEEPSEEK_TENANT":   "deepseek-tenant",
	"DOUBAO_REALTIME":   "doubao-realtime",
	"EDGE_NODE":         "edge-node",
	"GEMINI_TENANT":     "gemini-tenant",
	"MINI_MAX":          "minimax",
	"MINIMAX_TENANT":    "minimax-tenant",
	"OPENAI_TENANT":     "openai-tenant",
	"PUSH_TO_TALK":      "push-to-talk",
	"VOLC_TENANT":       "volc-tenant",
	"ZH_CN":             "zh-CN",
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
