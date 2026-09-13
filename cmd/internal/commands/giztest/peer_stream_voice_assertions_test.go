package giztestcmd

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

// Feed independently calculated raw traces to the real Giztest expect evaluator.
// Each negative selects one assertion so a digest mismatch cannot mask a broken
// lifecycle, interval or prebuffer assertion.
func TestMultiRoleVoiceAssertions(t *testing.T) {
	packets := voiceTonePacketsCount(t, 300, 160)
	for _, tc := range []struct {
		name, field string
		gap         time.Duration
		fault       string
		pass        bool
	}{
		{name: "voice", field: "/audio_integrity/sha256", pass: true},
		{name: "wrong voice", field: "/audio_integrity/sha256", fault: "voice"},
		{name: "interleave", field: "/audio_integrity/max_active", fault: "interleave"},
		{name: "missing EOS", field: "/audio_integrity/open", fault: "truncate"},
		{name: "late old packet", field: "/audio_integrity/violations", fault: "late"},
		{name: "interval at limit", field: "/audio_pacing/max_interval_ms", gap: 150 * time.Millisecond, pass: true},
		{name: "interval over limit", field: "/audio_pacing/max_interval_ms", gap: 151 * time.Millisecond},
		{name: "buffer covers gap", field: "/audio_pacing/underruns", gap: 500 * time.Millisecond, pass: true},
		{name: "buffer exhausted", field: "/audio_pacing/underruns", gap: 501 * time.Millisecond},
		{name: "buffer zero", field: "/audio_pacing/minimum_buffer_ms", gap: 500 * time.Millisecond, pass: true},
		{name: "buffer negative", field: "/audio_pacing/minimum_buffer_ms", gap: 501 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var integrity peerAudioIntegrity
			var pacing peerAudioPacing
			at := time.Unix(1, 0)
			for i, packet := range packets {
				if i > 0 {
					if i == 30 && tc.gap != 0 {
						at = at.Add(tc.gap)
					} else {
						at = at.Add(20 * time.Millisecond)
					}
				}
				data := string(packet)
				if tc.fault == "voice" {
					data = "wrong"
				}
				integrity.observe(integrityChunk("a", i == 0, false, data))
				if tc.fault == "interleave" && i == 5 {
					integrity.observe(integrityChunk("b", true, false, ""))
				}
				pacing.observe(at, [][]byte{packet})
			}
			if tc.fault != "truncate" {
				integrity.observe(integrityChunk("a", false, true, ""))
			}
			if tc.fault == "interleave" {
				integrity.observe(integrityChunk("b", false, true, ""))
			}
			if tc.fault == "late" {
				integrity.observe(integrityChunk("a", false, false, "late"))
			}
			d := &voiceAssertionDriver{driver: newDriver(false, nil), value: map[string]any{"audio_integrity": integrity.summary(), "audio_pacing": pacing.summary()}}
			doc, err := giztest.LoadDocument("../../../../tests/gizclaw-e2e/testdata/eino-voices/multi-turn.giztest.yaml", d)
			if err != nil {
				t.Fatal(err)
			}
			doc.Steps = doc.Steps[:1]
			expectation := doc.Steps[0].Expect[tc.field]
			if tc.field == "/audio_integrity/sha256" {
				expectation.Equals = voicePacketDigest(packets)
			}
			doc.Steps[0].Expect = map[string]giztest.Expectation{tc.field: expectation}
			report := giztest.Run(t.Context(), []*giztest.Document{doc}, giztest.Options{Driver: d, Parallel: 1, Out: io.Discard})
			if len(report.Tasks) != 1 {
				t.Fatalf("tasks=%d", len(report.Tasks))
			}
			task := report.Tasks[0]
			if (task.Status == "passed") != tc.pass {
				t.Fatalf("pass=%v task=%+v", tc.pass, task)
			}
			if !tc.pass && !strings.Contains(task.Error, tc.field) {
				t.Fatalf("unexpected failure: %s", task.Error)
			}
		})
	}
}

type voiceAssertionDriver struct {
	*driver
	value any
}

func (d *voiceAssertionDriver) Open(context.Context, *giztest.Document, *giztest.Variables) (giztest.Session, error) {
	return d, nil
}
func (d *voiceAssertionDriver) Execute(context.Context, giztest.StepRequest) (giztest.StepResult, error) {
	return giztest.StepResult{Value: d.value}, nil
}
func (*voiceAssertionDriver) Fingerprints() map[string]string { return nil }
func (*voiceAssertionDriver) CloseStreams() error             { return nil }
func (*voiceAssertionDriver) Close()                          {}
