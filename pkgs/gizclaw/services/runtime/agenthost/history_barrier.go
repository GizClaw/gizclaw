package agenthost

import "context"

func (r *historyRecorder) markText(key, excludedTurnID string) {
	r.mu.Lock()
	if r.pendingText == nil {
		r.pendingText = make(map[string]string)
	}
	if r.textChanged == nil {
		r.textChanged = make(chan struct{})
	}
	r.pendingText[key] = excludedTurnID
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
	return r.waitForPriorText(ctx, "")
}

func (r *historyRecorder) waitForPriorText(ctx context.Context, currentTurnID string) error {
	for {
		r.mu.Lock()
		err := r.textError
		pending := false
		for _, excludedTurnID := range r.pendingText {
			if currentTurnID == "" || excludedTurnID != currentTurnID {
				pending = true
				break
			}
		}
		changed := r.textChanged
		r.mu.Unlock()
		if err != nil {
			return err
		}
		if !pending {
			return nil
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-changed:
		}
	}
}
