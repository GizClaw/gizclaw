package gizclaw

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestRPCServerPetPixaDownloadStreamsBinary(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	payload := []byte("pet-pixa-payload")
	pixaPath := "pet-defs/petdef-a/pixa"
	service := &fakeGameplayPixaDownloadService{
		petPixaMetadata: rpcapi.PetPixaDownloadResponse{
			PetName:    "pet-a",
			PetDefName: "petdef-a",
			PixaPath:   &pixaPath,
			SizeBytes:  int64(len(payload)),
		},
		petPixaPayload: payload,
	}
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- (&rpcServer{serverResources: service}).Handle(serverSide)
	}()

	stream, err := newRPCStream(context.Background(), clientSide)
	if err != nil {
		t.Fatalf("newRPCStream() error = %v", err)
	}
	defer stream.Close()

	params, err := newRPCRequestParams(rpcapi.PetPixaDownloadRequest{PetName: "pet-a"}, (*rpcapi.RPCPayload).FromServerPetPixaDownloadRequest)
	if err != nil {
		t.Fatalf("newRPCRequestParams() error = %v", err)
	}
	if err := stream.WriteRequest(newRPCRequest("pet-pixa-download", rpcapi.RPCMethodServerPetPixaDownload, params)); err != nil {
		t.Fatalf("WriteRequest() error = %v", err)
	}
	if err := stream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}

	resp, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerPetPixaDownload)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("pet pixa response error = %+v", resp.Error)
	}
	gotMetadata, err := resp.Result.AsServerPetPixaDownloadResponse()
	if err != nil {
		t.Fatalf("AsServerPetPixaDownloadResponse() error = %v", err)
	}
	if gotMetadata.PetName != "pet-a" || gotMetadata.PetDefName != "petdef-a" || gotMetadata.SizeBytes != int64(len(payload)) || gotMetadata.PixaPath == nil || *gotMetadata.PixaPath != pixaPath {
		t.Fatalf("metadata = %+v", gotMetadata)
	}

	frame, err := stream.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame(binary) error = %v", err)
	}
	if frame.Type != rpcapi.FrameTypeBinary || !bytes.Equal(frame.Payload, payload) {
		t.Fatalf("binary frame = %+v", frame)
	}
	frame, err = stream.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame(EOS) error = %v", err)
	}
	if frame.Type != rpcapi.FrameTypeEOS {
		t.Fatalf("last frame type = %d, want EOS", frame.Type)
	}
	if err := clientSide.Close(); err != nil {
		t.Fatalf("client close error = %v", err)
	}
	select {
	case err := <-serverErrCh:
		if err != nil {
			t.Fatalf("server error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish")
	}
	if service.petPixaRequest.PetName != "pet-a" {
		t.Fatalf("request = %+v", service.petPixaRequest)
	}
}

