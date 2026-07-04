//go:build gizclaw_e2e

package chat

import (
	"context"
	"fmt"
)

// PrepareCgoPushToTalkWorkspace recreates and reloads a voice workspace using
// the same setup path as the Go chat e2e cases, then returns its workspace name.
func PrepareCgoPushToTalkWorkspace(ctx context.Context, configPath, contextConfigPath string) (string, error) {
	cfg, err := loadConfig(configPath, contextConfigPath)
	if err != nil {
		return "", err
	}
	cfg, err = workspaceCasePushToTalkRoundtrip.applyConfig(cfg)
	if err != nil {
		return "", err
	}
	client, serveDone, err := dialClient(cfg)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = client.Close()
		<-serveDone
	}()
	cfg, err = ensureWorkspace(ctx, client, cfg)
	if err != nil {
		return "", err
	}
	if err := selectAndReloadAgent(ctx, client, cfg); err != nil {
		return "", fmt.Errorf("select cgo chat workspace: %w", err)
	}
	return cfg.Workspace, nil
}
