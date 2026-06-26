package clientapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/clientservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/pion/webrtc/v4"
)

type ClientProvider func() (*gizcli.Client, error)
type ClientInvalidator func(*gizcli.Client)

var openAIHTTPClient = func(c *gizcli.Client) *http.Client {
	return c.HTTPClient(gizcli.ServiceOpenAI)
}

func Handler(client ClientProvider, invalidate ClientInvalidator) http.Handler {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	clientservice.RegisterHandlers(app, clientservice.NewStrictHandler(&playHTTPService{client: client, invalidate: invalidate}, nil))
	return adaptor.FiberApp(app)
}

type playHTTPService struct {
	client     ClientProvider
	invalidate ClientInvalidator
}

var _ clientservice.StrictServerInterface = (*playHTTPService)(nil)

func (s *playHTTPService) gizCLIClient() (*gizcli.Client, playHTTPErrorResponse, bool) {
	c, err := s.client()
	if err != nil {
		return nil, playHTTPErrorResponse{status: http.StatusServiceUnavailable, message: err.Error()}, false
	}
	if c.PeerConn() == nil && s.invalidate != nil {
		s.invalidate(c)
		c, err = s.client()
		if err != nil {
			return nil, playHTTPErrorResponse{status: http.StatusServiceUnavailable, message: err.Error()}, false
		}
	}
	return c, playHTTPErrorResponse{}, true
}

func (s *playHTTPService) rpcID() string {
	return "play-ui-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func (s *playHTTPService) ListPeerResourceNames(context.Context, clientservice.ListPeerResourceNamesRequestObject) (clientservice.ListPeerResourceNamesResponseObject, error) {
	return clientservice.ListPeerResourceNames200JSONResponse{
		Resources: []clientservice.PeerResourceName{
			clientservice.Workspaces,
			clientservice.Workflows,
			clientservice.Models,
			clientservice.Credentials,
			clientservice.Firmwares,
			clientservice.Voices,
			clientservice.Pets,
			clientservice.Contacts,
			clientservice.Friends,
			clientservice.FriendGroups,
			clientservice.Wallet,
			clientservice.WalletTransactions,
			clientservice.Rewards,
		},
	}, nil
}

func (s *playHTTPService) ListPeerWorkspaces(ctx context.Context, request clientservice.ListPeerWorkspacesRequestObject) (clientservice.ListPeerWorkspacesResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ListWorkspaces(ctx, s.rpcID(), rpcapi.WorkspaceListRequest{Cursor: request.Params.Cursor, Limit: playLimitPtr(request.Params.Limit)})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ListPeerWorkspaces200JSONResponse(*result), nil
}

func (s *playHTTPService) ListPeerWorkflows(ctx context.Context, request clientservice.ListPeerWorkflowsRequestObject) (clientservice.ListPeerWorkflowsResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ListWorkflows(ctx, s.rpcID(), rpcapi.WorkflowListRequest{Cursor: request.Params.Cursor, Limit: playLimitPtr(request.Params.Limit)})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ListPeerWorkflows200JSONResponse(*result), nil
}

func (s *playHTTPService) ListPeerModels(ctx context.Context, request clientservice.ListPeerModelsRequestObject) (clientservice.ListPeerModelsResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ListModels(ctx, s.rpcID(), rpcapi.ModelListRequest{Cursor: request.Params.Cursor, Limit: playLimitPtr(request.Params.Limit)})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ListPeerModels200JSONResponse(*result), nil
}

func (s *playHTTPService) ListPeerCredentials(ctx context.Context, request clientservice.ListPeerCredentialsRequestObject) (clientservice.ListPeerCredentialsResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ListCredentials(ctx, s.rpcID(), rpcapi.CredentialListRequest{Cursor: request.Params.Cursor, Limit: playLimitPtr(request.Params.Limit)})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ListPeerCredentials200JSONResponse(*sanitizePlayCredentialList(result)), nil
}

func (s *playHTTPService) ListPeerFirmwares(ctx context.Context, request clientservice.ListPeerFirmwaresRequestObject) (clientservice.ListPeerFirmwaresResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ListFirmwares(ctx, s.rpcID(), rpcapi.FirmwareListRequest{Cursor: request.Params.Cursor, Limit: playLimitPtr(request.Params.Limit)})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ListPeerFirmwares200JSONResponse(*result), nil
}

