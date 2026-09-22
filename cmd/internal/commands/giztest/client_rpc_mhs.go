package giztestcmd

import (
	"context"
	"encoding/json"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func installMhs(handlers *gizcli.DeviceControlHandlers, method string, response any) error {
	scripted, err := deviceControlErrorResponse(response)
	if err != nil {
		return err
	}
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	if method == "client.mhs.v0.read" {
		result := new(rpcpb.ClientMhsV0ReadResponse)
		if scripted == nil {
			if err := protojson.Unmarshal(data, result); err != nil {
				return err
			}
		}
		handlers.ReadMhsStates = func(context.Context, *rpcpb.ClientMhsV0ReadRequest) (*rpcpb.ClientMhsV0ReadResponse, error) {
			return proto.CloneOf(result), scripted
		}
	} else {
		result := new(rpcpb.ClientMhsV0WriteResponse)
		if scripted == nil {
			if err := protojson.Unmarshal(data, result); err != nil {
				return err
			}
		}
		handlers.WriteMhsStates = func(context.Context, *rpcpb.ClientMhsV0WriteRequest) (*rpcpb.ClientMhsV0WriteResponse, error) {
			return proto.CloneOf(result), scripted
		}
	}
	return nil
}
