package firmware

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

var firmwaresRoot = kv.Key{"by-name"}

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

type Server struct {
	Store  kv.Store
	Assets objectstore.ObjectStore
	Now    func() time.Time
}

type FirmwareAdminService interface {
	ListFirmwares(context.Context, adminservice.ListFirmwaresRequestObject) (adminservice.ListFirmwaresResponseObject, error)
	CreateFirmware(context.Context, adminservice.CreateFirmwareRequestObject) (adminservice.CreateFirmwareResponseObject, error)
	DeleteFirmware(context.Context, adminservice.DeleteFirmwareRequestObject) (adminservice.DeleteFirmwareResponseObject, error)
	GetFirmware(context.Context, adminservice.GetFirmwareRequestObject) (adminservice.GetFirmwareResponseObject, error)
	PutFirmware(context.Context, adminservice.PutFirmwareRequestObject) (adminservice.PutFirmwareResponseObject, error)
	ReleaseFirmware(context.Context, adminservice.ReleaseFirmwareRequestObject) (adminservice.ReleaseFirmwareResponseObject, error)
	RollbackFirmware(context.Context, adminservice.RollbackFirmwareRequestObject) (adminservice.RollbackFirmwareResponseObject, error)
	UploadFirmwareBin(context.Context, adminservice.UploadFirmwareBinRequestObject) (adminservice.UploadFirmwareBinResponseObject, error)
}

var _ FirmwareAdminService = (*Server)(nil)

