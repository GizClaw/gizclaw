package gizclaw_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const (
	testReadyTimeout = 10 * time.Second
	testProbeTimeout = time.Second
	testPollInterval = 20 * time.Millisecond
)

type testServer struct {
	server     *gizclaw.Server
	addr       string
	cipherMode gizwebrtc.CipherMode
	errCh      chan error
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
	return startTestServerWithCipherMode(t, "")
}

func startTestServerWithCipherMode(t *testing.T, cipherMode gizwebrtc.CipherMode) *testServer {
	t.Helper()

	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error: %v", err)
	}

	srv := &gizclaw.Server{
		LocalStatic: *keyPair,
		PeerStore:   mustBadgerInMemory(t, nil),
	}
	var signalingServer *httptest.Server
	srv.PeerListenerFactories = []gizclaw.PeerListenerFactory{
		func(opts gizclaw.PeerListenerOptions) (giznet.Listener, error) {
			l, err := (&gizwebrtc.ListenConfig{
				CipherMode:       cipherMode,
				SecurityPolicy:   opts.SecurityPolicy,
				PeerEventHandler: opts.PeerEventHandler,
			}).Listen(opts.KeyPair)
			if err != nil {
				return nil, err
			}
			srv.WebRTCSignalingHandler = l.SignalingHandler()
			signalingServer = httptest.NewServer(l.SignalingHandler())
			return l, nil
		},
	}

	ts := &testServer{
		server:     srv,
		cipherMode: cipherMode,
		errCh:      make(chan error, 1),
	}
	if err := srv.Listen(); err != nil {
		t.Fatalf("test server listen: %v", err)
	}
	if signalingServer == nil {
		t.Fatal("test server did not create WebRTC signaling server")
	}
	ts.addr = signalingServer.URL + gizwebrtc.SignalingPath
	go func() {
		ts.errCh <- srv.Serve()
	}()

	if err := waitForServerReady(ts.addr, srv.PublicKey(), ts.cipherMode, ts.errCh); err != nil {
		_ = srv.Close()
		select {
		case <-ts.errCh:
		case <-time.After(500 * time.Millisecond):
		}
		t.Fatalf("test server did not become ready: %v", err)
	}

	t.Cleanup(func() { _ = ts.server.Close() })
	t.Cleanup(signalingServer.Close)
	return ts
}

func newTestClient(t *testing.T, ts *testServer) *gizcli.Client {
	return newTestClientWithDevice(t, ts, apitypes.DeviceInfo{})
}

func newTestClientWithDevice(t *testing.T, ts *testServer, device apitypes.DeviceInfo) *gizcli.Client {
	t.Helper()

	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error: %v", err)
	}

	client := &gizcli.Client{
		KeyPair:       keyPair,
		DialTransport: testWebRTCDialTransport(ts.cipherMode),
		Device:        device,
	}
	startTestClient(t, client, ts.server.PublicKey(), ts.addr)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func waitForServerReady(addr string, pk giznet.PublicKey, cipherMode gizwebrtc.CipherMode, errCh <-chan error) error {
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

		client := &gizcli.Client{KeyPair: keyPair, DialTransport: testWebRTCDialTransport(cipherMode)}
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

			if err := probePeerHTTPReady(client); err == nil {
				_ = client.Close()
				return nil
			}
			time.Sleep(50 * time.Millisecond)
		}

		_ = client.Close()
		return fmt.Errorf("probe server public ready: not ready")
	})
}

func testWebRTCDialTransport(cipherMode gizwebrtc.CipherMode) gizcli.DialTransportFunc {
	return func(key *giznet.KeyPair, serverPK giznet.PublicKey, serverAddr string, securityPolicy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		listener, conn, err := gizwebrtc.Dial(ctx, key, serverPK, gizwebrtc.DialConfig{
			SignalingURL:   serverAddr,
			CipherMode:     cipherMode,
			SecurityPolicy: securityPolicy,
		})
		if err != nil {
			return nil, nil, err
		}
		return listener, conn, nil
	}
}

