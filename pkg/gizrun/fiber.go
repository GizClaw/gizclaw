package gizrun

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/metrics"
	"github.com/gofiber/fiber/v2"
)

const (
	debugRequestsTotal          = "debug_requests_total"
	debugRequestDurationSeconds = "debug_request_duration_seconds"
	httpRequestsTotal           = "http_requests_total"
	httpRequestDurationSeconds  = "http_request_duration_seconds"
)

func newDebugFiberApp(config ...fiber.Config) *fiber.App {
	app := fiber.New(config...)
	app.Use(httpMiddleware(debugRequestsTotal, debugRequestDurationSeconds))
	return app
}

func newFiberApp(config ...fiber.Config) *fiber.App {
	app := fiber.New(config...)
	app.Use(httpMiddleware(httpRequestsTotal, httpRequestDurationSeconds))
	return app
}

func httpMiddleware(counterName, histogramName string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		ctx := c.UserContext()
		if ctx == nil {
			ctx = context.Background()
		}
		ctx = tagFiberRequest(ctx, c)
		c.SetUserContext(ctx)

		err := c.Next()
		ctx = tagFiberResponse(ctx, c, err)
		if labels, ok := httpLabels(ctx); ok {
			duration := time.Since(start)
			slog.Info("http request", labels.Attr(), "duration", duration)
			counter := metrics.Counter(counterName)
			histogram := metrics.Histogram(histogramName)
			if counter != nil && histogram != nil {
				promLabels := labels.PrometheusLabels()
				counter.With(promLabels).Inc()
				histogram.With(promLabels).Observe(duration.Seconds())
			}
		}
		return err
	}
}

func tagFiberRequest(ctx context.Context, c *fiber.Ctx) context.Context {
	return tagHTTP(ctx,
		httpMethod, c.Method(),
		httpPath, c.Path(),
		httpHost, c.Hostname(),
	)
}

func tagFiberResponse(ctx context.Context, c *fiber.Ctx, err error) context.Context {
	statusCode := c.Response().StatusCode()
	if err != nil {
		if fiberErr, ok := err.(*fiber.Error); ok {
			statusCode = fiberErr.Code
		} else {
			statusCode = fiber.StatusInternalServerError
		}
	}
	if statusCode == 0 {
		statusCode = fiber.StatusOK
	}
	return tagHTTP(ctx,
		httpStatusCode, strconv.Itoa(statusCode),
	)
}