func (s *Server) ListFirmwares(ctx context.Context, request adminservice.ListFirmwaresRequestObject) (adminservice.ListFirmwaresResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.ListFirmwares500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listFirmwarePage(ctx, store, cursor, limit)
	if err != nil {
		return adminservice.ListFirmwares500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.ListFirmwares200JSONResponse(adminservice.FirmwareList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateFirmware(ctx context.Context, request adminservice.CreateFirmwareRequestObject) (adminservice.CreateFirmwareResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.CreateFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminservice.CreateFirmware400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE", "request body required")), nil
	}
	item, err := normalizeFirmwareUpsert(*request.Body, "")
	if err != nil {
		return adminservice.CreateFirmware400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE", err.Error())), nil
	}
	if _, err := Get(ctx, store, item.Name); err == nil {
		return adminservice.CreateFirmware409JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ALREADY_EXISTS", fmt.Sprintf("firmware %q already exists", item.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminservice.CreateFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	item.CreatedAt = now
	item.UpdatedAt = now
	if err := Write(ctx, store, item); err != nil {
		return adminservice.CreateFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.CreateFirmware200JSONResponse(item), nil
}

func (s *Server) DeleteFirmware(ctx context.Context, request adminservice.DeleteFirmwareRequestObject) (adminservice.DeleteFirmwareResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.DeleteFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	item, err := Get(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.DeleteFirmware404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_NOT_FOUND", fmt.Sprintf("firmware %q not found", name))), nil
		}
		return adminservice.DeleteFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, firmwareKey(name)); err != nil {
		return adminservice.DeleteFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := s.deleteAssetPrefix(name); err != nil {
		return adminservice.DeleteFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.DeleteFirmware200JSONResponse(item), nil
}

func (s *Server) GetFirmware(ctx context.Context, request adminservice.GetFirmwareRequestObject) (adminservice.GetFirmwareResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.GetFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	item, err := Get(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.GetFirmware404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_NOT_FOUND", fmt.Sprintf("firmware %q not found", name))), nil
		}
		return adminservice.GetFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetFirmware200JSONResponse(item), nil
}

func (s *Server) PutFirmware(ctx context.Context, request adminservice.PutFirmwareRequestObject) (adminservice.PutFirmwareResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.PutFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminservice.PutFirmware400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE", "request body required")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	item, err := normalizeFirmwareUpsert(*request.Body, name)
	if err != nil {
		return adminservice.PutFirmware400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE", err.Error())), nil
	}
	previous, err := Get(ctx, store, name)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminservice.PutFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	item.CreatedAt = now
	item.UpdatedAt = now
	var removed []string
	if err == nil {
		item.CreatedAt = previous.CreatedAt
		removed = mergeUploadedMetadata(previous.Slots, &item.Slots)
	}
	if err := Write(ctx, store, item); err != nil {
		return adminservice.PutFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := s.deleteAssetPaths(removed); err != nil {
		return adminservice.PutFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.PutFirmware200JSONResponse(item), nil
}

func (s *Server) ReleaseFirmware(ctx context.Context, request adminservice.ReleaseFirmwareRequestObject) (adminservice.ReleaseFirmwareResponseObject, error) {
	item, err := s.updateSlots(ctx, request.Name, releaseSlots)
	if err != nil {
		return releaseError(request.Name, err), nil
	}
	return adminservice.ReleaseFirmware200JSONResponse(item), nil
}

func (s *Server) RollbackFirmware(ctx context.Context, request adminservice.RollbackFirmwareRequestObject) (adminservice.RollbackFirmwareResponseObject, error) {
	item, err := s.updateSlots(ctx, request.Name, rollbackSlots)
	if err != nil {
		return rollbackError(request.Name, err), nil
	}
	return adminservice.RollbackFirmware200JSONResponse(item), nil
}

func (s *Server) UploadFirmwareBin(ctx context.Context, request adminservice.UploadFirmwareBinRequestObject) (adminservice.UploadFirmwareBinResponseObject, error) {
	item, err := s.uploadFirmwareBin(ctx, request)
	if err != nil {
		switch {
		case errors.Is(err, kv.ErrNotFound), errors.Is(err, errChannelNotFound), errors.Is(err, errBinNotFound):
			return adminservice.UploadFirmwareBin404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_BIN_NOT_FOUND", err.Error())), nil
		case errors.Is(err, errAssetsNotConfigured):
			return adminservice.UploadFirmwareBin500JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ASSETS_NOT_CONFIGURED", err.Error())), nil
		case errors.Is(err, errInvalidChannel), errors.Is(err, errInvalidBin):
			return adminservice.UploadFirmwareBin400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE_BIN", err.Error())), nil
		default:
			return adminservice.UploadFirmwareBin500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
		}
	}
	return adminservice.UploadFirmwareBin200JSONResponse(item), nil
}

var (
	errAssetsNotConfigured = errors.New("firmware asset store not configured")
	errInvalidChannel      = errors.New("invalid firmware channel")
	errInvalidBin          = errors.New("invalid firmware bin")
	errChannelNotFound     = errors.New("firmware channel not found")
	errBinNotFound         = errors.New("firmware bin not found")
)

func (s *Server) uploadFirmwareBin(ctx context.Context, request adminservice.UploadFirmwareBinRequestObject) (apitypes.Firmware, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	assets, err := s.assets()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return apitypes.Firmware{}, fmt.Errorf("%w: %v", errInvalidBin, err)
	}
	bin, err := url.PathUnescape(string(request.Bin))
	if err != nil {
		return apitypes.Firmware{}, fmt.Errorf("%w: %v", errInvalidBin, err)
	}
	name = strings.TrimSpace(name)
	bin = strings.TrimSpace(bin)
	channel := strings.TrimSpace(string(request.Channel))
	if name == "" || bin == "" {
		return apitypes.Firmware{}, errInvalidBin
	}
	item, err := Get(ctx, store, name)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	slot, ok := slotForChannel(&item.Slots, channel)
	if !ok {
		return apitypes.Firmware{}, fmt.Errorf("%w: %s", errChannelNotFound, channel)
	}
	artifact, ok := artifactForBin(slot, bin)
	if !ok {
		return apitypes.Firmware{}, fmt.Errorf("%w: %s/%s", errBinNotFound, channel, bin)
	}

	now := s.now()
	path, err := firmwareBinObjectPath(name, channel, bin, now)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	hash := sha256.New()
	var sizeCounter byteCounter
	reader := io.TeeReader(request.Body, io.MultiWriter(hash, &sizeCounter))
	if err := assets.Put(path, reader); err != nil {
		return apitypes.Firmware{}, err
	}

	oldPath := artifact.Path
	contentType := "application/octet-stream"
	size := sizeCounter.N
	sha256Hex := hex.EncodeToString(hash.Sum(nil))
	artifact.Path = &path
	artifact.Size = &size
	artifact.Sha256 = &sha256Hex
	artifact.ContentType = &contentType
	artifact.UploadedAt = &now
	item.UpdatedAt = now
	if err := Write(ctx, store, item); err != nil {
		_ = assets.Delete(path)
		return apitypes.Firmware{}, err
	}
	if oldPath != nil && *oldPath != "" && *oldPath != path {
		if err := assets.Delete(*oldPath); err != nil {
			return apitypes.Firmware{}, err
		}
	}
	return item, nil
}

func (s *Server) updateSlots(ctx context.Context, rawName string, mutate func(apitypes.FirmwareSlots) apitypes.FirmwareSlots) (apitypes.Firmware, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	name, err := url.PathUnescape(string(rawName))
	if err != nil {
		return apitypes.Firmware{}, fmt.Errorf("invalid params: %w", err)
	}
	item, err := Get(ctx, store, name)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	item.Slots = mutate(item.Slots)
	if !slotHasPayload(item.Slots.Stable) {
		return apitypes.Firmware{}, errStableEmpty
	}
	item.UpdatedAt = s.now()
	if err := Write(ctx, store, item); err != nil {
		return apitypes.Firmware{}, err
	}
	return item, nil
}

func releaseSlots(slots apitypes.FirmwareSlots) apitypes.FirmwareSlots {
	return apitypes.FirmwareSlots{
		Develop: slots.Beta,
		Beta:    slots.Stable,
		Stable:  slots.Pending,
		Pending: apitypes.FirmwareSlot{},
	}
}

func rollbackSlots(slots apitypes.FirmwareSlots) apitypes.FirmwareSlots {
	return apitypes.FirmwareSlots{
		Develop: apitypes.FirmwareSlot{},
		Beta:    slots.Develop,
		Stable:  slots.Beta,
		Pending: slots.Stable,
	}
}

var errStableEmpty = errors.New("stable slot must not be empty after operation")

func releaseError(name string, err error) adminservice.ReleaseFirmwareResponseObject {
	if errors.Is(err, kv.ErrNotFound) {
		return adminservice.ReleaseFirmware404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_NOT_FOUND", fmt.Sprintf("firmware %q not found", name)))
	}
	if errors.Is(err, errStableEmpty) {
		return adminservice.ReleaseFirmware409JSONResponse(apitypes.NewErrorResponse("FIRMWARE_STABLE_EMPTY", err.Error()))
	}
	return adminservice.ReleaseFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))
}

func rollbackError(name string, err error) adminservice.RollbackFirmwareResponseObject {
	if errors.Is(err, kv.ErrNotFound) {
		return adminservice.RollbackFirmware404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_NOT_FOUND", fmt.Sprintf("firmware %q not found", name)))
	}
	if errors.Is(err, errStableEmpty) {
		return adminservice.RollbackFirmware409JSONResponse(apitypes.NewErrorResponse("FIRMWARE_STABLE_EMPTY", err.Error()))
	}
	return adminservice.RollbackFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))
}

func Get(ctx context.Context, store kv.Store, name string) (apitypes.Firmware, error) {
	data, err := store.Get(ctx, firmwareKey(name))
	if err != nil {
		return apitypes.Firmware{}, err
	}
	var item apitypes.Firmware
	if err := json.Unmarshal(data, &item); err != nil {
		return apitypes.Firmware{}, err
	}
	return item, nil
}

func Write(ctx context.Context, store kv.Store, item apitypes.Firmware) error {
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("firmware: encode %s: %w", item.Name, err)
	}
	if err := store.Set(ctx, firmwareKey(item.Name), data); err != nil {
		return fmt.Errorf("firmware: write %s: %w", item.Name, err)
	}
	return nil
}

func listFirmwarePage(ctx context.Context, store kv.Store, cursor string, limit int) ([]apitypes.Firmware, bool, *string, error) {
	entries, err := kv.ListAfter(ctx, store, firmwaresRoot, cursorAfterKey(firmwaresRoot, cursor), limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	pageEntries, hasNext, nextCursor := paginateEntries(entries, limit)
	items := make([]apitypes.Firmware, 0, len(pageEntries))
	for _, entry := range pageEntries {
		var item apitypes.Firmware
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return nil, false, nil, fmt.Errorf("firmware: decode list %s: %w", entry.Key.String(), err)
		}
		items = append(items, item)
	}
	return items, hasNext, nextCursor, nil
}

func normalizeFirmwareUpsert(in adminservice.FirmwareUpsert, expectedName string) (apitypes.Firmware, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return apitypes.Firmware{}, errors.New("name is required")
	}
	if expectedName != "" && name != expectedName {
		return apitypes.Firmware{}, fmt.Errorf("name %q must match path name %q", name, expectedName)
	}
	slots, err := normalizeSlots(in.Slots)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	item := apitypes.Firmware{
		Name:  name,
		Slots: slots,
	}
	if in.Description != nil {
		description := strings.TrimSpace(*in.Description)
		if description != "" {
			item.Description = &description
		}
	}
	return item, nil
}

