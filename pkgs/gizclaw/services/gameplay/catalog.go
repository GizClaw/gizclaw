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
	items, hasNext, nextCursor, err := listJSON[apitypes.PetDef](ctx, store, petDefsRoot, cursor, limit)
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
	item, err := c.buildPetDef(id, request.Body.Spec, nil, time.Time{})
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
	previous, err := readJSON[apitypes.PetDef](ctx, store, petDefKey(id))
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutPetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	createdAt := time.Time{}
	var pixaPath *string
	if err == nil {
		createdAt = previous.CreatedAt
		pixaPath = previous.PixaPath
	}
	item, err := c.buildPetDef(id, request.Body.Spec, pixaPath, createdAt)
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
	if err := validatePetDefPixa(data); err != nil {
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
	item, err := readJSON[apitypes.PetDef](ctx, store, petDefKey(id))
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

func (c *Catalog) buildPetDef(id string, spec apitypes.PetDefSpec, pixaPath *string, createdAt time.Time) (apitypes.PetDef, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return apitypes.PetDef{}, errors.New("id is required")
	}
	if strings.TrimSpace(spec.DisplayName) == "" {
		return apitypes.PetDef{}, errors.New("display_name is required")
	}
	now := c.now()
	if createdAt.IsZero() {
		createdAt = now
	}
	return apitypes.PetDef{Id: id, Spec: spec, PixaPath: pixaPath, CreatedAt: createdAt, UpdatedAt: now}, nil
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
