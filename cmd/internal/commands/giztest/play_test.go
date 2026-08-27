package giztest

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

type blockingPlayOutput struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (f *blockingPlayOutput) Write(p []byte) (int, error) {
	f.once.Do(func() { close(f.started) })
	<-f.release
	return len(p), nil
}

func (f *blockingPlayOutput) Close() error { return nil }

type closeUnblocksPlayOutput struct {
	started   chan struct{}
	closed    chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
}

func (f *closeUnblocksPlayOutput) Write([]byte) (int, error) {
	f.startOnce.Do(func() { close(f.started) })
	<-f.closed
	return 0, io.ErrClosedPipe
}

func (f *closeUnblocksPlayOutput) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
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
	if err := session.observe("peer", "assistant", packet, false); err != nil {
		t.Fatal(err)
	}
	if err := session.observe("peer", "assistant", nil, true); err != nil {
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
	if err := session.observe("peer", "assistant", []byte{4}, false); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("observe after close error = %v", err)
	}
}

func TestPlaySessionStartsBeforeEndAfterJitterBuffer(t *testing.T) {
	decoder := &fakePlayDecoder{samples: []int16{1}}
	output := &fakePlayOutput{}
	session := &playSession{decoder: decoder, output: output}
	t.Cleanup(func() { _ = session.close() })
	packet := []byte{0xf8}
	packetCount := int(playStartBuffer / (20 * time.Millisecond))
	for range packetCount - 1 {
		if err := session.observe("peer", "assistant", packet, false); err != nil {
			t.Fatal(err)
		}
	}
	if output.Len() != 0 {
		t.Fatalf("played before jitter buffer filled: %d bytes", output.Len())
	}
	if err := session.observe("peer", "assistant", packet, false); err != nil {
		t.Fatal(err)
	}
	if err := session.syncPlayback(); err != nil {
		t.Fatal(err)
	}
	if got, want := output.Len(), packetCount*2; got != want {
		t.Fatalf("streaming bytes before EOS = %d, want %d", got, want)
	}
	if err := session.observe("peer", "assistant", packet, false); err != nil {
		t.Fatal(err)
	}
	if err := session.syncPlayback(); err != nil {
		t.Fatal(err)
	}
	if got, want := output.Len(), (packetCount+1)*2; got != want {
		t.Fatalf("next packet was not streamed immediately: bytes = %d, want %d", got, want)
	}
}

func TestPlaySessionDoesNotBlockReceiverOnPortAudio(t *testing.T) {
	output := &blockingPlayOutput{started: make(chan struct{}), release: make(chan struct{})}
	session := &playSession{decoder: &fakePlayDecoder{samples: []int16{1}}, output: output}
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(output.release) })
		_ = session.close()
	})
	packet := []byte{0xf8}
	packetCount := int(playStartBuffer / (20 * time.Millisecond))
	for range packetCount - 1 {
		if err := session.observe("peer", "assistant", packet, false); err != nil {
			t.Fatal(err)
		}
	}
	returned := make(chan error, 1)
	go func() { returned <- session.observe("peer", "assistant", packet, false) }()
	select {
	case <-output.started:
	case <-time.After(time.Second):
		t.Fatal("playback task did not start")
	}
	select {
	case err := <-returned:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("audio receiver blocked on PortAudio write")
	}
	releaseOnce.Do(func() { close(output.release) })
	if err := session.syncPlayback(); err != nil {
		t.Fatal(err)
	}
}

func TestPlaySessionCloseInterruptsBlockedPortAudioWrite(t *testing.T) {
	output := &closeUnblocksPlayOutput{started: make(chan struct{}), closed: make(chan struct{})}
	session := &playSession{decoder: &fakePlayDecoder{samples: []int16{1}}, output: output}
	packet := []byte{0xf8}
	for range int(playStartBuffer / (20 * time.Millisecond)) {
		if err := session.observe("peer", "assistant", packet, false); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-output.started:
	case <-time.After(time.Second):
		t.Fatal("playback task did not enter blocked write")
	}
	closed := make(chan error, 1)
	go func() { closed <- session.close() }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("session close did not interrupt blocked PortAudio write")
	}
}

