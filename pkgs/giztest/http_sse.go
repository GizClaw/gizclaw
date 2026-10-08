package giztest

import (
	"encoding/json"
	"errors"
	"strings"
)

const (
	maxHTTPSSEResponseBytes  = 4 << 20
	maxHTTPSSEResponseEvents = 16384
)

// DecodeHTTPEventStream projects a finite SSE response into a JSON-compatible
// object with events [{event, data}], last_event (when present), and raw.
// JSON-valued data is decoded; other data remains text. Only blank-line-terminated
// events with data are dispatched. Comments and reconnection metadata are ignored
// because this HTTP operation does not reconnect or subscribe indefinitely.
func DecodeHTTPEventStream(text string) (map[string]any, error) {
	if len(text) > maxHTTPSSEResponseBytes {
		return nil, errors.New("giztest: SSE response exceeds 4 MiB")
	}
	events := make([]any, 0)
	result := map[string]any{"raw": text}
	pending := strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(text, "\ufeff"), "\r\n", "\n"), "\r", "\n")
	name := "message"
	var data []string
	for {
		line, rest, terminated := strings.Cut(pending, "\n")
		if !terminated {
			break
		}
		pending = rest
		if line == "" {
			if len(data) != 0 {
				if len(events) >= maxHTTPSSEResponseEvents {
					return nil, errors.New("giztest: SSE response exceeds 16384 events")
				}
				text := strings.Join(data, "\n")
				var value any
				if json.Unmarshal([]byte(text), &value) != nil {
					value = text
				}
				event := map[string]any{"event": name, "data": value}
				events = append(events, event)
				result["last_event"] = event
			}
			data = nil
			name = "message"
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			name = value
			if name == "" {
				name = "message"
			}
		case "data":
			data = append(data, value)
		}
	}
	result["events"] = events
	return result, nil
}
