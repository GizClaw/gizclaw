package social

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
	_ "modernc.org/sqlite"
)

func TestSocialRPCSchemasAreTypedWithoutBody(t *testing.T) {
	data, err := os.ReadFile("../../../api/rpc/server.json")
	if err != nil {
		t.Fatalf("read rpc schema: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal rpc schema: %v", err)
	}
	for _, name := range []string{
		"ContactObject",
		"FriendRequestObject",
		"FriendObject",
		"FriendGroupObject",
		"FriendGroupMemberObject",
		"FriendGroupMessageObject",
	} {
		schema, ok := doc.Components.Schemas[name]
		if !ok {
			t.Fatalf("schema %s is missing", name)
		}
		if _, ok := schema.Properties["body"]; ok {
			t.Fatalf("schema %s unexpectedly has arbitrary body field", name)
		}
	}
}

func TestContactCRUDUsesDirectFieldsAndPerPeerScope(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)

	contact, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{
		DisplayName: strPtr("Alice"),
		PhoneNumber: strPtr("+1 (555) 0100"),
	})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	if got := stringValue(contact.DisplayName); got != "Alice" {
		t.Fatalf("display_name = %q", got)
	}
	if got := stringValue(contact.PhoneNumber); got != "+1 (555) 0100" {
		t.Fatalf("phone_number = %q", got)
	}

	if _, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{PhoneNumber: strPtr("15550100")}); err == nil {
		t.Fatal("CreateContact duplicate phone_number error = nil")
	}
	if _, err := s.CreateContact(ctx, "peer-b", rpcapi.ContactCreateRequest{PhoneNumber: strPtr("15550100")}); err != nil {
		t.Fatalf("CreateContact same phone for another peer: %v", err)
	}

	updated, err := s.PutContact(ctx, "peer-a", rpcapi.ContactPutRequest{
		Id:          contactID(contact),
		DisplayName: strPtr("Alice Zhang"),
		PhoneNumber: strPtr("+1 555 0101"),
	})
	if err != nil {
		t.Fatalf("PutContact: %v", err)
	}
	if got := stringValue(updated.DisplayName); got != "Alice Zhang" {
		t.Fatalf("updated display_name = %q", got)
	}
	phoneOnly, err := s.PutContact(ctx, "peer-a", rpcapi.ContactPutRequest{
		Id:          contactID(contact),
		PhoneNumber: strPtr("+1 555 0102"),
	})
	if err != nil {
		t.Fatalf("PutContact phone only: %v", err)
	}
	if got := stringValue(phoneOnly.DisplayName); got != "Alice Zhang" {
		t.Fatalf("phone-only PutContact display_name = %q, want previous value", got)
	}
	if _, err := s.PutContact(ctx, "peer-a", rpcapi.ContactPutRequest{
		Id:          contactID(contact),
		DisplayName: strPtr(""),
		PhoneNumber: strPtr(""),
	}); err == nil {
		t.Fatal("PutContact clearing all fields error = nil")
	}

	list, err := s.ListContacts(ctx, "peer-a", rpcapi.ContactListRequest{})
	if err != nil {
		t.Fatalf("ListContacts: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("ListContacts len = %d, want 1", len(list.Items))
	}
}

func TestContactDuplicatePhoneScansBeyondFirstPage(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	nextID := 0
	s.NewID = func() string {
		nextID++
		return fmt.Sprintf("contact-%03d", nextID)
	}

	var lastPhone string
	for i := range maxListLimit + 1 {
		lastPhone = fmt.Sprintf("+1 555 9%03d", i)
		if _, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{
			DisplayName: strPtr(fmt.Sprintf("Contact %03d", i)),
			PhoneNumber: strPtr(lastPhone),
		}); err != nil {
			t.Fatalf("CreateContact %d: %v", i, err)
		}
	}
	if _, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{PhoneNumber: strPtr(lastPhone)}); err == nil {
		t.Fatal("CreateContact duplicate phone beyond first page error = nil")
	}
}

