//go:build gizclaw_e2e

package social_test

import (
	"testing"

	cgointernal "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cgo/internal"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestCSDKSocialBasicRPC(t *testing.T) {
	h := clitest.NewSetupHarness(t, "cgo-social")
	identityDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_SOCIAL_PERSON_A_IDENTITY", "social-a")
	cgointernal.AssertServerAvailable(t, identityDir)
	cgointernal.CSDKSocialBasic(t, identityDir)
}

func TestCSDKSocialRelationships(t *testing.T) {
	h := clitest.NewSetupHarness(t, "cgo-social-relationships")
	identityADir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_SOCIAL_PERSON_A_IDENTITY", "social-a")
	identityBDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_SOCIAL_PERSON_B_IDENTITY", "social-b")
	cgointernal.AssertServerAvailable(t, identityADir)
	cgointernal.AssertServerAvailable(t, identityBDir)
	cgointernal.CSDKSocialRelationships(t, identityADir, identityBDir)
}