func (s *playHTTPService) ListPeerContacts(ctx context.Context, request clientservice.ListPeerContactsRequestObject) (clientservice.ListPeerContactsResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ListContacts(ctx, s.rpcID(), rpcapi.ContactListRequest{Cursor: request.Params.Cursor, Limit: playLimitPtr(request.Params.Limit)})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ListPeerContacts200JSONResponse(*result), nil
}

func (s *playHTTPService) CreatePeerContact(ctx context.Context, request clientservice.CreatePeerContactRequestObject) (clientservice.CreatePeerContactResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.CreateContact(ctx, s.rpcID(), *request.Body)
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.CreatePeerContact200JSONResponse(*result), nil
}

func (s *playHTTPService) GetPeerContact(ctx context.Context, request clientservice.GetPeerContactRequestObject) (clientservice.GetPeerContactResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.GetContact(ctx, s.rpcID(), rpcapi.ContactGetRequest{Id: request.Id})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.GetPeerContact200JSONResponse(*result), nil
}

func (s *playHTTPService) PutPeerContact(ctx context.Context, request clientservice.PutPeerContactRequestObject) (clientservice.PutPeerContactResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	body := *request.Body
	body.Id = request.Id
	result, err := c.PutContact(ctx, s.rpcID(), body)
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.PutPeerContact200JSONResponse(*result), nil
}

func (s *playHTTPService) DeletePeerContact(ctx context.Context, request clientservice.DeletePeerContactRequestObject) (clientservice.DeletePeerContactResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.DeleteContact(ctx, s.rpcID(), rpcapi.ContactDeleteRequest{Id: request.Id})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.DeletePeerContact200JSONResponse(*result), nil
}

func (s *playHTTPService) ListPeerFriends(ctx context.Context, request clientservice.ListPeerFriendsRequestObject) (clientservice.ListPeerFriendsResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ListFriends(ctx, s.rpcID(), rpcapi.FriendListRequest{Cursor: request.Params.Cursor, Limit: playLimitPtr(request.Params.Limit)})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ListPeerFriends200JSONResponse(*result), nil
}

func (s *playHTTPService) AddPeerFriend(ctx context.Context, request clientservice.AddPeerFriendRequestObject) (clientservice.AddPeerFriendResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.AddFriend(ctx, s.rpcID(), *request.Body)
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.AddPeerFriend200JSONResponse(*result), nil
}

func (s *playHTTPService) DeletePeerFriend(ctx context.Context, request clientservice.DeletePeerFriendRequestObject) (clientservice.DeletePeerFriendResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.DeleteFriend(ctx, s.rpcID(), rpcapi.FriendDeleteRequest{Id: request.Id})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.DeletePeerFriend200JSONResponse(*result), nil
}

func (s *playHTTPService) GetPeerFriendInviteToken(ctx context.Context, _ clientservice.GetPeerFriendInviteTokenRequestObject) (clientservice.GetPeerFriendInviteTokenResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.GetFriendInviteToken(ctx, s.rpcID(), rpcapi.FriendInviteTokenGetRequest{})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.GetPeerFriendInviteToken200JSONResponse(*result), nil
}

