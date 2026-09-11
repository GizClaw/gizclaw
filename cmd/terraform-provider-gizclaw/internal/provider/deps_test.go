package provider

import (
	"os/exec"
	"strings"
	"testing"
)

// TestProviderDependencyBoundary keeps the provider buildable on hosts
// without the native CLI toolchain: no CLI internals, embedded console,
// native model runtimes, or cgo in the CGO_ENABLED=0 release build.
func TestProviderDependencyBoundary(t *testing.T) {
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain is not available")
	}
	for _, platform := range []string{"darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64"} {
		goos, goarch, _ := strings.Cut(platform, "/")
		cmd := exec.Command(goBinary, "list", "-deps", "-f", "{{.ImportPath}} {{len .CgoFiles}}", "github.com/GizClaw/gizclaw-go/cmd/terraform-provider-gizclaw")
		cmd.Env = append(cmd.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch)
		// Only stdout carries package records; stderr may hold toolchain notices
		// such as module downloads.
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: go list: %v\n%s", platform, err, stderr.String())
		}
		for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
			importPath, cgoFiles, _ := strings.Cut(line, " ")
			if cgoFiles != "0" || importPath == "runtime/cgo" {
				t.Errorf("%s: %s compiles cgo files", platform, importPath)
			}
			for _, forbidden := range []string{
				"github.com/GizClaw/gizclaw-go/cmd/internal",
				"github.com/GizClaw/gizclaw-go/web/",
				"github.com/GizClaw/gizclaw-go/third_party/",
				"github.com/GizClaw/gizclaw-go/pkgs/agent/ncnn",
				"github.com/GizClaw/gizclaw-go/pkgs/audio/voiceprint",
				"github.com/yanyiwu/gojieba",
			} {
				if strings.HasPrefix(importPath, forbidden) {
					t.Errorf("%s: forbidden dependency %s", platform, importPath)
				}
			}
		}
	}
}
