package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/gameplay/badge"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

var (
	rewardDecisionTool = genx.MustNewFuncTool[rpcapi.RewardDecision](
		"decide_reward",
		"Return the reward to grant for a user claim. Use point_amount for points and badge_id for an optional badge.",
	)
	petActionDecisionTool = genx.MustNewFuncTool[rpcapi.PetActionDecision](
		"decide_pet_action",
		"Return point, life, and ability deltas for a pet action.",
	)
)

type genxRewardDecider struct {
	Generator genx.Generator
	Pattern   string
}

func (d genxRewardDecider) DecideReward(ctx context.Context, owner string, prompt string) (rpcapi.RewardDecision, error) {
	var mcb genx.ModelContextBuilder
	mcb.PromptText("reward_claim", strings.TrimSpace(`
You are the GizClaw reward policy engine.
Decide whether a peer's reward claim deserves points, a badge, or both.
Return only the decide_reward tool result. Use point_amount = 0 and badge_id = "" when not granting that part.
`))
	mcb.UserText("claim", fmt.Sprintf("peer: %s\nprompt: %s", owner, prompt))
	return invokeSystemDecision[rpcapi.RewardDecision](ctx, d.Generator, d.Pattern, mcb.Build(), rewardDecisionTool)
}

type genxPetActionDecider struct {
	Generator genx.Generator
	Pattern   string
}

func (d genxPetActionDecider) DecidePetAction(ctx context.Context, action string, prompt string, pet rpcapi.PetObject) (rpcapi.PetActionDecision, error) {
	var mcb genx.ModelContextBuilder
	mcb.PromptText("pet_action", strings.TrimSpace(`
You are the GizClaw pet action engine.
Given the action, prompt, and current pet state, return only the decide_pet_action tool result.
Use zero values for fields that should not change.
`))
	state, err := json.Marshal(pet)
	if err != nil {
		return rpcapi.PetActionDecision{}, err
	}
	mcb.UserText("pet_action", fmt.Sprintf("action: %s\nprompt: %s\ncurrent_pet: %s", action, prompt, state))
	return invokeSystemDecision[rpcapi.PetActionDecision](ctx, d.Generator, d.Pattern, mcb.Build(), petActionDecisionTool)
}

func invokeSystemDecision[T any](ctx context.Context, generator genx.Generator, pattern string, mctx genx.ModelContext, tool *genx.FuncTool) (T, error) {
	var zero T
	if generator == nil {
		return zero, errors.New("system task generator not configured")
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return zero, errors.New("system task generator pattern is required")
	}
	_, call, err := generator.Invoke(ctx, pattern, mctx, tool)
	if err != nil {
		return zero, err
	}
	if call == nil {
		return zero, errors.New("system task generator returned no tool call")
	}
	if tool == nil || call.Name != tool.Name {
		return zero, fmt.Errorf("system task generator returned unsupported tool call %q", call.Name)
	}
	var out T
	if err := json.Unmarshal([]byte(call.Arguments), &out); err != nil {
		return zero, fmt.Errorf("parse system task decision: %w", err)
	}
	return out, nil
}

type systemTaskPeer giznet.PublicKey

func (p systemTaskPeer) PublicKey() giznet.PublicKey {
	return giznet.PublicKey(p)
}

type systemTaskAuthorizer struct{}

func (systemTaskAuthorizer) Authorize(context.Context, acl.AuthorizeRequest) error {
	return nil
}

type badgeGrantResolver struct {
	Badges *badge.Server
	ACL    aclAuthorizer
}

func (r badgeGrantResolver) CanGrantBadge(ctx context.Context, owner string, badgeID string) error {
	if r.Badges == nil {
		return errors.New("badge service not configured")
	}
	_, err := r.Badges.Get(ctx, badgeID)
	if err != nil {
		return err
	}
	if r.ACL == nil {
		return errors.New("acl service not configured")
	}
	return r.ACL.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    acl.PublicKeySubject(owner),
		Resource:   acl.BadgeResource(badgeID),
		Permission: apitypes.ACLPermissionBadgeUse,
	})
}
