//go:build gizclaw_e2e

package edge_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

const gatewaySpeedBytes = 32 * 1024 * 1024

func TestGatewaySpeedOneVersusThreeClients(t *testing.T) {
	endpoint := loadGatewayEndpoint(t, requiredEnv(t, "GIZCLAW_E2E_EDGE_ENDPOINT"))
	clients := make([]*gizcli.Client, 3)
	for i := range clients {
		clients[i] = connect(t, endpoint)
		defer clients[i].Close()
	}

	for _, tc := range []struct {
		name    string
		request rpcapi.SpeedTestRequest
	}{
		{
			name: "upload",
			request: rpcapi.SpeedTestRequest{
				UpContentLength: gatewaySpeedBytes,
			},
		},
		{
			name: "download",
			request: rpcapi.SpeedTestRequest{
				DownContentLength: gatewaySpeedBytes,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			single := measureGatewaySpeed(t, clients[:1], tc.name+"-single", tc.request)
			concurrent := measureGatewaySpeed(t, clients, tc.name+"-three", tc.request)
			t.Logf(
				"%s single=%.2f Mbps three aggregate=%.2f Mbps min=%.2f Mbps max=%.2f Mbps",
				tc.name,
				single.aggregateMbps,
				concurrent.aggregateMbps,
				concurrent.minClientMbps,
				concurrent.maxClientMbps,
			)

			minClientRatio := optionalPositiveFloat(t, "GIZCLAW_E2E_SPEED_MIN_CLIENT_RATIO")
			if minClientRatio > 0 &&
				concurrent.minClientMbps < single.aggregateMbps*minClientRatio {
				t.Fatalf(
					"minimum client throughput %.2f Mbps is below %.2fx single-client %.2f Mbps",
					concurrent.minClientMbps,
					minClientRatio,
					single.aggregateMbps,
				)
			}
			minAggregateScale := optionalPositiveFloat(t, "GIZCLAW_E2E_SPEED_MIN_AGGREGATE_SCALE")
			if minAggregateScale > 0 &&
				concurrent.aggregateMbps < single.aggregateMbps*minAggregateScale {
				t.Fatalf(
					"aggregate throughput %.2f Mbps is below %.2fx single-client %.2f Mbps",
					concurrent.aggregateMbps,
					minAggregateScale,
					single.aggregateMbps,
				)
			}
			minAggregateMbps := optionalPositiveFloat(
				t,
				"GIZCLAW_E2E_SPEED_MIN_"+strings.ToUpper(tc.name)+"_AGGREGATE_MBPS",
			)
			if minAggregateMbps > 0 && concurrent.aggregateMbps < minAggregateMbps {
				t.Fatalf(
					"aggregate throughput %.2f Mbps is below %.2f Mbps",
					concurrent.aggregateMbps,
					minAggregateMbps,
				)
			}
		})
	}
}

type gatewaySpeedMeasurement struct {
	aggregateMbps float64
	minClientMbps float64
	maxClientMbps float64
}

func measureGatewaySpeed(
	t *testing.T,
	clients []*gizcli.Client,
	id string,
	request rpcapi.SpeedTestRequest,
) gatewaySpeedMeasurement {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	start := make(chan struct{})
	results := make([]gizcli.SpeedTestResult, len(clients))
	errCh := make(chan error, len(clients))
	var wg sync.WaitGroup
	for i, client := range clients {
		wg.Go(func() {
			<-start
			result, err := client.SpeedTest(
				ctx,
				fmt.Sprintf("all.speed_test.run.%s.%d", id, i),
				request,
			)
			if err != nil {
				errCh <- fmt.Errorf("client %d: %w", i, err)
				return
			}
			results[i] = result
		})
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}

	measurement := gatewaySpeedMeasurement{}
	var totalBytes int64
	var maxDuration time.Duration
	for i, result := range results {
		var bytes int64
		var duration time.Duration
		var mbps float64
		if request.UpContentLength > 0 {
			bytes = result.UpBytes
			duration = result.UpDuration
			mbps = result.UpMbps()
		} else {
			bytes = result.DownBytes
			duration = result.DownDuration
			mbps = result.DownMbps()
		}
		if bytes <= 0 || duration <= 0 || mbps <= 0 {
			t.Fatalf("client %d speed result = %+v, want positive direction measurement", i, result)
		}
		totalBytes += bytes
		maxDuration = max(maxDuration, duration)
		if i == 0 || mbps < measurement.minClientMbps {
			measurement.minClientMbps = mbps
		}
		measurement.maxClientMbps = max(measurement.maxClientMbps, mbps)
	}
	measurement.aggregateMbps = float64(totalBytes*8) / maxDuration.Seconds() / 1_000_000
	return measurement
}

func optionalPositiveFloat(t *testing.T, name string) float64 {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		t.Fatalf("%s must be a positive number, got %q", name, raw)
	}
	return value
}
