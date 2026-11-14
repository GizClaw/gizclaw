package gizclaw

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/runtimeprofiletest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// Registration uses the ordinary Peer Store even on an admitted connection.
// Read/write-separated backends own local write visibility; RPC has no shortcut.
func TestRPCRegistrationUsesPeerStore(t *testing.T) {
	for _, state := range []string{"present", "missing", "read-error"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			registrations := firmwareRegistrationServer(t, "h106-profile", "h106")
			firmwareID := "h106"
			response, err := registrations.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{
				Body: &adminhttp.RegistrationTokenUpsert{
					Id: "h106-token", Token: "h106-registration", RuntimeProfileId: "h106-profile", FirmwareId: &firmwareID,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			created, ok := response.(adminhttp.CreateRegistrationToken200JSONResponse)
			if !ok {
				t.Fatalf("CreateRegistrationToken() = %T", response)
			}
			peers := &peer.Server{Store: kv.NewMemory(nil), RegistrationFirmware: registrations.ResolveOwnerFirmware}
			manager := NewManager(peers)
			manager.RuntimeProfiles = registrations
			publicKey := giznet.PublicKey{19}
			conn := &testGiznetConn{publicKey: publicKey}
			if _, err := manager.activatePeer(ctx, conn); err != nil {
				t.Fatal(err)
			}
			connection := &PeerConn{Conn: conn, Service: &PeerService{manager: manager}}
			connection.initRPC()
			var lookupErr error
			if state == "missing" {
				lookupErr = kv.ErrNotFound
			} else if state == "read-error" {
				lookupErr = errors.New("peer reader unavailable")
			}
			reader := &admissionStore{
				Store:      peers.Store,
				beforeRead: func(context.Context) error { return lookupErr },
			}
			peers.Store = reader
			var projections atomic.Int32
			peers.RegistrationFirmware = func(context.Context, string) (*string, error) {
				projections.Add(1)
				return nil, errors.New("unused firmware projection unavailable")
			}
			registered, err := connection.rpc.dispatch(ctx, registrationRequest(created.Token))
			if err != nil {
				t.Fatal(err)
			}
			if reader.reads.Load() == 0 {
				t.Fatal("registration bypassed the Peer Store")
			}
			if projections.Load() != 0 {
				t.Fatal("registration resolved an unused firmware projection")
			}
			if state == "present" {
				if registered.Error != nil {
					t.Fatalf("registration rejected an available Peer: %#v", registered.Error)
				}
				binding, err := registrations.ResolveOwnerProfile(ctx, publicKey.String())
				if err != nil || binding.Id != "h106-profile" {
					t.Fatalf("owner profile = %#v, %v", binding, err)
				}
			} else {
				if registered.Error == nil || registered.Error.Code != rpcapi.StatusCodeInternal {
					t.Fatalf("registration ignored Peer lookup failure: %#v", registered.Error)
				}
				if _, err := registrations.ResolveOwnerProfile(ctx, publicKey.String()); !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("failed lookup left an owner binding: %v", err)
				}
				var activations int
				if err := registrations.DB.GetContext(ctx, &activations, "SELECT COUNT(*) FROM registration_token_activations"); err != nil || activations != 0 {
					t.Fatalf("failed lookup consumed activation: %d, %v", activations, err)
				}
			}
		})
	}
}

