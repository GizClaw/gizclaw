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
	"os"
	"path/filepath"
	"strings"
	"time"

	flowrecall "github.com/GizClaw/flowcraft/memory/recall"
	flowpostgres "github.com/GizClaw/flowcraft/memory/recall/store/postgres"
	flowworkspace "github.com/GizClaw/flowcraft/memory/recall/store/workspace"
	"github.com/GizClaw/flowcraft/memory/retrieval/bbh"
	retrievalpostgres "github.com/GizClaw/flowcraft/memory/retrieval/postgres"
	sdkworkspace "github.com/GizClaw/flowcraft/sdk/workspace"

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
	case "flowcraft_bbh":
		dir, err := managedBindingRoot(request.ServerRoot, request.ProfileID, request.BindingName)
		if err != nil {
			return nil, nil, err
		}
		return openFlowcraftLocal(ctx, dir, policy, config)
	case "flowcraft_object_store":
		connection, err := request.Binding.Connection.AsRuntimeProfileFlowcraftObjectStoreConnection()
		if err != nil {
			return nil, nil, err
		}
		dir := filepath.Clean(connection.Directory)
		return openFlowcraftLocal(ctx, dir, policy, config)
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

func managedBindingRoot(serverRoot, profileID, bindingName string) (string, error) {
	serverRoot = strings.TrimSpace(serverRoot)
	if serverRoot == "" {
		return "", errors.New("memory store: flowcraft_bbh requires the Server Workspace root")
	}
	if strings.TrimSpace(profileID) == "" || strings.TrimSpace(profileID) != profileID || !safePathSegment(bindingName) {
		return "", errors.New("memory store: RuntimeProfile ID is required and binding alias must be a safe path segment")
	}
	absoluteRoot, err := filepath.Abs(serverRoot)
	if err != nil {
		return "", fmt.Errorf("memory store: resolve Server Workspace root: %w", err)
	}
	base := filepath.Join(absoluteRoot, "data", "memory")
	target := filepath.Join(base, customid.OpaquePathSegment(profileID), bindingName)
	relative, err := filepath.Rel(base, target)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", errors.New("memory store: managed binding path escapes the Server Workspace root")
	}
	if err := rejectSymlinkPath(absoluteRoot, target); err != nil {
		return "", err
	}
	return target, nil
}

