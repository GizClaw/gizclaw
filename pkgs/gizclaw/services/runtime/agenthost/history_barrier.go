package agenthost

import "context"

func (r *historyRecorder) markText(key string) {
	r.mu.Lock()
	if r.pendingText == nil {
		r.pendingText = make(map[string]struct{})
	}
	if r.textChanged == nil {
		r.textChanged = make(chan struct{})
	}
	r.pendingText[key] = struct{}{}
	r.mu.Unlock()
}

func (r *historyRecorder) finishText(key string, err error) {
	r.mu.Lock()
	delete(r.pendingText, key)
	if err != nil && r.textError == nil {
		r.textError = err
	}
	if r.textChanged != nil {
		close(r.textChanged)
	}
	r.textChanged = make(chan struct{})
	r.mu.Unlock()
}

func (r *historyRecorder) waitForText(ctx context.Context) error {
	for {
		r.mu.Lock()
		err := r.textError
		pending := len(r.pendingText)
		changed := r.textChanged
		r.mu.Unlock()
		if err != nil {
			return err
		}
		if pending == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-changed:
		}
	}
}
