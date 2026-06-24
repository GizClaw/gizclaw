package acl

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestCanonicalSubject(t *testing.T) {
	got, err := CanonicalSubject(PublicKeySubject("z6Mk"))
	if err != nil {
		t.Fatalf("CanonicalSubject() error = %v", err)
	}
	if got != "pk:z6Mk" {
		t.Fatalf("CanonicalSubject() = %q, want pk:z6Mk", got)
	}
	got, err = CanonicalSubject(ViewSubject("under-12"))
	if err != nil {
		t.Fatalf("CanonicalSubject(view) error = %v", err)
	}
	if got != "view:under-12" {
		t.Fatalf("CanonicalSubject(view) = %q, want view:under-12", got)
	}
	got, err = CanonicalSubject(AllPeersSubject())
	if err != nil {
		t.Fatalf("CanonicalSubject(all_peers) error = %v", err)
	}
	if got != "all_peers:" {
		t.Fatalf("CanonicalSubject(all_peers) = %q, want all_peers:", got)
	}
	for _, subject := range []apitypes.ACLSubject{
		{},
		{Kind: SubjectKindPublicKey},
		{Kind: SubjectKindAllPeers, Id: "id"},
		{Kind: apitypes.ACLSubjectKind("bad"), Id: "id"},
		{Kind: SubjectKindPublicKey, Id: "bad:id"},
	} {
		if _, err := CanonicalSubject(subject); err == nil {
			t.Fatalf("CanonicalSubject(%#v) error = nil", subject)
		}
	}
}

func TestCanonicalResource(t *testing.T) {
	got, err := CanonicalResource(WorkspaceResource("demo"))
	if err != nil {
		t.Fatalf("CanonicalResource() error = %v", err)
	}
	if got != "workspace:demo" {
		t.Fatalf("CanonicalResource() = %q, want workspace:demo", got)
	}
	got, err = CanonicalResource(ViewResource("under-12"))
	if err != nil {
		t.Fatalf("CanonicalResource(view) error = %v", err)
	}
	if got != "view:under-12" {
		t.Fatalf("CanonicalResource(view) = %q, want view:under-12", got)
	}
	got, err = CanonicalResource(CredentialResource("openai-main"))
	if err != nil {
		t.Fatalf("CanonicalResource(credential) error = %v", err)
	}
	if got != "credential:openai-main" {
		t.Fatalf("CanonicalResource(credential) = %q, want credential:openai-main", got)
	}
	got, err = CanonicalResource(VoiceResource("volc-tenant:volc-main:voice-a"))
	if err != nil {
		t.Fatalf("CanonicalResource(voice with provider id) error = %v", err)
	}
	if got != "voice:volc-tenant:volc-main:voice-a" {
		t.Fatalf("CanonicalResource(voice with provider id) = %q, want voice:volc-tenant:volc-main:voice-a", got)
	}
	for _, resource := range []apitypes.ACLResource{
		{},
		{Kind: ResourceKindWorkspace},
		{Kind: apitypes.ACLResourceKind("bad"), Id: "id"},
	} {
		if _, err := CanonicalResource(resource); err == nil {
			t.Fatalf("CanonicalResource(%#v) error = nil", resource)
		}
	}
}
