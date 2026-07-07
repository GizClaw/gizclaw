package gizcli

import (
	"net"
	"strings"
	"testing"
	"time"

	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"google.golang.org/protobuf/proto"
)

func TestClientSendTelemetryFrame(t *testing.T) {
	var nilClient *Client
	if err := nilClient.SendTelemetryFrame(&telemetrypb.TelemetryFrame{}); err == nil || !strings.Contains(err.Error(), "nil client") {
		t.Fatalf("nil SendTelemetryFrame() error = %v", err)
	}
	if err := (&Client{}).SendTelemetryFrame(nil); err == nil || !strings.Contains(err.Error(), "nil telemetry frame") {
		t.Fatalf("nil frame SendTelemetryFrame() error = %v", err)
	}
	if err := (&Client{}).SendTelemetryFrame(&telemetrypb.TelemetryFrame{}); err == nil || !strings.Contains(err.Error(), "observations are required") {
		t.Fatalf("empty frame SendTelemetryFrame() error = %v", err)
	}
	if err := (&Client{}).SendTelemetryFrame(&telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{{}},
	}); err == nil || !strings.Contains(err.Error(), "observation 0 body is required") {
		t.Fatalf("missing body SendTelemetryFrame() error = %v", err)
	}
	percent := 1.0
	if err := (&Client{}).SendTelemetryFrame(&telemetrypb.TelemetryFrame{
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{Percent: &percent}},
		}},
	}); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("disconnected SendTelemetryFrame() error = %v", err)
	}

	conn := &recordingGiznetConn{}
	client := &Client{conn: conn}
	if err := client.SendBatteryTelemetry(87, true); err != nil {
		t.Fatalf("SendBatteryTelemetry() error = %v", err)
	}
	if conn.protocol != ProtocolTelemetry {
		t.Fatalf("protocol = %#x, want %#x", conn.protocol, ProtocolTelemetry)
	}
	var frame telemetrypb.TelemetryFrame
	if err := proto.Unmarshal(conn.payload, &frame); err != nil {
		t.Fatalf("decode telemetry payload: %v", err)
	}
	if frame.ObservedAtUnixMs <= 0 {
		t.Fatalf("ObservedAtUnixMs = %d, want client-side timestamp", frame.ObservedAtUnixMs)
	}
	if skew := time.Since(time.UnixMilli(frame.ObservedAtUnixMs)); skew < -time.Second || skew > time.Second {
		t.Fatalf("ObservedAtUnixMs = %d, outside current send window", frame.ObservedAtUnixMs)
	}
	if len(frame.Observations) != 1 {
		t.Fatalf("observations = %d, want 1", len(frame.Observations))
	}
	battery := frame.Observations[0].GetBattery()
	if battery == nil || battery.Percent == nil || *battery.Percent != 87 || battery.Charging == nil || !*battery.Charging {
		t.Fatalf("battery = %#v", battery)
	}

	preset := &telemetrypb.TelemetryFrame{
		ObservedAtUnixMs: 1234,
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{Percent: &percent}},
		}},
	}
	if err := client.SendTelemetryFrame(preset); err != nil {
		t.Fatalf("SendTelemetryFrame(preset) error = %v", err)
	}
	if preset.ObservedAtUnixMs != 1234 {
		t.Fatalf("SendTelemetryFrame mutated preset observed_at = %d", preset.ObservedAtUnixMs)
	}
}

type recordingGiznetConn struct {
	protocol byte
	payload  []byte
}

func (c *recordingGiznetConn) Dial(uint64) (net.Conn, error) { return nil, nil }
func (c *recordingGiznetConn) ListenService(uint64) giznet.ServiceListener {
	return nil
}
func (c *recordingGiznetConn) CloseService(uint64) error { return nil }
func (c *recordingGiznetConn) Read([]byte) (byte, int, error) {
	return 0, 0, net.ErrClosed
}
func (c *recordingGiznetConn) Write(protocol byte, payload []byte) (int, error) {
	c.protocol = protocol
	c.payload = append([]byte(nil), payload...)
	return len(payload), nil
}
func (c *recordingGiznetConn) PublicKey() giznet.PublicKey { return giznet.PublicKey{} }
func (c *recordingGiznetConn) PeerInfo() *giznet.PeerInfo  { return nil }
func (c *recordingGiznetConn) Close() error                { return nil }

var _ giznet.Conn = (*recordingGiznetConn)(nil)
