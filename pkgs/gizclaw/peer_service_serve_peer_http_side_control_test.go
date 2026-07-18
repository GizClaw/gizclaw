package gizclaw

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type fakeSideControlTelemetry struct {
	targets []giznet.PublicKey
}

func (f *fakeSideControlTelemetry) Latest(_ context.Context, target giznet.PublicKey, _ []apitypes.PeerTelemetryField) (apitypes.PeerTelemetryLatestResponse, error) {
	f.targets = append(f.targets, target)
	return apitypes.PeerTelemetryLatestResponse{PeerPublicKey: target.String(), Values: []apitypes.PeerTelemetryValue{}}, nil
}

func (f *fakeSideControlTelemetry) QueryRange(_ context.Context, target giznet.PublicKey, field apitypes.PeerTelemetryField, start, end time.Time, step time.Duration, _ int, order apitypes.PeerTelemetryOrder) (apitypes.PeerTelemetryRangeResponse, error) {
	f.targets = append(f.targets, target)
	return apitypes.PeerTelemetryRangeResponse{PeerPublicKey: target.String(), Field: field, StartTimeMs: start.UnixMilli(), EndTimeMs: end.UnixMilli(), StepMs: step.Milliseconds(), Points: []apitypes.PeerTelemetryPoint{}}, nil
}

func (f *fakeSideControlTelemetry) Aggregate(_ context.Context, target giznet.PublicKey, field apitypes.PeerTelemetryField, _, _ time.Time, bucket time.Duration, aggregate apitypes.PeerTelemetryAggregate) (apitypes.PeerTelemetryAggregateResponse, error) {
	f.targets = append(f.targets, target)
	return apitypes.PeerTelemetryAggregateResponse{PeerPublicKey: target.String(), Field: field, Aggregate: aggregate, BucketMs: bucket.Milliseconds(), Points: []apitypes.PeerTelemetryAggregatePoint{}}, nil
}

func TestSideControlTelemetryAlwaysQueriesBoundTarget(t *testing.T) {
	controller := giznet.PublicKey{1}
	target := giznet.PublicKey{2}
	ctx := publiclogin.WithPrincipal(context.Background(), publiclogin.Principal{
		Kind:            publiclogin.SessionKindSideControl,
		PublicKey:       controller,
		TargetPublicKey: target,
	})
	telemetry := &fakeSideControlTelemetry{}
	service := &peerHTTP{Telemetry: telemetry}

	latest, err := service.GetSideControlTelemetryLatest(ctx, peerhttp.GetSideControlTelemetryLatestRequestObject{})
	if err != nil {
		t.Fatalf("GetSideControlTelemetryLatest error = %v", err)
	}
	if _, ok := latest.(peerhttp.GetSideControlTelemetryLatest200JSONResponse); !ok {
		t.Fatalf("latest response = %T", latest)
	}
	query, err := service.QuerySideControlTelemetry(ctx, peerhttp.QuerySideControlTelemetryRequestObject{Params: peerhttp.QuerySideControlTelemetryParams{
		Field: apitypes.PeerTelemetryFieldNetworkRssiDbm, StartTimeMs: 1000, EndTimeMs: 2000,
	}})
	if err != nil {
		t.Fatalf("QuerySideControlTelemetry error = %v", err)
	}
	if _, ok := query.(peerhttp.QuerySideControlTelemetry200JSONResponse); !ok {
		t.Fatalf("query response = %T", query)
	}
	aggregate, err := service.AggregateSideControlTelemetry(ctx, peerhttp.AggregateSideControlTelemetryRequestObject{Params: peerhttp.AggregateSideControlTelemetryParams{
		Field: apitypes.PeerTelemetryFieldBatteryPercent, StartTimeMs: 1000, EndTimeMs: 3000, BucketMs: 1000, Aggregate: apitypes.PeerTelemetryAggregateAvg,
	}})
	if err != nil {
		t.Fatalf("AggregateSideControlTelemetry error = %v", err)
	}
	if _, ok := aggregate.(peerhttp.AggregateSideControlTelemetry200JSONResponse); !ok {
		t.Fatalf("aggregate response = %T", aggregate)
	}
	if len(telemetry.targets) != 3 {
		t.Fatalf("telemetry target calls = %d", len(telemetry.targets))
	}
	for _, got := range telemetry.targets {
		if got != target {
			t.Fatalf("telemetry target = %s, want %s", got, target)
		}
	}
}

func TestSideControlRouteRejectsPrimaryPrincipal(t *testing.T) {
	ctx := publiclogin.WithPrincipal(context.Background(), publiclogin.Principal{Kind: publiclogin.SessionKindPrimary, PublicKey: giznet.PublicKey{1}})
	response, err := (&peerHTTP{}).GetSideControlTelemetryLatest(ctx, peerhttp.GetSideControlTelemetryLatestRequestObject{})
	if err != nil {
		t.Fatalf("GetSideControlTelemetryLatest error = %v", err)
	}
	if _, ok := response.(peerhttp.GetSideControlTelemetryLatest403JSONResponse); !ok {
		t.Fatalf("response = %T, want 403", response)
	}
}
