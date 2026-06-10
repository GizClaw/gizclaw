package acl

import (
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

const (
	SubjectKindPublicKey = apitypes.ACLSubjectKindPk
	SubjectKindView      = apitypes.ACLSubjectKindView
	SubjectKindAllPeers  = apitypes.ACLSubjectKindAllPeers

	ResourceKindWorkspace  = apitypes.ACLResourceKindWorkspace
	ResourceKindWorkflow   = apitypes.ACLResourceKindWorkflow
	ResourceKindVoice      = apitypes.ACLResourceKindVoice
	ResourceKindCredential = apitypes.ACLResourceKindCredential
	ResourceKindModel      = apitypes.ACLResourceKindModel
	ResourceKindView       = apitypes.ACLResourceKindView
	ResourceKindPet        = apitypes.ACLResourceKindPet
	ResourceKindWallet     = apitypes.ACLResourceKindWallet
	ResourceKindContact    = apitypes.ACLResourceKindContact
	ResourceKindFriend     = apitypes.ACLResourceKindFriend
	ResourceKindFriendReq  = apitypes.ACLResourceKindFriendRequest
	ResourceKindGroup      = apitypes.ACLResourceKindGroup
	ResourceKindCall       = apitypes.ACLResourceKindCall
	ResourceKindGameResult = apitypes.ACLResourceKindGameResult
	ResourceKindReward     = apitypes.ACLResourceKindReward
)

var ErrDenied = errors.New("acl: denied")

func CanonicalSubject(subject apitypes.ACLSubject) (string, error) {
	kind := strings.TrimSpace(string(subject.Kind))
	id := strings.TrimSpace(subject.Id)
	if kind == "" {
		return "", errors.New("acl: subject kind is required")
	}
	if id == "" && subject.Kind != SubjectKindAllPeers {
		return "", errors.New("acl: subject id is required")
	}
	if id != "" && subject.Kind == SubjectKindAllPeers {
		return "", errors.New("acl: all_peers subject id must be empty")
	}
	if strings.Contains(kind, ":") {
		return "", fmt.Errorf("acl: subject kind %q must not contain ':'", kind)
	}
	if strings.Contains(id, ":") {
		return "", fmt.Errorf("acl: subject id %q must not contain ':'", id)
	}
	if !subject.Kind.Valid() {
		return "", fmt.Errorf("acl: unsupported subject kind %q", kind)
	}
	return kind + ":" + id, nil
}

func CanonicalResource(resource apitypes.ACLResource) (string, error) {
	kind := strings.TrimSpace(string(resource.Kind))
	id := strings.TrimSpace(resource.Id)
	if kind == "" {
		return "", errors.New("acl: resource kind is required")
	}
	if id == "" {
		return "", errors.New("acl: resource id is required")
	}
	if strings.Contains(kind, ":") {
		return "", fmt.Errorf("acl: resource kind %q must not contain ':'", kind)
	}
	if strings.Contains(id, ":") {
		return "", fmt.Errorf("acl: resource id %q must not contain ':'", id)
	}
	if !resource.Kind.Valid() {
		return "", fmt.Errorf("acl: unsupported resource kind %q", kind)
	}
	return kind + ":" + id, nil
}

func PublicKeySubject(publicKey string) apitypes.ACLSubject {
	return apitypes.ACLSubject{
		Kind: SubjectKindPublicKey,
		Id:   publicKey,
	}
}

func ViewSubject(name string) apitypes.ACLSubject {
	return apitypes.ACLSubject{
		Kind: SubjectKindView,
		Id:   name,
	}
}

func AllPeersSubject() apitypes.ACLSubject {
	return apitypes.ACLSubject{
		Kind: SubjectKindAllPeers,
	}
}

func WorkspaceResource(name string) apitypes.ACLResource {
	return apitypes.ACLResource{
		Kind: ResourceKindWorkspace,
		Id:   name,
	}
}

func ViewResource(name string) apitypes.ACLResource {
	return apitypes.ACLResource{
		Kind: ResourceKindView,
		Id:   name,
	}
}

func CredentialResource(name string) apitypes.ACLResource {
	return apitypes.ACLResource{
		Kind: ResourceKindCredential,
		Id:   name,
	}
}

func ModelResource(id string) apitypes.ACLResource {
	return apitypes.ACLResource{
		Kind: ResourceKindModel,
		Id:   id,
	}
}
