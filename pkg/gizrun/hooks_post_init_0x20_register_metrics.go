package gizrun

import "github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/metrics"

func init() {
	PostInitAt(0x20, func(*RunContext) error {
		if _, err := metrics.RegisterCounter(debugRequestsTotal,
			httpMethod,
			httpPath,
			httpHost,
			httpStatusCode,
		); err != nil {
			return err
		}
		if _, err := metrics.RegisterHistogram(debugRequestDurationSeconds,
			httpMethod,
			httpPath,
			httpHost,
			httpStatusCode,
		); err != nil {
			return err
		}
		if _, err := metrics.RegisterCounter(httpRequestsTotal,
			httpMethod,
			httpPath,
			httpHost,
			httpStatusCode,
		); err != nil {
			return err
		}
		if _, err := metrics.RegisterHistogram(httpRequestDurationSeconds,
			httpMethod,
			httpPath,
			httpHost,
			httpStatusCode,
		); err != nil {
			return err
		}
		return nil
	})
}
