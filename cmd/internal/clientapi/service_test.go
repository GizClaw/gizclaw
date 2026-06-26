package clientapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/clientservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
	"github.com/gofiber/fiber/v2"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func resetOpenAIHTTPClient(t *testing.T, fn func(*http.Request) (*http.Response, error)) {
	t.Helper()
	orig := openAIHTTPClient
	openAIHTTPClient = func(*gizcli.Client) *http.Client {
		return &http.Client{Transport: roundTripFunc(fn)}
	}
	t.Cleanup(func() { openAIHTTPClient = orig })
}

func TestSanitizePlayCredentialListRedactsBody(t *testing.T) {
	got := sanitizePlayCredentialList(&rpcapi.CredentialListResponse{
		Items: []rpcapi.Credential{
			{Name: "demo", Body: testRPCOpenAICredentialBody("secret")},
		},
	})
	if got == nil || len(got.Items) != 1 {
		t.Fatalf("sanitizePlayCredentialList() = %#v", got)
	}
	if testRPCCredentialBodyString(got.Items[0].Body, "api_key") != "" {
		t.Fatalf("credential body = %#v, want redacted", got.Items[0].Body)
	}
}

func TestPlayHTTPServiceClientUnavailableResponses(t *testing.T) {
	service := &playHTTPService{client: func() (*gizcli.Client, error) {
		return nil, errors.New("offline")
	}}
	ctx := context.Background()
	adopt := rpcapi.PetAdoptRequest{Name: "Pixa"}
	petAction := rpcapi.PetActionRequest{Prompt: "now"}
	petPut := rpcapi.PetPutRequest{Name: "Pixa"}
	rewardClaim := rpcapi.RewardClaimRequest{Prompt: "done"}
	friendAdd := rpcapi.FriendAddRequest{InviteToken: "token"}
	groupCreate := rpcapi.FriendGroupCreateRequest{Name: "room"}
	groupJoin := rpcapi.FriendGroupJoinRequest{InviteToken: "token"}
	groupPut := rpcapi.FriendGroupPutRequest{}
	groupMemberAdd := rpcapi.FriendGroupMemberAddRequest{PeerPublicKey: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}
	groupMemberPut := rpcapi.FriendGroupMemberPutRequest{Role: rpcapi.FriendGroupMemberMutableRole("admin")}
	contactCreate := rpcapi.ContactCreateRequest{DisplayName: ptr("Alice")}
	contactPut := rpcapi.ContactPutRequest{DisplayName: ptr("Alice Zhang")}
	offer := clientservice.WebRTCSessionDescription{Type: clientservice.Offer, Sdp: "v=0"}

	for name, call := range map[string]func() any{
		"workspaces": func() any {
			resp, _ := service.ListPeerWorkspaces(ctx, clientservice.ListPeerWorkspacesRequestObject{})
			return resp
		},
		"workflows": func() any {
			resp, _ := service.ListPeerWorkflows(ctx, clientservice.ListPeerWorkflowsRequestObject{})
			return resp
		},
		"models": func() any {
			resp, _ := service.ListPeerModels(ctx, clientservice.ListPeerModelsRequestObject{})
			return resp
		},
		"credentials": func() any {
			resp, _ := service.ListPeerCredentials(ctx, clientservice.ListPeerCredentialsRequestObject{})
			return resp
		},
		"firmwares": func() any {
			resp, _ := service.ListPeerFirmwares(ctx, clientservice.ListPeerFirmwaresRequestObject{})
			return resp
		},
		"contacts": func() any {
			resp, _ := service.ListPeerContacts(ctx, clientservice.ListPeerContactsRequestObject{})
			return resp
		},
		"create contact": func() any {
			resp, _ := service.CreatePeerContact(ctx, clientservice.CreatePeerContactRequestObject{Body: &contactCreate})
			return resp
		},
		"get contact": func() any {
			resp, _ := service.GetPeerContact(ctx, clientservice.GetPeerContactRequestObject{Id: "contact-a"})
			return resp
		},
		"put contact": func() any {
			resp, _ := service.PutPeerContact(ctx, clientservice.PutPeerContactRequestObject{Id: "contact-a", Body: &contactPut})
			return resp
		},
		"delete contact": func() any {
			resp, _ := service.DeletePeerContact(ctx, clientservice.DeletePeerContactRequestObject{Id: "contact-a"})
			return resp
		},
		"friends": func() any {
			resp, _ := service.ListPeerFriends(ctx, clientservice.ListPeerFriendsRequestObject{})
			return resp
		},
		"add friend": func() any {
			resp, _ := service.AddPeerFriend(ctx, clientservice.AddPeerFriendRequestObject{Body: &friendAdd})
			return resp
		},
		"delete friend": func() any {
			resp, _ := service.DeletePeerFriend(ctx, clientservice.DeletePeerFriendRequestObject{Id: "peer-b"})
			return resp
		},
		"get friend invite token": func() any {
			resp, _ := service.GetPeerFriendInviteToken(ctx, clientservice.GetPeerFriendInviteTokenRequestObject{})
			return resp
		},
		"create friend invite token": func() any {
			resp, _ := service.CreatePeerFriendInviteToken(ctx, clientservice.CreatePeerFriendInviteTokenRequestObject{})
			return resp
		},
		"clear friend invite token": func() any {
			resp, _ := service.ClearPeerFriendInviteToken(ctx, clientservice.ClearPeerFriendInviteTokenRequestObject{})
			return resp
		},
		"friend groups": func() any {
			resp, _ := service.ListPeerFriendGroups(ctx, clientservice.ListPeerFriendGroupsRequestObject{})
			return resp
		},
		"create friend group": func() any {
			resp, _ := service.CreatePeerFriendGroup(ctx, clientservice.CreatePeerFriendGroupRequestObject{Body: &groupCreate})
			return resp
		},
		"join friend group": func() any {
			resp, _ := service.JoinPeerFriendGroup(ctx, clientservice.JoinPeerFriendGroupRequestObject{Body: &groupJoin})
			return resp
		},
		"get friend group": func() any {
			resp, _ := service.GetPeerFriendGroup(ctx, clientservice.GetPeerFriendGroupRequestObject{Id: "group-a"})
			return resp
		},
		"put friend group": func() any {
			resp, _ := service.PutPeerFriendGroup(ctx, clientservice.PutPeerFriendGroupRequestObject{Id: "group-a", Body: &groupPut})
			return resp
		},
		"delete friend group": func() any {
			resp, _ := service.DeletePeerFriendGroup(ctx, clientservice.DeletePeerFriendGroupRequestObject{Id: "group-a"})
			return resp
		},
		"get friend group invite token": func() any {
			resp, _ := service.GetPeerFriendGroupInviteToken(ctx, clientservice.GetPeerFriendGroupInviteTokenRequestObject{Id: "group-a"})
			return resp
		},
		"create friend group invite token": func() any {
			resp, _ := service.CreatePeerFriendGroupInviteToken(ctx, clientservice.CreatePeerFriendGroupInviteTokenRequestObject{Id: "group-a"})
			return resp
		},
		"clear friend group invite token": func() any {
			resp, _ := service.ClearPeerFriendGroupInviteToken(ctx, clientservice.ClearPeerFriendGroupInviteTokenRequestObject{Id: "group-a"})
			return resp
		},
		"friend group members": func() any {
			resp, _ := service.ListPeerFriendGroupMembers(ctx, clientservice.ListPeerFriendGroupMembersRequestObject{Id: "group-a"})
			return resp
		},
		"add friend group member": func() any {
			resp, _ := service.AddPeerFriendGroupMember(ctx, clientservice.AddPeerFriendGroupMemberRequestObject{Id: "group-a", Body: &groupMemberAdd})
			return resp
		},
		"put friend group member": func() any {
			resp, _ := service.PutPeerFriendGroupMember(ctx, clientservice.PutPeerFriendGroupMemberRequestObject{Id: "group-a", MemberId: "peer-b", Body: &groupMemberPut})
			return resp
		},
		"delete friend group member": func() any {
			resp, _ := service.DeletePeerFriendGroupMember(ctx, clientservice.DeletePeerFriendGroupMemberRequestObject{Id: "group-a", MemberId: "peer-b"})
			return resp
		},
		"pets": func() any {
			resp, _ := service.ListPeerPets(ctx, clientservice.ListPeerPetsRequestObject{})
			return resp
		},
		"adopt pet": func() any {
			resp, _ := service.AdoptPeerPet(ctx, clientservice.AdoptPeerPetRequestObject{Body: &adopt})
			return resp
		},
		"get pet": func() any {
			resp, _ := service.GetPeerPet(ctx, clientservice.GetPeerPetRequestObject{Id: "pet-1"})
			return resp
		},
		"put pet": func() any {
			resp, _ := service.PutPeerPet(ctx, clientservice.PutPeerPetRequestObject{Id: "pet-1", Body: &petPut})
			return resp
		},
		"delete pet": func() any {
			resp, _ := service.DeletePeerPet(ctx, clientservice.DeletePeerPetRequestObject{Id: "pet-1"})
			return resp
		},
		"feed pet": func() any {
			resp, _ := service.FeedPeerPet(ctx, clientservice.FeedPeerPetRequestObject{Id: "pet-1", Body: &petAction})
			return resp
		},
		"wash pet": func() any {
			resp, _ := service.WashPeerPet(ctx, clientservice.WashPeerPetRequestObject{Id: "pet-1", Body: &petAction})
			return resp
		},
		"play pet": func() any {
			resp, _ := service.PlayWithPeerPet(ctx, clientservice.PlayWithPeerPetRequestObject{Id: "pet-1", Body: &petAction})
			return resp
		},
		"wallet": func() any {
			resp, _ := service.GetPeerWallet(ctx, clientservice.GetPeerWalletRequestObject{})
			return resp
		},
		"transactions": func() any {
			resp, _ := service.ListPeerWalletTransactions(ctx, clientservice.ListPeerWalletTransactionsRequestObject{})
			return resp
		},
		"transaction": func() any {
			resp, _ := service.GetPeerWalletTransaction(ctx, clientservice.GetPeerWalletTransactionRequestObject{Id: "tx-1"})
			return resp
		},
		"rewards": func() any {
			resp, _ := service.ListPeerRewards(ctx, clientservice.ListPeerRewardsRequestObject{})
			return resp
		},
		"reward": func() any {
			resp, _ := service.GetPeerReward(ctx, clientservice.GetPeerRewardRequestObject{Id: "reward-1"})
			return resp
		},
		"claim reward": func() any {
			resp, _ := service.ClaimPeerReward(ctx, clientservice.ClaimPeerRewardRequestObject{Body: &rewardClaim})
			return resp
		},
		"voices": func() any {
			resp, _ := service.ListPeerVoices(ctx, clientservice.ListPeerVoicesRequestObject{})
			return resp
		},
		"client voices": func() any {
			resp, _ := service.ListClientVoices(ctx, clientservice.ListClientVoicesRequestObject{})
			return resp
		},
		"workspace history": func() any {
			resp, _ := service.ListPeerWorkspaceHistory(ctx, clientservice.ListPeerWorkspaceHistoryRequestObject{WorkspaceName: "workspace-a"})
			return resp
		},
		"workspace history get": func() any {
			resp, _ := service.GetPeerWorkspaceHistory(ctx, clientservice.GetPeerWorkspaceHistoryRequestObject{WorkspaceName: "workspace-a", HistoryId: "history-a"})
			return resp
		},
		"workspace history audio": func() any {
			resp, _ := service.GetPeerWorkspaceHistoryAudio(ctx, clientservice.GetPeerWorkspaceHistoryAudioRequestObject{WorkspaceName: "workspace-a", HistoryId: "history-a"})
			return resp
		},
		"stream voices": func() any {
			resp, _ := service.StreamPlayableVoices(ctx, clientservice.StreamPlayableVoicesRequestObject{})
			return resp
		},
		"webrtc": func() any {
			resp, _ := service.CreateWebRTCOffer(ctx, clientservice.CreateWebRTCOfferRequestObject{Body: &offer})
			return resp
		},
	} {
		resp := call()
		errResp, ok := resp.(playHTTPErrorResponse)
		if !ok {
			t.Fatalf("%s response = %T, want playHTTPErrorResponse", name, resp)
		}
		if errResp.status != http.StatusServiceUnavailable {
			t.Fatalf("%s status = %d, want %d", name, errResp.status, http.StatusServiceUnavailable)
		}
	}
}

