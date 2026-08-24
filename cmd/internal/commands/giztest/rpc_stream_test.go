package giztest

import (
	"reflect"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestBoundedBufferRejectsOverflowWithoutPartialWrite(t *testing.T) {
	buffer := &boundedBuffer{max: 3}
	if _, err := buffer.Write([]byte("ab")); err != nil {
		t.Fatal(err)
	}
	if n, err := buffer.Write([]byte("cd")); err == nil || n != 0 || buffer.String() != "ab" {
		t.Fatalf("write=(%d,%v) buffer=%q", n, err, buffer.String())
	}
}

func TestSpeedTestOperationResult(t *testing.T) {
	result := gizcli.SpeedTestResult{
		UpContentLength:   65_536,
		DownContentLength: 131_072,
		UpBytes:           1_000_000,
		DownBytes:         2_000_000,
		UpDuration:        1500 * time.Millisecond,
		DownDuration:      2 * time.Second,
		Duration:          3 * time.Second,
	}
	got := speedTestOperationResult(result)
	wantEvidence := map[string]any{
		"method":              "all.speed_test.run",
		"bytes":               int64(2_000_000),
		"up_content_length":   int64(65_536),
		"down_content_length": int64(131_072),
		"up_bytes":            int64(1_000_000),
		"down_bytes":          int64(2_000_000),
		"up_duration_ms":      int64(1500),
		"down_duration_ms":    int64(2000),
		"duration_ms":         int64(3000),
		"up_mbps":             result.UpMbps(),
		"down_mbps":           result.DownMbps(),
	}
	wantObject := map[string]any{
		"bytes":               int64(2_000_000),
		"up_content_length":   int64(65_536),
		"down_content_length": int64(131_072),
		"up_bytes":            int64(1_000_000),
		"down_bytes":          int64(2_000_000),
		"up_duration_ms":      int64(1500),
		"down_duration_ms":    int64(2000),
		"duration_ms":         int64(3000),
		"up_mbps":             result.UpMbps(),
		"down_mbps":           result.DownMbps(),
		"UpContentLength":     int64(65_536),
		"DownContentLength":   int64(131_072),
		"UpBytes":             int64(1_000_000),
		"DownBytes":           int64(2_000_000),
		"UpDuration":          int64(1500 * time.Millisecond),
		"DownDuration":        int64(2 * time.Second),
		"Duration":            int64(3 * time.Second),
	}
	if !reflect.DeepEqual(got.evidence, wantEvidence) {
		t.Fatalf("evidence = %#v, want %#v", got.evidence, wantEvidence)
	}
	if !reflect.DeepEqual(got.assertion, wantObject) {
		t.Fatalf("assertion = %#v, want %#v", got.assertion, wantObject)
	}
	if !reflect.DeepEqual(got.saved, wantObject) {
		t.Fatalf("saved = %#v, want %#v", got.saved, wantObject)
	}
}

func TestSpeedTestOperationResultKeepsZeroDirectionsNumeric(t *testing.T) {
	got := speedTestOperationResult(gizcli.SpeedTestResult{})
	object := got.assertion.(map[string]any)
	for _, key := range []string{
		"bytes",
		"up_content_length",
		"down_content_length",
		"up_bytes",
		"down_bytes",
		"up_duration_ms",
		"down_duration_ms",
		"duration_ms",
		"up_mbps",
		"down_mbps",
	} {
		value, ok := numericTarget(object[key])
		if !ok || value != 0 {
			t.Fatalf("%s = %#v, want numeric zero", key, object[key])
		}
	}
}