func (s *playHTTPService) CreatePeerFriendInviteToken(ctx context.Context, _ clientservice.CreatePeerFriendInviteTokenRequestObject) (clientservice.CreatePeerFriendInviteTokenResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.CreateFriendInviteToken(ctx, s.rpcID(), rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.CreatePeerFriendInviteToken200JSONResponse(*result), nil
}

func (s *playHTTPService) ClearPeerFriendInviteToken(ctx context.Context, _ clientservice.ClearPeerFriendInviteTokenRequestObject) (clientservice.ClearPeerFriendInviteTokenResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ClearFriendInviteToken(ctx, s.rpcID(), rpcapi.FriendInviteTokenClearRequest{})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ClearPeerFriendInviteToken200JSONResponse(*result), nil
}

func (s *playHTTPService) ListPeerFriendGroups(ctx context.Context, request clientservice.ListPeerFriendGroupsRequestObject) (clientservice.ListPeerFriendGroupsResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ListFriendGroups(ctx, s.rpcID(), rpcapi.FriendGroupListRequest{Cursor: request.Params.Cursor, Limit: playLimitPtr(request.Params.Limit)})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ListPeerFriendGroups200JSONResponse(*result), nil
}

func (s *playHTTPService) CreatePeerFriendGroup(ctx context.Context, request clientservice.CreatePeerFriendGroupRequestObject) (clientservice.CreatePeerFriendGroupResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.CreateFriendGroup(ctx, s.rpcID(), *request.Body)
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.CreatePeerFriendGroup200JSONResponse(*result), nil
}

func (s *playHTTPService) JoinPeerFriendGroup(ctx context.Context, request clientservice.JoinPeerFriendGroupRequestObject) (clientservice.JoinPeerFriendGroupResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.JoinFriendGroup(ctx, s.rpcID(), *request.Body)
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.JoinPeerFriendGroup200JSONResponse(*result), nil
}

func (s *playHTTPService) GetPeerFriendGroup(ctx context.Context, request clientservice.GetPeerFriendGroupRequestObject) (clientservice.GetPeerFriendGroupResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.GetFriendGroup(ctx, s.rpcID(), rpcapi.FriendGroupGetRequest{Id: request.Id})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.GetPeerFriendGroup200JSONResponse(*result), nil
}

func (s *playHTTPService) PutPeerFriendGroup(ctx context.Context, request clientservice.PutPeerFriendGroupRequestObject) (clientservice.PutPeerFriendGroupResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	body := *request.Body
	body.Id = request.Id
	result, err := c.PutFriendGroup(ctx, s.rpcID(), body)
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.PutPeerFriendGroup200JSONResponse(*result), nil
}

func (s *playHTTPService) DeletePeerFriendGroup(ctx context.Context, request clientservice.DeletePeerFriendGroupRequestObject) (clientservice.DeletePeerFriendGroupResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.DeleteFriendGroup(ctx, s.rpcID(), rpcapi.FriendGroupDeleteRequest{Id: request.Id})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.DeletePeerFriendGroup200JSONResponse(*result), nil
}

func (s *playHTTPService) GetPeerFriendGroupInviteToken(ctx context.Context, request clientservice.GetPeerFriendGroupInviteTokenRequestObject) (clientservice.GetPeerFriendGroupInviteTokenResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.GetFriendGroupInviteToken(ctx, s.rpcID(), rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupId: request.Id})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.GetPeerFriendGroupInviteToken200JSONResponse(*result), nil
}

func (s *playHTTPService) CreatePeerFriendGroupInviteToken(ctx context.Context, request clientservice.CreatePeerFriendGroupInviteTokenRequestObject) (clientservice.CreatePeerFriendGroupInviteTokenResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.CreateFriendGroupInviteToken(ctx, s.rpcID(), rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupId: request.Id})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.CreatePeerFriendGroupInviteToken200JSONResponse(*result), nil
}

func (s *playHTTPService) ClearPeerFriendGroupInviteToken(ctx context.Context, request clientservice.ClearPeerFriendGroupInviteTokenRequestObject) (clientservice.ClearPeerFriendGroupInviteTokenResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ClearFriendGroupInviteToken(ctx, s.rpcID(), rpcapi.FriendGroupInviteTokenClearRequest{FriendGroupId: request.Id})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ClearPeerFriendGroupInviteToken200JSONResponse(*result), nil
}

func (s *playHTTPService) ListPeerFriendGroupMembers(ctx context.Context, request clientservice.ListPeerFriendGroupMembersRequestObject) (clientservice.ListPeerFriendGroupMembersResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ListFriendGroupMembers(ctx, s.rpcID(), rpcapi.FriendGroupMemberListRequest{FriendGroupId: &request.Id, Cursor: request.Params.Cursor, Limit: playLimitPtr(request.Params.Limit)})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ListPeerFriendGroupMembers200JSONResponse(*result), nil
}

func (s *playHTTPService) AddPeerFriendGroupMember(ctx context.Context, request clientservice.AddPeerFriendGroupMemberRequestObject) (clientservice.AddPeerFriendGroupMemberResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	body := *request.Body
	body.FriendGroupId = request.Id
	result, err := c.AddFriendGroupMember(ctx, s.rpcID(), body)
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.AddPeerFriendGroupMember200JSONResponse(*result), nil
}

func (s *playHTTPService) PutPeerFriendGroupMember(ctx context.Context, request clientservice.PutPeerFriendGroupMemberRequestObject) (clientservice.PutPeerFriendGroupMemberResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	body := *request.Body
	body.FriendGroupId = request.Id
	body.Id = request.MemberId
	result, err := c.PutFriendGroupMember(ctx, s.rpcID(), body)
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.PutPeerFriendGroupMember200JSONResponse(*result), nil
}

