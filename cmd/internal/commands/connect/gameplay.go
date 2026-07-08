package connectcmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/spf13/cobra"
)

func newGameplayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gameplay",
		Short: "Use peer gameplay resources",
	}
	cmd.AddCommand(
		newGameplayRulesetCmd(),
		newPetCmd(),
		newPointsCmd(),
		newBadgeCmd(),
		newGameResultCmd(),
		newRewardGrantCmd(),
	)
	return cmd
}

func newGameplayRulesetCmd() *cobra.Command {
	var opts connectRPCOptions
	var name string
	cmd := &cobra.Command{
		Use:   "ruleset",
		Short: "Show the current gameplay ruleset",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.GetGameRuleset(ctx, "server.game_ruleset.get", rpcapi.ServerGameRulesetGetRequest{Name: optionalString(name)})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&name, "name", "", "ruleset name")
	return cmd
}

func newPetCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "pet", Short: "Use peer pets"}
	cmd.AddCommand(newPetListCmd(), newPetGetCmd(), newPetAdoptCmd(), newPetRenameCmd(), newPetDeleteCmd(), newPetDriveCmd())
	return cmd
}

func newPetListCmd() *cobra.Command {
	var opts connectRPCOptions
	var cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.ListPets(ctx, "server.pet.list", rpcapi.ServerPetListRequest{Cursor: optionalString(cursor), Limit: optionalInt(limit)})
			})
		},
	}
	opts.addFlags(cmd)
	addListFlags(cmd, &cursor, &limit)
	return cmd
}

func newPetGetCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "get <pet-id>",
		Short: "Get a pet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.GetPet(ctx, "server.pet.get", rpcapi.ServerPetGetRequest{Id: args[0]})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newPetAdoptCmd() *cobra.Command {
	var opts connectRPCOptions
	var ruleset string
	var displayName string
	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Adopt a pet",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.AdoptPet(ctx, "server.pet.adopt", rpcapi.ServerPetAdoptRequest{
					RulesetName: optionalString(ruleset),
					DisplayName: optionalString(displayName),
				})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&ruleset, "ruleset", "", "ruleset name")
	cmd.Flags().StringVar(&displayName, "name", "", "pet display name")
	return cmd
}

func newPetRenameCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "rename <pet-id> <name>",
		Short: "Rename a pet",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return err
			}
			if strings.TrimSpace(args[1]) == "" {
				return fmt.Errorf("name must not be empty")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.PutPet(ctx, "server.pet.put", rpcapi.ServerPetPutRequest{Id: args[0], DisplayName: args[1]})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newPetDeleteCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "delete <pet-id>",
		Short: "Delete a pet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.DeletePet(ctx, "server.pet.delete", rpcapi.ServerPetDeleteRequest{Id: args[0]})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newPetDriveCmd() *cobra.Command {
	var opts connectRPCOptions
	var action string
	var game string
	var score int64
	var maxScore int64
	var difficulty string
	var outcome string
	var durationMs int64
	var idempotencyKey string
	var occurredAt string
	cmd := &cobra.Command{
		Use:   "drive <pet-id>",
		Short: "Drive pet gameplay state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				req := rpcapi.ServerPetDriveRequest{PetId: args[0], Action: optionalString(action)}
				if strings.TrimSpace(game) != "" {
					var occurredAtValue *time.Time
					if strings.TrimSpace(occurredAt) != "" {
						parsed, err := time.Parse(time.RFC3339Nano, occurredAt)
						if err != nil {
							return nil, fmt.Errorf("parse --occurred-at: %w", err)
						}
						occurredAtValue = &parsed
					}
					req.GameResult = &rpcapi.PetDriveGameResultInput{
						GameDefId:      game,
						Score:          optionalInt64(score),
						MaxScore:       optionalInt64(maxScore),
						Difficulty:     optionalString(difficulty),
						Outcome:        optionalString(outcome),
						DurationMs:     optionalInt64(durationMs),
						IdempotencyKey: optionalString(idempotencyKey),
						OccurredAt:     occurredAtValue,
					}
				}
				return c.DrivePet(ctx, "server.pet.drive", req)
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&action, "action", "", "drive action")
	cmd.Flags().StringVar(&game, "game", "", "game definition id")
	cmd.Flags().Int64Var(&score, "score", 0, "game score")
	cmd.Flags().Int64Var(&maxScore, "max-score", 0, "game max score")
	cmd.Flags().StringVar(&difficulty, "difficulty", "", "game difficulty")
	cmd.Flags().StringVar(&outcome, "outcome", "", "game outcome")
	cmd.Flags().Int64Var(&durationMs, "duration-ms", 0, "game duration in milliseconds")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "game result idempotency key")
	cmd.Flags().StringVar(&occurredAt, "occurred-at", "", "game result occurrence time in RFC3339 format")
	return cmd
}

func newPointsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "points", Short: "Use gameplay points"}
	cmd.AddCommand(newPointsGetCmd(), newPointsTransactionCmd())
	return cmd
}

func newPointsGetCmd() *cobra.Command {
	var opts connectRPCOptions
	var ruleset string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get points account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.GetPoints(ctx, "server.points.get", rpcapi.ServerPointsGetRequest{RulesetName: optionalString(ruleset)})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&ruleset, "ruleset", "", "ruleset name")
	return cmd
}

func newPointsTransactionCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "transactions", Short: "Use points transactions"}
	cmd.AddCommand(newPointsTransactionListCmd(), newPointsTransactionGetCmd())
	return cmd
}

func newPointsTransactionListCmd() *cobra.Command {
	var opts connectRPCOptions
	var cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List points transactions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.ListPointsTransactions(ctx, "server.points.transactions.list", rpcapi.ServerPointsTransactionListRequest{Cursor: optionalString(cursor), Limit: optionalInt(limit)})
			})
		},
	}
	opts.addFlags(cmd)
	addListFlags(cmd, &cursor, &limit)
	return cmd
}

func newPointsTransactionGetCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "get <transaction-id>",
		Short: "Get a points transaction",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.GetPointsTransaction(ctx, "server.points.transactions.get", rpcapi.ServerPointsTransactionGetRequest{Id: args[0]})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newBadgeCmd() *cobra.Command {
	return newListGetGameplayCmd("badge", "badges", func(ctx context.Context, c *gizcli.Client, cursor string, limit int) (any, error) {
		return c.ListBadges(ctx, "server.badge.list", rpcapi.ServerBadgeListRequest{Cursor: optionalString(cursor), Limit: optionalInt(limit)})
	}, func(ctx context.Context, c *gizcli.Client, id string) (any, error) {
		return c.GetBadge(ctx, "server.badge.get", rpcapi.ServerBadgeGetRequest{Id: id})
	})
}

func newGameResultCmd() *cobra.Command {
	return newListGetGameplayCmd("game-result", "game results", func(ctx context.Context, c *gizcli.Client, cursor string, limit int) (any, error) {
		return c.ListGameResults(ctx, "server.game_result.list", rpcapi.ServerGameResultListRequest{Cursor: optionalString(cursor), Limit: optionalInt(limit)})
	}, func(ctx context.Context, c *gizcli.Client, id string) (any, error) {
		return c.GetGameResult(ctx, "server.game_result.get", rpcapi.ServerGameResultGetRequest{Id: id})
	})
}

func newRewardGrantCmd() *cobra.Command {
	return newListGetGameplayCmd("reward-grant", "reward grants", func(ctx context.Context, c *gizcli.Client, cursor string, limit int) (any, error) {
		return c.ListRewardGrants(ctx, "server.reward_grant.list", rpcapi.ServerRewardGrantListRequest{Cursor: optionalString(cursor), Limit: optionalInt(limit)})
	}, func(ctx context.Context, c *gizcli.Client, id string) (any, error) {
		return c.GetRewardGrant(ctx, "server.reward_grant.get", rpcapi.ServerRewardGrantGetRequest{Id: id})
	})
}

func newListGetGameplayCmd(name, title string, list func(context.Context, *gizcli.Client, string, int) (any, error), get func(context.Context, *gizcli.Client, string) (any, error)) *cobra.Command {
	cmd := &cobra.Command{Use: name, Short: "Use " + title}
	cmd.AddCommand(newGameplayListCmd(title, list), newGameplayGetCmd(title, get))
	return cmd
}

func newGameplayListCmd(title string, list func(context.Context, *gizcli.Client, string, int) (any, error)) *cobra.Command {
	var opts connectRPCOptions
	var cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List " + title,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return list(ctx, c, cursor, limit)
			})
		},
	}
	opts.addFlags(cmd)
	addListFlags(cmd, &cursor, &limit)
	return cmd
}

func newGameplayGetCmd(title string, get func(context.Context, *gizcli.Client, string) (any, error)) *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get " + title,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return get(ctx, c, args[0])
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func addListFlags(cmd *cobra.Command, cursor *string, limit *int) {
	cmd.Flags().StringVar(cursor, "cursor", "", "pagination cursor")
	cmd.Flags().IntVar(limit, "limit", 0, "page size")
}

func optionalInt64(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}
