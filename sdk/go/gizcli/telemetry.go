package gizcli

import (
	"fmt"
	"time"

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
	if len(frame.GetObservations()) == 0 {
		return fmt.Errorf("gizclaw: telemetry frame observations are required")
	}
	for i, observation := range frame.GetObservations() {
		if observation == nil || observation.GetBody() == nil {
			return fmt.Errorf("gizclaw: telemetry observation %d body is required", i)
		}
	}
	conn := c.PeerConn()
	if conn == nil {
		return fmt.Errorf("gizclaw: client is not connected")
	}
	if frame.ObservedAtUnixMs == 0 {
		frame = proto.Clone(frame).(*telemetrypb.TelemetryFrame)
		frame.ObservedAtUnixMs = time.Now().UTC().UnixMilli()
	}
	payload, err := proto.Marshal(frame)
	if err != nil {
		return fmt.Errorf("gizclaw: encode telemetry frame: %w", err)
	}
	if len(payload) == 0 {
		return fmt.Errorf("gizclaw: empty telemetry frame")
	}
	if _, err := conn.Write(EventStreamTelemetry, payload); err != nil {
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

// SendOTATelemetry reports one OTA attempt state over the direct packet channel.
func (c *Client) SendOTATelemetry(observation *telemetrypb.OtaObservation) error {
	if observation == nil {
		return fmt.Errorf("gizclaw: nil ota observation")
	}
	return c.SendTelemetryFrame(&telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{{Body: &telemetrypb.Observation_Ota{Ota: observation}}},
	})
}

// SendAudioPlayerTelemetry reports the device's single player state and progress.
func (c *Client) SendAudioPlayerTelemetry(observation *telemetrypb.AudioPlayerObservation) error {
	if observation == nil {
		return fmt.Errorf("gizclaw: nil audioplayer observation")
	}
	return c.SendTelemetryFrame(&telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{{Body: &telemetrypb.Observation_Audioplayer{Audioplayer: observation}}},
	})
}
