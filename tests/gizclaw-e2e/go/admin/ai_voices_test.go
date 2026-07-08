//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIVoicesListAndGet(t *testing.T) {
	env := newAdminAPIHarness(t)

	resp, err := env.api.ListVoicesWithResponse(env.ctx, &adminhttp.ListVoicesParams{Limit: ptr[int32](50)})
	if err != nil {
		t.Fatalf("list voices: %v", err)
	}
	requireStatusOK(t, resp, resp.Body)
	if resp.JSON200 == nil {
		t.Fatalf("list voices missing JSON200")
	}
	if !hasAdminName(resp.JSON200.Items, "minimax-narrator-clone", func(item apitypes.Voice) string { return item.Id }) {
		t.Skip("minimax-narrator-clone voice is not configured in this e2e environment")
	}

	get, err := env.api.GetVoiceWithResponse(env.ctx, "minimax-narrator-clone")
	if err != nil {
		t.Fatalf("get voice: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Id != "minimax-narrator-clone" || get.JSON200.Provider.Name != "minimax-cn" {
		t.Fatalf("get voice = %#v", get.JSON200)
	}
}
