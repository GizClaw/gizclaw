//go:build darwin || linux

package main

import "syscall"

func readNativeProcessCPUSeconds() (float64, bool) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0, false
	}
	userSeconds := float64(usage.Utime.Sec) + float64(usage.Utime.Usec)/1e6
	systemSeconds := float64(usage.Stime.Sec) + float64(usage.Stime.Usec)/1e6
	return userSeconds + systemSeconds, true
}