func TestPlayHTTPServiceConnectedClientRPCErrorResponses(t *testing.T) {
	service := &playHTTPService{client: func() (*gizcli.Client, error) {
		return &gizcli.Client{}, nil
	}}
	ctx := context.Background()
	friendAdd := rpcapi.FriendAddRequest{InviteToken: "token"}
	groupCreate := rpcapi.FriendGroupCreateRequest{Name: "room"}
	groupJoin := rpcapi.FriendGroupJoinRequest{InviteToken: "token"}
	groupPut := rpcapi.FriendGroupPutRequest{}
	groupMemberAdd := rpcapi.FriendGroupMemberAddRequest{PeerPublicKey: "peer-b", Role: rpcapi.FriendGroupMemberMutableRole("member")}
	groupMemberPut := rpcapi.FriendGroupMemberPutRequest{Role: rpcapi.FriendGroupMemberMutableRole("admin")}
	contactCreate := rpcapi.ContactCreateRequest{DisplayName: ptr("Alice")}
	contactPut := rpcapi.ContactPutRequest{DisplayName: ptr("Alice Zhang")}

	for name, call := range map[string]func() any{
		"contacts": func() any {
			resp, _ := service.ListPeerContacts(ctx, clientservice.ListPeerContactsRequestObject{})
			return resp
		},
		"create contact": func() any {
			resp, _ := service.CreatePeerContact(ctx, clientservice.CreatePeerContactRequestObject{Body: &contactCreate})
			return resp
		},
		"get contact": func() any {
			resp, _ := service.GetPeerContact(ctx, clientservice.GetPeerContactRequestObject{Id: "contact-a"})
			return resp
		},
		"put contact": func() any {
			resp, _ := service.PutPeerContact(ctx, clientservice.PutPeerContactRequestObject{Id: "contact-a", Body: &contactPut})
			return resp
		},
		"delete contact": func() any {
			resp, _ := service.DeletePeerContact(ctx, clientservice.DeletePeerContactRequestObject{Id: "contact-a"})
			return resp
		},
		"friends": func() any {
			resp, _ := service.ListPeerFriends(ctx, clientservice.ListPeerFriendsRequestObject{})
			return resp
		},
		"add friend": func() any {
			resp, _ := service.AddPeerFriend(ctx, clientservice.AddPeerFriendRequestObject{Body: &friendAdd})
			return resp
		},
		"delete friend": func() any {
			resp, _ := service.DeletePeerFriend(ctx, clientservice.DeletePeerFriendRequestObject{Id: "peer-b"})
			return resp
		},
		"get friend invite token": func() any {
			resp, _ := service.GetPeerFriendInviteToken(ctx, clientservice.GetPeerFriendInviteTokenRequestObject{})
			return resp
		},
		"create friend invite token": func() any {
			resp, _ := service.CreatePeerFriendInviteToken(ctx, clientservice.CreatePeerFriendInviteTokenRequestObject{})
			return resp
		},
		"clear friend invite token": func() any {
			resp, _ := service.ClearPeerFriendInviteToken(ctx, clientservice.ClearPeerFriendInviteTokenRequestObject{})
			return resp
		},
		"friend groups": func() any {
			resp, _ := service.ListPeerFriendGroups(ctx, clientservice.ListPeerFriendGroupsRequestObject{})
			return resp
		},
		"create friend group": func() any {
			resp, _ := service.CreatePeerFriendGroup(ctx, clientservice.CreatePeerFriendGroupRequestObject{Body: &groupCreate})
			return resp
		},
		"join friend group": func() any {
			resp, _ := service.JoinPeerFriendGroup(ctx, clientservice.JoinPeerFriendGroupRequestObject{Body: &groupJoin})
			return resp
		},
		"get friend group": func() any {
			resp, _ := service.GetPeerFriendGroup(ctx, clientservice.GetPeerFriendGroupRequestObject{Id: "group-a"})
			return resp
		},
		"put friend group": func() any {
			resp, _ := service.PutPeerFriendGroup(ctx, clientservice.PutPeerFriendGroupRequestObject{Id: "group-a", Body: &groupPut})
			return resp
		},
		"delete friend group": func() any {
			resp, _ := service.DeletePeerFriendGroup(ctx, clientservice.DeletePeerFriendGroupRequestObject{Id: "group-a"})
			return resp
		},
		"get friend group invite token": func() any {
			resp, _ := service.GetPeerFriendGroupInviteToken(ctx, clientservice.GetPeerFriendGroupInviteTokenRequestObject{Id: "group-a"})
			return resp
		},
		"create friend group invite token": func() any {
			resp, _ := service.CreatePeerFriendGroupInviteToken(ctx, clientservice.CreatePeerFriendGroupInviteTokenRequestObject{Id: "group-a"})
			return resp
		},
		"clear friend group invite token": func() any {
			resp, _ := service.ClearPeerFriendGroupInviteToken(ctx, clientservice.ClearPeerFriendGroupInviteTokenRequestObject{Id: "group-a"})
			return resp
		},
		"friend group members": func() any {
			resp, _ := service.ListPeerFriendGroupMembers(ctx, clientservice.ListPeerFriendGroupMembersRequestObject{Id: "group-a"})
			return resp
		},
		"add friend group member": func() any {
			resp, _ := service.AddPeerFriendGroupMember(ctx, clientservice.AddPeerFriendGroupMemberRequestObject{Id: "group-a", Body: &groupMemberAdd})
			return resp
		},
		"put friend group member": func() any {
			resp, _ := service.PutPeerFriendGroupMember(ctx, clientservice.PutPeerFriendGroupMemberRequestObject{Id: "group-a", MemberId: "peer-b", Body: &groupMemberPut})
			return resp
		},
		"delete friend group member": func() any {
			resp, _ := service.DeletePeerFriendGroupMember(ctx, clientservice.DeletePeerFriendGroupMemberRequestObject{Id: "group-a", MemberId: "peer-b"})
			return resp
		},
		"workspace history": func() any {
			resp, _ := service.ListPeerWorkspaceHistory(ctx, clientservice.ListPeerWorkspaceHistoryRequestObject{WorkspaceName: "workspace-a"})
			return resp
		},
		"workspace history get": func() any {
			resp, _ := service.GetPeerWorkspaceHistory(ctx, clientservice.GetPeerWorkspaceHistoryRequestObject{WorkspaceName: "workspace-a", HistoryId: "history-a"})
			return resp
		},
		"workspace history audio": func() any {
			resp, _ := service.GetPeerWorkspaceHistoryAudio(ctx, clientservice.GetPeerWorkspaceHistoryAudioRequestObject{WorkspaceName: "workspace-a", HistoryId: "history-a"})
			return resp
		},
	} {
		resp := call()
		errResp, ok := resp.(playHTTPErrorResponse)
		if !ok {
			t.Fatalf("%s response = %T, want playHTTPErrorResponse", name, resp)
		}
		if errResp.status != http.StatusBadGateway {
			t.Fatalf("%s status = %d, want %d", name, errResp.status, http.StatusBadGateway)
		}
	}
}

