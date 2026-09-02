package gizedge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type orderedUpstreamTransport struct {
	entries []*upstreamTransport
}

var (
	errAPIKeyTargetUnconfigured = errors.New("edge: API key target Server is not configured")
	errAPIKeyTargetUnavailable  = errors.New("edge: API key target Server is unavailable")
)

func newOrderedUpstreamTransport(ctx context.Context, cfg Config) (*orderedUpstreamTransport, error) {
	upstreams, err := cfg.configuredUpstreams()
	if err != nil {
		return nil, err
	}
	transport := &orderedUpstreamTransport{entries: make([]*upstreamTransport, 0, len(upstreams))}
	for index, upstream := range upstreams {
		entryConfig := cfg.withUpstream(upstream)
		endpoint, err := upstreamConfigURL(upstream, fmt.Sprintf("upstreams[%d]", index))
		if err != nil {
			return nil, err
		}
		selector, err := newUpstreamRelaySelector(entryConfig)
		if err != nil {
			return nil, fmt.Errorf("edge: prepare upstream relay selector: %w", err)
		}
		transport.entries = append(transport.entries, &upstreamTransport{
			ctx: ctx, cfg: entryConfig, upstreamURL: endpoint, relay: selector,
		})
	}
	for _, entry := range transport.entries {
		if _, _, err := entry.currentConn(); err == nil {
			return transport, nil
		}
	}
	_ = transport.Close()
	return nil, errors.New("edge: no configured upstream is reachable")
}

func (t *orderedUpstreamTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if isAPIKeyRoutedPath(request.URL.Path) {
		apiKey, ok := bearerAPIKey(request.Header.Get("Authorization"))
		if !ok {
			return nil, errAPIKeyUnauthorized
		}
		assignment, err := t.resolveAPIKeyAssignment(request.Context(), apiKey)
		if err != nil {
			return nil, err
		}
		entry, err := t.entryForAssignment(assignment)
		if err != nil {
			return nil, err
		}
		response, err := entry.RoundTrip(request)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errAPIKeyTargetUnavailable, err)
		}
		return response, nil
	}
	var errs []error
	for _, entry := range t.entries {
		if _, _, err := entry.currentConn(); err != nil {
			errs = append(errs, err)
			continue
		}
		response, err := entry.RoundTrip(request)
		if err == nil {
			return response, nil
		}
		errs = append(errs, err)
		if request.Context().Err() != nil || !canRetryUpstreamRequest(request.Method) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("edge: no configured upstream is reachable: %w", errors.Join(errs...))
}

func (t *orderedUpstreamTransport) resolveAPIKeyAssignment(ctx context.Context, apiKey string) (*rpcpb.PeerAssignment, error) {
	var errs []error
	for _, entry := range t.entries {
		conn, epoch, err := entry.currentConn()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		assignment, err := resolveAPIKeyAssignment(ctx, conn, apiKey)
		if err == nil {
			return assignment, nil
		}
		if errors.Is(err, errAPIKeyUnauthorized) || errors.Is(err, errAPIKeyOwnerUnavailable) || !upstreamConnectionFailed(conn, err) {
			return nil, err
		}
		entry.resetConn(epoch, ctx.Err() == nil && (entry.ctx == nil || entry.ctx.Err() == nil))
		errs = append(errs, err)
	}
	return nil, fmt.Errorf("%w: no configured upstream can resolve API route: %w", errAPIKeyTargetUnavailable, errors.Join(errs...))
}

func (t *orderedUpstreamTransport) entryForAssignment(assignment *rpcpb.PeerAssignment) (*upstreamTransport, error) {
	if assignment == nil {
		return nil, errAPIKeyTargetUnconfigured
	}
	var serverKey giznet.PublicKey
	if err := serverKey.UnmarshalText([]byte(assignment.GetServerPublicKey())); err != nil || serverKey.IsZero() {
		return nil, errAPIKeyTargetUnconfigured
	}
	for _, entry := range t.entries {
		if entry.cfg.selectedUpstream.PublicKey == serverKey {
			return entry, nil
		}
	}
	return nil, errAPIKeyTargetUnconfigured
}

func isAPIKeyRoutedPath(path string) bool {
	return strings.HasPrefix(path, "/gizclaw/v1/") || strings.HasPrefix(path, "/openai/v1/")
}

func bearerAPIKey(authorization string) (string, bool) {
	scheme, credential, ok := strings.Cut(strings.TrimSpace(authorization), " ")
	credential = strings.TrimSpace(credential)
	return credential, ok && strings.EqualFold(scheme, "Bearer") && credential != ""
}

func (t *orderedUpstreamTransport) resolvePeerAssignment(
	ctx context.Context,
	peerKey giznet.PublicKey,
) (*rpcpb.PeerAssignment, error) {
	var errs []error
	for _, entry := range t.entries {
		conn, epoch, err := entry.currentConn()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		assignment, err := resolvePeerAssignment(ctx, conn, peerKey)
		if err == nil {
			return assignment, err
		}
		if errors.Is(err, errRouteAssignmentNotFound) {
			return &rpcpb.PeerAssignment{ServerPublicKey: entry.cfg.selectedUpstream.PublicKey.String()}, err
		}
		if !upstreamConnectionFailed(conn, err) {
			return nil, err
		}
		entry.resetConn(epoch, ctx.Err() == nil && (entry.ctx == nil || entry.ctx.Err() == nil))
		errs = append(errs, err)
	}
	return nil, fmt.Errorf("edge: no configured upstream can resolve peer route: %w", errors.Join(errs...))
}

func (t *orderedUpstreamTransport) Close() error {
	var errs []error
	for _, entry := range t.entries {
		errs = append(errs, entry.Close())
	}
	return errors.Join(errs...)
}
