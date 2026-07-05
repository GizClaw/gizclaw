package gizclaw

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	voicepkg "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay/badge"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay/petspecies"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestFirstPetSpeciesSelectorRequiresUsePermission(t *testing.T) {
	ctx := context.Background()
	species := &petspecies.Server{Store: kv.NewMemory(nil)}
	if _, err := species.Put(ctx, "cat", apitypes.PetSpeciesSpec{Name: "Cat"}); err != nil {
		t.Fatalf("Put cat error = %v", err)
	}
	if _, err := species.Put(ctx, "rabbit", apitypes.PetSpeciesSpec{Name: "Rabbit"}); err != nil {
		t.Fatalf("Put rabbit error = %v", err)
	}

	auth := businessACLFunc(func(_ context.Context, req acl.AuthorizeRequest) error {
		if req.Subject != acl.PublicKeySubject("peer-a") {
			t.Fatalf("subject = %#v, want peer-a", req.Subject)
		}
		if req.Permission != apitypes.ACLPermissionUse {
			t.Fatalf("permission = %q, want use", req.Permission)
		}
		if req.Resource == acl.PetSpeciesResource("rabbit") {
			return nil
		}
		return acl.ErrDenied
	})
	got, err := (firstPetSpeciesSelector{Service: species, ACL: auth}).SelectSpecies(ctx, "peer-a")
	if err != nil {
		t.Fatalf("SelectSpecies error = %v", err)
	}
	if got != "rabbit" {
		t.Fatalf("SelectSpecies = %q, want rabbit", got)
	}
}

func TestFirstPetSpeciesSelectorScansPagesAndHandlesConfig(t *testing.T) {
	ctx := context.Background()
	if _, err := (firstPetSpeciesSelector{}).SelectSpecies(ctx, "peer-a"); err == nil {
		t.Fatalf("SelectSpecies(no service) error = nil, want error")
	}
	species := &petspecies.Server{Store: kv.NewMemory(nil)}
	for i := 0; i < 51; i++ {
		id := "species-" + string(rune('a'+i/26)) + string(rune('a'+i%26))
		if _, err := species.Put(ctx, id, apitypes.PetSpeciesSpec{Name: id}); err != nil {
			t.Fatalf("Put %s error = %v", id, err)
		}
	}
	if _, err := (firstPetSpeciesSelector{Service: species}).SelectSpecies(ctx, "peer-a"); err == nil {
		t.Fatalf("SelectSpecies(no ACL) error = nil, want error")
	}

	target := "species-by"
	auth := businessACLFunc(func(_ context.Context, req acl.AuthorizeRequest) error {
		if req.Resource == acl.PetSpeciesResource(target) {
			return nil
		}
		return acl.ErrDenied
	})
	got, err := (firstPetSpeciesSelector{Service: species, ACL: auth}).SelectSpecies(ctx, "peer-a")
	if err != nil {
		t.Fatalf("SelectSpecies paged error = %v", err)
	}
	if got != target {
		t.Fatalf("SelectSpecies paged = %q, want %q", got, target)
	}

	_, err = (firstPetSpeciesSelector{Service: species, ACL: businessACLFunc(func(context.Context, acl.AuthorizeRequest) error {
		return acl.ErrDenied
	})}).SelectSpecies(ctx, "peer-a")
	if err == nil || err.Error() != "no usable pet species available" {
		t.Fatalf("SelectSpecies(all denied) error = %v, want no usable species", err)
	}
}

func TestFirstPetSpeciesSelectorFallsBackToPeerView(t *testing.T) {
	ctx := context.Background()
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	species := &petspecies.Server{Store: kv.NewMemory(nil)}
	if _, err := species.Put(ctx, "rabbit", apitypes.PetSpeciesSpec{Name: "Rabbit"}); err != nil {
		t.Fatalf("Put rabbit error = %v", err)
	}
	view := "default-client"
	auth := fakePeerACL{
		allowedSubject:    acl.ViewSubject(view),
		allowedResource:   acl.PetSpeciesResource("rabbit"),
		allowedPermission: apitypes.ACLPermissionUse,
	}
	got, err := (firstPetSpeciesSelector{
		Service: species,
		ACL:     auth,
		Peers:   fakePeerConfigGetter{view: &view},
	}).SelectSpecies(ctx, key.Public.String())
	if err != nil {
		t.Fatalf("SelectSpecies error = %v", err)
	}
	if got != "rabbit" {
		t.Fatalf("SelectSpecies = %q, want rabbit", got)
	}
}