func TestPlayHTTPServiceInvalidatesClosedCachedClient(t *testing.T) {
	stale := &gizcli.Client{}
	fresh := &gizcli.Client{}
	calls := 0
	invalidated := false
	service := &playHTTPService{
		client: func() (*gizcli.Client, error) {
			calls++
			if calls == 1 {
				return stale, nil
			}
			return fresh, nil
		},
		invalidate: func(c *gizcli.Client) {
			if c != stale {
				t.Fatalf("invalidate client = %p, want stale %p", c, stale)
			}
			invalidated = true
		},
	}

	got, errResp, ok := service.gizCLIClient()
	if !ok {
		t.Fatalf("gizCLIClient failed: %#v", errResp)
	}
	if got != fresh {
		t.Fatalf("gizCLIClient returned %p, want fresh %p", got, fresh)
	}
	if !invalidated {
		t.Fatal("gizCLIClient did not invalidate stale client")
	}
	if calls != 2 {
		t.Fatalf("client calls = %d, want 2", calls)
	}
}

func TestPlayHTTPServiceBodyRequiredResponses(t *testing.T) {
	service := &playHTTPService{client: func() (*gizcli.Client, error) {
		t.Fatal("body validation should not dial client")
		return nil, errors.New("unexpected dial")
	}}
	ctx := context.Background()

	for name, call := range map[string]func() any{
		"create contact": func() any {
			resp, _ := service.CreatePeerContact(ctx, clientservice.CreatePeerContactRequestObject{})
			return resp
		},
		"put contact": func() any {
			resp, _ := service.PutPeerContact(ctx, clientservice.PutPeerContactRequestObject{Id: "contact-a"})
			return resp
		},
		"add friend": func() any {
			resp, _ := service.AddPeerFriend(ctx, clientservice.AddPeerFriendRequestObject{})
			return resp
		},
		"create friend group": func() any {
			resp, _ := service.CreatePeerFriendGroup(ctx, clientservice.CreatePeerFriendGroupRequestObject{})
			return resp
		},
		"join friend group": func() any {
			resp, _ := service.JoinPeerFriendGroup(ctx, clientservice.JoinPeerFriendGroupRequestObject{})
			return resp
		},
		"put friend group": func() any {
			resp, _ := service.PutPeerFriendGroup(ctx, clientservice.PutPeerFriendGroupRequestObject{Id: "group-a"})
			return resp
		},
		"add friend group member": func() any {
			resp, _ := service.AddPeerFriendGroupMember(ctx, clientservice.AddPeerFriendGroupMemberRequestObject{Id: "group-a"})
			return resp
		},
		"put friend group member": func() any {
			resp, _ := service.PutPeerFriendGroupMember(ctx, clientservice.PutPeerFriendGroupMemberRequestObject{Id: "group-a", MemberId: "peer-b"})
			return resp
		},
		"adopt pet": func() any {
			resp, _ := service.AdoptPeerPet(ctx, clientservice.AdoptPeerPetRequestObject{})
			return resp
		},
		"put pet": func() any {
			resp, _ := service.PutPeerPet(ctx, clientservice.PutPeerPetRequestObject{Id: "pet-1"})
			return resp
		},
		"feed pet": func() any {
			resp, _ := service.FeedPeerPet(ctx, clientservice.FeedPeerPetRequestObject{Id: "pet-1"})
			return resp
		},
		"wash pet": func() any {
			resp, _ := service.WashPeerPet(ctx, clientservice.WashPeerPetRequestObject{Id: "pet-1"})
			return resp
		},
		"play pet": func() any {
			resp, _ := service.PlayWithPeerPet(ctx, clientservice.PlayWithPeerPetRequestObject{Id: "pet-1"})
			return resp
		},
		"claim reward": func() any {
			resp, _ := service.ClaimPeerReward(ctx, clientservice.ClaimPeerRewardRequestObject{})
			return resp
		},
		"webrtc": func() any {
			resp, _ := service.CreateWebRTCOffer(ctx, clientservice.CreateWebRTCOfferRequestObject{})
			return resp
		},
	} {
		resp := call()
		errResp, ok := resp.(playHTTPErrorResponse)
		if !ok {
			t.Fatalf("%s response = %T, want playHTTPErrorResponse", name, resp)
		}
		if errResp.status != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want %d", name, errResp.status, http.StatusBadRequest)
		}
	}
}

