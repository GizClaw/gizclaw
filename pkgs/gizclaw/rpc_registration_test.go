package gizclaw

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestRPCRegistrationReplacesSnapshotAndRejectedTokenPreservesIt(t *testing.T) {
	t.Parallel()
	registrations, tokenA := registrationServerAndToken(t, "profile-a")
	tokenB := createRegistrationToken(t, registrations, "profile-b")
	var snapshot atomic.Pointer[runtimeprofile.Registration]
	server := &rpcServer{
		registrations:   registrations,
		callerPublicKey: giznet.PublicKey{1},
		onRegistration: func(registration runtimeprofile.Registration) {
			snapshot.Store(&registration)
		},
	}

	response := registerRPC(t, server, tokenA)
	if response.RuntimeProfileName != "profile-a" || response.FirmwareID != nil {
		t.Fatalf("first registration = %#v", response)
	}
	if got := snapshot.Load(); got == nil || got.RuntimeProfile.Name != "profile-a" {
		t.Fatalf("first snapshot = %#v", got)
	}
	bound, err := registrations.ResolveOwnerProfile(t.Context(), server.callerPublicKey.String())
	if err != nil || bound.Name != "profile-a" {
		t.Fatalf("ResolveOwnerProfile() = %#v, %v", bound, err)
	}

	rejected, err := server.dispatch(context.Background(), registrationRequest("invalid-token"))
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Error == nil || rejected.Error.Code != rpcapi.RPCErrorCodeForbidden {
		t.Fatalf("invalid registration response = %#v", rejected)
	}
	if got := snapshot.Load(); got == nil || got.RuntimeProfile.Name != "profile-a" {
		t.Fatalf("rejected registration replaced snapshot: %#v", got)
	}

	response = registerRPC(t, server, tokenB)
	if response.RuntimeProfileName != "profile-b" {
		t.Fatalf("second registration = %#v", response)
	}
	if got := snapshot.Load(); got == nil || got.RuntimeProfile.Name != "profile-b" {
		t.Fatalf("second snapshot = %#v", got)
	}
	bound, err = registrations.ResolveOwnerProfile(t.Context(), server.callerPublicKey.String())
	if err != nil || bound.Name != "profile-b" {
		t.Fatalf("ResolveOwnerProfile(updated) = %#v, %v", bound, err)
	}
}

func TestRPCRegistrationSnapshotIsRaceSafe(t *testing.T) {
	registrations, tokenA := registrationServerAndToken(t, "profile-a")
	tokenB := createRegistrationToken(t, registrations, "profile-b")
	var snapshot atomic.Pointer[runtimeprofile.Registration]
	server := &rpcServer{registrations: registrations, onRegistration: func(registration runtimeprofile.Registration) {
		snapshot.Store(&registration)
	}}

	var wg sync.WaitGroup
	for i := range 32 {
		token := tokenA
		if i%2 == 1 {
			token = tokenB
		}
		wg.Go(func() {
			response, err := server.dispatch(context.Background(), registrationRequest(token))
			if err != nil || response.Error != nil {
				t.Errorf("concurrent registration = %#v, %v", response, err)
			}
		})
	}
	wg.Wait()

	registerRPC(t, server, tokenA)
	if got := snapshot.Load(); got == nil || got.RuntimeProfile.Name != "profile-a" {
		t.Fatalf("last successful registration snapshot = %#v", got)
	}
}

func TestRPCRegistrationSerializesOwnerBindingAndSnapshot(t *testing.T) {
	registrations, tokenA := registrationServerAndToken(t, "profile-a")
	tokenB := createRegistrationToken(t, registrations, "profile-b")
	bPersisted := make(chan struct{})
	registrations.Store = &ownerBindingObserver{
		Store:       registrations.Store,
		profileName: "profile-b",
		persisted:   bPersisted,
	}
	var snapshot atomic.Pointer[runtimeprofile.Registration]
	aPublishing := make(chan struct{})
	releaseA := make(chan struct{})
	publicKey := giznet.PublicKey{6}
	server := &rpcServer{
		registrations:   registrations,
		callerPublicKey: publicKey,
		onRegistration: func(registration runtimeprofile.Registration) {
			if registration.RuntimeProfile.Name == "profile-a" {
				close(aPublishing)
				<-releaseA
			}
			snapshot.Store(&registration)
		},
	}
	aDone := make(chan struct{})
	go func() {
		defer close(aDone)
		response, err := server.dispatch(context.Background(), registrationRequest(tokenA))
		if err != nil || response.Error != nil {
			t.Errorf("register profile-a = %#v, %v", response, err)
		}
	}()
	<-aPublishing
	bDone := make(chan struct{})
	bStarted := make(chan struct{})
	go func() {
		defer close(bDone)
		close(bStarted)
		response, err := server.dispatch(context.Background(), registrationRequest(tokenB))
		if err != nil || response.Error != nil {
			t.Errorf("register profile-b = %#v, %v", response, err)
		}
	}()
	<-bStarted
	select {
	case <-bPersisted:
		t.Fatal("profile-b owner binding crossed profile-a snapshot publication")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseA)
	<-aDone
	<-bDone

	bound, err := registrations.ResolveOwnerProfile(t.Context(), publicKey.String())
	if err != nil {
		t.Fatalf("ResolveOwnerProfile() error = %v", err)
	}
	active := snapshot.Load()
	if active == nil || active.RuntimeProfile.Name != bound.Name {
		t.Fatalf("snapshot = %#v, owner binding = %#v", active, bound)
	}
}

