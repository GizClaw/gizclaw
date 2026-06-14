package friendgroup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
	_ "modernc.org/sqlite"
)

func TestRolesAudioMessagesAndTTL(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	s.MessageDefaultTTL = time.Second
	s.MessageMaxAudioBytes = 16

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
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
	rc, err := s.MessageAssets.Get(socialutil.StringValue(msg.AudioPath))
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
	if _, err := s.GetFriendGroupMessage(ctx, "peer-c", rpcapi.FriendGroupMessageGetRequest{FriendGroupId: friendGroupID, Id: socialutil.StringValue(msg.Id)}); err != nil {
		t.Fatalf("GetFriendGroupMessage by member: %v", err)
	}
	if _, err := s.SendFriendGroupMessage(ctx, "peer-d", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
	}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("SendFriendGroupMessage by non-member error = %v, want kv.ErrNotFound", err)
	}

	s.Now = func() time.Time { return time.Date(2026, 6, 13, 0, 0, 2, 0, time.UTC) }
	if _, err := s.GetFriendGroupMessage(ctx, "peer-c", rpcapi.FriendGroupMessageGetRequest{FriendGroupId: friendGroupID, Id: socialutil.StringValue(msg.Id)}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("GetFriendGroupMessage expired error = %v, want kv.ErrNotFound", err)
	}
	if err := s.CleanupExpiredFriendGroupMessages(ctx); err != nil {
		t.Fatalf("CleanupExpiredFriendGroupMessages: %v", err)
	}
	if _, err := s.MessageAssets.Get(socialutil.StringValue(msg.AudioPath)); err == nil {
		t.Fatal("expired audio object still exists")
	}
}

func TestMembersMaintainACLBindings(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	s.ACL = newTestACL(t)

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
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

func TestMemberRollsBackWhenACLWriteFails(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	baseACL := newTestACL(t)
	s.ACL = baseACL

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)

	s.ACL = failingACL{ACL: baseACL, failPut: true}
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

	s.ACL = failingACL{ACL: baseACL, failPut: true}
	if _, err := s.PutFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberPutRequest{FriendGroupId: friendGroupID, Id: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("admin")}); err == nil {
		t.Fatal("PutFriendGroupMember with failing ACL error = nil")
	}
	member, err := s.groupMember(ctx, friendGroupID, "peer-b")
	if err != nil {
		t.Fatalf("groupMember after failed put: %v", err)
	}
	if socialutil.GroupRole(member) != rpcapi.FriendGroupMemberRoleMember {
		t.Fatalf("member role after failed put = %s, want member", socialutil.GroupRole(member))
	}

	s.ACL = failingACL{ACL: baseACL, failDelete: true}
	if _, err := s.DeleteFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: friendGroupID, Id: "peer-b"}); err == nil {
		t.Fatal("DeleteFriendGroupMember with failing ACL error = nil")
	}
	if _, err := s.groupMember(ctx, friendGroupID, "peer-b"); err != nil {
		t.Fatalf("groupMember after failed delete = %v, want preserved", err)
	}
}

func TestLifecycleDeletePathsAndPagination(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	s.ACL = newTestACL(t)

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
	group, err = s.PutFriendGroup(ctx, "peer-a", rpcapi.FriendGroupPutRequest{Id: friendGroupID, Name: strPtr("renamed")})
	if err != nil {
		t.Fatalf("PutFriendGroup: %v", err)
	}
	if socialutil.StringValue(group.Name) != "renamed" {
		t.Fatalf("PutFriendGroup name = %q, want renamed", socialutil.StringValue(group.Name))
	}
	if _, err := s.PutFriendGroup(ctx, "peer-a", rpcapi.FriendGroupPutRequest{Id: friendGroupID, Name: strPtr(" ")}); err == nil {
		t.Fatal("PutFriendGroup empty name error = nil")
	}
	if _, err := s.AddFriendGroupMember(ctx, "peer-a", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: friendGroupID, PeerId: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}); err != nil {
		t.Fatalf("AddFriendGroupMember: %v", err)
	}
	members, err := s.ListFriendGroupMembers(ctx, "peer-a", rpcapi.FriendGroupMemberListRequest{FriendGroupId: &friendGroupID, Limit: socialutil.IntPtr(1)})
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

	deleted, err := s.DeleteFriendGroup(ctx, "peer-a", rpcapi.FriendGroupDeleteRequest{Id: friendGroupID})
	if err != nil {
		t.Fatalf("DeleteFriendGroup: %v", err)
	}
	if socialutil.StringValue(deleted.Id) != friendGroupID {
		t.Fatalf("DeleteFriendGroup id = %q, want %q", socialutil.StringValue(deleted.Id), friendGroupID)
	}
	if _, err := s.MessageAssets.Get(socialutil.StringValue(msg.AudioPath)); err == nil {
		t.Fatal("group audio object still exists after group delete")
	}
}