func TestFriendRequestRequiresDeviceOTPAndCreatesSymmetricFriend(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)

	if _, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "bad"}); err == nil {
		t.Fatal("CreateFriendRequest malformed code error = nil")
	}
	if err := s.ReportFriendOTP(ctx, "peer-b", "123456"); err != nil {
		t.Fatalf("ReportFriendOTP: %v", err)
	}
	if _, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "000000"}); err == nil {
		t.Fatal("CreateFriendRequest wrong code error = nil")
	}
	req, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{
		ToPeerId: "peer-b",
		Code:     "123456",
		Message:  strPtr("hi"),
	})
	if err != nil {
		t.Fatalf("CreateFriendRequest: %v", err)
	}
	if req.State == nil || *req.State != rpcapi.FriendRequestStatePending {
		t.Fatalf("friend request state = %v, want pending", req.State)
	}
	duplicate, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "123456"})
	if err != nil {
		t.Fatalf("CreateFriendRequest duplicate pending: %v", err)
	}
	if stringValue(duplicate.Id) != stringValue(req.Id) {
		t.Fatalf("duplicate pending request id = %q, want %q", stringValue(duplicate.Id), stringValue(req.Id))
	}
	if _, err := s.CreateFriendRequest(ctx, "peer-x", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "123456"}); err == nil {
		t.Fatal("CreateFriendRequest consumed code for different requester error = nil")
	}
	if err := s.ReportFriendOTP(ctx, "peer-c", "333333"); err != nil {
		t.Fatalf("ReportFriendOTP expired: %v", err)
	}
	s.Now = func() time.Time { return time.Date(2026, 6, 13, 0, 11, 0, 0, time.UTC) }
	if _, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-c", Code: "333333"}); err == nil {
		t.Fatal("CreateFriendRequest expired code error = nil")
	}
	s.Now = func() time.Time { return time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC) }

	accepted, err := s.AcceptFriendRequest(ctx, "peer-b", rpcapi.FriendRequestAcceptRequest{Id: stringValue(req.Id)})
	if err != nil {
		t.Fatalf("AcceptFriendRequest: %v", err)
	}
	if accepted.State == nil || *accepted.State != rpcapi.FriendRequestStateAccepted {
		t.Fatalf("accepted state = %v", accepted.State)
	}
	acceptedAgain, err := s.AcceptFriendRequest(ctx, "peer-b", rpcapi.FriendRequestAcceptRequest{Id: stringValue(req.Id)})
	if err != nil {
		t.Fatalf("AcceptFriendRequest accepted request: %v", err)
	}
	if stringValue(acceptedAgain.Id) != stringValue(req.Id) || acceptedAgain.State == nil || *acceptedAgain.State != rpcapi.FriendRequestStateAccepted {
		t.Fatalf("accepted again = %#v, want same accepted request", acceptedAgain)
	}
	for _, peer := range []string{"peer-a", "peer-b"} {
		friends, err := s.ListFriends(ctx, peer, rpcapi.FriendListRequest{})
		if err != nil {
			t.Fatalf("ListFriends(%s): %v", peer, err)
		}
		if len(friends.Items) != 1 {
			t.Fatalf("ListFriends(%s) len = %d, want 1", peer, len(friends.Items))
		}
	}
	if err := s.ReportFriendOTP(ctx, "peer-b", "444444"); err != nil {
		t.Fatalf("ReportFriendOTP already friends: %v", err)
	}
	if _, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "444444"}); err == nil {
		t.Fatal("CreateFriendRequest already friends error = nil")
	}
}

func TestAcceptFriendRequestKeepsPendingWhenFriendRowsFail(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	if err := s.ReportFriendOTP(ctx, "peer-b", "123456"); err != nil {
		t.Fatalf("ReportFriendOTP: %v", err)
	}
	req, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "123456"})
	if err != nil {
		t.Fatalf("CreateFriendRequest: %v", err)
	}
	s.Friends = failingBatchSetStore{Store: kv.NewMemory(nil)}
	if _, err := s.AcceptFriendRequest(ctx, "peer-b", rpcapi.FriendRequestAcceptRequest{Id: stringValue(req.Id)}); err == nil {
		t.Fatal("AcceptFriendRequest with failing friend store error = nil")
	}
	requests, err := s.ListFriendRequests(ctx, "peer-b", rpcapi.FriendRequestListRequest{State: friendRequestStatePtr(rpcapi.FriendRequestStatePending)})
	if err != nil {
		t.Fatalf("ListFriendRequests pending: %v", err)
	}
	if len(requests.Items) != 1 || stringValue(requests.Items[0].Id) != stringValue(req.Id) {
		t.Fatalf("pending requests after failed accept = %#v, want original request", requests.Items)
	}
}

