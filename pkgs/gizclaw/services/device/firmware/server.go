package firmware

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/jmoiron/sqlx"
)

const (
	defaultListLimit                = 50
	maxListLimit                    = 200
	maxFirmwareSlotDescriptionBytes = 1024
	maxFirmwarePackageURLBytes      = 2048
	maxFirmwarePackageVersionBytes  = 128
	maxFirmwarePackageSize          = int64(1<<53 - 1)
)

// Keep this syntax aligned with FirmwarePackage.version in the source OpenAPI.
var firmwareVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

type Server struct {
	DB  *sqlx.DB
	Now func() time.Time
}

type FirmwareAdminService interface {
	ListFirmwares(context.Context, adminhttp.ListFirmwaresRequestObject) (adminhttp.ListFirmwaresResponseObject, error)
	CreateFirmware(context.Context, adminhttp.CreateFirmwareRequestObject) (adminhttp.CreateFirmwareResponseObject, error)
	DeleteFirmware(context.Context, adminhttp.DeleteFirmwareRequestObject) (adminhttp.DeleteFirmwareResponseObject, error)
	GetFirmware(context.Context, adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error)
	PutFirmware(context.Context, adminhttp.PutFirmwareRequestObject) (adminhttp.PutFirmwareResponseObject, error)
}

var _ FirmwareAdminService = (*Server)(nil)