func TestMemberDeleteRoleRules(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
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
	if socialutil.StringValue(deletedAdmin.PeerId) != "peer-c" {
		t.Fatalf("deleted admin peer_id = %q, want peer-c", socialutil.StringValue(deletedAdmin.PeerId))
	}
	selfDeleted, err := s.DeleteFriendGroupMember(ctx, "peer-b", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: friendGroupID, Id: "peer-b"})
	if err != nil {
		t.Fatalf("DeleteFriendGroupMember self member: %v", err)
	}
	if socialutil.StringValue(selfDeleted.PeerId) != "peer-b" {
		t.Fatalf("self deleted peer_id = %q, want peer-b", socialutil.StringValue(selfDeleted.PeerId))
	}
}

func TestDeleteClearsACLBindingsBeyondFirstPage(t *testing.T) {
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
	friendGroupID := socialutil.StringValue(group.Id)
	var lastPeer string
	for i := range socialutil.MaxListLimit + 1 {
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

func TestConfigurationErrorsAndHelpers(t *testing.T) {
	ctx := context.Background()
	empty := &Server{}
	if _, err := empty.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"}); err == nil {
		t.Fatal("CreateFriendGroup without store error = nil")
	}
	if _, err := empty.ListFriendGroupMembers(ctx, "peer-a", rpcapi.FriendGroupMemberListRequest{FriendGroupId: strPtr("group-a")}); err == nil {
		t.Fatal("ListFriendGroupMembers without store error = nil")
	}
	if _, err := empty.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{FriendGroupId: "group-a", AudioContentType: "audio/opus"}); err == nil {
		t.Fatal("SendFriendGroupMessage without store error = nil")
	}
	s := newTestServer(t)
	if _, err := s.CreateFriendGroup(ctx, "", rpcapi.FriendGroupCreateRequest{Name: "room"}); err == nil {
		t.Fatal("CreateFriendGroup empty owner error = nil")
	}
	if _, err := s.GetFriendGroup(ctx, "peer-a", rpcapi.FriendGroupGetRequest{}); err == nil {
		t.Fatal("GetFriendGroup empty id error = nil")
	}
	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
	if _, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/wav",
	}); err == nil {
		t.Fatal("SendFriendGroupMessage unsupported content type error = nil")
	}
	noAssets := *s
	noAssets.MessageAssets = nil
	if _, err := noAssets.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
	}); err == nil {
		t.Fatal("SendFriendGroupMessage without assets error = nil")
	}
	s.MessageMaxTTL = time.Second
	if _, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
		TtlSeconds:       socialutil.IntPtr(2),
	}); err == nil {
		t.Fatal("SendFriendGroupMessage exceeding max ttl error = nil")
	}
	defaultClock := &Server{Groups: kv.NewMemory(nil), Members: kv.NewMemory(nil), Messages: kv.NewMemory(nil), MessageAssets: objectstore.Dir(t.TempDir())}
	if _, err := defaultClock.CreateFriendGroup(ctx, "peer-z", rpcapi.FriendGroupCreateRequest{Name: "room"}); err != nil {
		t.Fatalf("CreateFriendGroup with default clock: %v", err)
	}

	a := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	b := a.Add(time.Second)
	if !socialutil.CompareByCreatedAtAsc(a, "a", b, "b") || !socialutil.CompareByCreatedAtAsc(a, "a", a, "b") || socialutil.CompareByCreatedAtAsc(b, "b", a, "a") {
		t.Fatal("CompareByCreatedAtAsc returned unexpected ordering")
	}
	if !socialutil.CompareByCreatedAtDesc(b, "b", a, "a") || !socialutil.CompareByCreatedAtDesc(a, "b", a, "a") || socialutil.CompareByCreatedAtDesc(a, "a", b, "b") {
		t.Fatal("CompareByCreatedAtDesc returned unexpected ordering")
	}
	if role := socialutil.GroupRole(rpcapi.FriendGroupMemberObject{}); role != "" {
		t.Fatalf("GroupRole without role = %q, want empty", role)
	}
	if _, _, err := socialutil.GroupACLRole("bogus"); err == nil {
		t.Fatal("GroupACLRole invalid role error = nil")
	}
	if id := (&Server{}).newID(); id == "" {
		t.Fatal("newID without override returned empty string")
	}
}

func TestCreateRollsBackPartialWrites(t *testing.T) {
	ctx := context.Background()
	groupStore := kv.NewMemory(nil)
	s := newTestServer(t)
	s.Groups = groupStore
	s.Members = failingSetStore{Store: kv.NewMemory(nil)}

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err == nil {
		t.Fatal("CreateFriendGroup with failing member store error = nil")
	}
	if socialutil.StringValue(group.Id) != "" {
		t.Fatalf("CreateFriendGroup returned partial group = %#v", group)
	}
	var groups []kv.Entry
	for entry, err := range groupStore.List(ctx, socialutil.GroupsRoot) {
		if err != nil {
			t.Fatalf("list groups after rollback: %v", err)
		}
		groups = append(groups, entry)
	}
	if len(groups) != 0 {
		t.Fatalf("groups after rollback = %#v, want empty", groups)
	}
}

