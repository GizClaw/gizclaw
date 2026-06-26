package adminapi

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
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
		resp, err := api.ListPeersWithResponse(ctx, &adminservice.ListPeersParams{
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
	resp, err := api.ApprovePeerWithResponse(ctx, publicKey, adminservice.ApproveRequest{Role: role})
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

func GetPeerConfig(ctx context.Context, c *gizcli.Client, publicKey string) (apitypes.Configuration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Configuration{}, err
	}
	resp, err := api.GetPeerConfigWithResponse(ctx, publicKey)
	if err != nil {
		return apitypes.Configuration{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Configuration{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func PutPeerConfig(ctx context.Context, c *gizcli.Client, publicKey string, cfg apitypes.Configuration) (apitypes.Configuration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Configuration{}, err
	}
	resp, err := api.PutPeerConfigWithResponse(ctx, publicKey, cfg)
	if err != nil {
		return apitypes.Configuration{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Configuration{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404)
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

func RefreshPeer(ctx context.Context, c *gizcli.Client, publicKey string) (adminservice.RefreshResult, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return adminservice.RefreshResult{}, err
	}
	resp, err := api.RefreshPeerWithResponse(ctx, publicKey)
	if err != nil {
		return adminservice.RefreshResult{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return adminservice.RefreshResult{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON409, resp.JSON502)
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
		resp, err := api.ListCredentialsWithResponse(ctx, &adminservice.ListCredentialsParams{
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

func CreateCredential(ctx context.Context, c *gizcli.Client, req adminservice.CredentialUpsert) (apitypes.Credential, error) {
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

func GetCredential(ctx context.Context, c *gizcli.Client, name string) (apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Credential{}, err
	}
	resp, err := api.GetCredentialWithResponse(ctx, string(name))
	if err != nil {
		return apitypes.Credential{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Credential{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutCredential(ctx context.Context, c *gizcli.Client, name string, req adminservice.CredentialUpsert) (apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Credential{}, err
	}
	resp, err := api.PutCredentialWithResponse(ctx, string(name), req)
	if err != nil {
		return apitypes.Credential{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Credential{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteCredential(ctx context.Context, c *gizcli.Client, name string) (apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Credential{}, err
	}
	resp, err := api.DeleteCredentialWithResponse(ctx, string(name))
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
		resp, err := api.ListFirmwaresWithResponse(ctx, &adminservice.ListFirmwaresParams{
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

func CreateFirmware(ctx context.Context, c *gizcli.Client, req adminservice.FirmwareUpsert) (apitypes.Firmware, error) {
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

func GetFirmware(ctx context.Context, c *gizcli.Client, name string) (apitypes.Firmware, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	resp, err := api.GetFirmwareWithResponse(ctx, name)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Firmware{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutFirmware(ctx context.Context, c *gizcli.Client, name string, req adminservice.FirmwareUpsert) (apitypes.Firmware, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	resp, err := api.PutFirmwareWithResponse(ctx, name, req)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Firmware{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteFirmware(ctx context.Context, c *gizcli.Client, name string) (apitypes.Firmware, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	resp, err := api.DeleteFirmwareWithResponse(ctx, name)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Firmware{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ReleaseFirmware(ctx context.Context, c *gizcli.Client, name string) (apitypes.Firmware, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	resp, err := api.ReleaseFirmwareWithResponse(ctx, name)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Firmware{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON409, resp.JSON500)
}

func RollbackFirmware(ctx context.Context, c *gizcli.Client, name string) (apitypes.Firmware, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	resp, err := api.RollbackFirmwareWithResponse(ctx, name)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Firmware{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON409, resp.JSON500)
}

func UploadFirmwareArtifact(ctx context.Context, c *gizcli.Client, name, channel string, body io.Reader) (apitypes.Firmware, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	resp, err := api.UploadFirmwareArtifactWithBodyWithResponse(ctx, name, adminservice.UploadFirmwareArtifactParamsChannel(channel), "application/x-tar", body)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Firmware{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON409, resp.JSON500)
}

func DownloadFirmwareArtifact(ctx context.Context, c *gizcli.Client, name, channel string) ([]byte, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	resp, err := api.DownloadFirmwareArtifactWithResponse(ctx, name, adminservice.DownloadFirmwareArtifactParamsChannel(channel))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() == 200 {
		return resp.Body, nil
	}
	return nil, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func DeleteFirmwareArtifact(ctx context.Context, c *gizcli.Client, name, channel string) (apitypes.Firmware, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	resp, err := api.DeleteFirmwareArtifactWithResponse(ctx, name, adminservice.DeleteFirmwareArtifactParamsChannel(channel))
	if err != nil {
		return apitypes.Firmware{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Firmware{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListFirmwareArtifactEntries(ctx context.Context, c *gizcli.Client, name, channel, path string) (apitypes.FirmwareArtifactList, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.FirmwareArtifactList{}, err
	}
	params := &adminservice.ListFirmwareArtifactEntriesParams{}
	if strings.TrimSpace(path) != "" {
		params.Path = &path
	}
	resp, err := api.ListFirmwareArtifactEntriesWithResponse(ctx, name, adminservice.ListFirmwareArtifactEntriesParamsChannel(channel), params)
	if err != nil {
		return apitypes.FirmwareArtifactList{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.FirmwareArtifactList{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON500)
}

func TreeFirmwareArtifactEntries(ctx context.Context, c *gizcli.Client, name, channel, path string) (apitypes.FirmwareArtifactTree, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.FirmwareArtifactTree{}, err
	}
	params := &adminservice.TreeFirmwareArtifactEntriesParams{}
	if strings.TrimSpace(path) != "" {
		params.Path = &path
	}
	resp, err := api.TreeFirmwareArtifactEntriesWithResponse(ctx, name, adminservice.TreeFirmwareArtifactEntriesParamsChannel(channel), params)
	if err != nil {
		return apitypes.FirmwareArtifactTree{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.FirmwareArtifactTree{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON500)
}

func StatFirmwareArtifactEntry(ctx context.Context, c *gizcli.Client, name, channel, path string) (apitypes.FirmwareArtifactStats, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.FirmwareArtifactStats{}, err
	}
	params := &adminservice.StatFirmwareArtifactEntryParams{}
	if strings.TrimSpace(path) != "" {
		params.Path = &path
	}
	resp, err := api.StatFirmwareArtifactEntryWithResponse(ctx, name, adminservice.StatFirmwareArtifactEntryParamsChannel(channel), params)
	if err != nil {
		return apitypes.FirmwareArtifactStats{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.FirmwareArtifactStats{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON500)
}

func DownloadFirmwareArtifactEntry(ctx context.Context, c *gizcli.Client, name, channel, path string) ([]byte, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	params := &adminservice.DownloadFirmwareArtifactEntryParams{Path: path}
	resp, err := api.DownloadFirmwareArtifactEntryWithResponse(ctx, name, adminservice.DownloadFirmwareArtifactEntryParamsChannel(channel), params)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() == 200 {
		return resp.Body, nil
	}
	return nil, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON500)
}

func ListMiniMaxTenants(ctx context.Context, c *gizcli.Client) ([]apitypes.MiniMaxTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.MiniMaxTenant], error) {
		resp, err := api.ListMiniMaxTenantsWithResponse(ctx, &adminservice.ListMiniMaxTenantsParams{
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

func CreateMiniMaxTenant(ctx context.Context, c *gizcli.Client, req adminservice.MiniMaxTenantUpsert) (apitypes.MiniMaxTenant, error) {
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

func GetMiniMaxTenant(ctx context.Context, c *gizcli.Client, name string) (apitypes.MiniMaxTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	resp, err := api.GetMiniMaxTenantWithResponse(ctx, string(name))
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.MiniMaxTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutMiniMaxTenant(ctx context.Context, c *gizcli.Client, name string, req adminservice.MiniMaxTenantUpsert) (apitypes.MiniMaxTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	resp, err := api.PutMiniMaxTenantWithResponse(ctx, string(name), req)
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.MiniMaxTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteMiniMaxTenant(ctx context.Context, c *gizcli.Client, name string) (apitypes.MiniMaxTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	resp, err := api.DeleteMiniMaxTenantWithResponse(ctx, string(name))
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.MiniMaxTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func SyncMiniMaxTenantVoices(ctx context.Context, c *gizcli.Client, name string) (adminservice.MiniMaxSyncVoicesResult, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return adminservice.MiniMaxSyncVoicesResult{}, err
	}
	resp, err := api.SyncMiniMaxTenantVoicesWithResponse(ctx, string(name))
	if err != nil {
		return adminservice.MiniMaxSyncVoicesResult{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return adminservice.MiniMaxSyncVoicesResult{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON500, resp.JSON502)
}

func ListVolcTenants(ctx context.Context, c *gizcli.Client) ([]apitypes.VolcTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.VolcTenant], error) {
		resp, err := api.ListVolcTenantsWithResponse(ctx, &adminservice.ListVolcTenantsParams{
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

func GetVolcTenant(ctx context.Context, c *gizcli.Client, name string) (apitypes.VolcTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.VolcTenant{}, err
	}
	resp, err := api.GetVolcTenantWithResponse(ctx, string(name))
	if err != nil {
		return apitypes.VolcTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.VolcTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func SyncVolcTenantVoices(ctx context.Context, c *gizcli.Client, name string) (adminservice.VolcSyncVoicesResult, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return adminservice.VolcSyncVoicesResult{}, err
	}
	resp, err := api.SyncVolcTenantVoicesWithResponse(ctx, string(name))
	if err != nil {
		return adminservice.VolcSyncVoicesResult{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return adminservice.VolcSyncVoicesResult{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON500, resp.JSON502)
}

func ListOpenAITenants(ctx context.Context, c *gizcli.Client) ([]apitypes.OpenAITenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.OpenAITenant], error) {
		resp, err := api.ListOpenAITenantsWithResponse(ctx, &adminservice.ListOpenAITenantsParams{
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

func GetOpenAITenant(ctx context.Context, c *gizcli.Client, name string) (apitypes.OpenAITenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.OpenAITenant{}, err
	}
	resp, err := api.GetOpenAITenantWithResponse(ctx, name)
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
		resp, err := api.ListGeminiTenantsWithResponse(ctx, &adminservice.ListGeminiTenantsParams{
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

func GetGeminiTenant(ctx context.Context, c *gizcli.Client, name string) (apitypes.GeminiTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.GeminiTenant{}, err
	}
	resp, err := api.GetGeminiTenantWithResponse(ctx, name)
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
		resp, err := api.ListDashScopeTenantsWithResponse(ctx, &adminservice.ListDashScopeTenantsParams{
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

func GetDashScopeTenant(ctx context.Context, c *gizcli.Client, name string) (apitypes.DashScopeTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.DashScopeTenant{}, err
	}
	resp, err := api.GetDashScopeTenantWithResponse(ctx, name)
	if err != nil {
		return apitypes.DashScopeTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.DashScopeTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListModels(ctx context.Context, c *gizcli.Client, source, providerKind, providerName string) ([]apitypes.Model, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	var sourceFilter *adminservice.ModelSource
	if source != "" {
		value := adminservice.ModelSource(source)
		sourceFilter = &value
	}
	var providerKindFilter *adminservice.ModelProviderKind
	if providerKind != "" {
		value := adminservice.ModelProviderKind(providerKind)
		providerKindFilter = &value
	}
	var providerNameFilter *string
	if providerName != "" {
		value := providerName
		providerNameFilter = &value
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.Model], error) {
		resp, err := api.ListModelsWithResponse(ctx, &adminservice.ListModelsParams{
			Source:       sourceFilter,
			ProviderKind: providerKindFilter,
			ProviderName: providerNameFilter,
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

func ListACLViews(ctx context.Context, c *gizcli.Client) ([]apitypes.ACLView, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.ACLView], error) {
		resp, err := api.ListACLViewsWithResponse(ctx, &adminservice.ListACLViewsParams{
			Cursor: cursor,
			Limit:  limit,
		})
		if err != nil {
			return pagedItems[apitypes.ACLView]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.ACLView]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.ACLView]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func GetACLView(ctx context.Context, c *gizcli.Client, name string) (apitypes.ACLView, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.ACLView{}, err
	}
	resp, err := api.GetACLViewWithResponse(ctx, name)
	if err != nil {
		return apitypes.ACLView{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.ACLView{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListVoices(ctx context.Context, c *gizcli.Client, source, providerKind, providerName string) ([]apitypes.Voice, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	var sourceFilter *adminservice.VoiceSource
	if source != "" {
		value := adminservice.VoiceSource(source)
		sourceFilter = &value
	}
	var providerKindFilter *adminservice.VoiceProviderKind
	if providerKind != "" {
		value := adminservice.VoiceProviderKind(providerKind)
		providerKindFilter = &value
	}
	var providerNameFilter *string
	if providerName != "" {
		value := string(providerName)
		providerNameFilter = &value
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.Voice], error) {
		resp, err := api.ListVoicesWithResponse(ctx, &adminservice.ListVoicesParams{
			Source:       sourceFilter,
			ProviderKind: providerKindFilter,
			ProviderName: providerNameFilter,
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

func CreateVoice(ctx context.Context, c *gizcli.Client, req adminservice.VoiceUpsert) (apitypes.Voice, error) {
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

func PutVoice(ctx context.Context, c *gizcli.Client, id string, req adminservice.VoiceUpsert) (apitypes.Voice, error) {
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

func ListWorkflows(ctx context.Context, c *gizcli.Client) ([]apitypes.WorkflowDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.WorkflowDocument], error) {
		resp, err := api.ListWorkflowsWithResponse(ctx, &adminservice.ListWorkflowsParams{
			Cursor: cursor,
			Limit:  limit,
		})
		if err != nil {
			return pagedItems[apitypes.WorkflowDocument]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.WorkflowDocument]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.WorkflowDocument]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func CreateWorkflow(ctx context.Context, c *gizcli.Client, req apitypes.WorkflowDocument) (apitypes.WorkflowDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	resp, err := api.CreateWorkflowWithResponse(ctx, req)
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.WorkflowDocument{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func GetWorkflow(ctx context.Context, c *gizcli.Client, name string) (apitypes.WorkflowDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	resp, err := api.GetWorkflowWithResponse(ctx, string(name))
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.WorkflowDocument{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutWorkflow(ctx context.Context, c *gizcli.Client, name string, req apitypes.WorkflowDocument) (apitypes.WorkflowDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	resp, err := api.PutWorkflowWithResponse(ctx, string(name), req)
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.WorkflowDocument{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteWorkflow(ctx context.Context, c *gizcli.Client, name string) (apitypes.WorkflowDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	resp, err := api.DeleteWorkflowWithResponse(ctx, string(name))
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.WorkflowDocument{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListWorkspaces(ctx context.Context, c *gizcli.Client) ([]apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *string, limit *int32) (pagedItems[apitypes.Workspace], error) {
		resp, err := api.ListWorkspacesWithResponse(ctx, &adminservice.ListWorkspacesParams{
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

func CreateWorkspace(ctx context.Context, c *gizcli.Client, req adminservice.WorkspaceUpsert) (apitypes.Workspace, error) {
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

func GetWorkspace(ctx context.Context, c *gizcli.Client, name string) (apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	resp, err := api.GetWorkspaceWithResponse(ctx, string(name))
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workspace{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutWorkspace(ctx context.Context, c *gizcli.Client, name string, req adminservice.WorkspaceUpsert) (apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	resp, err := api.PutWorkspaceWithResponse(ctx, string(name), req)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workspace{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteWorkspace(ctx context.Context, c *gizcli.Client, name string) (apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	resp, err := api.DeleteWorkspaceWithResponse(ctx, string(name))
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workspace{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func responseError(status int, body []byte, errs ...interface{}) error {
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
