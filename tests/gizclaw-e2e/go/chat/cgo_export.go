//go:build gizclaw_e2e

package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

// PrepareCgoPushToTalkWorkspace recreates and reloads a voice workspace using
// the same setup path as the Go chat e2e cases, then returns its workspace name.
func PrepareCgoPushToTalkWorkspace(ctx context.Context, configPath, contextConfigPath, runtimeWorkflowName, registrationToken string) (string, error) {
	return prepareCgoPushToTalkWorkspace(
		ctx,
		configPath,
		contextConfigPath,
		runtimeWorkflowName,
		registrationToken,
		"",
		true,
	)
}

// PrepareCgoPushToTalkWorkspaceNamed recreates a named voice Workspace without
// selecting it, so lifecycle tests can prepare multiple choices first.
func PrepareCgoPushToTalkWorkspaceNamed(
	ctx context.Context,
	configPath string,
	contextConfigPath string,
	runtimeWorkflowName string,
	registrationToken string,
	workspaceName string,
) (string, error) {
	return prepareCgoPushToTalkWorkspace(
		ctx,
		configPath,
		contextConfigPath,
		runtimeWorkflowName,
		registrationToken,
		workspaceName,
		false,
	)
}

// CleanupCgoPushToTalkWorkspaces stops the shared test Peer and deletes
// generated voice Workspaces after the C SDK scenario has closed its client.
func CleanupCgoPushToTalkWorkspaces(
	ctx context.Context,
	configPath string,
	contextConfigPath string,
	registrationToken string,
	workspaces ...string,
) (resultErr error) {
	cfg, err := loadConfig(configPath, contextConfigPath)
	if err != nil {
		return err
	}
	client, serveDone, err := dialClient(cfg)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, client.Close())
		<-serveDone
	}()
	if _, err := client.Register(ctx, "cgo-chat.cleanup.register", registrationToken); err != nil {
		return fmt.Errorf("register cgo chat cleanup client: %w", err)
	}
	if _, err := client.StopServerRun(ctx, "cgo-chat.cleanup.stop"); err != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("stop cgo chat cleanup Peer: %w", err))
	}
	for _, workspace := range workspaces {
		workspace = strings.TrimSpace(workspace)
		if workspace == "" {
			continue
		}
		if _, err := client.DeleteWorkspace(
			ctx,
			"cgo-chat.cleanup.delete."+workspace,
			rpcapi.WorkspaceDeleteRequest{Name: workspace},
		); err != nil && !isRPCNotFound(err) {
			resultErr = errors.Join(
				resultErr,
				fmt.Errorf("delete cgo chat Workspace %q: %w", workspace, err),
			)
		}
	}
	return resultErr
}

func prepareCgoPushToTalkWorkspace(
	ctx context.Context,
	configPath string,
	contextConfigPath string,
	runtimeWorkflowName string,
	registrationToken string,
	workspaceName string,
	selectWorkspace bool,
) (string, error) {
	cfg, err := loadConfig(configPath, contextConfigPath)
	if err != nil {
		return "", err
	}
	runtimeWorkflowName = strings.TrimSpace(runtimeWorkflowName)
	if runtimeWorkflowName == "" {
		return "", fmt.Errorf("runtime workflow alias is required")
	}
	cfg.Workflow.Name = runtimeWorkflowName
	cfg, err = workspaceCasePushToTalkRoundtrip.applyConfig(cfg)
	if err != nil {
		return "", err
	}
	if workspaceName = strings.TrimSpace(workspaceName); workspaceName != "" {
		cfg.Workspace = workspaceName
	} else if selectWorkspace {
		cfg.Workspace = fmt.Sprintf("%s-%x", cfg.Workspace, time.Now().UnixNano())
	}
	client, serveDone, err := dialClient(cfg)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = client.Close()
		<-serveDone
	}()
	if _, err := client.Register(ctx, "cgo-chat.register", registrationToken); err != nil {
		return "", fmt.Errorf("register cgo chat client: %w", err)
	}
	cfg, err = ensureWorkspace(ctx, client, cfg)
	if err != nil {
		return "", err
	}
	if selectWorkspace {
		if err := selectAndReloadAgent(ctx, client, cfg); err != nil {
			return "", fmt.Errorf("select cgo chat workspace: %w", err)
		}
	}
	return cfg.Workspace, nil
}
