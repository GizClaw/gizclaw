package memorystore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	flowrecall "github.com/GizClaw/flowcraft/memory/recall"
	flowpostgres "github.com/GizClaw/flowcraft/memory/recall/store/postgres"
	"github.com/GizClaw/flowcraft/memory/retrieval"
	retrievalpostgres "github.com/GizClaw/flowcraft/memory/retrieval/postgres"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	memoryflowcraft "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft"
	flowcraftredis8 "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft/redis8"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
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
	newStore      func(context.Context, memoryflowcraft.Config) (*memoryflowcraft.Store, error)

	mu                  sync.Mutex
	projectionSignature string
	closeOnce           sync.Once
	closeErr            error
}

func openSharedFlowcraft(ctx context.Context, request Request) (sharedBackend, error) {
	if err := validateLayoutBinding(request); err != nil {
		return nil, err
	}
	policy := request.Layout.Spec.Flowcraft
	connectionType, err := request.Binding.Connection.Discriminator()
	if err != nil {
		return nil, fmt.Errorf("memory store: decode flowcraft connection: %w", err)
	}
	switch connectionType {
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
	case "flowcraft_redis8":
		if boolValue(policy.GraphEnabled) {
			return nil, errors.New("memory store: flowcraft_redis8 cannot enable graph until Flowcraft exposes graph store injection")
		}
		connection, err := request.Binding.Connection.AsRuntimeProfileFlowcraftRedis8Connection()
		if err != nil {
			return nil, err
		}
		tlsCAFile := ""
		if connection.TlsCaFile != nil {
			tlsCAFile = *connection.TlsCaFile
		}
		owner, err := storage.New(map[string]storage.Config{
			"flowcraft-redis8": storage.RedisConfig{URL: connection.Url, TLSCAFile: tlsCAFile},
		})
		if err != nil {
			return nil, err
		}
		client, err := owner.Redis("flowcraft-redis8")
		if err != nil {
			return nil, errors.Join(err, owner.Close())
		}
		backend, err := flowcraftredis8.OpenBackend(ctx, client, flowcraftRedis8Prefix(request))
		if err != nil {
			return nil, errors.Join(err, owner.Close())
		}
		if err := owner.Close(); err != nil {
			return nil, errors.Join(err, backend.Close())
		}
		return &sharedFlowcraftBackend{
			temporal: backend.TemporalStore(), evidence: backend.EvidenceStore(),
			queue: backend.AsyncSemanticQueue(), outbox: backend.SideEffectOutbox(),
			index: backend.RetrievalIndex(), indexCloser: backend,
		}, nil
	default:
		return nil, fmt.Errorf("memory store: flowcraft driver cannot use connection type %q", connectionType)
	}
}

func (backend *sharedFlowcraftBackend) NewStore(ctx context.Context, request Request) (Result, io.Closer, error) {
	policy := request.Layout.Spec.Flowcraft
	config, err := flowcraftConfig(policy, request.ModelLoader)
	if err != nil {
		return Result{}, nil, err
	}
	signature, err := projectionSignature(policy)
	if err != nil {
		return Result{}, nil, err
	}

	backend.mu.Lock()
	// A remote projection must be rebuilt from the newly configured logical
	// Store. Keep this rare signature-changing path serialized, but let the
	// normal unchanged-signature path construct independent Workspace Stores
	// without holding the physical backend mutex.
	if signature != backend.projectionSignature {
		backend.configure(&config, policy)
		store, err := backend.construct(ctx, config)
		if err != nil {
			backend.mu.Unlock()
			return Result{}, nil, err
		}
		if err := rebuildAllScopes(ctx, store, backend.temporal); err != nil {
			backend.mu.Unlock()
			return Result{}, nil, errors.Join(err, store.Close())
		}
		backend.projectionSignature = signature
		backend.mu.Unlock()
		return flowcraftStoreResult(store), store, nil
	}

	backend.configure(&config, policy)
	backend.mu.Unlock()
	store, err := backend.construct(ctx, config)
	if err != nil {
		return Result{}, nil, err
	}
	return flowcraftStoreResult(store), store, nil
}

func (backend *sharedFlowcraftBackend) configure(
	config *memoryflowcraft.Config,
	policy apitypes.FlowcraftMemoryLayoutPolicy,
) {
	config.TemporalStore = backend.temporal
	config.EvidenceStore = backend.evidence
	config.SideEffectOutbox = backend.outbox
	config.RetrievalIndex = backend.index
	if policy.Write.Mode == apitypes.FlowcraftMemoryWritePolicyModeAsyncSemantic {
		config.AsyncQueue = backend.queue
	}
}

func (backend *sharedFlowcraftBackend) construct(
	ctx context.Context,
	config memoryflowcraft.Config,
) (*memoryflowcraft.Store, error) {
	if backend.newStore != nil {
		return backend.newStore(ctx, config)
	}
	return memoryflowcraft.New(ctx, config)
}

func flowcraftStoreResult(store *memoryflowcraft.Store) Result {
	return Result{
		Store: store, Driver: string(apitypes.RuntimeProfileMemoryDriverFlowcraft),
	}
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

var _ memory.Store = (*memoryflowcraft.Store)(nil)
