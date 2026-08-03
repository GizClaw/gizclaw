package memorystore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"

	flowrecall "github.com/GizClaw/flowcraft/memory/recall"
	flowpostgres "github.com/GizClaw/flowcraft/memory/recall/store/postgres"
	flowworkspace "github.com/GizClaw/flowcraft/memory/recall/store/workspace"
	"github.com/GizClaw/flowcraft/memory/retrieval"
	"github.com/GizClaw/flowcraft/memory/retrieval/bbh"
	retrievalpostgres "github.com/GizClaw/flowcraft/memory/retrieval/postgres"
	sdkworkspace "github.com/GizClaw/flowcraft/sdk/workspace"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	memoryflowcraft "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft"
)

// sharedBackend owns only deployment-specific physical dependencies. NewStore
// creates a policy-bearing logical Store for one Workspace generation.
type sharedBackend interface {
	NewStore(context.Context, Request) (Result, io.Closer, error)
	Close() error
}

func openSharedBackend(ctx context.Context, request Request) (sharedBackend, error) {
	switch request.Binding.Driver {
	case apitypes.RuntimeProfileMemoryDriverFlowcraft:
		return openSharedFlowcraft(ctx, request)
	case apitypes.RuntimeProfileMemoryDriverMem0, apitypes.RuntimeProfileMemoryDriverVolcMem0:
		result, err := Build(ctx, request)
		if err != nil {
			return nil, err
		}
		return &sharedRemoteBackend{result: result}, nil
	default:
		return nil, fmt.Errorf("memory store: unsupported shared driver %q", request.Binding.Driver)
	}
}

type sharedRemoteBackend struct {
	result Result
	once   sync.Once
	err    error
}

func (backend *sharedRemoteBackend) NewStore(context.Context, Request) (Result, io.Closer, error) {
	return Result{Store: backend.result.Store, Driver: backend.result.Driver}, nil, nil
}

func (backend *sharedRemoteBackend) Close() error {
	if backend == nil {
		return nil
	}
	backend.once.Do(func() {
		if backend.result.Closer != nil {
			backend.err = backend.result.Closer.Close()
		}
	})
	return backend.err
}

type sharedFlowcraftBackend struct {
	temporal flowrecall.TemporalStore
	evidence flowrecall.EvidenceStore
	queue    flowrecall.AsyncSemanticQueue
	outbox   flowrecall.SideEffectOutbox

	backendCloser io.Closer
	index         retrieval.Index
	indexCloser   io.Closer
	local         *sharedLocalProjection

	mu                  sync.Mutex
	projectionSignature string
	closeOnce           sync.Once
	closeErr            error
}

type sharedLocalProjection struct {
	dir   string
	index *sharedBBHIndex
}

func openSharedFlowcraft(ctx context.Context, request Request) (sharedBackend, error) {
	if err := validateLayoutBinding(request); err != nil {
		return nil, err
	}
	policy := request.Layout.Spec.Flowcraft
	config, err := flowcraftConfig(policy, request.ModelLoader)
	if err != nil {
		return nil, err
	}
	connectionType, err := request.Binding.Connection.Discriminator()
	if err != nil {
		return nil, fmt.Errorf("memory store: decode flowcraft connection: %w", err)
	}
	switch connectionType {
	case "flowcraft_bbh", "flowcraft_object_store":
		dir, err := sharedLocalDirectory(request, connectionType)
		if err != nil {
			return nil, err
		}
		metadataWorkspace, err := sdkworkspace.NewLocalWorkspace(dir)
		if err != nil {
			return nil, err
		}
		backend, err := flowworkspace.New(metadataWorkspace)
		if err != nil {
			return nil, err
		}
		if err := ensureLocalProjection(ctx, dir, backend, policy, config); err != nil {
			return nil, errors.Join(err, backend.Close())
		}
		retrievalWorkspace, err := sdkworkspace.NewLocalWorkspace(retrievalDirectory(dir))
		if err != nil {
			return nil, errors.Join(err, backend.Close())
		}
		rawIndex, err := bbh.New(retrievalWorkspace, bbh.WithConfig(mapBBHConfig(policy.Bbh)))
		if err != nil {
			return nil, errors.Join(err, backend.Close())
		}
		signature, err := projectionSignature(policy)
		if err != nil {
			return nil, errors.Join(err, rawIndex.Close(), backend.Close())
		}
		index := newSharedBBHIndex(rawIndex, mapBBHConfig(policy.Bbh))
		return &sharedFlowcraftBackend{
			temporal: backend.TemporalStore(), evidence: backend.EvidenceStore(),
			queue: backend.AsyncSemanticQueue(), outbox: backend.SideEffectOutbox(),
			backendCloser: backend, index: index, indexCloser: index,
			local:               &sharedLocalProjection{dir: dir, index: index},
			projectionSignature: signature,
		}, nil
	case "flowcraft_postgresql":
		connection, err := request.Binding.Connection.AsRuntimeProfileFlowcraftPostgreSQLConnection()
		if err != nil {
			return nil, err
		}
		backend, err := flowpostgres.Open(ctx, connection.Dsn)
		if err != nil {
			return nil, err
		}
		index, err := retrievalpostgres.Open(ctx, connection.Dsn)
		if err != nil {
			return nil, errors.Join(err, backend.Close())
		}
		return &sharedFlowcraftBackend{
			temporal: backend.TemporalStore(), evidence: backend.EvidenceStore(),
			queue: backend.AsyncSemanticQueue(), outbox: backend.SideEffectOutbox(),
			backendCloser: backend, index: index, indexCloser: index,
		}, nil
	default:
		return nil, fmt.Errorf("memory store: flowcraft driver cannot use connection type %q", connectionType)
	}
}

