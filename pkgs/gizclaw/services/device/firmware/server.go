package firmware

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var firmwaresRoot = kv.Key{"by-id"}

const (
	defaultListLimit                = 50
	maxListLimit                    = 200
	maxFirmwareSlotDescriptionBytes = 1024
	maxFirmwarePackageURLBytes      = 2048
	maxFirmwarePackageSize          = int64(1<<53 - 1)
)

type Server struct {
	Store kv.Store
	Now   func() time.Time
}

type FirmwareAdminService interface {
	ListFirmwares(context.Context, adminhttp.ListFirmwaresRequestObject) (adminhttp.ListFirmwaresResponseObject, error)
	CreateFirmware(context.Context, adminhttp.CreateFirmwareRequestObject) (adminhttp.CreateFirmwareResponseObject, error)
	DeleteFirmware(context.Context, adminhttp.DeleteFirmwareRequestObject) (adminhttp.DeleteFirmwareResponseObject, error)
	GetFirmware(context.Context, adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error)
	PutFirmware(context.Context, adminhttp.PutFirmwareRequestObject) (adminhttp.PutFirmwareResponseObject, error)
	ReleaseFirmware(context.Context, adminhttp.ReleaseFirmwareRequestObject) (adminhttp.ReleaseFirmwareResponseObject, error)
	RollbackFirmware(context.Context, adminhttp.RollbackFirmwareRequestObject) (adminhttp.RollbackFirmwareResponseObject, error)
}

var _ FirmwareAdminService = (*Server)(nil)

func (s *Server) ListFirmwares(ctx context.Context, request adminhttp.ListFirmwaresRequestObject) (adminhttp.ListFirmwaresResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.ListFirmwares500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listFirmwarePage(ctx, store, cursor, limit)
	if err != nil {
		return adminhttp.ListFirmwares500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListFirmwares200JSONResponse(adminhttp.FirmwareList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateFirmware(ctx context.Context, request adminhttp.CreateFirmwareRequestObject) (adminhttp.CreateFirmwareResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.CreateFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.CreateFirmware400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE", "request body required")), nil
	}
	item, err := normalizeFirmwareUpsert(*request.Body, "")
	if err != nil {
		return adminhttp.CreateFirmware400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE", err.Error())), nil
	}
	now := s.now()
	item.CreatedAt = now
	item.UpdatedAt = now
	data, err := json.Marshal(item)
	if err != nil {
		return adminhttp.CreateFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	_, created, err := kv.CreateIfAbsent(ctx, store, kv.Entry{Key: firmwareKey(item.Id), Value: data}, nil)
	if err != nil {
		return adminhttp.CreateFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if !created {
		return adminhttp.CreateFirmware409JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ALREADY_EXISTS", fmt.Sprintf("firmware %q already exists", item.Id))), nil
	}
	return adminhttp.CreateFirmware200JSONResponse(item), nil
}

func (s *Server) DeleteFirmware(ctx context.Context, request adminhttp.DeleteFirmwareRequestObject) (adminhttp.DeleteFirmwareResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.DeleteFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	item, err := Get(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteFirmware404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_NOT_FOUND", fmt.Sprintf("firmware %q not found", id))), nil
		}
		return adminhttp.DeleteFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, firmwareKey(id)); err != nil {
		return adminhttp.DeleteFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteFirmware200JSONResponse(item), nil
}

func (s *Server) GetFirmware(ctx context.Context, request adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.GetFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	item, err := Get(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetFirmware404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_NOT_FOUND", fmt.Sprintf("firmware %q not found", id))), nil
		}
		return adminhttp.GetFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetFirmware200JSONResponse(item), nil
}

func (s *Server) PutFirmware(ctx context.Context, request adminhttp.PutFirmwareRequestObject) (adminhttp.PutFirmwareResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.PutFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutFirmware400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE", "request body required")), nil
	}
	id := string(request.Id)
	item, err := normalizeFirmwareUpsert(*request.Body, id)
	if err != nil {
		return adminhttp.PutFirmware400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE", err.Error())), nil
	}
	previous, err := Get(ctx, store, id)
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutFirmware404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_NOT_FOUND", fmt.Sprintf("firmware %q not found", id))), nil
	}
	if err != nil {
		return adminhttp.PutFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	item.UpdatedAt = now
	item.CreatedAt = previous.CreatedAt
	if err := Write(ctx, store, item); err != nil {
		return adminhttp.PutFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutFirmware200JSONResponse(item), nil
}

func (s *Server) ReleaseFirmware(ctx context.Context, request adminhttp.ReleaseFirmwareRequestObject) (adminhttp.ReleaseFirmwareResponseObject, error) {
	item, err := s.updateSlots(ctx, request.Id, releaseSlots)
	if err != nil {
		return releaseError(request.Id, err), nil
	}
	return adminhttp.ReleaseFirmware200JSONResponse(item), nil
}

func (s *Server) RollbackFirmware(ctx context.Context, request adminhttp.RollbackFirmwareRequestObject) (adminhttp.RollbackFirmwareResponseObject, error) {
	item, err := s.updateSlots(ctx, request.Id, rollbackSlots)
	if err != nil {
		return rollbackError(request.Id, err), nil
	}
	return adminhttp.RollbackFirmware200JSONResponse(item), nil
}

var errInvalidChannel = errors.New("invalid firmware channel")

func (s *Server) updateSlots(ctx context.Context, rawID string, mutate func(apitypes.FirmwareSlots) apitypes.FirmwareSlots) (apitypes.Firmware, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Firmware{}, err
	}
	id := string(rawID)
	item, err := Get(ctx, store, id)
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

func releaseError(id string, err error) adminhttp.ReleaseFirmwareResponseObject {
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.ReleaseFirmware404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_NOT_FOUND", fmt.Sprintf("firmware %q not found", id)))
	}
	if errors.Is(err, errStableEmpty) {
		return adminhttp.ReleaseFirmware409JSONResponse(apitypes.NewErrorResponse("FIRMWARE_STABLE_EMPTY", err.Error()))
	}
	return adminhttp.ReleaseFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))
}

func rollbackError(id string, err error) adminhttp.RollbackFirmwareResponseObject {
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.RollbackFirmware404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_NOT_FOUND", fmt.Sprintf("firmware %q not found", id)))
	}
	if errors.Is(err, errStableEmpty) {
		return adminhttp.RollbackFirmware409JSONResponse(apitypes.NewErrorResponse("FIRMWARE_STABLE_EMPTY", err.Error()))
	}
	return adminhttp.RollbackFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))
}

