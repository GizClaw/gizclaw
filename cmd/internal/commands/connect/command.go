package connectcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/connection"
	"github.com/GizClaw/gizclaw-go/cmd/internal/deviceapi"
	"github.com/GizClaw/gizclaw-go/cmd/internal/peerapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/spf13/cobra"
)

var connectFromContext = connection.ConnectFromContext

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Connect to the GizClaw server",
	}
	cmd.AddCommand(
		newPingCmd(),
		newServerInfoCmd(),
		newSetNameCmd(),
		newRunStatusCmd(),
		newSayCmd(),
		newTestSpeedCmd(),
		newContactCmd(),
		newFriendCmd(),
		newFriendGroupCmd(),
		newFirmwareCmd(),
		newGameplayCmd(),
	)
	return cmd
}

func newPingCmd() *cobra.Command {
	var ctxName string

	cmd := &cobra.Command{
		Use:   "ping",
		Short: "Ping the server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connectFromContext(ctxName)
			if err != nil {
				return err
			}
			defer c.Close()

			t1 := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			ping, err := c.Ping(ctx, "ping")
			if err != nil {
				return err
			}
			t4 := time.Now()
			rtt := t4.Sub(t1)
			serverTime := time.UnixMilli(ping.ServerTime)
			clientMid := t1.Add(rtt / 2)
			clockDiff := serverTime.Sub(clientMid)

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Server Time: %s\n", serverTime.Format(time.RFC3339Nano))
			fmt.Fprintf(out, "RTT:         %v\n", rtt.Round(time.Microsecond))
			fmt.Fprintf(out, "Clock Diff:  %v\n", clockDiff.Round(time.Microsecond))
			return nil
		},
	}

	cmd.Flags().StringVar(&ctxName, "context", "", "context name (default: current)")
	return cmd
}

func newServerInfoCmd() *cobra.Command {
	var ctxName string
	cmd := &cobra.Command{
		Use:   "server-info",
		Short: "Show server information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connectFromContext(ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			info, err := peerapi.GetServerInfo(context.Background(), c)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(info)
		},
	}
	cmd.Flags().StringVar(&ctxName, "context", "", "context name (default: current)")
	return cmd
}

