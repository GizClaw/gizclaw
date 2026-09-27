package rpcapi

import (
	"fmt"
	"reflect"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// ClientHwdMetadata returns the protobuf message binding for a hardware type.
func ClientHwdMetadata(hwd rpcpb.ClientHwd) (*rpcpb.ClientHwdOptions, error) {
	value := hwd.Descriptor().Values().ByNumber(hwd.Number())
	if value == nil || !proto.HasExtension(value.Options(), rpcpb.E_ClientHwd) {
		return nil, fmt.Errorf("rpc: unknown client HWD %d", hwd)
	}
	meta, ok := proto.GetExtension(value.Options(), rpcpb.E_ClientHwd).(*rpcpb.ClientHwdOptions)
	if !ok || meta.GetName() == "" || meta.GetReadResponse() == "" || (meta.GetWriteRequest() == "") != (meta.GetWriteResponse() == "") {
		return nil, fmt.Errorf("rpc: invalid client HWD metadata for %d", hwd)
	}
	return proto.Clone(meta).(*rpcpb.ClientHwdOptions), nil
}

// ClientHwdByName resolves the stable HWD name in a RuntimeProfile manifest.
func ClientHwdByName(name string) (rpcpb.ClientHwd, error) {
	values := rpcpb.ClientHwd(0).Descriptor().Values()
	for i := 0; i < values.Len(); i++ {
		hwd := rpcpb.ClientHwd(values.Get(i).Number())
		meta, err := ClientHwdMetadata(hwd)
		if err == nil && meta.Name == name {
			return hwd, nil
		}
	}
	return 0, fmt.Errorf("rpc: unknown client HWD %q", name)
}

func clientHwdMessage(name string, data []byte) (proto.Message, error) {
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(rpcPayloadProtoPackage + name))
	if err != nil {
		return nil, err
	}
	message := typ.New().Interface()
	if err := proto.Unmarshal(data, message); err != nil {
		return nil, err
	}
	return message, nil
}

// ClientHwdReadResponseMessage decodes one typed hardware observation.
func ClientHwdReadResponseMessage(hwd rpcpb.ClientHwd, data []byte) (proto.Message, error) {
	meta, err := ClientHwdMetadata(hwd)
	if err != nil {
		return nil, err
	}
	return clientHwdMessage(meta.ReadResponse, data)
}

// ClientHwdWriteRequestMessage builds a typed write from validated JSON fields.
func ClientHwdWriteRequestMessage(hwd rpcpb.ClientHwd, value any) (proto.Message, error) {
	meta, err := ClientHwdMetadata(hwd)
	if err != nil {
		return nil, err
	}
	if meta.WriteRequest == "" {
		return nil, fmt.Errorf("rpc: client HWD %s is read-only", meta.Name)
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(rpcPayloadProtoPackage + meta.WriteRequest))
	if err != nil {
		return nil, err
	}
	message := typ.New()
	if err := fillProtoMessageFromGo(message, reflect.ValueOf(value), reflect.Value{}); err != nil {
		return nil, err
	}
	return message.Interface(), nil
}

// ClientHwdWriteRequestJSON decodes a strictly named protobuf JSON object.
func ClientHwdWriteRequestJSON(hwd rpcpb.ClientHwd, data []byte) (proto.Message, error) {
	meta, err := ClientHwdMetadata(hwd)
	if err != nil {
		return nil, err
	}
	if meta.WriteRequest == "" {
		return nil, fmt.Errorf("rpc: client HWD %s is read-only", meta.Name)
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(rpcPayloadProtoPackage + meta.WriteRequest))
	if err != nil {
		return nil, err
	}
	message := typ.New().Interface()
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(data, message); err != nil {
		return nil, err
	}
	return message, nil
}

// ClientHwdWriteRequestFromBytes decodes a device-facing typed write.
func ClientHwdWriteRequestFromBytes(hwd rpcpb.ClientHwd, data []byte) (proto.Message, error) {
	meta, err := ClientHwdMetadata(hwd)
	if err != nil {
		return nil, err
	}
	if meta.WriteRequest == "" {
		return nil, fmt.Errorf("rpc: client HWD %s is read-only", meta.Name)
	}
	return clientHwdMessage(meta.WriteRequest, data)
}

