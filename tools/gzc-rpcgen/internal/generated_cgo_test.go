//go:build cgo

package rpcgen

import "testing"

func TestGoldenCEncodeDecodeViaCgo(t *testing.T) {
	tests := []struct {
		name string
		run  func() int
	}{
		{"encode required fields", runGoldenCEncodeRequired},
		{"encode optional fields", runGoldenCEncodeOptional},
		{"decode required fields", runGoldenCDecodeRequired},
		{"decode optional fields", runGoldenCDecodeOptional},
		{"method constant", runGoldenCMethodConstant},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rc := tt.run(); rc != goldenCOk() {
				t.Fatalf("golden C fixture returned %d", rc)
			}
		})
	}
}
