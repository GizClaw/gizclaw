//go:build darwin && cgo

package main

import "testing"

func TestReadNativeFDCount(t *testing.T) {
	count, ok := readNativeFDCount()
	if !ok || count <= 0 {
		t.Fatalf("readNativeFDCount() = %d, %t, want a positive count", count, ok)
	}
}
