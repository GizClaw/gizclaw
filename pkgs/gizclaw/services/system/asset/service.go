// Package asset owns stable asset references, metadata, and ObjectStore-backed
// binary lifecycle for GizClaw product services.
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

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

const (
	assetSchemaVersion = 1
	maxIDAttempts      = 8
	stagingGrace       = 15 * time.Minute
)

// Options supplies deterministic seams for a Service.
type Options struct {
	IDGenerator IDGenerator
	Now         func() time.Time
}

// Service owns immutable asset metadata and binary lifecycle.
//
// The metadata and object stores remain owned by the composition root and are
// not closed by Service. Callers own business references and deletion policy.
type Service struct {
	repo    repository
	objects objectstore.ObjectStore
	newID   IDGenerator
	now     func() time.Time
	mu      sync.Mutex
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
	return &Service{
		repo:    repository{store: metadata},
		objects: objects,
		newID:   newID,
		now:     now,
	}, nil
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

// Delete removes an asset without inspecting business references and is safe
// to retry after partial cleanup. The caller owns referential integrity.
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
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	switch record.State {
	case stateReady:
		record.State = stateDeleting
		if err := s.repo.putAsset(ctx, record); err != nil {
			return err
		}
	case stateDeleting:
	case stateStaging:
		return fmt.Errorf("%w: asset %s is not deletable from state %s", ErrConflict, ref, record.State)
	default:
		return fmt.Errorf("%w: asset %s has state %q", ErrInvalid, ref, record.State)
	}
	return s.finishDelete(ctx, record)
}

// Reconcile resumes interrupted internal lifecycle work and validates ready records.
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
		if record.ExpiresAt != nil && !now.Before(*record.ExpiresAt) {
			if err := s.finishDelete(ctx, record); err != nil {
				errs = append(errs, err)
			}
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
	if objectErr == nil || errors.Is(objectErr, fs.ErrNotExist) {
		return errors.Join(cause, s.repo.deleteAsset(ctx, id))
	}
	return errors.Join(cause, fmt.Errorf("asset staging object cleanup %s: %w", id, objectErr))
}

func (s *Service) finishDelete(ctx context.Context, record assetRecord) error {
	if err := s.objects.Delete(objectName(record.ID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("asset object delete %s: %w", record.ID, err)
	}
	if err := s.repo.deleteAsset(ctx, record.ID); err != nil && !errors.Is(err, kv.ErrNotFound) {
		return err
	}
	return nil
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
