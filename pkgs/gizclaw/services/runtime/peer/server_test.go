package peer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
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

func registrationResultForTest(t *testing.T, result adminhttp.PeerRegistrationResult) apitypes.Registration {
	t.Helper()
	registration, err := result.AsExternalRef0Registration()
	if err != nil {
		t.Fatalf("decode Registration result: %v", err)
	}
	return registration
}

func TestDeleteSelfRetainsPeerFencesMutationsAndReusesDeletionEvent(t *testing.T) {
	ctx := context.Background()
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	publicKey := giznet.PublicKey{9}
	saveTestPeer(t, server, publicKey, apitypes.DeviceInfo{})
	if err := server.DeleteSelf(ctx, publicKey); err != nil {
		t.Fatalf("DeleteSelf(first): %v", err)
	}
	if _, err := server.BindFirmware(ctx, publicKey, "firmware-reconnected"); !errors.Is(err, ErrPeerPendingDeletion) {
		t.Fatalf("BindFirmware(marked) error = %v", err)
	}
	if _, err := server.SavePeer(ctx, apitypes.Peer{
		PublicKey: publicKey.String(),
		Role:      apitypes.PeerRoleClient,
		Status:    apitypes.PeerRegistrationStatusActive,
		Device:    apitypes.DeviceInfo{},
	}); !errors.Is(err, ErrPeerPendingDeletion) {
		t.Fatalf("SavePeer(marked) error = %v", err)
	}
	if err := server.BootstrapEdgeNodes(ctx, []giznet.PublicKey{publicKey}); !errors.Is(err, ErrPeerPendingDeletion) {
		t.Fatalf("BootstrapEdgeNodes(marked) error = %v", err)
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
	if count != 1 {
		t.Fatalf("pending deletion events = %d, want 1", count)
	}
}

func TestDeleteSelfRetryPreservesPeerButRejectsReconnect(t *testing.T) {
	ctx := context.Background()
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	publicKey := giznet.PublicKey{10}
	saveTestPeer(t, server, publicKey, apitypes.DeviceInfo{})
	if err := server.DeleteSelf(ctx, publicKey); err != nil {
		t.Fatalf("DeleteSelf(first): %v", err)
	}
	if err := server.DeleteSelf(ctx, publicKey); err != nil {
		t.Fatalf("DeleteSelf(retry): %v", err)
	}
	if _, err := server.EnsureConnectedPeer(ctx, publicKey); !errors.Is(err, ErrPeerPendingDeletion) {
		t.Fatalf("EnsureConnectedPeer error = %v", err)
	}
	if _, err := server.LoadPeer(ctx, publicKey); err != nil {
		t.Fatalf("LoadPeer(reconnected): %v", err)
	}
}

func TestDeleteSelfLegacyMarkerAllowsRetryAfterRemovedPeer(t *testing.T) {
	ctx := context.Background()
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	publicKey := giznet.PublicKey{11}
	publicKeyText := publicKey.String()
	record, err := pendingdeletion.New(
		pendingdeletion.KindPeer,
		publicKeyText,
		&publicKeyText,
		pendingdeletion.ReasonPeerDelete,
		map[string]string{"public_key": publicKeyText},
		time.Unix(1, 0),
	)
	if err != nil {
		t.Fatalf("New legacy marker: %v", err)
	}
	entries, err := pendingdeletion.KVEntries(record)
	if err != nil {
		t.Fatalf("KVEntries legacy marker: %v", err)
	}
	encodedPublicKey := base64.RawURLEncoding.EncodeToString([]byte(publicKeyText))
	entries = append(entries, kv.Entry{Key: kv.Key{
		"pending-deletion", "by-locator", string(record.Kind), encodedPublicKey, record.DeletionID,
	}})
	if err := server.Store.BatchSet(ctx, entries); err != nil {
		t.Fatalf("seed legacy marker: %v", err)
	}

	if err := server.DeleteSelf(ctx, publicKey); err != nil {
		t.Fatalf("DeleteSelf(legacy removed Peer retry): %v", err)
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
	sn := "sn%2Fpeer"
	tac := "12%34"
	serial := "87%2F654321"
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
	if registrationResultForTest(t, adminhttp.PeerRegistrationResult(getRegistered)).PublicKey != peerPublicKey {
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
	if len(listed.Items) != 1 || registrationResultForTest(t, listed.Items[0]).PublicKey != peerPublicKey {
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
	resolveSNResp, err := server.FindPeersBySN(ctx, adminhttp.FindPeersBySNRequestObject{Sn: sn})
	if err != nil {
		t.Fatalf("FindPeersBySN error: %v", err)
	}
	resolvedSN, ok := resolveSNResp.(adminhttp.FindPeersBySN200JSONResponse)
	if !ok {
		t.Fatalf("FindPeersBySN response type = %T", resolveSNResp)
	}
	if len(resolvedSN.Items) != 1 || registrationResultForTest(t, resolvedSN.Items[0]).PublicKey != peerPublicKey {
		t.Fatalf("FindPeersBySN = %+v", resolvedSN)
	}

	resolveIMEIResp, err := server.FindPubKeysByIMEI(ctx, adminhttp.FindPubKeysByIMEIRequestObject{
		Tac:    tac,
		Serial: serial,
	})
	if err != nil {
		t.Fatalf("FindPubKeysByIMEI error: %v", err)
	}
	resolvedIMEI, ok := resolveIMEIResp.(adminhttp.FindPubKeysByIMEI200JSONResponse)
	if !ok {
		t.Fatalf("FindPubKeysByIMEI response type = %T", resolveIMEIResp)
	}
	if len(resolvedIMEI.PublicKeys) != 1 || resolvedIMEI.PublicKeys[0] != peerPublicKey {
		t.Fatalf("FindPubKeysByIMEI = %+v", resolvedIMEI)
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
	deletedRegistration := registrationResultForTest(t, adminhttp.PeerRegistrationResult(deleted))
	if deletedRegistration.Role != apitypes.PeerRoleClient || deletedRegistration.Status != apitypes.PeerRegistrationStatusBlocked || deletedRegistration.ApprovedAt == nil {
		t.Fatalf("DeletePeer = %+v", deleted)
	}
	getDeletedResp, err := server.GetPeer(ctx, adminhttp.GetPeerRequestObject{PublicKey: string(peerPublicKey)})
	if err != nil {
		t.Fatalf("GetPeer after DeletePeer error: %v", err)
	}
	if _, ok := getDeletedResp.(adminhttp.GetPeer200JSONResponse); !ok {
		t.Fatalf("GetPeer after DeletePeer response type = %T", getDeletedResp)
	}
	listDeletedResp, err := server.ListPeers(ctx, adminhttp.ListPeersRequestObject{})
	if err != nil {
		t.Fatalf("ListPeers after DeletePeer error: %v", err)
	}
	listedDeleted, ok := listDeletedResp.(adminhttp.ListPeers200JSONResponse)
	if !ok || len(listedDeleted.Items) != 1 || registrationResultForTest(t, listedDeleted.Items[0]).PublicKey != peerPublicKey {
		t.Fatalf("ListPeers after DeletePeer response = %#v", listDeletedResp)
	}
	if pending, err := pendingdeletion.HasLocator(ctx, server.Store, pendingdeletion.KindPeer, peerPublicKey); err != nil || !pending {
		t.Fatalf("peer pending deletion = %v, error = %v", pending, err)
	}
	if response, err := server.FindPeersBySN(ctx, adminhttp.FindPeersBySNRequestObject{Sn: sn}); err != nil {
		t.Fatalf("FindPeersBySN after delete error: %v", err)
	} else if _, ok := response.(adminhttp.FindPeersBySN200JSONResponse); !ok {
		t.Fatalf("FindPeersBySN after delete response = %T", response)
	}
	if response, err := server.FindPubKeysByIMEI(ctx, adminhttp.FindPubKeysByIMEIRequestObject{Tac: tac, Serial: serial}); err != nil {
		t.Fatalf("FindPubKeysByIMEI after delete error: %v", err)
	} else if _, ok := response.(adminhttp.FindPubKeysByIMEI200JSONResponse); !ok {
		t.Fatalf("FindPubKeysByIMEI after delete response = %T", response)
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
		t.Fatalf("BootstrapEdgeNodes() while marked error = %v", err)
	}
}

func TestFindPeersBySNReturnsEveryMatchingPeer(t *testing.T) {
	ctx := context.Background()
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	sn := "shared-sn"
	first := giznet.PublicKey{1}
	second := giznet.PublicKey{2}
	saveTestPeer(t, server, first, apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn}})
	saveTestPeer(t, server, second, apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn}})

	response, err := server.FindPeersBySN(ctx, adminhttp.FindPeersBySNRequestObject{Sn: sn})
	if err != nil {
		t.Fatalf("FindPeersBySN error: %v", err)
	}
	matched, ok := response.(adminhttp.FindPeersBySN200JSONResponse)
	if !ok {
		t.Fatalf("FindPeersBySN response type = %T", response)
	}
	got := make(map[string]bool, len(matched.Items))
	for _, item := range matched.Items {
		got[registrationResultForTest(t, item).PublicKey] = true
	}
	if len(got) != 2 || !got[first.String()] || !got[second.String()] {
		t.Fatalf("FindPeersBySN public keys = %v", got)
	}

	emptyResponse, err := server.FindPeersBySN(ctx, adminhttp.FindPeersBySNRequestObject{Sn: "missing"})
	if err != nil {
		t.Fatalf("FindPeersBySN(missing) error: %v", err)
	}
	empty, ok := emptyResponse.(adminhttp.FindPeersBySN200JSONResponse)
	if !ok || len(empty.Items) != 0 {
		t.Fatalf("FindPeersBySN(missing) = %#v", emptyResponse)
	}
}

func TestFindPeersBySNRejectsInvalidSerialNumber(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	for _, sn := range []string{"", strings.Repeat("s", maxDeviceSNBytes+1), string([]byte{0xff})} {
		response, err := server.FindPeersBySN(context.Background(), adminhttp.FindPeersBySNRequestObject{Sn: sn})
		if err != nil {
			t.Fatalf("FindPeersBySN(%q) error: %v", sn, err)
		}
		invalid, ok := response.(adminhttp.FindPeersBySN400JSONResponse)
		if !ok || invalid.Error.Code != "INVALID_DEVICE_SN" {
			t.Fatalf("FindPeersBySN(%q) response = %#v", sn, response)
		}
	}
}

func TestFindPeersBySNRecoversLegacyIndexCollisions(t *testing.T) {
	ctx := context.Background()
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	sn := "legacy-shared-sn"
	first := giznet.PublicKey{3}
	second := giznet.PublicKey{4}
	saveTestPeer(t, server, first, apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn}})
	saveTestPeer(t, server, second, apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn}})
	if err := server.Store.BatchDelete(ctx, []kv.Key{snKey(sn, first.String()), snKey(sn, second.String())}); err != nil {
		t.Fatalf("delete current indexes: %v", err)
	}
	if err := server.Store.Set(ctx, snPrefix(sn), []byte(second.String())); err != nil {
		t.Fatalf("seed legacy index: %v", err)
	}

	response, err := server.FindPeersBySN(ctx, adminhttp.FindPeersBySNRequestObject{Sn: sn})
	if err != nil {
		t.Fatalf("FindPeersBySN error: %v", err)
	}
	matched, ok := response.(adminhttp.FindPeersBySN200JSONResponse)
	if !ok || len(matched.Items) != 2 {
		t.Fatalf("FindPeersBySN legacy response = %#v", response)
	}
}

