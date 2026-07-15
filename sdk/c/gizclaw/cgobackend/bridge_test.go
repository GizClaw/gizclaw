package cgobackend

import (
	"testing"
	"unsafe"
)

func TestCArrayLen(t *testing.T) {
	value := byte(0)
	ptr := unsafe.Pointer(&value)

	tests := []struct {
		name  string
		ptr   unsafe.Pointer
		count uint64
		want  int
		ok    bool
	}{
		{name: "empty nil", count: 0, want: 0, ok: true},
		{name: "empty non-nil", ptr: ptr, count: 0, want: 0, ok: true},
		{name: "nil with entries", count: 1, ok: false},
		{name: "one entry", ptr: ptr, count: 1, want: 1, ok: true},
		{name: "largest supported", ptr: ptr, count: 1<<31 - 1, want: 1<<31 - 1, ok: true},
		{name: "too large", ptr: ptr, count: 1 << 31, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cArrayLen(tt.ptr, tt.count)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("cArrayLen(%p, %d) = (%d, %t), want (%d, %t)", tt.ptr, tt.count, got, ok, tt.want, tt.ok)
			}
		})
	}
}
