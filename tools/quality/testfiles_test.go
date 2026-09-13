package main

import (
	"slices"
	"testing"
)

func TestTestFileOwnershipViolations(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"pkg/dock.go":                        "package pkg\n",
		"pkg/dock_test.go":                   "package pkg\n",
		"pkg/dock_latency_test.go":           "package pkg\n",
		"pkg/latency_test.go":                "package pkg\n",
		"tests/e2e/scenario_test.go":         "package e2e\n",
		"third_party/lib/orphan_test.go":     "package lib\n",
		"sdk/js/gizclaw/generated/x_test.go": "package generated\n",
	}
	tracked := make(map[string]bool, len(files))
	var names []string
	for name, contents := range files {
		writeTestFile(t, root, name, contents)
		tracked[name] = true
		names = append(names, name)
	}
	violations, pure := testFileOwnershipViolations(names)
	if want := []string{"pkg/latency_test.go"}; !slices.Equal(violations, want) {
		t.Fatalf("violations = %q, want %q", violations, want)
	}
	if want := []string{"tests/e2e/scenario_test.go"}; !slices.Equal(pure, want) {
		t.Fatalf("pure exemptions = %q, want %q", pure, want)
	}
}

func TestValidateTestFileExemptions(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "tests/e2e/scenario_test.go", "package e2e\n")
	writeTestFile(t, root, "pkg/source.go", "package pkg\n")
	writeTestFile(t, root, "pkg/stale_test.go", "package pkg\n")
	tracked := map[string]bool{
		"tests/e2e/scenario_test.go": true,
		"pkg/source.go":              true,
		"pkg/stale_test.go":          true,
	}
	pure := []string{"tests/e2e/scenario_test.go"}
	if err := validateTestFileExemptions([]byte("tests/e2e/scenario_test.go\n"), tracked, pure); err != nil {
		t.Fatalf("valid exemptions: %v", err)
	}
	for name, contents := range map[string][]byte{
		"missing newline": []byte("tests/e2e/scenario_test.go"),
		"stale":           []byte("pkg/stale_test.go\n"),
		"unknown":         []byte("tests/e2e/unknown_test.go\n"),
		"absolute":        []byte("/tests/e2e/scenario_test.go\n"),
		"duplicate":       []byte("tests/e2e/scenario_test.go\ntests/e2e/scenario_test.go\n"),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateTestFileExemptions(contents, tracked, pure); err == nil {
				t.Fatal("invalid exemptions were accepted")
			}
		})
	}
}
