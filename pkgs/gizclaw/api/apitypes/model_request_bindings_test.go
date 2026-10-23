package apitypes

import (
	"strings"
	"testing"
)

func TestModelRequestHeaderWireValues(t *testing.T) {
	for _, value := range []string{"0", "false", "two words", strings.Repeat("x", MaxModelRequestHeaderBytes)} {
		if err := ValidateModelRequestHeader("X-Device-Metadata", value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"", " padded", "padded ", "tab\tvalue", "\x00", "\x7f", "设备", "value\r\nOther: injected", strings.Repeat("x", MaxModelRequestHeaderBytes+1)} {
		if err := ValidateModelRequestHeader("X-Device-Metadata", value); err == nil {
			t.Fatal("invalid HTTP header value accepted")
		}
	}
}
