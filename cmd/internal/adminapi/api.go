package adminapi

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
)

type pagedItems[T any] struct {
	HasNext    bool
	Items      []T
	NextCursor *string
}

func collectAllPages[T any](
	fetchPage func(cursor *string, limit *int32) (pagedItems[T], error),
) ([]T, error) {
	limit := int32(200)
	var cursor *string
	items := make([]T, 0)
	for {
		page, err := fetchPage(cursor, &limit)
		if err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
		if !page.HasNext || page.NextCursor == nil || *page.NextCursor == "" {
			return items, nil
		}
		next := string(*page.NextCursor)
		if cursor != nil && next == *cursor {
			return nil, fmt.Errorf("gizclaw: paginated list cursor did not advance: %q", next)
		}
		cursor = &next
	}
}

func ListPeers(ctx context.Context, c *gizcli.Client) ([]apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.Registration], error) {
		resp, err := api.ListPeersWithResponse(ctx, &adminhttp.ListPeersParams{
			Cursor: cursor,
			Limit:  limit,
		})
		if err != nil {
			return pagedItems[apitypes.Registration]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.Registration]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.Registration]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func GetPeer(ctx context.Context, c *gizcli.Client, publicKey string) (apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Registration{}, err
	}
	resp, err := api.GetPeerWithResponse(ctx, publicKey)
	if err != nil {
		return apitypes.Registration{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Registration{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func FindPubKeyBySN(ctx context.Context, c *gizcli.Client, sn string) (string, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return "", err
	}
	resp, err := api.FindPubKeyBySNWithResponse(ctx, sn)
	if err != nil {
		return "", err
	}
	if resp.JSON200 != nil {
		return resp.JSON200.PublicKey, nil
	}
	return "", responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func FindPubKeyByIMEI(ctx context.Context, c *gizcli.Client, tac, serial string) (string, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return "", err
	}
	resp, err := api.FindPubKeyByIMEIWithResponse(ctx, tac, serial)
	if err != nil {
		return "", err
	}
	if resp.JSON200 != nil {
		return resp.JSON200.PublicKey, nil
	}
	return "", responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func ApprovePeer(ctx context.Context, c *gizcli.Client, publicKey string, role apitypes.PeerRole) (apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Registration{}, err
	}
	resp, err := api.ApprovePeerWithResponse(ctx, publicKey, adminhttp.ApproveRequest{Role: role})
	if err != nil {
		return apitypes.Registration{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Registration{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400)
}

func BlockPeer(ctx context.Context, c *gizcli.Client, publicKey string) (apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Registration{}, err
	}
	resp, err := api.BlockPeerWithResponse(ctx, publicKey)
	if err != nil {
		return apitypes.Registration{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Registration{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func GetPeerInfo(ctx context.Context, c *gizcli.Client, publicKey string) (apitypes.DeviceInfo, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	resp, err := api.GetPeerInfoWithResponse(ctx, publicKey)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.DeviceInfo{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func GetPeerRuntime(ctx context.Context, c *gizcli.Client, publicKey string) (apitypes.Runtime, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Runtime{}, err
	}
	resp, err := api.GetPeerRuntimeWithResponse(ctx, publicKey)
	if err != nil {
		return apitypes.Runtime{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Runtime{}, responseError(resp.StatusCode(), resp.Body)
}

func DeletePeer(ctx context.Context, c *gizcli.Client, publicKey string) (apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Registration{}, err
	}
	resp, err := api.DeletePeerWithResponse(ctx, publicKey)
	if err != nil {
		return apitypes.Registration{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Registration{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func RefreshPeer(ctx context.Context, c *gizcli.Client, publicKey string) (adminhttp.RefreshResult, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return adminhttp.RefreshResult{}, err
	}
	resp, err := api.RefreshPeerWithResponse(ctx, publicKey)
	if err != nil {
		return adminhttp.RefreshResult{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return adminhttp.RefreshResult{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON409, resp.JSON502)
}

func ListCredentials(ctx context.Context, c *gizcli.Client, provider string) ([]apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	var providerFilter *string
	if provider != "" {
		value := string(provider)
		providerFilter = &value
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.Credential], error) {
		resp, err := api.ListCredentialsWithResponse(ctx, &adminhttp.ListCredentialsParams{
			Provider: providerFilter,
			Cursor:   cursor,
			Limit:    limit,
		})
		if err != nil {
			return pagedItems[apitypes.Credential]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.Credential]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.Credential]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func CreateCredential(ctx context.Context, c *gizcli.Client, req adminhttp.CredentialUpsert) (apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Credential{}, err
	}
	resp, err := api.CreateCredentialWithResponse(ctx, req)
	if err != nil {
		return apitypes.Credential{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Credential{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func GetCredential(ctx context.Context, c *gizcli.Client, id string) (apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Credential{}, err
	}
	resp, err := api.GetCredentialWithResponse(ctx, id)
	if err != nil {
		return apitypes.Credential{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Credential{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutCredential(ctx context.Context, c *gizcli.Client, id string, req adminhttp.CredentialUpsert) (apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Credential{}, err
	}
	resp, err := api.PutCredentialWithResponse(ctx, id, req)
	if err != nil {
		return apitypes.Credential{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Credential{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteCredential(ctx context.Context, c *gizcli.Client, id string) (apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Credential{}, err
	}
	resp, err := api.DeleteCredentialWithResponse(ctx, id)
	if err != nil {
		return apitypes.Credential{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Credential{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListFirmwares(ctx context.Context, c *gizcli.Client) ([]apitypes.Firmware, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.Firmware], error) {
		resp, err := api.ListFirmwaresWithResponse(ctx, &adminhttp.ListFirmwaresParams{
			Cursor: cursor,
			Limit:  limit,
		})
		if err != nil {
			return pagedItems[apitypes.Firmware]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.Firmware]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.Firmware]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func CreateFirmware(ctx context.Context, c *gizcli.Client, req adminhttp.FirmwareUpsert) (apitypes.Firmware, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	resp, err := api.CreateFirmwareWithResponse(ctx, req)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Firmware{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func GetFirmware(ctx context.Context, c *gizcli.Client, id string) (apitypes.Firmware, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	resp, err := api.GetFirmwareWithResponse(ctx, id)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Firmware{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutFirmware(ctx context.Context, c *gizcli.Client, id string, req adminhttp.FirmwareUpsert) (apitypes.Firmware, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	resp, err := api.PutFirmwareWithResponse(ctx, id, req)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Firmware{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteFirmware(ctx context.Context, c *gizcli.Client, id string) (apitypes.Firmware, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	resp, err := api.DeleteFirmwareWithResponse(ctx, id)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Firmware{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ReleaseFirmware(ctx context.Context, c *gizcli.Client, id string) (apitypes.Firmware, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	resp, err := api.ReleaseFirmwareWithResponse(ctx, id)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Firmware{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON409, resp.JSON500)
}

func RollbackFirmware(ctx context.Context, c *gizcli.Client, id string) (apitypes.Firmware, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	resp, err := api.RollbackFirmwareWithResponse(ctx, id)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Firmware{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON409, resp.JSON500)
}

func UploadPetDefPixa(ctx context.Context, c *gizcli.Client, id string, body io.Reader) (apitypes.PetDef, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.PetDef{}, err
	}
	resp, err := api.UploadPetDefPixaWithBodyWithResponse(ctx, id, "application/octet-stream", body)
	if err != nil {
		return apitypes.PetDef{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.PetDef{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListMiniMaxTenants(ctx context.Context, c *gizcli.Client) ([]apitypes.MiniMaxTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.MiniMaxTenant], error) {
		resp, err := api.ListMiniMaxTenantsWithResponse(ctx, &adminhttp.ListMiniMaxTenantsParams{
			Cursor: cursor,
			Limit:  limit,
		})
		if err != nil {
			return pagedItems[apitypes.MiniMaxTenant]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.MiniMaxTenant]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.MiniMaxTenant]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func CreateMiniMaxTenant(ctx context.Context, c *gizcli.Client, req adminhttp.MiniMaxTenantUpsert) (apitypes.MiniMaxTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	resp, err := api.CreateMiniMaxTenantWithResponse(ctx, req)
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.MiniMaxTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func GetMiniMaxTenant(ctx context.Context, c *gizcli.Client, id string) (apitypes.MiniMaxTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	resp, err := api.GetMiniMaxTenantWithResponse(ctx, id)
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.MiniMaxTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutMiniMaxTenant(ctx context.Context, c *gizcli.Client, id string, req adminhttp.MiniMaxTenantUpsert) (apitypes.MiniMaxTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	resp, err := api.PutMiniMaxTenantWithResponse(ctx, id, req)
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.MiniMaxTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteMiniMaxTenant(ctx context.Context, c *gizcli.Client, id string) (apitypes.MiniMaxTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	resp, err := api.DeleteMiniMaxTenantWithResponse(ctx, id)
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.MiniMaxTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func SyncMiniMaxTenantVoices(ctx context.Context, c *gizcli.Client, id string) (adminhttp.MiniMaxSyncVoicesResult, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return adminhttp.MiniMaxSyncVoicesResult{}, err
	}
	resp, err := api.SyncMiniMaxTenantVoicesWithResponse(ctx, id)
	if err != nil {
		return adminhttp.MiniMaxSyncVoicesResult{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return adminhttp.MiniMaxSyncVoicesResult{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON500, resp.JSON502)
}

func ListVolcTenants(ctx context.Context, c *gizcli.Client) ([]apitypes.VolcTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.VolcTenant], error) {
		resp, err := api.ListVolcTenantsWithResponse(ctx, &adminhttp.ListVolcTenantsParams{
			Cursor: cursor,
			Limit:  limit,
		})
		if err != nil {
			return pagedItems[apitypes.VolcTenant]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.VolcTenant]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.VolcTenant]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func GetVolcTenant(ctx context.Context, c *gizcli.Client, id string) (apitypes.VolcTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.VolcTenant{}, err
	}
	resp, err := api.GetVolcTenantWithResponse(ctx, id)
	if err != nil {
		return apitypes.VolcTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.VolcTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func SyncVolcTenantVoices(ctx context.Context, c *gizcli.Client, id string) (adminhttp.VolcSyncVoicesResult, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return adminhttp.VolcSyncVoicesResult{}, err
	}
	resp, err := api.SyncVolcTenantVoicesWithResponse(ctx, id)
	if err != nil {
		return adminhttp.VolcSyncVoicesResult{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return adminhttp.VolcSyncVoicesResult{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON500, resp.JSON502)
}

func ListOpenAITenants(ctx context.Context, c *gizcli.Client) ([]apitypes.OpenAITenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.OpenAITenant], error) {
		resp, err := api.ListOpenAITenantsWithResponse(ctx, &adminhttp.ListOpenAITenantsParams{
			Cursor: cursor,
			Limit:  limit,
		})
		if err != nil {
			return pagedItems[apitypes.OpenAITenant]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.OpenAITenant]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.OpenAITenant]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func GetOpenAITenant(ctx context.Context, c *gizcli.Client, id string) (apitypes.OpenAITenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.OpenAITenant{}, err
	}
	resp, err := api.GetOpenAITenantWithResponse(ctx, id)
	if err != nil {
		return apitypes.OpenAITenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.OpenAITenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListGeminiTenants(ctx context.Context, c *gizcli.Client) ([]apitypes.GeminiTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.GeminiTenant], error) {
		resp, err := api.ListGeminiTenantsWithResponse(ctx, &adminhttp.ListGeminiTenantsParams{
			Cursor: cursor,
			Limit:  limit,
		})
		if err != nil {
			return pagedItems[apitypes.GeminiTenant]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.GeminiTenant]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.GeminiTenant]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func GetGeminiTenant(ctx context.Context, c *gizcli.Client, id string) (apitypes.GeminiTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.GeminiTenant{}, err
	}
	resp, err := api.GetGeminiTenantWithResponse(ctx, id)
	if err != nil {
		return apitypes.GeminiTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.GeminiTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListDashScopeTenants(ctx context.Context, c *gizcli.Client) ([]apitypes.DashScopeTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.DashScopeTenant], error) {
		resp, err := api.ListDashScopeTenantsWithResponse(ctx, &adminhttp.ListDashScopeTenantsParams{
			Cursor: cursor,
			Limit:  limit,
		})
		if err != nil {
			return pagedItems[apitypes.DashScopeTenant]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.DashScopeTenant]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.DashScopeTenant]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func GetDashScopeTenant(ctx context.Context, c *gizcli.Client, id string) (apitypes.DashScopeTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.DashScopeTenant{}, err
	}
	resp, err := api.GetDashScopeTenantWithResponse(ctx, id)
	if err != nil {
		return apitypes.DashScopeTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.DashScopeTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListDeepSeekTenants(ctx context.Context, c *gizcli.Client) ([]apitypes.DeepSeekTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.DeepSeekTenant], error) {
		resp, err := api.ListDeepSeekTenantsWithResponse(ctx, &adminhttp.ListDeepSeekTenantsParams{
			Cursor: cursor,
			Limit:  limit,
		})
		if err != nil {
			return pagedItems[apitypes.DeepSeekTenant]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.DeepSeekTenant]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.DeepSeekTenant]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func GetDeepSeekTenant(ctx context.Context, c *gizcli.Client, id string) (apitypes.DeepSeekTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.DeepSeekTenant{}, err
	}
	resp, err := api.GetDeepSeekTenantWithResponse(ctx, id)
	if err != nil {
		return apitypes.DeepSeekTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.DeepSeekTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func CreateDeepSeekTenant(ctx context.Context, c *gizcli.Client, req adminhttp.DeepSeekTenantUpsert) (apitypes.DeepSeekTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.DeepSeekTenant{}, err
	}
	resp, err := api.CreateDeepSeekTenantWithResponse(ctx, req)
	if err != nil {
		return apitypes.DeepSeekTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.DeepSeekTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func PutDeepSeekTenant(ctx context.Context, c *gizcli.Client, id string, req adminhttp.DeepSeekTenantUpsert) (apitypes.DeepSeekTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.DeepSeekTenant{}, err
	}
	resp, err := api.PutDeepSeekTenantWithResponse(ctx, id, req)
	if err != nil {
		return apitypes.DeepSeekTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.DeepSeekTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteDeepSeekTenant(ctx context.Context, c *gizcli.Client, id string) (apitypes.DeepSeekTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.DeepSeekTenant{}, err
	}
	resp, err := api.DeleteDeepSeekTenantWithResponse(ctx, id)
	if err != nil {
		return apitypes.DeepSeekTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.DeepSeekTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListModels(ctx context.Context, c *gizcli.Client, source, providerKind, providerID string) ([]apitypes.Model, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	var sourceFilter *adminhttp.ModelSource
	if source != "" {
		value := adminhttp.ModelSource(source)
		sourceFilter = &value
	}
	var providerKindFilter *adminhttp.ModelProviderKind
	if providerKind != "" {
		value := adminhttp.ModelProviderKind(providerKind)
		providerKindFilter = &value
	}
	var providerIDFilter *string
	if providerID != "" {
		value := providerID
		providerIDFilter = &value
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.Model], error) {
		resp, err := api.ListModelsWithResponse(ctx, &adminhttp.ListModelsParams{
			Source:       sourceFilter,
			ProviderKind: providerKindFilter,
			ProviderId:   providerIDFilter,
			Cursor:       cursor,
			Limit:        limit,
		})
		if err != nil {
			return pagedItems[apitypes.Model]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.Model]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.Model]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func GetModel(ctx context.Context, c *gizcli.Client, id string) (apitypes.Model, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Model{}, err
	}
	resp, err := api.GetModelWithResponse(ctx, id)
	if err != nil {
		return apitypes.Model{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Model{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListVoices(ctx context.Context, c *gizcli.Client, source, providerKind, providerID string) ([]apitypes.Voice, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	var sourceFilter *adminhttp.VoiceSource
	if source != "" {
		value := adminhttp.VoiceSource(source)
		sourceFilter = &value
	}
	var providerKindFilter *adminhttp.VoiceProviderKind
	if providerKind != "" {
		value := adminhttp.VoiceProviderKind(providerKind)
		providerKindFilter = &value
	}
	var providerIDFilter *string
	if providerID != "" {
		value := string(providerID)
		providerIDFilter = &value
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.Voice], error) {
		resp, err := api.ListVoicesWithResponse(ctx, &adminhttp.ListVoicesParams{
			Source:       sourceFilter,
			ProviderKind: providerKindFilter,
			ProviderId:   providerIDFilter,
			Cursor:       cursor,
			Limit:        limit,
		})
		if err != nil {
			return pagedItems[apitypes.Voice]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.Voice]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.Voice]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func CreateVoice(ctx context.Context, c *gizcli.Client, req adminhttp.VoiceUpsert) (apitypes.Voice, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Voice{}, err
	}
	resp, err := api.CreateVoiceWithResponse(ctx, req)
	if err != nil {
		return apitypes.Voice{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Voice{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func GetVoice(ctx context.Context, c *gizcli.Client, id string) (apitypes.Voice, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Voice{}, err
	}
	resp, err := api.GetVoiceWithResponse(ctx, string(id))
	if err != nil {
		return apitypes.Voice{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Voice{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutVoice(ctx context.Context, c *gizcli.Client, id string, req adminhttp.VoiceUpsert) (apitypes.Voice, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Voice{}, err
	}
	resp, err := api.PutVoiceWithResponse(ctx, string(id), req)
	if err != nil {
		return apitypes.Voice{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Voice{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func DeleteVoice(ctx context.Context, c *gizcli.Client, id string) (apitypes.Voice, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Voice{}, err
	}
	resp, err := api.DeleteVoiceWithResponse(ctx, string(id))
	if err != nil {
		return apitypes.Voice{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Voice{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListWorkflows(ctx context.Context, c *gizcli.Client) ([]apitypes.Workflow, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.Workflow], error) {
		resp, err := api.ListWorkflowsWithResponse(ctx, &adminhttp.ListWorkflowsParams{
			Cursor: cursor,
			Limit:  limit,
		})
		if err != nil {
			return pagedItems[apitypes.Workflow]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.Workflow]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.Workflow]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func CreateWorkflow(ctx context.Context, c *gizcli.Client, req apitypes.Workflow) (apitypes.Workflow, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workflow{}, err
	}
	resp, err := api.CreateWorkflowWithResponse(ctx, adminhttp.WorkflowUpsert{Id: req.Id, Spec: req.Spec})
	if err != nil {
		return apitypes.Workflow{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workflow{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func GetWorkflow(ctx context.Context, c *gizcli.Client, id string) (apitypes.Workflow, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workflow{}, err
	}
	resp, err := api.GetWorkflowWithResponse(ctx, id)
	if err != nil {
		return apitypes.Workflow{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workflow{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutWorkflow(ctx context.Context, c *gizcli.Client, id string, req apitypes.Workflow) (apitypes.Workflow, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workflow{}, err
	}
	resp, err := api.PutWorkflowWithResponse(ctx, id, adminhttp.WorkflowUpsert{Id: req.Id, Spec: req.Spec})
	if err != nil {
		return apitypes.Workflow{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workflow{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteWorkflow(ctx context.Context, c *gizcli.Client, id string) (apitypes.Workflow, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workflow{}, err
	}
	resp, err := api.DeleteWorkflowWithResponse(ctx, id)
	if err != nil {
		return apitypes.Workflow{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workflow{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListWorkspaces(ctx context.Context, c *gizcli.Client) ([]apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.Workspace], error) {
		resp, err := api.ListWorkspacesWithResponse(ctx, &adminhttp.ListWorkspacesParams{
			Cursor: cursor,
			Limit:  limit,
		})
		if err != nil {
			return pagedItems[apitypes.Workspace]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.Workspace]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.Workspace]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func CreateWorkspace(ctx context.Context, c *gizcli.Client, req adminhttp.WorkspaceUpsert) (apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	resp, err := api.CreateWorkspaceWithResponse(ctx, req)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workspace{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func GetWorkspace(ctx context.Context, c *gizcli.Client, id string) (apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	resp, err := api.GetWorkspaceWithResponse(ctx, id)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workspace{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutWorkspace(ctx context.Context, c *gizcli.Client, id string, req adminhttp.WorkspaceUpsert) (apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	resp, err := api.PutWorkspaceWithResponse(ctx, id, req)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workspace{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteWorkspace(ctx context.Context, c *gizcli.Client, id string) (apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	resp, err := api.DeleteWorkspaceWithResponse(ctx, id)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workspace{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListRuntimeProfiles(ctx context.Context, c *gizcli.Client) ([]apitypes.RuntimeProfile, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.RuntimeProfile], error) {
		resp, err := api.ListRuntimeProfilesWithResponse(ctx, &adminhttp.ListRuntimeProfilesParams{Cursor: cursor, Limit: limit})
		if err != nil {
			return pagedItems[apitypes.RuntimeProfile]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.RuntimeProfile]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.RuntimeProfile]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func CreateRuntimeProfile(ctx context.Context, c *gizcli.Client, req adminhttp.RuntimeProfileUpsert) (apitypes.RuntimeProfile, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	resp, err := api.CreateRuntimeProfileWithResponse(ctx, req)
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.RuntimeProfile{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func GetRuntimeProfile(ctx context.Context, c *gizcli.Client, id string) (apitypes.RuntimeProfile, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	resp, err := api.GetRuntimeProfileWithResponse(ctx, id)
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.RuntimeProfile{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutRuntimeProfile(ctx context.Context, c *gizcli.Client, id string, req adminhttp.RuntimeProfileUpsert) (apitypes.RuntimeProfile, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	resp, err := api.PutRuntimeProfileWithResponse(ctx, id, req)
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.RuntimeProfile{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteRuntimeProfile(ctx context.Context, c *gizcli.Client, id string) (apitypes.RuntimeProfile, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	resp, err := api.DeleteRuntimeProfileWithResponse(ctx, id)
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.RuntimeProfile{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListRegistrationTokens(ctx context.Context, c *gizcli.Client) ([]apitypes.RegistrationToken, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.RegistrationToken], error) {
		resp, err := api.ListRegistrationTokensWithResponse(ctx, &adminhttp.ListRegistrationTokensParams{Cursor: cursor, Limit: limit})
		if err != nil {
			return pagedItems[apitypes.RegistrationToken]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.RegistrationToken]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.RegistrationToken]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func CreateRegistrationToken(ctx context.Context, c *gizcli.Client, req adminhttp.RegistrationTokenUpsert) (apitypes.RegistrationToken, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.RegistrationToken{}, err
	}
	resp, err := api.CreateRegistrationTokenWithResponse(ctx, req)
	if err != nil {
		return apitypes.RegistrationToken{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.RegistrationToken{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func PutRegistrationToken(ctx context.Context, c *gizcli.Client, id string, req adminhttp.RegistrationTokenUpsert) (apitypes.RegistrationToken, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.RegistrationToken{}, err
	}
	resp, err := api.PutRegistrationTokenWithResponse(ctx, id, req)
	if err != nil {
		return apitypes.RegistrationToken{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.RegistrationToken{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func GetRegistrationToken(ctx context.Context, c *gizcli.Client, id string) (apitypes.RegistrationToken, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.RegistrationToken{}, err
	}
	resp, err := api.GetRegistrationTokenWithResponse(ctx, id)
	if err != nil {
		return apitypes.RegistrationToken{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.RegistrationToken{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func DeleteRegistrationToken(ctx context.Context, c *gizcli.Client, id string) (apitypes.RegistrationToken, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.RegistrationToken{}, err
	}
	resp, err := api.DeleteRegistrationTokenWithResponse(ctx, id)
	if err != nil {
		return apitypes.RegistrationToken{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.RegistrationToken{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func responseError(status int, body []byte, errs ...any) error {
	for _, errResp := range errs {
		switch e := errResp.(type) {
		case *apitypes.ErrorResponse:
			if e != nil {
				return fmt.Errorf("%s: %s", e.Error.Code, e.Error.Message)
			}
		}
	}
	text := strings.TrimSpace(string(body))
	if text != "" {
		return fmt.Errorf("unexpected status %d: %s", status, text)
	}
	if status != 0 {
		return fmt.Errorf("unexpected status %d", status)
	}
	return fmt.Errorf("unexpected empty response")
}
