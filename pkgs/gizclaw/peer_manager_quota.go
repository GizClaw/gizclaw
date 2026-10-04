package gizclaw

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerquota"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/quota"
)

type quotaReporter struct{ manager *Manager }

func (r quotaReporter) QuotaReport(ctx context.Context, peer giznet.PublicKey) (quota.QuotaRequest, error) {
	m := r.manager
	if m.Peers == nil || m.PeerUsage == nil {
		return quota.QuotaRequest{}, fmt.Errorf("gizclaw: quota requires Peer and SQL usage services")
	}
	stored, err := m.Peers.LoadPeer(ctx, peer)
	if err != nil {
		return quota.QuotaRequest{}, err
	}
	usage, err := m.PeerUsage.Hourly(ctx, peer)
	if err != nil {
		return quota.QuotaRequest{}, err
	}
	report := quota.QuotaRequest{PeerPublicKey: peer.String(), Identifiers: stored.Device.Identifiers, Usage: make([]quota.QuotaUsage, 0, len(usage))}
	for _, item := range usage {
		report.Usage = append(report.Usage, quota.QuotaUsage{ModelId: item.ModelID, Hour: item.Hour, Quantity: item.Quantity})
	}
	return report, nil
}

func (m *Manager) quotaAuthorizer(peer giznet.PublicKey, profile func(context.Context) (apitypes.RuntimeProfile, error)) func(context.Context) (context.Context, func(), error) {
	return func(ctx context.Context) (context.Context, func(), error) {
		var selected apitypes.RuntimeProfile
		var err error
		if profile != nil {
			selected, err = profile(ctx)
		} else if m != nil {
			selected, err = m.runtimeProfileForOwner(ctx, peer.String())
		} else {
			err = peergenx.ErrDenied
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%w: quota RuntimeProfile is unavailable", peergenx.ErrDenied)
		}
		policy, err := runtimeprofile.CustomQuotaPolicy(selected.Spec.Quota)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %w: invalid quota policy", peergenx.ErrDenied, peerquota.ErrUnavailable)
		}
		if policy == nil {
			return ctx, func() {}, nil
		}
		if m == nil || m.PeerQuota == nil {
			return nil, nil, fmt.Errorf("%w: %w: quota service is unavailable", peergenx.ErrDenied, peerquota.ErrUnavailable)
		}
		callCtx, release, err := m.PeerQuota.Authorize(ctx, peer, *policy)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %w", peergenx.ErrDenied, err)
		}
		return callCtx, release, nil
	}
}
