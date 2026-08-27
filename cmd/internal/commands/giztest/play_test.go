package giztest

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakePlayDecoder struct {
	packets [][]byte
	samples []int16
	err     error
	closed  bool
}

func (f *fakePlayDecoder) Decode(packet []byte, _ int, _ bool) ([]int16, error) {
	f.packets = append(f.packets, append([]byte(nil), packet...))
	return append([]int16(nil), f.samples...), f.err
}

func (f *fakePlayDecoder) Close() error {
	f.closed = true
	return nil
}

type fakePlayOutput struct {
	bytes.Buffer
	err    error
	closed bool
}

func TestNewPlaySessionChecksNativeRuntimesAndCleansPartialOpen(t *testing.T) {
	origOpusSupported := opusRuntimeSupportedFn
	origPortAudioSupported := portAudioRuntimeSupported
	origNewDecoder := newPlayDecoderFn
	origOpenOutput := openPlayOutputFn
	t.Cleanup(func() {
		opusRuntimeSupportedFn = origOpusSupported
		portAudioRuntimeSupported = origPortAudioSupported
		newPlayDecoderFn = origNewDecoder
		openPlayOutputFn = origOpenOutput
	})

	opusRuntimeSupportedFn = func() bool { return false }
	if _, err := newPlaySession(); err == nil || !strings.Contains(err.Error(), "Opus") {
		t.Fatalf("Opus runtime error = %v", err)
	}

	opusRuntimeSupportedFn = func() bool { return true }
	portAudioRuntimeSupported = func() bool { return false }
	if _, err := newPlaySession(); err == nil || !strings.Contains(err.Error(), "PortAudio") {
		t.Fatalf("PortAudio runtime error = %v", err)
	}

	portAudioRuntimeSupported = func() bool { return true }
	decoder := &fakePlayDecoder{}
	newPlayDecoderFn = func() (playDecoder, error) { return decoder, nil }
	openPlayOutputFn = func() (playOutput, error) { return nil, errors.New("no device") }
	if _, err := newPlaySession(); err == nil || !strings.Contains(err.Error(), "default PortAudio output") {
		t.Fatalf("output open error = %v", err)
	}
	if !decoder.closed {
		t.Fatal("decoder was not closed after output open failure")
	}
}

func (f *fakePlayOutput) Write(p []byte) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.Buffer.Write(p)
}

func (f *fakePlayOutput) Close() error {
	f.closed = true
	return nil
}

func TestPlaySessionObservesCopiesAndPlaysPackets(t *testing.T) {
	decoder := &fakePlayDecoder{samples: []int16{1, -2}}
	output := &fakePlayOutput{}
	session := &playSession{decoder: decoder, output: output}
	packet := []byte{1, 2, 3}
	if err := session.observe("peer", packet); err != nil {
		t.Fatal(err)
	}
	packet[0] = 9
	if got := session.packets[0][0]; got != 1 {
		t.Fatalf("recorded packet aliases caller input: %d", got)
	}
	if got := output.Bytes(); !bytes.Equal(got, []byte{1, 0, 0xfe, 0xff}) {
		t.Fatalf("played PCM = %v", got)
	}
	if err := session.close(); err != nil {
		t.Fatal(err)
	}
	if !decoder.closed || !output.closed {
		t.Fatal("playback resources were not closed")
	}
	if err := session.observe("peer", []byte{4}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("observe after close error = %v", err)
	}
}

func TestPlaySessionKeepsPacketWhenPlaybackFails(t *testing.T) {
	session := &playSession{
		decoder: &fakePlayDecoder{samples: []int16{1}},
		output:  &fakePlayOutput{err: errors.New("device disconnected")},
	}
	if err := session.observe("peer", []byte{7, 8}); err == nil || !strings.Contains(err.Error(), "PortAudio") {
		t.Fatalf("playback error = %v", err)
	}
	if session.bytes != 2 || len(session.packets) != 1 {
		t.Fatalf("partial record = bytes %d packets %d", session.bytes, len(session.packets))
	}
}

func TestPlaySessionEnforcesAudioLimitBeforeCopy(t *testing.T) {
	session := &playSession{decoder: &fakePlayDecoder{}, output: &fakePlayOutput{}, bytes: playMaxAudioBytes}
	if err := session.observe("peer", []byte{1}); err == nil || !strings.Contains(err.Error(), "fixed") {
		t.Fatalf("limit error = %v", err)
	}
	if len(session.packets) != 0 {
		t.Fatal("over-limit packet was retained")
	}
}

func TestWritePlayRecordCreatesReportAndOptionalAudio(t *testing.T) {
	for _, tc := range []struct {
		name      string
		packets   [][]byte
		wantAudio bool
	}{
		{name: "report only"},
		{name: "report and audio", packets: [][]byte{{1, 2, 3}}, wantAudio: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "record")
			report := Report{Version: "v1", Status: "failed", StartedAt: time.Unix(1, 0)}
			if err := writePlayRecord(output, report, tc.packets); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(output, "report.json"))
			if err != nil || !bytes.Contains(data, []byte(`"status": "failed"`)) {
				t.Fatalf("report data = %q, err = %v", data, err)
			}
			_, err = os.Stat(filepath.Join(output, "audio.ogg"))
			if tc.wantAudio && err != nil {
				t.Fatalf("audio artifact missing: %v", err)
			}
			if !tc.wantAudio && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unexpected audio artifact stat error: %v", err)
			}
			if err := writePlayRecord(output, report, nil); err == nil || !strings.Contains(err.Error(), "already exists") {
				t.Fatalf("existing record error = %v", err)
			}
		})
	}
}

func TestPlayRecordReservesWritableStagingAndAborts(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "record")
	record, err := newPlayRecord(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(record.temp); err != nil {
		t.Fatalf("staging directory missing: %v", err)
	}
	record.abort()
	if _, err := os.Stat(record.temp); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging directory survived abort: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("final directory exists after abort: %v", err)
	}
}

func TestValidatePlayDocumentRejectsConcurrentShapes(t *testing.T) {
	file := filepath.Join(t.TempDir(), "one.giztest.yaml")
	if err := os.WriteFile(file, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	valid := &Document{Repeat: 1}
	if err := validatePlayDocument(file, []*Document{valid}); err != nil {
		t.Fatal(err)
	}
	for name, docs := range map[string][]*Document{
		"multiple": {valid, valid},
		"repeat":   {{Repeat: 2}},
		"barrier":  {{Repeat: 1, Steps: []Step{{Barrier: &BarrierOperation{}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validatePlayDocument(file, docs); err == nil {
				t.Fatal("invalid play document accepted")
			}
		})
	}
	if err := validatePlayDocument(filepath.Dir(file), []*Document{valid}); err == nil || !strings.Contains(err.Error(), "regular") {
		t.Fatalf("directory error = %v", err)
	}
}

func TestMarkPlayReportFailed(t *testing.T) {
	report := Report{Status: "passed", Tasks: []TaskReport{{Status: "passed"}}}
	markPlayReportFailed(&report, errors.New("close failed"))
	if report.Status != "failed" || report.Tasks[0].Status != "failed" || report.Tasks[0].Error != "close failed" {
		t.Fatalf("report = %#v", report)
	}
}
