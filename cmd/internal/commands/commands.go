package commands

import (
	"os"
	"strings"

	"github.com/GizClaw/gizclaw-go/cmd/internal/buildinfo"
	admincmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin"
	connectcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/connect"
	contextcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/context"
	edgecmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/edge"
	genkeycmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/genkey"
	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	servecmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/serve"
	servicecmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/service"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	root := &cobra.Command{
		Use:     "gizclaw",
		Short:   "GizClaw - peer-to-peer toy network",
		Version: buildinfo.Version,
	}
	root.SetArgs(normalizeLegacyLongFlags(os.Args[1:]))

	root.AddCommand(
		servecmd.NewCmd(),
		servicecmd.NewCmd(),
		contextcmd.NewCmd(),
		genkeycmd.NewCmd(),
		connectcmd.NewCmd(),
		admincmd.NewCmd(),
		edgecmd.NewCmd(),
		giztestcmd.NewCmd(),
	)

	return root
}

func normalizeLegacyLongFlags(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(args))
	for _, arg := range args {
		if len(arg) > 2 && strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			normalized = append(normalized, "-"+arg)
			continue
		}
		normalized = append(normalized, arg)
	}
	return normalized
}
