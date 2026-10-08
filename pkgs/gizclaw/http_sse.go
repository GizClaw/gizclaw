package gizclaw

import (
	"bufio"
	"encoding/json"
	"fmt"
)

// writeSSEEvent sends and flushes one JSON-valued server-sent event.
func writeSSEEvent(w *bufio.Writer, event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
		return err
	}
	return w.Flush()
}