func (s *playHTTPService) DeletePeerFriendGroupMember(ctx context.Context, request clientservice.DeletePeerFriendGroupMemberRequestObject) (clientservice.DeletePeerFriendGroupMemberResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.DeleteFriendGroupMember(ctx, s.rpcID(), rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: request.Id, Id: request.MemberId})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.DeletePeerFriendGroupMember200JSONResponse(*result), nil
}

func (s *playHTTPService) ListPeerPets(ctx context.Context, request clientservice.ListPeerPetsRequestObject) (clientservice.ListPeerPetsResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ListPets(ctx, s.rpcID(), rpcapi.PetListRequest{Cursor: request.Params.Cursor, Limit: playLimitValue(request.Params.Limit)})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ListPeerPets200JSONResponse(*result), nil
}

func (s *playHTTPService) AdoptPeerPet(ctx context.Context, request clientservice.AdoptPeerPetRequestObject) (clientservice.AdoptPeerPetResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.AdoptPet(ctx, s.rpcID(), *request.Body)
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.AdoptPeerPet200JSONResponse(*result), nil
}

func (s *playHTTPService) GetPeerPet(ctx context.Context, request clientservice.GetPeerPetRequestObject) (clientservice.GetPeerPetResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.GetPet(ctx, s.rpcID(), rpcapi.PetGetRequest{Id: request.Id})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.GetPeerPet200JSONResponse(*result), nil
}

func (s *playHTTPService) PutPeerPet(ctx context.Context, request clientservice.PutPeerPetRequestObject) (clientservice.PutPeerPetResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	body := *request.Body
	body.Id = request.Id
	result, err := c.PutPet(ctx, s.rpcID(), body)
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.PutPeerPet200JSONResponse(*result), nil
}

func (s *playHTTPService) DeletePeerPet(ctx context.Context, request clientservice.DeletePeerPetRequestObject) (clientservice.DeletePeerPetResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.DeletePet(ctx, s.rpcID(), rpcapi.PetDeleteRequest{Id: request.Id})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.DeletePeerPet200JSONResponse(*result), nil
}

func (s *playHTTPService) FeedPeerPet(ctx context.Context, request clientservice.FeedPeerPetRequestObject) (clientservice.FeedPeerPetResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	body := *request.Body
	body.PetId = request.Id
	result, err := c.FeedPet(ctx, s.rpcID(), body)
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.FeedPeerPet200JSONResponse(*result), nil
}

func (s *playHTTPService) WashPeerPet(ctx context.Context, request clientservice.WashPeerPetRequestObject) (clientservice.WashPeerPetResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	body := *request.Body
	body.PetId = request.Id
	result, err := c.WashPet(ctx, s.rpcID(), body)
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.WashPeerPet200JSONResponse(*result), nil
}

func (s *playHTTPService) PlayWithPeerPet(ctx context.Context, request clientservice.PlayWithPeerPetRequestObject) (clientservice.PlayWithPeerPetResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	body := *request.Body
	body.PetId = request.Id
	result, err := c.PlayPet(ctx, s.rpcID(), body)
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.PlayWithPeerPet200JSONResponse(*result), nil
}

func (s *playHTTPService) GetPeerWallet(ctx context.Context, _ clientservice.GetPeerWalletRequestObject) (clientservice.GetPeerWalletResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.GetWallet(ctx, s.rpcID(), rpcapi.WalletGetRequest{})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.GetPeerWallet200JSONResponse(*result), nil
}

func (s *playHTTPService) ListPeerWalletTransactions(ctx context.Context, request clientservice.ListPeerWalletTransactionsRequestObject) (clientservice.ListPeerWalletTransactionsResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ListWalletTransactions(ctx, s.rpcID(), rpcapi.WalletTransactionsListRequest{Cursor: request.Params.Cursor, Limit: playLimitValue(request.Params.Limit)})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ListPeerWalletTransactions200JSONResponse(*result), nil
}

func (s *playHTTPService) GetPeerWalletTransaction(ctx context.Context, request clientservice.GetPeerWalletTransactionRequestObject) (clientservice.GetPeerWalletTransactionResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.GetWalletTransaction(ctx, s.rpcID(), rpcapi.WalletTransactionsGetRequest{Id: request.Id})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.GetPeerWalletTransaction200JSONResponse(*result), nil
}