func (s *Server) ListFirmwares(ctx context.Context, request adminhttp.ListFirmwaresRequestObject) (adminhttp.ListFirmwaresResponseObject, error) {
	store, err := s.database()
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
	store, err := s.database()
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
	data, err := json.Marshal(item.Slots)
	if err != nil {
		return adminhttp.CreateFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	result, err := store.ExecContext(ctx, store.Rebind(`INSERT INTO firmwares(id,description,slots_json,created_at,updated_at) VALUES (?,?,?,?,?) ON CONFLICT(id) DO NOTHING`), item.Id, item.Description, string(data), item.CreatedAt.Format(time.RFC3339Nano), item.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return adminhttp.CreateFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return adminhttp.CreateFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	created := affected == 1
	if !created {
		return adminhttp.CreateFirmware409JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ALREADY_EXISTS", fmt.Sprintf("firmware %q already exists", item.Id))), nil
	}
	return adminhttp.CreateFirmware200JSONResponse(item), nil
}

func (s *Server) DeleteFirmware(ctx context.Context, request adminhttp.DeleteFirmwareRequestObject) (adminhttp.DeleteFirmwareResponseObject, error) {
	store, err := s.database()
	if err != nil {
		return adminhttp.DeleteFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	// Validate the returned record before committing the deletion. Invalid
	// stored packages must not turn an error response into a successful delete.
	tx, err := store.BeginTxx(ctx, nil)
	if err != nil {
		return adminhttp.DeleteFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	defer tx.Rollback()
	item, err := scanFirmware(tx.QueryRowContext(ctx, store.Rebind(`DELETE FROM firmwares WHERE id=? RETURNING `+firmwareColumns), id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.DeleteFirmware404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_NOT_FOUND", fmt.Sprintf("firmware %q not found", id))), nil
		}
		return adminhttp.DeleteFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := tx.Commit(); err != nil {
		return adminhttp.DeleteFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteFirmware200JSONResponse(item), nil
}

func (s *Server) GetFirmware(ctx context.Context, request adminhttp.GetFirmwareRequestObject) (adminhttp.GetFirmwareResponseObject, error) {
	store, err := s.database()
	if err != nil {
		return adminhttp.GetFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	item, err := Get(ctx, store, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.GetFirmware404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_NOT_FOUND", fmt.Sprintf("firmware %q not found", id))), nil
		}
		return adminhttp.GetFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetFirmware200JSONResponse(item), nil
}

func (s *Server) PutFirmware(ctx context.Context, request adminhttp.PutFirmwareRequestObject) (adminhttp.PutFirmwareResponseObject, error) {
	store, err := s.database()
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
	data, err := json.Marshal(item.Slots)
	if err != nil {
		return adminhttp.PutFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	item, err = scanFirmware(store.QueryRowContext(ctx, store.Rebind(`UPDATE firmwares SET description=?,slots_json=?,updated_at=? WHERE id=? RETURNING `+firmwareColumns), item.Description, string(data), s.now().Format(time.RFC3339Nano), id))
	if errors.Is(err, sql.ErrNoRows) {
		return adminhttp.PutFirmware404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_NOT_FOUND", fmt.Sprintf("firmware %q not found", id))), nil
	}
	if err != nil {
		return adminhttp.PutFirmware500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutFirmware200JSONResponse(item), nil
}

const firmwareColumns = "id,description,slots_json,created_at,updated_at"

// Get reads a Firmware by its catalog ID.
func Get(ctx context.Context, db *sqlx.DB, id string) (apitypes.Firmware, error) {
	return scanFirmware(db.QueryRowContext(ctx, db.Rebind(`SELECT `+firmwareColumns+` FROM firmwares WHERE id=?`), id))
}

func scanFirmware(row interface{ Scan(...any) error }) (apitypes.Firmware, error) {
	var item apitypes.Firmware
	var slots, created, updated string
	if err := row.Scan(&item.Id, &item.Description, &slots, &created, &updated); err != nil {
		return item, err
	}
	if err := json.Unmarshal([]byte(slots), &item.Slots); err != nil {
		return item, err
	}
	for _, slot := range []apitypes.FirmwareSlot{item.Slots.Stable, item.Slots.Beta, item.Slots.Develop} {
		if slot.Package != nil {
			if err := validatePackageVersion(slot.Package.Version); err != nil {
				return apitypes.Firmware{}, fmt.Errorf("stored firmware package requires a valid version; replace the configuration using Admin PUT: %w", err)
			}
		}
	}
	var err error
	if item.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return item, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return item, err
}

func listFirmwarePage(ctx context.Context, db *sqlx.DB, cursor string, limit int) ([]apitypes.Firmware, bool, *string, error) {
	rows, err := db.QueryContext(ctx, db.Rebind(`SELECT `+firmwareColumns+` FROM firmwares WHERE id>? ORDER BY id LIMIT ?`), cursor, limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	items := make([]apitypes.Firmware, 0, limit+1)
	for rows.Next() {
		item, err := scanFirmware(rows)
		if err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, nil, err
	}
	if len(items) > limit {
		return items[:limit], true, &items[limit-1].Id, nil
	}
	return items, false, nil, nil
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

func validatePackageVersion(version string) error {
	if len(version) > maxFirmwarePackageVersionBytes || !firmwareVersionPattern.MatchString(version) {
		return errors.New("package version must be SemVer 2.0.0 without a leading v and contain at most 128 ASCII bytes")
	}
	return nil
}

func normalizePackage(in apitypes.FirmwarePackage) (apitypes.FirmwarePackage, error) {
	if err := validatePackageVersion(in.Version); err != nil {
		return apitypes.FirmwarePackage{}, err
	}
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
	return apitypes.FirmwarePackage{Url: rawURL, Sha256: sha256Value, Size: in.Size, Version: in.Version}, nil
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

// Initialize creates the Firmware catalog schema once during Server startup.
func (s *Server) Initialize(ctx context.Context) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS firmwares (
 id TEXT PRIMARY KEY CHECK(length(id)>0),
 description TEXT,
 slots_json TEXT NOT NULL,
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL
 )`)
	return err
}

func (s *Server) database() (*sqlx.DB, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("firmware database not configured")
	}
	switch s.DB.DriverName() {
	case "sqlite", "postgres":
	default:
		return nil, fmt.Errorf("firmware: unsupported SQL driver %q", s.DB.DriverName())
	}
	return s.DB, nil
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
