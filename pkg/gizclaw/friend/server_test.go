package friend

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func TestRequestRequiresDeviceOTPAndCreatesSymmetricFriend(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()

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
	if socialutil.StringValue(duplicate.Id) != socialutil.StringValue(req.Id) {
		t.Fatalf("duplicate pending request id = %q, want %q", socialutil.StringValue(duplicate.Id), socialutil.StringValue(req.Id))
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

	accepted, err := s.AcceptFriendRequest(ctx, "peer-b", rpcapi.FriendRequestAcceptRequest{Id: socialutil.StringValue(req.Id)})
	if err != nil {
		t.Fatalf("AcceptFriendRequest: %v", err)
	}
	if accepted.State == nil || *accepted.State != rpcapi.FriendRequestStateAccepted {
		t.Fatalf("accepted state = %v", accepted.State)
	}
	acceptedAgain, err := s.AcceptFriendRequest(ctx, "peer-b", rpcapi.FriendRequestAcceptRequest{Id: socialutil.StringValue(req.Id)})
	if err != nil {
		t.Fatalf("AcceptFriendRequest accepted request: %v", err)
	}
	if socialutil.StringValue(acceptedAgain.Id) != socialutil.StringValue(req.Id) || acceptedAgain.State == nil || *acceptedAgain.State != rpcapi.FriendRequestStateAccepted {
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

func TestAcceptKeepsPendingWhenFriendRowsFail(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()
	if err := s.ReportFriendOTP(ctx, "peer-b", "123456"); err != nil {
		t.Fatalf("ReportFriendOTP: %v", err)
	}
	req, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "123456"})
	if err != nil {
		t.Fatalf("CreateFriendRequest: %v", err)
	}
	s.Friends = failingBatchSetStore{Store: kv.NewMemory(nil)}
	if _, err := s.AcceptFriendRequest(ctx, "peer-b", rpcapi.FriendRequestAcceptRequest{Id: socialutil.StringValue(req.Id)}); err == nil {
		t.Fatal("AcceptFriendRequest with failing friend store error = nil")
	}
	requests, err := s.ListFriendRequests(ctx, "peer-b", rpcapi.FriendRequestListRequest{State: friendRequestStatePtr(rpcapi.FriendRequestStatePending)})
	if err != nil {
		t.Fatalf("ListFriendRequests pending: %v", err)
	}
	if len(requests.Items) != 1 || socialutil.StringValue(requests.Items[0].Id) != socialutil.StringValue(req.Id) {
		t.Fatalf("pending requests after failed accept = %#v, want original request", requests.Items)
	}
}

func TestDeleteAndFilteredLists(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()

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
	reqs, err := s.ListFriendRequests(ctx, "peer-b", rpcapi.FriendRequestListRequest{Box: friendRequestBoxPtr(rpcapi.FriendRequestBoxIncoming), Limit: socialutil.IntPtr(1)})
	if err != nil {
		t.Fatalf("ListFriendRequests: %v", err)
	}
	if len(reqs.Items) != 1 || socialutil.StringValue(reqs.Items[0].Id) != socialutil.StringValue(visibleReq.Id) || reqs.HasNext {
		t.Fatalf("ListFriendRequests page = %#v, want only visible request without next page", reqs)
	}
	if _, err := s.AcceptFriendRequest(ctx, "peer-b", rpcapi.FriendRequestAcceptRequest{Id: socialutil.StringValue(visibleReq.Id)}); err != nil {
		t.Fatalf("AcceptFriendRequest: %v", err)
	}
	deleted, err := s.DeleteFriend(ctx, "peer-a", rpcapi.FriendDeleteRequest{Id: socialutil.RelationID("peer-a", "peer-b")})
	if err != nil {
		t.Fatalf("DeleteFriend: %v", err)
	}
	if socialutil.StringValue(deleted.PeerId) != "peer-b" {
		t.Fatalf("DeleteFriend peer_id = %q, want peer-b", socialutil.StringValue(deleted.PeerId))
	}
	peerBFriends, err := s.ListFriends(ctx, "peer-b", rpcapi.FriendListRequest{})
	if err != nil {
		t.Fatalf("ListFriends peer-b: %v", err)
	}
	if len(peerBFriends.Items) != 0 {
		t.Fatalf("peer-b friends after delete = %#v, want none", peerBFriends.Items)
	}
}

func TestRejectAndAcceptRollbackPaths(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()

	if err := s.ReportFriendOTP(ctx, "peer-b", "101010"); err != nil {
		t.Fatalf("ReportFriendOTP reject: %v", err)
	}
	rejectedReq, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "101010"})
	if err != nil {
		t.Fatalf("CreateFriendRequest reject: %v", err)
	}
	rejectedReq, err = s.RejectFriendRequest(ctx, "peer-b", rpcapi.FriendRequestRejectRequest{Id: socialutil.StringValue(rejectedReq.Id)})
	if err != nil {
		t.Fatalf("RejectFriendRequest: %v", err)
	}
	if rejectedReq.State == nil || *rejectedReq.State != rpcapi.FriendRequestStateRejected {
		t.Fatalf("rejected state = %v, want rejected", rejectedReq.State)
	}
	if _, err := s.AcceptFriendRequest(ctx, "peer-b", rpcapi.FriendRequestAcceptRequest{Id: socialutil.StringValue(rejectedReq.Id)}); err == nil {
		t.Fatal("AcceptFriendRequest rejected request error = nil")
	}

	if err := s.ReportFriendOTP(ctx, "peer-c", "202020"); err != nil {
		t.Fatalf("ReportFriendOTP accept: %v", err)
	}
	acceptedReq, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-c", Code: "202020"})
	if err != nil {
		t.Fatalf("CreateFriendRequest accept: %v", err)
	}
	s.Requests = failingSetStore{Store: s.Requests}
	if _, err := s.AcceptFriendRequest(ctx, "peer-c", rpcapi.FriendRequestAcceptRequest{Id: socialutil.StringValue(acceptedReq.Id)}); err == nil {
		t.Fatal("AcceptFriendRequest with failing request store error = nil")
	}
	if _, err := s.GetFriendRelation(ctx, "peer-a", socialutil.RelationID("peer-a", "peer-c")); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("friend relation after accept rollback error = %v, want not found", err)
	}
}