func TestRPCRegistrationPersistsAndReturnsFirmwareReleaseLine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	registrations := &runtimeprofile.Server{
		Store: kv.NewMemory(nil),
		ResolveResource: func(_ context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
			if kind != apitypes.ResourceKindFirmware || name != "h106" {
				return apitypes.Resource{}, kv.ErrNotFound
			}
			var resource apitypes.Resource
			err := resource.FromFirmwareResource(apitypes.FirmwareResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.FirmwareResourceKindFirmware,
				Metadata:   apitypes.ResourceMetadata{Name: name},
			})
			return resource, err
		},
	}
	installTestSystemWorkflowResolver(registrations)
	profileName := "h106-production"
	profileResponse, err := registrations.PutRuntimeProfile(ctx, adminhttp.PutRuntimeProfileRequestObject{
		Name: profileName,
		Body: &adminhttp.RuntimeProfileUpsert{Name: profileName, Spec: apitypes.RuntimeProfileSpec{
			Workflows: testRuntimeProfileWorkflows(),
			Resources: apitypes.RuntimeProfileResources{},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := profileResponse.(adminhttp.PutRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("PutRuntimeProfile() = %#v", profileResponse)
	}
	firmwareID := "h106"
	tokenResponse, err := registrations.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Name: "h106-token", RuntimeProfileName: profileName, FirmwareId: &firmwareID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := tokenResponse.(adminhttp.CreateRegistrationToken200JSONResponse)
	if !ok {
		t.Fatalf("CreateRegistrationToken() = %#v", tokenResponse)
	}
	publicKey := giznet.PublicKey{9}
	peers := &peer.Server{Store: kv.NewMemory(nil)}
	if _, err := peers.EnsureConnectedPeer(ctx, publicKey); err != nil {
		t.Fatal(err)
	}
	var snapshot atomic.Pointer[runtimeprofile.Registration]
	server := &rpcServer{
		registrations:   registrations,
		peer:            peers,
		callerPublicKey: publicKey,
		onRegistration: func(registration runtimeprofile.Registration) {
			snapshot.Store(&registration)
		},
	}
	response := registerRPC(t, server, created.Token)
	if response.RuntimeProfileName != profileName || response.FirmwareID == nil || *response.FirmwareID != firmwareID {
		t.Fatalf("server.register = %#v", response)
	}
	stored, err := peers.LoadPeer(ctx, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if stored.FirmwareId == nil || *stored.FirmwareId != firmwareID {
		t.Fatalf("stored Peer = %#v, want firmware_id=%q", stored, firmwareID)
	}
	if active := snapshot.Load(); active == nil || active.FirmwareID == nil || *active.FirmwareID != firmwareID {
		t.Fatalf("active registration = %#v", active)
	}
}

func TestRPCRegistrationFirmwareBindingFailurePreservesSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	registrations := firmwareRegistrationServer(t, "h106-production", "h106")
	firmwareID := "h106"
	tokenResponse, err := registrations.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Name: "h106-token", RuntimeProfileName: "h106-production", FirmwareId: &firmwareID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	created := tokenResponse.(adminhttp.CreateRegistrationToken200JSONResponse)
	var snapshot atomic.Pointer[runtimeprofile.Registration]
	snapshot.Store(&runtimeprofile.Registration{RuntimeProfile: apitypes.RuntimeProfile{Name: "previous-profile"}})
	server := &rpcServer{
		registrations:   registrations,
		peer:            rejectingFirmwarePeer{},
		callerPublicKey: giznet.PublicKey{7},
		onRegistration: func(registration runtimeprofile.Registration) {
			snapshot.Store(&registration)
		},
	}
	_ = createRegistrationToken(t, registrations, "previous-profile")
	if err := registrations.BindOwnerProfile(ctx, server.callerPublicKey.String(), "previous-profile"); err != nil {
		t.Fatalf("BindOwnerProfile(previous) error = %v", err)
	}

	response, err := server.dispatch(ctx, registrationRequest(created.Token))
	if err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeInternalError {
		t.Fatalf("server.register = %#v, want internal error", response)
	}
	if active := snapshot.Load(); active == nil || active.RuntimeProfile.Name != "previous-profile" {
		t.Fatalf("failed firmware binding replaced snapshot: %#v", active)
	}
	bound, err := registrations.ResolveOwnerProfile(ctx, server.callerPublicKey.String())
	if err != nil || bound.Name != "previous-profile" {
		t.Fatalf("failed firmware binding replaced owner profile: %#v, %v", bound, err)
	}
}

func TestRPCRegistrationOwnerProfileBindingFailurePreservesFirmware(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	registrations := firmwareRegistrationServer(t, "h106-production", "h106")
	_ = createRegistrationToken(t, registrations, "previous-profile")
	publicKey := giznet.PublicKey{8}
	if err := registrations.BindOwnerProfile(ctx, publicKey.String(), "previous-profile"); err != nil {
		t.Fatalf("BindOwnerProfile(previous) error = %v", err)
	}
	firmwareID := "h106"
	tokenResponse, err := registrations.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Name: "h106-token", RuntimeProfileName: "h106-production", FirmwareId: &firmwareID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	created := tokenResponse.(adminhttp.CreateRegistrationToken200JSONResponse)

	peers := &peer.Server{Store: kv.NewMemory(nil)}
	if _, err := peers.EnsureConnectedPeer(ctx, publicKey); err != nil {
		t.Fatal(err)
	}
	if _, err := peers.BindFirmware(ctx, publicKey, "previous-firmware"); err != nil {
		t.Fatal(err)
	}
	registrations.Store = rejectingOwnerBindingStore{Store: registrations.Store}
	server := &rpcServer{registrations: registrations, peer: peers, callerPublicKey: publicKey}

	response, err := server.dispatch(ctx, registrationRequest(created.Token))
	if err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeInternalError {
		t.Fatalf("server.register = %#v, want internal error", response)
	}
	stored, err := peers.LoadPeer(ctx, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if stored.FirmwareId == nil || *stored.FirmwareId != "previous-firmware" {
		t.Fatalf("failed owner binding replaced firmware: %#v", stored)
	}
	bound, err := registrations.ResolveOwnerProfile(ctx, publicKey.String())
	if err != nil || bound.Name != "previous-profile" {
		t.Fatalf("failed owner binding replaced owner profile: %#v, %v", bound, err)
	}
}

type rejectingFirmwarePeer struct{}

type rejectingOwnerBindingStore struct {
	kv.Store
}

type ownerBindingObserver struct {
	kv.Store
	profileName string
	persisted   chan<- struct{}
	once        sync.Once
}

func (s *ownerBindingObserver) Set(ctx context.Context, key kv.Key, value []byte) error {
	if err := s.Store.Set(ctx, key, value); err != nil {
		return err
	}
	if strings.Contains(key.String(), "by-owner") && string(value) == s.profileName {
		s.once.Do(func() {
			close(s.persisted)
		})
	}
	return nil
}

func (s rejectingOwnerBindingStore) Set(ctx context.Context, key kv.Key, value []byte) error {
	if strings.Contains(key.String(), "by-owner") {
		return errors.New("owner binding store unavailable")
	}
	return s.Store.Set(ctx, key, value)
}

func (rejectingFirmwarePeer) GetSelfInfo(context.Context, giznet.PublicKey) (apitypes.DeviceInfo, error) {
	return apitypes.DeviceInfo{}, nil
}

func (rejectingFirmwarePeer) PutSelfInfo(context.Context, giznet.PublicKey, apitypes.DeviceInfo) (apitypes.DeviceInfo, error) {
	return apitypes.DeviceInfo{}, nil
}

func (rejectingFirmwarePeer) GetSelfRuntime(context.Context, giznet.PublicKey) apitypes.Runtime {
	return apitypes.Runtime{}
}

func (rejectingFirmwarePeer) BindFirmware(context.Context, giznet.PublicKey, string) (apitypes.Peer, error) {
	return apitypes.Peer{}, errors.New("store unavailable")
}

func (rejectingFirmwarePeer) DeleteSelf(context.Context, giznet.PublicKey) error {
	return nil
}

func firmwareRegistrationServer(t *testing.T, profileName, firmwareID string) *runtimeprofile.Server {
	t.Helper()
	server := &runtimeprofile.Server{
		Store: kv.NewMemory(nil),
		ResolveResource: func(_ context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
			if kind != apitypes.ResourceKindFirmware || name != firmwareID {
				return apitypes.Resource{}, kv.ErrNotFound
			}
			var resource apitypes.Resource
			err := resource.FromFirmwareResource(apitypes.FirmwareResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.FirmwareResourceKindFirmware,
				Metadata:   apitypes.ResourceMetadata{Name: name},
			})
			return resource, err
		},
	}
	installTestSystemWorkflowResolver(server)
	response, err := server.PutRuntimeProfile(context.Background(), adminhttp.PutRuntimeProfileRequestObject{
		Name: profileName,
		Body: &adminhttp.RuntimeProfileUpsert{Name: profileName, Spec: apitypes.RuntimeProfileSpec{
			Workflows: testRuntimeProfileWorkflows(),
			Resources: apitypes.RuntimeProfileResources{},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.PutRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("PutRuntimeProfile() = %#v", response)
	}
	return server
}

func registrationServerAndToken(t *testing.T, profileName string) (*runtimeprofile.Server, string) {
	t.Helper()
	server := &runtimeprofile.Server{Store: kv.NewMemory(nil)}
	return server, createRegistrationToken(t, server, profileName)
}

func createRegistrationToken(t *testing.T, server *runtimeprofile.Server, profileName string) string {
	t.Helper()
	ctx := context.Background()
	profileResponse, err := server.PutRuntimeProfile(ctx, adminhttp.PutRuntimeProfileRequestObject{
		Name: profileName,
		Body: &adminhttp.RuntimeProfileUpsert{
			Name: profileName,
			Spec: apitypes.RuntimeProfileSpec{
				Workflows: testRuntimeProfileWorkflows(),
				Resources: apitypes.RuntimeProfileResources{},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := profileResponse.(adminhttp.PutRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("put RuntimeProfile = %#v", profileResponse)
	}
	tokenResponse, err := server.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{
		Body: &adminhttp.RegistrationTokenUpsert{
			Name:               "token-" + profileName,
			RuntimeProfileName: profileName,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := tokenResponse.(adminhttp.CreateRegistrationToken200JSONResponse)
	if !ok || strings.TrimSpace(created.Token) == "" {
		t.Fatalf("create RegistrationToken = %#v", tokenResponse)
	}
	return created.Token
}

func testRuntimeProfileWorkflows() apitypes.RuntimeProfileWorkflows {
	return apitypes.RuntimeProfileWorkflows{
		System: apitypes.RuntimeProfileSystemWorkflows{
			FriendChatroom: "chatroom",
			GroupChatroom:  "chatroom",
			Pet:            "pet-care",
		},
		Collections: apitypes.RuntimeProfileWorkflowCollections{},
	}
}

func installTestSystemWorkflowResolver(server *runtimeprofile.Server) {
	fallback := server.ResolveResource
	server.ResolveResource = func(ctx context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
		if kind == apitypes.ResourceKindWorkflow && (name == "chatroom" || name == "pet-care") {
			spec := apitypes.WorkflowSpec{
				Driver:   apitypes.WorkflowDriverChatroom,
				Chatroom: &apitypes.ChatRoomWorkflowSpec{History: apitypes.ChatRoomWorkflowHistorySpec{}},
			}
			if name == "pet-care" {
				spec = apitypes.WorkflowSpec{
					Driver: apitypes.WorkflowDriverPet,
					Pet: &apitypes.PetWorkflowSpec{
						Driver:   apitypes.ReusableWorkflowDriverChatroom,
						Chatroom: &apitypes.ChatRoomWorkflowSpec{History: apitypes.ChatRoomWorkflowHistorySpec{}},
					},
				}
			}
			var resource apitypes.Resource
			err := resource.FromWorkflowResource(apitypes.WorkflowResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.WorkflowResourceKindWorkflow,
				Metadata:   apitypes.ResourceMetadata{Name: name},
				Spec:       spec,
			})
			return resource, err
		}
		if fallback != nil {
			return fallback(ctx, kind, name)
		}
		return apitypes.Resource{}, kv.ErrNotFound
	}
}

func registerRPC(t *testing.T, server *rpcServer, token string) rpcapi.ServerRegisterResponse {
	t.Helper()
	response, err := server.dispatch(context.Background(), registrationRequest(token))
	if err != nil {
		t.Fatal(err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("register response = %#v", response)
	}
	value, err := response.Result.AsServerRegisterResponse()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func registrationRequest(token string) *rpcapi.RPCRequest {
	return newRPCRequest("register", rpcapi.RPCMethodServerRegister, mustRPCParams(
		rpcapi.ServerRegisterRequest{Token: token},
		(*rpcapi.RPCPayload).FromServerRegisterRequest,
	))
}