func (s *playHTTPService) ListPeerRewards(ctx context.Context, request clientservice.ListPeerRewardsRequestObject) (clientservice.ListPeerRewardsResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ListRewards(ctx, s.rpcID(), rpcapi.RewardListRequest{Cursor: request.Params.Cursor, Limit: playLimitValue(request.Params.Limit)})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ListPeerRewards200JSONResponse(*result), nil
}

func (s *playHTTPService) GetPeerReward(ctx context.Context, request clientservice.GetPeerRewardRequestObject) (clientservice.GetPeerRewardResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.GetReward(ctx, s.rpcID(), rpcapi.RewardGetRequest{Id: request.Id})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.GetPeerReward200JSONResponse(*result), nil
}

func (s *playHTTPService) ClaimPeerReward(ctx context.Context, request clientservice.ClaimPeerRewardRequestObject) (clientservice.ClaimPeerRewardResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ClaimReward(ctx, s.rpcID(), *request.Body)
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ClaimPeerReward200JSONResponse(*result), nil
}

func (s *playHTTPService) ListPeerVoices(ctx context.Context, request clientservice.ListPeerVoicesRequestObject) (clientservice.ListPeerVoicesResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := listClientVoices(ctx, c, request.Params.Cursor, request.Params.Limit, request.Params.Source, request.Params.ProviderKind, request.Params.ProviderName)
	if err != nil {
		return playHTTPErrorResponse{status: http.StatusBadGateway, message: err.Error()}, nil
	}
	return clientservice.ListPeerVoices200JSONResponse(result), nil
}

func (s *playHTTPService) ListClientVoices(ctx context.Context, request clientservice.ListClientVoicesRequestObject) (clientservice.ListClientVoicesResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := listClientVoices(ctx, c, request.Params.Cursor, request.Params.Limit, request.Params.Source, request.Params.ProviderKind, request.Params.ProviderName)
	if err != nil {
		return playHTTPErrorResponse{status: http.StatusBadGateway, message: err.Error()}, nil
	}
	return clientservice.ListClientVoices200JSONResponse(result), nil
}

func (s *playHTTPService) ListPeerWorkspaceHistory(ctx context.Context, request clientservice.ListPeerWorkspaceHistoryRequestObject) (clientservice.ListPeerWorkspaceHistoryResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.ListWorkspaceHistory(ctx, s.rpcID(), rpcapi.WorkspaceHistoryListRequest{
		WorkspaceName: request.WorkspaceName,
		Cursor:        request.Params.Cursor,
		Limit:         playLimitPtr(request.Params.Limit),
		Order:         (*rpcapi.WorkspaceHistoryListRequestOrder)(request.Params.Order),
	})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.ListPeerWorkspaceHistory200JSONResponse(*result), nil
}

func (s *playHTTPService) GetPeerWorkspaceHistory(ctx context.Context, request clientservice.GetPeerWorkspaceHistoryRequestObject) (clientservice.GetPeerWorkspaceHistoryResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	result, err := c.GetWorkspaceHistory(ctx, s.rpcID(), rpcapi.WorkspaceHistoryGetRequest{WorkspaceName: request.WorkspaceName, HistoryId: request.HistoryId})
	if err != nil {
		return playHTTPError(err), nil
	}
	return clientservice.GetPeerWorkspaceHistory200JSONResponse(*result), nil
}

func (s *playHTTPService) GetPeerWorkspaceHistoryAudio(ctx context.Context, request clientservice.GetPeerWorkspaceHistoryAudioRequestObject) (clientservice.GetPeerWorkspaceHistoryAudioResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	var audio bytes.Buffer
	result, err := c.GetWorkspaceHistoryAudio(ctx, s.rpcID(), rpcapi.WorkspaceHistoryAudioGetRequest{WorkspaceName: request.WorkspaceName, HistoryId: request.HistoryId}, &audio)
	if err != nil {
		return playHTTPError(err), nil
	}
	size := result.Metadata.SizeBytes
	if size == 0 {
		size = int64(audio.Len())
	}
	return clientservice.GetPeerWorkspaceHistoryAudio200ApplicationoctetStreamResponse{Body: bytes.NewReader(audio.Bytes()), ContentLength: size}, nil
}

