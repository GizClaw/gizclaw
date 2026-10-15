package minimaxtts

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http/httptrace"
	"os"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

// Timestamps are offsets from one monotonic clock. HTTP trace callbacks can
// overlap on connection attempts; atomics keep them out of provider locks.
type synthesisTiming struct {
	start              time.Time
	meta               streamkit.TTSMeta
	transport          Transport
	enabled            bool
	logged             atomic.Bool
	reused             atomic.Bool
	attempts           atomic.Int64
	dnsStart           atomic.Int64
	dnsDone            atomic.Int64
	tcpStart           atomic.Int64
	tcpDone            atomic.Int64
	tlsStart           atomic.Int64
	tlsDone            atomic.Int64
	connectionReady    atomic.Int64
	requestSent        atomic.Int64
	responseFirstByte  atomic.Int64
	firstProviderAudio atomic.Int64
	firstOutput        atomic.Int64
}

func newSynthesisTiming(ctx context.Context, meta streamkit.TTSMeta, transport Transport) (context.Context, *synthesisTiming) {
	t := &synthesisTiming{start: time.Now(), meta: meta, transport: transport, enabled: os.Getenv("GIZCLAW_TTS_DEBUG") != ""}
	if !t.enabled {
		return ctx, t
	}
	return httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { t.mark(&t.dnsStart) },
		DNSDone:  func(httptrace.DNSDoneInfo) { t.mark(&t.dnsDone) },
		ConnectStart: func(string, string) {
			t.mark(&t.tcpStart)
		},
		ConnectDone: func(_, _ string, err error) {
			if err == nil {
				t.mark(&t.tcpDone)
			}
		},
		TLSHandshakeStart: func() { t.mark(&t.tlsStart) },
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			if err == nil {
				t.mark(&t.tlsDone)
			}
		},
		GotConn: func(info httptrace.GotConnInfo) {
			t.attempts.Add(1)
			if t.connectionReady.CompareAndSwap(0, time.Since(t.start).Nanoseconds()+1) {
				t.reused.Store(info.Reused)
			}
		},
		WroteRequest:         func(httptrace.WroteRequestInfo) { t.mark(&t.requestSent) },
		GotFirstResponseByte: func() { t.mark(&t.responseFirstByte) },
	}), t
}

func (t *synthesisTiming) mark(dst *atomic.Int64) {
	if t.enabled {
		dst.CompareAndSwap(0, time.Since(t.start).Nanoseconds()+1)
	}
}

func timingOffset(stamp *atomic.Int64) float64 {
	if value := stamp.Load(); value > 0 {
		return float64(value-1) / float64(time.Millisecond)
	}
	return -1
}

func timingInterval(start, end *atomic.Int64) float64 {
	s, e := start.Load(), end.Load()
	if s == 0 || e < s {
		return -1
	}
	return float64(e-s) / float64(time.Millisecond)
}

func (t *synthesisTiming) logFirst(ctx context.Context, localEmit time.Duration) {
	if !t.enabled || t.firstOutput.Load() == 0 || !t.logged.CompareAndSwap(false, true) {
		return
	}
	dns, tcp, handshake := timingInterval(&t.dnsStart, &t.dnsDone), timingInterval(&t.tcpStart, &t.tcpDone), timingInterval(&t.tlsStart, &t.tlsDone)
	if t.reused.Load() {
		// A speculative dial can complete after an idle connection won. Its
		// DNS/TLS work is not on this request's critical path.
		dns, tcp, handshake = 0, 0, 0
	}
	slog.InfoContext(ctx, "minimax tts: first audio timing",
		"stream_id", t.meta.StreamID, "segment_index", t.meta.SegmentIndex,
		"transport", string(t.transport), "connection_reused", t.reused.Load(),
		"http_attempts", t.attempts.Load(), "connection_ready_ms", timingOffset(&t.connectionReady),
		"dns_ms", dns, "tcp_ms", tcp, "tls_ms", handshake,
		"response_headers_ms", timingInterval(&t.requestSent, &t.responseFirstByte),
		"request_to_provider_audio_ms", timingInterval(&t.requestSent, &t.firstProviderAudio),
		"first_provider_audio_ms", timingOffset(&t.firstProviderAudio),
		"first_output_ready_ms", timingOffset(&t.firstOutput),
		"local_emit_ms", float64(localEmit)/float64(time.Millisecond))
}