func TestConfigurationAndValidationErrors(t *testing.T) {
	ctx := context.Background()
	empty := &Server{}
	if _, err := empty.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "123456"}); err == nil {
		t.Fatal("CreateFriendRequest without store error = nil")
	}
	if _, err := empty.ListFriends(ctx, "peer-a", rpcapi.FriendListRequest{}); err == nil {
		t.Fatal("ListFriends without store error = nil")
	}

	s := newTestServer()
	if err := s.ReportFriendOTP(ctx, "", "123456"); err == nil {
		t.Fatal("ReportFriendOTP empty peer error = nil")
	}
	if _, err := s.ListFriendRequests(ctx, "peer-a", rpcapi.FriendRequestListRequest{Box: friendRequestBoxPtr(rpcapi.FriendRequestBox("bogus"))}); err == nil {
		t.Fatal("ListFriendRequests invalid box error = nil")
	}
	bogusState := rpcapi.FriendRequestState("bogus")
	if _, err := s.ListFriendRequests(ctx, "peer-a", rpcapi.FriendRequestListRequest{State: &bogusState}); err == nil {
		t.Fatal("ListFriendRequests invalid state error = nil")
	}
	if _, err := s.CreateFriendRequest(ctx, "", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "123456"}); err == nil {
		t.Fatal("CreateFriendRequest empty owner error = nil")
	}
	if _, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-a", Code: "123456"}); err == nil {
		t.Fatal("CreateFriendRequest self error = nil")
	}
	defaultClock := &Server{Requests: kv.NewMemory(nil), Friends: kv.NewMemory(nil)}
	if err := defaultClock.ReportFriendOTP(ctx, "peer-z", "999999"); err != nil {
		t.Fatalf("ReportFriendOTP with default clock: %v", err)
	}
	if id := (&Server{}).newID(); id == "" {
		t.Fatal("newID without override returned empty string")
	}
}

func TestCreateFriendRequestPropagatesOTPStoreErrors(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()
	s.Requests = failingGetStore{Store: s.Requests}

	_, err := s.CreateFriendRequest(ctx, "peer-a", rpcapi.FriendRequestCreateRequest{ToPeerId: "peer-b", Code: "123456"})
	if err == nil {
		t.Fatal("CreateFriendRequest with failing OTP store error = nil")
	}
	if err.Error() != "forced get failure" {
		t.Fatalf("CreateFriendRequest error = %v, want forced get failure", err)
	}
}

func newTestServer() *Server {
	store := kv.NewMemory(nil)
	now := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	nextID := 0
	return &Server{
		Requests: store,
		Friends:  store,
		Now:      func() time.Time { return now },
		NewID: func() string {
			nextID++
			return "id-" + string(rune('a'+nextID-1))
		},
	}
}

type failingBatchSetStore struct {
	kv.Store
}

func (s failingBatchSetStore) BatchSet(context.Context, []kv.Entry) error {
	return errors.New("forced batch set failure")
}

type failingSetStore struct {
	kv.Store
}

func (s failingSetStore) Set(context.Context, kv.Key, []byte) error {
	return errors.New("forced set failure")
}

type failingGetStore struct {
	kv.Store
}

func (s failingGetStore) Get(context.Context, kv.Key) ([]byte, error) {
	return nil, errors.New("forced get failure")
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
