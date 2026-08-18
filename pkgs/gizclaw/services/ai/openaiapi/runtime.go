package openaiapi

import (
	"context"
	"errors"
	"sync"
)

// ResponseRuntime serializes active Responses per Workspace while permitting
// independent Workspaces to run concurrently.
type ResponseRuntime struct {
	mu       sync.Mutex
	active   map[string]*activeResponse
	closed   bool
	closeCtx context.Context
	cancel   context.CancelFunc
}

type activeResponse struct {
	id     string
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func NewResponseRuntime() *ResponseRuntime {
	ctx, cancel := context.WithCancel(context.Background())
	return &ResponseRuntime{active: make(map[string]*activeResponse), closeCtx: ctx, cancel: cancel}
}

func (r *ResponseRuntime) acquire(workspaceID, responseID string) (*activeResponse, error) {
	if r == nil {
		return nil, errors.New("openaiapi: Response runtime is unavailable")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, errors.New("openaiapi: Response runtime is closed")
	}
	if r.active == nil {
		r.active = make(map[string]*activeResponse)
	}
	if r.active[workspaceID] != nil {
		return nil, errors.New("a Response is already active for this Conversation")
	}
	ctx, cancel := context.WithCancel(r.closeCtx)
	entry := &activeResponse{id: responseID, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	r.active[workspaceID] = entry
	return entry, nil
}

func (r *ResponseRuntime) release(workspaceID string, entry *activeResponse) {
	if r == nil || entry == nil {
		return
	}
	r.mu.Lock()
	if r.active[workspaceID] == entry {
		delete(r.active, workspaceID)
		close(entry.done)
	}
	r.mu.Unlock()
	entry.cancel()
}

func (r *ResponseRuntime) cancelResponse(workspaceID, responseID string) (<-chan struct{}, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	entry := r.active[workspaceID]
	r.mu.Unlock()
	if entry == nil || entry.id != responseID {
		return nil, false
	}
	entry.cancel()
	return entry.done, true
}

func (r *ResponseRuntime) isActive(workspaceID, responseID string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.active[workspaceID]
	return entry != nil && entry.id == responseID
}

func (r *ResponseRuntime) context(parent context.Context, entry *activeResponse) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(r.closeCtx, cancel)
	stopActive := context.AfterFunc(entry.ctx, cancel)
	return ctx, func() {
		stop()
		stopActive()
		cancel()
	}
}

func (r *ResponseRuntime) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		r.cancel()
	}
	done := make([]<-chan struct{}, 0, len(r.active))
	for _, entry := range r.active {
		entry.cancel()
		done = append(done, entry.done)
	}
	r.mu.Unlock()
	for _, channel := range done {
		select {
		case <-channel:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
