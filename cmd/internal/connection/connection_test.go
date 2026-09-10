package connection

import (
	"strings"
	"testing"
)

func TestDialFromContextUsesCLIConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if _, _, _, err := DialFromContext(""); err == nil || !strings.Contains(err.Error(), "no active context") {
		t.Fatalf("DialFromContext error = %v", err)
	}
	if _, err := ConnectFromContext("missing"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("ConnectFromContext error = %v", err)
	}
}
