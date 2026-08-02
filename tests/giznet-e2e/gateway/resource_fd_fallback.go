//go:build !darwin || !cgo

package main

func readNativeFDCount() (int, bool) {
	return -1, false
}
