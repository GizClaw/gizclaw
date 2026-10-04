package memorystore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	memorymem0 "github.com/GizClaw/gizclaw-go/pkgs/store/memory/mem0"
)

// sharedBackend owns only deployment-specific physical dependencies. NewStore
// creates a policy-bearing logical Store for one Workspace generation.
type sharedBackend interface {
	NewStore(context.Context, Request) (Result, io.Closer, error)
	Close() error
}

func openSharedBackend(ctx context.Context, request Request) (sharedBackend, error) {
	switch request.Binding.Driver {
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

func (backend *sharedRemoteBackend) NewStore(_ context.Context, request Request) (Result, io.Closer, error) {
	if request.Binding.Driver == apitypes.RuntimeProfileMemoryDriverMem0 {
		connectionType, err := request.Binding.Connection.Discriminator()
		if err != nil {
			return Result{}, nil, err
		}
		if connectionType == "mem0_self_hosted" {
			instructions, err := selfHostedInstructions(request.Layout.Spec.Mem0SelfHosted)
			if err != nil {
				return Result{}, nil, err
			}
			store, ok := backend.result.Store.(*memorymem0.Store)
			if !ok {
				return Result{}, nil, errors.New("self-hosted mem0 backend has an incompatible Store")
			}
			logical, err := store.WithCustomInstructions(instructions)
			return Result{Store: logical, Driver: backend.result.Driver}, nil, err
		}
	}
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
