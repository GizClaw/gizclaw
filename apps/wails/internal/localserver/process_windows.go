//go:build windows

package localserver

import (
	"os"
	"syscall"
)

const stillActive = 259

func processRunning(process *os.Process) bool {
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(process.Pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	var exitCode uint32
	return syscall.GetExitCodeProcess(handle, &exitCode) == nil && exitCode == stillActive
}