func TestRPCRegistrationRechecksConnectionAfterResolution(t *testing.T) {
	for _, state := range []string{"deleting", "replaced"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			registrations := firmwareRegistrationServer(t, "h106-profile", "h106")
			firmwareID := "h106"
			response, err := registrations.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{
				Body: &adminhttp.RegistrationTokenUpsert{
					Id: "h106-token", Token: "h106-registration", RuntimeProfileId: "h106-profile", FirmwareId: &firmwareID,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			created, ok := response.(adminhttp.CreateRegistrationToken200JSONResponse)
			if !ok {
				t.Fatalf("CreateRegistrationToken() = %T", response)
			}
			peers := &peer.Server{Store: kv.NewMemory(nil), RegistrationFirmware: registrations.ResolveOwnerFirmware}
			manager := NewManager(peers)
			manager.RuntimeProfiles = registrations
			publicKey := giznet.PublicKey{20}
			conn := &trackingGiznetConn{testGiznetConn: &testGiznetConn{publicKey: publicKey}}
			if _, err := manager.activatePeer(ctx, conn); err != nil {
				t.Fatal(err)
			}
			connection := &PeerConn{Conn: conn, Service: &PeerService{manager: manager}}
			connection.initRPC()
			mutation := &blockingDeletionMutationStore{
				Store: peers.Store, entered: make(chan struct{}), release: make(chan struct{}),
			}
			reader := &admissionStore{Store: mutation}
			peers.Store = reader

			resolving, resume := make(chan struct{}), make(chan struct{})
			resumeResolution := sync.OnceFunc(func() { close(resume) })
			defer resumeResolution()
			resolve := registrations.ResolveResource
			var firstResolution atomic.Bool
			registrations.ResolveResource = func(ctx context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
				if firstResolution.CompareAndSwap(false, true) {
					close(resolving)
					select {
					case <-resume:
					case <-ctx.Done():
						return apitypes.Resource{}, ctx.Err()
					}
				}
				return resolve(ctx, kind, name)
			}
			type registrationResult struct {
				response *rpcapi.RPCResponse
				err      error
			}
			registered := make(chan registrationResult, 1)
			go func() {
				response, err := connection.rpc.dispatch(ctx, registrationRequest(created.Token))
				registered <- registrationResult{response: response, err: err}
			}()
			select {
			case <-resolving:
			case <-ctx.Done():
				t.Fatal("registration did not reach its first firmware resolver")
			}

			switch state {
			case "deleting":
				deleted := make(chan error, 1)
				go func() { deleted <- connection.rpc.deletePeerSelf(ctx) }()
				defer func() {
					close(mutation.release)
					select {
					case err := <-deleted:
						if err != nil {
							t.Errorf("delete Peer: %v", err)
						}
					case <-ctx.Done():
						t.Error("Peer deletion did not finish after releasing its mutation")
					}
				}()
				select {
				case <-mutation.entered:
				case <-ctx.Done():
					t.Fatal("Peer deletion did not reach its durable mutation")
				}
				if !connection.isRetiring() {
					t.Fatal("deleting connection is not retiring")
				}
			case "replaced":
				replacement := &testGiznetConn{publicKey: publicKey}
				if previous, err := manager.activatePeer(ctx, replacement); err != nil || previous != conn {
					t.Fatalf("replace connection = %v, %v; want previous %v", previous, err, conn)
				}
			}
			if conn.closed.Load() {
				t.Fatal("test closed the old transport before registration resumed")
			}
			readsBefore := reader.reads.Load()
			resumeResolution()
			select {
			case result := <-registered:
				if result.err != nil || result.response == nil || result.response.Error == nil || result.response.Error.Code != rpcapi.StatusCodeUnavailable {
					t.Fatalf("inactive connection registration = %#v, %v; want unavailable", result.response, result.err)
				}
			case <-ctx.Done():
				t.Fatal("registration did not finish after firmware resolution resumed")
			}
			if reader.reads.Load() <= readsBefore {
				t.Fatal("inactive connection bypassed ordinary Peer validation")
			}
			if conn.closed.Load() {
				t.Fatal("registration rejection relied on transport closure")
			}
			if _, err := registrations.ResolveOwnerProfile(ctx, publicKey.String()); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("inactive registration left an owner binding: %v", err)
			}
			var activations int
			if err := registrations.DB.GetContext(ctx, &activations, "SELECT COUNT(*) FROM registration_token_activations"); err != nil || activations != 0 {
				t.Fatalf("inactive registration consumed activation: %d, %v", activations, err)
			}
			if connection.registration.Load() != nil {
				t.Fatal("inactive registration published a connection snapshot")
			}
		})
	}
}

