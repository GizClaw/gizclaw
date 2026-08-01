//go:build gizclaw_e2e

package social_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	cgointernal "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cgo/internal"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestCSDKSocialBasicRPC(t *testing.T) {
	h := clitest.NewSetupHarness(t, "cgo-social")
	identityDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_SOCIAL_PERSON_A_IDENTITY", "social-a")
	cgointernal.AssertServerAvailable(t, identityDir)
	registrationToken := createCSDKSocialRegistrationToken(t, h, "basic")
	cgointernal.CSDKSocialBasic(t, identityDir, registrationToken)
}

func TestCSDKSocialRelationships(t *testing.T) {
	h := clitest.NewSetupHarness(t, "cgo-social-relationships")
	identityADir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_SOCIAL_PERSON_A_IDENTITY", "social-a")
	identityBDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_SOCIAL_PERSON_B_IDENTITY", "social-b")
	cgointernal.AssertServerAvailable(t, identityADir)
	cgointernal.AssertServerAvailable(t, identityBDir)
	registrationToken := createCSDKSocialRegistrationToken(t, h, "relationships")
	cgointernal.CSDKSocialRelationships(t, identityADir, identityBDir, registrationToken)
}

func createCSDKSocialRegistrationToken(
	t *testing.T,
	h *clitest.Harness,
	scenario string,
) string {
	t.Helper()
	const adminContext = "cgo-social-registration-admin"
	adminDir := cgointernal.SharedIdentityDir(
		t,
		h,
		"GIZCLAW_E2E_ADMIN_IDENTITY",
		"admin",
	)
	h.SetContextDirAlias(adminContext, adminDir)
	tokenName := fmt.Sprintf(
		"e2e-c-social-%s-%d",
		scenario,
		time.Now().UnixNano(),
	)

	admin := h.ConnectClientFromContext(adminContext)
	api, err := admin.ServerAdminClient()
	if err != nil {
		admin.Close()
		t.Fatalf("create C social admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	response, err := api.CreateRegistrationTokenWithResponse(
		ctx,
		adminhttp.RegistrationTokenUpsert{
			Name:               tokenName,
			Token:              tokenName,
			RuntimeProfileName: "default-gameplay",
		},
	)
	cancel()
	admin.Close()
	if err != nil {
		t.Fatalf("create C social RegistrationToken: %v", err)
	}
	if response.JSON200 == nil || response.JSON200.Token == "" {
		t.Fatalf(
			"create C social RegistrationToken status %d: %s",
			response.StatusCode(),
			strings.TrimSpace(string(response.Body)),
		)
	}

	t.Cleanup(func() {
		cleanupAdmin := h.ConnectClientFromContext(adminContext)
		defer cleanupAdmin.Close()
		cleanupAPI, err := cleanupAdmin.ServerAdminClient()
		if err != nil {
			t.Errorf("create C social cleanup admin client: %v", err)
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			15*time.Second,
		)
		defer cleanupCancel()
		cleanupResponse, err := cleanupAPI.DeleteRegistrationTokenWithResponse(
			cleanupCtx,
			tokenName,
		)
		if err != nil {
			t.Errorf("delete C social RegistrationToken: %v", err)
			return
		}
		if cleanupResponse.JSON200 == nil {
			t.Errorf(
				"delete C social RegistrationToken status %d: %s",
				cleanupResponse.StatusCode(),
				strings.TrimSpace(string(cleanupResponse.Body)),
			)
		}
	})
	return response.JSON200.Token
}
