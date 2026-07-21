package admincmd

import (
	credentialscmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/credentials"
	dashscopetenantscmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/dashscopetenants"
	deepseektenantscmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/deepseektenants"
	firmwarescmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/firmwares"
	geminitenantscmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/geminitenants"
	minimaxtenantscmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/minimaxtenants"
	modelscmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/models"
	openaitenantscmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/openaitenants"
	peerscmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/peers"
	petdefscmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/petdefs"
	registrationtokenscmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/registrationtokens"
	runtimeprofilescmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/runtimeprofiles"
	voicescmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/voices"
	volctenantscmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/volctenants"
	workflowscmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/workflows"
	workspacescmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/workspaces"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	var ctxName string
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Admin control-plane commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.Flags().StringVar(&ctxName, "context", "", "context name (default: current)")
	cmd.AddCommand(
		newApplyCmd(&ctxName),
		newDeleteCmd(&ctxName),
		newShowCmd(&ctxName),
		peerscmd.NewCmd(),
		petdefscmd.NewCmd(),
		registrationtokenscmd.NewCmd(),
		runtimeprofilescmd.NewCmd(),
		credentialscmd.NewCmd(),
		firmwarescmd.NewCmd(),
		openaitenantscmd.NewCmd(),
		geminitenantscmd.NewCmd(),
		dashscopetenantscmd.NewCmd(),
		deepseektenantscmd.NewCmd(),
		minimaxtenantscmd.NewCmd(),
		volctenantscmd.NewCmd(),
		modelscmd.NewCmd(),
		voicescmd.NewCmd(),
		workflowscmd.NewCmd(),
		workspacescmd.NewCmd(),
	)
	return cmd
}
