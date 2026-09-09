package streamlog

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
)

func (r *Recorder) recordStageMetrics(ctx context.Context, attrs []slog.Attr, validContent bool) {
	if r.boundary != "transformer_output" || r.model.Model == "" {
		return
	}
	var event, role, mime, inputMIME, result string
	var contentMS, endMS, bytes float64
	var hasContent, hasEnd bool
	for _, a := range attrs {
		switch a.Key {
		case "event":
			event = a.Value.String()
		case "role":
			role = a.Value.String()
		case "mime_type":
			mime = a.Value.String()
		case "input_mime_type":
			inputMIME = a.Value.String()
		case "result":
			result = a.Value.String()
		case "input_content_elapsed_ms":
			contentMS = a.Value.Float64()
			hasContent = true
		case "after_input_end_ms":
			endMS = a.Value.Float64()
			hasEnd = true
		case "content_bytes":
			bytes = float64(a.Value.Int64())
		}
	}
	labels := []gizmetrics.Label{{Name: "provider", Value: r.model.Provider}, {Name: "model", Value: r.model.Model}, {Name: "mode", Value: r.model.Kind}}
	observe := func(name string, ms float64) {
		gizmetrics.ObserveDuration(ctx, name, time.Duration(ms*float64(time.Millisecond)), labels...)
	}
	asr := (r.model.Kind == "asr" || r.model.Kind == "realtime" || r.model.Kind == "ast") && role == "user" && mime == "text/plain" && strings.HasPrefix(inputMIME, "audio/")
	if asr {
		if event == "first_text" && hasContent {
			observe("genx_asr_first_text_seconds", contentMS)
		}
		if event == "stream_end" && result == "completed" && validContent && bytes > 0 && hasEnd {
			observe("genx_asr_final_result_seconds", endMS)
		}
	}
	if event == "first_audio" && hasContent && r.model.Kind == "tts" && inputMIME == "text/plain" {
		observe("genx_tts_first_audio_seconds", contentMS)
	}
	if r.model.Kind == "realtime" && role == "assistant" && hasContent {
		if event == "first_text" {
			observe("genx_realtime_first_text_seconds", contentMS)
		}
		if event == "first_audio" {
			observe("genx_realtime_first_audio_seconds", contentMS)
		}
	}
}
