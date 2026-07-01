//go:build gizclaw_e2e

package social_test

import "testing"

func TestSocialContactRPC(t *testing.T) {
	h := newSocialSimulatorHarness(t)

	assertContactRPCs(t, h)
}
