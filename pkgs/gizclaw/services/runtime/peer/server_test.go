package peer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func saveTestPeer(t *testing.T, server *Server, publicKey giznet.PublicKey, device apitypes.DeviceInfo) {
	t.Helper()
	if _, err := server.SavePeer(context.Background(), apitypes.Peer{
		PublicKey: publicKey.String(),
		Role:      apitypes.PeerRoleClient,
		Status:    apitypes.PeerRegistrationStatusActive,
		Device:    device,
	}); err != nil {
		t.Fatalf("SavePeer(%s) error: %v", publicKey, err)
	}
}

func TestDeleteSelfReconnectCreatesDistinctDeletionEvents(t *testing.T) {
	ctx := context.Background()
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	publicKey := giznet.PublicKey{9}
	saveTestPeer(t, server, publicKey, apitypes.DeviceInfo{})
	if err := server.DeleteSelf(ctx, publicKey); err != nil {
		t.Fatalf("DeleteSelf(first): %v", err)
	}
	if _, err := server.EnsureConnectedPeer(ctx, publicKey); err != nil {
		t.Fatalf("EnsureConnectedPeer: %v", err)
	}
	if err := server.DeleteSelf(ctx, publicKey); err != nil {
		t.Fatalf("DeleteSelf(second): %v", err)
	}
	count := 0
	for _, err := range server.Store.List(ctx, kv.Key{"pending-deletion", "by-id"}) {
		if err != nil {
			t.Fatalf("list pending deletions: %v", err)
		}
		count++
	}
	if count != 2 {
		t.Fatalf("pending deletion events = %d, want 2", count)
	}
}

type stubPeerManager struct {
	runtime       apitypes.Runtime
	refreshResult adminhttp.RefreshResult
	refreshOnline bool
	refreshErr    error
}

func (m stubPeerManager) PeerRuntime(context.Context, giznet.PublicKey) apitypes.Runtime {
	return m.runtime
}

func (m stubPeerManager) RefreshPeer(context.Context, giznet.PublicKey) (adminhttp.RefreshResult, bool, error) {
	return m.refreshResult, m.refreshOnline, m.refreshErr
}

