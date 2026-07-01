package main

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/appconfig"
	"github.com/GizClaw/gizclaw-go/apps/wails/internal/bridge"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestAppExposesContextRuntimeWithoutServerAccess(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	app, err := NewAppWithPaths(appconfig.NewPaths(t.TempDir()))
	if err != nil {
		t.Fatalf("NewAppWithPaths() error = %v", err)
	}
	runtime, err := app.CreateContext(bridge.CreateContextRequest{
		Description:     "Local e2e",
		Endpoint:        "127.0.0.1:9820",
		Name:            "local",
		ServerPublicKey: serverKey.Public.String(),
	})
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	if runtime.Context == nil || runtime.SignalingURL != "http://127.0.0.1:9820/webrtc/v1/offer" {
		t.Fatalf("runtime = %+v", runtime)
	}
	bootstrap, err := app.Bootstrap()
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if len(bootstrap.Contexts) != 1 || bootstrap.Runtime.Context == nil {
		t.Fatalf("bootstrap = %+v", bootstrap)
	}
}
