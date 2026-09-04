package peerresource

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// statusError takes a canonical rpcapi.StatusCode, but an HTTP status is an
// untyped constant, so statusError(id, http.StatusNotFound, msg) still
// compiles and silently produces a code outside the canonical set that Valid()
// then collapses to INTERNAL. The compiler cannot catch it and it has already
// slipped in twice, so the package guards the call shape directly.
func TestStatusErrorIsNeverCalledWithAnHTTPStatus(t *testing.T) {
	pattern := regexp.MustCompile(`statusError\([^,]+,\s*(http\.Status\w+|\d{3})\b`)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || name == "status_error_guard_test.go" {
			continue
		}
		source, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for index, line := range strings.Split(string(source), "\n") {
			if pattern.MatchString(line) {
				t.Errorf("%s:%d passes an HTTP status to statusError; use an rpcapi.StatusCode, or rpcapi.StatusCodeFromHTTP for a status that genuinely starts as HTTP:\n\t%s",
					name, index+1, strings.TrimSpace(line))
			}
		}
	}
}