func Get(ctx context.Context, store kv.Store, id string) (apitypes.Firmware, error) {
	data, err := store.Get(ctx, firmwareKey(id))
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
		return fmt.Errorf("firmware: encode %s: %w", item.Id, err)
	}
	if err := store.Set(ctx, firmwareKey(item.Id), data); err != nil {
		return fmt.Errorf("firmware: write %s: %w", item.Id, err)
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

func normalizeFirmwareUpsert(in adminhttp.FirmwareUpsert, expectedID string) (apitypes.Firmware, error) {
	id := in.Id
	if err := customid.ValidateResourceID(id); err != nil {
		return apitypes.Firmware{}, err
	}
	if expectedID != "" && id != expectedID {
		return apitypes.Firmware{}, fmt.Errorf("id %q must match path id %q", id, expectedID)
	}
	slots, err := normalizeSlots(in.Slots)
	if err != nil {
		return apitypes.Firmware{}, err
	}
	item := apitypes.Firmware{
		Id:    id,
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
	if in.Description != nil {
		description := strings.TrimSpace(*in.Description)
		if description != "" {
			if len(description) > maxFirmwareSlotDescriptionBytes {
				return out, fmt.Errorf("description must contain at most %d bytes", maxFirmwareSlotDescriptionBytes)
			}
			out.Description = &description
		}
	}
	if in.Package != nil {
		firmwarePackage, err := normalizePackage(*in.Package)
		if err != nil {
			return out, err
		}
		out.Package = &firmwarePackage
	}
	return out, nil
}

func normalizePackage(in apitypes.FirmwarePackage) (apitypes.FirmwarePackage, error) {
	rawURL := strings.TrimSpace(in.Url)
	if len(rawURL) > maxFirmwarePackageURLBytes {
		return apitypes.FirmwarePackage{}, fmt.Errorf("package url must contain at most %d bytes", maxFirmwarePackageURLBytes)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return apitypes.FirmwarePackage{}, errors.New("package url must be an absolute HTTPS URL without userinfo or fragment")
	}
	if parsed.Hostname() == "" || strings.HasSuffix(parsed.Host, ":") {
		return apitypes.FirmwarePackage{}, errors.New("package url must contain a valid HTTPS authority")
	}
	if port := parsed.Port(); port != "" {
		value, portErr := strconv.ParseUint(port, 10, 16)
		if portErr != nil || value == 0 {
			return apitypes.FirmwarePackage{}, errors.New("package url must contain a valid HTTPS authority port")
		}
	}
	sha256Value := strings.ToLower(strings.TrimSpace(in.Sha256))
	decoded, err := hex.DecodeString(sha256Value)
	if err != nil || len(decoded) != 32 {
		return apitypes.FirmwarePackage{}, errors.New("package sha256 must contain 64 hexadecimal characters")
	}
	if in.Size <= 0 || in.Size > maxFirmwarePackageSize {
		return apitypes.FirmwarePackage{}, fmt.Errorf("package size must be between 1 and %d", maxFirmwarePackageSize)
	}
	return apitypes.FirmwarePackage{Url: rawURL, Sha256: sha256Value, Size: in.Size}, nil
}

func slotHasPayload(slot apitypes.FirmwareSlot) bool {
	if slot.Description != nil && strings.TrimSpace(*slot.Description) != "" {
		return true
	}
	if slot.Package != nil {
		return true
	}
	return false
}

func firmwareKey(id string) kv.Key {
	return append(append(kv.Key{}, firmwaresRoot...), escapeStoreSegment(id))
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

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
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
