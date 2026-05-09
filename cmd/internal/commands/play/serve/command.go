package playservecmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/cmd/internal/client"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	var ctxName string
	var name string
	var manufacturer string
	var model string
	var hardwareRevision string
	var sn string
	var depot string
	var firmwareSemver string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Connect and serve reverse device API",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, serverPK, serverAddr, err := client.DialFromContext(ctxName)
			if err != nil {
				return err
			}
			defer c.Close()

			c.Device = apitypes.DeviceInfo{
				Name: optionalString(name),
				Sn:   optionalString(sn),
				Hardware: &apitypes.HardwareInfo{
					Manufacturer:     optionalString(manufacturer),
					Model:            optionalString(model),
					HardwareRevision: optionalString(hardwareRevision),
					Depot:            optionalString(depot),
					FirmwareSemver:   optionalString(firmwareSemver),
				},
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			go func() {
				<-ctx.Done()
				_ = c.Close()
			}()
			if err := c.Dial(serverPK, serverAddr); err != nil {
				return err
			}
			return c.Serve()
		},
	}
	cmd.Flags().StringVar(&ctxName, "context", "", "context name (default: current)")
	cmd.Flags().StringVar(&name, "name", "", "device name")
	cmd.Flags().StringVar(&manufacturer, "manufacturer", "", "manufacturer")
	cmd.Flags().StringVar(&model, "model", "", "model")
	cmd.Flags().StringVar(&hardwareRevision, "hardware-revision", "", "hardware revision")
	cmd.Flags().StringVar(&sn, "sn", "", "serial number")
	cmd.Flags().StringVar(&depot, "depot", "", "firmware depot")
	cmd.Flags().StringVar(&firmwareSemver, "firmware-semver", "", "firmware semver")
	return cmd
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
