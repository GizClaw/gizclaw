package peerresource

import (
	"context"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func appConfigServer(t *testing.T, revision string, config apitypes.RuntimeProfileAppConfig) *Server {
	t.Helper()
	profile := apitypes.RuntimeProfile{
		Id:       "e2e-profile",
		Revision: revision,
		Spec:     apitypes.RuntimeProfileSpec{AppConfig: &config},
	}
	return &Server{RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile }}
}

func appConfigListResponse(t *testing.T, response *rpcapi.RPCResponse) rpcapi.AppConfigListResponse {
	t.Helper()
	if response.Error != nil || response.Result == nil {
		t.Fatalf("app config list response = %#v", response)
	}
	decoded, err := response.Result.AsAppConfigListResponse()
	if err != nil {
		t.Fatalf("AsAppConfigListResponse() error = %v", err)
	}
	return decoded
}

func TestAppConfigListPagesSortedKeysWithProfileIdentity(t *testing.T) {
	t.Parallel()
	server := appConfigServer(t, "revision-1", apitypes.RuntimeProfileAppConfig{
		"ui.theme": "dark", "audio.gain": "3", "wake.word": "hey",
	})
	limit := 2
	var params rpcapi.RPCPayload
	if err := params.FromAppConfigListRequest(rpcapi.AppConfigListRequest{Limit: &limit}); err != nil {
		t.Fatal(err)
	}
	first := appConfigListResponse(t, server.handleAppConfigList(context.Background(), &rpcapi.RPCRequest{Id: "list", Params: &params}))
	if !reflect.DeepEqual(first.Keys, []string{"audio.gain", "ui.theme"}) || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("first page = %#v", first)
	}
	if first.RuntimeProfileName != "e2e-profile" || first.RuntimeProfileRevision != "revision-1" {
		t.Fatalf("profile identity = %#v", first)
	}

	if err := params.FromAppConfigListRequest(rpcapi.AppConfigListRequest{Cursor: first.NextCursor, Limit: &limit}); err != nil {
		t.Fatal(err)
	}
	second := appConfigListResponse(t, server.handleAppConfigList(context.Background(), &rpcapi.RPCRequest{Id: "list", Params: &params}))
	if !reflect.DeepEqual(second.Keys, []string{"wake.word"}) || second.HasNext || second.NextCursor != nil {
		t.Fatalf("second page = %#v", second)
	}
}

func TestAppConfigListRejectsCursorFromAnotherRevision(t *testing.T) {
	t.Parallel()
	limit := 1
	var params rpcapi.RPCPayload
	if err := params.FromAppConfigListRequest(rpcapi.AppConfigListRequest{Limit: &limit}); err != nil {
		t.Fatal(err)
	}
	first := appConfigListResponse(t, appConfigServer(t, "revision-1", apitypes.RuntimeProfileAppConfig{
		"a.key": "1", "b.key": "2",
	}).handleAppConfigList(context.Background(), &rpcapi.RPCRequest{Id: "list", Params: &params}))
	if first.NextCursor == nil {
		t.Fatalf("first page = %#v, want a cursor", first)
	}
	if err := params.FromAppConfigListRequest(rpcapi.AppConfigListRequest{Cursor: first.NextCursor, Limit: &limit}); err != nil {
		t.Fatal(err)
	}
	updated := appConfigServer(t, "revision-2", apitypes.RuntimeProfileAppConfig{"a.key": "1", "b.key": "2"})
	response := updated.handleAppConfigList(context.Background(), &rpcapi.RPCRequest{Id: "list", Params: &params})
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeAborted {
		t.Fatalf("stale cursor response = %#v, want ABORTED", response)
	}
}

func TestAppConfigListReturnsEmptyPageWhenUnconfigured(t *testing.T) {
	t.Parallel()
	profile := apitypes.RuntimeProfile{Id: "e2e-profile", Revision: "revision-1"}
	server := &Server{RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile }}
	decoded := appConfigListResponse(t, server.handleAppConfigList(context.Background(), &rpcapi.RPCRequest{Id: "list"}))
	if len(decoded.Keys) != 0 || decoded.HasNext || decoded.NextCursor != nil {
		t.Fatalf("unconfigured page = %#v", decoded)
	}
}

