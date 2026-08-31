// Package memorystore constructs one Workspace-owned provider-neutral Memory
// Store from a RuntimeProfile binding and its connection-free MemoryLayout.
package memorystore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	flowrecall "github.com/GizClaw/flowcraft/memory/recall"
	flowpostgres "github.com/GizClaw/flowcraft/memory/recall/store/postgres"
	retrievalpostgres "github.com/GizClaw/flowcraft/memory/retrieval/postgres"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	memoryflowcraft "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft"
	flowcraftredis8 "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft/redis8"
	memorymem0 "github.com/GizClaw/gizclaw-go/pkgs/store/memory/mem0"
	memoryvolc "github.com/GizClaw/gizclaw-go/pkgs/store/memory/volc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

type Request struct {
	WorkspaceID     string
	ProfileID       string
	ProfileRevision string
	BindingName     string
	Layout          apitypes.MemoryLayout
	Binding         apitypes.RuntimeProfileMemoryBinding
	ModelLoader     memoryflowcraft.ModelLoader
	ServerRoot      string
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
	case apitypes.RuntimeProfileMemoryDriverFlowcraft:
		store, closer, err := buildFlowcraft(ctx, request)
		if err != nil {
			return Result{}, err
		}
		return Result{Store: store, Driver: string(request.Binding.Driver), Closer: closer}, nil
	case apitypes.RuntimeProfileMemoryDriverMem0:
		connection, err := request.Binding.Connection.AsRuntimeProfileMem0Connection()
		if err != nil {
			return Result{}, fmt.Errorf("memory store: decode mem0 connection: %w", err)
		}
		poll, err := parsePollInterval(connection.PollInterval)
		if err != nil {
			return Result{}, err
		}
		// ProjectId is the deployment identity paired with this key. Mem0's
		// data plane selects the Project through the API key and has no
		// project_id request field.
		store, err := memorymem0.New(memorymem0.Config{
			Endpoint: connection.Endpoint, APIKey: connection.ApiKey,
			Flavor: memorymem0.Platform, PollInterval: poll,
		})
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

func buildFlowcraft(ctx context.Context, request Request) (*memoryflowcraft.Store, io.Closer, error) {
	connectionType, err := request.Binding.Connection.Discriminator()
	if err != nil {
		return nil, nil, fmt.Errorf("memory store: decode flowcraft connection: %w", err)
	}
	policy := request.Layout.Spec.Flowcraft
	config, err := flowcraftConfig(policy, request.ModelLoader)
	if err != nil {
		return nil, nil, err
	}
	switch connectionType {
	case "flowcraft_postgresql":
		connection, err := request.Binding.Connection.AsRuntimeProfileFlowcraftPostgreSQLConnection()
		if err != nil {
			return nil, nil, err
		}
		return openFlowcraftPostgres(ctx, connection.Dsn, request.WorkspaceID, policy, config)
	case "flowcraft_redis8":
		connection, err := request.Binding.Connection.AsRuntimeProfileFlowcraftRedis8Connection()
		if err != nil {
			return nil, nil, err
		}
		return openFlowcraftRedis8(ctx, connection, flowcraftRedis8Prefix(request), policy, config)
	default:
		return nil, nil, fmt.Errorf("memory store: flowcraft driver cannot use connection type %q", connectionType)
	}
}

func flowcraftRedis8Prefix(request Request) string {
	sum := sha256.Sum256([]byte(request.ProfileID + "\x00" + request.BindingName))
	return "gizclaw:flowcraft:redis8:" + hex.EncodeToString(sum[:16])
}

func openFlowcraftRedis8(
	ctx context.Context,
	connection apitypes.RuntimeProfileFlowcraftRedis8Connection,
	prefix string,
	policy apitypes.FlowcraftMemoryLayoutPolicy,
	config memoryflowcraft.Config,
) (*memoryflowcraft.Store, io.Closer, error) {
	if boolValue(policy.GraphEnabled) {
		return nil, nil, errors.New("memory store: flowcraft_redis8 cannot enable graph until Flowcraft exposes graph store injection")
	}
	tlsCAFile := ""
	if connection.TlsCaFile != nil {
		tlsCAFile = *connection.TlsCaFile
	}
	owner, err := storage.New(map[string]storage.Config{
		"flowcraft-redis8": storage.RedisConfig{URL: connection.Url, TLSCAFile: tlsCAFile},
	})
	if err != nil {
		return nil, nil, err
	}
	client, err := owner.Redis("flowcraft-redis8")
	if err != nil {
		return nil, nil, errors.Join(err, owner.Close())
	}
	backend, err := flowcraftredis8.OpenBackend(ctx, client, prefix)
	if err != nil {
		return nil, nil, errors.Join(err, owner.Close())
	}
	if err := owner.Close(); err != nil {
		return nil, nil, errors.Join(err, backend.Close())
	}
	config.TemporalStore = backend.TemporalStore()
	config.EvidenceStore = backend.EvidenceStore()
	config.SideEffectOutbox = backend.SideEffectOutbox()
	config.RetrievalIndex = backend.RetrievalIndex()
	if policy.Write.Mode == apitypes.FlowcraftMemoryWritePolicyModeAsyncSemantic {
		config.AsyncQueue = backend.AsyncSemanticQueue()
	}
	store, err := memoryflowcraft.New(ctx, config)
	if err != nil {
		return nil, nil, errors.Join(err, backend.Close())
	}
	if err := rebuildAllScopes(ctx, store, backend.TemporalStore()); err != nil {
		return nil, nil, errors.Join(err, store.Close(), backend.Close())
	}
	return store, multiCloser([]io.Closer{backend, store}), nil
}

func flowcraftConfig(policy apitypes.FlowcraftMemoryLayoutPolicy, loader memoryflowcraft.ModelLoader) (memoryflowcraft.Config, error) {
	stageTimeout := time.Duration(0)
	if policy.Extraction.StageTimeout != nil {
		value, err := time.ParseDuration(*policy.Extraction.StageTimeout)
		if err != nil || value <= 0 {
			return memoryflowcraft.Config{}, errors.New("memory store: flowcraft extraction stage_timeout must be positive")
		}
		stageTimeout = value
	}
	var temperature *float64
	if policy.Extraction.Temperature != nil {
		value := float64(*policy.Extraction.Temperature)
		temperature = &value
	}
	systemPrompt := strings.TrimSpace(valueOrEmpty(policy.Extraction.SystemPrompt))
	if lanes := layoutLanePrompt(policy.Lanes); lanes != "" {
		if systemPrompt != "" {
			systemPrompt += "\n\n"
		}
		systemPrompt += lanes
	}
	extractionModel := policy.Extraction.Model
	if policy.Extraction.Enabled != nil && !*policy.Extraction.Enabled {
		extractionModel = ""
	}
	config := memoryflowcraft.Config{
		Loader: loader,
		Extraction: memoryflowcraft.ExtractionConfig{
			Model: extractionModel, Mode: flowrecall.LLMExtractionMode(policy.Extraction.Mode),
			SystemPrompt: systemPrompt, SchemaName: valueOrEmpty(policy.Extraction.SchemaName),
			Temperature: temperature, StageTimeout: stageTimeout,
		},
		GraphEnabled: boolValue(policy.GraphEnabled),
		Tier:         string(policy.Write.Tier),
		LaneNames:    flowcraftLaneNames(policy.Lanes),
	}
	if policy.Embedding != nil {
		config.Embedding.Model = policy.Embedding.Model
	}
	if policy.Rerank != nil {
		config.Rerank.Model = policy.Rerank.Model
	}
	return config, nil
}

func flowcraftLaneNames(lanes []apitypes.FlowcraftMemoryLanePolicy) []string {
	result := make([]string, 0, len(lanes))
	for _, lane := range lanes {
		if name := strings.TrimSpace(lane.Name); name != "" {
			result = append(result, name)
		}
	}
	return result
}

func openFlowcraftPostgres(ctx context.Context, dsn, workspaceID string, policy apitypes.FlowcraftMemoryLayoutPolicy, config memoryflowcraft.Config) (*memoryflowcraft.Store, io.Closer, error) {
	backend, err := flowpostgres.Open(ctx, dsn)
	if err != nil {
		return nil, nil, err
	}
	owned := []io.Closer{backend}
	fail := func(err error) (*memoryflowcraft.Store, io.Closer, error) {
		return nil, nil, errors.Join(err, closeAll(owned))
	}
	index, err := retrievalpostgres.Open(ctx, dsn)
	if err != nil {
		return fail(err)
	}
	owned = append(owned, index)
	config.TemporalStore = backend.TemporalStore()
	config.EvidenceStore = backend.EvidenceStore()
	config.SideEffectOutbox = backend.SideEffectOutbox()
	config.RetrievalIndex = index
	if policy.Write.Mode == apitypes.FlowcraftMemoryWritePolicyModeAsyncSemantic {
		config.AsyncQueue = backend.AsyncSemanticQueue()
	}
	store, err := memoryflowcraft.New(ctx, config)
	if err != nil {
		return fail(err)
	}
	if err := store.Rebuild(ctx, memory.Scope{AppID: workspaceID}); err != nil {
		return fail(errors.Join(err, store.Close()))
	}
	return store, multiCloser(append(owned, store)), nil
}

func rebuildAllScopes(ctx context.Context, store *memoryflowcraft.Store, temporal flowrecall.TemporalStore) error {
	enumerator, ok := temporal.(flowrecall.ScopeEnumerator)
	if !ok {
		return errors.New("memory store: Flowcraft canonical backend cannot enumerate scopes for rebuild")
	}
	scopes, err := enumerator.ListScopes(ctx, flowrecall.ScopeListQuery{})
	if err != nil {
		return fmt.Errorf("memory store: enumerate canonical scopes: %w", err)
	}
	for _, scope := range scopes {
		if err := store.Rebuild(ctx, memory.Scope{
			AppID: scope.RuntimeID, UserID: scope.UserID, AgentID: scope.AgentID,
		}); err != nil {
			return fmt.Errorf("memory store: rebuild Workspace %q derived index: %w", scope.RuntimeID, err)
		}
	}
	return nil
}

func projectionSignature(policy apitypes.FlowcraftMemoryLayoutPolicy) (string, error) {
	payload, err := json.Marshal(struct {
		Embedding    *apitypes.FlowcraftMemoryModelPolicy `json:"embedding,omitempty"`
		Rerank       *apitypes.FlowcraftMemoryModelPolicy `json:"rerank,omitempty"`
		GraphEnabled *bool                                `json:"graph_enabled,omitempty"`
	}{
		Embedding: policy.Embedding, Rerank: policy.Rerank, GraphEnabled: policy.GraphEnabled,
	})
	if err != nil {
		return "", fmt.Errorf("memory store: encode derived-index policy: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func layoutLanePrompt(lanes []apitypes.FlowcraftMemoryLanePolicy) string {
	if len(lanes) == 0 {
		return ""
	}
	var result strings.Builder
	result.WriteString("Classify extracted facts into these memory lanes. Begin every extracted fact with the exact lane name followed by a colon and space (\"<lane>: ...\"):\n")
	for _, lane := range lanes {
		fmt.Fprintf(&result, "- %s (%s)", lane.Name, lane.Kind)
		if lane.Description != nil && strings.TrimSpace(*lane.Description) != "" {
			fmt.Fprintf(&result, ": %s", strings.TrimSpace(*lane.Description))
		}
		if lane.Extract != nil && strings.TrimSpace(*lane.Extract) != "" {
			fmt.Fprintf(&result, " Extract: %s", strings.TrimSpace(*lane.Extract))
		}
		result.WriteByte('\n')
	}
	return strings.TrimSpace(result.String())
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
	for index := len(closers) - 1; index >= 0; index-- {
		if closers[index] != nil {
			err = errors.Join(err, closers[index].Close())
		}
	}
	return err
}
