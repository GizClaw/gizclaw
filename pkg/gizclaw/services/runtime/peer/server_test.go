package peer

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/serverpublic"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

type stubPeerManager struct {
	runtime       apitypes.Runtime
	refreshResult adminservice.RefreshResult
	refreshOnline bool
	refreshErr    error
}

func (m stubPeerManager) PeerRuntime(context.Context, giznet.PublicKey) apitypes.Runtime {
	return m.runtime
}

func (m stubPeerManager) RefreshPeer(context.Context, giznet.PublicKey) (adminservice.RefreshResult, bool, error) {
	return m.refreshResult, m.refreshOnline, m.refreshErr
}

func saveTestPeer(t *testing.T, server *Server, publicKey giznet.PublicKey, device apitypes.DeviceInfo) {
	t.Helper()
	if _, err := server.SavePeer(context.Background(), apitypes.Peer{
		PublicKey:     publicKey.String(),
		Role:          apitypes.PeerRoleClient,
		Status:        apitypes.PeerRegistrationStatusActive,
		Device:        device,
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SavePeer(%s) error: %v", publicKey, err)
	}
}

func TestServerAdminPeerHandlers(t *testing.T) {
	server := &Server{
		Store: mustBadgerInMemory(t, nil),
	}

	peerKey := giznet.PublicKey{1}
	peerPublicKey := peerKey.String()
	ctx := context.Background()
	sn := "sn-peer"
	tac := "12345678"
	serial := "87654321"
	labelKey := "region"
	labelValue := "cn"

	saveTestPeer(t, server, peerKey, apitypes.DeviceInfo{
		Sn: &sn,
		Hardware: &apitypes.HardwareInfo{
			Imeis:  &[]apitypes.PeerIMEI{{Tac: tac, Serial: serial}},
			Labels: &[]apitypes.PeerLabel{{Key: labelKey, Value: labelValue}},
		},
	})

	getResp, err := server.GetPeer(ctx, adminservice.GetPeerRequestObject{
		PublicKey: string(peerPublicKey),
	})
	if err != nil {
		t.Fatalf("GetPeer error: %v", err)
	}
	getRegistered, ok := getResp.(adminservice.GetPeer200JSONResponse)
	if !ok {
		t.Fatalf("GetPeer response type = %T", getResp)
	}
	if getRegistered.PublicKey != peerPublicKey {
		t.Fatalf("GetPeer = %+v", getRegistered)
	}

	listResp, err := server.ListPeers(ctx, adminservice.ListPeersRequestObject{})
	if err != nil {
		t.Fatalf("ListPeers error: %v", err)
	}
	listed, ok := listResp.(adminservice.ListPeers200JSONResponse)
	if !ok {
		t.Fatalf("ListPeers response type = %T", listResp)
	}
	if len(listed.Items) != 1 || listed.Items[0].PublicKey != peerPublicKey {
		t.Fatalf("ListPeers items = %+v", listed.Items)
	}

	view := "under-12"
	putConfigResp, err := server.PutPeerConfig(ctx, adminservice.PutPeerConfigRequestObject{
		PublicKey: string(peerPublicKey),
		Body: &adminservice.PutPeerConfigJSONRequestBody{
			View: &view,
		},
	})
	if err != nil {
		t.Fatalf("PutPeerConfig error: %v", err)
	}
	if _, ok := putConfigResp.(adminservice.PutPeerConfig200JSONResponse); !ok {
		t.Fatalf("PutPeerConfig response type = %T", putConfigResp)
	}

	getConfigResp, err := server.GetPeerConfig(ctx, adminservice.GetPeerConfigRequestObject{
		PublicKey: string(peerPublicKey),
	})
	if err != nil {
		t.Fatalf("GetPeerConfig error: %v", err)
	}
	cfg, ok := getConfigResp.(adminservice.GetPeerConfig200JSONResponse)
	if !ok {
		t.Fatalf("GetPeerConfig response type = %T", getConfigResp)
	}
	if cfg.View == nil || *cfg.View != view {
		t.Fatalf("GetPeerConfig = %+v", cfg)
	}

	getInfoResp, err := server.GetPeerInfo(ctx, adminservice.GetPeerInfoRequestObject{
		PublicKey: string(peerPublicKey),
	})
	if err != nil {
		t.Fatalf("GetPeerInfo error: %v", err)
	}
	info, ok := getInfoResp.(adminservice.GetPeerInfo200JSONResponse)
	if !ok {
		t.Fatalf("GetPeerInfo response type = %T", getInfoResp)
	}
	if info.Hardware == nil || info.Hardware.Imeis == nil || len(*info.Hardware.Imeis) != 1 {
		t.Fatalf("GetPeerInfo = %+v", info)
	}

	updatedName := "renamed-peer"
	putInfoResp, err := server.PutPeerInfo(ctx, adminservice.PutPeerInfoRequestObject{
		PublicKey: string(peerPublicKey),
		Body: &adminservice.PutPeerInfoJSONRequestBody{
			Name: &updatedName,
			Sn:   &sn,
			Hardware: &apitypes.HardwareInfo{
				Imeis:  &[]apitypes.PeerIMEI{{Tac: tac, Serial: serial}},
				Labels: &[]apitypes.PeerLabel{{Key: labelKey, Value: labelValue}},
			},
		},
	})
	if err != nil {
		t.Fatalf("PutPeerInfo error: %v", err)
	}
	updatedInfo, ok := putInfoResp.(adminservice.PutPeerInfo200JSONResponse)
	if !ok {
		t.Fatalf("PutPeerInfo response type = %T", putInfoResp)
	}
	if updatedInfo.Name == nil || *updatedInfo.Name != updatedName {
		t.Fatalf("PutPeerInfo = %+v", updatedInfo)
	}

	resolveSNResp, err := server.FindPubKeyBySN(ctx, adminservice.FindPubKeyBySNRequestObject{Sn: sn})
	if err != nil {
		t.Fatalf("FindPubKeyBySN error: %v", err)
	}
	resolvedSN, ok := resolveSNResp.(adminservice.FindPubKeyBySN200JSONResponse)
	if !ok {
		t.Fatalf("FindPubKeyBySN response type = %T", resolveSNResp)
	}
	if resolvedSN.PublicKey != peerPublicKey {
		t.Fatalf("FindPubKeyBySN = %+v", resolvedSN)
	}

	resolveIMEIResp, err := server.FindPubKeyByIMEI(ctx, adminservice.FindPubKeyByIMEIRequestObject{
		Tac:    tac,
		Serial: serial,
	})
	if err != nil {
		t.Fatalf("FindPubKeyByIMEI error: %v", err)
	}
	resolvedIMEI, ok := resolveIMEIResp.(adminservice.FindPubKeyByIMEI200JSONResponse)
	if !ok {
		t.Fatalf("FindPubKeyByIMEI response type = %T", resolveIMEIResp)
	}
	if resolvedIMEI.PublicKey != peerPublicKey {
		t.Fatalf("FindPubKeyByIMEI = %+v", resolvedIMEI)
	}

	approveResp, err := server.ApprovePeer(ctx, adminservice.ApprovePeerRequestObject{
		PublicKey: string(peerPublicKey),
		Body:      &adminservice.ApprovePeerJSONRequestBody{Role: apitypes.PeerRoleClient},
	})
	if err != nil {
		t.Fatalf("ApprovePeer error: %v", err)
	}
	approved, ok := approveResp.(adminservice.ApprovePeer200JSONResponse)
	if !ok {
		t.Fatalf("ApprovePeer response type = %T", approveResp)
	}
	if approved.Role != apitypes.PeerRoleClient || approved.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("ApprovePeer = %+v", approved)
	}

	blockResp, err := server.BlockPeer(ctx, adminservice.BlockPeerRequestObject{
		PublicKey: string(peerPublicKey),
	})
	if err != nil {
		t.Fatalf("BlockPeer error: %v", err)
	}
	blocked, ok := blockResp.(adminservice.BlockPeer200JSONResponse)
	if !ok {
		t.Fatalf("BlockPeer response type = %T", blockResp)
	}
	if blocked.Status != apitypes.PeerRegistrationStatusBlocked {
		t.Fatalf("BlockPeer = %+v", blocked)
	}

	deleteResp, err := server.DeletePeer(ctx, adminservice.DeletePeerRequestObject{
		PublicKey: string(peerPublicKey),
	})
	if err != nil {
		t.Fatalf("DeletePeer error: %v", err)
	}
	deleted, ok := deleteResp.(adminservice.DeletePeer200JSONResponse)
	if !ok {
		t.Fatalf("DeletePeer response type = %T", deleteResp)
	}
	if deleted.Role != apitypes.PeerRoleClient || deleted.Status != apitypes.PeerRegistrationStatusBlocked || deleted.ApprovedAt == nil {
		t.Fatalf("DeletePeer = %+v", deleted)
	}
	getDeletedResp, err := server.GetPeer(ctx, adminservice.GetPeerRequestObject{PublicKey: string(peerPublicKey)})
	if err != nil {
		t.Fatalf("GetPeer after DeletePeer error: %v", err)
	}
	if _, ok := getDeletedResp.(adminservice.GetPeer404JSONResponse); !ok {
		t.Fatalf("GetPeer after DeletePeer response type = %T", getDeletedResp)
	}
}

