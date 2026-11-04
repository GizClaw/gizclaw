//go:build store_e2e

package store_e2e_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func requiredEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}

func uniqueTable(prefix string) string {
	return fmt.Sprintf("gzc_e2e_%s_%d", prefix, time.Now().UnixNano())
}