func TestPlaySessionCuePrecedesLatencyClock(t *testing.T) {
	output := &fakePlayOutput{}
	var logs bytes.Buffer
	session := &playSession{decoder: &fakePlayDecoder{}, output: output, out: &logs}
	t.Cleanup(func() { _ = session.close() })
	if err := session.cue(); err != nil {
		t.Fatal(err)
	}
	if output.Len() == 0 || session.cueAt.IsZero() {
		t.Fatalf("cue output = %d bytes, cueAt = %v", output.Len(), session.cueAt)
	}
	if len(session.packets) != 0 || !strings.Contains(logs.String(), "started after cue") {
		t.Fatalf("cue changed recording or omitted status: packets=%d logs=%q", len(session.packets), logs.String())
	}
}

func TestPlaySessionResetsDecoderBetweenUtterances(t *testing.T) {
	originalNewDecoder := newPlayDecoderFn
	t.Cleanup(func() { newPlayDecoderFn = originalNewDecoder })
	first := &fakePlayDecoder{samples: []int16{1}}
	second := &fakePlayDecoder{samples: []int16{2}}
	created := 0
	newPlayDecoderFn = func() (playDecoder, error) {
		created++
		if created == 1 {
			return second, nil
		}
		return nil, errors.New("unexpected decoder creation")
	}
	session := &playSession{decoder: first, output: &fakePlayOutput{}}
	t.Cleanup(func() { _ = session.close() })
	if err := session.observe("tester", "assistant", []byte{1}, true); err != nil {
		t.Fatal(err)
	}
	if err := session.observe("candidate", "assistant", []byte{2}, true); err != nil {
		t.Fatal(err)
	}
	if !first.closed || !second.closed || created != 1 {
		t.Fatalf("decoder lifecycle: first_closed=%v second_closed=%v created=%d", first.closed, second.closed, created)
	}
	if len(first.packets) != 1 || len(second.packets) != 1 || first.packets[0][0] != 1 || second.packets[0][0] != 2 {
		t.Fatalf("decoder packets: first=%v second=%v", first.packets, second.packets)
	}
}

func TestPlaySessionKeepsPacketWhenPlaybackFails(t *testing.T) {
	session := &playSession{
		decoder: &fakePlayDecoder{samples: []int16{1}},
		output:  &fakePlayOutput{err: errors.New("device disconnected")},
	}
	t.Cleanup(func() { _ = session.close() })
	if err := session.observe("peer", "assistant", []byte{7, 8}, false); err != nil {
		t.Fatal(err)
	}
	if err := session.observe("peer", "assistant", nil, true); err == nil || !strings.Contains(err.Error(), "PortAudio") {
		t.Fatalf("playback error = %v", err)
	}
	if session.bytes != 2 || len(session.packets) != 1 {
		t.Fatalf("partial record = bytes %d packets %d", session.bytes, len(session.packets))
	}
}

func TestPlaySessionEnforcesAudioLimitBeforeCopy(t *testing.T) {
	session := &playSession{decoder: &fakePlayDecoder{}, output: &fakePlayOutput{}, bytes: playMaxAudioBytes}
	if err := session.observe("peer", "assistant", []byte{1}, false); err == nil || !strings.Contains(err.Error(), "fixed") {
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
	if _, err := newPlayRecord(target); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("reserved target was not protected: %v", err)
	}
	record.abort()
	if _, err := os.Stat(record.temp); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging directory survived abort: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("final directory exists after abort: %v", err)
	}
}

func TestValidatePlayOutputAllowsDisabledRecording(t *testing.T) {
	if err := validatePlayOutput(""); err != nil {
		t.Fatal(err)
	}
}

func TestPlaySessionCanDiscardRecording(t *testing.T) {
	session := &playSession{
		decoder:          &fakePlayDecoder{samples: []int16{1}},
		output:           &fakePlayOutput{},
		bytes:            playMaxAudioBytes,
		discardRecording: true,
	}
	defer func() { _ = session.close() }()
	if err := session.observe("peer", "assistant", []byte{1}, true); err != nil {
		t.Fatal(err)
	}
	if len(session.packets) != 0 || session.bytes != playMaxAudioBytes+1 {
		t.Fatalf("discarded recording retained packets=%d bytes=%d", len(session.packets), session.bytes)
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
