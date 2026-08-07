//go:build gizclaw_e2e

package delete_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

const (
	deleteAdminContext = "delete-admin"
	deleteProfileID    = "default-gameplay"
	deleteCollection   = "deletion"
	deleteWorkflowName = "workspace"
	deleteWorkflowID   = "flowcraft-scenario-000"
)

type deletionHarness struct {
	ctx   context.Context
	h     *clitest.Harness
	admin *gizcli.Client
	api   *adminhttp.ClientWithResponses
}

type deletionPeer struct {
	contextName string
	publicKey   string
	serial      string
	client      *gizcli.Client
}

type activeDeletionTransform struct {
	cancel context.CancelFunc
	input  *activeDeletionInput
	output genx.Stream
	done   chan error
}

type activeDeletionInput struct {
	chunks      chan *genx.MessageChunk
	closed      chan struct{}
	consumed    chan struct{}
	endConsumed chan struct{}
	consumeOnce sync.Once
	endOnce     sync.Once
	closeOnce   sync.Once
}

func newDeletionHarness(t *testing.T) *deletionHarness {
	t.Helper()
	sandbox := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_ADMIN_SANDBOX"))
	var h *clitest.Harness
	if sandbox == "" {
		h = clitest.NewSetupHarness(t, "client-delete")
	} else {
		h = clitest.NewPersistentHarnessForRoot(t, "tests/gizclaw-e2e/cmd", "client-delete", sandbox)
		h.UseSetupServer()
	}
	h.InstallFixedAdminContext(deleteAdminContext).MustSucceed(t)
	h.RequireAdminContextEndpoint(deleteAdminContext)
	admin := h.ConnectClientFromContextEventually(deleteAdminContext, 30*time.Second)
	api, err := admin.ServerAdminClient()
	if err != nil {
		_ = admin.Close()
		t.Fatalf("create deletion Admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	env := &deletionHarness{ctx: ctx, h: h, admin: admin, api: api}
	t.Cleanup(func() { _ = env.admin.Close() })
	env.ensureRuntimeProfile(t)
	return env
}

func (e *deletionHarness) ensureRuntimeProfile(t *testing.T) {
	t.Helper()
	profile, found, err := clitest.RuntimeProfileByID(e.ctx, e.api, deleteProfileID)
	if err != nil || !found {
		t.Fatalf("resolve deletion RuntimeProfile: found=%v error=%v", found, err)
	}
	if profile.Spec.Workflows.Collections == nil {
		profile.Spec.Workflows.Collections = apitypes.RuntimeProfileWorkflowCollections{}
	}
	profile.Spec.Workflows.Collections[deleteCollection] = map[string]apitypes.RuntimeProfileBinding{
		deleteWorkflowName: {
			ResourceId: deleteWorkflowID,
			I18n: map[string]apitypes.RuntimeProfileI18nText{
				"en":    {DisplayName: "Deletion Workspace"},
				"zh-CN": {DisplayName: "删除工作区"},
			},
		},
	}
	if _, err := clitest.UpsertRuntimeProfile(e.ctx, e.api, adminhttp.RuntimeProfileUpsert{Id: profile.Id, Spec: profile.Spec}); err != nil {
		t.Fatalf("update deletion RuntimeProfile: %v", err)
	}
}

func (e *deletionHarness) newPeer(t *testing.T, contextName string) deletionPeer {
	t.Helper()
	e.h.CreateContext(contextName).MustSucceed(t)
	e.h.RequireClientContextEndpoint(contextName)
	publicKey := e.h.ContextPublicKey(contextName)
	serial := "client-" + contextName + "-" + publicKey
	e.h.RegisterContext(contextName, "--sn", serial).MustSucceed(t)
	// RegisterContext uses the fixed setup Admin identity. Its new connection
	// intentionally replaces the harness' existing Admin session, so reconnect
	// before provisioning the RuntimeProfile token.
	e.reconnectAdmin(t)
	tokenID := "e2e-delete-token-" + contextName
	if err := clitest.DeleteRegistrationTokenByID(e.ctx, e.api, tokenID); err != nil {
		t.Fatalf("retire old deletion RegistrationToken: %v", err)
	}
	token, err := e.api.CreateRegistrationTokenWithResponse(e.ctx, adminhttp.RegistrationTokenUpsert{
		Id: tokenID, Token: tokenID, RuntimeProfileId: deleteProfileID,
	})
	if err != nil || token.JSON200 == nil {
		t.Fatalf("create deletion RegistrationToken: status=%d body=%s error=%v", token.StatusCode(), token.Body, err)
	}
	client := e.h.ConnectClientFromContext(contextName)
	t.Cleanup(func() { _ = client.Close() })
	registered, err := client.Register(e.ctx, "delete.register."+contextName, token.JSON200.Token)
	if err != nil {
		t.Fatalf("register deletion Peer %q: %v", contextName, err)
	}
	if registered.RuntimeProfileName != deleteProfileID {
		t.Fatalf("registered RuntimeProfile = %q, want %q", registered.RuntimeProfileName, deleteProfileID)
	}
	return deletionPeer{contextName: contextName, publicKey: publicKey, serial: serial, client: client}
}

func (e *deletionHarness) reconnectAdmin(t *testing.T) {
	t.Helper()
	_ = e.admin.Close()
	e.admin = e.h.ConnectClientFromContextEventually(deleteAdminContext, 30*time.Second)
	api, err := e.admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("reconnect deletion Admin client: %v", err)
	}
	e.api = api
}

func deletionWorkspaceParameters(t *testing.T) *rpcapi.WorkspaceParameters {
	t.Helper()
	input := rpcapi.WorkspaceInputModePushToTalk
	var parameters rpcapi.WorkspaceParameters
	if err := parameters.FromFlowcraftWorkspaceParameters(rpcapi.FlowcraftWorkspaceParameters{
		AgentType: rpcapi.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
		Input:     &input,
	}); err != nil {
		t.Fatal(err)
	}
	return &parameters
}

func (e *deletionHarness) createWorkspace(t *testing.T, peer deletionPeer, name string) (rpcapi.Workspace, apitypes.Workspace) {
	t.Helper()
	created, err := peer.client.CreateWorkspace(e.ctx, "delete.workspace.create."+name, rpcapi.WorkspaceCreateRequest{
		Name: name, Collection: deleteCollection, WorkflowName: deleteWorkflowName,
		Parameters: deletionWorkspaceParameters(t),
	})
	if err != nil {
		t.Fatalf("create deletion Workspace: %v", err)
	}
	stored, found, err := clitest.WorkspaceByName(e.ctx, e.api, created.Name)
	if err != nil || !found {
		t.Fatalf("resolve deletion Workspace: found=%v error=%v", found, err)
	}
	return *created, stored
}

func (e *deletionHarness) startWorkspace(t *testing.T, peer deletionPeer, workspaceName string) {
	t.Helper()
	if _, err := peer.client.SetServerRunWorkspace(e.ctx, "delete.workspace.select."+workspaceName, rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: workspaceName}); err != nil {
		t.Fatalf("select active Workspace: %v", err)
	}
	state, err := peer.client.ReloadServerRunWorkspace(e.ctx, "delete.workspace.reload."+workspaceName)
	if err != nil {
		t.Fatalf("start active Workspace: %v", err)
	}
	if state.RuntimeState != rpcapi.PeerRunStatusStateRunning || state.ActiveWorkspaceName == nil || *state.ActiveWorkspaceName != workspaceName {
		t.Fatalf("active Workspace state = %#v, want running %q", state, workspaceName)
	}
}

