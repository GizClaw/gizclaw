//go:build !darwin && !linux

package main

func readNativeProcessCPUSeconds() (float64, bool) {
	return 0, false
}
