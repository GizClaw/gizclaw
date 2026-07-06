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

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
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
	ListGameRulesets(context.Context, adminservice.ListGameRulesetsRequestObject) (adminservice.ListGameRulesetsResponseObject, error)
	CreateGameRuleset(context.Context, adminservice.CreateGameRulesetRequestObject) (adminservice.CreateGameRulesetResponseObject, error)
	DeleteGameRuleset(context.Context, adminservice.DeleteGameRulesetRequestObject) (adminservice.DeleteGameRulesetResponseObject, error)
	GetGameRuleset(context.Context, adminservice.GetGameRulesetRequestObject) (adminservice.GetGameRulesetResponseObject, error)
	PutGameRuleset(context.Context, adminservice.PutGameRulesetRequestObject) (adminservice.PutGameRulesetResponseObject, error)
	ListPetDefs(context.Context, adminservice.ListPetDefsRequestObject) (adminservice.ListPetDefsResponseObject, error)
	CreatePetDef(context.Context, adminservice.CreatePetDefRequestObject) (adminservice.CreatePetDefResponseObject, error)
	DeletePetDef(context.Context, adminservice.DeletePetDefRequestObject) (adminservice.DeletePetDefResponseObject, error)
	GetPetDef(context.Context, adminservice.GetPetDefRequestObject) (adminservice.GetPetDefResponseObject, error)
	PutPetDef(context.Context, adminservice.PutPetDefRequestObject) (adminservice.PutPetDefResponseObject, error)
	DownloadPetDefPixa(context.Context, adminservice.DownloadPetDefPixaRequestObject) (adminservice.DownloadPetDefPixaResponseObject, error)
	UploadPetDefPixa(context.Context, adminservice.UploadPetDefPixaRequestObject) (adminservice.UploadPetDefPixaResponseObject, error)
	ListBadgeDefs(context.Context, adminservice.ListBadgeDefsRequestObject) (adminservice.ListBadgeDefsResponseObject, error)
	CreateBadgeDef(context.Context, adminservice.CreateBadgeDefRequestObject) (adminservice.CreateBadgeDefResponseObject, error)
	DeleteBadgeDef(context.Context, adminservice.DeleteBadgeDefRequestObject) (adminservice.DeleteBadgeDefResponseObject, error)
	GetBadgeDef(context.Context, adminservice.GetBadgeDefRequestObject) (adminservice.GetBadgeDefResponseObject, error)
	PutBadgeDef(context.Context, adminservice.PutBadgeDefRequestObject) (adminservice.PutBadgeDefResponseObject, error)
	DownloadBadgeDefPixa(context.Context, adminservice.DownloadBadgeDefPixaRequestObject) (adminservice.DownloadBadgeDefPixaResponseObject, error)
	UploadBadgeDefPixa(context.Context, adminservice.UploadBadgeDefPixaRequestObject) (adminservice.UploadBadgeDefPixaResponseObject, error)
	ListGameDefs(context.Context, adminservice.ListGameDefsRequestObject) (adminservice.ListGameDefsResponseObject, error)
	CreateGameDef(context.Context, adminservice.CreateGameDefRequestObject) (adminservice.CreateGameDefResponseObject, error)
	DeleteGameDef(context.Context, adminservice.DeleteGameDefRequestObject) (adminservice.DeleteGameDefResponseObject, error)
	GetGameDef(context.Context, adminservice.GetGameDefRequestObject) (adminservice.GetGameDefResponseObject, error)
	PutGameDef(context.Context, adminservice.PutGameDefRequestObject) (adminservice.PutGameDefResponseObject, error)
}

var _ CatalogAdminService = (*Catalog)(nil)

