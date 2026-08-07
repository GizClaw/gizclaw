//go:build darwin || linux

package main

import "testing"

func TestReadNativeProcessCPUSeconds(t *testing.T) {
	first, ok := readNativeProcessCPUSeconds()
	if !ok || first < 0 {
		t.Fatalf("readNativeProcessCPUSeconds() = %f, %t", first, ok)
	}
	second, ok := readNativeProcessCPUSeconds()
	if !ok || second < first {
		t.Fatalf("second readNativeProcessCPUSeconds() = %f, %t, want at least %f", second, ok, first)
	}
	point := readResourcePoint(false)
	if point.CPUSecondsSource != "native_process_rusage" || point.CPUSeconds < second {
		t.Fatalf("resource CPU = %f from %q, want native process CPU at least %f", point.CPUSeconds, point.CPUSecondsSource, second)
	}
}