func TestDeletePropagatesCleanupErrors(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	s.ACL = newTestACL(t)
	baseAssets := s.MessageAssets
	s.MessageAssets = failingDeletePrefixStore{ObjectStore: baseAssets}

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	friendGroupID := socialutil.StringValue(group.Id)
	if _, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    friendGroupID,
		AudioBase64:      []byte("opus"),
		AudioContentType: "audio/opus",
	}); err != nil {
		t.Fatalf("SendFriendGroupMessage: %v", err)
	}
	if _, err := s.DeleteFriendGroup(ctx, "peer-a", rpcapi.FriendGroupDeleteRequest{Id: friendGroupID}); err == nil {
		t.Fatal("DeleteFriendGroup with failing asset cleanup error = nil")
	}
	if _, err := s.GetFriendGroup(ctx, "peer-a", rpcapi.FriendGroupGetRequest{Id: friendGroupID}); err != nil {
		t.Fatalf("GetFriendGroup after failed delete = %v, want group preserved", err)
	}
}

func TestSendMessageDeletesObjectWhenMetadataWriteFails(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	baseAssets := s.MessageAssets
	s.Messages = failingSetStore{Store: kv.NewMemory(nil)}

	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup: %v", err)
	}
	if _, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    socialutil.StringValue(group.Id),
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

	if _, err := s.CreateFriendGroup(ctx, "peer-x", rpcapi.FriendGroupCreateRequest{Name: "other"}); err != nil {
		t.Fatalf("CreateFriendGroup unrelated: %v", err)
	}
	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatalf("CreateFriendGroup visible: %v", err)
	}
	friendGroups, err := s.ListFriendGroups(ctx, "peer-a", rpcapi.FriendGroupListRequest{Limit: socialutil.IntPtr(1)})
	if err != nil {
		t.Fatalf("ListFriendGroups: %v", err)
	}
	if len(friendGroups.Items) != 1 || socialutil.StringValue(friendGroups.Items[0].Id) != socialutil.StringValue(group.Id) || friendGroups.HasNext {
		t.Fatalf("ListFriendGroups page = %#v, want only visible group without next page", friendGroups)
	}

	olderMessage, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    socialutil.StringValue(group.Id),
		AudioBase64:      []byte("old"),
		AudioContentType: "audio/opus",
	})
	if err != nil {
		t.Fatalf("SendFriendGroupMessage older: %v", err)
	}
	newerMessage, err := s.SendFriendGroupMessage(ctx, "peer-a", rpcapi.FriendGroupMessageSendRequest{
		FriendGroupId:    socialutil.StringValue(group.Id),
		AudioBase64:      []byte("new"),
		AudioContentType: "audio/opus",
	})
	if err != nil {
		t.Fatalf("SendFriendGroupMessage newer: %v", err)
	}
	messages, err := s.ListFriendGroupMessages(ctx, "peer-a", rpcapi.FriendGroupMessageListRequest{FriendGroupId: group.Id, Limit: socialutil.IntPtr(1)})
	if err != nil {
		t.Fatalf("ListFriendGroupMessages first page: %v", err)
	}
	if len(messages.Items) != 1 || socialutil.StringValue(messages.Items[0].Id) != socialutil.StringValue(newerMessage.Id) || !messages.HasNext || messages.NextCursor == nil {
		t.Fatalf("ListFriendGroupMessages first page = %#v, want newest message and next cursor", messages)
	}
	messages, err = s.ListFriendGroupMessages(ctx, "peer-a", rpcapi.FriendGroupMessageListRequest{FriendGroupId: group.Id, Limit: socialutil.IntPtr(1), Cursor: messages.NextCursor})
	if err != nil {
		t.Fatalf("ListFriendGroupMessages second page: %v", err)
	}
	if len(messages.Items) != 1 || socialutil.StringValue(messages.Items[0].Id) != socialutil.StringValue(olderMessage.Id) || messages.HasNext {
		t.Fatalf("ListFriendGroupMessages second page = %#v, want older message without next page", messages)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	store := kv.NewMemory(nil)
	now := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	nextID := 0
	return &Server{
		Groups:        store,
		Members:       store,
		Messages:      store,
		MessageAssets: objectstore.Dir(t.TempDir()),
		Now:           func() time.Time { return now },
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

type failingDeletePrefixStore struct {
	objectstore.ObjectStore
}

func (s failingDeletePrefixStore) DeletePrefix(string) error {
	return errors.New("forced delete prefix failure")
}

type failingACL struct {
	ACL
	failPut    bool
	failDelete bool
}

func (a failingACL) PutPolicyBinding(ctx context.Context, id string, priority float64, policy apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error) {
	if a.failPut {
		return apitypes.ACLPolicyBinding{}, errors.New("forced put policy binding failure")
	}
	return a.ACL.PutPolicyBinding(ctx, id, priority, policy)
}

func (a failingACL) DeletePolicyBinding(ctx context.Context, id string) (apitypes.ACLPolicyBinding, error) {
	if a.failDelete {
		return apitypes.ACLPolicyBinding{}, errors.New("forced delete policy binding failure")
	}
	return a.ACL.DeletePolicyBinding(ctx, id)
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
