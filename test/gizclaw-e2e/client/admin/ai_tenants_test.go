//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestAdminAPITenantsListAndGet(t *testing.T) {
	env := newAdminAPIHarness(t)

	openAIList, err := env.api.ListOpenAITenantsWithResponse(env.ctx, nil)
	if err != nil {
		t.Fatalf("list OpenAI tenants: %v", err)
	}
	requireStatusOK(t, openAIList, openAIList.Body)
	if openAIList.JSON200 == nil {
		t.Fatalf("list OpenAI tenants missing JSON200")
	}
	requireName(t, openAIList.JSON200.Items, "openai-lab", func(item apitypes.OpenAITenant) string { return item.Name })
	openAIGet, err := env.api.GetOpenAITenantWithResponse(env.ctx, "openai-lab")
	if err != nil {
		t.Fatalf("get OpenAI tenant: %v", err)
	}
	requireStatusOK(t, openAIGet, openAIGet.Body)

	miniMaxList, err := env.api.ListMiniMaxTenantsWithResponse(env.ctx, nil)
	if err != nil {
		t.Fatalf("list MiniMax tenants: %v", err)
	}
	requireStatusOK(t, miniMaxList, miniMaxList.Body)
	if miniMaxList.JSON200 == nil {
		t.Fatalf("list MiniMax tenants missing JSON200")
	}
	requireName(t, miniMaxList.JSON200.Items, "minimax-main", func(item apitypes.MiniMaxTenant) string { return item.Name })
	miniMaxGet, err := env.api.GetMiniMaxTenantWithResponse(env.ctx, "minimax-main")
	if err != nil {
		t.Fatalf("get MiniMax tenant: %v", err)
	}
	requireStatusOK(t, miniMaxGet, miniMaxGet.Body)
	if miniMaxGet.JSON200 == nil || miniMaxGet.JSON200.Name != "minimax-main" {
		t.Fatalf("get MiniMax tenant = %#v", miniMaxGet.JSON200)
	}

	volcList, err := env.api.ListVolcTenantsWithResponse(env.ctx, nil)
	if err != nil {
		t.Fatalf("list Volc tenants: %v", err)
	}
	requireStatusOK(t, volcList, volcList.Body)
	if volcList.JSON200 == nil {
		t.Fatalf("list Volc tenants missing JSON200")
	}
	requireName(t, volcList.JSON200.Items, "volc-lab", func(item apitypes.VolcTenant) string { return item.Name })
	volcGet, err := env.api.GetVolcTenantWithResponse(env.ctx, "volc-lab")
	if err != nil {
		t.Fatalf("get Volc tenant: %v", err)
	}
	requireStatusOK(t, volcGet, volcGet.Body)
	if volcGet.JSON200 == nil || volcGet.JSON200.Name != "volc-lab" {
		t.Fatalf("get Volc tenant = %#v", volcGet.JSON200)
	}

	geminiList, err := env.api.ListGeminiTenantsWithResponse(env.ctx, nil)
	if err != nil {
		t.Fatalf("list Gemini tenants: %v", err)
	}
	requireStatusOK(t, geminiList, geminiList.Body)
	if geminiList.JSON200 == nil {
		t.Fatalf("list Gemini tenants missing JSON200")
	}
	requireName(t, geminiList.JSON200.Items, "gemini-main", func(item apitypes.GeminiTenant) string { return item.Name })
	geminiGet, err := env.api.GetGeminiTenantWithResponse(env.ctx, "gemini-main")
	if err != nil {
		t.Fatalf("get Gemini tenant: %v", err)
	}
	requireStatusOK(t, geminiGet, geminiGet.Body)
	if geminiGet.JSON200 == nil || geminiGet.JSON200.Name != "gemini-main" {
		t.Fatalf("get Gemini tenant = %#v", geminiGet.JSON200)
	}

	dashScopeList, err := env.api.ListDashScopeTenantsWithResponse(env.ctx, nil)
	if err != nil {
		t.Fatalf("list DashScope tenants: %v", err)
	}
	requireStatusOK(t, dashScopeList, dashScopeList.Body)
	if dashScopeList.JSON200 == nil {
		t.Fatalf("list DashScope tenants missing JSON200")
	}
	requireName(t, dashScopeList.JSON200.Items, "dashscope-main", func(item apitypes.DashScopeTenant) string { return item.Name })
	dashScopeGet, err := env.api.GetDashScopeTenantWithResponse(env.ctx, "dashscope-main")
	if err != nil {
		t.Fatalf("get DashScope tenant: %v", err)
	}
	requireStatusOK(t, dashScopeGet, dashScopeGet.Body)
	if dashScopeGet.JSON200 == nil || dashScopeGet.JSON200.Name != "dashscope-main" {
		t.Fatalf("get DashScope tenant = %#v", dashScopeGet.JSON200)
	}
}
