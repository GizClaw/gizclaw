package acl

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

const (
	policyBindingIDPrefixBytes = 6
	policyBindingIDPartLimit   = 24
	policyBindingIDPKPartLimit = 8
)

func GeneratePolicyBindingID(policy apitypes.ACLPolicy) (string, error) {
	policy, err := normalizePolicy(policy)
	if err != nil {
		return "", err
	}
	var prefix [policyBindingIDPrefixBytes]byte
	if _, err := rand.Read(prefix[:]); err != nil {
		return "", err
	}
	parts := []string{
		hex.EncodeToString(prefix[:]),
		policyBindingIDPart(string(policy.Subject.Kind), policyBindingIDPartLimit),
		policyBindingIDPart(policy.Subject.Id, policyBindingIDPKPartLimit),
		policyBindingIDPart(string(policy.Resource.Kind), policyBindingIDPartLimit),
		policyBindingIDPart(policy.Resource.Id, policyBindingIDPartLimit),
		policyBindingIDPart(string(policy.Role), policyBindingIDPartLimit),
	}
	return strings.Join(parts, "-"), nil
}

func policyBindingIDPart(value string, limit int) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "x"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
		if b.Len() >= limit {
			break
		}
	}
	part := strings.Trim(b.String(), "-")
	if part == "" {
		return "x"
	}
	return part
}

func policyBindingIDOrGenerate(id string, policy apitypes.ACLPolicy) (string, error) {
	id = strings.TrimSpace(id)
	if id != "" {
		return validateName(id, "policy binding id")
	}
	id, err := GeneratePolicyBindingID(policy)
	if err != nil {
		return "", err
	}
	if id == "" {
		return "", errors.New("acl: generated policy binding id is empty")
	}
	return id, nil
}
