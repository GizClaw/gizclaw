package gameplay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"reflect"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

var (
	gameRulesetsRoot = kv.Key{"by-name"}
	petDefsRoot      = kv.Key{"by-id"}
	badgeDefsRoot    = kv.Key{"by-id"}
	gameDefsRoot     = kv.Key{"by-id"}
)

type Catalog struct {
	GameRulesets kv.Store
	PetDefs      kv.Store
	BadgeDefs    kv.Store
	GameDefs     kv.Store
	Assets       objectstore.ObjectStore
	Now          func() time.Time
}

type CatalogAdminService interface {
	ListGameRulesets(context.Context, adminhttp.ListGameRulesetsRequestObject) (adminhttp.ListGameRulesetsResponseObject, error)
	CreateGameRuleset(context.Context, adminhttp.CreateGameRulesetRequestObject) (adminhttp.CreateGameRulesetResponseObject, error)
	DeleteGameRuleset(context.Context, adminhttp.DeleteGameRulesetRequestObject) (adminhttp.DeleteGameRulesetResponseObject, error)
	GetGameRuleset(context.Context, adminhttp.GetGameRulesetRequestObject) (adminhttp.GetGameRulesetResponseObject, error)
	PutGameRuleset(context.Context, adminhttp.PutGameRulesetRequestObject) (adminhttp.PutGameRulesetResponseObject, error)
	ListPetDefs(context.Context, adminhttp.ListPetDefsRequestObject) (adminhttp.ListPetDefsResponseObject, error)
	CreatePetDef(context.Context, adminhttp.CreatePetDefRequestObject) (adminhttp.CreatePetDefResponseObject, error)
	DeletePetDef(context.Context, adminhttp.DeletePetDefRequestObject) (adminhttp.DeletePetDefResponseObject, error)
	GetPetDef(context.Context, adminhttp.GetPetDefRequestObject) (adminhttp.GetPetDefResponseObject, error)
	PutPetDef(context.Context, adminhttp.PutPetDefRequestObject) (adminhttp.PutPetDefResponseObject, error)
	DownloadPetDefPixa(context.Context, adminhttp.DownloadPetDefPixaRequestObject) (adminhttp.DownloadPetDefPixaResponseObject, error)
	UploadPetDefPixa(context.Context, adminhttp.UploadPetDefPixaRequestObject) (adminhttp.UploadPetDefPixaResponseObject, error)
	ListBadgeDefs(context.Context, adminhttp.ListBadgeDefsRequestObject) (adminhttp.ListBadgeDefsResponseObject, error)
	CreateBadgeDef(context.Context, adminhttp.CreateBadgeDefRequestObject) (adminhttp.CreateBadgeDefResponseObject, error)
	DeleteBadgeDef(context.Context, adminhttp.DeleteBadgeDefRequestObject) (adminhttp.DeleteBadgeDefResponseObject, error)
	GetBadgeDef(context.Context, adminhttp.GetBadgeDefRequestObject) (adminhttp.GetBadgeDefResponseObject, error)
	PutBadgeDef(context.Context, adminhttp.PutBadgeDefRequestObject) (adminhttp.PutBadgeDefResponseObject, error)
	DownloadBadgeDefPixa(context.Context, adminhttp.DownloadBadgeDefPixaRequestObject) (adminhttp.DownloadBadgeDefPixaResponseObject, error)
	UploadBadgeDefPixa(context.Context, adminhttp.UploadBadgeDefPixaRequestObject) (adminhttp.UploadBadgeDefPixaResponseObject, error)
	ListGameDefs(context.Context, adminhttp.ListGameDefsRequestObject) (adminhttp.ListGameDefsResponseObject, error)
	CreateGameDef(context.Context, adminhttp.CreateGameDefRequestObject) (adminhttp.CreateGameDefResponseObject, error)
	DeleteGameDef(context.Context, adminhttp.DeleteGameDefRequestObject) (adminhttp.DeleteGameDefResponseObject, error)
	GetGameDef(context.Context, adminhttp.GetGameDefRequestObject) (adminhttp.GetGameDefResponseObject, error)
	PutGameDef(context.Context, adminhttp.PutGameDefRequestObject) (adminhttp.PutGameDefResponseObject, error)
}

var _ CatalogAdminService = (*Catalog)(nil)