func newSetNameCmd() *cobra.Command {
	var ctxName string
	cmd := &cobra.Command{
		Use:   "set-name <name>",
		Short: "Set current device name",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			if strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("device name must not be empty")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connectFromContext(ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			info, err := deviceapi.SetName(context.Background(), c, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(info)
		},
	}
	cmd.Flags().StringVar(&ctxName, "context", "", "context name (default: current)")
	return cmd
}

func newRunStatusCmd() *cobra.Command {
	var opts connectRPCOptions

	cmd := &cobra.Command{
		Use:   "run-status",
		Short: "Show server run status for this connection",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.GetServerRunStatus(ctx, "server.run.status", rpcapi.ServerGetRunStatusRequest{})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newSayCmd() *cobra.Command {
	var opts connectRPCOptions
	var voiceAlias string

	cmd := &cobra.Command{
		Use:   "say --voice <voice-alias> <text>",
		Short: "Ask the server to synthesize speech for this connection",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
				return err
			}
			if strings.TrimSpace(strings.Join(args, " ")) == "" {
				return fmt.Errorf("text must not be empty")
			}
			if strings.TrimSpace(voiceAlias) == "" {
				return fmt.Errorf("voice alias is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.ServerRunSay(ctx, "server.run.say", rpcapi.ServerRunSayRequest{
					Text:       strings.Join(args, " "),
					VoiceAlias: strings.TrimSpace(voiceAlias),
				})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&voiceAlias, "voice", "", "RuntimeProfile voice alias")
	return cmd
}

func newTestSpeedCmd() *cobra.Command {
	var ctxName string
	var upContentLength int64 = 10 * 1024 * 1024
	var downContentLength int64 = 10 * 1024 * 1024
	var timeout time.Duration = 30 * time.Second

	cmd := &cobra.Command{
		Use:   "test-speed",
		Short: "Measure concurrent upload and download throughput",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connectFromContext(ctxName)
			if err != nil {
				return err
			}
			defer c.Close()

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			result, err := c.SpeedTest(ctx, "all.speed_test.run", rpcapi.SpeedTestRequest{
				UpContentLength:   upContentLength,
				DownContentLength: downContentLength,
			})
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Up Bytes:     %d\n", result.UpBytes)
			fmt.Fprintf(out, "Down Bytes:   %d\n", result.DownBytes)
			fmt.Fprintf(out, "Duration:     %v\n", result.Duration.Round(time.Millisecond))
			fmt.Fprintf(out, "Up Speed:     %.2f Mbps\n", result.UpMbps())
			fmt.Fprintf(out, "Down Speed:   %.2f Mbps\n", result.DownMbps())
			return nil
		},
	}
	cmd.Flags().StringVar(&ctxName, "context", "", "context name (default: current)")
	cmd.Flags().Int64Var(&upContentLength, "up-content-length", upContentLength, "upload byte count")
	cmd.Flags().Int64Var(&downContentLength, "down-content-length", downContentLength, "download byte count")
	cmd.Flags().DurationVar(&timeout, "timeout", timeout, "speed test timeout")
	return cmd
}

type connectRPCOptions struct {
	contextName       string
	registrationToken string
	timeout           time.Duration
}

func (o *connectRPCOptions) addFlags(cmd *cobra.Command) {
	o.timeout = 30 * time.Second
	cmd.Flags().StringVar(&o.contextName, "context", "", "context name (default: current)")
	cmd.Flags().StringVar(&o.registrationToken, "registration-token", "", "register this connection with a pre-provisioned token")
	cmd.Flags().DurationVar(&o.timeout, "timeout", o.timeout, "RPC timeout")
}

func runConnectJSON(cmd *cobra.Command, opts connectRPCOptions, run func(context.Context, *gizcli.Client) (any, error)) error {
	c, err := connectFromContext(opts.contextName)
	if err != nil {
		return err
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()
	if err := registerConnectClient(ctx, c, opts); err != nil {
		return err
	}
	result, err := run(ctx, c)
	if err != nil {
		return err
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
}

func registerConnectClient(ctx context.Context, client *gizcli.Client, opts connectRPCOptions) error {
	token := strings.TrimSpace(opts.registrationToken)
	if token == "" {
		return nil
	}
	_, err := client.Register(ctx, "server.register", token)
	return err
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalInt(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func nonEmptyFlag(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be empty", name)
	}
	return nil
}

func firmwareChannelFlag(value string) (rpcapi.FirmwareChannelName, error) {
	channel := rpcapi.FirmwareChannelName(strings.TrimSpace(value))
	if !channel.Valid() {
		return "", fmt.Errorf("channel must be one of stable, beta, develop, pending")
	}
	return channel, nil
}

func newFirmwareCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "firmware",
		Short: "Inspect and download firmware through server RPC",
	}
	cmd.AddCommand(
		newFirmwareListCmd(),
		newFirmwareGetCmd(),
		newFirmwareDownloadCmd(),
	)
	return cmd
}

func newFirmwareListCmd() *cobra.Command {
	var opts connectRPCOptions
	var cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List firmware release lines readable by this peer",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.ListFirmwares(ctx, "firmware.list", rpcapi.FirmwareListRequest{
					Cursor: optionalString(cursor),
					Limit:  optionalInt(limit),
				})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	cmd.Flags().IntVar(&limit, "limit", 0, "page size")
	return cmd
}