func TestPlayHTTPServiceListPeerResourceNames(t *testing.T) {
	service := &playHTTPService{}
	resp, err := service.ListPeerResourceNames(context.Background(), clientservice.ListPeerResourceNamesRequestObject{})
	if err != nil {
		t.Fatalf("ListPeerResourceNames error = %v", err)
	}
	okResp, ok := resp.(clientservice.ListPeerResourceNames200JSONResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}
	if len(okResp.Resources) == 0 {
		t.Fatal("resources should not be empty")
	}
}

func TestClientAPIHandlerServesResourceCatalog(t *testing.T) {
	handler := Handler(func() (*gizcli.Client, error) {
		t.Fatal("catalog should not dial client")
		return nil, errors.New("unexpected dial")
	}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/peer-resources", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /peer-resources status = %d", rec.Code)
	}
}

func TestCreateWebRTCOfferRejectsInvalidOffer(t *testing.T) {
	service := &playHTTPService{client: func() (*gizcli.Client, error) {
		t.Fatal("invalid offer should not dial client")
		return nil, errors.New("unexpected dial")
	}}
	body := clientservice.WebRTCSessionDescription{Type: clientservice.Offer}
	resp, _ := service.CreateWebRTCOffer(context.Background(), clientservice.CreateWebRTCOfferRequestObject{Body: &body})
	errResp, ok := resp.(playHTTPErrorResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}
	if errResp.status != http.StatusBadRequest {
		t.Fatalf("status = %d", errResp.status)
	}
}

