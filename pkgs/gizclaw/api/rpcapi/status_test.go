package rpcapi

import (
	"net/http"
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

// Every canonical code has to survive the wire in both directions. The old
// enum was only ever exercised for two values, which is how a lossy mapping
// stayed invisible.
func TestRPCStatusRoundTripsEveryCanonicalCode(t *testing.T) {
	for code := StatusCodeOK; code <= StatusCodeUnauthenticated; code++ {
		resp := &RPCResponse{V: RPCVersionV1, Id: "req-1", Error: &RPCStatus{
			Code:    code,
			Message: "failed",
			Reason:  "SOME_REASON",
		}}
		msg, err := EncodeRPCResponse(resp)
		if err != nil {
			t.Fatalf("%s: encode: %v", code, err)
		}
		decoded, err := DecodeRPCResponse(msg)
		if err != nil {
			t.Fatalf("%s: decode: %v", code, err)
		}
		if decoded.Error == nil {
			t.Fatalf("%s: decoded response carries no status", code)
		}
		if decoded.Error.Code != code {
			t.Fatalf("%s: code = %s, want %s", code, decoded.Error.Code, code)
		}
		if decoded.Error.Message != "failed" || decoded.Error.Reason != "SOME_REASON" {
			t.Fatalf("%s: decoded = %#v", code, decoded.Error)
		}
	}
}

// A status without a reason must not put an empty ErrorInfo on the wire, so a
// peer can tell "unclassified" from "classified as the empty string".
func TestRPCStatusOmitsErrorInfoWithoutReason(t *testing.T) {
	msg, err := EncodeRPCResponse(&RPCResponse{
		V: RPCVersionV1, Id: "req-1",
		Error: &RPCStatus{Code: StatusCodeInternal, Message: "boom"},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	status := msg.GetStatus()
	if status == nil {
		t.Fatal("encoded response carries no status")
	}
	if status.GetInfo() != nil {
		t.Fatalf("status info = %#v, want nil", status.GetInfo())
	}
	if status.GetCode() != rpcpb.StatusCode_STATUS_CODE_INTERNAL {
		t.Fatalf("wire code = %v", status.GetCode())
	}
}

// The reason is set alongside a domain, so a consumer can tell a GizClaw
// reason from one a device made up.
func TestRPCStatusCarriesErrorDomainWithReason(t *testing.T) {
	msg, err := EncodeRPCResponse(&RPCResponse{
		V: RPCVersionV1, Id: "req-1",
		Error: &RPCStatus{Code: StatusCodeFailedPrecondition, Message: "pending", Reason: "WORKSPACE_PENDING_DELETION"},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	info := msg.GetStatus().GetInfo()
	if info.GetReason() != "WORKSPACE_PENDING_DELETION" || info.GetDomain() != ErrorDomain {
		t.Fatalf("info = %#v", info)
	}
}

func TestStatusCodeValidRejectsValuesOutsideTheCanonicalRange(t *testing.T) {
	for _, code := range []StatusCode{-1, -32603, 17, 404, 409} {
		if code.Valid() {
			t.Fatalf("%d should not be a valid status code", int(code))
		}
	}
}

// The HTTP projection has to round-trip for the codes that have an exact HTTP
// counterpart. The codes that share a status (ALREADY_EXISTS and ABORTED both
// map to 409) are checked one way only.
func TestStatusCodeHTTPMappingRoundTrips(t *testing.T) {
	for _, code := range []StatusCode{
		StatusCodeOK,
		StatusCodeInvalidArgument,
		StatusCodeDeadlineExceeded,
		StatusCodeNotFound,
		StatusCodePermissionDenied,
		StatusCodeUnauthenticated,
		StatusCodeResourceExhausted,
		StatusCodeFailedPrecondition,
		StatusCodeUnimplemented,
		StatusCodeUnavailable,
		StatusCodeCancelled,
	} {
		if got := StatusCodeFromHTTP(code.HTTPStatus()); got != code {
			t.Fatalf("%s -> %d -> %s", code, code.HTTPStatus(), got)
		}
	}
	if got := StatusCodeAlreadyExists.HTTPStatus(); got != http.StatusConflict {
		t.Fatalf("ALREADY_EXISTS -> %d, want 409", got)
	}
	if got := StatusCodeAborted.HTTPStatus(); got != http.StatusConflict {
		t.Fatalf("ABORTED -> %d, want 409", got)
	}
	if got := StatusCodeInternal.HTTPStatus(); got != http.StatusInternalServerError {
		t.Fatalf("INTERNAL -> %d, want 500", got)
	}
}

// An unmapped status must never be reported as success.
func TestStatusCodeFromHTTPNeverReportsUnmappedFailuresAsOK(t *testing.T) {
	for _, status := range []int{402, 418, 451, 500, 502, 599} {
		if got := StatusCodeFromHTTP(status); got == StatusCodeOK {
			t.Fatalf("HTTP %d mapped to OK", status)
		}
	}
}