func (s *playHTTPService) StreamPlayableVoices(ctx context.Context, request clientservice.StreamPlayableVoicesRequestObject) (clientservice.StreamPlayableVoicesResponseObject, error) {
	c, errResp, ok := s.gizCLIClient()
	if !ok {
		return errResp, nil
	}
	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		streamPlayableVoices(ctx, writer, c, s.invalidate, request.Params.ProviderKind, request.Params.ProviderName, request.Params.Limit)
	}()
	return clientservice.StreamPlayableVoices200TexteventStreamResponse{Body: reader}, nil
}

func (s *playHTTPService) CreateWebRTCOffer(ctx context.Context, request clientservice.CreateWebRTCOfferRequestObject) (clientservice.CreateWebRTCOfferResponseObject, error) {
	if request.Body == nil {
		return playHTTPErrorResponse{status: http.StatusBadRequest, message: "request body required"}, nil
	}
	answer, errResp, ok := createPlayWebRTCAnswer(ctx, s.client, *request.Body)
	if !ok {
		return errResp, nil
	}
	return clientservice.CreateWebRTCOffer200JSONResponse(answer), nil
}

func playLimitValue(value *int) int {
	limit := 20
	if value != nil && *value > 0 {
		limit = *value
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func playLimitPtr(value *int) *int {
	limit := playLimitValue(value)
	return &limit
}

func playRPCErrorStatus(err error) int {
	var rpcErr rpcapi.Error
	if !errors.As(err, &rpcErr) {
		return http.StatusBadGateway
	}
	switch rpcErr.Code {
	case rpcapi.RPCErrorCodeForbidden:
		return http.StatusForbidden
	case rpcapi.RPCErrorCodeNotFound:
		return http.StatusNotFound
	case rpcapi.RPCErrorCodeBadRequest:
		if strings.Contains(rpcErr.Message, "acl:") {
			return http.StatusForbidden
		}
		return http.StatusBadRequest
	case rpcapi.RPCErrorCodeInvalidParams, rpcapi.RPCErrorCodeInvalidRequest:
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
	}
}

type playHTTPErrorResponse struct {
	status  int
	message string
}

func playHTTPError(err error) playHTTPErrorResponse {
	return playHTTPErrorResponse{status: playRPCErrorStatus(err), message: err.Error()}
}

func (r playHTTPErrorResponse) write(ctx *fiber.Ctx) error {
	status := r.status
	if status == 0 {
		status = http.StatusBadGateway
	}
	ctx.Status(status)
	return ctx.SendString(r.message)
}

func (r playHTTPErrorResponse) VisitListPeerCredentialsResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitListPeerContactsResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitCreatePeerContactResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitGetPeerContactResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitPutPeerContactResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitDeletePeerContactResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitListPeerFriendsResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitAddPeerFriendResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitDeletePeerFriendResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitGetPeerFriendInviteTokenResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitCreatePeerFriendInviteTokenResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitClearPeerFriendInviteTokenResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitListPeerFriendGroupsResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitCreatePeerFriendGroupResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitJoinPeerFriendGroupResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitGetPeerFriendGroupResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitPutPeerFriendGroupResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitDeletePeerFriendGroupResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitGetPeerFriendGroupInviteTokenResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitCreatePeerFriendGroupInviteTokenResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitClearPeerFriendGroupInviteTokenResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitListPeerFriendGroupMembersResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitAddPeerFriendGroupMemberResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitPutPeerFriendGroupMemberResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitDeletePeerFriendGroupMemberResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitListPeerWorkspaceHistoryResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitGetPeerWorkspaceHistoryResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitGetPeerWorkspaceHistoryAudioResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitListPeerModelsResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitListPeerFirmwaresResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitListPeerPetsResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitAdoptPeerPetResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitDeletePeerPetResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitGetPeerPetResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitPutPeerPetResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitFeedPeerPetResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitPlayWithPeerPetResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitWashPeerPetResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitListPeerRewardsResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitClaimPeerRewardResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitGetPeerRewardResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitListPeerVoicesResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitGetPeerWalletResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitListPeerWalletTransactionsResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitGetPeerWalletTransactionResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitListPeerWorkflowsResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitListPeerWorkspacesResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitStreamPlayableVoicesResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitListClientVoicesResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func (r playHTTPErrorResponse) VisitCreateWebRTCOfferResponse(ctx *fiber.Ctx) error {
	return r.write(ctx)
}

func sanitizePlayCredentialList(result *rpcapi.CredentialListResponse) *rpcapi.CredentialListResponse {
	if result == nil {
		return nil
	}
	out := *result
	out.Items = append([]rpcapi.Credential(nil), result.Items...)
	for i := range out.Items {
		out.Items[i].Body = rpcapi.CredentialBody{}
	}
	return &out
}

func streamPlayableVoices(ctx context.Context, w io.Writer, c *gizcli.Client, invalidate ClientInvalidator, providerKind *apitypes.VoiceProviderKind, providerName *string, limitParam *int) {
	limit := playLimitValue(limitParam)
	cursor := ""
	for page := 0; page < 10; page++ {
		list, err := fetchPlayVoicePage(ctx, c, cursor, limit, nil, nil, nil)
		if err != nil {
			if invalidate != nil {
				invalidate(c)
			}
			writePlayVoiceStreamEvent(w, clientservice.PlayVoiceStreamEvent{Error: ptr(err.Error())})
			return
		}
		for _, voice := range playVoiceListItems(list) {
			if !playVoiceMatches(voice, nil, providerKind, providerName) {
				continue
			}
			voice := voice
			writePlayVoiceStreamEvent(w, clientservice.PlayVoiceStreamEvent{Voice: &voice})
		}
		if !list.HasNext || list.NextCursor == nil || strings.TrimSpace(*list.NextCursor) == "" {
			break
		}
		cursor = *list.NextCursor
	}
	writePlayVoiceStreamEvent(w, clientservice.PlayVoiceStreamEvent{Done: ptr(true)})
}

func listClientVoices(
	ctx context.Context,
	c *gizcli.Client,
	cursor *string,
	limit *int,
	source *apitypes.VoiceSource,
	providerKind *apitypes.VoiceProviderKind,
	providerName *string,
) (clientservice.ClientVoiceListResponse, error) {
	cursorValue := ""
	if cursor != nil {
		cursorValue = *cursor
	}
	list, err := fetchPlayVoicePage(ctx, c, cursorValue, playLimitValue(limit), source, providerKind, providerName)
	if err != nil {
		return clientservice.ClientVoiceListResponse{}, err
	}
	filtered := make([]apitypes.Voice, 0, len(list.Data))
	for _, voice := range playVoiceListItems(list) {
		if playVoiceMatches(voice, source, providerKind, providerName) {
			filtered = append(filtered, voice)
		}
	}
	list.Data = filtered
	list.Items = &filtered
	if list.Object == "" {
		list.Object = clientservice.List
	}
	return list, nil
}

func fetchPlayVoicePage(
	ctx context.Context,
	c *gizcli.Client,
	cursor string,
	limit int,
	source *apitypes.VoiceSource,
	providerKind *apitypes.VoiceProviderKind,
	providerName *string,
) (clientservice.ClientVoiceListResponse, error) {
	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	if source != nil {
		query.Set("source", string(*source))
	}
	if providerKind != nil {
		query.Set("provider_kind", string(*providerKind))
	}
	if providerName != nil {
		query.Set("provider_name", *providerName)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://gizclaw/v1/voices?"+query.Encode(), nil)
	if err != nil {
		return clientservice.ClientVoiceListResponse{}, err
	}
	resp, err := openAIHTTPClient(c).Do(req)
	if err != nil {
		return clientservice.ClientVoiceListResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return clientservice.ClientVoiceListResponse{}, fmt.Errorf("list voices failed: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out clientservice.ClientVoiceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return clientservice.ClientVoiceListResponse{}, err
	}
	return out, nil
}

func playVoiceListItems(list clientservice.ClientVoiceListResponse) []apitypes.Voice {
	items := append([]apitypes.Voice(nil), list.Data...)
	if list.Items != nil {
		items = append(items, (*list.Items)...)
	}
	return items
}

func playVoiceMatches(voice apitypes.Voice, source *apitypes.VoiceSource, providerKind *apitypes.VoiceProviderKind, providerName *string) bool {
	if source != nil && voice.Source != *source {
		return false
	}
	if providerKind != nil && voice.Provider.Kind != *providerKind {
		return false
	}
	if providerName != nil && voice.Provider.Name != *providerName {
		return false
	}
	return true
}

func writePlayVoiceStreamEvent(w io.Writer, event clientservice.PlayVoiceStreamEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		data = []byte(`{"error":"encode voice stream event failed"}`)
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

func ptr[T any](value T) *T {
	return &value
}

func createPlayWebRTCAnswer(ctx context.Context, client ClientProvider, req clientservice.WebRTCSessionDescription) (clientservice.WebRTCSessionDescription, playHTTPErrorResponse, bool) {
	if req.Type != clientservice.Offer || strings.TrimSpace(req.Sdp) == "" {
		return clientservice.WebRTCSessionDescription{}, playHTTPErrorResponse{status: http.StatusBadRequest, message: "invalid webrtc offer"}, false
	}
	c, err := client()
	if err != nil {
		return clientservice.WebRTCSessionDescription{}, playWebRTCError("get client failed", err, http.StatusServiceUnavailable), false
	}
	if err := reloadPlayRunForWebRTC(ctx, c); err != nil {
		return clientservice.WebRTCSessionDescription{}, playWebRTCError("reload peer run failed", err, http.StatusBadGateway), false
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return clientservice.WebRTCSessionDescription{}, playWebRTCError("create peer connection failed", err, http.StatusInternalServerError), false
	}
	registration, err := c.RegisterTo(pc)
	if err != nil {
		_ = pc.Close()
		return clientservice.WebRTCSessionDescription{}, playWebRTCError("register peer connection failed", err, http.StatusInternalServerError), false
	}
	closeWebRTC := func() {
		_ = registration.Close()
		_ = pc.Close()
	}
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateFailed,
			webrtc.PeerConnectionStateDisconnected,
			webrtc.PeerConnectionStateClosed:
			closeWebRTC()
		}
	})

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: req.Sdp}); err != nil {
		closeWebRTC()
		return clientservice.WebRTCSessionDescription{}, playWebRTCError("set remote description failed", err, http.StatusBadRequest), false
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		closeWebRTC()
		return clientservice.WebRTCSessionDescription{}, playWebRTCError("create answer failed", err, http.StatusInternalServerError), false
	}

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		closeWebRTC()
		return clientservice.WebRTCSessionDescription{}, playWebRTCError("set local description failed", err, http.StatusInternalServerError), false
	}
	select {
	case <-gatherComplete:
	case <-ctx.Done():
		closeWebRTC()
		return clientservice.WebRTCSessionDescription{}, playHTTPErrorResponse{status: http.StatusBadGateway, message: ctx.Err().Error()}, false
	}

	local := pc.LocalDescription()
	if local == nil {
		closeWebRTC()
		return clientservice.WebRTCSessionDescription{}, playWebRTCError("missing local description", fmt.Errorf("local description is nil"), http.StatusInternalServerError), false
	}

	return clientservice.WebRTCSessionDescription{
		Sdp:  local.SDP,
		Type: clientservice.WebRTCSessionDescriptionType(local.Type.String()),
	}, playHTTPErrorResponse{}, true
}

