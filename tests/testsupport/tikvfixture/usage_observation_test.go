package tikvfixture

import (
	"testing"

	"github.com/pingcap/kvproto/pkg/errorpb"
	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/tikv/client-go/v2/tikvrpc"
)

func TestUsageGetObservationMatchesNativeSDKEmptyValueSemantics(t *testing.T) {
	for _, test := range []struct {
		name                               string
		response                           *kvrpcpb.GetResponse
		missing, positive, flags, failures int64
	}{
		{name: "TiKV8-empty-without-flag", response: &kvrpcpb.GetResponse{}, missing: 1},
		{name: "empty-with-flag", response: &kvrpcpb.GetResponse{NotFound: true}, missing: 1, flags: 1},
		{name: "actual-value", response: &kvrpcpb.GetResponse{Value: []byte{1, 0, 0}}, positive: 1},
		{name: "region-error-is-not-absence", response: &kvrpcpb.GetResponse{RegionError: &errorpb.Error{Message: "not ready"}}, failures: 1},
		{name: "key-error-is-not-absence", response: &kvrpcpb.GetResponse{Error: &kvrpcpb.KeyError{Abort: "actual failure"}}, failures: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			rule := &usageRule{}
			response := &tikvrpc.Response{Resp: test.response}
			usageComplete([]*usageRule{rule}, response, nil)
			state := rule.observation()
			if state.MissingResponses != test.missing || state.PositiveResponses != test.positive || state.NativeNotFoundFlags != test.flags || state.Errors != test.failures {
				t.Fatalf("actual native response classified incorrectly: %+v", state)
			}
			if response.Resp != test.response {
				t.Fatal("observation replaced the native response")
			}
		})
	}
}

func TestUsageFailureDiagnosticsOmitSecretsAndSuccessfulBodies(t *testing.T) {
	if value := usageFailureDetail(map[string]any{"api_key": "do-not-log-this"}); value != "response body omitted" {
		t.Fatalf("successful body logged: %s", value)
	}
	value := usageFailureDetail("actual cause gizclaw_sk_v1_generated_fixture_secret Bearer generated-key")
	if value != "actual cause [REDACTED] [REDACTED]" {
		t.Fatalf("failure diagnostic credential redaction: %s", value)
	}
}