func (e *deletionHarness) startActiveTransform(t *testing.T, peer deletionPeer) *activeDeletionTransform {
	t.Helper()
	input := &activeDeletionInput{
		chunks:      make(chan *genx.MessageChunk, 1),
		closed:      make(chan struct{}),
		consumed:    make(chan struct{}),
		endConsumed: make(chan struct{}),
	}
	input.chunks <- &genx.MessageChunk{
		Role: genx.RoleUser,
		Name: "transcript",
		Part: genx.Text("keep this deletion test stream active"),
		Ctrl: &genx.StreamCtrl{StreamID: "delete-active", Label: "transcript"},
	}
	runCtx, cancel := context.WithCancel(context.Background())
	output, err := peer.client.Transform(runCtx, input)
	if err != nil {
		cancel()
		t.Fatalf("start active Workspace Transform: %v", err)
	}
	active := &activeDeletionTransform{cancel: cancel, input: input, output: output, done: make(chan error, 1)}
	go func() {
		for {
			chunk, err := output.Next()
			if err != nil {
				active.done <- err
				return
			}
			if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.StreamID == "delete-active" && chunk.Ctrl.EndOfStream {
				if chunk.Ctrl.Error == "" {
					active.done <- nil
				} else {
					active.done <- fmt.Errorf("%s", chunk.Ctrl.Error)
				}
				return
			}
		}
	}()
	t.Cleanup(active.close)
	select {
	case <-input.consumed:
	case <-time.After(10 * time.Second):
		t.Fatal("active Workspace Transform did not consume its first input chunk")
	}
	select {
	case err := <-active.done:
		t.Fatalf("Workspace Transform terminated before deletion: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	return active
}

func (a *activeDeletionTransform) requireTerminated(t *testing.T, ctx context.Context, label string) {
	t.Helper()
	select {
	case err := <-a.done:
		if err == nil {
			t.Fatalf("%s Transform ended without a terminal error", label)
		}
		return
	default:
	}
	a.input.chunks <- &genx.MessageChunk{
		Role: genx.RoleUser,
		Name: "transcript",
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: "delete-active", Label: "transcript", EndOfStream: true},
	}
	select {
	case <-a.input.endConsumed:
		a.input.Close()
	case <-time.After(10 * time.Second):
		t.Fatalf("%s active stream did not consume its post-delete EOS", label)
	}
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case err := <-a.done:
		if err == nil {
			t.Fatalf("%s Transform ended without a terminal error", label)
		}
	case <-timer.C:
		t.Fatalf("%s active stream accepted post-delete use instead of terminating", label)
	case <-ctx.Done():
		t.Fatalf("%s Transform remained active after deletion: %v", label, ctx.Err())
	}
}