func TestRPCRegistrationRechecksCapacityAfterAdmission(t *testing.T) {
	registrations, token := registrationServerAndToken(t, "limited")
	response, err := registrations.PutRegistrationToken(t.Context(), adminhttp.PutRegistrationTokenRequestObject{
		Id: "token-limited", Body: &adminhttp.RegistrationTokenUpsert{
			Id: "token-limited", Token: token, RuntimeProfileId: "limited", MaxActivations: new(int64(1)),
		},
	})
	if _, ok := response.(adminhttp.PutRegistrationToken200JSONResponse); err != nil || !ok {
		t.Fatalf("set limit: %T, %v", response, err)
	}
	// Both connections passed admission before either registered.
	for range 2 {
		if err := registrations.PreflightRegistration(t.Context(), token); err != nil {
			t.Fatal(err)
		}
	}
	first := &rpcServer{registrations: registrations, callerPublicKey: giznet.PublicKey{1}}
	registerRPC(t, first, token)
	second := &rpcServer{registrations: registrations, callerPublicKey: giznet.PublicKey{2},
		onRegistration: func(runtimeprofile.Registration) { t.Error("denial published a snapshot") },
	}
	rejected, err := second.dispatch(t.Context(), registrationRequest(token))
	if err != nil || rejected.Error == nil || rejected.Error.Code != rpcapi.StatusCodePermissionDenied {
		t.Fatalf("exhausted registration: %#v, %v", rejected, err)
	}
	if _, err := registrations.ResolveOwnerProfile(t.Context(), second.callerPublicKey.String()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("denial left an owner binding: %v", err)
	}
	registerRPC(t, first, token)
	var count int
	if err := registrations.DB.GetContext(t.Context(), &count, "SELECT COUNT(*) FROM registration_token_activations"); err != nil || count != 1 {
		t.Fatalf("activation count = %d, %v", count, err)
	}
}

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
	if response.RuntimeProfileName != "profile-a" {
		t.Fatalf("first registration = %#v", response)
	}
	if got := snapshot.Load(); got == nil || got.RuntimeProfile.Id != "profile-a" {
		t.Fatalf("first snapshot = %#v", got)
	}
	bound, err := registrations.ResolveOwnerProfile(t.Context(), server.callerPublicKey.String())
	if err != nil || bound.Id != "profile-a" {
		t.Fatalf("ResolveOwnerProfile() = %#v, %v", bound, err)
	}

	rejected, err := server.dispatch(context.Background(), registrationRequest("invalid-token"))
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Error == nil || rejected.Error.Code != rpcapi.StatusCodePermissionDenied {
		t.Fatalf("invalid registration response = %#v", rejected)
	}
	if got := snapshot.Load(); got == nil || got.RuntimeProfile.Id != "profile-a" {
		t.Fatalf("rejected registration replaced snapshot: %#v", got)
	}

	response = registerRPC(t, server, tokenB)
	if response.RuntimeProfileName != "profile-b" {
		t.Fatalf("second registration = %#v", response)
	}
	if got := snapshot.Load(); got == nil || got.RuntimeProfile.Id != "profile-b" {
		t.Fatalf("second snapshot = %#v", got)
	}
	bound, err = registrations.ResolveOwnerProfile(t.Context(), server.callerPublicKey.String())
	if err != nil || bound.Id != "profile-b" {
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
	if got := snapshot.Load(); got == nil || got.RuntimeProfile.Id != "profile-a" {
		t.Fatalf("last successful registration snapshot = %#v", got)
	}
}