func normalizeSlots(in apitypes.FirmwareSlots) (apitypes.FirmwareSlots, error) {
	var err error
	out := apitypes.FirmwareSlots{}
	if out.Stable, err = normalizeSlot(in.Stable); err != nil {
		return out, fmt.Errorf("stable: %w", err)
	}
	if out.Beta, err = normalizeSlot(in.Beta); err != nil {
		return out, fmt.Errorf("beta: %w", err)
	}
	if out.Develop, err = normalizeSlot(in.Develop); err != nil {
		return out, fmt.Errorf("develop: %w", err)
	}
	if out.Pending, err = normalizeSlot(in.Pending); err != nil {
		return out, fmt.Errorf("pending: %w", err)
	}
	return out, nil
}

func normalizeSlot(in apitypes.FirmwareSlot) (apitypes.FirmwareSlot, error) {
	out := apitypes.FirmwareSlot{}
	if in.Version != nil {
		version := strings.TrimSpace(*in.Version)
		if version != "" {
			out.Version = &version
		}
	}
	if in.Description != nil {
		description := strings.TrimSpace(*in.Description)
		if description != "" {
			out.Description = &description
		}
	}
	if in.Artifacts != nil {
		artifacts := make([]apitypes.FirmwareArtifact, 0, len(*in.Artifacts))
		for i, artifact := range *in.Artifacts {
			next, err := normalizeArtifact(artifact)
			if err != nil {
				return out, fmt.Errorf("artifact[%d]: %w", i, err)
			}
			artifacts = append(artifacts, next)
		}
		if len(artifacts) > 0 {
			out.Artifacts = &artifacts
		}
	}
	return out, nil
}