func TestServerAdminPeerHandlers(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}

	peerKey := giznet.PublicKey{1}
	peerPublicKey := peerKey.String()
	ctx := context.Background()
	sn := "sn-peer"
	tac := "12345678"
	serial := "87654321"
	labelKey := "region"
	labelValue := "cn"

	saveTestPeer(t, server, peerKey, apitypes.DeviceInfo{
		Identifiers: &apitypes.DeviceIdentifiers{
			Sn:     &sn,
			Imeis:  &[]apitypes.PeerIMEI{{Tac: tac, Serial: serial}},
			Labels: &[]apitypes.PeerLabel{{Key: labelKey, Value: labelValue}},
		},
	})

	getResp, err := server.GetPeer(ctx, adminhttp.GetPeerRequestObject{
		PublicKey: string(peerPublicKey),
	})
	if err != nil {
		t.Fatalf("GetPeer error: %v", err)
	}
	getRegistered, ok := getResp.(adminhttp.GetPeer200JSONResponse)
	if !ok {
		t.Fatalf("GetPeer response type = %T", getResp)
	}
	if getRegistered.PublicKey != peerPublicKey {
		t.Fatalf("GetPeer = %+v", getRegistered)
	}

	listResp, err := server.ListPeers(ctx, adminhttp.ListPeersRequestObject{})
	if err != nil {
		t.Fatalf("ListPeers error: %v", err)
	}
	listed, ok := listResp.(adminhttp.ListPeers200JSONResponse)
	if !ok {
		t.Fatalf("ListPeers response type = %T", listResp)
	}
	if len(listed.Items) != 1 || listed.Items[0].PublicKey != peerPublicKey {
		t.Fatalf("ListPeers items = %+v", listed.Items)
	}

	getInfoResp, err := server.GetPeerInfo(ctx, adminhttp.GetPeerInfoRequestObject{
		PublicKey: string(peerPublicKey),
	})
	if err != nil {
		t.Fatalf("GetPeerInfo error: %v", err)
	}
	info, ok := getInfoResp.(adminhttp.GetPeerInfo200JSONResponse)
	if !ok {
		t.Fatalf("GetPeerInfo response type = %T", getInfoResp)
	}
	if info.Identifiers == nil || info.Identifiers.Imeis == nil || len(*info.Identifiers.Imeis) != 1 {
		t.Fatalf("GetPeerInfo = %+v", info)
	}

	updatedName := "renamed-peer"
	putInfoResp, err := server.PutPeerInfo(ctx, adminhttp.PutPeerInfoRequestObject{
		PublicKey: string(peerPublicKey),
		Body: &adminhttp.PutPeerInfoJSONRequestBody{
			Name: &updatedName,
		},
	})
	if err != nil {
		t.Fatalf("PutPeerInfo error: %v", err)
	}
	updatedInfo, ok := putInfoResp.(adminhttp.PutPeerInfo200JSONResponse)
	if !ok {
		t.Fatalf("PutPeerInfo response type = %T", putInfoResp)
	}
	if updatedInfo.Name == nil || *updatedInfo.Name != updatedName {
		t.Fatalf("PutPeerInfo = %+v", updatedInfo)
	}
	if updatedInfo.Identifiers == nil || updatedInfo.Identifiers.Sn == nil || *updatedInfo.Identifiers.Sn != sn {
		t.Fatalf("PutPeerInfo did not preserve identifiers = %+v", updatedInfo)
	}
	resolveSNResp, err := server.FindPubKeyBySN(ctx, adminhttp.FindPubKeyBySNRequestObject{Sn: sn})
	if err != nil {
		t.Fatalf("FindPubKeyBySN error: %v", err)
	}
	resolvedSN, ok := resolveSNResp.(adminhttp.FindPubKeyBySN200JSONResponse)
	if !ok {
		t.Fatalf("FindPubKeyBySN response type = %T", resolveSNResp)
	}
	if resolvedSN.PublicKey != peerPublicKey {
		t.Fatalf("FindPubKeyBySN = %+v", resolvedSN)
	}

	resolveIMEIResp, err := server.FindPubKeyByIMEI(ctx, adminhttp.FindPubKeyByIMEIRequestObject{
		Tac:    tac,
		Serial: serial,
	})
	if err != nil {
		t.Fatalf("FindPubKeyByIMEI error: %v", err)
	}
	resolvedIMEI, ok := resolveIMEIResp.(adminhttp.FindPubKeyByIMEI200JSONResponse)
	if !ok {
		t.Fatalf("FindPubKeyByIMEI response type = %T", resolveIMEIResp)
	}
	if resolvedIMEI.PublicKey != peerPublicKey {
		t.Fatalf("FindPubKeyByIMEI = %+v", resolvedIMEI)
	}

	approveResp, err := server.ApprovePeer(ctx, adminhttp.ApprovePeerRequestObject{
		PublicKey: string(peerPublicKey),
		Body:      &adminhttp.ApprovePeerJSONRequestBody{Role: apitypes.PeerRoleClient},
	})
	if err != nil {
		t.Fatalf("ApprovePeer error: %v", err)
	}
	approved, ok := approveResp.(adminhttp.ApprovePeer200JSONResponse)
	if !ok {
		t.Fatalf("ApprovePeer response type = %T", approveResp)
	}
	if approved.Role != apitypes.PeerRoleClient || approved.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("ApprovePeer = %+v", approved)
	}

	blockResp, err := server.BlockPeer(ctx, adminhttp.BlockPeerRequestObject{
		PublicKey: string(peerPublicKey),
	})
	if err != nil {
		t.Fatalf("BlockPeer error: %v", err)
	}
	blocked, ok := blockResp.(adminhttp.BlockPeer200JSONResponse)
	if !ok {
		t.Fatalf("BlockPeer response type = %T", blockResp)
	}
	if blocked.Status != apitypes.PeerRegistrationStatusBlocked {
		t.Fatalf("BlockPeer = %+v", blocked)
	}

	deleteResp, err := server.DeletePeer(ctx, adminhttp.DeletePeerRequestObject{
		PublicKey: string(peerPublicKey),
	})
	if err != nil {
		t.Fatalf("DeletePeer error: %v", err)
	}
	deleted, ok := deleteResp.(adminhttp.DeletePeer200JSONResponse)
	if !ok {
		t.Fatalf("DeletePeer response type = %T", deleteResp)
	}
	if deleted.Role != apitypes.PeerRoleClient || deleted.Status != apitypes.PeerRegistrationStatusBlocked || deleted.ApprovedAt == nil {
		t.Fatalf("DeletePeer = %+v", deleted)
	}
	getDeletedResp, err := server.GetPeer(ctx, adminhttp.GetPeerRequestObject{PublicKey: string(peerPublicKey)})
	if err != nil {
		t.Fatalf("GetPeer after DeletePeer error: %v", err)
	}
	if _, ok := getDeletedResp.(adminhttp.GetPeer404JSONResponse); !ok {
		t.Fatalf("GetPeer after DeletePeer response type = %T", getDeletedResp)
	}
	if pending, err := pendingdeletion.HasLocator(ctx, server.Store, pendingdeletion.KindPeer, peerPublicKey); err != nil || !pending {
		t.Fatalf("peer pending deletion = %v, error = %v", pending, err)
	}
	if response, err := server.FindPubKeyBySN(ctx, adminhttp.FindPubKeyBySNRequestObject{Sn: sn}); err != nil {
		t.Fatalf("FindPubKeyBySN after delete error: %v", err)
	} else if _, ok := response.(adminhttp.FindPubKeyBySN404JSONResponse); !ok {
		t.Fatalf("FindPubKeyBySN after delete response = %T", response)
	}
	if response, err := server.FindPubKeyByIMEI(ctx, adminhttp.FindPubKeyByIMEIRequestObject{Tac: tac, Serial: serial}); err != nil {
		t.Fatalf("FindPubKeyByIMEI after delete error: %v", err)
	} else if _, ok := response.(adminhttp.FindPubKeyByIMEI404JSONResponse); !ok {
		t.Fatalf("FindPubKeyByIMEI after delete response = %T", response)
	}
	var pendingRecord pendingdeletion.Record
	for entry, err := range server.Store.List(ctx, kv.Key{"pending-deletion", "by-id"}) {
		if err != nil {
			t.Fatalf("list pending deletions: %v", err)
		}
		if err := json.Unmarshal(entry.Value, &pendingRecord); err != nil {
			t.Fatalf("decode pending deletion: %v", err)
		}
		break
	}
	var descriptor map[string]any
	if err := json.Unmarshal(pendingRecord.Descriptor, &descriptor); err != nil {
		t.Fatalf("decode pending descriptor: %v", err)
	}
	if len(descriptor) != 1 || descriptor["public_key"] != peerPublicKey {
		t.Fatalf("pending descriptor = %#v, want only public_key", descriptor)
	}
	if err := server.DeleteSelf(ctx, peerKey); err != nil {
		t.Fatalf("DeleteSelf() retry error = %v", err)
	}
	if err := server.BootstrapEdgeNodes(ctx, []giznet.PublicKey{peerKey}); !errors.Is(err, ErrPeerPendingDeletion) {
		t.Fatalf("BootstrapEdgeNodes() while pending error = %v, want ErrPeerPendingDeletion", err)
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
			Identifiers: &apitypes.DeviceIdentifiers{
				Labels: &[]apitypes.PeerLabel{{Key: "region", Value: labelValue}},
			},
		})
	}

	registerPeer(peerA, "cn")
	registerPeer(peerB, "cn")
	registerPeer(peerC, "us")

	limit := int32(1)
	resp, err := server.ListPeers(context.Background(), adminhttp.ListPeersRequestObject{
		Params: adminhttp.ListPeersParams{
			Limit: &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListPeers pagination error: %v", err)
	}
	listed, ok := resp.(adminhttp.ListPeers200JSONResponse)
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
	resp, err := server.ListPeers(context.Background(), adminhttp.ListPeersRequestObject{
		Params: adminhttp.ListPeersParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListPeers first page error: %v", err)
	}
	firstPage, ok := resp.(adminhttp.ListPeers200JSONResponse)
	if !ok {
		t.Fatalf("ListPeers first response type = %T", resp)
	}
	if len(firstPage.Items) != 2 || firstPage.Items[0].PublicKey != peerBText || firstPage.Items[1].PublicKey != peerAText {
		t.Fatalf("ListPeers first page = %+v", firstPage.Items)
	}
	if !firstPage.HasNext || firstPage.NextCursor == nil || *firstPage.NextCursor != peerAText {
		t.Fatalf("ListPeers first page metadata = %+v", firstPage)
	}

	resp, err = server.ListPeers(context.Background(), adminhttp.ListPeersRequestObject{
		Params: adminhttp.ListPeersParams{
			Cursor: firstPage.NextCursor,
			Limit:  &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListPeers second page error: %v", err)
	}
	secondPage, ok := resp.(adminhttp.ListPeers200JSONResponse)
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
	resp, err := server.ListPeers(context.Background(), adminhttp.ListPeersRequestObject{
		Params: adminhttp.ListPeersParams{Limit: &zero},
	})
	if err != nil {
		t.Fatalf("ListPeers zero limit error: %v", err)
	}
	defaultPage, ok := resp.(adminhttp.ListPeers200JSONResponse)
	if !ok {
		t.Fatalf("ListPeers zero limit response type = %T", resp)
	}
	if len(defaultPage.Items) != 3 || defaultPage.HasNext {
		t.Fatalf("ListPeers zero limit = %+v", defaultPage)
	}

	tooLarge := int32(999)
	resp, err = server.ListPeers(context.Background(), adminhttp.ListPeersRequestObject{
		Params: adminhttp.ListPeersParams{Limit: &tooLarge},
	})
	if err != nil {
		t.Fatalf("ListPeers large limit error: %v", err)
	}
	clampedPage, ok := resp.(adminhttp.ListPeers200JSONResponse)
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
	server := &Server{
		Store: mustBadgerInMemory(t, nil),
		PeerManager: stubPeerManager{
			runtime: apitypes.Runtime{
				LastAddr:   &runtimeAddr,
				LastSeenAt: now,
				Online:     true,
			},
			refreshResult: adminhttp.RefreshResult{
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

	getPeerRuntimeResp, err := server.GetPeerRuntime(context.Background(), adminhttp.GetPeerRuntimeRequestObject{
		PublicKey: peerKey.String(),
	})
	if err != nil {
		t.Fatalf("GetPeerRuntime error: %v", err)
	}
	peerRuntime, ok := getPeerRuntimeResp.(adminhttp.GetPeerRuntime200JSONResponse)
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

	refreshResp, err := server.RefreshPeer(context.Background(), adminhttp.RefreshPeerRequestObject{
		PublicKey: peerKey.String(),
	})
	if err != nil {
		t.Fatalf("RefreshPeer error: %v", err)
	}
	refreshed, ok := refreshResp.(adminhttp.RefreshPeer200JSONResponse)
	if !ok {
		t.Fatalf("RefreshPeer response type = %T", refreshResp)
	}
	if refreshed.Peer.PublicKey != peerKey.String() || refreshed.UpdatedFields == nil || len(*refreshed.UpdatedFields) != 1 {
		t.Fatalf("RefreshPeer = %+v", refreshed)
	}
}

func TestPeerHTTPHandlers(t *testing.T) {
	before := time.Now()
	peerKey := giznet.PublicKey{5}
	server := &Server{
		Store:           mustBadgerInMemory(t, nil),
		BuildCommit:     "deadbeef",
		Endpoint:        "127.0.0.1:9820",
		ServerPublicKey: giznet.PublicKey{1},
		SignalingPath:   "/webrtc/v1/offer",
	}

	name := "peer-a"
	sn := "sn-1"
	labelKey := "region"
	labelValue := "cn"

	saveTestPeer(t, server, peerKey, apitypes.DeviceInfo{
		Name: &name,
		Identifiers: &apitypes.DeviceIdentifiers{
			Sn:     &sn,
			Labels: &[]apitypes.PeerLabel{{Key: labelKey, Value: labelValue}},
		},
	})

	info, err := server.GetSelfInfo(context.Background(), peerKey)
	if err != nil {
		t.Fatalf("GetSelfInfo error: %v", err)
	}
	if info.Identifiers == nil || info.Identifiers.Sn == nil || *info.Identifiers.Sn != sn {
		t.Fatalf("GetSelfInfo identifiers = %v", info.Identifiers)
	}

	serverInfoResp, err := server.GetServerInfo(context.Background(), peerhttp.GetServerInfoRequestObject{})
	if err != nil {
		t.Fatalf("GetServerInfo error: %v", err)
	}
	serverInfo, ok := serverInfoResp.(peerhttp.GetServerInfo200JSONResponse)
	if !ok {
		t.Fatalf("GetServerInfo response type = %T", serverInfoResp)
	}
	if serverInfo.BuildCommit != "deadbeef" || serverInfo.PublicKey != server.ServerPublicKey.String() {
		t.Fatalf("GetServerInfo = %+v", serverInfo)
	}
	if serverInfo.Protocol != "gizclaw-webrtc" {
		t.Fatalf("GetServerInfo protocol = %q, want gizclaw-webrtc", serverInfo.Protocol)
	}
	if serverInfo.Endpoint != server.Endpoint {
		t.Fatalf("GetServerInfo endpoint = %q, want %q", serverInfo.Endpoint, server.Endpoint)
	}
	if serverInfo.SignalingPath != server.SignalingPath {
		t.Fatalf("GetServerInfo signaling_path = %q, want %q", serverInfo.SignalingPath, server.SignalingPath)
	}
	if !serverInfo.Ice.Udp || serverInfo.Ice.Tcp {
		t.Fatalf("GetServerInfo ice = %+v, want udp=true tcp=false", serverInfo.Ice)
	}
	if serverInfo.ServerTime < before.UnixMilli() || serverInfo.ServerTime > time.Now().Add(time.Second).UnixMilli() {
		t.Fatalf("GetServerInfo = %+v", serverInfo)
	}
}

func TestGetServerInfoReportsICETCP(t *testing.T) {
	server := &Server{ICETCP: true}

	serverInfoResp, err := server.GetServerInfo(context.Background(), peerhttp.GetServerInfoRequestObject{})
	if err != nil {
		t.Fatalf("GetServerInfo error: %v", err)
	}
	serverInfo, ok := serverInfoResp.(peerhttp.GetServerInfo200JSONResponse)
	if !ok {
		t.Fatalf("GetServerInfo response type = %T", serverInfoResp)
	}
	if !serverInfo.Ice.Udp || !serverInfo.Ice.Tcp {
		t.Fatalf("GetServerInfo ice = %+v, want udp=true tcp=true", serverInfo.Ice)
	}
}

func TestServerInfoICEServersPreserveStaticCredentials(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	servers := serverInfoICEServersAt([]gizwebrtc.ICEServer{{
		URLs:       []string{"turn:edge.example.com:3478?transport=udp"},
		Username:   "edge",
		Credential: "static-password",
	}}, now)
	if servers == nil || len(*servers) != 1 {
		t.Fatalf("serverInfoICEServersAt = %#v, want one server", servers)
	}
	got := (*servers)[0]
	if got.Username == nil || *got.Username != "edge" {
		t.Fatalf("username = %v, want static username", got.Username)
	}
	if got.Credential == nil || *got.Credential != "static-password" {
		t.Fatalf("credential = %v, want static credential", got.Credential)
	}
}

func TestServerInfoICEServersMintShortLivedCredentials(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	servers := serverInfoICEServersAt([]gizwebrtc.ICEServer{{
		URLs:           []string{"turn:edge.example.com:3478?transport=udp"},
		Username:       "edge",
		Credential:     "long-term-secret",
		CredentialMode: gizwebrtc.ICECredentialModeTURNREST,
	}}, now)
	if servers == nil || len(*servers) != 1 {
		t.Fatalf("serverInfoICEServersAt = %#v, want one server", servers)
	}
	got := (*servers)[0]
	if got.Username == nil || *got.Username != "1700000600:edge" {
		t.Fatalf("username = %v, want short-lived REST username", got.Username)
	}
	if got.Credential == nil {
		t.Fatal("credential is nil")
	}
	if *got.Credential == "long-term-secret" {
		t.Fatal("server-info exposed the long-term TURN secret")
	}
	if want := turnRESTCredential("long-term-secret", *got.Username); *got.Credential != want {
		t.Fatalf("credential = %q, want %q", *got.Credential, want)
	}
}

func TestPeerHTTPHandlersPutInfoAndRuntime(t *testing.T) {
	now := time.Unix(1_700_500_000, 0).UTC()
	runtimeAddr := "10.0.0.1:8888"
	peerKey := giznet.PublicKey{4}
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
	saveTestPeer(t, server, peerKey, apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn}})

	newEmoji := "🧑‍🚀"
	putInfo, err := server.PutSelfInfo(context.Background(), peerKey, apitypes.DeviceInfo{Emoji: &newEmoji})
	if err != nil {
		t.Fatalf("PutSelfInfo error: %v", err)
	}
	if putInfo.Emoji == nil || *putInfo.Emoji != newEmoji {
		t.Fatalf("PutSelfInfo = %+v", putInfo)
	}
	tooLong := string(make([]byte, 65))
	if _, err := server.PutSelfInfo(context.Background(), peerKey, apitypes.DeviceInfo{Emoji: &tooLong}); !errors.Is(err, ErrInvalidInfo) {
		t.Fatalf("PutSelfInfo(long emoji) error = %v, want ErrInvalidInfo", err)
	}
	tooLongName := string(make([]byte, 257))
	if _, err := server.PutSelfInfo(context.Background(), peerKey, apitypes.DeviceInfo{Name: &tooLongName}); !errors.Is(err, ErrInvalidInfo) {
		t.Fatalf("PutSelfInfo(long name) error = %v, want ErrInvalidInfo", err)
	}
	invalidUTF8Name := string([]byte{0xff})
	if _, err := server.putInfo(context.Background(), peerKey, apitypes.DeviceInfo{Name: &invalidUTF8Name}); !errors.Is(err, ErrInvalidInfo) {
		t.Fatalf("putInfo(invalid UTF-8 name) error = %v, want ErrInvalidInfo", err)
	}

	publicRuntime := server.GetSelfRuntime(context.Background(), peerKey)
	if !publicRuntime.Online || publicRuntime.LastAddr == nil || *publicRuntime.LastAddr != runtimeAddr {
		t.Fatalf("GetSelfRuntime = %+v", publicRuntime)
	}
}