func (c *Catalog) ListGameRulesets(ctx context.Context, request adminservice.ListGameRulesetsRequestObject) (adminservice.ListGameRulesetsResponseObject, error) {
	store, err := c.store(c.GameRulesets, "game rulesets")
	if err != nil {
		return adminservice.ListGameRulesets500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listJSON[apitypes.GameRuleset](ctx, store, gameRulesetsRoot, cursor, limit)
	if err != nil {
		return adminservice.ListGameRulesets500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.ListGameRulesets200JSONResponse(adminservice.GameRulesetList{Items: items, HasNext: hasNext, NextCursor: nextCursor}), nil
}

func (c *Catalog) CreateGameRuleset(ctx context.Context, request adminservice.CreateGameRulesetRequestObject) (adminservice.CreateGameRulesetResponseObject, error) {
	if request.Body == nil {
		return adminservice.CreateGameRuleset400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_RULESET", "request body required")), nil
	}
	store, err := c.store(c.GameRulesets, "game rulesets")
	if err != nil {
		return adminservice.CreateGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name := strings.TrimSpace(request.Body.Name)
	item, err := c.buildGameRuleset(name, request.Body.Spec, time.Time{})
	if err != nil {
		return adminservice.CreateGameRuleset400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_RULESET", err.Error())), nil
	}
	if _, err := store.Get(ctx, rulesetKey(item.Name)); err == nil {
		return adminservice.CreateGameRuleset409JSONResponse(apitypes.NewErrorResponse("GAME_RULESET_ALREADY_EXISTS", fmt.Sprintf("game ruleset %q already exists", item.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminservice.CreateGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := writeJSON(ctx, store, rulesetKey(item.Name), item); err != nil {
		return adminservice.CreateGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.CreateGameRuleset200JSONResponse(item), nil
}

func (c *Catalog) DeleteGameRuleset(ctx context.Context, request adminservice.DeleteGameRulesetRequestObject) (adminservice.DeleteGameRulesetResponseObject, error) {
	store, err := c.store(c.GameRulesets, "game rulesets")
	if err != nil {
		return adminservice.DeleteGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := pathID(request.Name)
	if err != nil {
		return nil, err
	}
	item, err := readJSON[apitypes.GameRuleset](ctx, store, rulesetKey(name))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.DeleteGameRuleset404JSONResponse(apitypes.NewErrorResponse("GAME_RULESET_NOT_FOUND", fmt.Sprintf("game ruleset %q not found", name))), nil
		}
		return adminservice.DeleteGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, rulesetKey(name)); err != nil {
		return adminservice.DeleteGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.DeleteGameRuleset200JSONResponse(item), nil
}

func (c *Catalog) GetGameRuleset(ctx context.Context, request adminservice.GetGameRulesetRequestObject) (adminservice.GetGameRulesetResponseObject, error) {
	item, err := c.GetGameRulesetByName(ctx, request.Name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.GetGameRuleset404JSONResponse(apitypes.NewErrorResponse("GAME_RULESET_NOT_FOUND", err.Error())), nil
		}
		return adminservice.GetGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetGameRuleset200JSONResponse(item), nil
}

func (c *Catalog) PutGameRuleset(ctx context.Context, request adminservice.PutGameRulesetRequestObject) (adminservice.PutGameRulesetResponseObject, error) {
	if request.Body == nil {
		return adminservice.PutGameRuleset400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_RULESET", "request body required")), nil
	}
	store, err := c.store(c.GameRulesets, "game rulesets")
	if err != nil {
		return adminservice.PutGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := pathID(request.Name)
	if err != nil {
		return nil, err
	}
	previous, err := readJSON[apitypes.GameRuleset](ctx, store, rulesetKey(name))
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminservice.PutGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	createdAt := time.Time{}
	if err == nil {
		createdAt = previous.CreatedAt
	}
	item, err := c.buildGameRuleset(name, request.Body.Spec, createdAt)
	if err != nil {
		return adminservice.PutGameRuleset400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_RULESET", err.Error())), nil
	}
	if err := writeJSON(ctx, store, rulesetKey(item.Name), item); err != nil {
		return adminservice.PutGameRuleset500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.PutGameRuleset200JSONResponse(item), nil
}

func (c *Catalog) ListPetDefs(ctx context.Context, request adminservice.ListPetDefsRequestObject) (adminservice.ListPetDefsResponseObject, error) {
	store, err := c.store(c.PetDefs, "pet defs")
	if err != nil {
		return adminservice.ListPetDefs500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listJSON[apitypes.PetDef](ctx, store, petDefsRoot, cursor, limit)
	if err != nil {
		return adminservice.ListPetDefs500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.ListPetDefs200JSONResponse(adminservice.PetDefList{Items: items, HasNext: hasNext, NextCursor: nextCursor}), nil
}

func (c *Catalog) CreatePetDef(ctx context.Context, request adminservice.CreatePetDefRequestObject) (adminservice.CreatePetDefResponseObject, error) {
	if request.Body == nil {
		return adminservice.CreatePetDef400JSONResponse(apitypes.NewErrorResponse("INVALID_PET_DEF", "request body required")), nil
	}
	store, err := c.store(c.PetDefs, "pet defs")
	if err != nil {
		return adminservice.CreatePetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := strings.TrimSpace(request.Body.Id)
	item, err := c.buildPetDef(id, request.Body.Spec, nil, time.Time{})
	if err != nil {
		return adminservice.CreatePetDef400JSONResponse(apitypes.NewErrorResponse("INVALID_PET_DEF", err.Error())), nil
	}
	if _, err := store.Get(ctx, petDefKey(item.Id)); err == nil {
		return adminservice.CreatePetDef409JSONResponse(apitypes.NewErrorResponse("PET_DEF_ALREADY_EXISTS", fmt.Sprintf("pet def %q already exists", item.Id))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminservice.CreatePetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := writeJSON(ctx, store, petDefKey(item.Id), item); err != nil {
		return adminservice.CreatePetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.CreatePetDef200JSONResponse(item), nil
}

func (c *Catalog) DeletePetDef(ctx context.Context, request adminservice.DeletePetDefRequestObject) (adminservice.DeletePetDefResponseObject, error) {
	store, err := c.store(c.PetDefs, "pet defs")
	if err != nil {
		return adminservice.DeletePetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	item, err := readJSON[apitypes.PetDef](ctx, store, petDefKey(id))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.DeletePetDef404JSONResponse(apitypes.NewErrorResponse("PET_DEF_NOT_FOUND", fmt.Sprintf("pet def %q not found", id))), nil
		}
		return adminservice.DeletePetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, petDefKey(id)); err != nil {
		return adminservice.DeletePetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if c.Assets != nil {
		_ = c.Assets.DeletePrefix(path.Join("pet-defs", id))
	}
	return adminservice.DeletePetDef200JSONResponse(item), nil
}

func (c *Catalog) GetPetDef(ctx context.Context, request adminservice.GetPetDefRequestObject) (adminservice.GetPetDefResponseObject, error) {
	item, err := c.GetPetDefByID(ctx, request.Id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.GetPetDef404JSONResponse(apitypes.NewErrorResponse("PET_DEF_NOT_FOUND", err.Error())), nil
		}
		return adminservice.GetPetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetPetDef200JSONResponse(item), nil
}

func (c *Catalog) PutPetDef(ctx context.Context, request adminservice.PutPetDefRequestObject) (adminservice.PutPetDefResponseObject, error) {
	if request.Body == nil {
		return adminservice.PutPetDef400JSONResponse(apitypes.NewErrorResponse("INVALID_PET_DEF", "request body required")), nil
	}
	store, err := c.store(c.PetDefs, "pet defs")
	if err != nil {
		return adminservice.PutPetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	previous, err := readJSON[apitypes.PetDef](ctx, store, petDefKey(id))
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminservice.PutPetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	createdAt := time.Time{}
	var pixaPath *string
	if err == nil {
		createdAt = previous.CreatedAt
		pixaPath = previous.PixaPath
	}
	item, err := c.buildPetDef(id, request.Body.Spec, pixaPath, createdAt)
	if err != nil {
		return adminservice.PutPetDef400JSONResponse(apitypes.NewErrorResponse("INVALID_PET_DEF", err.Error())), nil
	}
	if err := writeJSON(ctx, store, petDefKey(item.Id), item); err != nil {
		return adminservice.PutPetDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.PutPetDef200JSONResponse(item), nil
}

func (c *Catalog) DownloadPetDefPixa(ctx context.Context, request adminservice.DownloadPetDefPixaRequestObject) (adminservice.DownloadPetDefPixaResponseObject, error) {
	item, err := c.GetPetDefByID(ctx, request.Id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.DownloadPetDefPixa404JSONResponse(apitypes.NewErrorResponse("PET_DEF_NOT_FOUND", err.Error())), nil
		}
		return adminservice.DownloadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	reader, size, err := c.openAsset(valueOrZero(item.PixaPath))
	if err != nil {
		return adminservice.DownloadPetDefPixa404JSONResponse(apitypes.NewErrorResponse("PET_DEF_PIXA_NOT_FOUND", err.Error())), nil
	}
	return adminservice.DownloadPetDefPixa200ApplicationoctetStreamResponse{Body: reader, ContentLength: size}, nil
}

func (c *Catalog) UploadPetDefPixa(ctx context.Context, request adminservice.UploadPetDefPixaRequestObject) (adminservice.UploadPetDefPixaResponseObject, error) {
	if request.Body == nil {
		return adminservice.UploadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INVALID_PET_DEF_PIXA", "request body required")), nil
	}
	store, err := c.store(c.PetDefs, "pet defs")
	if err != nil {
		return adminservice.UploadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	item, err := c.GetPetDefByID(ctx, request.Id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.UploadPetDefPixa404JSONResponse(apitypes.NewErrorResponse("PET_DEF_NOT_FOUND", err.Error())), nil
		}
		return adminservice.UploadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	data, err := io.ReadAll(request.Body)
	if err != nil {
		return adminservice.UploadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := validatePetDefPixa(data); err != nil {
		return adminservice.UploadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INVALID_PET_DEF_PIXA", err.Error())), nil
	}
	pixaPath := path.Join("pet-defs", item.Id, "pixa")
	if err := c.putAsset(pixaPath, bytes.NewReader(data)); err != nil {
		return adminservice.UploadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	item.PixaPath = &pixaPath
	item.UpdatedAt = c.now()
	if err := writeJSON(ctx, store, petDefKey(item.Id), item); err != nil {
		return adminservice.UploadPetDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.UploadPetDefPixa200JSONResponse(item), nil
}

func (c *Catalog) ListBadgeDefs(ctx context.Context, request adminservice.ListBadgeDefsRequestObject) (adminservice.ListBadgeDefsResponseObject, error) {
	store, err := c.store(c.BadgeDefs, "badge defs")
	if err != nil {
		return adminservice.ListBadgeDefs500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listJSON[apitypes.BadgeDef](ctx, store, badgeDefsRoot, cursor, limit)
	if err != nil {
		return adminservice.ListBadgeDefs500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.ListBadgeDefs200JSONResponse(adminservice.BadgeDefList{Items: items, HasNext: hasNext, NextCursor: nextCursor}), nil
}

func (c *Catalog) CreateBadgeDef(ctx context.Context, request adminservice.CreateBadgeDefRequestObject) (adminservice.CreateBadgeDefResponseObject, error) {
	if request.Body == nil {
		return adminservice.CreateBadgeDef400JSONResponse(apitypes.NewErrorResponse("INVALID_BADGE_DEF", "request body required")), nil
	}
	store, err := c.store(c.BadgeDefs, "badge defs")
	if err != nil {
		return adminservice.CreateBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := strings.TrimSpace(request.Body.Id)
	item, err := c.buildBadgeDef(id, request.Body.Spec, nil, time.Time{})
	if err != nil {
		return adminservice.CreateBadgeDef400JSONResponse(apitypes.NewErrorResponse("INVALID_BADGE_DEF", err.Error())), nil
	}
	if _, err := store.Get(ctx, badgeDefKey(item.Id)); err == nil {
		return adminservice.CreateBadgeDef409JSONResponse(apitypes.NewErrorResponse("BADGE_DEF_ALREADY_EXISTS", fmt.Sprintf("badge def %q already exists", item.Id))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminservice.CreateBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := writeJSON(ctx, store, badgeDefKey(item.Id), item); err != nil {
		return adminservice.CreateBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.CreateBadgeDef200JSONResponse(item), nil
}

func (c *Catalog) DeleteBadgeDef(ctx context.Context, request adminservice.DeleteBadgeDefRequestObject) (adminservice.DeleteBadgeDefResponseObject, error) {
	store, err := c.store(c.BadgeDefs, "badge defs")
	if err != nil {
		return adminservice.DeleteBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	item, err := readJSON[apitypes.BadgeDef](ctx, store, badgeDefKey(id))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.DeleteBadgeDef404JSONResponse(apitypes.NewErrorResponse("BADGE_DEF_NOT_FOUND", fmt.Sprintf("badge def %q not found", id))), nil
		}
		return adminservice.DeleteBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, badgeDefKey(id)); err != nil {
		return adminservice.DeleteBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if c.Assets != nil {
		_ = c.Assets.DeletePrefix(path.Join("badge-defs", id))
	}
	return adminservice.DeleteBadgeDef200JSONResponse(item), nil
}

func (c *Catalog) GetBadgeDef(ctx context.Context, request adminservice.GetBadgeDefRequestObject) (adminservice.GetBadgeDefResponseObject, error) {
	item, err := c.GetBadgeDefByID(ctx, request.Id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.GetBadgeDef404JSONResponse(apitypes.NewErrorResponse("BADGE_DEF_NOT_FOUND", err.Error())), nil
		}
		return adminservice.GetBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetBadgeDef200JSONResponse(item), nil
}

func (c *Catalog) PutBadgeDef(ctx context.Context, request adminservice.PutBadgeDefRequestObject) (adminservice.PutBadgeDefResponseObject, error) {
	if request.Body == nil {
		return adminservice.PutBadgeDef400JSONResponse(apitypes.NewErrorResponse("INVALID_BADGE_DEF", "request body required")), nil
	}
	store, err := c.store(c.BadgeDefs, "badge defs")
	if err != nil {
		return adminservice.PutBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	previous, err := readJSON[apitypes.BadgeDef](ctx, store, badgeDefKey(id))
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminservice.PutBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	createdAt := time.Time{}
	var pixaPath *string
	if err == nil {
		createdAt = previous.CreatedAt
		pixaPath = previous.PixaPath
	}
	item, err := c.buildBadgeDef(id, request.Body.Spec, pixaPath, createdAt)
	if err != nil {
		return adminservice.PutBadgeDef400JSONResponse(apitypes.NewErrorResponse("INVALID_BADGE_DEF", err.Error())), nil
	}
	if err := writeJSON(ctx, store, badgeDefKey(item.Id), item); err != nil {
		return adminservice.PutBadgeDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.PutBadgeDef200JSONResponse(item), nil
}

func (c *Catalog) DownloadBadgeDefPixa(ctx context.Context, request adminservice.DownloadBadgeDefPixaRequestObject) (adminservice.DownloadBadgeDefPixaResponseObject, error) {
	item, err := c.GetBadgeDefByID(ctx, request.Id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.DownloadBadgeDefPixa404JSONResponse(apitypes.NewErrorResponse("BADGE_DEF_NOT_FOUND", err.Error())), nil
		}
		return adminservice.DownloadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	reader, size, err := c.openAsset(valueOrZero(item.PixaPath))
	if err != nil {
		return adminservice.DownloadBadgeDefPixa404JSONResponse(apitypes.NewErrorResponse("BADGE_DEF_PIXA_NOT_FOUND", err.Error())), nil
	}
	return adminservice.DownloadBadgeDefPixa200ApplicationoctetStreamResponse{Body: reader, ContentLength: size}, nil
}

func (c *Catalog) UploadBadgeDefPixa(ctx context.Context, request adminservice.UploadBadgeDefPixaRequestObject) (adminservice.UploadBadgeDefPixaResponseObject, error) {
	if request.Body == nil {
		return adminservice.UploadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INVALID_BADGE_DEF_PIXA", "request body required")), nil
	}
	store, err := c.store(c.BadgeDefs, "badge defs")
	if err != nil {
		return adminservice.UploadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	item, err := c.GetBadgeDefByID(ctx, request.Id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.UploadBadgeDefPixa404JSONResponse(apitypes.NewErrorResponse("BADGE_DEF_NOT_FOUND", err.Error())), nil
		}
		return adminservice.UploadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	data, err := io.ReadAll(request.Body)
	if err != nil {
		return adminservice.UploadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := validateBadgeDefPixa(data); err != nil {
		return adminservice.UploadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INVALID_BADGE_DEF_PIXA", err.Error())), nil
	}
	pixaPath := path.Join("badge-defs", item.Id, "pixa")
	if err := c.putAsset(pixaPath, bytes.NewReader(data)); err != nil {
		return adminservice.UploadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	item.PixaPath = &pixaPath
	item.UpdatedAt = c.now()
	if err := writeJSON(ctx, store, badgeDefKey(item.Id), item); err != nil {
		return adminservice.UploadBadgeDefPixa500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.UploadBadgeDefPixa200JSONResponse(item), nil
}

func (c *Catalog) ListGameDefs(ctx context.Context, request adminservice.ListGameDefsRequestObject) (adminservice.ListGameDefsResponseObject, error) {
	store, err := c.store(c.GameDefs, "game defs")
	if err != nil {
		return adminservice.ListGameDefs500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listJSON[apitypes.GameDef](ctx, store, gameDefsRoot, cursor, limit)
	if err != nil {
		return adminservice.ListGameDefs500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.ListGameDefs200JSONResponse(adminservice.GameDefList{Items: items, HasNext: hasNext, NextCursor: nextCursor}), nil
}

func (c *Catalog) CreateGameDef(ctx context.Context, request adminservice.CreateGameDefRequestObject) (adminservice.CreateGameDefResponseObject, error) {
	if request.Body == nil {
		return adminservice.CreateGameDef400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_DEF", "request body required")), nil
	}
	store, err := c.store(c.GameDefs, "game defs")
	if err != nil {
		return adminservice.CreateGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := strings.TrimSpace(request.Body.Id)
	item, err := c.buildGameDef(id, request.Body.Spec, time.Time{})
	if err != nil {
		return adminservice.CreateGameDef400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_DEF", err.Error())), nil
	}
	if _, err := store.Get(ctx, gameDefKey(item.Id)); err == nil {
		return adminservice.CreateGameDef409JSONResponse(apitypes.NewErrorResponse("GAME_DEF_ALREADY_EXISTS", fmt.Sprintf("game def %q already exists", item.Id))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminservice.CreateGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := writeJSON(ctx, store, gameDefKey(item.Id), item); err != nil {
		return adminservice.CreateGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.CreateGameDef200JSONResponse(item), nil
}

func (c *Catalog) DeleteGameDef(ctx context.Context, request adminservice.DeleteGameDefRequestObject) (adminservice.DeleteGameDefResponseObject, error) {
	store, err := c.store(c.GameDefs, "game defs")
	if err != nil {
		return adminservice.DeleteGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	item, err := readJSON[apitypes.GameDef](ctx, store, gameDefKey(id))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.DeleteGameDef404JSONResponse(apitypes.NewErrorResponse("GAME_DEF_NOT_FOUND", fmt.Sprintf("game def %q not found", id))), nil
		}
		return adminservice.DeleteGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, gameDefKey(id)); err != nil {
		return adminservice.DeleteGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.DeleteGameDef200JSONResponse(item), nil
}

func (c *Catalog) GetGameDef(ctx context.Context, request adminservice.GetGameDefRequestObject) (adminservice.GetGameDefResponseObject, error) {
	item, err := c.GetGameDefByID(ctx, request.Id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.GetGameDef404JSONResponse(apitypes.NewErrorResponse("GAME_DEF_NOT_FOUND", err.Error())), nil
		}
		return adminservice.GetGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetGameDef200JSONResponse(item), nil
}

func (c *Catalog) PutGameDef(ctx context.Context, request adminservice.PutGameDefRequestObject) (adminservice.PutGameDefResponseObject, error) {
	if request.Body == nil {
		return adminservice.PutGameDef400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_DEF", "request body required")), nil
	}
	store, err := c.store(c.GameDefs, "game defs")
	if err != nil {
		return adminservice.PutGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	previous, err := readJSON[apitypes.GameDef](ctx, store, gameDefKey(id))
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminservice.PutGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	createdAt := time.Time{}
	if err == nil {
		createdAt = previous.CreatedAt
	}
	item, err := c.buildGameDef(id, request.Body.Spec, createdAt)
	if err != nil {
		return adminservice.PutGameDef400JSONResponse(apitypes.NewErrorResponse("INVALID_GAME_DEF", err.Error())), nil
	}
	if err := writeJSON(ctx, store, gameDefKey(item.Id), item); err != nil {
		return adminservice.PutGameDef500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.PutGameDef200JSONResponse(item), nil
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