func startTestClient(t *testing.T, c *gizcli.Client, serverPK giznet.PublicKey, addr string) {
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
		return probePeerHTTPReady(c)
	}); err != nil {
		t.Fatalf("test client did not become ready: %v", err)
	}
}

func probePeerHTTPReady(c *gizcli.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), testProbeTimeout)
	defer cancel()
	_, err := getServerInfo(ctx, c)
	return err
}

func ensureAdminPeer(t testing.TB, ts *testServer, c *gizcli.Client, info apitypes.DeviceInfo) string {
	t.Helper()
	publicKey := c.KeyPair.Public
	if err := waitUntil(testReadyTimeout, func() error {
		peer, err := ts.server.Manager().Peers.LoadPeer(context.Background(), publicKey)
		if err != nil {
			return err
		}
		peer.Role = apitypes.PeerRoleAdmin
		peer.Status = apitypes.PeerRegistrationStatusActive
		peer.Device = info
		if _, err := ts.server.Manager().Peers.SavePeer(context.Background(), peer); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("ensure admin peer: %v", err)
	}
	return publicKey.String()
}

func ensurePeerInfo(t testing.TB, c *gizcli.Client, info apitypes.DeviceInfo) string {
	t.Helper()
	if _, err := putInfo(context.Background(), c, info); err != nil {
		t.Fatalf("PutInfo error: %v", err)
	}
	return c.KeyPair.Public.String()
}

func getServerInfo(ctx context.Context, c *gizcli.Client) (apitypes.ServerInfo, error) {
	api, err := c.PeerHTTPClient()
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

func getInfo(ctx context.Context, c *gizcli.Client) (apitypes.DeviceInfo, error) {
	resp, err := c.GetServerInfo(ctx, "server.info.get")
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	return convertIntegrationAPIType[apitypes.DeviceInfo](*resp)
}

func putInfo(ctx context.Context, c *gizcli.Client, info apitypes.DeviceInfo) (apitypes.DeviceInfo, error) {
	rpcReq, err := convertIntegrationAPIType[rpcapi.ServerPutInfoRequest](info)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	resp, err := c.PutServerInfo(ctx, "server.info.put", rpcReq)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	return convertIntegrationAPIType[apitypes.DeviceInfo](*resp)
}

func getRuntime(ctx context.Context, c *gizcli.Client) (apitypes.Runtime, error) {
	resp, err := c.GetServerRuntime(ctx, "server.runtime.get")
	if err != nil {
		return apitypes.Runtime{}, err
	}
	return convertIntegrationAPIType[apitypes.Runtime](*resp)
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

func listWorkflows(ctx context.Context, c *gizcli.Client) ([]apitypes.Workflow, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	limit := int32(200)
	var cursor *string
	items := make([]apitypes.Workflow, 0)
	for {
		resp, err := api.ListWorkflowsWithResponse(ctx, &adminhttp.ListWorkflowsParams{
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
		next := string(*resp.JSON200.NextCursor)
		cursor = &next
	}
}

func createWorkflow(ctx context.Context, c *gizcli.Client, doc apitypes.Workflow) (apitypes.Workflow, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workflow{}, err
	}
	resp, err := api.CreateWorkflowWithResponse(ctx, doc)
	if err != nil {
		return apitypes.Workflow{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workflow{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500)
}

func getWorkflow(ctx context.Context, c *gizcli.Client, name string) (apitypes.Workflow, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workflow{}, err
	}
	resp, err := api.GetWorkflowWithResponse(ctx, name)
	if err != nil {
		return apitypes.Workflow{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workflow{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func putWorkflow(ctx context.Context, c *gizcli.Client, name string, doc apitypes.Workflow) (apitypes.Workflow, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workflow{}, err
	}
	resp, err := api.PutWorkflowWithResponse(ctx, name, doc)
	if err != nil {
		return apitypes.Workflow{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workflow{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON500)
}

func deleteWorkflow(ctx context.Context, c *gizcli.Client, name string) (apitypes.Workflow, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return apitypes.Workflow{}, err
	}
	resp, err := api.DeleteWorkflowWithResponse(ctx, name)
	if err != nil {
		return apitypes.Workflow{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Workflow{}, responseError(resp.StatusCode(), resp.Body, resp.JSON404, resp.JSON500)
}

func listWorkspaces(ctx context.Context, c *gizcli.Client) ([]apitypes.Workspace, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	limit := int32(200)
	var cursor *string
	items := make([]apitypes.Workspace, 0)
	for {
		resp, err := api.ListWorkspacesWithResponse(ctx, &adminhttp.ListWorkspacesParams{
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
		next := string(*resp.JSON200.NextCursor)
		cursor = &next
	}
}

func createWorkspace(ctx context.Context, c *gizcli.Client, body adminhttp.WorkspaceUpsert) (apitypes.Workspace, error) {
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

func getWorkspace(ctx context.Context, c *gizcli.Client, name string) (apitypes.Workspace, error) {
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

func putWorkspace(ctx context.Context, c *gizcli.Client, name string, body adminhttp.WorkspaceUpsert) (apitypes.Workspace, error) {
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

func deleteWorkspace(ctx context.Context, c *gizcli.Client, name string) (apitypes.Workspace, error) {
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

func listCredentials(ctx context.Context, c *gizcli.Client, provider *string) ([]apitypes.Credential, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	limit := int32(200)
	var cursor *string
	items := make([]apitypes.Credential, 0)
	for {
		resp, err := api.ListCredentialsWithResponse(ctx, &adminhttp.ListCredentialsParams{
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
		next := string(*resp.JSON200.NextCursor)
		cursor = &next
	}
}

func createCredential(ctx context.Context, c *gizcli.Client, body adminhttp.CredentialUpsert) (apitypes.Credential, error) {
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

func getCredential(ctx context.Context, c *gizcli.Client, name string) (apitypes.Credential, error) {
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

func putCredential(ctx context.Context, c *gizcli.Client, name string, body adminhttp.CredentialUpsert) (apitypes.Credential, error) {
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

func deleteCredential(ctx context.Context, c *gizcli.Client, name string) (apitypes.Credential, error) {
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

func listPeers(ctx context.Context, c *gizcli.Client) ([]apitypes.Registration, error) {
	api, err := c.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	limit := int32(200)
	var cursor *string
	items := make([]apitypes.Registration, 0)
	for {
		resp, err := api.ListPeersWithResponse(ctx, &adminhttp.ListPeersParams{
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
		next := string(*resp.JSON200.NextCursor)
		cursor = &next
	}
}

func getPeer(ctx context.Context, c *gizcli.Client, publicKey string) (apitypes.Registration, error) {
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

func findPubKeyBySN(ctx context.Context, c *gizcli.Client, sn string) (string, error) {
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

func findPubKeyByIMEI(ctx context.Context, c *gizcli.Client, tac, serial string) (string, error) {
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

func approvePeer(ctx context.Context, c *gizcli.Client, publicKey string, role apitypes.PeerRole) (apitypes.Registration, error) {
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

func blockPeer(ctx context.Context, c *gizcli.Client, publicKey string) (apitypes.Registration, error) {
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

func getPeerInfo(ctx context.Context, c *gizcli.Client, publicKey string) (apitypes.DeviceInfo, error) {
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

func getPeerConfig(ctx context.Context, c *gizcli.Client, publicKey string) (apitypes.Configuration, error) {
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

func putPeerConfig(ctx context.Context, c *gizcli.Client, publicKey string, cfg apitypes.Configuration) (apitypes.Configuration, error) {
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

func getPeerRuntime(ctx context.Context, c *gizcli.Client, publicKey string) (apitypes.Runtime, error) {
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

func deletePeer(ctx context.Context, c *gizcli.Client, publicKey string) (apitypes.Registration, error) {
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

func refreshPeer(ctx context.Context, c *gizcli.Client, publicKey string) (adminhttp.RefreshResult, error) {
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
