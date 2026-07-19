package peerscmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestPeerCommandsReturnContextErrors(t *testing.T) {
	cases := [][]string{
		{"list"},
		{"get", "device-pk"},
		{"resolve-sn", "sn-001"},
		{"resolve-imei", "12345678", "000001"},
		{"approve", "device-pk", "client"},
		{"block", "device-pk"},
		{"info", "device-pk"},
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
		getPeerRuntime = originalRuntime
		deletePeer = originalDelete
		refreshPeer = originalRefresh
	}
}