func TestFirstVoiceSelectorUsesFirstVoice(t *testing.T) {
	ctx := context.Background()
	if _, err := (firstVoiceSelector{}).SelectVoice(ctx, "peer-a"); err == nil {
		t.Fatalf("SelectVoice(no service) error = nil, want error")
	}
	voices := &voicepkg.Server{Store: kv.NewMemory(nil)}
	if _, err := (firstVoiceSelector{Service: voices}).SelectVoice(ctx, "peer-a"); err == nil {
		t.Fatalf("SelectVoice(empty) error = nil, want error")
	}
	body := adminservice.VoiceUpsert{
		Id:     "voice-a",
		Source: apitypes.VoiceSourceManual,
		Provider: apitypes.VoiceProvider{
			Kind: "openai-tenant",
			Name: "main",
		},
	}
	if resp, err := voices.CreateVoice(ctx, adminservice.CreateVoiceRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateVoice error = %v", err)
	} else if _, ok := resp.(adminservice.CreateVoice200JSONResponse); !ok {
		t.Fatalf("CreateVoice response = %#v", resp)
	}
	got, err := (firstVoiceSelector{Service: voices}).SelectVoice(ctx, "peer-a")
	if err != nil {
		t.Fatalf("SelectVoice error = %v", err)
	}
	if got != "voice-a" {
		t.Fatalf("SelectVoice = %q, want voice-a", got)
	}
}

func TestBadgeGrantResolverRequiresUsePermission(t *testing.T) {
	ctx := context.Background()
	badges := &badge.Server{Store: kv.NewMemory(nil)}
	if _, err := badges.Put(ctx, "founder", apitypes.BadgeSpec{Name: "Founder"}); err != nil {
		t.Fatalf("Put badge error = %v", err)
	}

	auth := businessACLFunc(func(_ context.Context, req acl.AuthorizeRequest) error {
		if req.Subject != acl.PublicKeySubject("peer-a") {
			t.Fatalf("subject = %#v, want peer-a", req.Subject)
		}
		if req.Resource != acl.BadgeResource("founder") {
			t.Fatalf("resource = %#v, want founder badge", req.Resource)
		}
		if req.Permission != apitypes.ACLPermissionUse {
			t.Fatalf("permission = %q, want use", req.Permission)
		}
		return acl.ErrDenied
	})
	err := (badgeGrantResolver{Badges: badges, ACL: auth}).CanGrantBadge(ctx, "peer-a", "founder")
	if !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("CanGrantBadge error = %v, want %v", err, acl.ErrDenied)
	}
}

func TestBadgeGrantResolverSuccessAndConfigErrors(t *testing.T) {
	ctx := context.Background()
	if err := (badgeGrantResolver{}).CanGrantBadge(ctx, "peer-a", "founder"); err == nil {
		t.Fatalf("CanGrantBadge(no service) error = nil, want error")
	}
	badges := &badge.Server{Store: kv.NewMemory(nil)}
	if err := (badgeGrantResolver{Badges: badges}).CanGrantBadge(ctx, "peer-a", "founder"); err == nil {
		t.Fatalf("CanGrantBadge(missing badge) error = nil, want error")
	}
	if _, err := badges.Put(ctx, "founder", apitypes.BadgeSpec{Name: "Founder"}); err != nil {
		t.Fatalf("Put badge error = %v", err)
	}
	if err := (badgeGrantResolver{Badges: badges}).CanGrantBadge(ctx, "peer-a", "founder"); err == nil {
		t.Fatalf("CanGrantBadge(no ACL) error = nil, want error")
	}
	auth := businessACLFunc(func(_ context.Context, req acl.AuthorizeRequest) error {
		if req.Resource != acl.BadgeResource("founder") || req.Permission != apitypes.ACLPermissionUse {
			t.Fatalf("authorize request = %#v", req)
		}
		return nil
	})
	if err := (badgeGrantResolver{Badges: badges, ACL: auth}).CanGrantBadge(ctx, "peer-a", "founder"); err != nil {
		t.Fatalf("CanGrantBadge success error = %v", err)
	}
}

type businessACLFunc func(context.Context, acl.AuthorizeRequest) error

func (f businessACLFunc) Authorize(ctx context.Context, req acl.AuthorizeRequest) error {
	return f(ctx, req)
}
