package giztestcmd

import (
	"encoding/json"
	"fmt"

	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"google.golang.org/protobuf/encoding/protojson"
)

func decodeTelemetryFrame(input any) (*telemetrypb.TelemetryFrame, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	frame := new(telemetrypb.TelemetryFrame)
	if err := protojson.Unmarshal(data, frame); err != nil {
		return nil, err
	}
	if len(frame.Observations) == 0 {
		return nil, fmt.Errorf("telemetry requires observations")
	}
	for _, observation := range frame.Observations {
		if observation == nil || observation.Body == nil {
			return nil, fmt.Errorf("telemetry requires observation bodies")
		}
	}
	return frame, nil
}

func (s *session) executeTelemetry(req giztest.StepRequest) (giztest.StepResult, error) {
	client, err := s.clients.get(req.Step.Client)
	if err != nil {
		return giztest.StepResult{}, err
	}
	input, err := req.Vars.Resolve(req.Step.Telemetry.Frame)
	if err != nil {
		return giztest.StepResult{}, err
	}
	frame, err := decodeTelemetryFrame(input)
	if err != nil {
		return giztest.StepResult{}, err
	}
	if err := client.SendTelemetryFrame(frame); err != nil {
		return giztest.StepResult{}, err
	}
	// Packet acceptance is not persistence: scenarios must poll status separately.
	return giztest.StepResult{Value: map[string]any{"sent": true, "observations": len(frame.Observations)}}, nil
}
