package customid

import (
	"strings"
	"testing"
)

func TestValidateAcceptsCanonicalCustomIDs(t *testing.T) {
	for _, id := range []string{
		"abcdefgh",
		"abc12345",
		"a2345678",
		"alpha001",
		"alpha-01",
		"alpha_01",
		"alpha.01",
		strings.Repeat("a", MaxLength),
	} {
		t.Run(id, func(t *testing.T) {
			if err := Validate(id); err != nil {
				t.Fatalf("Validate(%q): %v", id, err)
			}
		})
	}
}

func TestValidateRejectsInvalidCustomIDs(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   string
	}{
		{name: "empty"},
		{name: "too short", id: "abcdefg"},
		{name: "too long", id: strings.Repeat("a", MaxLength+1)},
		{name: "starts with digit", id: "1bcdefgh"},
		{name: "starts with underscore", id: "_bcdefgh"},
		{name: "ends with dash", id: "abcdefg-"},
		{name: "ends with dot", id: "abcdefg."},
		{name: "uppercase", id: "abcDefgh"},
		{name: "space", id: "abc defg"},
		{name: "trimmed only by caller", id: " abcdefgh"},
		{name: "slash", id: "abc/defg"},
		{name: "colon", id: "abc:defg"},
		{name: "percent", id: "abc%defg"},
		{name: "unicode", id: "abcdef好"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.id); err == nil {
				t.Fatalf("Validate(%q) succeeded", tc.id)
			}
		})
	}
}

func TestValidateFieldAnnotatesFieldName(t *testing.T) {
	err := ValidateField("metadata.name", "short")
	if err == nil {
		t.Fatal("ValidateField succeeded")
	}
	if got := err.Error(); !strings.Contains(got, "metadata.name:") {
		t.Fatalf("ValidateField error = %q, want field prefix", got)
	}
}

func TestValidateRegistrationTokenName(t *testing.T) {
	for _, name := range []string{"desktop-local", "app:com.gizclaw.opensource"} {
		if err := ValidateRegistrationTokenName(name); err != nil {
			t.Errorf("ValidateRegistrationTokenName(%q) = %v", name, err)
		}
	}
	for _, name := range []string{"app:", "app:Com.gizclaw.opensource", "other:com.gizclaw.opensource"} {
		if err := ValidateRegistrationTokenName(name); err == nil {
			t.Errorf("ValidateRegistrationTokenName(%q) succeeded", name)
		}
	}
}
