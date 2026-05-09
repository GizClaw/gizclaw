package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/gearservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

func ConnectFromContext(name string) (*gizclaw.Client, error) {
	c, serverPK, serverAddr, err := DialFromContext(name)
	if err != nil {
		return nil, err
	}
	if err := c.Dial(serverPK, serverAddr); err != nil {
		return nil, err
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.Serve()
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			_ = c.Close()
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("gizclaw: client stopped before ready")
		default:
		}
		if err := probeServerPublicReady(c); err == nil {
			return c, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = c.Close()
	return nil, fmt.Errorf("gizclaw: timeout waiting for client readiness")
}

func probeServerPublicReady(c *gizclaw.Client) error {
	if c == nil {
		return fmt.Errorf("gizclaw: nil client")
	}
	if c.PeerConn() == nil {
		return fmt.Errorf("gizclaw: client is not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := GetServerInfo(ctx, c)
	return err
}

func Register(ctx context.Context, c *gizclaw.Client, req gearservice.RegistrationRequest) (gearservice.RegistrationResult, error) {
	rpcReq, err := convertClientAPIType[rpcapi.GearRegisterRequest](req)
	if err != nil {
		return gearservice.RegistrationResult{}, err
	}
	resp, err := c.RegisterGear(ctx, "gear.registration.register", rpcReq)
	if err != nil {
		return gearservice.RegistrationResult{}, err
	}
	return convertClientAPIType[gearservice.RegistrationResult](*resp)
}

func GetConfig(ctx context.Context, c *gizclaw.Client) (apitypes.Configuration, error) {
	resp, err := c.GetGearConfig(ctx, "gear.config.get")
	if err != nil {
		return apitypes.Configuration{}, err
	}
	cfg, err := convertClientAPIType[apitypes.Configuration](*resp)
	if err != nil {
		return apitypes.Configuration{}, err
	}
	if cfg.Firmware == nil {
		cfg.Firmware = &apitypes.FirmwareConfig{}
	}
	return cfg, nil
}

func GetServerInfo(ctx context.Context, c *gizclaw.Client) (apitypes.ServerInfo, error) {
	api, err := c.ServerPublicClient()
	if err != nil {
		return apitypes.ServerInfo{}, err
	}
	resp, err := api.GetServerInfoWithResponse(ctx)
	if err != nil {
		return apitypes.ServerInfo{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.ServerInfo{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400)
}

func GetInfo(ctx context.Context, c *gizclaw.Client) (apitypes.DeviceInfo, error) {
	resp, err := c.GetGearInfo(ctx, "gear.info.get")
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	return convertClientAPIType[apitypes.DeviceInfo](*resp)
}

func PutInfo(ctx context.Context, c *gizclaw.Client, info apitypes.DeviceInfo) (apitypes.DeviceInfo, error) {
	rpcReq, err := convertClientAPIType[rpcapi.GearPutInfoRequest](info)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	resp, err := c.PutGearInfo(ctx, "gear.info.put", rpcReq)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	return convertClientAPIType[apitypes.DeviceInfo](*resp)
}

func GetRuntime(ctx context.Context, c *gizclaw.Client) (apitypes.Runtime, error) {
	resp, err := c.GetGearRuntime(ctx, "gear.runtime.get")
	if err != nil {
		return apitypes.Runtime{}, err
	}
	return convertClientAPIType[apitypes.Runtime](*resp)
}

func GetRegistration(ctx context.Context, c *gizclaw.Client) (apitypes.Registration, error) {
	resp, err := c.GetGearRegistration(ctx, "gear.registration.get")
	if err != nil {
		return apitypes.Registration{}, err
	}
	return convertClientAPIType[apitypes.Registration](*resp)
}

func GetOTA(ctx context.Context, c *gizclaw.Client) (apitypes.OTASummary, error) {
	resp, err := c.GetGearOTA(ctx, "gear.ota.get")
	if err != nil {
		return apitypes.OTASummary{}, err
	}
	return convertClientAPIType[apitypes.OTASummary](*resp)
}

func convertClientAPIType[T any](value any) (T, error) {
	var out T
	data, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func DownloadFirmware(ctx context.Context, c *gizclaw.Client, path string) ([]byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://gizclaw/download/firmware/"+url.PathEscape(path), nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.HTTPClient(gizclaw.ServiceGear).Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, err
		}
		return data, resp.Header.Clone(), nil
	}
	body, _ := io.ReadAll(resp.Body)
	return nil, nil, responseError(resp.StatusCode, body)
}

func ListFirmwares(ctx context.Context, c *gizclaw.Client) ([]apitypes.Depot, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	resp, err := api.ListDepotsWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
	}
	return resp.JSON200.Items, nil
}

func GetFirmwareDepot(ctx context.Context, c *gizclaw.Client, depot string) (apitypes.Depot, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Depot{}, err
	}
	resp, err := api.GetDepotWithResponse(ctx, depot)
	if err != nil {
		return apitypes.Depot{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Depot{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func GetFirmwareChannel(ctx context.Context, c *gizclaw.Client, depot string, channel adminservice.Channel) (apitypes.DepotRelease, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.DepotRelease{}, err
	}
	resp, err := api.GetChannelWithResponse(ctx, depot, channel)
	if err != nil {
		return apitypes.DepotRelease{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.DepotRelease{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func PutFirmwareInfo(ctx context.Context, c *gizclaw.Client, depot string, info apitypes.DepotInfo) (apitypes.Depot, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Depot{}, err
	}
	resp, err := api.PutDepotInfoWithResponse(ctx, depot, info)
	if err != nil {
		return apitypes.Depot{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Depot{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func UploadFirmware(ctx context.Context, c *gizclaw.Client, depot string, channel adminservice.Channel, data []byte) (apitypes.DepotRelease, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.DepotRelease{}, err
	}
	resp, err := api.PutChannelWithBodyWithResponse(ctx, depot, channel, "application/octet-stream", bytes.NewReader(data))
	if err != nil {
		return apitypes.DepotRelease{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.DepotRelease{}, responseError(resp.StatusCode(), resp.Body, resp.JSON409)
}

func ReleaseFirmware(ctx context.Context, c *gizclaw.Client, depot string) (apitypes.Depot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://gizclaw/depots/"+url.PathEscape(depot)+"/@release", nil)
	if err != nil {
		return apitypes.Depot{}, err
	}
	resp, err := c.HTTPClient(gizclaw.ServiceAdmin).Do(req)
	if err != nil {
		return apitypes.Depot{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return apitypes.Depot{}, err
	}
	if resp.StatusCode == http.StatusOK {
		var out apitypes.Depot
		if err := json.Unmarshal(body, &out); err != nil {
			return apitypes.Depot{}, err
		}
		return out, nil
	}
	return apitypes.Depot{}, responseError(resp.StatusCode, body)
}

func RollbackFirmware(ctx context.Context, c *gizclaw.Client, depot string) (apitypes.Depot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://gizclaw/depots/"+url.PathEscape(depot)+"/@rollback", nil)
	if err != nil {
		return apitypes.Depot{}, err
	}
	resp, err := c.HTTPClient(gizclaw.ServiceAdmin).Do(req)
	if err != nil {
		return apitypes.Depot{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return apitypes.Depot{}, err
	}
	if resp.StatusCode == http.StatusOK {
		var out apitypes.Depot
		if err := json.Unmarshal(body, &out); err != nil {
			return apitypes.Depot{}, err
		}
		return out, nil
	}
	return apitypes.Depot{}, responseError(resp.StatusCode, body)
}

type pagedItems[T any] struct {
	HasNext    bool
	Items      []T
	NextCursor *string
}

func collectAllPages[T any](
	fetchPage func(cursor *adminservice.Cursor, limit *adminservice.Limit) (pagedItems[T], error),
) ([]T, error) {
	limit := adminservice.Limit(200)
	var cursor *adminservice.Cursor
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
		next := adminservice.Cursor(*page.NextCursor)
		cursor = &next
	}
}

func ListGears(ctx context.Context, c *gizclaw.Client) ([]apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *adminservice.Cursor, limit *adminservice.Limit) (pagedItems[apitypes.Registration], error) {
		resp, err := api.ListGearsWithResponse(ctx, &adminservice.ListGearsParams{
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

func GetGear(ctx context.Context, c *gizclaw.Client, publicKey string) (apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Registration{}, err
	}
	resp, err := api.GetGearWithResponse(ctx, publicKey)
	if err != nil {
		return apitypes.Registration{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Registration{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func ResolveGearBySN(ctx context.Context, c *gizclaw.Client, sn string) (string, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return "", err
	}
	resp, err := api.ResolveBySNWithResponse(ctx, sn)
	if err != nil {
		return "", err
	}
	if resp.JSON200 != nil {
		return resp.JSON200.PublicKey, nil
	}
	return "", responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func ResolveGearByIMEI(ctx context.Context, c *gizclaw.Client, tac, serial string) (string, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return "", err
	}
	resp, err := api.ResolveByIMEIWithResponse(ctx, tac, serial)
	if err != nil {
		return "", err
	}
	if resp.JSON200 != nil {
		return resp.JSON200.PublicKey, nil
	}
	return "", responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func ApproveGear(ctx context.Context, c *gizclaw.Client, publicKey string, role apitypes.GearRole) (apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Registration{}, err
	}
	resp, err := api.ApproveGearWithResponse(ctx, publicKey, adminservice.ApproveRequest{Role: role})
	if err != nil {
		return apitypes.Registration{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Registration{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400)
}

func BlockGear(ctx context.Context, c *gizclaw.Client, publicKey string) (apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Registration{}, err
	}
	resp, err := api.BlockGearWithResponse(ctx, publicKey)
	if err != nil {
		return apitypes.Registration{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Registration{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func GetGearInfo(ctx context.Context, c *gizclaw.Client, publicKey string) (apitypes.DeviceInfo, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	resp, err := api.GetGearInfoWithResponse(ctx, publicKey)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.DeviceInfo{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func GetGearConfig(ctx context.Context, c *gizclaw.Client, publicKey string) (apitypes.Configuration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Configuration{}, err
	}
	resp, err := api.GetGearConfigWithResponse(ctx, publicKey)
	if err != nil {
		return apitypes.Configuration{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Configuration{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func PutGearConfig(ctx context.Context, c *gizclaw.Client, publicKey string, cfg apitypes.Configuration) (apitypes.Configuration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Configuration{}, err
	}
	resp, err := api.PutGearConfigWithResponse(ctx, publicKey, cfg)
	if err != nil {
		return apitypes.Configuration{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Configuration{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404)
}

func GetGearRuntime(ctx context.Context, c *gizclaw.Client, publicKey string) (apitypes.Runtime, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Runtime{}, err
	}
	resp, err := api.GetGearRuntimeWithResponse(ctx, publicKey)
	if err != nil {
		return apitypes.Runtime{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Runtime{}, responseError(resp.StatusCode(), resp.Body)
}

func GetGearOTA(ctx context.Context, c *gizclaw.Client, publicKey string) (apitypes.OTASummary, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.OTASummary{}, err
	}
	resp, err := api.GetGearOTAWithResponse(ctx, publicKey)
	if err != nil {
		return apitypes.OTASummary{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.OTASummary{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func ListGearsByLabel(ctx context.Context, c *gizclaw.Client, key, value string) ([]apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *adminservice.Cursor, limit *adminservice.Limit) (pagedItems[apitypes.Registration], error) {
		resp, err := api.ListByLabelWithResponse(ctx, key, value, &adminservice.ListByLabelParams{
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

func ListGearsByCertification(ctx context.Context, c *gizclaw.Client, pType apitypes.GearCertificationType, authority apitypes.GearCertificationAuthority, id string) ([]apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *adminservice.Cursor, limit *adminservice.Limit) (pagedItems[apitypes.Registration], error) {
		resp, err := api.ListByCertificationWithResponse(ctx, pType, authority, id, &adminservice.ListByCertificationParams{
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

func ListGearsByFirmware(ctx context.Context, c *gizclaw.Client, depot string, channel apitypes.GearFirmwareChannel) ([]apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *adminservice.Cursor, limit *adminservice.Limit) (pagedItems[apitypes.Registration], error) {
		resp, err := api.ListByFirmwareWithResponse(ctx, depot, channel, &adminservice.ListByFirmwareParams{
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

func DeleteGear(ctx context.Context, c *gizclaw.Client, publicKey string) (apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Registration{}, err
	}
	resp, err := api.DeleteGearWithResponse(ctx, publicKey)
	if err != nil {
		return apitypes.Registration{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Registration{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func RefreshGear(ctx context.Context, c *gizclaw.Client, publicKey string) (adminservice.RefreshResult, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return adminservice.RefreshResult{}, err
	}
	resp, err := api.RefreshGearWithResponse(ctx, publicKey)
	if err != nil {
		return adminservice.RefreshResult{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return adminservice.RefreshResult{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON409, resp.JSON502)
}

func ListCredentials(ctx context.Context, c *gizclaw.Client, provider string) ([]apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	var providerFilter *adminservice.CredentialProvider
	if provider != "" {
		value := adminservice.CredentialProvider(provider)
		providerFilter = &value
	}
	return collectAllPages(func(cursor *adminservice.Cursor, limit *adminservice.Limit) (pagedItems[apitypes.Credential], error) {
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

func CreateCredential(ctx context.Context, c *gizclaw.Client, req adminservice.CredentialUpsert) (apitypes.Credential, error) {
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

func GetCredential(ctx context.Context, c *gizclaw.Client, name string) (apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Credential{}, err
	}
	resp, err := api.GetCredentialWithResponse(ctx, adminservice.CredentialName(name))
	if err != nil {
		return apitypes.Credential{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Credential{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutCredential(ctx context.Context, c *gizclaw.Client, name string, req adminservice.CredentialUpsert) (apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Credential{}, err
	}
	resp, err := api.PutCredentialWithResponse(ctx, adminservice.CredentialName(name), req)
	if err != nil {
		return apitypes.Credential{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Credential{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteCredential(ctx context.Context, c *gizclaw.Client, name string) (apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Credential{}, err
	}
	resp, err := api.DeleteCredentialWithResponse(ctx, adminservice.CredentialName(name))
	if err != nil {
		return apitypes.Credential{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Credential{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListMiniMaxTenants(ctx context.Context, c *gizclaw.Client) ([]apitypes.MiniMaxTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *adminservice.Cursor, limit *adminservice.Limit) (pagedItems[apitypes.MiniMaxTenant], error) {
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

func CreateMiniMaxTenant(ctx context.Context, c *gizclaw.Client, req adminservice.MiniMaxTenantUpsert) (apitypes.MiniMaxTenant, error) {
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

func GetMiniMaxTenant(ctx context.Context, c *gizclaw.Client, name string) (apitypes.MiniMaxTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	resp, err := api.GetMiniMaxTenantWithResponse(ctx, adminservice.MiniMaxTenantName(name))
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.MiniMaxTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutMiniMaxTenant(ctx context.Context, c *gizclaw.Client, name string, req adminservice.MiniMaxTenantUpsert) (apitypes.MiniMaxTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	resp, err := api.PutMiniMaxTenantWithResponse(ctx, adminservice.MiniMaxTenantName(name), req)
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.MiniMaxTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteMiniMaxTenant(ctx context.Context, c *gizclaw.Client, name string) (apitypes.MiniMaxTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	resp, err := api.DeleteMiniMaxTenantWithResponse(ctx, adminservice.MiniMaxTenantName(name))
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.MiniMaxTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func SyncMiniMaxTenantVoices(ctx context.Context, c *gizclaw.Client, name string) (adminservice.MiniMaxSyncVoicesResult, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return adminservice.MiniMaxSyncVoicesResult{}, err
	}
	resp, err := api.SyncMiniMaxTenantVoicesWithResponse(ctx, adminservice.MiniMaxTenantName(name))
	if err != nil {
		return adminservice.MiniMaxSyncVoicesResult{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return adminservice.MiniMaxSyncVoicesResult{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON500, resp.JSON502)
}

func ListVolcTenants(ctx context.Context, c *gizclaw.Client) ([]apitypes.VolcTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *adminservice.Cursor, limit *adminservice.Limit) (pagedItems[apitypes.VolcTenant], error) {
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

func GetVolcTenant(ctx context.Context, c *gizclaw.Client, name string) (apitypes.VolcTenant, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.VolcTenant{}, err
	}
	resp, err := api.GetVolcTenantWithResponse(ctx, adminservice.VolcTenantName(name))
	if err != nil {
		return apitypes.VolcTenant{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.VolcTenant{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func SyncVolcTenantVoices(ctx context.Context, c *gizclaw.Client, name string) (adminservice.VolcSyncVoicesResult, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return adminservice.VolcSyncVoicesResult{}, err
	}
	resp, err := api.SyncVolcTenantVoicesWithResponse(ctx, adminservice.VolcTenantName(name))
	if err != nil {
		return adminservice.VolcSyncVoicesResult{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return adminservice.VolcSyncVoicesResult{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON500, resp.JSON502)
}

func ListVoices(ctx context.Context, c *gizclaw.Client, source, providerKind, providerName string) ([]apitypes.Voice, error) {
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
	var providerNameFilter *adminservice.VoiceProviderName
	if providerName != "" {
		value := adminservice.VoiceProviderName(providerName)
		providerNameFilter = &value
	}
	return collectAllPages(func(cursor *adminservice.Cursor, limit *adminservice.Limit) (pagedItems[apitypes.Voice], error) {
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

func CreateVoice(ctx context.Context, c *gizclaw.Client, req adminservice.VoiceUpsert) (apitypes.Voice, error) {
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

func GetVoice(ctx context.Context, c *gizclaw.Client, id string) (apitypes.Voice, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Voice{}, err
	}
	resp, err := api.GetVoiceWithResponse(ctx, adminservice.VoiceID(id))
	if err != nil {
		return apitypes.Voice{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Voice{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutVoice(ctx context.Context, c *gizclaw.Client, id string, req adminservice.VoiceUpsert) (apitypes.Voice, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Voice{}, err
	}
	resp, err := api.PutVoiceWithResponse(ctx, adminservice.VoiceID(id), req)
	if err != nil {
		return apitypes.Voice{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Voice{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func DeleteVoice(ctx context.Context, c *gizclaw.Client, id string) (apitypes.Voice, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Voice{}, err
	}
	resp, err := api.DeleteVoiceWithResponse(ctx, adminservice.VoiceID(id))
	if err != nil {
		return apitypes.Voice{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Voice{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListWorkspaceTemplates(ctx context.Context, c *gizclaw.Client) ([]apitypes.WorkflowTemplateDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *adminservice.Cursor, limit *adminservice.Limit) (pagedItems[apitypes.WorkflowTemplateDocument], error) {
		resp, err := api.ListWorkspaceTemplatesWithResponse(ctx, &adminservice.ListWorkspaceTemplatesParams{
			Cursor: cursor,
			Limit:  limit,
		})
		if err != nil {
			return pagedItems[apitypes.WorkflowTemplateDocument]{}, err
		}
		if resp.JSON200 == nil {
			return pagedItems[apitypes.WorkflowTemplateDocument]{}, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		return pagedItems[apitypes.WorkflowTemplateDocument]{
			HasNext:    resp.JSON200.HasNext,
			Items:      resp.JSON200.Items,
			NextCursor: resp.JSON200.NextCursor,
		}, nil
	})
}

func CreateWorkspaceTemplate(ctx context.Context, c *gizclaw.Client, req apitypes.WorkflowTemplateDocument) (apitypes.WorkflowTemplateDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.WorkflowTemplateDocument{}, err
	}
	resp, err := api.CreateWorkspaceTemplateWithResponse(ctx, req)
	if err != nil {
		return apitypes.WorkflowTemplateDocument{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.WorkflowTemplateDocument{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func GetWorkspaceTemplate(ctx context.Context, c *gizclaw.Client, name string) (apitypes.WorkflowTemplateDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.WorkflowTemplateDocument{}, err
	}
	resp, err := api.GetWorkspaceTemplateWithResponse(ctx, adminservice.WorkspaceTemplateName(name))
	if err != nil {
		return apitypes.WorkflowTemplateDocument{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.WorkflowTemplateDocument{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutWorkspaceTemplate(ctx context.Context, c *gizclaw.Client, name string, req apitypes.WorkflowTemplateDocument) (apitypes.WorkflowTemplateDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.WorkflowTemplateDocument{}, err
	}
	resp, err := api.PutWorkspaceTemplateWithResponse(ctx, adminservice.WorkspaceTemplateName(name), req)
	if err != nil {
		return apitypes.WorkflowTemplateDocument{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.WorkflowTemplateDocument{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteWorkspaceTemplate(ctx context.Context, c *gizclaw.Client, name string) (apitypes.WorkflowTemplateDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.WorkflowTemplateDocument{}, err
	}
	resp, err := api.DeleteWorkspaceTemplateWithResponse(ctx, adminservice.WorkspaceTemplateName(name))
	if err != nil {
		return apitypes.WorkflowTemplateDocument{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.WorkflowTemplateDocument{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func ListWorkspaces(ctx context.Context, c *gizclaw.Client) ([]apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	return collectAllPages(func(cursor *adminservice.Cursor, limit *adminservice.Limit) (pagedItems[apitypes.Workspace], error) {
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

func CreateWorkspace(ctx context.Context, c *gizclaw.Client, req adminservice.WorkspaceUpsert) (apitypes.Workspace, error) {
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

func GetWorkspace(ctx context.Context, c *gizclaw.Client, name string) (apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	resp, err := api.GetWorkspaceWithResponse(ctx, adminservice.WorkspaceName(name))
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workspace{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func PutWorkspace(ctx context.Context, c *gizclaw.Client, name string, req adminservice.WorkspaceUpsert) (apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	resp, err := api.PutWorkspaceWithResponse(ctx, adminservice.WorkspaceName(name), req)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workspace{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func DeleteWorkspace(ctx context.Context, c *gizclaw.Client, name string) (apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	resp, err := api.DeleteWorkspaceWithResponse(ctx, adminservice.WorkspaceName(name))
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
