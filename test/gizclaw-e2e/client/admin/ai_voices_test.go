//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestAdminAPIVoicesListAndGet(t *testing.T) {
	env := newAdminAPIHarness(t)

	resp, err := env.api.ListVoicesWithResponse(env.ctx, &adminservice.ListVoicesParams{Limit: ptr[int32](50)})
	if err != nil {
		t.Fatalf("list voices: %v", err)
	}
	requireStatusOK(t, resp, resp.Body)
	if resp.JSON200 == nil {
		t.Fatalf("list voices missing JSON200")
	}
	requireName(t, resp.JSON200.Items, "minimax-catalog-voice", func(item apitypes.Voice) string { return item.Id })

	get, err := env.api.GetVoiceWithResponse(env.ctx, "minimax-catalog-voice")
	if err != nil {
		t.Fatalf("get voice: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Id != "minimax-catalog-voice" || get.JSON200.Provider.Name != "minimax-main" {
		t.Fatalf("get voice = %#v", get.JSON200)
	}
}