func TestReloadPlayRunForWebRTCUsesWorkspaceRuntimeWhenSelected(t *testing.T) {
	active := "workspace-a"
	client := &fakePlayWebRTCReloader{
		workspaceState: &rpcapi.ServerGetRunWorkspaceResponse{
			ActiveWorkspaceName: &active,
			WorkspaceName:       active,
		},
	}
	if err := reloadPlayRunForWebRTC(context.Background(), client); err != nil {
		t.Fatalf("reloadPlayRunForWebRTC() error = %v", err)
	}
	if client.workspaceReloads != 1 || client.runReloads != 0 {
		t.Fatalf("reloads workspace=%d run=%d, want workspace only", client.workspaceReloads, client.runReloads)
	}
}

func TestReloadPlayRunForWebRTCFallsBackToRunRuntimeWithoutWorkspace(t *testing.T) {
	client := &fakePlayWebRTCReloader{workspaceState: &rpcapi.ServerGetRunWorkspaceResponse{}}
	if err := reloadPlayRunForWebRTC(context.Background(), client); err != nil {
		t.Fatalf("reloadPlayRunForWebRTC() error = %v", err)
	}
	if client.workspaceReloads != 0 || client.runReloads != 1 {
		t.Fatalf("reloads workspace=%d run=%d, want run only", client.workspaceReloads, client.runReloads)
	}
}

