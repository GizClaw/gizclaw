//go:build unix

package localserver

import (
	"errors"
	"os"
	"syscall"
)

func processRunning(process *os.Process) bool {
	err := process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
