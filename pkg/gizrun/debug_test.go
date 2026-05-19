package gizrun

import "testing"

func TestNormalizeDebugPort(t *testing.T) {
	tests := []struct {
		name string
		port int
		want string
	}{
		{name: "port", port: 6060, want: "127.0.0.1:6060"},
		{name: "disabled", port: 0, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeDebugPort(tt.port)
			if err != nil {
				t.Fatalf("normalizeDebugPort() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeDebugPort() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeDebugPortRejectsOutOfRange(t *testing.T) {
	for _, port := range []int{-1, 65536} {
		if _, err := normalizeDebugPort(port); err == nil {
			t.Fatalf("normalizeDebugPort(%d) error = nil", port)
		}
	}
}