func TestReloadPlayRunForWebRTCErrorBranches(t *testing.T) {
	if err := reloadPlayRunForWebRTC(context.Background(), nil); err == nil {
		t.Fatal("reloadPlayRunForWebRTC(nil) error = nil")
	}

	client := &fakePlayWebRTCReloader{workspaceStateErr: errors.New("workspace unavailable")}
	if err := reloadPlayRunForWebRTC(context.Background(), client); err == nil || !strings.Contains(err.Error(), "workspace unavailable") {
		t.Fatalf("workspace error = %v", err)
	}

	client = &fakePlayWebRTCReloader{
		workspaceStateErr: errors.New("workspace not configured"),
		runReloadErr:      errors.New("run unavailable"),
	}
	if err := reloadPlayRunForWebRTC(context.Background(), client); err == nil || !strings.Contains(err.Error(), "run unavailable") {
		t.Fatalf("run reload error = %v", err)
	}

	client = &fakePlayWebRTCReloader{
		workspaceStateErr: errors.New("workspace not configured"),
		runReloadErr:      errors.New("run not configured"),
	}
	if err := reloadPlayRunForWebRTC(context.Background(), client); err != nil {
		t.Fatalf("not configured error = %v, want nil", err)
	}
}

func TestPlayHTTPErrorResponseVisitors(t *testing.T) {
	visitors := []func(playHTTPErrorResponse, *fiber.Ctx) error{
		playHTTPErrorResponse.VisitListPeerCredentialsResponse,
		playHTTPErrorResponse.VisitListPeerFirmwaresResponse,
		playHTTPErrorResponse.VisitListPeerModelsResponse,
		playHTTPErrorResponse.VisitListPeerPetsResponse,
		playHTTPErrorResponse.VisitAdoptPeerPetResponse,
		playHTTPErrorResponse.VisitDeletePeerPetResponse,
		playHTTPErrorResponse.VisitGetPeerPetResponse,
		playHTTPErrorResponse.VisitPutPeerPetResponse,
		playHTTPErrorResponse.VisitFeedPeerPetResponse,
		playHTTPErrorResponse.VisitPlayWithPeerPetResponse,
		playHTTPErrorResponse.VisitWashPeerPetResponse,
		playHTTPErrorResponse.VisitListPeerRewardsResponse,
		playHTTPErrorResponse.VisitClaimPeerRewardResponse,
		playHTTPErrorResponse.VisitGetPeerRewardResponse,
		playHTTPErrorResponse.VisitListPeerVoicesResponse,
		playHTTPErrorResponse.VisitGetPeerWalletResponse,
		playHTTPErrorResponse.VisitListPeerWalletTransactionsResponse,
		playHTTPErrorResponse.VisitGetPeerWalletTransactionResponse,
		playHTTPErrorResponse.VisitListPeerWorkflowsResponse,
		playHTTPErrorResponse.VisitListPeerWorkspacesResponse,
		playHTTPErrorResponse.VisitListPeerContactsResponse,
		playHTTPErrorResponse.VisitCreatePeerContactResponse,
		playHTTPErrorResponse.VisitGetPeerContactResponse,
		playHTTPErrorResponse.VisitPutPeerContactResponse,
		playHTTPErrorResponse.VisitDeletePeerContactResponse,
		playHTTPErrorResponse.VisitListPeerFriendsResponse,
		playHTTPErrorResponse.VisitAddPeerFriendResponse,
		playHTTPErrorResponse.VisitDeletePeerFriendResponse,
		playHTTPErrorResponse.VisitGetPeerFriendInviteTokenResponse,
		playHTTPErrorResponse.VisitCreatePeerFriendInviteTokenResponse,
		playHTTPErrorResponse.VisitClearPeerFriendInviteTokenResponse,
		playHTTPErrorResponse.VisitListPeerFriendGroupsResponse,
		playHTTPErrorResponse.VisitCreatePeerFriendGroupResponse,
		playHTTPErrorResponse.VisitJoinPeerFriendGroupResponse,
		playHTTPErrorResponse.VisitGetPeerFriendGroupResponse,
		playHTTPErrorResponse.VisitPutPeerFriendGroupResponse,
		playHTTPErrorResponse.VisitDeletePeerFriendGroupResponse,
		playHTTPErrorResponse.VisitGetPeerFriendGroupInviteTokenResponse,
		playHTTPErrorResponse.VisitCreatePeerFriendGroupInviteTokenResponse,
		playHTTPErrorResponse.VisitClearPeerFriendGroupInviteTokenResponse,
		playHTTPErrorResponse.VisitListPeerFriendGroupMembersResponse,
		playHTTPErrorResponse.VisitAddPeerFriendGroupMemberResponse,
		playHTTPErrorResponse.VisitPutPeerFriendGroupMemberResponse,
		playHTTPErrorResponse.VisitDeletePeerFriendGroupMemberResponse,
		playHTTPErrorResponse.VisitListPeerWorkspaceHistoryResponse,
		playHTTPErrorResponse.VisitGetPeerWorkspaceHistoryResponse,
		playHTTPErrorResponse.VisitGetPeerWorkspaceHistoryAudioResponse,
		playHTTPErrorResponse.VisitStreamPlayableVoicesResponse,
		playHTTPErrorResponse.VisitListClientVoicesResponse,
		playHTTPErrorResponse.VisitCreateWebRTCOfferResponse,
	}
	for i, visit := range visitors {
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Get("/", func(c *fiber.Ctx) error {
			return visit(playHTTPErrorResponse{status: http.StatusTeapot, message: "teapot"}, c)
		})
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
		if err != nil {
			t.Fatalf("visitor %d error = %v", i, err)
		}
		if resp.StatusCode != http.StatusTeapot {
			t.Fatalf("visitor %d status = %d", i, resp.StatusCode)
		}
	}
}

