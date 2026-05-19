package gizrun

import (
	"errors"
	"net"
	"strconv"
)

func normalizeDebugPort(port int) (string, error) {
	if port == 0 {
		return "", nil
	}
	if port < 0 || port > 65535 {
		return "", errors.New("gizrun: debug port is invalid")
	}
	return net.JoinHostPort("127.0.0.1", strconv.FormatUint(uint64(port), 10)), nil
}
