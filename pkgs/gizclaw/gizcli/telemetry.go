package gizcli

import (
	"fmt"

	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"google.golang.org/protobuf/proto"
)

// SendTelemetryFrame sends one protobuf telemetry frame over the direct packet channel.
func (c *Client) SendTelemetryFrame(frame *telemetrypb.TelemetryFrame) error {
	if c == nil {
		return fmt.Errorf("gizclaw: nil client")
	}
	if frame == nil {
		return fmt.Errorf("gizclaw: nil telemetry frame")
	}
	conn := c.PeerConn()
	if conn == nil {
		return fmt.Errorf("gizclaw: client is not connected")
	}
	payload, err := proto.Marshal(frame)
	if err != nil {
		return fmt.Errorf("gizclaw: encode telemetry frame: %w", err)
	}
	if len(payload) == 0 {
		return fmt.Errorf("gizclaw: empty telemetry frame")
	}
	if _, err := conn.Write(ProtocolTelemetry, payload); err != nil {
		return fmt.Errorf("gizclaw: send telemetry frame: %w", err)
	}
	return nil
}

// SendBatteryTelemetry reports the current battery snapshot.
func (c *Client) SendBatteryTelemetry(percent int, charging bool) error {
	percentValue := float64(percent)
	return c.SendTelemetryFrame(&telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{{
			ObservedAtDeltaMs: 0,
			Body: &telemetrypb.Observation_Battery{
				Battery: &telemetrypb.BatteryObservation{
					Percent:  &percentValue,
					Charging: &charging,
				},
			},
		}},
	})
}