func newFirmwareGetCmd() *cobra.Command {
	var opts connectRPCOptions
	var firmwareID string
	cmd := &cobra.Command{
		Use:   "get --firmware-id <id>",
		Short: "Get a firmware release-line document",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return err
			}
			return nonEmptyFlag("firmware-id", firmwareID)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.GetFirmware(ctx, "firmware.get", rpcapi.FirmwareGetRequest{
					FirmwareId: strings.TrimSpace(firmwareID),
				})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&firmwareID, "firmware-id", "", "firmware id")
	return cmd
}

func newFirmwareDownloadCmd() *cobra.Command {
	var opts connectRPCOptions
	var firmwareID string
	var channelValue string
	var artifactPath string
	var output string
	cmd := &cobra.Command{
		Use:   "download --firmware-id <id> --channel <channel> --path <artifact-path> --output <file>",
		Short: "Download a firmware artifact file",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return err
			}
			for name, value := range map[string]string{
				"firmware-id": firmwareID,
				"path":        artifactPath,
				"output":      output,
			} {
				if err := nonEmptyFlag(name, value); err != nil {
					return err
				}
			}
			if _, err := firmwareChannelFlag(channelValue); err != nil {
				return err
			}
			if strings.TrimSpace(output) == "-" {
				return fmt.Errorf("output must be a file path")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connectFromContext(opts.contextName)
			if err != nil {
				return err
			}
			defer c.Close()
			ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
			defer cancel()
			if err := registerConnectClient(ctx, c, opts); err != nil {
				return err
			}
			channel, err := firmwareChannelFlag(channelValue)
			if err != nil {
				return err
			}
			out, err := os.Create(strings.TrimSpace(output))
			if err != nil {
				return err
			}
			result, err := c.DownloadFirmware(ctx, "firmware.files.download", rpcapi.FirmwareFilesDownloadRequest{
				FirmwareId: strings.TrimSpace(firmwareID),
				Channel:    channel,
				Path:       strings.TrimSpace(artifactPath),
			}, out)
			closeErr := out.Close()
			if err != nil {
				_ = os.Remove(strings.TrimSpace(output))
				return err
			}
			if closeErr != nil {
				_ = os.Remove(strings.TrimSpace(output))
				return closeErr
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
				Metadata rpcapi.FirmwareFilesDownloadResponse `json:"metadata"`
				Bytes    int64                                `json:"bytes"`
				Output   string                               `json:"output"`
			}{
				Metadata: result.Metadata,
				Bytes:    result.Bytes,
				Output:   strings.TrimSpace(output),
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&firmwareID, "firmware-id", "", "firmware id")
	cmd.Flags().StringVar(&channelValue, "channel", "stable", "firmware channel")
	cmd.Flags().StringVar(&artifactPath, "path", "", "artifact file path")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file")
	return cmd
}

func newContactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contact",
		Short: "Manage external contacts through server RPC",
	}
	cmd.AddCommand(newContactListCmd(), newContactGetCmd(), newContactCreateCmd(), newContactPutCmd(), newContactDeleteCmd())
	return cmd
}

func newContactListCmd() *cobra.Command {
	var opts connectRPCOptions
	var cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List contacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.ListContacts(ctx, "contact.list", rpcapi.ContactListRequest{Cursor: optionalString(cursor), Limit: optionalInt(limit)})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of contacts to return")
	return cmd
}

func newContactGetCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "get <contact-id>",
		Short: "Get a contact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.GetContact(ctx, "contact.get", rpcapi.ContactGetRequest{Id: args[0]})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newContactCreateCmd() *cobra.Command {
	var opts connectRPCOptions
	var displayName string
	var phoneNumber string
	cmd := &cobra.Command{
		Use:   "create [--display-name <name>] [--phone-number <phone>]",
		Short: "Create a contact",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.NoArgs(cmd, args); err != nil {
				return err
			}
			if strings.TrimSpace(displayName) == "" && strings.TrimSpace(phoneNumber) == "" {
				return fmt.Errorf("display-name or phone-number is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.CreateContact(ctx, "contact.create", rpcapi.ContactCreateRequest{
					DisplayName: optionalString(displayName),
					PhoneNumber: optionalString(phoneNumber),
				})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&displayName, "display-name", "", "contact display name")
	cmd.Flags().StringVar(&phoneNumber, "phone-number", "", "contact phone number")
	return cmd
}