func TestFriendGroupRolesAudioMessagesAndTTL(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	s.MessageDefaultTTL = time.Second
	s.MessageMaxAudioBytes = 16

	if err := s.ReportFriendOTP(ctx, "peer-b", "222222"); err != nil {
		t.Fatalf("ReportFriendOTP: %v", err)
	}
	req, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "222222"})
	if err != nil {
		t.Fatalf("CreateFriendRequest: %v", err)
	}
	if _, err := s.AcceptFriendRequest(ctx, "peer-b", rpcapi.FriendRequestAcceptRequest{Id: stringValue(req.Id)}); err != nil {
		t.Fatalf("AcceptFriendRequest: %v", err)
	}

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := stringValue(group.Id)
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerId: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err != nil {
		t.Fatalf("AddFriendGroupMember member: %v", err)
	}
	if _, err := s.PutFriendGroupMember(ctx, "peer-b", rpcapi.FriendGroupMemberPutRequest{FriendGroupId: friendGroupID, Id: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("admin")}); err == nil {
		t.Fatal("PutFriendGroupMember by member error = nil")
	}
	if _, err := s.PutFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberPutRequest{FriendGroupId: friendGroupID, Id: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("admin")}); err != nil {
		t.Fatalf("PutFriendGroupMember by owner: %v", err)
	}
	if _, err := s.AddFriendGroupMember(ctx, "peer-b", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerId: "peer-c", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err != nil {
		t.Fatalf("AddFriendGroupMember by admin: %v", err)
	}
	if _, err := s.AddFriendGroupMember(ctx, "peer-b", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerId: "peer-d", Role: rpcapi.FriendGroupMemberMutableRole("admin")}); err == nil {
		t.Fatal("admin adding admin error = nil")
	}
	if _, err := s.GetFriendGroup(ctx, "peer-d", rpcapi.FriendGroupGetRequest{Id: friendGroupID}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("GetFriendGroup by non-member error = %v, want kv.ErrNotFound", err)
	}

	msg, err := s.SendFriendGroupMessage(ctx, "peer-b", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    " " + friendGroupID + " ",
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
	})
	if err != nil {
		t.Fatalf("SendFriendGroupMessage: %v", err)
	}
	if msg.AudioPath == nil || strings.Contains(*msg.AudioPath, "..") || filepath.IsAbs(*msg.AudioPath) {
		t.Fatalf("audio_path = %v", msg.AudioPath)
	}
	rc, err := s.MessageAssets.Get(stringValue(msg.AudioPath))
	if err != nil {
		t.Fatalf("Get audio object: %v", err)
	}
	data, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(data) != "opus" {
		t.Fatalf("audio bytes = %q", data)
	}
	if _, err := s.SendFriendGroupMessage(ctx, "peer-b", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("0123456789abcdefg"),
		AudioContentType: "audio/opus",
	}); err == nil {
		t.Fatal("oversized SendFriendGroupMessage error = nil")
	}
	if _, err := s.GetFriendGroupMessage(ctx, "peer-c", rpcapi.FriendGroupMessageGetRequest{FriendGroupId: friendGroupID, Id: stringValue(msg.Id)}); err != nil {
		t.Fatalf("GetFriendGroupMessage by member: %v", err)
	}
	if _, err := s.GetFriendGroupMessage(ctx, "peer-d", rpcapi.FriendGroupMessageGetRequest{FriendGroupId: friendGroupID, Id: stringValue(msg.Id)}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("GetFriendGroupMessage by non-member error = %v, want kv.ErrNotFound", err)
	}
	if _, err := s.SendFriendGroupMessage(ctx, "peer-d", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
	}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("SendFriendGroupMessage by non-member error = %v, want kv.ErrNotFound", err)
	}

	s.Now = func() time.Time { return time.Date(2026, 6, 13, 0, 0, 2, 0, time.UTC) }
	if _, err := s.GetFriendGroupMessage(ctx, "peer-c", rpcapi.FriendGroupMessageGetRequest{FriendGroupId: friendGroupID, Id: stringValue(msg.Id)}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("GetFriendGroupMessage expired error = %v, want kv.ErrNotFound", err)
	}
	if err := s.CleanupExpiredFriendGroupMessages(ctx); err != nil {
		t.Fatalf("CleanupExpiredFriendGroupMessages: %v", err)
	}
	if _, err := s.MessageAssets.Get(stringValue(msg.AudioPath)); err == nil {
		t.Fatal("expired audio object still exists")
	}
}

