// Package asset owns stable asset references, metadata, reverse bindings, and
// ObjectStore-backed binary lifecycle for GizClaw product services.
package asset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"mime"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

const (
	assetSchemaVersion = 1
	maxIDAttempts      = 8
	stagingGrace       = 15 * time.Minute
)

// Options supplies deterministic seams and owner resolvers for a Service.
type Options struct {
	IDGenerator IDGenerator
	Now         func() time.Time
	Resolvers   map[OwnerKind]OwnerResolver
}

// Service owns asset metadata, reverse bindings, and binary lifecycle.
//
// The metadata and object stores remain owned by the composition root and are
// not closed by Service.
type Service struct {
	repo       repository
	objects    objectstore.ObjectStore
	newID      IDGenerator
	now        func() time.Time
	mu         sync.Mutex
	resolverMu sync.RWMutex
	resolvers  map[OwnerKind]OwnerResolver
}

// New creates an AssetService without starting background work.
func New(metadata kv.Store, objects objectstore.ObjectStore, options Options) (*Service, error) {
	if metadata == nil {
		return nil, fmt.Errorf("%w: metadata store is required", ErrInvalid)
	}
	if objects == nil {
		return nil, fmt.Errorf("%w: object store is required", ErrInvalid)
	}
	newID := options.IDGenerator
	if newID == nil {
		newID = randomID
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	service := &Service{
		repo:      repository{store: metadata},
		objects:   objects,
		newID:     newID,
		now:       now,
		resolvers: make(map[OwnerKind]OwnerResolver),
	}
	for kind, resolver := range options.Resolvers {
		if err := service.RegisterOwnerResolver(kind, resolver); err != nil {
			return nil, err
		}
	}
	return service, nil
}

// RegisterOwnerResolver registers the resolver for one closed owner kind.
func (s *Service) RegisterOwnerResolver(kind OwnerKind, resolver OwnerResolver) error {
	if s == nil {
		return fmt.Errorf("%w: nil service", ErrInvalid)
	}
	if !kind.Valid() || resolver == nil {
		return fmt.Errorf("%w: invalid owner resolver for %q", ErrInvalid, kind)
	}
	s.resolverMu.Lock()
	defer s.resolverMu.Unlock()
	if _, exists := s.resolvers[kind]; exists {
		return fmt.Errorf("%w: owner resolver already registered for %q", ErrConflict, kind)
	}
	s.resolvers[kind] = resolver
	return nil
}

// Put streams a new immutable asset into ObjectStore and publishes it atomically.
func (s *Service) Put(ctx context.Context, request PutRequest, body io.Reader) (Asset, error) {
	if s == nil || body == nil {
		return Asset{}, fmt.Errorf("%w: service and body are required", ErrInvalid)
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(request.MediaType))
	if err != nil || mediaType == "" || !strings.Contains(mediaType, "/") {
		return Asset{}, fmt.Errorf("%w: invalid media type %q", ErrInvalid, request.MediaType)
	}
	mediaType = strings.ToLower(mediaType)
	if request.MaxBytes <= 0 {
		return Asset{}, fmt.Errorf("%w: max bytes must be positive", ErrInvalid)
	}
	now := s.now().UTC()
	var expiresAt *time.Time
	if request.ExpiresAt != nil {
		deadline := request.ExpiresAt.UTC()
		if !deadline.After(now) {
			return Asset{}, fmt.Errorf("%w: expiration must be in the future", ErrInvalid)
		}
		expiresAt = &deadline
	}
	record, err := s.reserve(ctx, mediaType, now, expiresAt)
	if err != nil {
		return Asset{}, err
	}

	upload := newUploadReader(ctx, body, request.MaxBytes)
	objectName := objectName(record.ID)
	if expiresAt == nil {
		err = s.objects.Put(objectName, upload)
	} else {
		err = s.objects.PutWithDeadline(objectName, upload, *expiresAt)
	}
	if err == nil && upload.size > request.MaxBytes {
		err = ErrTooLarge
	}
	if err != nil {
		return Asset{}, s.rollbackStaging(ctx, record.ID, fmt.Errorf("asset object put: %w", err))
	}

	record.State = stateReady
	record.SizeBytes = upload.size
	record.SHA256 = hex.EncodeToString(upload.digest.Sum(nil))
	if err := s.repo.putAsset(ctx, record); err != nil {
		return Asset{}, s.rollbackStaging(ctx, record.ID, err)
	}
	return assetFromRecord(record)
}

// Get returns ready asset metadata without opening the binary body.
func (s *Service) Get(ctx context.Context, ref Ref) (Asset, error) {
	record, err := s.readyRecord(ctx, ref)
	if err != nil {
		return Asset{}, err
	}
	return assetFromRecord(record)
}

// Open returns ready metadata and an owned reader for the immutable binary body.
func (s *Service) Open(ctx context.Context, ref Ref) (Asset, io.ReadCloser, error) {
	record, err := s.readyRecord(ctx, ref)
	if err != nil {
		return Asset{}, nil, err
	}
	reader, err := s.objects.Get(objectName(record.ID))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Asset{}, nil, fmt.Errorf("%w: %s", ErrNotFound, ref)
		}
		return Asset{}, nil, fmt.Errorf("asset object open %s: %w", ref, err)
	}
	asset, err := assetFromRecord(record)
	if err != nil {
		_ = reader.Close()
		return Asset{}, nil, err
	}
	return asset, newVerifyingReadCloser(reader, asset.Metadata), nil
}

