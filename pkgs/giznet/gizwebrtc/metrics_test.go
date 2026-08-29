package gizwebrtc

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestMetricsLabelsStayBounded(t *testing.T) {
	if got := normalizedMetricsNodeRole("edge"); got != "edge" {
		t.Fatalf("normalized edge role = %q", got)
	}
	if got := normalizedMetricsNodeRole("customer-controlled-value"); got != "application" {
		t.Fatalf("normalized unknown role = %q", got)
	}
	for status, want := range map[int]string{
		http.StatusOK:                  "success",
		http.StatusBadRequest:          "rejected",
		http.StatusServiceUnavailable:  "over_capacity",
		http.StatusInternalServerError: "internal_error",
	} {
		if got := signalingResult(status); got != want {
			t.Errorf("signalingResult(%d) = %q, want %q", status, got, want)
		}
	}
	for err, want := range map[error]string{
		nil:                      "success",
		context.Canceled:         "canceled",
		context.DeadlineExceeded: "timeout",
		errors.New("failure"):    "error",
	} {
		if got := serviceResult(err); got != want {
			t.Errorf("serviceResult(%v) = %q, want %q", err, got, want)
		}
	}
}
