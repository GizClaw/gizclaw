package customid

import (
	"strings"
	"testing"
)

func TestValidateResourceID(t *testing.T) {
	for _, value := range []string{"plain", "a/b", "a:b", ".config", "...", "中文", strings.Repeat("x", MaxResourceIDCharacters), strings.Repeat("𐀀", MaxResourceIDCharacters)} {
		if err := ValidateResourceID(value); err != nil {
			t.Errorf("ValidateResourceID(%q) error = %v", value, err)
		}
	}
	for _, value := range []string{"", " ", " leading", "trailing ", ".", "..", strings.Repeat("x", MaxResourceIDCharacters+1), strings.Repeat("𐀀", MaxResourceIDCharacters+1)} {
		if err := ValidateResourceID(value); err == nil {
			t.Errorf("ValidateResourceID(%q) error = %v, want rejection", value, err)
		}
	}
}

func TestEscapeStoreSegmentRoundTrip(t *testing.T) {
	for _, value := range []string{"plain", "tenant:model", "a/b", "100%", ".", "..", "中文"} {
		escaped := EscapeStoreSegment(value)
		if strings.Contains(escaped, ":") {
			t.Fatalf("EscapeStoreSegment(%q) = %q contains the kv separator", value, escaped)
		}
		if got := UnescapeStoreSegment(escaped); got != value {
			t.Fatalf("UnescapeStoreSegment(EscapeStoreSegment(%q)) = %q", value, got)
		}
	}
}

func TestOpaquePathSegmentIsolatedAndDistinct(t *testing.T) {
	seen := map[string]string{}
	for _, value := range []string{".", "..", "team", "team/foo", "id-Lg", "a:b", "中文", strings.Repeat("x", 4096)} {
		segment := OpaquePathSegment(value)
		if segment == "." || segment == ".." || strings.ContainsAny(segment, `/\\:`) {
			t.Fatalf("OpaquePathSegment(%q) = %q is not portable", value, segment)
		}
		if len(segment) > 255 {
			t.Fatalf("OpaquePathSegment(%q) length = %d, exceeds NAME_MAX", value, len(segment))
		}
		if previous, ok := seen[segment]; ok {
			t.Fatalf("OpaquePathSegment collision: %q and %q -> %q", previous, value, segment)
		}
		seen[segment] = value
	}
}