type fakePlayWebRTCReloader struct {
	workspaceState     *rpcapi.ServerGetRunWorkspaceResponse
	workspaceStateErr  error
	workspaceReloadErr error
	runReloadErr       error
	workspaceReloads   int
	runReloads         int
}

func (f *fakePlayWebRTCReloader) GetServerRunWorkspace(context.Context, string) (*rpcapi.ServerGetRunWorkspaceResponse, error) {
	return f.workspaceState, f.workspaceStateErr
}

func (f *fakePlayWebRTCReloader) ReloadServerRunWorkspace(context.Context, string) (*rpcapi.ServerReloadRunWorkspaceResponse, error) {
	f.workspaceReloads++
	return &rpcapi.ServerReloadRunWorkspaceResponse{}, f.workspaceReloadErr
}

func (f *fakePlayWebRTCReloader) ReloadServerRun(context.Context, string) (*rpcapi.ServerReloadRunResponse, error) {
	f.runReloads++
	return &rpcapi.ServerReloadRunResponse{}, f.runReloadErr
}

func TestPlayHTTPErrorResponseDefaultStatus(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/", func(c *fiber.Ctx) error {
		return (playHTTPErrorResponse{message: "bad gateway"}).write(c)
	})
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("app.Test error = %v", err)
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestPlayVoiceMatches(t *testing.T) {
	source := apitypes.VoiceSource("global")
	kind := apitypes.VoiceProviderKind("openai")
	name := "main"
	voice := apitypes.Voice{
		Source: source,
		Provider: apitypes.VoiceProvider{
			Kind: kind,
			Name: name,
		},
	}
	if !playVoiceMatches(voice, &source, &kind, &name) {
		t.Fatal("voice should match exact filters")
	}
	otherName := "other"
	if playVoiceMatches(voice, &source, &kind, &otherName) {
		t.Fatal("voice matched wrong provider name")
	}
	otherSource := apitypes.VoiceSource("custom")
	if playVoiceMatches(voice, &otherSource, &kind, &name) {
		t.Fatal("voice matched wrong source")
	}
	otherKind := apitypes.VoiceProviderKind("minimax")
	if playVoiceMatches(voice, &source, &otherKind, &name) {
		t.Fatal("voice matched wrong provider kind")
	}
}

func TestPlayLimitValue(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value *int
		want  int
	}{
		{name: "default", want: 20},
		{name: "positive", value: ptr(10), want: 10},
		{name: "non-positive", value: ptr(0), want: 20},
		{name: "cap", value: ptr(101), want: 100},
	} {
		if got := playLimitValue(tc.value); got != tc.want {
			t.Fatalf("%s limit = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestPlayLimitPtr(t *testing.T) {
	got := playLimitPtr(ptr(0))
	if got == nil || *got != 20 {
		t.Fatalf("playLimitPtr = %v, want 20", got)
	}
}

func TestPlayRPCErrorStatus(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{name: "plain", err: errors.New("plain"), want: http.StatusBadGateway},
		{name: "forbidden", err: rpcapi.Error{Code: rpcapi.RPCErrorCodeForbidden}, want: http.StatusForbidden},
		{name: "not found", err: rpcapi.Error{Code: rpcapi.RPCErrorCodeNotFound}, want: http.StatusNotFound},
		{name: "bad request", err: rpcapi.Error{Code: rpcapi.RPCErrorCodeBadRequest}, want: http.StatusBadRequest},
		{name: "acl", err: rpcapi.Error{Code: rpcapi.RPCErrorCodeBadRequest, Message: "acl: denied"}, want: http.StatusForbidden},
		{name: "invalid params", err: rpcapi.Error{Code: rpcapi.RPCErrorCodeInvalidParams}, want: http.StatusBadRequest},
	} {
		if got := playRPCErrorStatus(tc.err); got != tc.want {
			t.Fatalf("%s status = %d, want %d", tc.name, got, tc.want)
		}
	}
	if got := playHTTPError(rpcapi.Error{Code: rpcapi.RPCErrorCodeNotFound, Message: "missing"}); got.status != http.StatusNotFound {
		t.Fatalf("playHTTPError status = %d", got.status)
	}
}

func TestPlayVoiceListItemsUsesDataAndItems(t *testing.T) {
	dataVoice := apitypes.Voice{Id: "data"}
	itemVoice := apitypes.Voice{Id: "item"}
	got := playVoiceListItems(clientservice.ClientVoiceListResponse{
		Data:  []apitypes.Voice{dataVoice},
		Items: &[]apitypes.Voice{itemVoice},
	})
	if len(got) != 2 || got[0].Id != "data" || got[1].Id != "item" {
		t.Fatalf("items = %#v", got)
	}
}

func TestWritePlayVoiceStreamEvent(t *testing.T) {
	var buf bytes.Buffer
	writePlayVoiceStreamEvent(&buf, clientservice.PlayVoiceStreamEvent{Done: ptr(true)})
	if got := buf.String(); got == "" || !bytes.Contains(buf.Bytes(), []byte("data:")) {
		t.Fatalf("event = %q", got)
	}
}

func TestFetchPlayVoicePage(t *testing.T) {
	resetOpenAIHTTPClient(t, func(r *http.Request) (*http.Response, error) {
		if got := r.URL.Query().Get("limit"); got != "7" {
			t.Fatalf("limit = %q", got)
		}
		if got := r.URL.Query().Get("cursor"); got != "next" {
			t.Fatalf("cursor = %q", got)
		}
		body := `{"object":"list","data":[{"id":"voice-1","name":"Voice","source":"global","provider":{"kind":"openai","name":"main"}}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body)), Header: http.Header{}}, nil
	})
	source := apitypes.VoiceSource("global")
	kind := apitypes.VoiceProviderKind("openai")
	name := "main"
	got, err := fetchPlayVoicePage(context.Background(), nil, "next", 7, &source, &kind, &name)
	if err != nil {
		t.Fatalf("fetchPlayVoicePage error = %v", err)
	}
	if len(got.Data) != 1 || got.Data[0].Id != "voice-1" {
		t.Fatalf("voices = %#v", got.Data)
	}
}

func TestFetchPlayVoicePageHTTPError(t *testing.T) {
	resetOpenAIHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(bytes.NewBufferString("upstream")), Header: http.Header{}}, nil
	})
	_, err := fetchPlayVoicePage(context.Background(), nil, "", 20, nil, nil, nil)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("HTTP 502 upstream")) {
		t.Fatalf("fetchPlayVoicePage error = %v", err)
	}
}

func TestListClientVoicesFiltersAndSetsObject(t *testing.T) {
	resetOpenAIHTTPClient(t, func(*http.Request) (*http.Response, error) {
		body := `{"data":[{"id":"match","name":"Match","source":"global","provider":{"kind":"openai","name":"main"}},{"id":"skip","name":"Skip","source":"global","provider":{"kind":"openai","name":"other"}}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body)), Header: http.Header{}}, nil
	})
	kind := apitypes.VoiceProviderKind("openai")
	name := "main"
	got, err := listClientVoices(context.Background(), nil, nil, nil, nil, &kind, &name)
	if err != nil {
		t.Fatalf("listClientVoices error = %v", err)
	}
	if got.Object != clientservice.List || len(got.Data) != 1 || got.Data[0].Id != "match" {
		t.Fatalf("response = %#v", got)
	}
}

