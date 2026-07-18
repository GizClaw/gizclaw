//go:build !unix && !windows

package localserver

import "os"

func processRunning(*os.Process) bool {
	return false
}
