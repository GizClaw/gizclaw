//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -lproc
#include <libproc.h>
#include <stdlib.h>

static int gizclaw_count_open_fds(pid_t pid) {
	int required = proc_pidinfo(pid, PROC_PIDLISTFDS, 0, NULL, 0);
	if (required <= 0) {
		return -1;
	}

	size_t padding = 64 * sizeof(struct proc_fdinfo);
	size_t capacity = (size_t)required + padding;
	struct proc_fdinfo *fds = malloc(capacity);
	if (fds == NULL) {
		return -1;
	}
	int bytes = proc_pidinfo(pid, PROC_PIDLISTFDS, 0, fds, (int)capacity);
	free(fds);
	if (bytes <= 0 || (size_t)bytes % sizeof(struct proc_fdinfo) != 0) {
		return -1;
	}
	return bytes / sizeof(struct proc_fdinfo);
}
*/
import "C"

import "os"

func readNativeFDCount() (int, bool) {
	count := int(C.gizclaw_count_open_fds(C.pid_t(os.Getpid())))
	return count, count >= 0
}
