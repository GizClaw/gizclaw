package peerscmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestPutConfigUsesFilePayload(t *testing.T) {
	original := openPeerConfigClient
	fake := &fakePeerConfigClient{}
	openPeerConfigClient = func(string) (peerConfigClient, error) {
		return fake, nil
	}
	defer func() { openPeerConfigClient = original }()

	file := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"view":"under-12"}`)
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := NewCmd()
	cmd.SetArgs([]string{"put-config", "device-pk", "--file", file})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if fake.putCfg.View == nil || *fake.putCfg.View != "under-12" {
		t.Fatalf("put config = %+v", fake.putCfg)
	}
}

func TestPeerCommandsReturnContextErrors(t *testing.T) {
	cases := [][]string{
		{"list"},
		{"get", "device-pk"},
		{"resolve-sn", "sn-001"},
		{"resolve-imei", "12345678", "000001"},
		{"approve", "device-pk", "client"},
		{"block", "device-pk"},
		{"info", "device-pk"},
		{"config", "device-pk"},
		{"runtime", "device-pk"},
		{"delete", "device-pk"},
		{"refresh", "device-pk"},
	}
	for _, args := range cases {
		t.Run(args[0], func(t *testing.T) {
			cmd := NewCmd()
			cmd.SetArgs(append(args, "--context", "__missing_context__"))
			if err := cmd.Execute(); err == nil {
				t.Fatal("Execute error = nil")
			}
		})
	}
}

func TestPeerCommandsUseClientOperations(t *testing.T) {
	restore := stubPeerCommandClients(t)
	defer restore()

	cases := [][]string{
		{"list"},
		{"get", "device-pk"},
		{"resolve-sn", "sn-001"},
		{"resolve-imei", "12345678", "000001"},
		{"approve", "device-pk", "client"},
		{"block", "device-pk"},
		{"info", "device-pk"},
		{"config", "device-pk"},
		{"runtime", "device-pk"},
		{"delete", "device-pk"},
		{"refresh", "device-pk"},
	}
	for _, args := range cases {
		t.Run(args[0], func(t *testing.T) {
			cmd := NewCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			if out.Len() == 0 {
				t.Fatal("command produced no output")
			}
		})
	}
}

func stubPeerCommandClients(t *testing.T) func() {
	t.Helper()
	originalConnect := connectFromContext
	originalList := listPeers
	originalGet := getPeer
	originalResolveSN := findPubKeyBySN
	originalResolveIMEI := findPubKeyByIMEI
	originalApprove := approvePeer
	originalBlock := blockPeer
	originalInfo := getPeerInfo
	originalConfig := getPeerConfig
	originalPutConfig := putPeerConfig
	originalRuntime := getPeerRuntime
	originalDelete := deletePeer
	originalRefresh := refreshPeer

	devicePublicKey := giznet.PublicKey{1}
	registration := apitypes.Registration{
		PublicKey: devicePublicKey.String(),
		Role:      apitypes.PeerRoleClient,
		Status:    apitypes.PeerRegistrationStatusActive,
	}
	connectFromContext = func(string) (*gizcli.Client, error) { return &gizcli.Client{}, nil }
	listPeers = func(context.Context, *gizcli.Client) ([]apitypes.Registration, error) {
		return []apitypes.Registration{registration}, nil
	}
	getPeer = func(context.Context, *gizcli.Client, string) (apitypes.Registration, error) {
		return registration, nil
	}
	findPubKeyBySN = func(context.Context, *gizcli.Client, string) (string, error) { return "device-pk", nil }
	findPubKeyByIMEI = func(context.Context, *gizcli.Client, string, string) (string, error) {
		return "device-pk", nil
	}
	approvePeer = func(context.Context, *gizcli.Client, string, apitypes.PeerRole) (apitypes.Registration, error) {
		return registration, nil
	}
	blockPeer = func(context.Context, *gizcli.Client, string) (apitypes.Registration, error) {
		return registration, nil
	}
	getPeerInfo = func(context.Context, *gizcli.Client, string) (apitypes.DeviceInfo, error) {
		return apitypes.DeviceInfo{}, nil
	}
	getPeerConfig = func(context.Context, *gizcli.Client, string) (apitypes.Configuration, error) {
		return apitypes.Configuration{}, nil
	}
	putPeerConfig = func(_ context.Context, _ *gizcli.Client, _ string, cfg apitypes.Configuration) (apitypes.Configuration, error) {
		return cfg, nil
	}
	getPeerRuntime = func(context.Context, *gizcli.Client, string) (apitypes.Runtime, error) {
		online := true
		return apitypes.Runtime{Online: online}, nil
	}
	deletePeer = func(context.Context, *gizcli.Client, string) (apitypes.Registration, error) {
		return registration, nil
	}
	refreshPeer = func(context.Context, *gizcli.Client, string) (adminhttp.RefreshResult, error) {
		return adminhttp.RefreshResult{Peer: apitypes.Peer{PublicKey: devicePublicKey.String()}}, nil
	}

	return func() {
		connectFromContext = originalConnect
		listPeers = originalList
		getPeer = originalGet
		findPubKeyBySN = originalResolveSN
		findPubKeyByIMEI = originalResolveIMEI
		approvePeer = originalApprove
		blockPeer = originalBlock
		getPeerInfo = originalInfo
		getPeerConfig = originalConfig
		putPeerConfig = originalPutConfig
		getPeerRuntime = originalRuntime
		deletePeer = originalDelete
		refreshPeer = originalRefresh
	}
}

type fakePeerConfigClient struct {
	getCfg apitypes.Configuration
	putCfg apitypes.Configuration
}

func (f *fakePeerConfigClient) GetPeerConfig(context.Context, string) (apitypes.Configuration, error) {
	return f.getCfg, nil
}

func (f *fakePeerConfigClient) PutPeerConfig(_ context.Context, _ string, cfg apitypes.Configuration) (apitypes.Configuration, error) {
	f.putCfg = cfg
	return cfg, nil
}

func (f *fakePeerConfigClient) Close() error { return nil }
