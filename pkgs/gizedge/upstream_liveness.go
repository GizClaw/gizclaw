package gizedge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
)

// ICE consent only proves that the Server's UDP socket still answers STUN.
// An upstream whose DTLS, SCTP or Server application stops making progress
// keeps a healthy ICE pair and never reports an error: Pion's SCTP T3 timer
// retransmits forever and DataChannel opens complete locally without a peer
// acknowledgement. Every upstream is therefore probed end to end through the
// Edge HTTP service the Server already exposes to Edge nodes.
const (
	upstreamLivenessInterval = 10 * time.Second
	upstreamLivenessTimeout  = 2 * time.Second
	// A forwarded request that has not received response headers after this
	// delay triggers an immediate probe, so a dead upstream is evicted and the
	// request retried long before a typical 5-10 second client timeout.
	upstreamStallProbeDelay      = time.Second
	upstreamRedialBackoffInitial = 500 * time.Millisecond
	upstreamRedialBackoffMaximum = 30 * time.Second
	upstreamLivenessProbePath    = "/server-info"
)

var (
	errUpstreamLivenessProbe = errors.New("edge: upstream liveness probe failed")
	// errUpstreamSlow reports a probe that timed out while the association
	// still delivered other inbound data: a synchronized burst can queue the
	// probe behind congested SCTP traffic without the upstream being dead.
	errUpstreamSlow = errors.New("edge: upstream slow")
)

type upstreamLivenessConfig struct {
	interval       time.Duration
	timeout        time.Duration
	stallDelay     time.Duration
	backoffInitial time.Duration
	backoffMaximum time.Duration
	probe          func(context.Context, giznet.Conn) error
}

func (c upstreamLivenessConfig) withDefaults() upstreamLivenessConfig {
	if c.interval <= 0 {
		c.interval = upstreamLivenessInterval
	}
	if c.timeout <= 0 {
		c.timeout = upstreamLivenessTimeout
	}
	if c.stallDelay <= 0 {
		c.stallDelay = upstreamStallProbeDelay
	}
	if c.backoffInitial <= 0 {
		c.backoffInitial = upstreamRedialBackoffInitial
	}
	if c.backoffMaximum < c.backoffInitial {
		c.backoffMaximum = max(upstreamRedialBackoffMaximum, c.backoffInitial)
	}
	if c.probe == nil {
		c.probe = probeUpstreamLiveness
	}
	return c
}

// check runs one bounded probe. Any HTTP response proves that DTLS, SCTP and
// the Server's Edge HTTP service still make progress on conn. A timed-out
// probe is only a stall when conn received nothing else meanwhile.
func (c upstreamLivenessConfig) check(ctx context.Context, conn giznet.Conn) error {
	rxBefore, rxKnown := upstreamRxBytes(conn)
	probeCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	err := c.probe(probeCtx, conn)
	if err == nil {
		return nil
	}
	if ctx.Err() == nil && probeCtx.Err() != nil {
		if rxAfter, ok := upstreamRxBytes(conn); rxKnown && ok && rxAfter > rxBefore {
			// Slow is not a liveness failure: never wrap errUpstreamLivenessProbe.
			return fmt.Errorf("%w: no probe response within %s but received %d bytes: %w",
				errUpstreamSlow, c.timeout, rxAfter-rxBefore, err)
		}
		return fmt.Errorf("%w: no response and no inbound data within %s: %w",
			errUpstreamLivenessProbe, c.timeout, err)
	}
	return fmt.Errorf("%w: %w", errUpstreamLivenessProbe, err)
}

func upstreamRxBytes(conn giznet.Conn) (uint64, bool) {
	info := conn.PeerInfo()
	if info == nil {
		return 0, false
	}
	return info.RxBytes, true
}

func probeUpstreamLiveness(ctx context.Context, conn giznet.Conn) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://gizclaw"+upstreamLivenessProbePath, nil)
	if err != nil {
		return err
	}
	resp, err := gizhttp.NewRoundTripper(conn, gizclaw.ServiceEdgeHTTP).RoundTrip(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	return resp.Body.Close()
}

func nextUpstreamRedialBackoff(current time.Duration, cfg upstreamLivenessConfig) time.Duration {
	return min(current*2, cfg.backoffMaximum)
}
