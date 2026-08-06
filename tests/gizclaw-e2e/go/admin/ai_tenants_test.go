//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
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
	openAI := requireName(t, openAIList.JSON200.Items, "fake-openai", func(item apitypes.OpenAITenant) string { return item.Id })
	openAIGet, err := env.api.GetOpenAITenantWithResponse(env.ctx, openAI.Id)
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
	if hasAdminName(miniMaxList.JSON200.Items, "minimax-cn", func(item apitypes.MiniMaxTenant) string { return item.Id }) {
		miniMax := requireName(t, miniMaxList.JSON200.Items, "minimax-cn", func(item apitypes.MiniMaxTenant) string { return item.Id })
		miniMaxGet, err := env.api.GetMiniMaxTenantWithResponse(env.ctx, miniMax.Id)
		if err != nil {
			t.Fatalf("get MiniMax tenant: %v", err)
		}
		requireStatusOK(t, miniMaxGet, miniMaxGet.Body)
		if miniMaxGet.JSON200 == nil || miniMaxGet.JSON200.Id != "minimax-cn" {
			t.Fatalf("get MiniMax tenant = %#v", miniMaxGet.JSON200)
		}
	} else {
		t.Log("minimax-cn tenant is not configured in this e2e environment")
	}

	volcList, err := env.api.ListVolcTenantsWithResponse(env.ctx, nil)
	if err != nil {
		t.Fatalf("list Volc tenants: %v", err)
	}
	requireStatusOK(t, volcList, volcList.Body)
	if volcList.JSON200 == nil {
		t.Fatalf("list Volc tenants missing JSON200")
	}
	if hasAdminName(volcList.JSON200.Items, "volc-main", func(item apitypes.VolcTenant) string { return item.Id }) {
		volc := requireName(t, volcList.JSON200.Items, "volc-main", func(item apitypes.VolcTenant) string { return item.Id })
		volcGet, err := env.api.GetVolcTenantWithResponse(env.ctx, volc.Id)
		if err != nil {
			t.Fatalf("get Volc tenant: %v", err)
		}
		requireStatusOK(t, volcGet, volcGet.Body)
		if volcGet.JSON200 == nil || volcGet.JSON200.Id != "volc-main" {
			t.Fatalf("get Volc tenant = %#v", volcGet.JSON200)
		}
	} else {
		t.Log("volc-main tenant is not configured in this e2e environment")
	}

	geminiList, err := env.api.ListGeminiTenantsWithResponse(env.ctx, nil)
	if err != nil {
		t.Fatalf("list Gemini tenants: %v", err)
	}
	requireStatusOK(t, geminiList, geminiList.Body)
	if geminiList.JSON200 == nil {
		t.Fatalf("list Gemini tenants missing JSON200")
	}
	if hasAdminName(geminiList.JSON200.Items, "gemini-main", func(item apitypes.GeminiTenant) string { return item.Id }) {
		gemini := requireName(t, geminiList.JSON200.Items, "gemini-main", func(item apitypes.GeminiTenant) string { return item.Id })
		geminiGet, err := env.api.GetGeminiTenantWithResponse(env.ctx, gemini.Id)
		if err != nil {
			t.Fatalf("get Gemini tenant: %v", err)
		}
		requireStatusOK(t, geminiGet, geminiGet.Body)
		if geminiGet.JSON200 == nil || geminiGet.JSON200.Id != "gemini-main" {
			t.Fatalf("get Gemini tenant = %#v", geminiGet.JSON200)
		}
	} else {
		t.Log("gemini-main tenant is not configured in this e2e environment")
	}

	dashScopeList, err := env.api.ListDashScopeTenantsWithResponse(env.ctx, nil)
	if err != nil {
		t.Fatalf("list DashScope tenants: %v", err)
	}
	requireStatusOK(t, dashScopeList, dashScopeList.Body)
	if dashScopeList.JSON200 == nil {
		t.Fatalf("list DashScope tenants missing JSON200")
	}
	if hasAdminName(dashScopeList.JSON200.Items, "qwen-dashscope-main", func(item apitypes.DashScopeTenant) string { return item.Id }) {
		dashScope := requireName(t, dashScopeList.JSON200.Items, "qwen-dashscope-main", func(item apitypes.DashScopeTenant) string { return item.Id })
		dashScopeGet, err := env.api.GetDashScopeTenantWithResponse(env.ctx, dashScope.Id)
		if err != nil {
			t.Fatalf("get DashScope tenant: %v", err)
		}
		requireStatusOK(t, dashScopeGet, dashScopeGet.Body)
		if dashScopeGet.JSON200 == nil || dashScopeGet.JSON200.Id != "qwen-dashscope-main" {
			t.Fatalf("get DashScope tenant = %#v", dashScopeGet.JSON200)
		}
	} else {
		t.Log("qwen-dashscope-main tenant is not configured in this e2e environment")
	}
}
