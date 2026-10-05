// Package memorystore constructs one Layout-scoped provider-neutral Memory
// Store from a RuntimeProfile binding and its connection-free MemoryLayout.
package memorystore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	memorymem0 "github.com/GizClaw/gizclaw-go/pkgs/store/memory/mem0"
	memoryvolc "github.com/GizClaw/gizclaw-go/pkgs/store/memory/volc"
)

type Request struct {
	WorkspaceID     string
	OwnerPublicKey  string
	ProfileID       string
	ProfileRevision string
	BindingName     string
	Layout          apitypes.MemoryLayout
	Binding         apitypes.RuntimeProfileMemoryBinding
	ServerRoot      string

	// maintenance opens the binding for Workspace purge and verification:
	// it loads no model and never rebuilds a derived index.
	maintenance bool
}

type Result struct {
	Store  memory.Store
	Driver string
	Closer io.Closer
}

func Build(ctx context.Context, request Request) (Result, error) {
	if err := customid.ValidateResourceID(request.WorkspaceID); err != nil {
		return Result{}, fmt.Errorf("memory store: invalid workspace id: %w", err)
	}
	if err := validateLayoutBinding(request); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(request.BindingName) == "" {
		return Result{}, errors.New("memory store: binding name is required")
	}
	switch request.Binding.Driver {
	case apitypes.RuntimeProfileMemoryDriverMem0:
		store, err := buildMem0(request.Binding.Connection, request.Layout.Spec)
		if err != nil {
			return Result{}, fmt.Errorf("memory store: construct mem0: %w", err)
		}
		return Result{Store: store, Driver: string(request.Binding.Driver)}, nil
	case apitypes.RuntimeProfileMemoryDriverVolcMem0:
		connection, err := request.Binding.Connection.AsRuntimeProfileVolcMem0Connection()
		if err != nil {
			return Result{}, fmt.Errorf("memory store: decode volc_mem0 connection: %w", err)
		}
		poll, err := parsePollInterval(connection.PollInterval)
		if err != nil {
			return Result{}, err
		}
		// MemoryProjectId is retained for deployment identity and audit. The
		// selected data-plane key performs Project routing at runtime.
		store, err := memoryvolc.Open(ctx, memoryvolc.Config{
			Mem0: memorymem0.Config{
				Endpoint: connection.Endpoint, APIKey: connection.ApiKey,
				Flavor: memorymem0.VolcPlatform, PollInterval: poll,
			},
			MemoryProjectID: connection.MemoryProjectId,
		})
		if err != nil {
			return Result{}, fmt.Errorf("memory store: construct volc_mem0: %w", err)
		}
		return Result{Store: store, Driver: string(request.Binding.Driver)}, nil
	default:
		return Result{}, fmt.Errorf("memory store: unsupported driver %q", request.Binding.Driver)
	}
}

func buildMem0(connection apitypes.RuntimeProfileMemoryConnection, policy apitypes.MemoryLayoutSpec) (*memorymem0.Store, error) {
	connectionType, err := connection.Discriminator()
	if err != nil {
		return nil, fmt.Errorf("decode Mem0 connection: %w", err)
	}
	var config memorymem0.Config
	switch connectionType {
	case "mem0":
		value, err := connection.AsRuntimeProfileMem0Connection()
		if err != nil {
			return nil, err
		}
		poll, err := parsePollInterval(value.PollInterval)
		if err != nil {
			return nil, err
		}
		// The Platform API key selects its project; ProjectId is control-plane identity.
		config = memorymem0.Config{Endpoint: value.Endpoint, APIKey: value.ApiKey,
			Flavor: memorymem0.Platform, PollInterval: poll}
	case "mem0_self_hosted":
		value, err := connection.AsRuntimeProfileMem0SelfHostedConnection()
		if err != nil {
			return nil, err
		}
		config = memorymem0.Config{Endpoint: value.Endpoint, Flavor: memorymem0.SelfHosted}
		config.CustomInstructions, err = selfHostedInstructions(policy.Mem0SelfHosted)
		if err != nil {
			return nil, err
		}
		if value.ApiKey != nil {
			config.APIKey = *value.ApiKey
		}
	default:
		return nil, fmt.Errorf("unsupported Mem0 connection type %q", connectionType)
	}
	return memorymem0.New(config)
}

func selfHostedInstructions(policy *apitypes.Mem0SelfHostedMemoryLayoutPolicy) (string, error) {
	if policy == nil {
		return "", errors.New("memory store: spec.mem0_self_hosted is required for a mem0_self_hosted connection")
	}
	if policy.CustomInstructions != nil {
		return *policy.CustomInstructions, nil
	}
	return "", nil
}

func validateLayoutBinding(request Request) error {
	if request.Layout.Id == "" || request.Layout.Id != request.Binding.LayoutId {
		return fmt.Errorf(
			"memory store: layout id %q does not match binding layout_id %q",
			request.Layout.Id,
			request.Binding.LayoutId,
		)
	}
	return nil
}

func parsePollInterval(raw *string) (time.Duration, error) {
	if raw == nil {
		return 0, nil
	}
	value, err := time.ParseDuration(*raw)
	if err != nil || value <= 0 {
		return 0, errors.New("memory store: poll_interval must be a positive duration")
	}
	return value, nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

type multiCloser []io.Closer

func (closers multiCloser) Close() error { return closeAll(closers) }

func closeAll(closers []io.Closer) error {
	var err error
	for index := range slices.Backward(closers) {
		if closers[index] != nil {
			err = errors.Join(err, closers[index].Close())
		}
	}
	return err
}