func TestFriendGroupMembersMaintainACLBindings(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	s.ACL = newTestACL(t)

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := stringValue(group.Id)
	if err := s.ACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-a"),
		Resource:   acl.FriendGroupResource(friendGroupID),
		Permission: apitypes.ACLPermissionFriendGroupAdmin,
	}); err != nil {
		t.Fatalf("owner friend group admin authorize: %v", err)
	}

	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerId: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err != nil {
		t.Fatalf("AddFriendGroupMember: %v", err)
	}
	if err := s.ACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-b"),
		Resource:   acl.FriendGroupResource(friendGroupID),
		Permission: apitypes.ACLPermissionFriendGroupUse,
	}); err != nil {
		t.Fatalf("member group use authorize: %v", err)
	}
	if err := s.ACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-b"),
		Resource:   acl.FriendGroupResource(friendGroupID),
		Permission: apitypes.ACLPermissionFriendGroupAdmin,
	}); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("member friend group admin authorize error = %v, want denied", err)
	}

	if _, err := s.PutFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberPutRequest{FriendGroupId: friendGroupID, Id: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("admin")}); err != nil {
		t.Fatalf("PutFriendGroupMember: %v", err)
	}
	if err := s.ACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-b"),
		Resource:   acl.FriendGroupResource(friendGroupID),
		Permission: apitypes.ACLPermissionFriendGroupAdmin,
	}); err != nil {
		t.Fatalf("admin friend group admin authorize: %v", err)
	}

	if _, err := s.DeleteFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: friendGroupID, Id: "peer-b"}); err != nil {
		t.Fatalf("DeleteFriendGroupMember: %v", err)
	}
	if err := s.ACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-b"),
		Resource:   acl.FriendGroupResource(friendGroupID),
		Permission: apitypes.ACLPermissionFriendGroupUse,
	}); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("deleted member group use authorize error = %v, want denied", err)
	}
}

func TestFriendGroupMemberRollsBackWhenACLWriteFails(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	baseACL := newTestACL(t)
	s.ACL = baseACL

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := stringValue(group.Id)

	s.ACL = failingFriendGroupACL{FriendGroupACL: baseACL, failPut: true}
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerId: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err == nil {
		t.Fatal("AddFriendGroupMember with failing ACL error = nil")
	}
	if _, err := s.groupMember(ctx, friendGroupID, "peer-b"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("member after failed add error = %v, want not found", err)
	}

	s.ACL = baseACL
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerId: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err != nil {
		t.Fatalf("AddFriendGroupMember: %v", err)
	}

	s.ACL = failingFriendGroupACL{FriendGroupACL: baseACL, failPut: true}
	if _, err := s.PutFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberPutRequest{FriendGroupId: friendGroupID, Id: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("admin")}); err == nil {
		t.Fatal("PutFriendGroupMember with failing ACL error = nil")
	}
	member, err := s.groupMember(ctx, friendGroupID, "peer-b")
	if err != nil {
		t.Fatalf("groupMember after failed put: %v", err)
	}
	if groupRole(member) != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("member role after failed put = %s, want member", groupRole(member))
	}

	s.ACL = failingFriendGroupACL{FriendGroupACL: baseACL, failDelete: true}
	if _, err := s.DeleteFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: friendGroupID, Id: "peer-b"}); err == nil {
		t.Fatal("DeleteFriendGroupMember with failing ACL error = nil")
	}
	if _, err := s.groupMember(ctx, friendGroupID, "peer-b"); err != nil {
		t.Fatalf("groupMember after failed delete = %v, want preserved", err)
	}
}

func TestCRUDDeletePathsAndFriendGroupLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	s.ACL = newTestACL(t)

	contact, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{DisplayName: strPtr("Alice")})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	gotContact, err := s.GetContact(ctx, "peer-a", rpcapi.ContactGetRequest{Id: contactID(contact)})
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if stringValue(gotContact.Id) != contactID(contact) {
		t.Fatalf("GetContact id = %q, want %q", stringValue(gotContact.Id), contactID(contact))
	}
	deletedContact, err := s.DeleteContact(ctx, "peer-a", rpcapi.ContactDeleteRequest{Id: contactID(contact)})
	if err != nil {
		t.Fatalf("DeleteContact: %v", err)
	}
	if stringValue(deletedContact.Id) != contactID(contact) {
		t.Fatalf("DeleteContact id = %q, want %q", stringValue(deletedContact.Id), contactID(contact))
	}

	if err := s.ReportFriendOTP(ctx, "peer-b", "101010"); err != nil {
		t.Fatalf("ReportFriendOTP reject: %v", err)
	}
	rejectedReq, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "101010"})
	if err != nil {
		t.Fatalf("CreateFriendRequest reject: %v", err)
	}
	rejectedReq, err = s.RejectFriendRequest(ctx, "peer-b", rpcapi.FriendRequestRejectRequest{Id: stringValue(rejectedReq.Id)})
	if err != nil {
		t.Fatalf("RejectFriendRequest: %v", err)
	}
	if rejectedReq.State == nil || *rejectedReq.State != rpcapi.FriendRequestStateRejected {
		t.Fatalf("rejected state = %v, want rejected", rejectedReq.State)
	}

	if err := s.ReportFriendOTP(ctx, "peer-b", "202020"); err != nil {
		t.Fatalf("ReportFriendOTP accept: %v", err)
	}
	acceptedReq, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "202020"})
	if err != nil {
		t.Fatalf("CreateFriendRequest accept: %v", err)
	}
	if _, err := s.AcceptFriendRequest(ctx, "peer-b", rpcapi.FriendRequestAcceptRequest{Id: stringValue(acceptedReq.Id)}); err != nil {
		t.Fatalf("AcceptFriendRequest: %v", err)
	}
	deletedFriend, err := s.DeleteFriend(ctx, "peer-a", rpcapi.FriendDeleteRequest{Id: relationID("peer-a", "peer-b")})
	if err != nil {
		t.Fatalf("DeleteFriend: %v", err)
	}
	if stringValue(deletedFriend.PeerId) != "peer-b" {
		t.Fatalf("DeleteFriend peer_id = %q, want peer-b", stringValue(deletedFriend.PeerId))
	}
	peerBFriends, err := s.ListFriends(ctx, "peer-b", rpcapi.FriendListRequest{})
	if err != nil {
		t.Fatalf("ListFriends peer-b: %v", err)
	}
	if len(peerBFriends.Items) != 0 {
		t.Fatalf("peer-b friends after delete = %#v, want none", peerBFriends.Items)
	}

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := stringValue(group.Id)
	group, err = s.PutFriendGroup(ctx, "peer-a", rpcapi.FriendGroupPutRequest{Id: friendGroupID, Name: strPtr("renamed")})
	if err != nil {
		t.Fatalf("PutFriendGroup: %v", err)
	}
	if stringValue(group.Name) != "renamed" {
		t.Fatalf("PutFriendGroup name = %q, want renamed", stringValue(group.Name))
	}
	if _, err := s.PutFriendGroup(ctx, "peer-a", rpcapi.FriendGroupPutRequest{Id: friendGroupID, Name: strPtr(" ")}); err == nil {
		t.Fatal("PutFriendGroup empty name error = nil")
	}
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerId: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err != nil {
		t.Fatalf("AddFriendGroupMember: %v", err)
	}
	members, err := s.ListFriendGroupMembers(ctx, "peer-a", rpcapi.FriendGroupMemberListRequest{FriendGroupId: &friendGroupID, Limit: intPtr(1)})
	if err != nil {
		t.Fatalf("ListFriendGroupMembers: %v", err)
	}
	if len(members.Items) != 1 || !members.HasNext {
		t.Fatalf("ListFriendGroupMembers = %#v, want first page with next", members)
	}
	msg, err := s.SendFriendGroupMessage(ctx, "peer-b", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
	})
	if err != nil {
		t.Fatalf("SendFriendGroupMessage before delete: %v", err)
	}

	deletedFriendGroup, err := s.DeleteFriendGroup(ctx, "peer-a", rpcapi.FriendGroupDeleteRequest{Id: friendGroupID})
	if err != nil {
		t.Fatalf("DeleteFriendGroup: %v", err)
	}
	if stringValue(deletedFriendGroup.Id) != friendGroupID {
		t.Fatalf("DeleteFriendGroup id = %q, want %q", stringValue(deletedFriendGroup.Id), friendGroupID)
	}
	if _, err := s.MessageAssets.Get(stringValue(msg.AudioPath)); err == nil {
		t.Fatal("group audio object still exists after group delete")
	}
}

func TestFriendGroupMemberDeleteRoleRules(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := stringValue(group.Id)
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerId: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err != nil {
		t.Fatalf("AddFriendGroupMember peer-b: %v", err)
	}
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerId: "peer-c", Role: rpcapi.FriendGroupMemberMutableRole("admin")}); err != nil {
		t.Fatalf("AddFriendGroupMember peer-c admin: %v", err)
	}
	if _, err := s.DeleteFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: friendGroupID, Id: "peer-a"}); err == nil {
		t.Fatal("DeleteFriendGroupMember owner error = nil")
	}
	if _, err := s.DeleteFriendGroupMember(ctx, "peer-b", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: friendGroupID, Id: "peer-c"}); err == nil {
		t.Fatal("DeleteFriendGroupMember admin by member error = nil")
	}
	deletedAdmin, err := s.DeleteFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: friendGroupID, Id: "peer-c"})
	if err != nil {
		t.Fatalf("DeleteFriendGroupMember admin by owner: %v", err)
	}
	if stringValue(deletedAdmin.PeerId) != "peer-c" {
		t.Fatalf("deleted admin peer_id = %q, want peer-c", stringValue(deletedAdmin.PeerId))
	}
	selfDeleted, err := s.DeleteFriendGroupMember(ctx, "peer-b", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: friendGroupID, Id: "peer-b"})
	if err != nil {
		t.Fatalf("DeleteFriendGroupMember self member: %v", err)
	}
	if stringValue(selfDeleted.PeerId) != "peer-b" {
		t.Fatalf("self deleted peer_id = %q, want peer-b", stringValue(selfDeleted.PeerId))
	}
}