func normalizeArtifact(in apitypes.FirmwareArtifact) (apitypes.FirmwareArtifact, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return apitypes.FirmwareArtifact{}, errors.New("name is required")
	}
	kind := apitypes.FirmwareArtifactKind(strings.TrimSpace(string(in.Kind)))
	if kind == "" {
		return apitypes.FirmwareArtifact{}, errors.New("kind is required")
	}
	if !kind.Valid() {
		return apitypes.FirmwareArtifact{}, fmt.Errorf("unsupported kind %q", kind)
	}
	return apitypes.FirmwareArtifact{Name: name, Kind: kind}, nil
}

func slotHasPayload(slot apitypes.FirmwareSlot) bool {
	if slot.Version != nil && strings.TrimSpace(*slot.Version) != "" {
		return true
	}
	if slot.Artifacts != nil && len(*slot.Artifacts) > 0 {
		return true
	}
	return false
}

func firmwareKey(name string) kv.Key {
	return append(append(kv.Key{}, firmwaresRoot...), escapeStoreSegment(name))
}

func escapeStoreSegment(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	return strings.ReplaceAll(value, ":", "%3A")
}

func normalizeListParams(cursor *string, limit *int32) (string, int) {
	nextCursor := ""
	if cursor != nil {
		nextCursor = *cursor
	}
	nextLimit := defaultListLimit
	if limit != nil {
		nextLimit = int(*limit)
	}
	if nextLimit <= 0 {
		nextLimit = defaultListLimit
	}
	if nextLimit > maxListLimit {
		nextLimit = maxListLimit
	}
	return nextCursor, nextLimit
}

func cursorAfterKey(prefix kv.Key, cursor string) kv.Key {
	if cursor == "" {
		return nil
	}
	after := append(kv.Key{}, prefix...)
	return append(after, cursor)
}

func paginateEntries(entries []kv.Entry, limit int) ([]kv.Entry, bool, *string) {
	if len(entries) == 0 {
		return nil, false, nil
	}
	hasNext := len(entries) > limit
	if !hasNext {
		return entries, false, nil
	}
	page := entries[:limit]
	if len(page) == 0 || len(page[len(page)-1].Key) == 0 {
		return page, true, nil
	}
	nextCursor := page[len(page)-1].Key[len(page[len(page)-1].Key)-1]
	return page, true, &nextCursor
}

func (s *Server) store() (kv.Store, error) {
	if s == nil || s.Store == nil {
		return nil, errors.New("firmware store not configured")
	}
	return s.Store, nil
}

