//go:build gizclaw_e2e

package rpc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestRegistrationBindsFirmwareRPC(t *testing.T) {
	env := newFirmwareResourceHarness(t)

	tests := []struct {
		channel     rpcapi.FirmwareChannelName
		description string
		url         string
		sha256      string
		size        int64
	}{
		{rpcapi.FirmwareChannelNameStable, "Devkit stable package", "https://firmware.example.invalid/devkit/stable.tar.zlib", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", 4096},
		{rpcapi.FirmwareChannelNameBeta, "Devkit beta package", "https://firmware.example.invalid/devkit/beta.tar.zlib", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", 8192},
		{rpcapi.FirmwareChannelNameDevelop, "", "https://firmware.example.invalid/devkit/develop.tar.zlib", "123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0", 12288},
		{rpcapi.FirmwareChannelNamePending, "Devkit pending package", "https://firmware.example.invalid/devkit/pending.tar.zlib", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210", 16384},
	}
	for _, tc := range tests {
		t.Run(string(tc.channel), func(t *testing.T) {
			got, err := env.peer.GetFirmware(env.ctx, "firmware.get."+string(tc.channel), rpcapi.FirmwareGetRequest{Channel: tc.channel})
			if err != nil {
				t.Fatalf("firmware.get %s: %v", tc.channel, err)
			}
			if got.FirmwareName != sharedFirmware || got.Channel != tc.channel || got.Url != tc.url || got.Sha256 != tc.sha256 || got.Size != tc.size {
				t.Fatalf("firmware.get %s = %#v", tc.channel, got)
			}
			if tc.description == "" {
				if got.Description != nil {
					t.Fatalf("firmware.get %s description = %q, want absent", tc.channel, *got.Description)
				}
			} else if got.Description == nil || *got.Description != tc.description {
				t.Fatalf("firmware.get %s description = %#v, want %q", tc.channel, got.Description, tc.description)
			}
		})
	}

	_, err := env.peer.GetFirmware(env.ctx, "firmware.get.invalid", rpcapi.FirmwareGetRequest{})
	requireFirmwareRPCError(t, err, rpcapi.RPCErrorCodeInvalidParams, "invalid params")

	denied := env.h.ConnectClientFromContext("peer-denied")
	defer denied.Close()
	_, err = denied.GetFirmware(env.ctx, "firmware.get.denied", rpcapi.FirmwareGetRequest{Channel: rpcapi.FirmwareChannelNameStable})
	requireFirmwareRPCError(t, err, rpcapi.RPCErrorCodeNotFound, "firmware is not bound to peer")
}

func newFirmwareResourceHarness(t *testing.T) *serverResourceHarness {
	t.Helper()
	h := clitest.NewSetupHarness(t, "client-rpc-firmware")
	aliasSetupAdminContext(t, h)
	registerSetupPeer(t, h, "peer-a", "peer-a-firmware-sn", true)
	registerSetupPeer(t, h, "peer-denied", "peer-denied-firmware-sn", false)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	peer := h.ConnectClientFromContext("peer-a")
	t.Cleanup(func() { peer.Close() })
	registerRuntimeProfile(t, h, peer, "peer-a", apitypes.RuntimeProfileSpec{
		Resources: apitypes.RuntimeProfileResources{},
		Workflows: apitypes.RuntimeProfileWorkflows{
			System: apitypes.RuntimeProfileSystemWorkflows{
				FriendChatroom: "chatroom-direct",
				GroupChatroom:  "chatroom-direct",
				Pet:            "pet-chatroom",
			},
			Collections: apitypes.RuntimeProfileWorkflowCollections{},
		},
	})
	return &serverResourceHarness{h: h, ctx: ctx, peer: peer}
}

func requireFirmwareRPCError(t *testing.T, err error, code rpcapi.RPCErrorCode, message string) {
	t.Helper()
	var rpcErr rpcapi.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != code || rpcErr.Message != message {
		t.Fatalf("firmware RPC error = %#v, want code %d message %q", err, code, message)
	}
}