func TestDeleteFriendGroupClearsACLBindingsBeyondFirstPage(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	s.ACL = newTestACL(t)
	nextID := 0
	s.NewID = func() string {
		nextID++
		return fmt.Sprintf("id-%03d", nextID)
	}

	group, err := s.CreateFriendGroup(ctx, "peer-owner", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := stringValue(group.Id)
	var lastPeer string
	for i := range maxListLimit + 1 {
		lastPeer = fmt.Sprintf("peer-%03d", i)
		if _, err := s.AddFriendGroupMember(ctx, "peer-owner", rpcapi.FriendGroupMemberAddRequest{
			FriendGroupId: friendGroupID,
			PeerId:        lastPeer,
			Role:          rpcapi.FriendGroupMemberMutableRole("member"),
		}); err != nil {
			t.Fatalf("AddFriendGroupMember %d: %v", i, err)
		}
	}
	if err := s.ACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject(lastPeer),
		Resource:   acl.FriendGroupResource(friendGroupID),
		Permission: apitypes.ACLPermissionFriendGroupUse,
	}); err != nil {
		t.Fatalf("last member group use authorize before delete: %v", err)
	}
	if _, err := s.DeleteFriendGroup(ctx, "peer-owner", rpcapi.FriendGroupDeleteRequest{Id: friendGroupID}); err != nil {
		t.Fatalf("DeleteFriendGroup: %v", err)
	}
	if err := s.ACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject(lastPeer),
		Resource:   acl.FriendGroupResource(friendGroupID),
		Permission: apitypes.ACLPermissionFriendGroupUse,
	}); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("last member group use authorize after delete error = %v, want denied", err)
	}
}

func TestServiceConfigurationErrorsAndHelpers(t *testing.T) {
	ctx := context.Background()
	empty := &Server{}
	if _, err := empty.ListContacts(ctx, "peer-a", rpcapi.ContactListRequest{}); err == nil {
		t.Fatal("ListContacts without store error = nil")
	}
	if _, err := empty.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "123456"}); err == nil {
		t.Fatal("CreateFriendRequest without store error = nil")
	}
	if _, err := empty.ListFriends(ctx, "peer-a", rpcapi.FriendListRequest{}); err == nil {
		t.Fatal("ListFriends without store error = nil")
	}
	if _, err := empty.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"}); err == nil {
		t.Fatal("CreateFriendGroup without store error = nil")
	}
	if _, err := empty.ListFriendGroupMembers(ctx, "peer-a", rpcapi.FriendGroupMemberListRequest{FriendGroupId: strPtr("group-a")}); err == nil {
		t.Fatal("ListFriendGroupMembers without store error = nil")
	}
	if _, err := empty.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{FriendGroupId: "group-a", AudioContentType: "audio/opus"}); err == nil {
		t.Fatal("SendFriendGroupMessage without store error = nil")
	}

	a := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	b := a.Add(time.Second)
	if !compareByCreatedAtAsc(a, "a", b, "b") || !compareByCreatedAtAsc(a, "a", a, "b") || compareByCreatedAtAsc(b, "b", a, "a") {
		t.Fatal("compareByCreatedAtAsc returned unexpected ordering")
	}
	if !compareByCreatedAtDesc(b, "b", a, "a") || !compareByCreatedAtDesc(a, "b", a, "a") || compareByCreatedAtDesc(a, "a", b, "b") {
		t.Fatal("compareByCreatedAtDesc returned unexpected ordering")
	}
	if role := groupRole(rpcapi.FriendGroupMemberObject{}); role != "" {
		t.Fatalf("groupRole without role = %q, want empty", role)
	}
	if _, _, err := groupACLRole("bogus"); err == nil {
		t.Fatal("groupACLRole invalid role error = nil")
	}
	s := newTestServer(t)
	if _, err := s.ListFriendRequests(ctx, "peer-a", rpcapi.FriendRequestListRequest{Box: friendRequestBoxPtr(rpcapi.FriendRequestBox("bogus"))}); err == nil {
		t.Fatal("ListFriendRequests invalid box error = nil")
	}
	bogusState := rpcapi.FriendRequestState("bogus")
	if _, err := s.ListFriendRequests(ctx, "peer-a", rpcapi.FriendRequestListRequest{State: &bogusState}); err == nil {
		t.Fatal("ListFriendRequests invalid state error = nil")
	}
	randomID := (&Server{}).newID()
	if randomID == "" {
		t.Fatal("newID without override returned empty string")
	}
}

