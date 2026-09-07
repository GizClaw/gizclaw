package peerrun

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestOTARuntimeSnapshotOrdering(t *testing.T) {
	s := newTestServer(t)
	peer := testPublicKey(t)
	ctx := context.Background()
	base := time.Unix(100, 0).UTC()
	put := func(state, id string, delta int, percent *float64) {
		t.Helper()
		if err := s.PutOTAStatus(ctx, peer, apitypes.PeerOtaStatus{State: state, UpdateId: id, ObservedAt: base.Add(time.Duration(delta) * time.Second), DownloadPercent: percent}); err != nil {
			t.Fatal(err)
		}
	}
	put("started", "one", 0, nil)
	put("downloading", "one", 1, new(50.0))
	put("downloading", "one", 0, new(10.0))
	put("downloading", "one", 2, new(20.0))
	got, err := s.GetStatus(ctx, peer)
	if err != nil || got.Ota == nil || got.Ota.DownloadPercent == nil || *got.Ota.DownloadPercent != 50 {
		t.Fatalf("progress: %+v, %v", got, err)
	}
	for _, terminal := range []string{"succeeded", "failed"} {
		put(terminal, "one", 3, new(0.0))
		got, err = s.GetStatus(ctx, peer)
		if err != nil || got.Ota == nil || got.Ota.State != "downloading" || got.Ota.DownloadPercent == nil || *got.Ota.DownloadPercent != 50 {
			t.Fatalf("regressing %s report: %+v, %v", terminal, got, err)
		}
	}
	put("succeeded", "one", 3, nil)
	put("downloading", "one", 4, new(90.0))
	put("failed", "one", 5, nil)
	got, err = s.GetStatus(ctx, peer)
	if err != nil || got.Ota.State != "succeeded" || got.ReportedAt == nil || !got.ReportedAt.Equal(base.Add(3*time.Second)) {
		t.Fatalf("terminal: %+v, %v", got, err)
	}
	if _, err = s.PutStatus(ctx, peer, apitypes.PeerStatus{Volume: new(22), Ota: &apitypes.PeerOtaStatus{State: "started"}}); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetStatus(ctx, peer)
	if err != nil || got.Ota.State != "succeeded" || got.Volume == nil || *got.Volume != 22 {
		t.Fatalf("control write overwrote OTA: %+v, %v", got, err)
	}
	put("started", "two", 6, nil)
	put("downloading", "one", 2, new(100.0))
	got, err = s.GetStatus(ctx, peer)
	if err != nil || got.Ota.UpdateId != "two" || got.Ota.DownloadPercent != nil {
		t.Fatalf("new attempt: %+v, %v", got, err)
	}
}

func TestConcurrentOTAProgressCannotOverwriteSuccess(t *testing.T) {
	s := newTestServer(t)
	peer := testPublicKey(t)
	base := time.Unix(100, 0).UTC()
	ctx := t.Context()
	if err := s.PutOTAStatus(ctx, peer, apitypes.PeerOtaStatus{State: "started", UpdateId: "one", ObservedAt: base}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	result := make(chan error, 2)
	for _, next := range []apitypes.PeerOtaStatus{
		{State: "downloading", UpdateId: "one", ObservedAt: base.Add(time.Second), DownloadPercent: new(50.0)},
		{State: "succeeded", UpdateId: "one", ObservedAt: base.Add(2 * time.Second)},
	} {
		go func() { <-start; result <- s.PutOTAStatus(ctx, peer, next) }()
	}
	close(start)
	for range 2 {
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.GetStatus(ctx, peer)
	if err != nil || got.Ota == nil || got.Ota.State != "succeeded" {
		t.Fatalf("concurrent result: %+v, %v", got, err)
	}
}
