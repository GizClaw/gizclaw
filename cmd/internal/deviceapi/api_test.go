package deviceapi

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func resetRPCHooks(t *testing.T) {
	t.Helper()
	origGetInfo := getServerInfoRPC
	origPutInfo := putServerInfoRPC
	origGetRuntime := getServerRuntimeRPC
	t.Cleanup(func() {
		getServerInfoRPC = origGetInfo
		putServerInfoRPC = origPutInfo
		getServerRuntimeRPC = origGetRuntime
	})
}

func TestGetInfo(t *testing.T) {
	resetRPCHooks(t)
	name := "Pixa"
	getServerInfoRPC = func(_ context.Context, _ *gizcli.Client, id string) (*rpcapi.ServerGetInfoResponse, error) {
		if id != "server.info.get" {
			t.Fatalf("id = %q", id)
		}
		return &rpcapi.ServerGetInfoResponse{Name: &name}, nil
	}
	got, err := GetInfo(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetInfo error = %v", err)
	}
	if got.Name == nil || *got.Name != name {
		t.Fatalf("Name = %v, want %q", got.Name, name)
	}
}

func TestGetInfoPropagatesError(t *testing.T) {
	resetRPCHooks(t)
	getServerInfoRPC = func(context.Context, *gizcli.Client, string) (*rpcapi.ServerGetInfoResponse, error) {
		return nil, errors.New("offline")
	}
	_, err := GetInfo(context.Background(), nil)
	if err == nil || err.Error() != "offline" {
		t.Fatalf("GetInfo error = %v", err)
	}
}

func TestPutInfo(t *testing.T) {
	resetRPCHooks(t)
	name := "Pixa"
	putServerInfoRPC = func(_ context.Context, _ *gizcli.Client, id string, req rpcapi.ServerPutInfoRequest) (*rpcapi.ServerPutInfoResponse, error) {
		if id != "server.info.put" {
			t.Fatalf("id = %q", id)
		}
		if req.Name == nil || *req.Name != name {
			t.Fatalf("request name = %v", req.Name)
		}
		return &rpcapi.ServerPutInfoResponse{Name: req.Name}, nil
	}
	got, err := PutInfo(context.Background(), nil, apitypes.DeviceInfo{Name: &name})
	if err != nil {
		t.Fatalf("PutInfo error = %v", err)
	}
	if got.Name == nil || *got.Name != name {
		t.Fatalf("Name = %v, want %q", got.Name, name)
	}
}

func TestPutInfoPropagatesError(t *testing.T) {
	resetRPCHooks(t)
	putServerInfoRPC = func(context.Context, *gizcli.Client, string, rpcapi.ServerPutInfoRequest) (*rpcapi.ServerPutInfoResponse, error) {
		return nil, errors.New("write failed")
	}
	_, err := PutInfo(context.Background(), nil, apitypes.DeviceInfo{})
	if err == nil || err.Error() != "write failed" {
		t.Fatalf("PutInfo error = %v", err)
	}
}

func TestSetName(t *testing.T) {
	resetRPCHooks(t)
	oldName := "old"
	newName := "new"
	getServerInfoRPC = func(context.Context, *gizcli.Client, string) (*rpcapi.ServerGetInfoResponse, error) {
		return &rpcapi.ServerGetInfoResponse{Name: &oldName}, nil
	}
	putServerInfoRPC = func(_ context.Context, _ *gizcli.Client, _ string, req rpcapi.ServerPutInfoRequest) (*rpcapi.ServerPutInfoResponse, error) {
		if req.Name == nil || *req.Name != newName {
			t.Fatalf("request name = %v", req.Name)
		}
		return &rpcapi.ServerPutInfoResponse{Name: req.Name}, nil
	}
	got, err := SetName(context.Background(), nil, newName)
	if err != nil {
		t.Fatalf("SetName error = %v", err)
	}
	if got.Name == nil || *got.Name != newName {
		t.Fatalf("Name = %v, want %q", got.Name, newName)
	}
}

func TestSetNamePropagatesGetInfoError(t *testing.T) {
	resetRPCHooks(t)
	getServerInfoRPC = func(context.Context, *gizcli.Client, string) (*rpcapi.ServerGetInfoResponse, error) {
		return nil, errors.New("read failed")
	}
	_, err := SetName(context.Background(), nil, "new")
	if err == nil || err.Error() != "read failed" {
		t.Fatalf("SetName error = %v", err)
	}
}

func TestSetNamePropagatesPutInfoError(t *testing.T) {
	resetRPCHooks(t)
	getServerInfoRPC = func(context.Context, *gizcli.Client, string) (*rpcapi.ServerGetInfoResponse, error) {
		return &rpcapi.ServerGetInfoResponse{}, nil
	}
	putServerInfoRPC = func(context.Context, *gizcli.Client, string, rpcapi.ServerPutInfoRequest) (*rpcapi.ServerPutInfoResponse, error) {
		return nil, errors.New("write failed")
	}
	_, err := SetName(context.Background(), nil, "new")
	if err == nil || err.Error() != "write failed" {
		t.Fatalf("SetName error = %v", err)
	}
}

func TestGetRuntime(t *testing.T) {
	resetRPCHooks(t)
	getServerRuntimeRPC = func(_ context.Context, _ *gizcli.Client, id string) (*rpcapi.ServerGetRuntimeResponse, error) {
		if id != "server.runtime.get" {
			t.Fatalf("id = %q", id)
		}
		return &rpcapi.ServerGetRuntimeResponse{}, nil
	}
	if _, err := GetRuntime(context.Background(), nil); err != nil {
		t.Fatalf("GetRuntime error = %v", err)
	}
}

func TestGetRuntimePropagatesError(t *testing.T) {
	resetRPCHooks(t)
	getServerRuntimeRPC = func(context.Context, *gizcli.Client, string) (*rpcapi.ServerGetRuntimeResponse, error) {
		return nil, errors.New("runtime failed")
	}
	_, err := GetRuntime(context.Background(), nil)
	if err == nil || err.Error() != "runtime failed" {
		t.Fatalf("GetRuntime error = %v", err)
	}
}

func TestConvertClientAPIType(t *testing.T) {
	type in struct {
		Name string `json:"name"`
	}
	type out struct {
		Name string `json:"name"`
	}
	got, err := convertClientAPIType[out](in{Name: "demo"})
	if err != nil {
		t.Fatalf("convertClientAPIType error = %v", err)
	}
	if got.Name != "demo" {
		t.Fatalf("Name = %q, want demo", got.Name)
	}
}

func TestConvertClientAPITypeMarshalError(t *testing.T) {
	_, err := convertClientAPIType[struct{}](make(chan int))
	if err == nil {
		t.Fatal("convertClientAPIType should fail for non-marshalable input")
	}
}

func TestConvertClientAPITypeUnmarshalError(t *testing.T) {
	type out struct {
		Count int `json:"count"`
	}
	_, err := convertClientAPIType[out](struct {
		Count string `json:"count"`
	}{Count: "not-a-number"})
	if err == nil {
		t.Fatal("convertClientAPIType should fail for incompatible output type")
	}
}