func TestSaveRefreshedDeviceFieldsPreservesConcurrentProfileUpdate(t *testing.T) {
	ctx := context.Background()
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	publicKey := giznet.PublicKey{5}
	saveTestPeer(t, server, publicKey, apitypes.DeviceInfo{})
	name := "updated-while-refreshing"
	if _, err := server.PutSelfInfo(ctx, publicKey, apitypes.DeviceInfo{Name: &name}); err != nil {
		t.Fatalf("PutSelfInfo error: %v", err)
	}
	sn := "refreshed-sn"
	saved, err := server.SaveRefreshedDeviceFields(
		ctx,
		publicKey,
		apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn}},
		[]string{"device.identifiers.sn"},
	)
	if err != nil {
		t.Fatalf("SaveRefreshedDeviceFields error: %v", err)
	}
	if saved.Device.Name == nil || *saved.Device.Name != name {
		t.Fatalf("saved profile name = %#v", saved.Device.Name)
	}
	if saved.Device.Identifiers == nil || saved.Device.Identifiers.Sn == nil || *saved.Device.Identifiers.Sn != sn {
		t.Fatalf("saved identifiers = %#v", saved.Device.Identifiers)
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
	if len(listed.Items) != 1 || registrationResultForTest(t, listed.Items[0]).PublicKey != peerAText {
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
	if len(firstPage.Items) != 2 || registrationResultForTest(t, firstPage.Items[0]).PublicKey != peerBText || registrationResultForTest(t, firstPage.Items[1]).PublicKey != peerAText {
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
	if len(secondPage.Items) != 1 || registrationResultForTest(t, secondPage.Items[0]).PublicKey != peerCText {
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
		BuildVersion:    "0.2.5",
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
	if serverInfo.Version != "0.2.5" || serverInfo.BuildCommit != "deadbeef" || serverInfo.PublicKey != server.ServerPublicKey.String() {
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

func TestGetServerInfoDefaultsDevelopmentBuildIdentity(t *testing.T) {
	server := &Server{ServerPublicKey: giznet.PublicKey{1}}
	response, err := server.GetServerInfo(context.Background(), peerhttp.GetServerInfoRequestObject{})
	if err != nil {
		t.Fatalf("GetServerInfo() error = %v", err)
	}
	info, ok := response.(peerhttp.GetServerInfo200JSONResponse)
	if !ok {
		t.Fatalf("GetServerInfo response type = %T", response)
	}
	if info.Version != "dev" || info.BuildCommit != "dev" {
		t.Fatalf("development build identity = version %q commit %q", info.Version, info.BuildCommit)
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

func TestPutSelfInfoPartialUpdate(t *testing.T) {
	peerKey := giznet.PublicKey{5}
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	name := "device-1"
	emoji := "🐈"
	saveTestPeer(t, server, peerKey, apitypes.DeviceInfo{Name: &name, Emoji: &emoji})

	newName := "device-2"
	updated, err := server.PutSelfInfo(context.Background(), peerKey, apitypes.DeviceInfo{Name: &newName})
	if err != nil {
		t.Fatalf("PutSelfInfo(name only) error: %v", err)
	}
	if updated.Name == nil || *updated.Name != newName {
		t.Fatalf("PutSelfInfo(name only) name = %+v, want %q", updated.Name, newName)
	}
	if updated.Emoji == nil || *updated.Emoji != emoji {
		t.Fatalf("PutSelfInfo(name only) emoji = %+v, want %q", updated.Emoji, emoji)
	}

	newEmoji := "🦊"
	updated, err = server.PutSelfInfo(context.Background(), peerKey, apitypes.DeviceInfo{Emoji: &newEmoji})
	if err != nil {
		t.Fatalf("PutSelfInfo(emoji only) error: %v", err)
	}
	if updated.Name == nil || *updated.Name != newName {
		t.Fatalf("PutSelfInfo(emoji only) name = %+v, want %q", updated.Name, newName)
	}
	if updated.Emoji == nil || *updated.Emoji != newEmoji {
		t.Fatalf("PutSelfInfo(emoji only) emoji = %+v, want %q", updated.Emoji, newEmoji)
	}

	updated, err = server.PutSelfInfo(context.Background(), peerKey, apitypes.DeviceInfo{})
	if err != nil {
		t.Fatalf("PutSelfInfo(empty) error: %v", err)
	}
	if updated.Name == nil || *updated.Name != newName || updated.Emoji == nil || *updated.Emoji != newEmoji {
		t.Fatalf("PutSelfInfo(empty) = %+v, want profile unchanged", updated)
	}
}

func TestPutPeerInfoPartialUpdate(t *testing.T) {
	ctx := context.Background()
	peerKey := giznet.PublicKey{6}
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	name := "device-1"
	emoji := "🐈"
	saveTestPeer(t, server, peerKey, apitypes.DeviceInfo{Name: &name, Emoji: &emoji})

	put := func(body adminhttp.PutPeerInfoJSONRequestBody) apitypes.DeviceInfo {
		t.Helper()
		response, err := server.PutPeerInfo(ctx, adminhttp.PutPeerInfoRequestObject{
			PublicKey: peerKey.String(), Body: &body,
		})
		if err != nil {
			t.Fatalf("PutPeerInfo error: %v", err)
		}
		updated, ok := response.(adminhttp.PutPeerInfo200JSONResponse)
		if !ok {
			t.Fatalf("PutPeerInfo response type = %T", response)
		}
		return apitypes.DeviceInfo(updated)
	}

	newName := "device-2"
	renamed := put(adminhttp.PutPeerInfoJSONRequestBody{Name: &newName})
	if renamed.Name == nil || *renamed.Name != newName || renamed.Emoji == nil || *renamed.Emoji != emoji {
		t.Fatalf("PutPeerInfo(name only) = %+v, want emoji %q preserved", renamed, emoji)
	}

	newEmoji := "🦊"
	reemojied := put(adminhttp.PutPeerInfoJSONRequestBody{Emoji: &newEmoji})
	if reemojied.Name == nil || *reemojied.Name != newName || reemojied.Emoji == nil || *reemojied.Emoji != newEmoji {
		t.Fatalf("PutPeerInfo(emoji only) = %+v, want name %q preserved", reemojied, newName)
	}

	unchanged := put(adminhttp.PutPeerInfoJSONRequestBody{})
	if unchanged.Name == nil || *unchanged.Name != newName || unchanged.Emoji == nil || *unchanged.Emoji != newEmoji {
		t.Fatalf("PutPeerInfo(empty) = %+v, want profile unchanged", unchanged)
	}
}