func TestFriendGroupCreateRollsBackPartialWrites(t *testing.T) {
	ctx := context.Background()
	groupStore := kv.NewMemory(nil)
	s := newTestServer(t)
	s.FriendGroups = groupStore
	s.FriendGroupMembers = failingSetStore{Store: kv.NewMemory(nil)}

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err == nil {
		t.Fatal("CreateFriendGroup with failing member store error = nil")
	}
	if stringValue(group.Id) != "" {
		t.Fatalf("CreateFriendGroup returned partial group = %#v", group)
	}
	var groups []kv.Entry
	for entry, err := range groupStore.List(ctx, groupsRoot) {
		if err != nil {
			t.Fatalf("list groups after rollback: %v", err)
		}
		groups = append(groups, entry)
	}
	if len(groups) != 0 {
		t.Fatalf("groups after rollback = %#v, want empty", groups)
	}
}

func TestDeleteFriendGroupPropagatesCleanupErrors(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	s.ACL = newTestACL(t)
	baseAssets := s.MessageAssets
	s.MessageAssets = failingDeletePrefixStore{ObjectStore: baseAssets}

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := stringValue(group.Id)
	if _, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
	}); err != nil {
		t.Fatalf("SendFriendGroupMessage: %v", err)
	}
	if err := s.ACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-a"),
		Resource:   acl.FriendGroupResource(friendGroupID),
		Permission: apitypes.ACLPermissionFriendGroupAdmin,
	}); err != nil {
		t.Fatalf("owner friend group admin authorize before delete: %v", err)
	}
	if _, err := s.DeleteFriendGroup(ctx, "peer-a", rpcapi.FriendGroupDeleteRequest{Id: friendGroupID}); err == nil {
		t.Fatal("DeleteFriendGroup with failing asset cleanup error = nil")
	}
	if _, err := s.GetFriendGroup(ctx, "peer-a", rpcapi.FriendGroupGetRequest{Id: friendGroupID}); err != nil {
		t.Fatalf("GetFriendGroup after failed delete = %v, want group preserved", err)
	}
	if err := s.ACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject("peer-a"),
		Resource:   acl.FriendGroupResource(friendGroupID),
		Permission: apitypes.ACLPermissionFriendGroupAdmin,
	}); err != nil {
		t.Fatalf("owner friend group admin authorize after failed delete = %v, want ACL preserved", err)
	}
}

func TestSendFriendGroupMessageDeletesObjectWhenMetadataWriteFails(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	baseAssets := s.MessageAssets
	s.FriendGroupMessages = failingSetStore{Store: kv.NewMemory(nil)}

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	if _, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    stringValue(group.Id),
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
	}); err == nil {
		t.Fatal("SendFriendGroupMessage with failing metadata store error = nil")
	}
	objects, err := baseAssets.List("")
	if err != nil {
		t.Fatalf("List message assets: %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("message assets after failed send = %#v, want empty", objects)
	}
}

