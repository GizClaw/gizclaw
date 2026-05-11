package gizclaw_test

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/gearservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/store/depotstore"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

const (
	testReadyTimeout = 10 * time.Second
	testProbeTimeout = time.Second
	testPollInterval = 20 * time.Millisecond
)

type testServer struct {
	server *gizclaw.Server
	addr   string
	errCh  chan error
}

func allocateUDPAddr(t testing.TB) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate UDP addr: %v", err)
	}
	addr := pc.LocalAddr().(*net.UDPAddr)
	_ = pc.Close()
	return fmt.Sprintf("127.0.0.1:%d", addr.Port)
}

func waitUntil(timeout time.Duration, check func() error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := check(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(testPollInterval)
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("condition not satisfied before timeout")
}

func mustBadgerInMemory(t testing.TB, opts *kv.Options) kv.Store {
	t.Helper()
	store, err := kv.NewBadgerInMemory(opts)
	if err != nil {
		t.Fatalf("NewBadgerInMemory: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func startTestServer(t *testing.T) *testServer {
	t.Helper()

	root := t.TempDir()
	firmwareRoot := filepath.Join(root, "firmware")
	if err := os.MkdirAll(firmwareRoot, 0o755); err != nil {
		t.Fatalf("mkdir firmware root: %v", err)
	}

	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error: %v", err)
	}

	srv := &gizclaw.Server{
		KeyPair:    keyPair,
		ListenAddr: allocateUDPAddr(t),
		PeerStore:  mustBadgerInMemory(t, nil),
		DepotStore: depotstore.Dir(firmwareRoot),
	}

	ts := &testServer{
		server: srv,
		addr:   srv.ListenAddr,
		errCh:  make(chan error, 1),
	}
	if err := srv.Listen(); err != nil {
		t.Fatalf("test server listen: %v", err)
	}
	go func() {
		ts.errCh <- srv.Serve()
	}()

	if err := waitForServerReady(ts.addr, srv.PublicKey(), ts.errCh); err != nil {
		_ = srv.Close()
		select {
		case <-ts.errCh:
		case <-time.After(500 * time.Millisecond):
		}
		t.Fatalf("test server did not become ready: %v", err)
	}

	t.Cleanup(func() { _ = ts.server.Close() })
	return ts
}

func newTestClient(t *testing.T, ts *testServer) *gizclaw.Client {
	t.Helper()

	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error: %v", err)
	}

	client := &gizclaw.Client{KeyPair: keyPair}
	startTestClient(t, client, ts.server.PublicKey(), ts.addr)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func waitForServerReady(addr string, pk giznet.PublicKey, errCh <-chan error) error {
	return waitUntil(testReadyTimeout, func() error {
		select {
		case err := <-errCh:
			return fmt.Errorf("test server exited before ready: %w", err)
		default:
		}

		keyPair, err := giznet.GenerateKeyPair()
		if err != nil {
			return fmt.Errorf("GenerateKeyPair(ready check): %w", err)
		}

		client := &gizclaw.Client{KeyPair: keyPair}
		if err := client.Dial(pk, addr); err != nil {
			_ = client.Close()
			return fmt.Errorf("dial ready check: %w", err)
		}
		dialErrCh := make(chan error, 1)
		go func() {
			dialErrCh <- client.Serve()
		}()

		for range 20 {
			select {
			case err := <-dialErrCh:
				_ = client.Close()
				if err != nil {
					return fmt.Errorf("dial ready check: %w", err)
				}
				return fmt.Errorf("dial ready check: client stopped before ready")
			default:
			}

			if err := probeServerPublicReady(client); err == nil {
				_ = client.Close()
				return nil
			}
			time.Sleep(50 * time.Millisecond)
		}

		_ = client.Close()
		return fmt.Errorf("probe server public ready: not ready")
	})
}

func startTestClient(t *testing.T, c *gizclaw.Client, serverPK giznet.PublicKey, addr string) {
	t.Helper()

	if err := c.Dial(serverPK, addr); err != nil {
		t.Fatalf("test client dial: %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.Serve()
	}()

	if err := waitUntil(testReadyTimeout, func() error {
		select {
		case err := <-errCh:
			if err != nil {
				return err
			}
			return fmt.Errorf("client stopped before ready")
		default:
		}
		return probeServerPublicReady(c)
	}); err != nil {
		t.Fatalf("test client did not become ready: %v", err)
	}
}

func probeServerPublicReady(c *gizclaw.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), testProbeTimeout)
	defer cancel()
	_, err := getServerInfo(ctx, c)
	return err
}

func register(ctx context.Context, c *gizclaw.Client, req gearservice.RegistrationRequest) (gearservice.RegistrationResult, error) {
	rpcReq, err := convertIntegrationAPIType[rpcapi.GearRegisterRequest](req)
	if err != nil {
		return gearservice.RegistrationResult{}, err
	}
	resp, err := c.RegisterGear(ctx, "gear.registration.register", rpcReq)
	if err != nil {
		return gearservice.RegistrationResult{}, err
	}
	return convertIntegrationAPIType[gearservice.RegistrationResult](*resp)
}

func getServerInfo(ctx context.Context, c *gizclaw.Client) (apitypes.ServerInfo, error) {
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

func getInfo(ctx context.Context, c *gizclaw.Client) (apitypes.DeviceInfo, error) {
	resp, err := c.GetGearInfo(ctx, "gear.info.get")
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	return convertIntegrationAPIType[apitypes.DeviceInfo](*resp)
}

func putInfo(ctx context.Context, c *gizclaw.Client, info apitypes.DeviceInfo) (apitypes.DeviceInfo, error) {
	rpcReq, err := convertIntegrationAPIType[rpcapi.GearPutInfoRequest](info)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	resp, err := c.PutGearInfo(ctx, "gear.info.put", rpcReq)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	return convertIntegrationAPIType[apitypes.DeviceInfo](*resp)
}

func getRuntime(ctx context.Context, c *gizclaw.Client) (apitypes.Runtime, error) {
	resp, err := c.GetGearRuntime(ctx, "gear.runtime.get")
	if err != nil {
		return apitypes.Runtime{}, err
	}
	return convertIntegrationAPIType[apitypes.Runtime](*resp)
}

func getRegistration(ctx context.Context, c *gizclaw.Client) (apitypes.Registration, error) {
	resp, err := c.GetGearRegistration(ctx, "gear.registration.get")
	if err != nil {
		return apitypes.Registration{}, err
	}
	return convertIntegrationAPIType[apitypes.Registration](*resp)
}

func getConfig(ctx context.Context, c *gizclaw.Client) (apitypes.Configuration, error) {
	resp, err := c.GetGearConfig(ctx, "gear.config.get")
	if err != nil {
		return apitypes.Configuration{}, err
	}
	cfg, err := convertIntegrationAPIType[apitypes.Configuration](*resp)
	if err != nil {
		return apitypes.Configuration{}, err
	}
	if cfg.Firmware == nil {
		cfg.Firmware = &apitypes.FirmwareConfig{}
	}
	return cfg, nil
}

func getOTA(ctx context.Context, c *gizclaw.Client) (apitypes.OTASummary, error) {
	resp, err := c.GetGearOTA(ctx, "gear.ota.get")
	if err != nil {
		return apitypes.OTASummary{}, err
	}
	return convertIntegrationAPIType[apitypes.OTASummary](*resp)
}

func downloadFirmware(ctx context.Context, c *gizclaw.Client, path string) ([]byte, http.Header, error) {
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

func convertIntegrationAPIType[T any](value any) (T, error) {
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

func listFirmwares(ctx context.Context, c *gizclaw.Client) ([]apitypes.Depot, error) {
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

func getFirmwareDepot(ctx context.Context, c *gizclaw.Client, depot string) (apitypes.Depot, error) {
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

func getFirmwareChannel(ctx context.Context, c *gizclaw.Client, depot string, channel adminservice.Channel) (apitypes.DepotRelease, error) {
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

func putFirmwareInfo(ctx context.Context, c *gizclaw.Client, depot string, info apitypes.DepotInfo) (apitypes.Depot, error) {
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

func uploadFirmware(ctx context.Context, c *gizclaw.Client, depot string, channel adminservice.Channel, data []byte) (apitypes.DepotRelease, error) {
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

func releaseFirmware(ctx context.Context, c *gizclaw.Client, depot string) (apitypes.Depot, error) {
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

func rollbackFirmware(ctx context.Context, c *gizclaw.Client, depot string) (apitypes.Depot, error) {
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

func listWorkflows(ctx context.Context, c *gizclaw.Client) ([]apitypes.WorkflowDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	limit := adminservice.Limit(200)
	var cursor *adminservice.Cursor
	items := make([]apitypes.WorkflowDocument, 0)
	for {
		resp, err := api.ListWorkflowsWithResponse(ctx, &adminservice.ListWorkflowsParams{
			Cursor: cursor,
			Limit:  &limit,
		})
		if err != nil {
			return nil, err
		}
		if resp.JSON200 == nil {
			return nil, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		items = append(items, resp.JSON200.Items...)
		if !resp.JSON200.HasNext || resp.JSON200.NextCursor == nil || *resp.JSON200.NextCursor == "" {
			return items, nil
		}
		next := adminservice.Cursor(*resp.JSON200.NextCursor)
		cursor = &next
	}
}

func createWorkflow(ctx context.Context, c *gizclaw.Client, doc apitypes.WorkflowDocument) (apitypes.WorkflowDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	resp, err := api.CreateWorkflowWithResponse(ctx, doc)
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.WorkflowDocument{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func getWorkflow(ctx context.Context, c *gizclaw.Client, name string) (apitypes.WorkflowDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	resp, err := api.GetWorkflowWithResponse(ctx, name)
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.WorkflowDocument{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func putWorkflow(ctx context.Context, c *gizclaw.Client, name string, doc apitypes.WorkflowDocument) (apitypes.WorkflowDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	resp, err := api.PutWorkflowWithResponse(ctx, name, doc)
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.WorkflowDocument{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func deleteWorkflow(ctx context.Context, c *gizclaw.Client, name string) (apitypes.WorkflowDocument, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	resp, err := api.DeleteWorkflowWithResponse(ctx, name)
	if err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.WorkflowDocument{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func listWorkspaces(ctx context.Context, c *gizclaw.Client) ([]apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	limit := adminservice.Limit(200)
	var cursor *adminservice.Cursor
	items := make([]apitypes.Workspace, 0)
	for {
		resp, err := api.ListWorkspacesWithResponse(ctx, &adminservice.ListWorkspacesParams{
			Cursor: cursor,
			Limit:  &limit,
		})
		if err != nil {
			return nil, err
		}
		if resp.JSON200 == nil {
			return nil, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		items = append(items, resp.JSON200.Items...)
		if !resp.JSON200.HasNext || resp.JSON200.NextCursor == nil || *resp.JSON200.NextCursor == "" {
			return items, nil
		}
		next := adminservice.Cursor(*resp.JSON200.NextCursor)
		cursor = &next
	}
}

func createWorkspace(ctx context.Context, c *gizclaw.Client, body adminservice.WorkspaceUpsert) (apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	resp, err := api.CreateWorkspaceWithResponse(ctx, body)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workspace{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func getWorkspace(ctx context.Context, c *gizclaw.Client, name string) (apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	resp, err := api.GetWorkspaceWithResponse(ctx, name)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workspace{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func putWorkspace(ctx context.Context, c *gizclaw.Client, name string, body adminservice.WorkspaceUpsert) (apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	resp, err := api.PutWorkspaceWithResponse(ctx, name, body)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workspace{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func deleteWorkspace(ctx context.Context, c *gizclaw.Client, name string) (apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	resp, err := api.DeleteWorkspaceWithResponse(ctx, name)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workspace{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func listCredentials(ctx context.Context, c *gizclaw.Client, provider *apitypes.CredentialProvider) ([]apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	limit := adminservice.Limit(200)
	var cursor *adminservice.Cursor
	items := make([]apitypes.Credential, 0)
	for {
		resp, err := api.ListCredentialsWithResponse(ctx, &adminservice.ListCredentialsParams{
			Provider: provider,
			Cursor:   cursor,
			Limit:    &limit,
		})
		if err != nil {
			return nil, err
		}
		if resp.JSON200 == nil {
			return nil, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		items = append(items, resp.JSON200.Items...)
		if !resp.JSON200.HasNext || resp.JSON200.NextCursor == nil || *resp.JSON200.NextCursor == "" {
			return items, nil
		}
		next := adminservice.Cursor(*resp.JSON200.NextCursor)
		cursor = &next
	}
}

func createCredential(ctx context.Context, c *gizclaw.Client, body adminservice.CredentialUpsert) (apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Credential{}, err
	}
	resp, err := api.CreateCredentialWithResponse(ctx, body)
	if err != nil {
		return apitypes.Credential{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Credential{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func getCredential(ctx context.Context, c *gizclaw.Client, name string) (apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Credential{}, err
	}
	resp, err := api.GetCredentialWithResponse(ctx, name)
	if err != nil {
		return apitypes.Credential{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Credential{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func putCredential(ctx context.Context, c *gizclaw.Client, name string, body adminservice.CredentialUpsert) (apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Credential{}, err
	}
	resp, err := api.PutCredentialWithResponse(ctx, name, body)
	if err != nil {
		return apitypes.Credential{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Credential{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func deleteCredential(ctx context.Context, c *gizclaw.Client, name string) (apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Credential{}, err
	}
	resp, err := api.DeleteCredentialWithResponse(ctx, name)
	if err != nil {
		return apitypes.Credential{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Credential{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func listPeers(ctx context.Context, c *gizclaw.Client) ([]apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	limit := adminservice.Limit(200)
	var cursor *adminservice.Cursor
	items := make([]apitypes.Registration, 0)
	for {
		resp, err := api.ListPeersWithResponse(ctx, &adminservice.ListPeersParams{
			Cursor: cursor,
			Limit:  &limit,
		})
		if err != nil {
			return nil, err
		}
		if resp.JSON200 == nil {
			return nil, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		items = append(items, resp.JSON200.Items...)
		if !resp.JSON200.HasNext || resp.JSON200.NextCursor == nil || *resp.JSON200.NextCursor == "" {
			return items, nil
		}
		next := adminservice.Cursor(*resp.JSON200.NextCursor)
		cursor = &next
	}
}

func getPeer(ctx context.Context, c *gizclaw.Client, publicKey string) (apitypes.Registration, error) {
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

func resolvePeerBySN(ctx context.Context, c *gizclaw.Client, sn string) (string, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return "", err
	}
	resp, err := api.ResolvePeerBySNWithResponse(ctx, sn)
	if err != nil {
		return "", err
	}
	if resp.JSON200 != nil {
		return resp.JSON200.PublicKey, nil
	}
	return "", responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func resolvePeerByIMEI(ctx context.Context, c *gizclaw.Client, tac, serial string) (string, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return "", err
	}
	resp, err := api.ResolvePeerByIMEIWithResponse(ctx, tac, serial)
	if err != nil {
		return "", err
	}
	if resp.JSON200 != nil {
		return resp.JSON200.PublicKey, nil
	}
	return "", responseError(resp.StatusCode(), resp.Body, resp.JSON404)
}

func approvePeer(ctx context.Context, c *gizclaw.Client, publicKey string, role apitypes.GearRole) (apitypes.Registration, error) {
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

func blockPeer(ctx context.Context, c *gizclaw.Client, publicKey string) (apitypes.Registration, error) {
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

func getPeerInfo(ctx context.Context, c *gizclaw.Client, publicKey string) (apitypes.DeviceInfo, error) {
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

func getPeerConfig(ctx context.Context, c *gizclaw.Client, publicKey string) (apitypes.Configuration, error) {
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

func putPeerConfig(ctx context.Context, c *gizclaw.Client, publicKey string, cfg apitypes.Configuration) (apitypes.Configuration, error) {
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

func getPeerRuntime(ctx context.Context, c *gizclaw.Client, publicKey string) (apitypes.Runtime, error) {
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

func listPeersByLabel(ctx context.Context, c *gizclaw.Client, key, value string) ([]apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	limit := adminservice.Limit(200)
	var cursor *adminservice.Cursor
	items := make([]apitypes.Registration, 0)
	for {
		resp, err := api.ListPeersByLabelWithResponse(ctx, key, value, &adminservice.ListPeersByLabelParams{
			Cursor: cursor,
			Limit:  &limit,
		})
		if err != nil {
			return nil, err
		}
		if resp.JSON200 == nil {
			return nil, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		items = append(items, resp.JSON200.Items...)
		if !resp.JSON200.HasNext || resp.JSON200.NextCursor == nil || *resp.JSON200.NextCursor == "" {
			return items, nil
		}
		next := adminservice.Cursor(*resp.JSON200.NextCursor)
		cursor = &next
	}
}

func listPeersByCertification(ctx context.Context, c *gizclaw.Client, pType apitypes.GearCertificationType, authority apitypes.GearCertificationAuthority, id string) ([]apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	limit := adminservice.Limit(200)
	var cursor *adminservice.Cursor
	items := make([]apitypes.Registration, 0)
	for {
		resp, err := api.ListPeersByCertificationWithResponse(ctx, pType, authority, id, &adminservice.ListPeersByCertificationParams{
			Cursor: cursor,
			Limit:  &limit,
		})
		if err != nil {
			return nil, err
		}
		if resp.JSON200 == nil {
			return nil, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		items = append(items, resp.JSON200.Items...)
		if !resp.JSON200.HasNext || resp.JSON200.NextCursor == nil || *resp.JSON200.NextCursor == "" {
			return items, nil
		}
		next := adminservice.Cursor(*resp.JSON200.NextCursor)
		cursor = &next
	}
}

func listPeersByFirmware(ctx context.Context, c *gizclaw.Client, depot string, channel apitypes.GearFirmwareChannel) ([]apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	limit := adminservice.Limit(200)
	var cursor *adminservice.Cursor
	items := make([]apitypes.Registration, 0)
	for {
		resp, err := api.ListPeersByFirmwareWithResponse(ctx, depot, channel, &adminservice.ListPeersByFirmwareParams{
			Cursor: cursor,
			Limit:  &limit,
		})
		if err != nil {
			return nil, err
		}
		if resp.JSON200 == nil {
			return nil, responseError(resp.StatusCode(), resp.Body, resp.JSON500)
		}
		items = append(items, resp.JSON200.Items...)
		if !resp.JSON200.HasNext || resp.JSON200.NextCursor == nil || *resp.JSON200.NextCursor == "" {
			return items, nil
		}
		next := adminservice.Cursor(*resp.JSON200.NextCursor)
		cursor = &next
	}
}

func deletePeer(ctx context.Context, c *gizclaw.Client, publicKey string) (apitypes.Registration, error) {
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

func refreshPeer(ctx context.Context, c *gizclaw.Client, publicKey string) (adminservice.RefreshResult, error) {
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

func buildReleaseTar(t *testing.T, release apitypes.DepotRelease, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	manifest, err := json.Marshal(release)
	if err != nil {
		t.Fatalf("marshal release: %v", err)
	}

	writeTarFile(t, tw, "manifest.json", manifest)
	for name, data := range files {
		writeTarFile(t, tw, name, data)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	return buf.Bytes()
}

func writeTarFile(t *testing.T, tw *tar.Writer, name string, data []byte) {
	t.Helper()

	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("tar write: %v", err)
	}
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

func strPtr(value string) *string {
	return &value
}