func TestAppConfigGetReturnsValueVerbatim(t *testing.T) {
	t.Parallel()
	value := "{\n  \"theme\": \"深色\"\n}\n  "
	server := appConfigServer(t, "revision-1", apitypes.RuntimeProfileAppConfig{"ui.theme": value, "empty": ""})
	for _, testCase := range []struct{ key, want string }{{"ui.theme", value}, {"empty", ""}} {
		var params rpcapi.RPCPayload
		if err := params.FromAppConfigGetRequest(rpcapi.AppConfigGetRequest{Key: testCase.key}); err != nil {
			t.Fatal(err)
		}
		response := server.handleAppConfigGet(context.Background(), &rpcapi.RPCRequest{Id: "get", Params: &params})
		if response.Error != nil || response.Result == nil {
			t.Fatalf("app config get response = %#v", response)
		}
		decoded, err := response.Result.AsAppConfigGetResponse()
		if err != nil {
			t.Fatalf("AsAppConfigGetResponse() error = %v", err)
		}
		if decoded.Value != testCase.want {
			t.Fatalf("app config %q = %q, want %q", testCase.key, decoded.Value, testCase.want)
		}
		if decoded.RuntimeProfileName != "e2e-profile" || decoded.RuntimeProfileRevision != "revision-1" {
			t.Fatalf("profile identity = %#v", decoded)
		}
	}
}

func TestAppConfigGetRejectsMissingAndEmptyKeys(t *testing.T) {
	t.Parallel()
	server := appConfigServer(t, "revision-1", apitypes.RuntimeProfileAppConfig{"ui.theme": "dark"})
	cases := map[string]struct {
		key  string
		want rpcapi.StatusCode
	}{
		"missing":    {key: "missing.key", want: rpcapi.StatusCodeNotFound},
		"empty":      {key: "   ", want: rpcapi.StatusCodeInvalidArgument},
		"unrelated":  {key: "ui", want: rpcapi.StatusCodeNotFound},
		"prefix-key": {key: "ui.theme.dark", want: rpcapi.StatusCodeNotFound},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var params rpcapi.RPCPayload
			if err := params.FromAppConfigGetRequest(rpcapi.AppConfigGetRequest{Key: testCase.key}); err != nil {
				t.Fatal(err)
			}
			response := server.handleAppConfigGet(context.Background(), &rpcapi.RPCRequest{Id: "get", Params: &params})
			if response.Error == nil || response.Error.Code != testCase.want || response.Result != nil {
				t.Fatalf("app config get response = %#v, want %v", response, testCase.want)
			}
		})
	}
}

func TestAppConfigMethodsDispatch(t *testing.T) {
	t.Parallel()
	server := appConfigServer(t, "revision-1", apitypes.RuntimeProfileAppConfig{"ui.theme": "dark"})
	var listParams rpcapi.RPCPayload
	if err := listParams.FromAppConfigListRequest(rpcapi.AppConfigListRequest{}); err != nil {
		t.Fatal(err)
	}
	response, handled, err := server.Dispatch(context.Background(), &rpcapi.RPCRequest{
		Id: "list", Method: rpcapi.RPCMethodServerAppConfigList, Params: &listParams,
	})
	if err != nil || !handled || response.Error != nil {
		t.Fatalf("dispatch list = %#v, handled=%v error=%v", response, handled, err)
	}
	var getParams rpcapi.RPCPayload
	if err := getParams.FromAppConfigGetRequest(rpcapi.AppConfigGetRequest{Key: "ui.theme"}); err != nil {
		t.Fatal(err)
	}
	response, handled, err = server.Dispatch(context.Background(), &rpcapi.RPCRequest{
		Id: "get", Method: rpcapi.RPCMethodServerAppConfigGet, Params: &getParams,
	})
	if err != nil || !handled || response.Error != nil {
		t.Fatalf("dispatch get = %#v, handled=%v error=%v", response, handled, err)
	}
}