func TestFilteredListsPaginateAfterFilteringAndSortNewestFirst(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)

	if err := s.ReportFriendOTP(ctx, "peer-y", "111111"); err != nil {
		t.Fatalf("ReportFriendOTP peer-y: %v", err)
	}
	if _, err := s.CreateFriendRequest(ctx, "peer-x", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-y", Code: "111111"}); err != nil {
		t.Fatalf("CreateFriendRequest unrelated: %v", err)
	}
	if err := s.ReportFriendOTP(ctx, "peer-b", "222222"); err != nil {
		t.Fatalf("ReportFriendOTP peer-b: %v", err)
	}
	visibleReq, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "222222"})
	if err != nil {
		t.Fatalf("CreateFriendRequest visible: %v", err)
	}
	reqs, err := s.ListFriendRequests(ctx, "peer-b", rpcapi.FriendRequestListRequest{Box: friendRequestBoxPtr(rpcapi.FriendRequestBoxIncoming), Limit: intPtr(1)})
	if err != nil {
		t.Fatalf("ListFriendRequests: %v", err)
	}
	if len(reqs.Items) != 1 || stringValue(reqs.Items[0].Id) != stringValue(visibleReq.Id) || reqs.HasNext {
		t.Fatalf("ListFriendRequests page = %#v, want only visible request without next page", reqs)
	}
	if _, err := s.AcceptFriendRequest(ctx, "peer-b", rpcapi.FriendRequestAcceptRequest{Id: stringValue(visibleReq.Id)}); err != nil {
		t.Fatalf("AcceptFriendRequest: %v", err)
	}

	if _, err := s.CreateFriendGroup(ctx, "peer-x", rpcapi.FriendGroupCreateRequest{Name: "other"}); err != nil {
		t.Fatalf("CreateFriendGroup unrelated: %v", err)
	}
	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup visible: %v", err)
	}
	friendGroups, err := s.ListFriendGroups(ctx, "peer-a", rpcapi.FriendGroupListRequest{Limit: intPtr(1)})
	if err != nil {
		t.Fatalf("ListFriendGroups: %v", err)
	}
	if len(friendGroups.Items) != 1 || stringValue(friendGroups.Items[0].Id) != stringValue(group.Id) || friendGroups.HasNext {
		t.Fatalf("ListFriendGroups page = %#v, want only visible group without next page", friendGroups)
	}

	olderMessage, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    stringValue(group.Id),
		AudioBase64:      []byte("old"),
		AudioContentType: "audio/opus",
	})
	if err != nil {
		t.Fatalf("SendFriendGroupMessage older: %v", err)
	}
	newerMessage, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    stringValue(group.Id),
		AudioBase64:      []byte("new"),
		AudioContentType: "audio/opus",
	})
	if err != nil {
		t.Fatalf("SendFriendGroupMessage newer: %v", err)
	}
	messages, err := s.ListFriendGroupMessages(ctx, "peer-a", rpcapi.FriendGroupMessageListRequest{FriendGroupId: group.Id, Limit: intPtr(1)})
	if err != nil {
		t.Fatalf("ListFriendGroupMessages first page: %v", err)
	}
	if len(messages.Items) != 1 || stringValue(messages.Items[0].Id) != stringValue(newerMessage.Id) || !messages.HasNext || messages.NextCursor == nil {
		t.Fatalf("ListFriendGroupMessages first page = %#v, want newest message and next cursor", messages)
	}
	messages, err = s.ListFriendGroupMessages(ctx, "peer-a", rpcapi.FriendGroupMessageListRequest{FriendGroupId: group.Id, Limit: intPtr(1), Cursor: messages.NextCursor})
	if err != nil {
		t.Fatalf("ListFriendGroupMessages second page: %v", err)
	}
	if len(messages.Items) != 1 || stringValue(messages.Items[0].Id) != stringValue(olderMessage.Id) || messages.HasNext {
		t.Fatalf("ListFriendGroupMessages second page = %#v, want older message without next page", messages)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	store := kv.NewMemory(nil)
	now := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	nextID := 0
	return &Server{
		Contacts:            store,
		FriendRequests:      store,
		Friends:             store,
		FriendGroups:        store,
		FriendGroupMembers:  store,
		FriendGroupMessages: store,
		MessageAssets:       objectstore.Dir(t.TempDir()),
		Now:                 func() time.Time { return now },
		NewID: func() string {
			nextID++
			return "id-" + string(rune('a'+nextID-1))
		},
	}
}

type failingSetStore struct {
	kv.Store
}

func (s failingSetStore) Set(context.Context, kv.Key, []byte) error {
	return errors.New("forced set failure")
}

type failingBatchSetStore struct {
	kv.Store
}

func (s failingBatchSetStore) BatchSet(context.Context, []kv.Entry) error {
	return errors.New("forced batch set failure")
}

type failingDeletePrefixStore struct {
	objectstore.ObjectStore
}

func (s failingDeletePrefixStore) DeletePrefix(string) error {
	return errors.New("forced delete prefix failure")
}

type failingFriendGroupACL struct {
	FriendGroupACL
	failPut    bool
	failDelete bool
}

func (a failingFriendGroupACL) PutPolicyBinding(ctx context.Context, id string, priority float64, policy apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error) {
	if a.failPut {
		return apitypes.ACLPolicyBinding{}, errors.New("forced put policy binding failure")
	}
	return a.FriendGroupACL.PutPolicyBinding(ctx, id, priority, policy)
}

func (a failingFriendGroupACL) DeletePolicyBinding(ctx context.Context, id string) (apitypes.ACLPolicyBinding, error) {
	if a.failDelete {
		return apitypes.ACLPolicyBinding{}, errors.New("forced delete policy binding failure")
	}
	return a.FriendGroupACL.DeletePolicyBinding(ctx, id)
}

func newTestACL(t *testing.T) *acl.Server {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	server := &acl.Server{DB: db}
	if err := server.Migration(context.Background()); err != nil {
		t.Fatalf("acl migration: %v", err)
	}
	return server
}

func strPtr(v string) *string {
	return &v
}

func friendRequestBoxPtr(v rpcapi.FriendRequestBox) *rpcapi.FriendRequestBox {
	return &v
}

func friendRequestStatePtr(v rpcapi.FriendRequestState) *rpcapi.FriendRequestState {
	return &v
}

func contactID(item rpcapi.ContactObject) string {
	if item.Id == nil {
		return ""
	}
	return *item.Id
}
