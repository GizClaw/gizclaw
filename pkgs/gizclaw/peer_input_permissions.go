package gizclaw

import (
	"context"
	"sync"
	"time"
)

const (
	peerPermissionRefreshInterval = 5 * time.Second
	peerPermissionRequestTimeout  = 2 * time.Second
	peerPermissionMaxAge          = 7 * time.Second
)

type peerInputPermission struct {
	run       peerRunState
	isSFU     bool
	denial    *inputAccessError
	revision  uint64
	checkedAt time.Time
}

// peerInputPermissions retains only one connection's current runtime decision.
// mu protects snapshots and worker ownership, never a database call.
type peerInputPermissions struct {
	mu      sync.Mutex
	value   peerInputPermission
	valid   bool
	waiting chan struct{}
	cancel  context.CancelFunc
	done    chan struct{}
	closed  bool
}

func (h *PeerConn) inputPermission(ctx context.Context, refresh bool) peerInputPermission {
	revision := h.agentHost.RuntimeRevision()
	denied := peerInputPermission{revision: revision, denial: sfuAccessCheckFailedError()}
	if revision%2 != 0 {
		// Reload ends the old route before publishing its replacement. The
		// SDK may rearm immediately; await publication before admission.
		waitCtx, cancel := context.WithTimeout(ctx, peerPermissionRequestTimeout)
		stable, err := h.agentHost.WaitRuntimeRevision(waitCtx)
		cancel()
		if err != nil {
			return denied
		}
		revision = stable
		denied.revision = stable
	}
	cache := &h.permissions
	for {
		cache.mu.Lock()
		if cache.closed {
			cache.mu.Unlock()
			return denied
		}
		if !refresh && cache.valid && cache.value.revision == revision {
			value := cache.value
			cache.mu.Unlock()
			// Expired results fail closed locally. The worker owns retrying storage.
			if time.Since(value.checkedAt) > peerPermissionMaxAge {
				return denied
			}
			return value
		}
		if waiting := cache.waiting; waiting != nil {
			cache.mu.Unlock()
			select {
			case <-ctx.Done():
				return denied
			case <-waiting:
			}
			// The completed lookup satisfies this refresh unless the runtime changed.
			refresh = false
			continue
		}
		waiting := make(chan struct{})
		cache.waiting = waiting
		cache.mu.Unlock()

		value := h.readInputPermission(ctx, revision)
		cache.mu.Lock()
		if !cache.closed && h.agentHost.RuntimeRevision() == revision {
			cache.value = value
			cache.valid = true
		} else {
			value = denied
		}
		cache.waiting = nil
		close(waiting)
		cache.mu.Unlock()
		return value
	}
}

func (h *PeerConn) readInputPermission(parent context.Context, revision uint64) peerInputPermission {
	ctx, cancel := context.WithTimeout(parent, peerPermissionRequestTimeout)
	defer cancel()
	value := peerInputPermission{revision: revision, checkedAt: time.Now()}
	run, err := h.currentRunState(ctx)
	value.run = run
	if err != nil {
		value.denial = sfuAccessCheckFailedError()
		return value
	}
	if run.workspaceName != "" {
		value.isSFU, value.denial = h.Service.manager.sfuInputAccess(ctx, h.Conn.PublicKey(), run.workspaceName)
		if value.denial == nil && value.isSFU && !run.active {
			value.denial = sfuRuntimeNotAttachedError()
		}
	}
	if ctx.Err() != nil {
		value.denial = sfuAccessCheckFailedError()
	}
	return value
}

// The mandatory Event stream owns the refresh worker. Initial admission and
// runtime revision changes are checked on demand; ordinary turns read memory.
func (h *PeerConn) startInputAccessRefresh() {
	cache := &h.permissions
	cache.mu.Lock()
	if cache.closed || cache.cancel != nil {
		cache.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	cache.cancel = cancel
	cache.done = done
	cache.mu.Unlock()
	go func() {
		defer close(done)
		ticker := time.NewTicker(peerPermissionRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			cache.mu.Lock()
			initialized := cache.valid
			cache.mu.Unlock()
			if initialized {
				h.inputPermission(ctx, true)
			}
		}
	}()
}

func (h *PeerConn) stopInputAccessRefresh() {
	cache := &h.permissions
	cache.mu.Lock()
	cache.closed = true
	cancel, done := cache.cancel, cache.done
	cache.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (h *PeerConn) permissionAllowsAudio(revision uint64) bool {
	cache := &h.permissions
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.closed {
		return false
	}
	if !cache.valid {
		return true
	}
	return cache.value.revision == revision && cache.value.denial == nil && time.Since(cache.value.checkedAt) <= peerPermissionMaxAge
}

// A same-workspace recovery retains the existing expiry; it must not extend
// cached membership merely because the agent restarted.
func (h *PeerConn) advanceInputPermissionRevision(workspace string, previous, current uint64) {
	cache := &h.permissions
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.valid && cache.value.revision == previous && cache.value.run.workspaceName == workspace && cache.value.denial == nil && h.agentHost.RuntimeRevision() == current {
		cache.value.revision = current
		cache.value.run.active = true
	}
}