// ClientHwdWriteResponseMessage decodes the applied hardware state.
func ClientHwdWriteResponseMessage(hwd rpcpb.ClientHwd, data []byte) (proto.Message, error) {
	meta, err := ClientHwdMetadata(hwd)
	if err != nil {
		return nil, err
	}
	if meta.WriteResponse == "" {
		return nil, fmt.Errorf("rpc: client HWD %s is read-only", meta.Name)
	}
	return clientHwdMessage(meta.WriteResponse, data)
}

// ValidateMhsHwdReadRequest checks a device-facing HWD read envelope.
func ValidateMhsHwdReadRequest(request *rpcpb.ClientMhsV0ReadRequest) error {
	if request == nil || !ValidMhsKey(request.Id) {
		return fmt.Errorf("invalid HWD read request")
	}
	_, err := ClientHwdMetadata(request.Hwd)
	return err
}

// ValidateMhsHwdWriteRequest checks a writable HWD and its typed payload.
func ValidateMhsHwdWriteRequest(request *rpcpb.ClientMhsV0WriteRequest) error {
	if request == nil || !ValidMhsKey(request.Id) || len(request.Payload) == 0 {
		return fmt.Errorf("invalid HWD write request")
	}
	message, err := ClientHwdWriteRequestFromBytes(request.Hwd, request.Payload)
	if err != nil {
		return err
	}
	switch value := message.(type) {
	case *rpcpb.DisplayHwdWriteRequest:
		if value.BrightnessPercent == nil && value.Enabled == nil && value.OffTimeoutMs == nil || value.BrightnessPercent != nil && *value.BrightnessPercent > 100 {
			return fmt.Errorf("invalid display HWD write")
		}
	case *rpcpb.LedHwdWriteRequest:
		if value.Enabled == nil && value.BrightnessPercent == nil || value.BrightnessPercent != nil && *value.BrightnessPercent > 100 {
			return fmt.Errorf("invalid led HWD write")
		}
	case *rpcpb.SpeakerHwdWriteRequest:
		if value.VolumePercent == nil && value.Muted == nil || value.VolumePercent != nil && *value.VolumePercent > 100 {
			return fmt.Errorf("invalid speaker HWD write")
		}
	default:
		return fmt.Errorf("unsupported HWD write %T", message)
	}
	return nil
}

// ValidateMhsHwdReadResponse checks the registered protobuf response shape.
func ValidateMhsHwdReadResponse(hwd rpcpb.ClientHwd, response *rpcpb.ClientMhsV0ReadResponse) error {
	if response == nil || len(response.Payload) == 0 {
		return fmt.Errorf("invalid HWD read response")
	}
	message, err := ClientHwdReadResponseMessage(hwd, response.Payload)
	if err != nil {
		return err
	}
	if proto.Size(message) == 0 {
		return fmt.Errorf("empty HWD read response")
	}
	return nil
}

// ValidateMhsHwdWriteResponse checks the registered applied-value shape.
func ValidateMhsHwdWriteResponse(hwd rpcpb.ClientHwd, response *rpcpb.ClientMhsV0WriteResponse) error {
	if response == nil || len(response.Payload) == 0 {
		return fmt.Errorf("invalid HWD write response")
	}
	message, err := ClientHwdWriteResponseMessage(hwd, response.Payload)
	if err != nil {
		return err
	}
	switch value := message.(type) {
	case *rpcpb.DisplayHwdWriteResponse:
		if value.Applied != nil && proto.Size(value.Applied) > 0 {
			return nil
		}
	case *rpcpb.LedHwdWriteResponse:
		if value.Applied != nil && proto.Size(value.Applied) > 0 {
			return nil
		}
	case *rpcpb.SpeakerHwdWriteResponse:
		if value.Applied != nil && proto.Size(value.Applied) > 0 {
			return nil
		}
	}
	return fmt.Errorf("invalid HWD applied value")
}
