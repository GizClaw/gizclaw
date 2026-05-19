package gizrun

import (
	"errors"

	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/metrics"
)

func init() {
	PostInitAt(0x21, func(ctx *RunContext) error {
		config, err := ctx.metricsPush.normalize()
		if err != nil {
			return err
		}
		if !config.Enabled {
			return nil
		}
		gatherer := metrics.Gatherer()
		if gatherer == nil {
			return errors.New("gizrun metrics push: metrics gatherer is not available")
		}
		pusher, err := newMetricsPusher(config, gatherer)
		if err != nil {
			return err
		}
		ctx.metricsPusher = pusher
		pusher.start()
		return nil
	})
}