func TestRPCServerPetPixaDownloadStreamsPublishedAsset(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 30, 4, 0, 0, 0, time.UTC)
	publishedBytes := makePublishedPetPixa()
	catalog := &gameplay.Catalog{
		PetDefs:   kv.NewMemory(nil),
		BadgeDefs: kv.NewMemory(nil),
		GameDefs:  kv.NewMemory(nil),
		Assets:    objectstore.Dir(t.TempDir()),
		Now:       func() time.Time { return now },
	}
	createResp, err := catalog.CreatePetDef(ctx, adminhttp.CreatePetDefRequestObject{
		Body: &adminhttp.PetDefUpsert{
			Name: "petdef-rpc",
			Spec: apitypes.PetDefSpec{
				Character: apitypes.PetDefCharacterSpec{Prompt: "Friendly RPC pet."},
				Voice:     apitypes.PetDefVoiceSpec{Prompt: "Warm and concise."},
				Visual: apitypes.PetDefVisualSpec{
					Bindings: apitypes.PetDefVisualBindingsSpec{
						Behaviors: apitypes.PetDefBehaviorBindingsSpec{Feed: "idle", Bathe: "idle", Play: "idle", Heal: "idle"},
						States:    apitypes.PetDefStateBindingsSpec{Idle: "idle", Sick: "idle", Dead: "idle"},
					},
					Pixa: apitypes.PetDefPixaSpec{
						AssetRef: "asset://pets/dewey.pixa",
						Metadata: apitypes.PetDefPixaMetadata{
							Version: "1",
							Canvas:  apitypes.PetDefPixaCanvasMetadata{Width: 4, Height: 4},
							Clips:   []apitypes.PetDefPixaClipMetadata{{Id: "idle", PixaClipName: "idle"}},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreatePetDef() error = %v", err)
	}
	createdPetDef, ok := createResp.(adminhttp.CreatePetDef200JSONResponse)
	if !ok {
		t.Fatalf("CreatePetDef() response = %#v", createResp)
	}
	uploadResp, err := catalog.UploadPetDefPixa(ctx, adminhttp.UploadPetDefPixaRequestObject{
		Id:   createdPetDef.Id,
		Body: io.NopCloser(bytes.NewReader(publishedBytes)),
	})
	if err != nil {
		t.Fatalf("UploadPetDefPixa() error = %v", err)
	}
	if _, ok := uploadResp.(adminhttp.UploadPetDefPixa200JSONResponse); !ok {
		t.Fatalf("UploadPetDefPixa() response = %#v", uploadResp)
	}

	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sqlx.Open() error = %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	var caller giznet.PublicKey
	caller[0] = 1
	owner := caller.String()
	initialBalance, adoptionCost := int64(1), int64(0)
	petDefs := map[string]apitypes.RuntimeProfileBinding{
		"rpc": {ResourceId: createdPetDef.Id},
	}
	pool := []apitypes.RuntimeProfilePetPoolEntry{{
		PetDef: "rpc", Weight: 1, AdoptionCost: &adoptionCost,
	}}
	profile := apitypes.RuntimeProfile{
		Id:   "runtime-profile-rpc",
		Name: "rpc-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Resources: apitypes.RuntimeProfileResources{PetDefs: &petDefs},
			Workflows: apitypes.RuntimeProfileWorkflows{
				System: apitypes.RuntimeProfileSystemWorkflows{Pet: "pet-workflow"},
			},
			Gameplay: &apitypes.RuntimeProfileGameplaySpec{
				Points:   &apitypes.RuntimeProfilePointsSpec{InitialBalance: &initialBalance},
				Adoption: &apitypes.RuntimeProfileAdoptionSpec{Pool: &pool},
				Pet:      &apitypes.RuntimeProfilePetGameplaySpec{Games: map[string]apitypes.RuntimeProfileGameSpec{}},
			},
		},
	}
	runtime := &gameplay.Runtime{
		DB:         db,
		Catalog:    catalog,
		Workflows:  rpcPixaWorkflowService{},
		Workspaces: rpcPixaWorkspaceService{},
		Now:        func() time.Time { return now },
		NewID:      func() string { return "pet-rpc" },
		PickWeight: func(int64) int64 { return 0 },
	}
	adopted, err := runtime.AdoptPet(
		gameplay.WithRuntimeProfile(ctx, profile),
		owner,
		apitypes.PetAdoptRequest{Name: "pet-rpc-name", DisplayName: "RPC Pet"},
	)
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}

	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	resources := &peerresource.Server{
		Caller:         caller,
		Gameplay:       runtime,
		RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile },
	}
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- (&rpcServer{serverResources: resources}).Handle(serverSide)
	}()

	stream, err := newRPCStream(ctx, clientSide)
	if err != nil {
		t.Fatalf("newRPCStream() error = %v", err)
	}
	defer stream.Close()
	params, err := newRPCRequestParams(
		rpcapi.PetPixaDownloadRequest{PetName: adopted.Pet.Name},
		(*rpcapi.RPCPayload).FromServerPetPixaDownloadRequest,
	)
	if err != nil {
		t.Fatalf("newRPCRequestParams() error = %v", err)
	}
	if err := stream.WriteRequest(newRPCRequest("published-pet-pixa", rpcapi.RPCMethodServerPetPixaDownload, params)); err != nil {
		t.Fatalf("WriteRequest() error = %v", err)
	}
	if err := stream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}
	resp, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerPetPixaDownload)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("pet pixa response error = %+v", resp.Error)
	}
	metadata, err := resp.Result.AsServerPetPixaDownloadResponse()
	if err != nil {
		t.Fatalf("AsServerPetPixaDownloadResponse() error = %v", err)
	}
	if metadata.PetName != adopted.Pet.Name || metadata.PetDefName != "rpc" || metadata.SizeBytes != int64(len(publishedBytes)) {
		t.Fatalf("metadata = %+v", metadata)
	}
	var downloaded bytes.Buffer
	for {
		frame, err := stream.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame() error = %v", err)
		}
		if frame.Type == rpcapi.FrameTypeEOS {
			break
		}
		if frame.Type != rpcapi.FrameTypeBinary {
			t.Fatalf("frame type = %d, want binary or EOS", frame.Type)
		}
		if _, err := downloaded.Write(frame.Payload); err != nil {
			t.Fatalf("downloaded.Write() error = %v", err)
		}
	}
	if !bytes.Equal(downloaded.Bytes(), publishedBytes) {
		t.Fatalf("RPC download bytes differ from accepted upload")
	}
	if err := clientSide.Close(); err != nil {
		t.Fatalf("client close error = %v", err)
	}
	select {
	case err := <-serverErrCh:
		if err != nil {
			t.Fatalf("server error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish")
	}
}