func newContactPutCmd() *cobra.Command {
	var opts connectRPCOptions
	var displayName string
	var phoneNumber string
	cmd := &cobra.Command{
		Use:   "put <contact-id> [--display-name <name>] [--phone-number <phone>]",
		Short: "Update a contact",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			if strings.TrimSpace(displayName) == "" && strings.TrimSpace(phoneNumber) == "" {
				return fmt.Errorf("display-name or phone-number is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.PutContact(ctx, "contact.put", rpcapi.ContactPutRequest{
					Id:          args[0],
					DisplayName: optionalString(displayName),
					PhoneNumber: optionalString(phoneNumber),
				})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&displayName, "display-name", "", "contact display name")
	cmd.Flags().StringVar(&phoneNumber, "phone-number", "", "contact phone number")
	return cmd
}

func newContactDeleteCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "delete <contact-id>",
		Short: "Delete a contact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.DeleteContact(ctx, "contact.delete", rpcapi.ContactDeleteRequest{Id: args[0]})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newFriendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "friend",
		Short: "Manage friends through server RPC",
	}
	cmd.AddCommand(newFriendListCmd(), newFriendAddCmd(), newFriendDeleteCmd(), newFriendInviteTokenCmd())
	return cmd
}

func newFriendListCmd() *cobra.Command {
	var opts connectRPCOptions
	var cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List friends",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.ListFriends(ctx, "friend.list", rpcapi.FriendListRequest{Cursor: optionalString(cursor), Limit: optionalInt(limit)})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of friends to return")
	return cmd
}

func newFriendDeleteCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "delete <friend-relation-id>",
		Short: "Delete a friend relation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.DeleteFriend(ctx, "friend.delete", rpcapi.FriendDeleteRequest{Id: args[0]})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newFriendAddCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "add <invite-token>",
		Short: "Add a friend by invite token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.AddFriend(ctx, "friend.add", rpcapi.FriendAddRequest{InviteToken: strings.TrimSpace(args[0])})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newFriendInviteTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "invite-token",
		Short: "Manage this peer's friend invite token",
	}
	cmd.AddCommand(newFriendInviteTokenGetCmd(), newFriendInviteTokenCreateCmd(), newFriendInviteTokenClearCmd())
	return cmd
}

func newFriendInviteTokenGetCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get the active friend invite token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.GetFriendInviteToken(ctx, "friend.invite_token.get", rpcapi.FriendInviteTokenGetRequest{})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newFriendInviteTokenCreateCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create or refresh the friend invite token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.CreateFriendInviteToken(ctx, "friend.invite_token.create", rpcapi.FriendInviteTokenCreateRequest{})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newFriendInviteTokenClearCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear the active friend invite token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.ClearFriendInviteToken(ctx, "friend.invite_token.clear", rpcapi.FriendInviteTokenClearRequest{})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newFriendGroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "friend-group",
		Short: "Manage friend groups through server RPC",
	}
	cmd.AddCommand(newFriendGroupListCmd(), newFriendGroupGetCmd(), newFriendGroupCreateCmd(), newFriendGroupPutCmd(), newFriendGroupDeleteCmd(), newFriendGroupJoinCmd(), newFriendGroupInviteTokenCmd(), newFriendGroupMembersCmd(), newFriendGroupMessagesCmd())
	return cmd
}

func newFriendGroupListCmd() *cobra.Command {
	var opts connectRPCOptions
	var cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List friend groups",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.ListFriendGroups(ctx, "friend_group.list", rpcapi.FriendGroupListRequest{Cursor: optionalString(cursor), Limit: optionalInt(limit)})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of friend groups to return")
	return cmd
}

func newFriendGroupGetCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "get <friend-group-id>",
		Short: "Get a friend group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.GetFriendGroup(ctx, "friend_group.get", rpcapi.FriendGroupGetRequest{Id: args[0]})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newFriendGroupCreateCmd() *cobra.Command {
	var opts connectRPCOptions
	var description string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a friend group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.CreateFriendGroup(ctx, "friend_group.create", rpcapi.FriendGroupCreateRequest{Name: args[0], Description: optionalString(description)})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&description, "description", "", "friend group description")
	return cmd
}

func newFriendGroupPutCmd() *cobra.Command {
	var opts connectRPCOptions
	var name string
	var description string
	cmd := &cobra.Command{
		Use:   "put <friend-group-id> [--name <name>] [--description <text>]",
		Short: "Update a friend group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.PutFriendGroup(ctx, "friend_group.put", rpcapi.FriendGroupPutRequest{Id: args[0], Name: optionalString(name), Description: optionalString(description)})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&name, "name", "", "friend group name")
	cmd.Flags().StringVar(&description, "description", "", "friend group description")
	return cmd
}

func newFriendGroupDeleteCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "delete <friend-group-id>",
		Short: "Delete a friend group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.DeleteFriendGroup(ctx, "friend_group.delete", rpcapi.FriendGroupDeleteRequest{Id: args[0]})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newFriendGroupJoinCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "join <invite-token>",
		Short: "Join a friend group by invite token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.JoinFriendGroup(ctx, "friend_group.join", rpcapi.FriendGroupJoinRequest{InviteToken: strings.TrimSpace(args[0])})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newFriendGroupInviteTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "invite-token",
		Short: "Manage a friend group's invite token",
	}
	cmd.AddCommand(newFriendGroupInviteTokenGetCmd(), newFriendGroupInviteTokenCreateCmd(), newFriendGroupInviteTokenClearCmd())
	return cmd
}

func newFriendGroupInviteTokenGetCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "get <friend-group-id>",
		Short: "Get the active friend group invite token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.GetFriendGroupInviteToken(ctx, "friend_group.invite_token.get", rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupId: args[0]})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newFriendGroupInviteTokenCreateCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "create <friend-group-id>",
		Short: "Create or refresh a friend group invite token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.CreateFriendGroupInviteToken(ctx, "friend_group.invite_token.create", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupId: args[0]})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newFriendGroupInviteTokenClearCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "clear <friend-group-id>",
		Short: "Clear a friend group invite token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.ClearFriendGroupInviteToken(ctx, "friend_group.invite_token.clear", rpcapi.FriendGroupInviteTokenClearRequest{FriendGroupId: args[0]})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newFriendGroupMembersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "members",
		Short: "Manage friend group members",
	}
	cmd.AddCommand(newFriendGroupMembersListCmd(), newFriendGroupMembersAddCmd(), newFriendGroupMembersPutCmd(), newFriendGroupMembersDeleteCmd())
	return cmd
}

func newFriendGroupMembersListCmd() *cobra.Command {
	var opts connectRPCOptions
	var cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "list <friend-group-id>",
		Short: "List friend group members",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.ListFriendGroupMembers(ctx, "friend_group.members.list", rpcapi.FriendGroupMemberListRequest{FriendGroupId: &args[0], Cursor: optionalString(cursor), Limit: optionalInt(limit)})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of members to return")
	return cmd
}

func newFriendGroupMembersAddCmd() *cobra.Command {
	var opts connectRPCOptions
	var role string
	cmd := &cobra.Command{
		Use:   "add <friend-group-id> <peer-public-key> --role <member|admin>",
		Short: "Add a friend group member",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return err
			}
			return nonEmptyFlag("role", role)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.AddFriendGroupMember(ctx, "friend_group.members.add", rpcapi.FriendGroupMemberAddRequest{FriendGroupId: args[0], PeerPublicKey: args[1], Role: rpcapi.FriendGroupMemberMutableRole(strings.TrimSpace(role))})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&role, "role", "", "member role")
	return cmd
}