type playWebRTCRunReloader interface {
	GetServerRunWorkspace(context.Context, string) (*rpcapi.ServerGetRunWorkspaceResponse, error)
	ReloadServerRunWorkspace(context.Context, string) (*rpcapi.ServerReloadRunWorkspaceResponse, error)
	ReloadServerRun(context.Context, string) (*rpcapi.ServerReloadRunResponse, error)
}

func reloadPlayRunForWebRTC(ctx context.Context, c playWebRTCRunReloader) error {
	if c == nil {
		return fmt.Errorf("client is required")
	}
	state, err := c.GetServerRunWorkspace(ctx, "play-webrtc-workspace-state")
	if err == nil && playWebRTCWorkspaceName(state) != "" {
		_, err = c.ReloadServerRunWorkspace(ctx, "play-webrtc-workspace-reload")
		return err
	}
	if err != nil && !strings.Contains(err.Error(), "not configured") {
		return err
	}
	_, err = c.ReloadServerRun(ctx, "play-webrtc-reload")
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "not configured") {
		return nil
	}
	return err
}

func playWebRTCWorkspaceName(state *rpcapi.ServerGetRunWorkspaceResponse) string {
	if state == nil {
		return ""
	}
	for _, value := range []string{
		stringPtrValue(state.ActiveWorkspaceName),
		state.WorkspaceName,
		stringPtrValue(state.SelectedWorkspaceName),
		stringPtrValue(state.PendingWorkspaceName),
	} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func playWebRTCError(message string, err error, status int) playHTTPErrorResponse {
	slog.Error("gizclaw: play webrtc signaling failed", "message", message, "error", err, "status", status)
	return playHTTPErrorResponse{status: status, message: fmt.Sprintf("%s: %v", message, err)}
}
