//go:build gizclaw_e2e

package admissiondocker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/admissiontest"
)

func TestDockerAdmission(t *testing.T) {
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	cli := os.Getenv("GIZCLAW_ADMISSION_GO_RUNNER")
	endpoint := os.Getenv("GIZCLAW_E2E_SERVER_ENDPOINT")
	if cli == "" || endpoint == "" {
		t.Fatal("run run_admission_docker_tests.sh: Docker endpoint and CLI are required")
	}
	admin := func(ctx context.Context, body any, result any, args ...string) error {
		var input bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&input).Encode(body); err != nil {
				return err
			}
		}
		command := exec.CommandContext(ctx, cli, append([]string{"admin"}, append(args, "--context", "admin")...)...)
		command.Stdin = &input
		var diagnostic bytes.Buffer
		command.Stderr = &diagnostic
		output, err := command.Output()
		if err != nil {
			return fmt.Errorf("admin %s: %w\n%s", args[0], err, diagnostic.Bytes())
		}
		if result != nil {
			return json.Unmarshal(output, result)
		}
		return nil
	}
	for _, runner := range []struct {
		name    string
		command []string
	}{
		{"Go", []string{cli, "test"}},
		{"JavaScript", []string{"node", "--experimental-strip-types", filepath.Join(root, "tests/gizclaw-e2e/js/giztest/index.ts")}},
		{"C", []string{os.Getenv("GIZCLAW_ADMISSION_C_RUNNER"), "test"}},
		{"Flutter", []string{os.Getenv("GIZCLAW_ADMISSION_FLUTTER_RUNNER")}},
	} {
		t.Run(runner.name, func(t *testing.T) {
			if runner.command[0] == "" {
				t.Fatal("every admission runner is required")
			}
			admissiontest.Run(t, admissiontest.Host{
				PublicEndpoint: "http://" + os.Getenv("GIZCLAW_E2E_EDGE_ENDPOINT"),
				Endpoint:       "http://" + endpoint, Documents: filepath.Join(root, "tests/gizclaw-e2e/testdata/admission"),
				Run: func(ctx context.Context, file, report string) ([]byte, error) {
					args := append(append([]string{}, runner.command[1:]...), "run", file, "--parallel", "1", "--output", report)
					command := exec.CommandContext(ctx, runner.command[0], args...)
					command.Dir = root
					return command.CombinedOutput()
				},
				CreateProfile: func(ctx context.Context, body adminhttp.RuntimeProfileUpsert) error {
					return admin(ctx, body, nil, "runtime-profiles", "create", "-f", "-")
				},
				CreateToken: func(ctx context.Context, body adminhttp.RegistrationTokenUpsert) error {
					return admin(ctx, body, nil, "registration-tokens", "create", "-f", "-")
				},
				GetToken: func(ctx context.Context, id string) (apitypes.RegistrationToken, error) {
					var item apitypes.RegistrationToken
					err := admin(ctx, nil, &item, "registration-tokens", "get", id)
					return item, err
				},
				Block: func(ctx context.Context, key string) error { return admin(ctx, nil, nil, "peers", "block", key) },
				Approve: func(ctx context.Context, key string) error {
					return admin(ctx, nil, nil, "peers", "approve", key, "client")
				},
				GetPeer: func(ctx context.Context, key string) (apitypes.Registration, error) {
					var item apitypes.Registration
					err := admin(ctx, nil, &item, "peers", "get", key)
					return item, err
				},
				Runtime: func(ctx context.Context, key string) (apitypes.Runtime, error) {
					var item apitypes.Runtime
					err := admin(ctx, nil, &item, "peers", "runtime", key)
					return item, err
				},
			})
		})
	}
}