func safePathSegment(value string) bool {
	trimmed := strings.TrimSpace(value)
	return value == trimmed && value != "" && value != "." && value != ".." && filepath.Base(value) == value &&
		!strings.ContainsAny(value, `/\`)
}

func rejectSymlinkPath(root, target string) error {
	current := filepath.Clean(root)
	for {
		info, err := os.Lstat(current)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("memory store: managed path %q contains a symlink", current)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("memory store: inspect managed path %q: %w", current, err)
		}
		if current == filepath.Clean(target) {
			return nil
		}
		relative, err := filepath.Rel(current, target)
		if err != nil || relative == "." {
			return nil
		}
		next := strings.Split(relative, string(filepath.Separator))[0]
		current = filepath.Join(current, next)
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

func openFlowcraftLocal(ctx context.Context, dir string, policy apitypes.FlowcraftMemoryLayoutPolicy, config memoryflowcraft.Config) (*memoryflowcraft.Store, io.Closer, error) {
	metadataWorkspace, err := sdkworkspace.NewLocalWorkspace(dir)
	if err != nil {
		return nil, nil, err
	}
	backend, err := flowworkspace.New(metadataWorkspace)
	if err != nil {
		return nil, nil, err
	}
	owned := []io.Closer{backend}
	fail := func(err error) (*memoryflowcraft.Store, io.Closer, error) {
		return nil, nil, errors.Join(err, closeAll(owned))
	}
	if err := ensureLocalProjection(ctx, dir, backend.TemporalStore(), backend.EvidenceStore(), backend.SideEffectOutbox(), policy, config); err != nil {
		return fail(err)
	}
	retrievalWorkspace, err := sdkworkspace.NewLocalWorkspace(filepath.Join(dir, "retrieval"))
	if err != nil {
		return fail(err)
	}
	index, err := bbh.New(retrievalWorkspace, bbh.WithConfig(mapBBHConfig(policy.Bbh)))
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
	return store, multiCloser(append(owned, store)), nil
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

const projectionManifestName = ".gizclaw-retrieval-layout"

func ensureLocalProjection(
	ctx context.Context,
	dir string,
	temporal flowrecall.TemporalStore,
	evidence flowrecall.EvidenceStore,
	outbox flowrecall.SideEffectOutbox,
	policy apitypes.FlowcraftMemoryLayoutPolicy,
	config memoryflowcraft.Config,
) error {
	prepared, err := prepareLocalProjection(ctx, dir, temporal, evidence, outbox, policy, config)
	if err != nil || prepared == nil {
		return err
	}
	defer prepared.Abort()
	index, err := prepared.Publish()
	if err != nil {
		return err
	}
	return index.Close()
}

type preparedLocalProjection struct {
	dir        string
	stagingDir string
	config     bbh.Config
	published  bool
}

func prepareLocalProjection(
	ctx context.Context,
	dir string,
	temporal flowrecall.TemporalStore,
	evidence flowrecall.EvidenceStore,
	outbox flowrecall.SideEffectOutbox,
	policy apitypes.FlowcraftMemoryLayoutPolicy,
	config memoryflowcraft.Config,
) (*preparedLocalProjection, error) {
	signature, err := projectionSignature(policy)
	if err != nil {
		return nil, err
	}
	retrievalDir := filepath.Join(dir, "retrieval")
	manifestPath := filepath.Join(retrievalDir, projectionManifestName)
	if current, err := os.ReadFile(manifestPath); err == nil && strings.TrimSpace(string(current)) == signature {
		return nil, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("memory store: read derived-index manifest: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("memory store: create Flowcraft binding root: %w", err)
	}
	stagingDir, err := os.MkdirTemp(dir, ".retrieval-rebuild-")
	if err != nil {
		return nil, fmt.Errorf("memory store: create derived-index staging directory: %w", err)
	}
	bbhConfig := mapBBHConfig(policy.Bbh)
	prepared := &preparedLocalProjection{
		dir: dir, stagingDir: stagingDir, config: bbhConfig,
	}
	fail := func(err error) (*preparedLocalProjection, error) {
		prepared.Abort()
		return nil, err
	}
	stagingWorkspace, err := sdkworkspace.NewLocalWorkspace(stagingDir)
	if err != nil {
		return fail(err)
	}
	index, err := bbh.New(stagingWorkspace, bbh.WithConfig(bbhConfig))
	if err != nil {
		return fail(err)
	}
	stagingConfig := config
	stagingConfig.TemporalStore = temporal
	stagingConfig.EvidenceStore = evidence
	stagingConfig.SideEffectOutbox = outbox
	stagingConfig.AsyncQueue = nil
	stagingConfig.RetrievalIndex = index
	store, err := memoryflowcraft.New(ctx, stagingConfig)
	if err != nil {
		return fail(errors.Join(err, index.Close()))
	}
	rebuildErr := rebuildAllScopes(ctx, store, temporal)
	closeErr := errors.Join(store.Close(), index.Close())
	if rebuildErr != nil || closeErr != nil {
		return fail(errors.Join(rebuildErr, closeErr))
	}
	if err := writeProjectionManifest(filepath.Join(stagingDir, projectionManifestName), signature); err != nil {
		return fail(err)
	}
	return prepared, nil
}

func (prepared *preparedLocalProjection) Publish() (*bbh.Index, error) {
	if prepared == nil {
		return nil, errors.New("memory store: prepared derived index is required")
	}
	retrievalDir := filepath.Join(prepared.dir, "retrieval")
	backupDir := filepath.Join(prepared.dir, ".retrieval-previous")
	_ = os.RemoveAll(backupDir)
	hadPrevious := false
	if _, err := os.Stat(retrievalDir); err == nil {
		if err := os.Rename(retrievalDir, backupDir); err != nil {
			return nil, fmt.Errorf("memory store: preserve previous derived index: %w", err)
		}
		hadPrevious = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("memory store: inspect previous derived index: %w", err)
	}
	restore := func() {
		_ = os.RemoveAll(retrievalDir)
		if hadPrevious {
			_ = os.Rename(backupDir, retrievalDir)
		}
	}
	if err := os.Rename(prepared.stagingDir, retrievalDir); err != nil {
		restore()
		return nil, fmt.Errorf("memory store: publish rebuilt derived index: %w", err)
	}
	retrievalWorkspace, err := sdkworkspace.NewLocalWorkspace(retrievalDir)
	if err != nil {
		restore()
		return nil, err
	}
	index, err := bbh.New(retrievalWorkspace, bbh.WithConfig(prepared.config))
	if err != nil {
		restore()
		return nil, fmt.Errorf("memory store: open rebuilt derived index: %w", err)
	}
	prepared.published = true
	if hadPrevious {
		_ = os.RemoveAll(backupDir)
	}
	return index, nil
}

func (prepared *preparedLocalProjection) Abort() {
	if prepared == nil || prepared.published {
		return
	}
	_ = os.RemoveAll(prepared.stagingDir)
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
		BBH          *apitypes.FlowcraftMemoryBBHPolicy   `json:"bbh,omitempty"`
		GraphEnabled *bool                                `json:"graph_enabled,omitempty"`
	}{
		Embedding: policy.Embedding, Rerank: policy.Rerank, BBH: policy.Bbh, GraphEnabled: policy.GraphEnabled,
	})
	if err != nil {
		return "", fmt.Errorf("memory store: encode derived-index policy: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func mapBBHConfig(policy *apitypes.FlowcraftMemoryBBHPolicy) bbh.Config {
	config := bbh.Config{}
	if policy == nil {
		return config
	}
	if policy.SearchOverfetch != nil {
		config.SearchOverfetch = *policy.SearchOverfetch
	}
	if policy.Bleve != nil {
		if policy.Bleve.Analyzer != nil {
			config.Bleve.Analyzer = string(*policy.Bleve.Analyzer)
		}
		if policy.Bleve.Gojieba != nil {
			value := policy.Bleve.Gojieba
			if value.Mode != nil {
				config.Bleve.Gojieba.Mode = string(*value.Mode)
			}
			config.Bleve.Gojieba.HMM = value.Hmm
			config.Bleve.Gojieba.DictPath = valueOrEmpty(value.DictPath)
			config.Bleve.Gojieba.HMMPath = valueOrEmpty(value.HmmPath)
			config.Bleve.Gojieba.UserDictPath = valueOrEmpty(value.UserDictPath)
			config.Bleve.Gojieba.IDFPath = valueOrEmpty(value.IdfPath)
			config.Bleve.Gojieba.StopWordsPath = valueOrEmpty(value.StopWordsPath)
		}
	}
	if policy.Hnsw != nil && policy.Hnsw.FlushInterval != nil {
		if value, err := time.ParseDuration(*policy.Hnsw.FlushInterval); err == nil {
			config.HNSW.FlushInterval.Duration = value
		}
	}
	return config
}

func writeProjectionManifest(path, signature string) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".retrieval-layout-")
	if err != nil {
		return fmt.Errorf("memory store: create derived-index manifest: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.WriteString(signature + "\n"); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("memory store: publish derived-index manifest: %w", err)
	}
	return nil
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