// Bind records an active reverse reference after verifying the current owner
// structure contains ref. Call Protect before a not-yet-committed owner write.
func (s *Service) Bind(ctx context.Context, ref Ref, binding Binding) error {
	if s == nil {
		return fmt.Errorf("%w: nil service", ErrInvalid)
	}
	if err := validateOwner(binding.Owner); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.readyRecord(ctx, ref)
	if err != nil {
		return err
	}
	live, err := s.bindingLive(ctx, ref, binding)
	if err != nil {
		return err
	}
	if !live {
		return fmt.Errorf("%w: owner does not reference %s", ErrInvalid, ref)
	}
	return s.repo.bind(ctx, record.ID, binding, bindingStateActive, s.now(), record.ExpiresAt)
}

// Protect creates a pending reverse reference before an owner write so Delete
// cannot race the owner commit. The caller must Activate or Unbind it.
func (s *Service) Protect(ctx context.Context, ref Ref, binding Binding) error {
	if s == nil {
		return fmt.Errorf("%w: nil service", ErrInvalid)
	}
	if err := validateOwner(binding.Owner); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.readyRecord(ctx, ref)
	if err != nil {
		return err
	}
	return s.repo.bind(ctx, record.ID, binding, bindingStatePending, s.now(), record.ExpiresAt)
}

// Activate verifies a protected owner commit and publishes its binding.
func (s *Service) Activate(ctx context.Context, ref Ref, binding Binding) error {
	if s == nil {
		return fmt.Errorf("%w: nil service", ErrInvalid)
	}
	if err := validateOwner(binding.Owner); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.readyRecord(ctx, ref)
	if err != nil {
		return err
	}
	live, err := s.bindingLive(ctx, ref, binding)
	if err != nil {
		return err
	}
	if !live {
		return fmt.Errorf("%w: owner does not reference %s", ErrInvalid, ref)
	}
	return s.repo.bind(ctx, record.ID, binding, bindingStateActive, s.now(), record.ExpiresAt)
}

// Unbind removes one reverse reference from an asset and owner.
func (s *Service) Unbind(ctx context.Context, ref Ref, binding Binding) error {
	if s == nil {
		return fmt.Errorf("%w: nil service", ErrInvalid)
	}
	if err := validateOwner(binding.Owner); err != nil {
		return err
	}
	id, err := ref.id()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.repo.unbind(ctx, id, binding.Owner)
}

// Bindings lists reverse references for an asset.
func (s *Service) Bindings(ctx context.Context, ref Ref) ([]Binding, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: nil service", ErrInvalid)
	}
	id, err := ref.id()
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.asset(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.bindings(ctx, id)
}