func TestStreamPlayableVoicesWritesErrorAndInvalidates(t *testing.T) {
	resetOpenAIHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})
	var invalidated bool
	var buf bytes.Buffer
	streamPlayableVoices(context.Background(), &buf, nil, func(*gizcli.Client) { invalidated = true }, nil, nil, nil)
	if !invalidated {
		t.Fatal("client was not invalidated")
	}
	if !bytes.Contains(buf.Bytes(), []byte("offline")) {
		t.Fatalf("stream = %q", buf.String())
	}
}

func TestStreamPlayableVoicesPaginatesAndFilters(t *testing.T) {
	kind := apitypes.VoiceProviderKind("openai")
	name := "main"
	next := "next-page"
	calls := 0
	resetOpenAIHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		calls++
		if got := req.URL.Query().Get("limit"); got != "1" {
			t.Fatalf("limit = %q, want 1", got)
		}
		if calls == 2 {
			if got := req.URL.Query().Get("cursor"); got != next {
				t.Fatalf("cursor = %q, want %q", got, next)
			}
		}
		voice := apitypes.Voice{
			Id:     fmt.Sprintf("voice-%d", calls),
			Source: apitypes.VoiceSource("global"),
			Provider: apitypes.VoiceProvider{
				Kind: kind,
				Name: name,
			},
		}
		filtered := apitypes.Voice{
			Id:     "filtered",
			Source: apitypes.VoiceSource("global"),
			Provider: apitypes.VoiceProvider{
				Kind: kind,
				Name: "other",
			},
		}
		body := clientservice.ClientVoiceListResponse{
			Data:    []apitypes.Voice{voice, filtered},
			HasNext: calls == 1,
			Object:  clientservice.List,
		}
		if calls == 1 {
			body.NextCursor = &next
		}
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal voice page: %v", err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}}, nil
	})

	var out bytes.Buffer
	streamPlayableVoices(context.Background(), &out, &gizcli.Client{}, nil, &kind, &name, ptr(1))
	text := out.String()
	if calls != 2 {
		t.Fatalf("voice page calls = %d, want 2", calls)
	}
	if !strings.Contains(text, "voice-1") || !strings.Contains(text, "voice-2") || strings.Contains(text, "filtered") || !strings.Contains(text, `"done":true`) {
		t.Fatalf("stream output = %q", text)
	}
}

func TestCreatePlayWebRTCAnswerPreflightErrors(t *testing.T) {
	offer := clientservice.WebRTCSessionDescription{Type: clientservice.Offer, Sdp: "v=0"}
	if _, errResp, ok := createPlayWebRTCAnswer(context.Background(), func() (*gizcli.Client, error) {
		return nil, errors.New("offline")
	}, offer); ok || errResp.status != http.StatusServiceUnavailable {
		t.Fatalf("client error response = %#v ok=%v, want unavailable", errResp, ok)
	}
	if _, errResp, ok := createPlayWebRTCAnswer(context.Background(), func() (*gizcli.Client, error) {
		return &gizcli.Client{}, nil
	}, offer); ok || errResp.status != http.StatusBadGateway {
		t.Fatalf("reload error response = %#v ok=%v, want bad gateway", errResp, ok)
	}
}