func (c *Catalog) ListGameRulesets(ctx context.Context, request adminhttp.ListGameRulesetsRequestObject) (adminhttp.ListGameRulesetsResponseObject, error) {
	store, err := c.store(c.GameRulesets, "game rulesets")
	if err != nil {
		return adminhttp.ListGameRulesets500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listJSON[apitypes.GameRuleset](ctx, store, gameRulesetsRoot, cursor, limit)
	if err != nil {
		return adminhttp.ListGameRulesets500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListGameRulesets200JSONResponse(adminhttp.GameRulesetList{Items: items, HasNext: hasNext, NextCursor: nextCursor}), nil
}

func (c *Catalog) CreateGameRuleset(ctx context.Context, request adminhttp.CreateGameRulesetRequestObject) (adminhttp.CreateGameRulesetResponseObject, error) {
	if request.Body == nil {
		return adminhttp.CreateGameRuleset400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_RULESET", "request body required")), nil
	}
	store, err := c.store(c.GameRulesets, "game rulesets")
	if err != nil {
		return adminhttp.CreateGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name := strings.TrimSpace(request.Body.Name)
	item, err := c.buildGameRuleset(name, request.Body.Spec, time.Time{})
	if err != nil {
		return adminhttp.CreateGameRuleset400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_RULESET", err.Error())), nil
	}
	if _, err := store.Get(ctx, rulesetKey(item.Name)); err == nil {
		return adminhttp.CreateGameRuleset409JSONResponse(apitypes.NewErrorResponse("GAME_RULESET_ALREADY_EXISTS", fmt.Sprintf("game ruleset %q already exists", item.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := writeJSON(ctx, store, rulesetKey(item.Name), item); err != nil {
		return adminhttp.CreateGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.CreateGameRuleset200JSONResponse(item), nil
}

func (c *Catalog) DeleteGameRuleset(ctx context.Context, request adminhttp.DeleteGameRulesetRequestObject) (adminhttp.DeleteGameRulesetResponseObject, error) {
	store, err := c.store(c.GameRulesets, "game rulesets")
	if err != nil {
		return adminhttp.DeleteGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := pathID(request.Name)
	if err != nil {
		return nil, err
	}
	item, err := readJSON[apitypes.GameRuleset](ctx, store, rulesetKey(name))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteGameRuleset404JSONResponse(apitypes.NewErrorResponse("GAME_RULESET_NOT_FOUND", fmt.Sprintf("game ruleset %q not found", name))), nil
		}
		return adminhttp.DeleteGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, rulesetKey(name)); err != nil {
		return adminhttp.DeleteGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteGameRuleset200JSONResponse(item), nil
}

func (c *Catalog) GetGameRuleset(ctx context.Context, request adminhttp.GetGameRulesetRequestObject) (adminhttp.GetGameRulesetResponseObject, error) {
	item, err := c.GetGameRulesetByName(ctx, request.Name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetGameRuleset404JSONResponse(apitypes.NewErrorResponse("GAME_RULESET_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.GetGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetGameRuleset200JSONResponse(item), nil
}

func (c *Catalog) PutGameRuleset(ctx context.Context, request adminhttp.PutGameRulesetRequestObject) (adminhttp.PutGameRulesetResponseObject, error) {
	if request.Body == nil {
		return adminhttp.PutGameRuleset400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_RULESET", "request body required")), nil
	}
	store, err := c.store(c.GameRulesets, "game rulesets")
	if err != nil {
		return adminhttp.PutGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := pathID(request.Name)
	if err != nil {
		return nil, err
	}
	previous, err := readJSON[apitypes.GameRuleset](ctx, store, rulesetKey(name))
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	createdAt := time.Time{}
	if err == nil {
		createdAt = previous.CreatedAt
	}
	item, err := c.buildGameRuleset(name, request.Body.Spec, createdAt)
	if err != nil {
		return adminhttp.PutGameRuleset400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_RULESET", err.Error())), nil
	}
	if err := writeJSON(ctx, store, rulesetKey(item.Name), item); err != nil {
		return adminhttp.PutGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutGameRuleset200JSONResponse(item), nil
}

func (c *Catalog) ListPetDefs(ctx context.Context, request adminhttp.ListPetDefsRequestObject) (adminhttp.ListPetDefsResponseObject, error) {
	store, err := c.store(c.PetDefs, "pet defs")
	if err != nil {
		return adminhttp.ListPetDefs500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := c.listPetDefJSON(ctx, store, cursor, limit)
	if err != nil {
		return adminhttp.ListPetDefs500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListPetDefs200JSONResponse(adminhttp.PetDefList{Items: items, HasNext: hasNext, NextCursor: nextCursor}), nil
}

func (c *Catalog) CreatePetDef(ctx context.Context, request adminhttp.CreatePetDefRequestObject) (adminhttp.CreatePetDefResponseObject, error) {
	if request.Body == nil {
		return adminhttp.CreatePetDef400JSONResponse(apitypes.NewErrorResponse("INVALID_PET_DEF", "request body required")), nil
	}
	store, err := c.store(c.PetDefs, "pet defs")
	if err != nil {
		return adminhttp.CreatePetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := strings.TrimSpace(request.Body.Id)
	item, err := c.buildPetDef(id, request.Body.Spec, request.Body.I18n, nil, time.Time{})
	if err != nil {
		return adminhttp.CreatePetDef400JSONResponse(apitypes.NewErrorResponse("INVALID_PET_DEF", err.Error())), nil
	}
	if _, err := store.Get(ctx, petDefKey(item.Id)); err == nil {
		return adminhttp.CreatePetDef409JSONResponse(apitypes.NewErrorResponse("PET_DEF_ALREADY_EXISTS", fmt.Sprintf("pet def %q already exists", item.Id))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreatePetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := writeJSON(ctx, store, petDefKey(item.Id), item); err != nil {
		return adminhttp.CreatePetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.CreatePetDef200JSONResponse(item), nil
}

func (c *Catalog) DeletePetDef(ctx context.Context, request adminhttp.DeletePetDefRequestObject) (adminhttp.DeletePetDefResponseObject, error) {
	store, err := c.store(c.PetDefs, "pet defs")
	if err != nil {
		return adminhttp.DeletePetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	item, err := readJSON[apitypes.PetDef](ctx, store, petDefKey(id))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeletePetDef404JSONResponse(apitypes.NewErrorResponse("PET_DEF_NOT_FOUND", fmt.Sprintf("pet def %q not found", id))), nil
		}
		return adminhttp.DeletePetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, petDefKey(id)); err != nil {
		return adminhttp.DeletePetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if c.Assets != nil {
		_ = c.Assets.DeletePrefix(path.Join("pet-defs", id))
	}
	return adminhttp.DeletePetDef200JSONResponse(item), nil
}

func (c *Catalog) GetPetDef(ctx context.Context, request adminhttp.GetPetDefRequestObject) (adminhttp.GetPetDefResponseObject, error) {
	item, err := c.GetPetDefByID(ctx, request.Id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetPetDef404JSONResponse(apitypes.NewErrorResponse("PET_DEF_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.GetPetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetPetDef200JSONResponse(item), nil
}

func (c *Catalog) PutPetDef(ctx context.Context, request adminhttp.PutPetDefRequestObject) (adminhttp.PutPetDefResponseObject, error) {
	if request.Body == nil {
		return adminhttp.PutPetDef400JSONResponse(apitypes.NewErrorResponse("INVALID_PET_DEF", "request body required")), nil
	}
	store, err := c.store(c.PetDefs, "pet defs")
	if err != nil {
		return adminhttp.PutPetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	previous, err := c.readPetDefJSON(ctx, store, petDefKey(id))
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutPetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	createdAt := time.Time{}
	var pixaPath *string
	if err == nil {
		createdAt = previous.CreatedAt
		pixaPath = previous.PixaPath
		if pixaPath != nil && !reflect.DeepEqual(previous.Spec.Visual.Pixa.Metadata, request.Body.Spec.Visual.Pixa.Metadata) {
			pixaPath = nil
		}
	}
	item, err := c.buildPetDefForUpdate(id, request.Body.Spec, request.Body.I18n, previous, err == nil, pixaPath, createdAt)
	if err != nil {
		return adminhttp.PutPetDef400JSONResponse(apitypes.NewErrorResponse("INVALID_PET_DEF", err.Error())), nil
	}
	if err := writeJSON(ctx, store, petDefKey(item.Id), item); err != nil {
		return adminhttp.PutPetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutPetDef200JSONResponse(item), nil
}

func (c *Catalog) DownloadPetDefPixa(ctx context.Context, request adminhttp.DownloadPetDefPixaRequestObject) (adminhttp.DownloadPetDefPixaResponseObject, error) {
	item, err := c.GetPetDefByID(ctx, request.Id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DownloadPetDefPixa404JSONResponse(apitypes.NewErrorResponse("PET_DEF_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.DownloadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	reader, size, err := c.openAsset(valueOrZero(item.PixaPath))
	if err != nil {
		return adminhttp.DownloadPetDefPixa404JSONResponse(apitypes.NewErrorResponse("PET_DEF_PIXA_NOT_FOUND", err.Error())), nil
	}
	return adminhttp.DownloadPetDefPixa200ApplicationoctetStreamResponse{Body: reader, ContentLength: size}, nil
}

func (c *Catalog) UploadPetDefPixa(ctx context.Context, request adminhttp.UploadPetDefPixaRequestObject) (adminhttp.UploadPetDefPixaResponseObject, error) {
	if request.Body == nil {
		return adminhttp.UploadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INVALID_PET_DEF_PIXA", "request body required")), nil
	}
	store, err := c.store(c.PetDefs, "pet defs")
	if err != nil {
		return adminhttp.UploadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	item, err := c.GetPetDefByID(ctx, request.Id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.UploadPetDefPixa404JSONResponse(apitypes.NewErrorResponse("PET_DEF_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.UploadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	data, err := io.ReadAll(request.Body)
	if err != nil {
		return adminhttp.UploadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := validatePetDefPixa(data, item.Spec.Visual.Pixa.Metadata); err != nil {
		return adminhttp.UploadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INVALID_PET_DEF_PIXA", err.Error())), nil
	}
	pixaPath := path.Join("pet-defs", item.Id, "pixa")
	if err := c.putAsset(pixaPath, bytes.NewReader(data)); err != nil {
		return adminhttp.UploadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	item.PixaPath = &pixaPath
	item.UpdatedAt = c.now()
	if err := writeJSON(ctx, store, petDefKey(item.Id), item); err != nil {
		return adminhttp.UploadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.UploadPetDefPixa200JSONResponse(item), nil
}

func (c *Catalog) ListBadgeDefs(ctx context.Context, request adminhttp.ListBadgeDefsRequestObject) (adminhttp.ListBadgeDefsResponseObject, error) {
	store, err := c.store(c.BadgeDefs, "badge defs")
	if err != nil {
		return adminhttp.ListBadgeDefs500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listJSON[apitypes.BadgeDef](ctx, store, badgeDefsRoot, cursor, limit)
	if err != nil {
		return adminhttp.ListBadgeDefs500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListBadgeDefs200JSONResponse(adminhttp.BadgeDefList{Items: items, HasNext: hasNext, NextCursor: nextCursor}), nil
}

func (c *Catalog) CreateBadgeDef(ctx context.Context, request adminhttp.CreateBadgeDefRequestObject) (adminhttp.CreateBadgeDefResponseObject, error) {
	if request.Body == nil {
		return adminhttp.CreateBadgeDef400JSONResponse(apitypes.NewErrorResponse("INVALID_BADGE_DEF", "request body required")), nil
	}
	store, err := c.store(c.BadgeDefs, "badge defs")
	if err != nil {
		return adminhttp.CreateBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := strings.TrimSpace(request.Body.Id)
	item, err := c.buildBadgeDef(id, request.Body.Spec, nil, time.Time{})
	if err != nil {
		return adminhttp.CreateBadgeDef400JSONResponse(apitypes.NewErrorResponse("INVALID_BADGE_DEF", err.Error())), nil
	}
	if _, err := store.Get(ctx, badgeDefKey(item.Id)); err == nil {
		return adminhttp.CreateBadgeDef409JSONResponse(apitypes.NewErrorResponse("BADGE_DEF_ALREADY_EXISTS", fmt.Sprintf("badge def %q already exists", item.Id))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := writeJSON(ctx, store, badgeDefKey(item.Id), item); err != nil {
		return adminhttp.CreateBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.CreateBadgeDef200JSONResponse(item), nil
}

func (c *Catalog) DeleteBadgeDef(ctx context.Context, request adminhttp.DeleteBadgeDefRequestObject) (adminhttp.DeleteBadgeDefResponseObject, error) {
	store, err := c.store(c.BadgeDefs, "badge defs")
	if err != nil {
		return adminhttp.DeleteBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	item, err := readJSON[apitypes.BadgeDef](ctx, store, badgeDefKey(id))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteBadgeDef404JSONResponse(apitypes.NewErrorResponse("BADGE_DEF_NOT_FOUND", fmt.Sprintf("badge def %q not found", id))), nil
		}
		return adminhttp.DeleteBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, badgeDefKey(id)); err != nil {
		return adminhttp.DeleteBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if c.Assets != nil {
		_ = c.Assets.DeletePrefix(path.Join("badge-defs", id))
	}
	return adminhttp.DeleteBadgeDef200JSONResponse(item), nil
}

func (c *Catalog) GetBadgeDef(ctx context.Context, request adminhttp.GetBadgeDefRequestObject) (adminhttp.GetBadgeDefResponseObject, error) {
	item, err := c.GetBadgeDefByID(ctx, request.Id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetBadgeDef404JSONResponse(apitypes.NewErrorResponse("BADGE_DEF_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.GetBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetBadgeDef200JSONResponse(item), nil
}

func (c *Catalog) PutBadgeDef(ctx context.Context, request adminhttp.PutBadgeDefRequestObject) (adminhttp.PutBadgeDefResponseObject, error) {
	if request.Body == nil {
		return adminhttp.PutBadgeDef400JSONResponse(apitypes.NewErrorResponse("INVALID_BADGE_DEF", "request body required")), nil
	}
	store, err := c.store(c.BadgeDefs, "badge defs")
	if err != nil {
		return adminhttp.PutBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	previous, err := readJSON[apitypes.BadgeDef](ctx, store, badgeDefKey(id))
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	createdAt := time.Time{}
	var pixaPath *string
	if err == nil {
		createdAt = previous.CreatedAt
		pixaPath = previous.PixaPath
	}
	item, err := c.buildBadgeDef(id, request.Body.Spec, pixaPath, createdAt)
	if err != nil {
		return adminhttp.PutBadgeDef400JSONResponse(apitypes.NewErrorResponse("INVALID_BADGE_DEF", err.Error())), nil
	}
	if err := writeJSON(ctx, store, badgeDefKey(item.Id), item); err != nil {
		return adminhttp.PutBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutBadgeDef200JSONResponse(item), nil
}

func (c *Catalog) DownloadBadgeDefPixa(ctx context.Context, request adminhttp.DownloadBadgeDefPixaRequestObject) (adminhttp.DownloadBadgeDefPixaResponseObject, error) {
	item, err := c.GetBadgeDefByID(ctx, request.Id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DownloadBadgeDefPixa404JSONResponse(apitypes.NewErrorResponse("BADGE_DEF_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.DownloadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	reader, size, err := c.openAsset(valueOrZero(item.PixaPath))
	if err != nil {
		return adminhttp.DownloadBadgeDefPixa404JSONResponse(apitypes.NewErrorResponse("BADGE_DEF_PIXA_NOT_FOUND", err.Error())), nil
	}
	return adminhttp.DownloadBadgeDefPixa200ApplicationoctetStreamResponse{Body: reader, ContentLength: size}, nil
}

func (c *Catalog) UploadBadgeDefPixa(ctx context.Context, request adminhttp.UploadBadgeDefPixaRequestObject) (adminhttp.UploadBadgeDefPixaResponseObject, error) {
	if request.Body == nil {
		return adminhttp.UploadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INVALID_BADGE_DEF_PIXA", "request body required")), nil
	}
	store, err := c.store(c.BadgeDefs, "badge defs")
	if err != nil {
		return adminhttp.UploadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	item, err := c.GetBadgeDefByID(ctx, request.Id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.UploadBadgeDefPixa404JSONResponse(apitypes.NewErrorResponse("BADGE_DEF_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.UploadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	data, err := io.ReadAll(request.Body)
	if err != nil {
		return adminhttp.UploadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := validateBadgeDefPixa(data); err != nil {
		return adminhttp.UploadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INVALID_BADGE_DEF_PIXA", err.Error())), nil
	}
	pixaPath := path.Join("badge-defs", item.Id, "pixa")
	if err := c.putAsset(pixaPath, bytes.NewReader(data)); err != nil {
		return adminhttp.UploadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	item.PixaPath = &pixaPath
	item.UpdatedAt = c.now()
	if err := writeJSON(ctx, store, badgeDefKey(item.Id), item); err != nil {
		return adminhttp.UploadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.UploadBadgeDefPixa200JSONResponse(item), nil
}

func (c *Catalog) ListGameDefs(ctx context.Context, request adminhttp.ListGameDefsRequestObject) (adminhttp.ListGameDefsResponseObject, error) {
	store, err := c.store(c.GameDefs, "game defs")
	if err != nil {
		return adminhttp.ListGameDefs500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listJSON[apitypes.GameDef](ctx, store, gameDefsRoot, cursor, limit)
	if err != nil {
		return adminhttp.ListGameDefs500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListGameDefs200JSONResponse(adminhttp.GameDefList{Items: items, HasNext: hasNext, NextCursor: nextCursor}), nil
}

func (c *Catalog) CreateGameDef(ctx context.Context, request adminhttp.CreateGameDefRequestObject) (adminhttp.CreateGameDefResponseObject, error) {
	if request.Body == nil {
		return adminhttp.CreateGameDef400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_DEF", "request body required")), nil
	}
	store, err := c.store(c.GameDefs, "game defs")
	if err != nil {
		return adminhttp.CreateGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := strings.TrimSpace(request.Body.Id)
	item, err := c.buildGameDef(id, request.Body.Spec, time.Time{})
	if err != nil {
		return adminhttp.CreateGameDef400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_DEF", err.Error())), nil
	}
	if _, err := store.Get(ctx, gameDefKey(item.Id)); err == nil {
		return adminhttp.CreateGameDef409JSONResponse(apitypes.NewErrorResponse("GAME_DEF_ALREADY_EXISTS", fmt.Sprintf("game def %q already exists", item.Id))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := writeJSON(ctx, store, gameDefKey(item.Id), item); err != nil {
		return adminhttp.CreateGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.CreateGameDef200JSONResponse(item), nil
}

func (c *Catalog) DeleteGameDef(ctx context.Context, request adminhttp.DeleteGameDefRequestObject) (adminhttp.DeleteGameDefResponseObject, error) {
	store, err := c.store(c.GameDefs, "game defs")
	if err != nil {
		return adminhttp.DeleteGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	item, err := readJSON[apitypes.GameDef](ctx, store, gameDefKey(id))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteGameDef404JSONResponse(apitypes.NewErrorResponse("GAME_DEF_NOT_FOUND", fmt.Sprintf("game def %q not found", id))), nil
		}
		return adminhttp.DeleteGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, gameDefKey(id)); err != nil {
		return adminhttp.DeleteGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteGameDef200JSONResponse(item), nil
}

func (c *Catalog) GetGameDef(ctx context.Context, request adminhttp.GetGameDefRequestObject) (adminhttp.GetGameDefResponseObject, error) {
	item, err := c.GetGameDefByID(ctx, request.Id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetGameDef404JSONResponse(apitypes.NewErrorResponse("GAME_DEF_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.GetGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetGameDef200JSONResponse(item), nil
}

func (c *Catalog) PutGameDef(ctx context.Context, request adminhttp.PutGameDefRequestObject) (adminhttp.PutGameDefResponseObject, error) {
	if request.Body == nil {
		return adminhttp.PutGameDef400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_DEF", "request body required")), nil
	}
	store, err := c.store(c.GameDefs, "game defs")
	if err != nil {
		return adminhttp.PutGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	previous, err := readJSON[apitypes.GameDef](ctx, store, gameDefKey(id))
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	createdAt := time.Time{}
	if err == nil {
		createdAt = previous.CreatedAt
	}
	item, err := c.buildGameDef(id, request.Body.Spec, createdAt)
	if err != nil {
		return adminhttp.PutGameDef400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_DEF", err.Error())), nil
	}
	if err := writeJSON(ctx, store, gameDefKey(item.Id), item); err != nil {
		return adminhttp.PutGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutGameDef200JSONResponse(item), nil
}

func (c *Catalog) GetGameRulesetByName(ctx context.Context, name string) (apitypes.GameRuleset, error) {
	store, err := c.store(c.GameRulesets, "game rulesets")
	if err != nil {
		return apitypes.GameRuleset{}, err
	}
	id, err := pathID(name)
	if err != nil {
		return apitypes.GameRuleset{}, err
	}
	item, err := readJSON[apitypes.GameRuleset](ctx, store, rulesetKey(id))
	if errors.Is(err, kv.ErrNotFound) {
		return apitypes.GameRuleset{}, fmt.Errorf("game ruleset %q not found: %w", id, kv.ErrNotFound)
	}
	return item, err
}

func (c *Catalog) GetPetDefByID(ctx context.Context, id string) (apitypes.PetDef, error) {
	store, err := c.store(c.PetDefs, "pet defs")
	if err != nil {
		return apitypes.PetDef{}, err
	}
	id, err = pathID(id)
	if err != nil {
		return apitypes.PetDef{}, err
	}
	item, err := c.readPetDefJSON(ctx, store, petDefKey(id))
	if errors.Is(err, kv.ErrNotFound) {
		return apitypes.PetDef{}, fmt.Errorf("pet def %q not found: %w", id, kv.ErrNotFound)
	}
	return item, err
}

func (c *Catalog) GetBadgeDefByID(ctx context.Context, id string) (apitypes.BadgeDef, error) {
	store, err := c.store(c.BadgeDefs, "badge defs")
	if err != nil {
		return apitypes.BadgeDef{}, err
	}
	id, err = pathID(id)
	if err != nil {
		return apitypes.BadgeDef{}, err
	}
	item, err := readJSON[apitypes.BadgeDef](ctx, store, badgeDefKey(id))
	if errors.Is(err, kv.ErrNotFound) {
		return apitypes.BadgeDef{}, fmt.Errorf("badge def %q not found: %w", id, kv.ErrNotFound)
	}
	return item, err
}

func (c *Catalog) GetGameDefByID(ctx context.Context, id string) (apitypes.GameDef, error) {
	store, err := c.store(c.GameDefs, "game defs")
	if err != nil {
		return apitypes.GameDef{}, err
	}
	id, err = pathID(id)
	if err != nil {
		return apitypes.GameDef{}, err
	}
	item, err := readJSON[apitypes.GameDef](ctx, store, gameDefKey(id))
	if errors.Is(err, kv.ErrNotFound) {
		return apitypes.GameDef{}, fmt.Errorf("game def %q not found: %w", id, kv.ErrNotFound)
	}
	return item, err
}

func (c *Catalog) buildGameRuleset(name string, spec apitypes.GameRulesetSpec, createdAt time.Time) (apitypes.GameRuleset, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return apitypes.GameRuleset{}, errors.New("name is required")
	}
	if len(spec.PetPool) == 0 {
		return apitypes.GameRuleset{}, errors.New("pet_pool is required")
	}
	for i, entry := range spec.PetPool {
		if strings.TrimSpace(entry.PetdefId) == "" {
			return apitypes.GameRuleset{}, fmt.Errorf("pet_pool[%d].petdef_id is required", i)
		}
		if entry.Weight <= 0 {
			return apitypes.GameRuleset{}, fmt.Errorf("pet_pool[%d].weight must be positive", i)
		}
	}
	now := c.now()
	if createdAt.IsZero() {
		createdAt = now
	}
	return apitypes.GameRuleset{Name: name, Spec: spec, CreatedAt: createdAt, UpdatedAt: now}, nil
}

func (c *Catalog) buildPetDef(id string, spec apitypes.PetDefSpec, i18n apitypes.PetDefI18nSpec, pixaPath *string, createdAt time.Time) (apitypes.PetDef, error) {
	return c.buildPetDefWithValidator(id, spec, i18n, pixaPath, createdAt, validatePetDef)
}

func (c *Catalog) buildPetDefForUpdate(id string, spec apitypes.PetDefSpec, i18n apitypes.PetDefI18nSpec, previous apitypes.PetDef, hasPrevious bool, pixaPath *string, createdAt time.Time) (apitypes.PetDef, error) {
	validate := func(spec apitypes.PetDefSpec, i18n apitypes.PetDefI18nSpec) error {
		err := validatePetDef(spec, i18n)
		if err == nil {
			return nil
		}
		if hasPrevious && isLegacyMigratedPetDef(previous) && isLegacyMigratedPetDefSpec(id, spec) {
			if legacyErr := validateMigratedLegacyPetDef(spec, i18n); legacyErr == nil {
				return nil
			}
		}
		return err
	}
	return c.buildPetDefWithValidator(id, spec, i18n, pixaPath, createdAt, validate)
}

func (c *Catalog) buildPetDefWithValidator(id string, spec apitypes.PetDefSpec, i18n apitypes.PetDefI18nSpec, pixaPath *string, createdAt time.Time, validate func(apitypes.PetDefSpec, apitypes.PetDefI18nSpec) error) (apitypes.PetDef, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return apitypes.PetDef{}, errors.New("id is required")
	}
	if err := validate(spec, i18n); err != nil {
		return apitypes.PetDef{}, err
	}
	now := c.now()
	if createdAt.IsZero() {
		createdAt = now
	}
	return apitypes.PetDef{Id: id, Spec: spec, I18n: i18n, PixaPath: pixaPath, CreatedAt: createdAt, UpdatedAt: now}, nil
}

func validatePetDef(spec apitypes.PetDefSpec, i18n apitypes.PetDefI18nSpec) error {
	if err := validatePetAttrGroup("attr.life", spec.Attr.Life); err != nil {
		return err
	}
	if err := validatePetAttrGroup("attr.progression", spec.Attr.Progression); err != nil {
		return err
	}
	if _, ok := spec.Attr.Progression["xp"]; !ok {
		return errors.New("attr.progression.xp is required")
	}
	if strings.TrimSpace(spec.Character.Prompt) == "" {
		return errors.New("character.prompt is required")
	}
	if strings.TrimSpace(spec.Voice.VoiceId) == "" {
		return errors.New("voice.voice_id is required")
	}
	if strings.TrimSpace(spec.Voice.Prompt) == "" {
		return errors.New("voice.prompt is required")
	}
	if err := validatePetDefVisual(spec.Visual); err != nil {
		return err
	}
	if err := validatePetDefDrive(spec.Drive, spec.Visual.Pixa.Metadata.Clips, spec.Attr); err != nil {
		return err
	}
	if err := validatePetDefI18n(spec, i18n); err != nil {
		return err
	}
	return nil
}

func validatePetAttrGroup(path string, group apitypes.PetAttrGroupSpec) error {
	if len(group) == 0 {
		return fmt.Errorf("%s must define at least one attribute", path)
	}
	for id := range group {
		if id == "" {
			return fmt.Errorf("%s contains an empty attribute id", path)
		}
		if strings.TrimSpace(id) != id {
			return fmt.Errorf("%s.%s must not contain leading or trailing whitespace", path, id)
		}
		switch id {
		case "initial", "display_name", "description", petProgressionStorageMarker:
			return fmt.Errorf("%s.%s uses a reserved attribute id", path, id)
		}
	}
	return nil
}

func validatePetDefVisual(visual apitypes.PetDefVisualSpec) error {
	if strings.TrimSpace(visual.Pixa.AssetRef) == "" {
		return errors.New("visual.pixa.asset_ref is required")
	}
	if strings.TrimSpace(visual.Pixa.Metadata.Version) == "" {
		return errors.New("visual.pixa.metadata.version is required")
	}
	if strings.TrimSpace(visual.Pixa.Metadata.Version) != "1" {
		return errors.New("visual.pixa.metadata.version must be 1")
	}
	if visual.Pixa.Metadata.Canvas.Width <= 0 || visual.Pixa.Metadata.Canvas.Height <= 0 {
		return errors.New("visual.pixa.metadata.canvas width and height must be positive")
	}
	if visual.Pixa.Metadata.Canvas.Width > pixaMaxCanvasSize || visual.Pixa.Metadata.Canvas.Height > pixaMaxCanvasSize {
		return fmt.Errorf("visual.pixa.metadata.canvas width and height must be <= %d", pixaMaxCanvasSize)
	}
	if len(visual.Pixa.Metadata.Clips) == 0 {
		return errors.New("visual.pixa.metadata.clips is required")
	}
	seenIDs := map[string]struct{}{}
	seenPixaClipNames := map[string]struct{}{}
	for i, clip := range visual.Pixa.Metadata.Clips {
		id := strings.TrimSpace(clip.Id)
		if id == "" {
			return fmt.Errorf("visual.pixa.metadata.clips[%d].id is required", i)
		}
		if id != clip.Id {
			return fmt.Errorf("visual.pixa.metadata.clips[%d].id must not contain leading or trailing whitespace", i)
		}
		if _, ok := seenIDs[id]; ok {
			return fmt.Errorf("visual.pixa.metadata.clips[%d].id %q is duplicated", i, id)
		}
		seenIDs[id] = struct{}{}
		pixaClipName := strings.TrimSpace(clip.PixaClipName)
		if pixaClipName == "" {
			return fmt.Errorf("visual.pixa.metadata.clips[%d].pixa_clip_name is required", i)
		}
		if pixaClipName != clip.PixaClipName {
			return fmt.Errorf("visual.pixa.metadata.clips[%d].pixa_clip_name must not contain leading or trailing whitespace", i)
		}
		if len([]byte(pixaClipName)) > pixaClipNameSize {
			return fmt.Errorf("visual.pixa.metadata.clips[%d].pixa_clip_name must be at most %d bytes", i, pixaClipNameSize)
		}
		if _, ok := seenPixaClipNames[pixaClipName]; ok {
			return fmt.Errorf("visual.pixa.metadata.clips[%d].pixa_clip_name %q is duplicated", i, pixaClipName)
		}
		seenPixaClipNames[pixaClipName] = struct{}{}
	}
	return nil
}

func validatePetDefDrive(drive apitypes.PetDefDriveSpec, clips []apitypes.PetDefPixaClipMetadata, attr apitypes.PetDefAttrSpec) error {
	if len(drive.Actions) == 0 {
		return errors.New("drive.actions is required")
	}
	clipIDs := map[string]struct{}{}
	for _, clip := range clips {
		clipIDs[clip.Id] = struct{}{}
	}
	seen := map[string]struct{}{}
	for i, action := range drive.Actions {
		id := strings.TrimSpace(action.Id)
		if id == "" {
			return fmt.Errorf("drive.actions[%d].id is required", i)
		}
		if id != action.Id {
			return fmt.Errorf("drive.actions[%d].id must not contain leading or trailing whitespace", i)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("drive.actions[%d].id %q is duplicated", i, id)
		}
		seen[id] = struct{}{}
		if action.Cost < 0 {
			return fmt.Errorf("drive.actions[%d].cost must be non-negative", i)
		}
		if action.VisualClipId != nil {
			clipID := strings.TrimSpace(*action.VisualClipId)
			if clipID == "" {
				return fmt.Errorf("drive.actions[%d].visual_clip_id must not be empty", i)
			}
			if clipID != *action.VisualClipId {
				return fmt.Errorf("drive.actions[%d].visual_clip_id must not contain leading or trailing whitespace", i)
			}
			if _, ok := clipIDs[clipID]; !ok {
				return fmt.Errorf("drive.actions[%d].visual_clip_id %q is not in visual.pixa.metadata.clips", i, clipID)
			}
		}
		if action.Effect != nil && action.Effect.AttrDelta != nil && action.Effect.AttrDelta.Life != nil {
			for attrID := range *action.Effect.AttrDelta.Life {
				if strings.TrimSpace(attrID) == "" {
					return fmt.Errorf("drive.actions[%d].effect.attr_delta.life contains an empty attribute id", i)
				}
				if strings.TrimSpace(attrID) != attrID {
					return fmt.Errorf("drive.actions[%d].effect.attr_delta.life.%s must not contain leading or trailing whitespace", i, attrID)
				}
				if _, ok := attr.Life[attrID]; !ok {
					return fmt.Errorf("drive.actions[%d].effect.attr_delta.life.%s does not match a PetDef life attribute", i, attrID)
				}
			}
		}
	}
	for i, clip := range clips {
		if clip.ActionId == nil {
			continue
		}
		actionID := strings.TrimSpace(*clip.ActionId)
		if actionID == "" {
			return fmt.Errorf("visual.pixa.metadata.clips[%d].action_id must not be empty", i)
		}
		if actionID != *clip.ActionId {
			return fmt.Errorf("visual.pixa.metadata.clips[%d].action_id must not contain leading or trailing whitespace", i)
		}
		if _, ok := seen[actionID]; !ok {
			return fmt.Errorf("visual.pixa.metadata.clips[%d].action_id %q is not in drive.actions", i, actionID)
		}
	}
	return nil
}

func validatePetDefI18n(spec apitypes.PetDefSpec, i18n apitypes.PetDefI18nSpec) error {
	if strings.TrimSpace(i18n.DefaultLocale) == "" {
		return errors.New("i18n.default_locale is required")
	}
	if strings.TrimSpace(i18n.DefaultLocale) != i18n.DefaultLocale {
		return errors.New("i18n.default_locale must not contain leading or trailing whitespace")
	}
	locales := i18n.AdditionalProperties
	if _, ok := locales[i18n.DefaultLocale]; !ok {
		return fmt.Errorf("i18n.%s is required", i18n.DefaultLocale)
	}
	for localeName, locale := range locales {
		if strings.TrimSpace(localeName) == "" {
			return errors.New("i18n contains an empty locale")
		}
		if localeName == "default_locale" {
			return errors.New("i18n.default_locale is reserved")
		}
		if strings.TrimSpace(localeName) != localeName {
			return fmt.Errorf("i18n.%s must not contain leading or trailing whitespace", localeName)
		}
		if locale.Attr != nil {
			if err := validateI18nAttrGroup("i18n."+localeName+".attr.life", locale.Attr.Life, spec.Attr.Life); err != nil {
				return err
			}
			if err := validateI18nAttrGroup("i18n."+localeName+".attr.progression", locale.Attr.Progression, spec.Attr.Progression); err != nil {
				return err
			}
		}
		if locale.Drive != nil && locale.Drive.Actions != nil {
			actionIDs := map[string]struct{}{}
			for _, action := range spec.Drive.Actions {
				actionIDs[action.Id] = struct{}{}
			}
			for actionID := range *locale.Drive.Actions {
				text := (*locale.Drive.Actions)[actionID]
				if _, ok := actionIDs[actionID]; !ok {
					return fmt.Errorf("i18n.%s.drive.actions.%s does not match a drive action", localeName, actionID)
				}
				if strings.TrimSpace(text.DisplayName) == "" {
					return fmt.Errorf("i18n.%s.drive.actions.%s.display_name is required", localeName, actionID)
				}
			}
		}
	}
	return nil
}

func validateI18nAttrGroup(path string, values *apitypes.PetDefI18nAttrGroup, defs apitypes.PetAttrGroupSpec) error {
	if values == nil {
		return nil
	}
	for attrID, text := range *values {
		if _, ok := defs[attrID]; !ok {
			return fmt.Errorf("%s.%s does not match a PetDef attribute", path, attrID)
		}
		if strings.TrimSpace(text.DisplayName) == "" {
			return fmt.Errorf("%s.%s.display_name is required", path, attrID)
		}
	}
	return nil
}

func (c *Catalog) buildBadgeDef(id string, spec apitypes.BadgeDefSpec, pixaPath *string, createdAt time.Time) (apitypes.BadgeDef, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return apitypes.BadgeDef{}, errors.New("id is required")
	}
	if strings.TrimSpace(spec.DisplayName) == "" {
		return apitypes.BadgeDef{}, errors.New("display_name is required")
	}
	now := c.now()
	if createdAt.IsZero() {
		createdAt = now
	}
	return apitypes.BadgeDef{Id: id, Spec: spec, PixaPath: pixaPath, CreatedAt: createdAt, UpdatedAt: now}, nil
}

func (c *Catalog) buildGameDef(id string, spec apitypes.GameDefSpec, createdAt time.Time) (apitypes.GameDef, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return apitypes.GameDef{}, errors.New("id is required")
	}
	if strings.TrimSpace(spec.DisplayName) == "" {
		return apitypes.GameDef{}, errors.New("display_name is required")
	}
	now := c.now()
	if createdAt.IsZero() {
		createdAt = now
	}
	return apitypes.GameDef{Id: id, Spec: spec, CreatedAt: createdAt, UpdatedAt: now}, nil
}

func (c *Catalog) store(store kv.Store, name string) (kv.Store, error) {
	if store == nil {
		return nil, fmt.Errorf("gameplay: %s store is not configured", name)
	}
	return store, nil
}

func (c *Catalog) now() time.Time {
	if c != nil && c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

func (c *Catalog) putAsset(name string, reader io.Reader) error {
	if c == nil || c.Assets == nil {
		return errors.New("gameplay: assets store is not configured")
	}
	return c.Assets.Put(name, reader)
}

func (c *Catalog) OpenAsset(name string) (io.ReadCloser, int64, error) {
	return c.openAsset(name)
}

func (c *Catalog) openAsset(name string) (io.ReadCloser, int64, error) {
	if c == nil || c.Assets == nil {
		return nil, 0, errors.New("gameplay: assets store is not configured")
	}
	if strings.TrimSpace(name) == "" {
		return nil, 0, errors.New("asset path is empty")
	}
	reader, err := c.Assets.Get(name)
	if err != nil {
		return nil, 0, err
	}
	size := int64(0)
	if infos, err := c.Assets.List(name); err == nil {
		for _, info := range infos {
			if info.Name == name {
				size = info.Size
				break
			}
		}
	}
	return reader, size, nil
}

func rulesetKey(name string) kv.Key { return append(append(kv.Key(nil), gameRulesetsRoot...), name) }
func petDefKey(id string) kv.Key    { return append(append(kv.Key(nil), petDefsRoot...), id) }
func badgeDefKey(id string) kv.Key  { return append(append(kv.Key(nil), badgeDefsRoot...), id) }
func gameDefKey(id string) kv.Key   { return append(append(kv.Key(nil), gameDefsRoot...), id) }

func writeJSON(ctx context.Context, store kv.Store, key kv.Key, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return store.Set(ctx, key, data)
}

func readJSON[T any](ctx context.Context, store kv.Store, key kv.Key) (T, error) {
	var out T
	data, err := store.Get(ctx, key)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

type legacyPetDefJSON struct {
	Id   string `json:"id"`
	Spec struct {
		DisplayName    string            `json:"display_name"`
		Description    string            `json:"description"`
		WorkflowName   *string           `json:"workflow_name,omitempty"`
		InitialLife    map[string]int64  `json:"initial_life"`
		InitialAbility map[string]int64  `json:"initial_ability"`
		Character      map[string]string `json:"character,omitempty"`
		Voice          map[string]string `json:"voice,omitempty"`
	} `json:"spec"`
	PixaPath  *string   `json:"pixa_path,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type preP22PetDefJSON struct {
	Id   string `json:"id"`
	Spec struct {
		apitypes.PetDefSpec
		DefaultLocale string                  `json:"default_locale"`
		I18n          apitypes.PetDefI18nSpec `json:"i18n"`
	} `json:"spec"`
	PixaPath  *string   `json:"pixa_path,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type legacyGameRulesetAction struct {
	Cost   int64
	Reward apitypes.GameRewardSpec
	Effect apitypes.PetDefActionEffectSpec
}

type legacyGameRulesetJSON struct {
	Spec struct {
		Drive *struct {
			ActionCosts   map[string]int64            `json:"action_costs"`
			ActionRewards map[string]legacyRewardSpec `json:"action_rewards"`
		} `json:"drive"`
	} `json:"spec"`
}

type legacyRewardSpec struct {
	BadgeExpDelta map[string]int64 `json:"badge_exp_delta"`
	LifeDelta     map[string]int64 `json:"life_delta"`
	PetExpDelta   *int64           `json:"pet_exp_delta"`
	PointsDelta   *int64           `json:"points_delta"`
}

func (c *Catalog) readPetDefJSON(ctx context.Context, store kv.Store, key kv.Key) (apitypes.PetDef, error) {
	data, err := store.Get(ctx, key)
	if err != nil {
		return apitypes.PetDef{}, err
	}
	var item apitypes.PetDef
	if err := json.Unmarshal(data, &item); err != nil {
		return apitypes.PetDef{}, err
	}
	if err := validatePetDef(item.Spec, item.I18n); err == nil {
		return item, nil
	}
	if isLegacyMigratedPetDef(item) {
		if err := validateMigratedLegacyPetDef(item.Spec, item.I18n); err == nil {
			return item, nil
		}
	}
	return c.migrateLegacyPetDefJSON(data)
}

func (c *Catalog) listPetDefJSON(ctx context.Context, store kv.Store, cursor string, limit int) ([]apitypes.PetDef, bool, *string, error) {
	entries, err := kv.ListAfter(ctx, store, petDefsRoot, cursorAfterKey(petDefsRoot, cursor), limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	pageEntries, hasNext, nextCursor := paginateEntries(entries, limit)
	items := make([]apitypes.PetDef, 0, len(pageEntries))
	for _, entry := range pageEntries {
		item, err := c.migratePetDefData(entry.Value)
		if err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	return items, hasNext, nextCursor, nil
}

func (c *Catalog) migratePetDefData(data []byte) (apitypes.PetDef, error) {
	var item apitypes.PetDef
	if err := json.Unmarshal(data, &item); err != nil {
		return apitypes.PetDef{}, err
	}
	if err := validatePetDef(item.Spec, item.I18n); err == nil {
		return item, nil
	}
	if isLegacyMigratedPetDef(item) {
		if err := validateMigratedLegacyPetDef(item.Spec, item.I18n); err == nil {
			return item, nil
		}
	}
	return c.migrateLegacyPetDefJSON(data)
}

func (c *Catalog) migrateLegacyPetDefJSON(data []byte) (apitypes.PetDef, error) {
	if item, ok := migratePreP22PetDefJSON(data); ok {
		return item, nil
	}
	var legacy legacyPetDefJSON
	if err := json.Unmarshal(data, &legacy); err != nil {
		return apitypes.PetDef{}, err
	}
	if legacy.Id == "" || legacy.Spec.DisplayName == "" {
		var item apitypes.PetDef
		if err := json.Unmarshal(data, &item); err != nil {
			return apitypes.PetDef{}, err
		}
		return apitypes.PetDef{}, validatePetDef(item.Spec, item.I18n)
	}
	if len(legacy.Spec.InitialLife) == 0 {
		legacy.Spec.InitialLife = map[string]int64{"hunger": 100}
	}
	description := legacy.Spec.Description
	if strings.TrimSpace(description) == "" {
		description = legacy.Spec.DisplayName
	}
	pixaMetadata := c.legacyPetDefPixaMetadata(legacy.PixaPath)
	spec := apitypes.PetDefSpec{
		WorkflowName: legacy.Spec.WorkflowName,
		Attr: apitypes.PetDefAttrSpec{
			Life:        apitypes.PetAttrGroupSpec{},
			Progression: apitypes.PetAttrGroupSpec{"xp": {Initial: 0}},
		},
		Character: apitypes.PetDefCharacterSpec{Prompt: legacy.Spec.Character["prompt"]},
		Voice: apitypes.PetDefVoiceSpec{
			VoiceId: legacy.Spec.Voice["voice_id"],
			Prompt:  legacy.Spec.Voice["prompt"],
		},
		Drive: apitypes.PetDefDriveSpec{Actions: []apitypes.PetDefActionSpec{}},
		Visual: apitypes.PetDefVisualSpec{
			Refs: apitypes.PetDefVisualRefsSpec{},
			Pixa: apitypes.PetDefPixaSpec{
				AssetRef: "asset://pets/" + legacy.Id + "/pet.pixa",
				Metadata: pixaMetadata,
			},
		},
	}
	i18n := apitypes.PetDefI18nSpec{
		DefaultLocale: "en",
		AdditionalProperties: map[string]apitypes.PetDefI18nCatalog{
			"en": {
				DisplayName: &legacy.Spec.DisplayName,
				Description: &description,
			},
		},
	}
	for id, initial := range legacy.Spec.InitialLife {
		spec.Attr.Life[id] = apitypes.PetAttrValueSpec{Initial: initial}
	}
	if legacy.Spec.InitialAbility != nil {
		if xp, ok := legacy.Spec.InitialAbility["xp"]; ok {
			spec.Attr.Progression["xp"] = apitypes.PetAttrValueSpec{Initial: xp}
		}
	}
	if strings.TrimSpace(spec.Character.Prompt) == "" {
		spec.Character.Prompt = legacy.Spec.DisplayName
	}
	if strings.TrimSpace(spec.Voice.VoiceId) == "" {
		spec.Voice.VoiceId = "default"
	}
	if strings.TrimSpace(spec.Voice.Prompt) == "" {
		spec.Voice.Prompt = legacy.Spec.DisplayName
	}
	if err := validateMigratedLegacyPetDef(spec, i18n); err != nil {
		return apitypes.PetDef{}, err
	}
	return apitypes.PetDef{
		Id:        legacy.Id,
		Spec:      spec,
		I18n:      i18n,
		PixaPath:  legacy.PixaPath,
		CreatedAt: legacy.CreatedAt,
		UpdatedAt: legacy.UpdatedAt,
	}, nil
}

func migratePreP22PetDefJSON(data []byte) (apitypes.PetDef, bool) {
	var legacy preP22PetDefJSON
	if err := json.Unmarshal(data, &legacy); err != nil {
		return apitypes.PetDef{}, false
	}
	if strings.TrimSpace(legacy.Id) == "" || strings.TrimSpace(legacy.Spec.DefaultLocale) == "" {
		return apitypes.PetDef{}, false
	}
	i18n := legacy.Spec.I18n
	i18n.DefaultLocale = legacy.Spec.DefaultLocale
	item := apitypes.PetDef{
		Id:        legacy.Id,
		Spec:      legacy.Spec.PetDefSpec,
		I18n:      i18n,
		PixaPath:  legacy.PixaPath,
		CreatedAt: legacy.CreatedAt,
		UpdatedAt: legacy.UpdatedAt,
	}
	if err := validatePetDef(item.Spec, item.I18n); err == nil {
		return item, true
	}
	if isLegacyMigratedPetDef(item) {
		if err := validateMigratedLegacyPetDef(item.Spec, item.I18n); err == nil {
			return item, true
		}
	}
	return apitypes.PetDef{}, false
}

func (c *Catalog) legacyPetDefPixaMetadata(pixaPath *string) apitypes.PetDefPixaMetadata {
	metadata := apitypes.PetDefPixaMetadata{
		Version: "1",
		Canvas:  apitypes.PetDefPixaCanvasMetadata{Width: 60, Height: 60},
		Clips: []apitypes.PetDefPixaClipMetadata{
			{Id: "idle", PixaClipName: "idle"},
		},
	}
	if pixaPath == nil {
		return metadata
	}
	reader, _, err := c.openAsset(*pixaPath)
	if err != nil {
		return metadata
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return metadata
	}
	asset, err := parsePixa(data)
	if err != nil {
		return metadata
	}
	metadata.Canvas = apitypes.PetDefPixaCanvasMetadata{
		Width:  int64(asset.width),
		Height: int64(asset.height),
	}
	return metadata
}

func validateMigratedLegacyPetDef(spec apitypes.PetDefSpec, i18n apitypes.PetDefI18nSpec) error {
	if err := validatePetAttrGroup("attr.life", spec.Attr.Life); err != nil {
		return err
	}
	if err := validatePetAttrGroup("attr.progression", spec.Attr.Progression); err != nil {
		return err
	}
	if _, ok := spec.Attr.Progression["xp"]; !ok {
		return errors.New("attr.progression.xp is required")
	}
	if strings.TrimSpace(spec.Character.Prompt) == "" {
		return errors.New("character.prompt is required")
	}
	if strings.TrimSpace(spec.Voice.VoiceId) == "" {
		return errors.New("voice.voice_id is required")
	}
	if strings.TrimSpace(spec.Voice.Prompt) == "" {
		return errors.New("voice.prompt is required")
	}
	if err := validatePetDefVisual(spec.Visual); err != nil {
		return err
	}
	return validatePetDefI18n(spec, i18n)
}

func (c *Catalog) legacyGameRulesetAction(ctx context.Context, rulesetName, actionID string) (legacyGameRulesetAction, bool, error) {
	store, err := c.store(c.GameRulesets, "game rulesets")
	if err != nil {
		return legacyGameRulesetAction{}, false, err
	}
	name, err := pathID(rulesetName)
	if err != nil {
		return legacyGameRulesetAction{}, false, err
	}
	data, err := store.Get(ctx, rulesetKey(name))
	if err != nil {
		return legacyGameRulesetAction{}, false, err
	}
	var legacy legacyGameRulesetJSON
	if err := json.Unmarshal(data, &legacy); err != nil {
		return legacyGameRulesetAction{}, false, err
	}
	actionID = strings.TrimSpace(actionID)
	if legacy.Spec.Drive == nil || actionID == "" {
		return legacyGameRulesetAction{}, false, nil
	}
	out := legacyGameRulesetAction{}
	found := false
	if cost, ok := legacy.Spec.Drive.ActionCosts[actionID]; ok {
		out.Cost = cost
		found = true
	}
	if reward, ok := legacy.Spec.Drive.ActionRewards[actionID]; ok {
		out.Reward = legacyReward(reward)
		if len(reward.LifeDelta) > 0 {
			life := apitypes.PetLife(reward.LifeDelta)
			out.Effect.AttrDelta = &apitypes.PetAttrDelta{Life: &life}
		}
		found = true
	}
	return out, found, nil
}

func legacyReward(in legacyRewardSpec) apitypes.GameRewardSpec {
	out := apitypes.GameRewardSpec{
		PetExpDelta: in.PetExpDelta,
		PointsDelta: in.PointsDelta,
	}
	if len(in.BadgeExpDelta) > 0 {
		out.BadgeExpDelta = &in.BadgeExpDelta
	}
	return out
}

func listJSON[T any](ctx context.Context, store kv.Store, prefix kv.Key, cursor string, limit int) ([]T, bool, *string, error) {
	entries, err := kv.ListAfter(ctx, store, prefix, cursorAfterKey(prefix, cursor), limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	pageEntries, hasNext, nextCursor := paginateEntries(entries, limit)
	items := make([]T, 0, len(pageEntries))
	for _, entry := range pageEntries {
		var item T
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	return items, hasNext, nextCursor, nil
}

func paginateEntries(entries []kv.Entry, limit int) ([]kv.Entry, bool, *string) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	hasNext := len(entries) > limit
	if hasNext {
		entries = entries[:limit]
	}
	var nextCursor *string
	if hasNext && len(entries) > 0 {
		cursor := entries[len(entries)-1].Key.String()
		nextCursor = &cursor
	}
	return entries, hasNext, nextCursor
}

func normalizeListParams(cursor *string, limit *int32) (string, int) {
	normalizedLimit := defaultListLimit
	if limit != nil && *limit > 0 {
		normalizedLimit = int(*limit)
	}
	if normalizedLimit > maxListLimit {
		normalizedLimit = maxListLimit
	}
	if cursor == nil {
		return "", normalizedLimit
	}
	return strings.TrimSpace(*cursor), normalizedLimit
}

func cursorAfterKey(prefix kv.Key, cursor string) kv.Key {
	if strings.TrimSpace(cursor) == "" {
		return nil
	}
	if strings.Contains(cursor, ":") {
		return kv.Key(strings.Split(cursor, ":"))
	}
	return append(append(kv.Key(nil), prefix...), cursor)
}

func pathID(id string) (string, error) {
	value, err := url.PathUnescape(id)
	if err != nil {
		return "", fmt.Errorf("invalid path id: %w", err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("id is required")
	}
	return value, nil
}

func valueOrZero[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}
