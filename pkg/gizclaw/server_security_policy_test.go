package gizclaw

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peer"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

func testServerSecurityPolicy(peers *peer.Server) *ServerSecurityPolicy {
	return (*ServerSecurityPolicy)(&Server{manager: NewManager(peers)})
}

func TestServerSecurityPolicyAllowsGearServiceForActiveGear(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	service := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	if _, err := service.SaveGear(context.Background(), apitypes.Gear{
		PublicKey:     keyPair.Public.String(),
		Role:          apitypes.GearRoleGear,
		Status:        apitypes.GearStatusActive,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SaveGear error = %v", err)
	}

	policy := testServerSecurityPolicy(service)
	if !policy.AllowService(keyPair.Public, ServiceGear) {
		t.Fatal("active gear should allow gear service")
	}
	if policy.AllowService(keyPair.Public, ServiceAdmin) {
		t.Fatal("active gear should not allow admin service without admin role")
	}
	if !policy.AllowService(keyPair.Public, ServiceServerPublic) {
		t.Fatal("active gear should allow server public service")
	}
	if policy.AllowService(keyPair.Public, 0xffff) {
		t.Fatal("active gear should not allow unknown service")
	}
}

func TestServerSecurityPolicyAllowsAdminServiceForActiveAdminGear(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	service := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	if _, err := service.SaveGear(context.Background(), apitypes.Gear{
		PublicKey:     keyPair.Public.String(),
		Role:          apitypes.GearRoleAdmin,
		Status:        apitypes.GearStatusActive,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SaveGear error = %v", err)
	}

	policy := testServerSecurityPolicy(service)
	if !policy.AllowService(keyPair.Public, ServiceAdmin) {
		t.Fatal("active admin gear should allow admin service")
	}
}

func TestServerSecurityPolicyRequiresAdminRoleForAdminService(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	service := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	if _, err := service.EnsureConnectedGear(context.Background(), keyPair.Public); err != nil {
		t.Fatalf("EnsureConnectedGear error = %v", err)
	}
	policy := testServerSecurityPolicy(service)
	if policy.AllowService(keyPair.Public, ServiceAdmin) {
		t.Fatal("non-admin gear should not allow admin service")
	}
	if !policy.AllowService(keyPair.Public, ServiceGear) {
		t.Fatal("active gear should still allow gear service")
	}
	stored, err := service.LoadGear(context.Background(), keyPair.Public)
	if err != nil {
		t.Fatalf("LoadGear error = %v", err)
	}
	if stored.Role != apitypes.GearRoleUnspecified {
		t.Fatalf("policy changed stored role to %q", stored.Role)
	}
}

func TestServerSecurityPolicyAllowsPublicServicesWithoutGearLookup(t *testing.T) {
	policy := (*ServerSecurityPolicy)(&Server{manager: &Manager{}})
	if !policy.AllowService(giznet.PublicKey{}, ServiceRPC) {
		t.Fatal("policy should allow rpc service")
	}
	if !policy.AllowService(giznet.PublicKey{}, ServiceServerPublic) {
		t.Fatal("policy should allow server public service")
	}
}

func TestServerSecurityPolicyAllowsGearServiceForUnknownGear(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	policy := testServerSecurityPolicy(&peer.Server{Store: mustBadgerInMemory(t, nil)})
	if !policy.AllowService(keyPair.Public, ServiceGear) {
		t.Fatal("unknown gear should allow gear service for registration")
	}
	if policy.AllowService(keyPair.Public, ServiceAdmin) {
		t.Fatal("unknown gear should not allow admin service")
	}
}

func TestServerSecurityPolicyDeniesProtectedServicesForBlockedGear(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	service := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	ctx := context.Background()
	if _, err := service.SaveGear(ctx, apitypes.Gear{
		PublicKey:     keyPair.Public.String(),
		Role:          apitypes.GearRoleUnspecified,
		Status:        apitypes.GearStatusBlocked,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SaveGear error = %v", err)
	}

	policy := testServerSecurityPolicy(service)
	if policy.AllowService(keyPair.Public, ServiceAdmin) {
		t.Fatal("blocked gear should not allow admin service")
	}
	if policy.AllowService(keyPair.Public, ServiceGear) {
		t.Fatal("blocked gear should not allow gear service")
	}
}