func TestServerListPeersPagination(t *testing.T) {
	server := &Server{
		Store: mustBadgerInMemory(t, nil),
	}

	peerA := giznet.PublicKey{1}
	peerB := giznet.PublicKey{2}
	peerC := giznet.PublicKey{3}
	peerAText := peerA.String()

	registerPeer := func(publicKey giznet.PublicKey, labelValue string) {
		saveTestPeer(t, server, publicKey, apitypes.DeviceInfo{
			Hardware: &apitypes.HardwareInfo{
				Labels: &[]apitypes.PeerLabel{{Key: "region", Value: labelValue}},
			},
		})
	}

	registerPeer(peerA, "cn")
	registerPeer(peerB, "cn")
	registerPeer(peerC, "us")

	limit := int32(1)
	resp, err := server.ListPeers(context.Background(), adminservice.ListPeersRequestObject{
		Params: adminservice.ListPeersParams{
			Limit: &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListPeers pagination error: %v", err)
	}
	listed, ok := resp.(adminservice.ListPeers200JSONResponse)
	if !ok {
		t.Fatalf("ListPeers response type = %T", resp)
	}
	if !listed.HasNext || listed.NextCursor == nil || *listed.NextCursor != peerAText {
		t.Fatalf("ListPeers pagination metadata = %+v", listed)
	}
	if len(listed.Items) != 1 || listed.Items[0].PublicKey != peerAText {
		t.Fatalf("ListPeers paged items = %+v", listed.Items)
	}

}

func TestServerListPeersPaginationPreservesCreationOrder(t *testing.T) {
	server := &Server{
		Store: mustBadgerInMemory(t, nil),
	}

	peerA := giznet.PublicKey{1}
	peerB := giznet.PublicKey{2}
	peerC := giznet.PublicKey{3}
	peerAText := peerA.String()
	peerBText := peerB.String()
	peerCText := peerC.String()

	registerPeer := func(publicKey giznet.PublicKey) {
		saveTestPeer(t, server, publicKey, apitypes.DeviceInfo{})
	}

	registerPeer(peerB)
	registerPeer(peerA)
	registerPeer(peerC)

	limit := int32(2)
	resp, err := server.ListPeers(context.Background(), adminservice.ListPeersRequestObject{
		Params: adminservice.ListPeersParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListPeers first page error: %v", err)
	}
	firstPage, ok := resp.(adminservice.ListPeers200JSONResponse)
	if !ok {
		t.Fatalf("ListPeers first response type = %T", resp)
	}
	if len(firstPage.Items) != 2 || firstPage.Items[0].PublicKey != peerBText || firstPage.Items[1].PublicKey != peerAText {
		t.Fatalf("ListPeers first page = %+v", firstPage.Items)
	}
	if !firstPage.HasNext || firstPage.NextCursor == nil || *firstPage.NextCursor != peerAText {
		t.Fatalf("ListPeers first page metadata = %+v", firstPage)
	}

	resp, err = server.ListPeers(context.Background(), adminservice.ListPeersRequestObject{
		Params: adminservice.ListPeersParams{
			Cursor: firstPage.NextCursor,
			Limit:  &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListPeers second page error: %v", err)
	}
	secondPage, ok := resp.(adminservice.ListPeers200JSONResponse)
	if !ok {
		t.Fatalf("ListPeers second response type = %T", resp)
	}
	if len(secondPage.Items) != 1 || secondPage.Items[0].PublicKey != peerCText {
		t.Fatalf("ListPeers second page = %+v", secondPage.Items)
	}
}

func TestServerListPeersLimitClampsToConfiguredBounds(t *testing.T) {
	server := &Server{
		Store: mustBadgerInMemory(t, nil),
	}
	for _, publicKey := range []giznet.PublicKey{{1}, {2}, {3}} {
		saveTestPeer(t, server, publicKey, apitypes.DeviceInfo{})
	}

	zero := int32(0)
	resp, err := server.ListPeers(context.Background(), adminservice.ListPeersRequestObject{
		Params: adminservice.ListPeersParams{Limit: &zero},
	})
	if err != nil {
		t.Fatalf("ListPeers zero limit error: %v", err)
	}
	defaultPage, ok := resp.(adminservice.ListPeers200JSONResponse)
	if !ok {
		t.Fatalf("ListPeers zero limit response type = %T", resp)
	}
	if len(defaultPage.Items) != 3 || defaultPage.HasNext {
		t.Fatalf("ListPeers zero limit = %+v", defaultPage)
	}

	tooLarge := int32(999)
	resp, err = server.ListPeers(context.Background(), adminservice.ListPeersRequestObject{
		Params: adminservice.ListPeersParams{Limit: &tooLarge},
	})
	if err != nil {
		t.Fatalf("ListPeers large limit error: %v", err)
	}
	clampedPage, ok := resp.(adminservice.ListPeers200JSONResponse)
	if !ok {
		t.Fatalf("ListPeers large limit response type = %T", resp)
	}
	if len(clampedPage.Items) != 3 || clampedPage.HasNext {
		t.Fatalf("ListPeers large limit = %+v", clampedPage)
	}
}

func TestServerRuntimeHandlers(t *testing.T) {
	now := time.Unix(1_700_200_000, 0).UTC()
	runtimeAddr := "10.0.0.1:1234"
	peerKey := giznet.PublicKey{3}
	peerPublicKey := peerKey.String()
	server := &Server{
		Store: mustBadgerInMemory(t, nil),
		PeerManager: stubPeerManager{
			runtime: apitypes.Runtime{
				LastAddr:   &runtimeAddr,
				LastSeenAt: now,
				Online:     true,
			},
			refreshResult: adminservice.RefreshResult{
				Peer: apitypes.Peer{
					PublicKey: peerKey.String(),
					Role:      apitypes.PeerRoleServer,
					Status:    apitypes.PeerRegistrationStatusActive,
				},
				UpdatedFields: &[]string{"device.name"},
			},
			refreshOnline: true,
		},
	}

	saveTestPeer(t, server, peerKey, apitypes.DeviceInfo{})

	getPeerRuntimeResp, err := server.GetPeerRuntime(context.Background(), adminservice.GetPeerRuntimeRequestObject{
		PublicKey: string(peerPublicKey),
	})
	if err != nil {
		t.Fatalf("GetPeerRuntime error: %v", err)
	}
	peerRuntime, ok := getPeerRuntimeResp.(adminservice.GetPeerRuntime200JSONResponse)
	if !ok {
		t.Fatalf("GetPeerRuntime response type = %T", getPeerRuntimeResp)
	}
	if !peerRuntime.Online || peerRuntime.LastAddr == nil || *peerRuntime.LastAddr != runtimeAddr {
		t.Fatalf("GetPeerRuntime = %+v", peerRuntime)
	}

	publicRuntime := server.GetSelfRuntime(context.Background(), peerKey)
	if !publicRuntime.Online || publicRuntime.LastAddr == nil || *publicRuntime.LastAddr != runtimeAddr {
		t.Fatalf("GetSelfRuntime = %+v", publicRuntime)
	}

	refreshResp, err := server.RefreshPeer(context.Background(), adminservice.RefreshPeerRequestObject{
		PublicKey: string(peerPublicKey),
	})
	if err != nil {
		t.Fatalf("RefreshPeer error: %v", err)
	}
	refreshed, ok := refreshResp.(adminservice.RefreshPeer200JSONResponse)
	if !ok {
		t.Fatalf("RefreshPeer response type = %T", refreshResp)
	}
	if refreshed.Peer.PublicKey != peerPublicKey || refreshed.UpdatedFields == nil || len(*refreshed.UpdatedFields) != 1 {
		t.Fatalf("RefreshPeer = %+v", refreshed)
	}
}

func TestServerPublicHandlers(t *testing.T) {
	before := time.Now()
	peerKey := giznet.PublicKey{5}
	server := &Server{
		Store:           mustBadgerInMemory(t, nil),
		BuildCommit:     "deadbeef",
		ServerPublicKey: giznet.PublicKey{1},
	}

	name := "peer-a"
	sn := "sn-1"
	labelKey := "region"
	labelValue := "cn"

	saveTestPeer(t, server, peerKey, apitypes.DeviceInfo{
		Name: &name,
		Sn:   &sn,
		Hardware: &apitypes.HardwareInfo{
			Labels: &[]apitypes.PeerLabel{{Key: labelKey, Value: labelValue}},
		},
	})

	info, err := server.GetSelfInfo(context.Background(), peerKey)
	if err != nil {
		t.Fatalf("GetSelfInfo error: %v", err)
	}
	if info.Sn == nil || *info.Sn != sn {
		t.Fatalf("GetSelfInfo sn = %v", info.Sn)
	}

	serverInfoResp, err := server.GetServerInfo(context.Background(), serverpublic.GetServerInfoRequestObject{})
	if err != nil {
		t.Fatalf("GetServerInfo error: %v", err)
	}
	serverInfo, ok := serverInfoResp.(serverpublic.GetServerInfo200JSONResponse)
	if !ok {
		t.Fatalf("GetServerInfo response type = %T", serverInfoResp)
	}
	if serverInfo.BuildCommit != "deadbeef" || serverInfo.PublicKey != server.ServerPublicKey.String() {
		t.Fatalf("GetServerInfo = %+v", serverInfo)
	}
	if serverInfo.ServerTime < before.UnixMilli() || serverInfo.ServerTime > time.Now().Add(time.Second).UnixMilli() {
		t.Fatalf("GetServerInfo = %+v", serverInfo)
	}
}

func TestServerPublicHandlersPutInfoConfigAndRuntime(t *testing.T) {
	now := time.Unix(1_700_500_000, 0).UTC()
	runtimeAddr := "10.0.0.1:8888"
	peerKey := giznet.PublicKey{4}
	peerPublicKey := peerKey.String()
	server := &Server{
		Store: mustBadgerInMemory(t, nil),
		PeerManager: stubPeerManager{
			runtime: apitypes.Runtime{
				LastAddr:   &runtimeAddr,
				LastSeenAt: now,
				Online:     true,
			},
		},
	}

	sn := "sn-old"
	saveTestPeer(t, server, peerKey, apitypes.DeviceInfo{Sn: &sn})

	view := "under-12"
	_, err := server.PutPeerConfig(context.Background(), adminservice.PutPeerConfigRequestObject{
		PublicKey: string(peerPublicKey),
		Body: &adminservice.PutPeerConfigJSONRequestBody{
			View: &view,
		},
	})
	if err != nil {
		t.Fatalf("PutPeerConfig error: %v", err)
	}

	getConfigResp, err := server.GetPeerConfig(context.Background(), adminservice.GetPeerConfigRequestObject{
		PublicKey: string(peerPublicKey),
	})
	if err != nil {
		t.Fatalf("GetPeerConfig error: %v", err)
	}
	cfg, ok := getConfigResp.(adminservice.GetPeerConfig200JSONResponse)
	if !ok {
		t.Fatalf("GetPeerConfig response type = %T", getConfigResp)
	}
	if cfg.View == nil || *cfg.View != view {
		t.Fatalf("GetPeerConfig = %+v", cfg)
	}

	newSN := "sn-new"
	putInfo, err := server.PutSelfInfo(context.Background(), peerKey, apitypes.DeviceInfo{Sn: &newSN})
	if err != nil {
		t.Fatalf("PutSelfInfo error: %v", err)
	}
	if putInfo.Sn == nil || *putInfo.Sn != newSN {
		t.Fatalf("PutSelfInfo = %+v", putInfo)
	}

	publicRuntime := server.GetSelfRuntime(context.Background(), peerKey)
	if !publicRuntime.Online || publicRuntime.LastAddr == nil || *publicRuntime.LastAddr != runtimeAddr {
		t.Fatalf("GetSelfRuntime = %+v", publicRuntime)
	}
}