func (a *activeDeletionTransform) close() {
	if a == nil {
		return
	}
	a.cancel()
	_ = a.input.Close()
	_ = a.output.Close()
}

func (s *activeDeletionInput) Next() (*genx.MessageChunk, error) {
	select {
	case chunk := <-s.chunks:
		s.consumeOnce.Do(func() { close(s.consumed) })
		if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.EndOfStream {
			s.endOnce.Do(func() { close(s.endConsumed) })
		}
		return chunk, nil
	case <-s.closed:
		return nil, genx.ErrDone
	}
}

func (s *activeDeletionInput) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

func (s *activeDeletionInput) CloseWithError(error) error { return s.Close() }

var _ genx.Stream = (*activeDeletionInput)(nil)

func (e *deletionHarness) runStatus(peer deletionPeer, requestID string) (*rpcapi.ServerGetRunStatusResponse, error) {
	ctx, cancel := context.WithTimeout(e.ctx, 3*time.Second)
	defer cancel()
	return peer.client.GetServerRunStatus(ctx, requestID)
}

func (e *deletionHarness) waitRunStopped(t *testing.T, peer deletionPeer, requestID string) {
	t.Helper()
	waitUntil(t, e.ctx, "Peer runtime stop", func() (bool, string) {
		status, err := e.runStatus(peer, requestID)
		if err != nil {
			return false, err.Error()
		}
		return status.State == rpcapi.PeerRunStatusStateStopped, fmt.Sprintf("status=%#v", status)
	})
}

func (e *deletionHarness) waitWorkspaceAbsent(t *testing.T, workspaceID string) {
	t.Helper()
	waitUntil(t, e.ctx, "Workspace deletion", func() (bool, string) {
		response, err := e.api.GetWorkspaceWithResponse(e.ctx, workspaceID)
		if err != nil {
			return false, err.Error()
		}
		return response.StatusCode() == http.StatusNotFound, fmt.Sprintf("status=%d body=%s", response.StatusCode(), response.Body)
	})
}

func (e *deletionHarness) findFriendGroup(t *testing.T, owner, name string) adminhttp.AdminFriendGroupObject {
	t.Helper()
	limit := 200
	response, err := e.api.ListFriendGroupsWithResponse(e.ctx, &adminhttp.ListFriendGroupsParams{Limit: &limit})
	if err != nil || response.JSON200 == nil {
		t.Fatalf("list Friend Groups: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	for _, group := range response.JSON200.Items {
		if group.CreatedByPeerPublicKey == owner && group.Name == name {
			return group
		}
	}
	t.Fatalf("Friend Group owner=%q name=%q not found", owner, name)
	return adminhttp.AdminFriendGroupObject{}
}

func waitUntil(t *testing.T, ctx context.Context, label string, ready func() (bool, string)) {
	t.Helper()
	for {
		if ok, _ := ready(); ok {
			return
		}
		select {
		case <-ctx.Done():
			_, detail := ready()
			t.Fatalf("%s did not complete: %s: %v", label, detail, ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func assertNoPendingDeletion(t *testing.T, env *deletionHarness, source string, kind apitypes.PendingDeletionKind, resourceID string) {
	t.Helper()
	response, err := env.api.ListPendingDeletionsWithResponse(env.ctx, &adminhttp.ListPendingDeletionsParams{Source: &source, Kind: &kind})
	if err != nil || response.JSON200 == nil {
		t.Fatalf("list %s/%s pending deletions: status=%d body=%s error=%v", source, kind, response.StatusCode(), response.Body, err)
	}
	for _, task := range response.JSON200.Items {
		if task.ResourceId == resourceID {
			t.Fatalf("completed %s/%s deletion retained task: %#v", source, kind, task)
		}
	}
}