func (s *Server) assets() (objectstore.ObjectStore, error) {
	if s == nil || s.Assets == nil {
		return nil, errAssetsNotConfigured
	}
	return s.Assets, nil
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type byteCounter struct {
	N int64
}

func (c *byteCounter) Write(p []byte) (int, error) {
	c.N += int64(len(p))
	return len(p), nil
}

func slotForChannel(slots *apitypes.FirmwareSlots, channel string) (*apitypes.FirmwareSlot, bool) {
	switch channel {
	case "stable":
		return &slots.Stable, true
	case "beta":
		return &slots.Beta, true
	case "develop":
		return &slots.Develop, true
	case "pending":
		return &slots.Pending, true
	default:
		return nil, false
	}
}

func artifactForBin(slot *apitypes.FirmwareSlot, bin string) (*apitypes.FirmwareArtifact, bool) {
	if slot == nil || slot.Artifacts == nil {
		return nil, false
	}
	for i := range *slot.Artifacts {
		if (*slot.Artifacts)[i].Name == bin {
			return &(*slot.Artifacts)[i], true
		}
	}
	return nil, false
}

func mergeUploadedMetadata(previous apitypes.FirmwareSlots, next *apitypes.FirmwareSlots) []string {
	removed := artifactPaths(previous)
	mergeSlotUploadedMetadata(previous.Stable, &next.Stable, removed)
	mergeSlotUploadedMetadata(previous.Beta, &next.Beta, removed)
	mergeSlotUploadedMetadata(previous.Develop, &next.Develop, removed)
	mergeSlotUploadedMetadata(previous.Pending, &next.Pending, removed)
	out := make([]string, 0, len(removed))
	for path := range removed {
		out = append(out, path)
	}
	return out
}

func mergeSlotUploadedMetadata(previous apitypes.FirmwareSlot, next *apitypes.FirmwareSlot, removed map[string]struct{}) {
	if previous.Artifacts == nil || next.Artifacts == nil {
		return
	}
	previousByKey := make(map[string]apitypes.FirmwareArtifact, len(*previous.Artifacts))
	for _, artifact := range *previous.Artifacts {
		previousByKey[artifactIdentity(artifact)] = artifact
	}
	for i := range *next.Artifacts {
		current := &(*next.Artifacts)[i]
		previous, ok := previousByKey[artifactIdentity(*current)]
		if !ok {
			continue
		}
		current.Path = previous.Path
		current.Size = previous.Size
		current.Sha256 = previous.Sha256
		current.ContentType = previous.ContentType
		current.UploadedAt = previous.UploadedAt
		if previous.Path != nil {
			delete(removed, *previous.Path)
		}
	}
}

func artifactIdentity(artifact apitypes.FirmwareArtifact) string {
	return artifact.Name + "\x00" + string(artifact.Kind)
}

func artifactPaths(slots apitypes.FirmwareSlots) map[string]struct{} {
	out := make(map[string]struct{})
	collectSlotArtifactPaths(slots.Stable, out)
	collectSlotArtifactPaths(slots.Beta, out)
	collectSlotArtifactPaths(slots.Develop, out)
	collectSlotArtifactPaths(slots.Pending, out)
	return out
}

func collectSlotArtifactPaths(slot apitypes.FirmwareSlot, out map[string]struct{}) {
	if slot.Artifacts == nil {
		return
	}
	for _, artifact := range *slot.Artifacts {
		if artifact.Path != nil && *artifact.Path != "" {
			out[*artifact.Path] = struct{}{}
		}
	}
}

func (s *Server) deleteAssetPaths(paths []string) error {
	if len(paths) == 0 || s == nil || s.Assets == nil {
		return nil
	}
	for _, path := range paths {
		if err := s.Assets.Delete(path); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) deleteAssetPrefix(name string) error {
	if s == nil || s.Assets == nil {
		return nil
	}
	return s.Assets.DeletePrefix(firmwareAssetPrefix(name))
}

func firmwareBinObjectPath(name, channel, bin string, uploadedAt time.Time) (string, error) {
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("firmware: generate bin path nonce: %w", err)
	}
	fileName := uploadedAt.UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(nonce[:]) + ".bin"
	return strings.Join([]string{objectPathSegment(name), objectPathSegment(channel), objectPathSegment(bin), fileName}, "/"), nil
}

func firmwareAssetPrefix(name string) string {
	return objectPathSegment(name)
}

func objectPathSegment(value string) string {
	value = strings.TrimSpace(value)
	replacer := strings.NewReplacer("%", "%25", "/", "%2F", "\\", "%5C", ":", "%3A")
	return replacer.Replace(value)
}