func newFriendGroupMembersPutCmd() *cobra.Command {
	var opts connectRPCOptions
	var role string
	cmd := &cobra.Command{
		Use:   "put <friend-group-id> <peer-public-key> --role <member|admin>",
		Short: "Update a friend group member role",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return err
			}
			return nonEmptyFlag("role", role)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.PutFriendGroupMember(ctx, "friend_group.members.put", rpcapi.FriendGroupMemberPutRequest{FriendGroupId: args[0], Id: args[1], Role: rpcapi.FriendGroupMemberMutableRole(strings.TrimSpace(role))})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&role, "role", "", "member role")
	return cmd
}

func newFriendGroupMembersDeleteCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:   "delete <friend-group-id> <peer-public-key>",
		Short: "Delete a friend group member",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.DeleteFriendGroupMember(ctx, "friend_group.members.delete", rpcapi.FriendGroupMemberDeleteRequest{FriendGroupId: args[0], Id: args[1]})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newFriendGroupMessagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:        "messages",
		Short:      "Manage friend group audio messages",
		Deprecated: "use active workspace runtime and workspace history instead",
	}
	cmd.AddCommand(newFriendGroupMessagesListCmd(), newFriendGroupMessagesGetCmd(), newFriendGroupMessagesSendCmd())
	return cmd
}

func newFriendGroupMessagesListCmd() *cobra.Command {
	var opts connectRPCOptions
	var cursor string
	var limit int
	cmd := &cobra.Command{
		Use:        "list <friend-group-id>",
		Short:      "List friend group messages",
		Deprecated: "use workspace history list/get instead",
		Args:       cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.ListFriendGroupMessages(ctx, "friend_group.messages.list", rpcapi.FriendGroupMessageListRequest{FriendGroupId: &args[0], Cursor: optionalString(cursor), Limit: optionalInt(limit)})
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of messages to return")
	return cmd
}

func newFriendGroupMessagesGetCmd() *cobra.Command {
	var opts connectRPCOptions
	cmd := &cobra.Command{
		Use:        "get <friend-group-id> <message-id>",
		Short:      "Get friend group message metadata",
		Deprecated: "use workspace history get/audio.get instead",
		Args:       cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				return c.GetFriendGroupMessage(ctx, "friend_group.messages.get", rpcapi.FriendGroupMessageGetRequest{FriendGroupId: args[0], Id: args[1]})
			})
		},
	}
	opts.addFlags(cmd)
	return cmd
}

func newFriendGroupMessagesSendCmd() *cobra.Command {
	var opts connectRPCOptions
	var audioFile string
	var contentType string
	var ttlSeconds int
	cmd := &cobra.Command{
		Use:        "send <friend-group-id> --audio-file <path>",
		Short:      "Send a friend group audio message",
		Deprecated: "send through the active workspace runtime instead",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			return nonEmptyFlag("audio-file", audioFile)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			audio, err := os.ReadFile(audioFile)
			if err != nil {
				return err
			}
			return runConnectJSON(cmd, opts, func(ctx context.Context, c *gizcli.Client) (any, error) {
				req := rpcapi.FriendGroupMessageSendRequest{FriendGroupId: args[0], AudioBase64: audio, AudioContentType: strings.TrimSpace(contentType)}
				if ttlSeconds > 0 {
					req.TtlSeconds = &ttlSeconds
				}
				return c.SendFriendGroupMessage(ctx, "friend_group.messages.send", req)
			})
		},
	}
	opts.addFlags(cmd)
	cmd.Flags().StringVar(&audioFile, "audio-file", "", "audio file to upload")
	cmd.Flags().StringVar(&contentType, "content-type", "audio/opus", "audio content type")
	cmd.Flags().IntVar(&ttlSeconds, "ttl-seconds", 0, "message ttl in seconds")
	return cmd
}

func stringPtr(value string) *string {
	return &value
}