func sharedLocalDirectory(request Request, connectionType string) (string, error) {
	if connectionType == "flowcraft_bbh" {
		return managedBindingRoot(request.ServerRoot, request.ProfileID, request.BindingName)
	}
	connection, err := request.Binding.Connection.AsRuntimeProfileFlowcraftObjectStoreConnection()
	if err != nil {
		return "", err
	}
	return filepath.Clean(connection.Directory), nil
}

func retrievalDirectory(dir string) string {
	return filepath.Join(dir, "retrieval")
}

func (backend *sharedFlowcraftBackend) NewStore(ctx context.Context, request Request) (Result, io.Closer, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()

	policy := request.Layout.Spec.Flowcraft
	config, err := flowcraftConfig(policy, request.ModelLoader)
	if err != nil {
		return Result{}, nil, err
	}
	signature, err := projectionSignature(policy)
	if err != nil {
		return Result{}, nil, err
	}
	if backend.local != nil && signature != backend.projectionSignature {
		workspaceBackend, ok := backend.backendCloser.(*flowworkspace.Backend)
		if !ok {
			return Result{}, nil, errors.New("memory store: invalid shared Flowcraft workspace backend")
		}
		if err := backend.local.index.Rebuild(
			ctx,
			backend.local.dir,
			workspaceBackend,
			policy,
			config,
		); err != nil {
			return Result{}, nil, err
		}
		backend.projectionSignature = signature
	}

	config.TemporalStore = backend.temporal
	config.EvidenceStore = backend.evidence
	config.SideEffectOutbox = backend.outbox
	config.RetrievalIndex = backend.index
	if policy.Write.Mode == apitypes.FlowcraftMemoryWritePolicyModeAsyncSemantic {
		config.AsyncQueue = backend.queue
	}
	store, err := memoryflowcraft.New(ctx, config)
	if err != nil {
		return Result{}, nil, err
	}
	if backend.local == nil && signature != backend.projectionSignature {
		if err := rebuildAllScopes(ctx, store, backend.temporal); err != nil {
			return Result{}, nil, errors.Join(err, store.Close())
		}
		backend.projectionSignature = signature
	}
	return Result{
		Store: store, Driver: string(apitypes.RuntimeProfileMemoryDriverFlowcraft),
	}, store, nil
}

func (backend *sharedFlowcraftBackend) Close() error {
	if backend == nil {
		return nil
	}
	backend.closeOnce.Do(func() {
		backend.closeErr = errors.Join(
			closeOne(backend.indexCloser),
			closeOne(backend.backendCloser),
		)
	})
	return backend.closeErr
}

func closeOne(closer io.Closer) error {
	if closer == nil {
		return nil
	}
	return closer.Close()
}

// sharedBBHIndex keeps the physical BBH handle stable for all logical Stores.
// Rebuild takes the exclusive lock, so existing Store operations never observe
// a closed or partially published derived index.
type sharedBBHIndex struct {
	mu     sync.RWMutex
	index  *bbh.Index
	config bbh.Config
}

func newSharedBBHIndex(index *bbh.Index, config bbh.Config) *sharedBBHIndex {
	return &sharedBBHIndex{index: index, config: config}
}

func (index *sharedBBHIndex) Rebuild(
	ctx context.Context,
	dir string,
	backend *flowworkspace.Backend,
	policy apitypes.FlowcraftMemoryLayoutPolicy,
	config memoryflowcraft.Config,
) error {
	// Build and validate the complete replacement before blocking live users.
	prepared, err := prepareLocalProjection(ctx, dir, backend, policy, config)
	if err != nil {
		return err
	}
	if prepared == nil {
		return nil
	}
	defer prepared.Abort()

	index.mu.Lock()
	defer index.mu.Unlock()
	if index.index == nil {
		return errors.New("memory store: shared BBH index is closed")
	}
	oldConfig := index.config
	if err := index.index.Close(); err != nil {
		return fmt.Errorf("memory store: close previous derived index: %w", err)
	}
	replacement, err := prepared.Publish()
	if err != nil {
		reopened, reopenErr := openBBHIndex(dir, oldConfig)
		index.index = reopened
		return errors.Join(err, reopenErr)
	}
	index.index = replacement
	index.config = mapBBHConfig(policy.Bbh)
	return nil
}