func TestRPCRegistrationSerializesOwnerBindingAndSnapshot(t *testing.T) {
	registrations, tokenA := registrationServerAndToken(t, "profile-a")
	tokenB := createRegistrationToken(t, registrations, "profile-b")
	var snapshot atomic.Pointer[runtimeprofile.Registration]
	aPublishing := make(chan struct{})
	releaseA := make(chan struct{})
	publicKey := giznet.PublicKey{6}
	server := &rpcServer{
		registrations:   registrations,
		callerPublicKey: publicKey,
		onRegistration: func(registration runtimeprofile.Registration) {
			if registration.RuntimeProfile.Id == "profile-a" {
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
	case <-bDone:
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
	if active == nil || active.RuntimeProfile.Id != bound.Id {
		t.Fatalf("snapshot = %#v, owner binding = %#v", active, bound)
	}
}

func TestRPCRegistrationPersistsAndReturnsFirmwareReleaseLine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	registrations := &runtimeprofile.Server{
		DB: runtimeprofiletest.New(t).DB,
		ResolveResource: func(_ context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
			if kind != apitypes.ResourceKindFirmware || name != "h106" {
				return apitypes.Resource{}, kv.ErrNotFound
			}
			var resource apitypes.Resource
			err := resource.FromFirmwareResource(apitypes.FirmwareResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.FirmwareResourceKindFirmware,
				Metadata:   apitypes.ResourceMetadata{Id: name},
				Spec:       apitypes.FirmwareSpec{},
			})
			return resource, err
		},
	}
	installTestSystemWorkflowResolver(registrations)
	profileName := "h106-production"
	profileResponse, err := registrations.CreateRuntimeProfile(ctx, adminhttp.CreateRuntimeProfileRequestObject{
		Body: &adminhttp.RuntimeProfileUpsert{Id: profileName, Spec: apitypes.RuntimeProfileSpec{
			Workflows: testRuntimeProfileWorkflows(),
			Resources: apitypes.RuntimeProfileResources{},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := profileResponse.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("CreateRuntimeProfile() = %#v", profileResponse)
	}
	firmwareID := "h106"
	tokenResponse, err := registrations.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Id: "h106-token", Token: "h106-registration", RuntimeProfileId: profileName, FirmwareId: &firmwareID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := tokenResponse.(adminhttp.CreateRegistrationToken200JSONResponse)
	if !ok {
		t.Fatalf("CreateRegistrationToken() = %#v", tokenResponse)
	}
	publicKey := giznet.PublicKey{9}
	peers := &peer.Server{Store: kv.NewMemory(nil), RegistrationFirmware: registrations.ResolveOwnerFirmware}
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
	if response.RuntimeProfileName != profileName {
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
		Id: "h106-token", Token: "h106-registration", RuntimeProfileId: "h106-production", FirmwareId: &firmwareID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	created := tokenResponse.(adminhttp.CreateRegistrationToken200JSONResponse)
	var snapshot atomic.Pointer[runtimeprofile.Registration]
	snapshot.Store(&runtimeprofile.Registration{RuntimeProfile: apitypes.RuntimeProfile{Id: "previous-profile"}})
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
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeInternal {
		t.Fatalf("server.register = %#v, want internal error", response)
	}
	if active := snapshot.Load(); active == nil || active.RuntimeProfile.Id != "previous-profile" {
		t.Fatalf("failed firmware binding replaced snapshot: %#v", active)
	}
	bound, err := registrations.ResolveOwnerProfile(ctx, server.callerPublicKey.String())
	if err != nil || bound.Id != "previous-profile" {
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
		Id: "h106-token", Token: "h106-registration", RuntimeProfileId: "h106-production", FirmwareId: &firmwareID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	created := tokenResponse.(adminhttp.CreateRegistrationToken200JSONResponse)

	peers := &peer.Server{Store: kv.NewMemory(nil), RegistrationFirmware: registrations.ResolveOwnerFirmware}
	if _, err := peers.EnsureConnectedPeer(ctx, publicKey); err != nil {
		t.Fatal(err)
	}
	if _, err := peers.BindFirmware(ctx, publicKey, "previous-firmware"); err != nil {
		t.Fatal(err)
	}
	if _, err := registrations.DB.ExecContext(ctx, `CREATE TRIGGER reject_owner_update BEFORE UPDATE ON runtime_profile_owners BEGIN SELECT RAISE(ABORT,'owner binding database unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	server := &rpcServer{registrations: registrations, peer: peers, callerPublicKey: publicKey}

	response, err := server.dispatch(ctx, registrationRequest(created.Token))
	if err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeInternal {
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
	if err != nil || bound.Id != "previous-profile" {
		t.Fatalf("failed owner binding replaced owner profile: %#v, %v", bound, err)
	}
	var activations int
	if err := registrations.DB.GetContext(ctx, &activations, "SELECT COUNT(*) FROM registration_token_activations"); err != nil || activations != 0 {
		t.Fatalf("failed binding consumed activation: %d %v", activations, err)
	}
	if firmware, err := registrations.ResolveOwnerFirmware(ctx, publicKey.String()); err != nil || firmware != nil {
		t.Fatalf("failed transaction published firmware: %v %v", firmware, err)
	}
}

type rejectingFirmwarePeer struct{}

func (rejectingFirmwarePeer) GetSelfInfo(context.Context, giznet.PublicKey) (apitypes.DeviceInfo, error) {
	return apitypes.DeviceInfo{}, nil
}

func (rejectingFirmwarePeer) PutSelfInfo(context.Context, giznet.PublicKey, apitypes.DeviceInfo) (apitypes.DeviceInfo, error) {
	return apitypes.DeviceInfo{}, nil
}

func (rejectingFirmwarePeer) GetSelfRuntime(context.Context, giznet.PublicKey) apitypes.Runtime {
	return apitypes.Runtime{}
}

func (rejectingFirmwarePeer) ValidatePeerRecord(context.Context, giznet.PublicKey) error {
	return errors.New("store unavailable")
}

func (rejectingFirmwarePeer) DeleteSelf(context.Context, giznet.PublicKey) error {
	return nil
}

func firmwareRegistrationServer(t *testing.T, profileName, firmwareID string) *runtimeprofile.Server {
	t.Helper()
	server := &runtimeprofile.Server{
		DB: runtimeprofiletest.New(t).DB,
		ResolveResource: func(_ context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
			if kind != apitypes.ResourceKindFirmware || name != firmwareID {
				return apitypes.Resource{}, kv.ErrNotFound
			}
			var resource apitypes.Resource
			err := resource.FromFirmwareResource(apitypes.FirmwareResource{
				ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
				Kind:       apitypes.FirmwareResourceKindFirmware,
				Metadata:   apitypes.ResourceMetadata{Id: name},
				Spec:       apitypes.FirmwareSpec{},
			})
			return resource, err
		},
	}
	installTestSystemWorkflowResolver(server)
	response, err := server.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{
		Body: &adminhttp.RuntimeProfileUpsert{Id: profileName, Spec: apitypes.RuntimeProfileSpec{
			Workflows: testRuntimeProfileWorkflows(),
			Resources: apitypes.RuntimeProfileResources{},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("CreateRuntimeProfile() = %#v", response)
	}
	return server
}

func registrationServerAndToken(t *testing.T, profileName string) (*runtimeprofile.Server, string) {
	t.Helper()
	server := &runtimeprofile.Server{DB: runtimeprofiletest.New(t).DB}
	return server, createRegistrationToken(t, server, profileName)
}

func createRegistrationToken(t *testing.T, server *runtimeprofile.Server, profileName string) string {
	t.Helper()
	ctx := context.Background()
	profileResponse, err := server.CreateRuntimeProfile(ctx, adminhttp.CreateRuntimeProfileRequestObject{
		Body: &adminhttp.RuntimeProfileUpsert{
			Id: profileName,
			Spec: apitypes.RuntimeProfileSpec{
				Workflows: testRuntimeProfileWorkflows(),
				Resources: apitypes.RuntimeProfileResources{},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := profileResponse.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("create RuntimeProfile = %#v", profileResponse)
	}
	tokenResponse, err := server.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{
		Body: &adminhttp.RegistrationTokenUpsert{
			Id:               "token-" + profileName,
			Token:            "registration-" + profileName,
			RuntimeProfileId: profileName,
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
	return apitypes.RuntimeProfileWorkflows{}
}

func installTestSystemWorkflowResolver(server *runtimeprofile.Server) {
	fallback := server.ResolveResource
	server.ResolveResource = func(ctx context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
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