// LiveBindings resolves owners, removes stale reverse references, and returns
// only bindings whose current owner structure still contains ref.
func (s *Service) LiveBindings(ctx context.Context, ref Ref) ([]Binding, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: nil service", ErrInvalid)
	}
	id, err := ref.id()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.readyRecord(ctx, ref); err != nil {
		return nil, err
	}
	records, err := s.repo.bindingRecords(ctx, id)
	if err != nil {
		return nil, err
	}
	liveBindings := make([]Binding, 0, len(records))
	for _, record := range records {
		binding := Binding{Owner: record.Owner}
		if record.State == bindingStatePending {
			continue
		}
		live, err := s.bindingLive(ctx, ref, binding)
		if err != nil {
			return nil, err
		}
		if live {
			liveBindings = append(liveBindings, binding)
			continue
		}
		if err := s.repo.unbind(ctx, id, binding.Owner); err != nil {
			return nil, err
		}
	}
	return liveBindings, nil
}

// UnbindOwner removes all reverse references for an owner without deleting assets.
func (s *Service) UnbindOwner(ctx context.Context, owner Owner) error {
	if s == nil {
		return fmt.Errorf("%w: nil service", ErrInvalid)
	}
	if err := validateOwner(owner); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ids, err := s.repo.ownerAssetIDs(ctx, owner)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.repo.unbind(ctx, id, owner); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes an unreferenced asset and is safe to retry after partial cleanup.
func (s *Service) Delete(ctx context.Context, ref Ref) error {
	if s == nil {
		return fmt.Errorf("%w: nil service", ErrInvalid)
	}
	id, err := ref.id()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.repo.asset(ctx, id)
	if err != nil {
		return err
	}
	if record.State == stateReady {
		bindings, err := s.repo.bindingRecords(ctx, id)
		if err != nil {
			return err
		}
		for _, record := range bindings {
			binding := Binding{Owner: record.Owner}
			if record.State == bindingStatePending && s.now().UTC().Sub(record.CreatedAt.UTC()) < stagingGrace {
				return fmt.Errorf("%w: %s has a pending owner commit", ErrInUse, ref)
			}
			live, err := s.bindingLive(ctx, ref, binding)
			if err != nil {
				return err
			}
			if live {
				return fmt.Errorf("%w: %s is referenced by %s/%s", ErrInUse, ref, binding.Owner.Kind, binding.Owner.ID)
			}
			if err := s.repo.unbind(ctx, id, binding.Owner); err != nil {
				return err
			}
		}
		record.State = stateDeleting
		if err := s.repo.putAsset(ctx, record); err != nil {
			return err
		}
	} else if record.State != stateDeleting {
		return fmt.Errorf("%w: asset %s is not deletable from state %s", ErrConflict, ref, record.State)
	}
	return s.finishDelete(ctx, record)
}

// Reconcile resumes interrupted lifecycle work and validates ready records and bindings.
func (s *Service) Reconcile(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("%w: nil service", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	now := s.now().UTC()
	for entry, err := range s.repo.store.List(ctx, kv.Key{"assets", "by-id"}) {
		if err != nil {
			errs = append(errs, fmt.Errorf("asset reconcile list: %w", err))
			continue
		}
		var record assetRecord
		if err := json.Unmarshal(entry.Value, &record); err != nil {
			errs = append(errs, fmt.Errorf("asset reconcile decode %v: %w", entry.Key, err))
			continue
		}
		if len(entry.Key) != 3 || record.SchemaVersion != assetSchemaVersion || entry.Key[2] != record.ID {
			errs = append(errs, fmt.Errorf("%w: invalid asset record %v", ErrInvalid, entry.Key))
			continue
		}
		if _, err := refFromID(record.ID); err != nil {
			errs = append(errs, fmt.Errorf("asset reconcile record %v: %w", entry.Key, err))
			continue
		}
		switch record.State {
		case stateStaging:
			if now.Sub(record.CreatedAt) >= stagingGrace {
				if err := s.finishDelete(ctx, record); err != nil {
					errs = append(errs, err)
				}
			}
		case stateDeleting:
			if err := s.finishDelete(ctx, record); err != nil {
				errs = append(errs, err)
			}
		case stateReady:
			asset, err := assetFromRecord(record)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			reader, err := s.objects.Get(objectName(record.ID))
			if err != nil {
				errs = append(errs, fmt.Errorf("asset ready object %s: %w", record.ID, err))
				continue
			}
			verified := newVerifyingReadCloser(reader, asset.Metadata)
			if _, err := io.Copy(io.Discard, verified); err != nil {
				errs = append(errs, fmt.Errorf("asset ready object verify %s: %w", record.ID, err))
			}
			if err := verified.Close(); err != nil {
				errs = append(errs, fmt.Errorf("asset ready object close %s: %w", record.ID, err))
			}
		default:
			errs = append(errs, fmt.Errorf("%w: asset %s has state %q", ErrInvalid, record.ID, record.State))
		}
	}
	for entry, err := range s.repo.store.List(ctx, kv.Key{"bindings", "by-asset"}) {
		if err != nil {
			errs = append(errs, fmt.Errorf("asset binding reconcile list: %w", err))
			continue
		}
		if len(entry.Key) != 5 {
			errs = append(errs, fmt.Errorf("%w: invalid binding key %v", ErrInvalid, entry.Key))
			continue
		}
		ref, err := refFromID(entry.Key[2])
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var binding bindingRecord
		if err := json.Unmarshal(entry.Value, &binding); err != nil {
			errs = append(errs, fmt.Errorf("asset binding reconcile decode: %w", err))
			continue
		}
		if _, err := s.repo.asset(ctx, entry.Key[2]); errors.Is(err, ErrNotFound) {
			if err := s.repo.unbind(ctx, entry.Key[2], binding.Owner); err != nil {
				errs = append(errs, err)
			}
			continue
		} else if err != nil {
			errs = append(errs, err)
			continue
		}
		live, err := s.bindingLive(ctx, ref, Binding{Owner: binding.Owner})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if live && binding.State == bindingStatePending {
			assetRecord, err := s.repo.asset(ctx, entry.Key[2])
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if err := s.repo.bind(ctx, entry.Key[2], Binding{Owner: binding.Owner}, bindingStateActive, now, assetRecord.ExpiresAt); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if !live && (binding.State == bindingStateActive || now.Sub(binding.CreatedAt.UTC()) >= stagingGrace) {
			if err := s.repo.unbind(ctx, entry.Key[2], binding.Owner); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (s *Service) reserve(ctx context.Context, mediaType string, now time.Time, expiresAt *time.Time) (assetRecord, error) {
	for range maxIDAttempts {
		idBytes, err := s.newID()
		if err != nil {
			return assetRecord{}, fmt.Errorf("asset id generate: %w", err)
		}
		id := hex.EncodeToString(idBytes[:])
		s.mu.Lock()
		_, err = s.repo.asset(ctx, id)
		if err == nil {
			s.mu.Unlock()
			continue
		}
		if !errors.Is(err, ErrNotFound) {
			s.mu.Unlock()
			return assetRecord{}, err
		}
		record := assetRecord{
			SchemaVersion: assetSchemaVersion,
			ID:            id,
			MediaType:     mediaType,
			CreatedAt:     now,
			ExpiresAt:     expiresAt,
			State:         stateStaging,
		}
		err = s.repo.putAsset(ctx, record)
		s.mu.Unlock()
		if err != nil {
			return assetRecord{}, err
		}
		return record, nil
	}
	return assetRecord{}, fmt.Errorf("%w: asset id collision retry limit reached", ErrConflict)
}

func (s *Service) readyRecord(ctx context.Context, ref Ref) (assetRecord, error) {
	if s == nil {
		return assetRecord{}, fmt.Errorf("%w: nil service", ErrInvalid)
	}
	id, err := ref.id()
	if err != nil {
		return assetRecord{}, err
	}
	record, err := s.repo.asset(ctx, id)
	if err != nil {
		return assetRecord{}, err
	}
	if record.State != stateReady {
		return assetRecord{}, fmt.Errorf("%w: %s is not ready", ErrNotFound, ref)
	}
	if record.ExpiresAt != nil && !s.now().Before(*record.ExpiresAt) {
		return assetRecord{}, fmt.Errorf("%w: %s is expired", ErrNotFound, ref)
	}
	return record, nil
}

func (s *Service) rollbackStaging(ctx context.Context, id string, cause error) error {
	objectErr := s.objects.Delete(objectName(id))
	if objectErr == nil {
		return errors.Join(cause, s.repo.deleteAsset(ctx, id))
	}
	return errors.Join(cause, fmt.Errorf("asset staging object cleanup %s: %w", id, objectErr))
}

func (s *Service) finishDelete(ctx context.Context, record assetRecord) error {
	if err := s.objects.Delete(objectName(record.ID)); err != nil {
		return fmt.Errorf("asset object delete %s: %w", record.ID, err)
	}
	bindings, err := s.repo.bindings(ctx, record.ID)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		if err := s.repo.unbind(ctx, record.ID, binding.Owner); err != nil {
			return err
		}
	}
	return s.repo.deleteAsset(ctx, record.ID)
}

func (s *Service) bindingLive(ctx context.Context, ref Ref, binding Binding) (bool, error) {
	s.resolverMu.RLock()
	resolver := s.resolvers[binding.Owner.Kind]
	s.resolverMu.RUnlock()
	if resolver == nil {
		return false, fmt.Errorf("%w: %s", ErrResolverUnavailable, binding.Owner.Kind)
	}
	snapshot, err := resolver.ResolveAssetOwner(ctx, binding.Owner)
	if err != nil {
		return false, fmt.Errorf("asset resolve owner %s/%s: %w", binding.Owner.Kind, binding.Owner.ID, err)
	}
	if !snapshot.Exists {
		return false, nil
	}
	for _, candidate := range snapshot.Refs {
		if candidate == ref {
			return true, nil
		}
	}
	return false, nil
}

func assetFromRecord(record assetRecord) (Asset, error) {
	ref, err := refFromID(record.ID)
	if err != nil {
		return Asset{}, err
	}
	digest, err := hex.DecodeString(record.SHA256)
	if err != nil || len(digest) != sha256.Size {
		return Asset{}, fmt.Errorf("%w: invalid sha256 for %s", ErrInvalid, ref)
	}
	var sha [sha256.Size]byte
	copy(sha[:], digest)
	return Asset{Metadata: Metadata{
		Ref:       ref,
		MediaType: record.MediaType,
		SizeBytes: record.SizeBytes,
		SHA256:    sha,
		CreatedAt: record.CreatedAt.UTC(),
		ExpiresAt: record.ExpiresAt,
	}}, nil
}

func validateOwner(owner Owner) error {
	if !owner.Kind.Valid() {
		return fmt.Errorf("%w: unsupported owner kind %q", ErrInvalid, owner.Kind)
	}
	if owner.ID == "" || owner.ID != strings.TrimSpace(owner.ID) || len(owner.ID) > 512 || strings.Count(owner.ID, "/") != 1 {
		return fmt.Errorf("%w: invalid owner id for %s", ErrInvalid, owner.Kind)
	}
	parts := strings.Split(owner.ID, "/")
	if parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("%w: invalid owner id for %s", ErrInvalid, owner.Kind)
	}
	if strings.IndexFunc(owner.ID, unicode.IsControl) >= 0 {
		return fmt.Errorf("%w: owner id contains control characters", ErrInvalid)
	}
	return nil
}

func objectName(id string) string {
	return "blobs/" + id[:2] + "/" + id
}

type uploadReader struct {
	ctx       context.Context
	reader    io.Reader
	remaining int64
	size      int64
	digest    hash.Hash
}

type verifyingReadCloser struct {
	reader   io.ReadCloser
	expected Metadata
	digest   hash.Hash
	size     int64
	verified bool
}

func newVerifyingReadCloser(reader io.ReadCloser, expected Metadata) *verifyingReadCloser {
	return &verifyingReadCloser{reader: reader, expected: expected, digest: sha256.New()}
}

func (r *verifyingReadCloser) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.size += int64(n)
		_, _ = r.digest.Write(p[:n])
	}
	if err == io.EOF && !r.verified {
		r.verified = true
		var digest [sha256.Size]byte
		copy(digest[:], r.digest.Sum(nil))
		if r.size != r.expected.SizeBytes || digest != r.expected.SHA256 {
			return n, fmt.Errorf("%w: %s metadata size=%d sha256=%x, object size=%d sha256=%x", ErrIntegrity, r.expected.Ref, r.expected.SizeBytes, r.expected.SHA256, r.size, digest)
		}
	}
	return n, err
}

func (r *verifyingReadCloser) Close() error {
	return r.reader.Close()
}

func newUploadReader(ctx context.Context, reader io.Reader, maxBytes int64) *uploadReader {
	return &uploadReader{
		ctx:       ctx,
		reader:    reader,
		remaining: maxBytes + 1,
		digest:    sha256.New(),
	}
}

func (r *uploadReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	if n > 0 {
		r.remaining -= int64(n)
		r.size += int64(n)
		_, _ = r.digest.Write(p[:n])
	}
	return n, err
}
