//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIVoicesListAndGet(t *testing.T) {
	env := newAdminAPIHarness(t)

	items := collectAdminPages(t, 50, func(cursor *string, limit int32) ([]apitypes.Voice, bool, *string) {
		resp, err := env.api.ListVoicesWithResponse(env.ctx, &adminhttp.ListVoicesParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list voices: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatalf("list voices missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	seed := requireName(t, items, "minimax-narrator-clone", func(item apitypes.Voice) string { return item.Name })

	get, err := env.api.GetVoiceWithResponse(env.ctx, seed.Id)
	if err != nil {
		t.Fatalf("get voice: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil || get.JSON200.Id != seed.Id || get.JSON200.Name != seed.Name || get.JSON200.Provider.Id == "" {
		t.Fatalf("get voice = %#v", get.JSON200)
	}
}