func makePublishedPetPixa() []byte {
	const (
		headerSize = 40
		clipSize   = 56
		frameSize  = 16
	)
	payload := []byte{4, 0, 1, 0, 2, 1, 1, 0, 1, 0, 2, 1, 1, 0, 4, 0}
	paletteOffset := headerSize
	clipOffset := paletteOffset + 4
	frameOffset := clipOffset + clipSize
	payloadOffset := frameOffset + frameSize
	data := make([]byte, payloadOffset+len(payload))
	copy(data[:4], "PIXA")
	binary.LittleEndian.PutUint16(data[4:6], 1)
	binary.LittleEndian.PutUint16(data[6:8], headerSize)
	binary.LittleEndian.PutUint16(data[8:10], 4)
	binary.LittleEndian.PutUint16(data[10:12], 4)
	binary.LittleEndian.PutUint16(data[12:14], 2)
	binary.LittleEndian.PutUint16(data[14:16], 1)
	binary.LittleEndian.PutUint32(data[16:20], 1)
	binary.LittleEndian.PutUint32(data[20:24], uint32(paletteOffset))
	binary.LittleEndian.PutUint32(data[24:28], uint32(clipOffset))
	binary.LittleEndian.PutUint32(data[28:32], uint32(frameOffset))
	binary.LittleEndian.PutUint32(data[32:36], uint32(payloadOffset))
	binary.LittleEndian.PutUint32(data[36:40], uint32(len(payload)))
	binary.LittleEndian.PutUint16(data[paletteOffset+2:paletteOffset+4], 0x07e0)
	copy(data[clipOffset:clipOffset+32], "idle")
	binary.LittleEndian.PutUint32(data[clipOffset+40:clipOffset+44], 1)
	data[frameOffset+3] = 1
	binary.LittleEndian.PutUint32(data[frameOffset+8:frameOffset+12], uint32(len(payload)))
	copy(data[payloadOffset:], payload)
	return data
}

func TestWriteRPCDownloadPreservesWriteEOF(t *testing.T) {
	conn := &failAfterWritesConn{remaining: 2, err: io.EOF}
	stream := &rpcStream{ctx: context.Background(), conn: conn}
	req := &rpcapi.RPCRequest{Id: "download", Method: rpcapi.RPCMethodServerBadgeDefPixaDownload}
	err := writeRPCDownload(
		context.Background(),
		stream,
		req,
		rpcapi.BadgeDefPixaDownloadResponse{Name: "badge"},
		(*rpcapi.RPCPayload).FromBadgeDefPixaDownloadResponse,
		bytes.NewReader([]byte("payload")),
	)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("writeRPCDownload() error = %v, want EOF", err)
	}
}

type failAfterWritesConn struct {
	remaining int
	err       error
}

func (c *failAfterWritesConn) Read([]byte) (int, error) { return 0, io.EOF }

func (c *failAfterWritesConn) Write(body []byte) (int, error) {
	if c.remaining == 0 {
		return 0, c.err
	}
	c.remaining--
	return len(body), nil
}

func (*failAfterWritesConn) Close() error                     { return nil }
func (*failAfterWritesConn) LocalAddr() net.Addr              { return nil }
func (*failAfterWritesConn) RemoteAddr() net.Addr             { return nil }
func (*failAfterWritesConn) SetDeadline(time.Time) error      { return nil }
func (*failAfterWritesConn) SetReadDeadline(time.Time) error  { return nil }
func (*failAfterWritesConn) SetWriteDeadline(time.Time) error { return nil }

type fakeGameplayPixaDownloadService struct {
	petPixaMetadata rpcapi.PetPixaDownloadResponse
	petPixaPayload  []byte
	petPixaRequest  rpcapi.PetPixaDownloadRequest
}

type rpcPixaWorkflowService struct{}

func (rpcPixaWorkflowService) GetWorkflow(context.Context, adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	return adminhttp.GetWorkflow200JSONResponse(apitypes.Workflow{
		Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverPet},
	}), nil
}

type rpcPixaWorkspaceService struct{}

func (rpcPixaWorkspaceService) CreateSystemWorkspace(_ context.Context, body adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	return apitypes.Workspace{Id: "id-" + body.Name, Name: body.Name, WorkflowId: body.WorkflowId}, true, nil
}

func (rpcPixaWorkspaceService) DeleteSystemWorkspace(_ context.Context, name string) (apitypes.Workspace, error) {
	return apitypes.Workspace{Name: name}, nil
}

func (rpcPixaWorkspaceService) GetWorkspace(_ context.Context, request adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	return adminhttp.GetWorkspace200JSONResponse(apitypes.Workspace{Id: request.Id}), nil
}

func (rpcPixaWorkspaceService) GetWorkspaceByName(_ context.Context, name string) (apitypes.Workspace, error) {
	return apitypes.Workspace{Id: "id-" + name, Name: name}, nil
}

func (f *fakeGameplayPixaDownloadService) PreparePetPixaDownload(_ context.Context, request rpcapi.PetPixaDownloadRequest) (rpcapi.PetPixaDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	f.petPixaRequest = request
	return f.petPixaMetadata, io.NopCloser(bytes.NewReader(f.petPixaPayload)), nil, nil
}

func (f *fakeGameplayPixaDownloadService) PrepareBadgeDefPixaDownload(context.Context, rpcapi.BadgeDefPixaDownloadRequest) (rpcapi.BadgeDefPixaDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	return rpcapi.BadgeDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "not found"}, nil
}

func (f *fakeGameplayPixaDownloadService) Dispatch(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error) {
	return nil, false, nil
}