func openBBHIndex(dir string, config bbh.Config) (*bbh.Index, error) {
	workspace, err := sdkworkspace.NewLocalWorkspace(retrievalDirectory(dir))
	if err != nil {
		return nil, err
	}
	return bbh.New(workspace, bbh.WithConfig(config))
}

func (index *sharedBBHIndex) Close() error {
	index.mu.Lock()
	defer index.mu.Unlock()
	if index.index == nil {
		return nil
	}
	err := index.index.Close()
	index.index = nil
	return err
}

func (index *sharedBBHIndex) current() (*bbh.Index, error) {
	if index == nil || index.index == nil {
		return nil, errors.New("memory store: shared BBH index is unavailable")
	}
	return index.index, nil
}

func (index *sharedBBHIndex) Upsert(ctx context.Context, namespace string, docs []retrieval.Doc) error {
	index.mu.RLock()
	defer index.mu.RUnlock()
	current, err := index.current()
	if err != nil {
		return err
	}
	return current.Upsert(ctx, namespace, docs)
}

func (index *sharedBBHIndex) Delete(ctx context.Context, namespace string, ids []string) error {
	index.mu.RLock()
	defer index.mu.RUnlock()
	current, err := index.current()
	if err != nil {
		return err
	}
	return current.Delete(ctx, namespace, ids)
}

func (index *sharedBBHIndex) Search(ctx context.Context, namespace string, request retrieval.SearchRequest) (*retrieval.SearchResponse, error) {
	index.mu.RLock()
	defer index.mu.RUnlock()
	current, err := index.current()
	if err != nil {
		return nil, err
	}
	return current.Search(ctx, namespace, request)
}

func (index *sharedBBHIndex) List(ctx context.Context, namespace string, request retrieval.ListRequest) (*retrieval.ListResponse, error) {
	index.mu.RLock()
	defer index.mu.RUnlock()
	current, err := index.current()
	if err != nil {
		return nil, err
	}
	return current.List(ctx, namespace, request)
}

func (index *sharedBBHIndex) Capabilities() retrieval.Capabilities {
	index.mu.RLock()
	defer index.mu.RUnlock()
	current, err := index.current()
	if err != nil {
		return retrieval.Capabilities{}
	}
	return current.Capabilities()
}

func (index *sharedBBHIndex) Get(ctx context.Context, namespace, id string) (retrieval.Doc, bool, error) {
	index.mu.RLock()
	defer index.mu.RUnlock()
	current, err := index.current()
	if err != nil {
		return retrieval.Doc{}, false, err
	}
	return current.Get(ctx, namespace, id)
}

func (index *sharedBBHIndex) SupportsFilter(filter retrieval.Filter) bool {
	index.mu.RLock()
	defer index.mu.RUnlock()
	current, err := index.current()
	return err == nil && current.SupportsFilter(filter)
}

func (index *sharedBBHIndex) Iterate(ctx context.Context, namespace, cursor string, batch int) ([]retrieval.Doc, string, error) {
	index.mu.RLock()
	defer index.mu.RUnlock()
	current, err := index.current()
	if err != nil {
		return nil, "", err
	}
	return current.Iterate(ctx, namespace, cursor, batch)
}

func (index *sharedBBHIndex) Count(ctx context.Context, namespace string, filter retrieval.Filter) (int64, error) {
	index.mu.RLock()
	defer index.mu.RUnlock()
	current, err := index.current()
	if err != nil {
		return 0, err
	}
	return current.Count(ctx, namespace, filter)
}

func (index *sharedBBHIndex) DeleteByFilter(ctx context.Context, namespace string, filter retrieval.Filter) (int64, error) {
	index.mu.RLock()
	defer index.mu.RUnlock()
	current, err := index.current()
	if err != nil {
		return 0, err
	}
	return current.DeleteByFilter(ctx, namespace, filter)
}

func (index *sharedBBHIndex) Drop(ctx context.Context, namespace string) error {
	index.mu.RLock()
	defer index.mu.RUnlock()
	current, err := index.current()
	if err != nil {
		return err
	}
	return current.Drop(ctx, namespace)
}

func (index *sharedBBHIndex) WarmNamespace(ctx context.Context, namespace string) error {
	index.mu.RLock()
	defer index.mu.RUnlock()
	current, err := index.current()
	if err != nil {
		return err
	}
	return current.WarmNamespace(ctx, namespace)
}

var (
	_ retrieval.Index             = (*sharedBBHIndex)(nil)
	_ retrieval.DocGetter         = (*sharedBBHIndex)(nil)
	_ retrieval.Filterable        = (*sharedBBHIndex)(nil)
	_ retrieval.Iterable          = (*sharedBBHIndex)(nil)
	_ retrieval.Countable         = (*sharedBBHIndex)(nil)
	_ retrieval.DeletableByFilter = (*sharedBBHIndex)(nil)
	_ retrieval.Droppable         = (*sharedBBHIndex)(nil)
	_ retrieval.NamespaceWarmer   = (*sharedBBHIndex)(nil)
	_ memory.Store                = (*memoryflowcraft.Store)(nil)
)
