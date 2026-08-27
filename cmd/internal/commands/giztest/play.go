package giztest

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/portaudio"
)

const playMaxAudioBytes = 16 << 20

type playDecoder interface {
	Decode(packet []byte, frameSize int, fec bool) ([]int16, error)
	Close() error
}

type playOutput interface {
	Write([]byte) (int, error)
	Close() error
}

var (
	opusRuntimeSupportedFn    = opus.IsRuntimeSupported
	portAudioRuntimeSupported = portaudio.NativeRuntimeSupported
	newPlayDecoderFn          = func() (playDecoder, error) { return opus.NewDecoder(16000, 1) }
	openPlayOutputFn          = func() (playOutput, error) {
		return portaudio.OpenPlayback(pcm.L16Mono16K, portaudio.PlaybackOptions{})
	}
)

type playSession struct {
	decoder playDecoder
	output  playOutput
	packets [][]byte
	bytes   int
	closed  atomic.Bool
}

func newPlaySession() (*playSession, error) {
	if !opusRuntimeSupportedFn() {
		return nil, fmt.Errorf("Opus playback is unavailable on this runtime; rebuild for a supported platform with CGO_ENABLED=1")
	}
	if !portAudioRuntimeSupported() {
		return nil, fmt.Errorf("PortAudio backend %q is unavailable on this runtime; rebuild for a supported platform with CGO_ENABLED=1", portaudio.BackendName())
	}
	decoder, err := newPlayDecoderFn()
	if err != nil {
		return nil, fmt.Errorf("create Opus playback decoder: %w", err)
	}
	output, err := openPlayOutputFn()
	if err != nil {
		_ = decoder.Close()
		return nil, fmt.Errorf("open default PortAudio output: %w", err)
	}
	return &playSession{decoder: decoder, output: output}, nil
}

func (s *playSession) observe(_, _ string, packets [][]byte) error {
	if s == nil || s.closed.Load() {
		return errors.New("play session is closed")
	}
	if len(packets) == 0 {
		return nil
	}
	utteranceBytes := 0
	for _, packet := range packets {
		if utteranceBytes > playMaxAudioBytes-len(packet) {
			return fmt.Errorf("play audio exceeds fixed %d-byte limit", playMaxAudioBytes)
		}
		utteranceBytes += len(packet)
	}
	if s.bytes > playMaxAudioBytes-utteranceBytes {
		return fmt.Errorf("play audio exceeds fixed %d-byte limit", playMaxAudioBytes)
	}
	var pcmAudio bytes.Buffer
	const maxFrameSize = 16000 * 3 / 25
	for _, packet := range packets {
		copyOfPacket := append([]byte(nil), packet...)
		s.packets = append(s.packets, copyOfPacket)
		s.bytes += len(copyOfPacket)
		samples, err := s.decoder.Decode(packet, maxFrameSize, false)
		if err != nil {
			return fmt.Errorf("decode Opus packet: %w", err)
		}
		for _, sample := range samples {
			if err := binary.Write(&pcmAudio, binary.LittleEndian, sample); err != nil {
				return fmt.Errorf("buffer decoded PCM: %w", err)
			}
		}
	}
	if _, err := io.Copy(s.output, &pcmAudio); err != nil {
		return fmt.Errorf("write buffered PCM to PortAudio: %w", err)
	}
	return nil
}

func (s *playSession) close() error {
	if s == nil || !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	return errors.Join(s.output.Close(), s.decoder.Close())
}

func validatePlayDocument(path string, docs []*Document) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect Giztest file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("play requires one regular Giztest file")
	}
	if len(docs) != 1 {
		return fmt.Errorf("play requires exactly one Giztest document")
	}
	doc := docs[0]
	if doc.Repeat != 1 {
		return fmt.Errorf("play requires repeat 1, got %d", doc.Repeat)
	}
	if documentHasBarrier(doc) {
		return fmt.Errorf("play does not support barrier steps")
	}
	return nil
}

func validatePlayOutput(path string) error {
	if path == "" {
		return fmt.Errorf("play requires --output")
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("play output path already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect play output path: %w", err)
	}
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil {
		return fmt.Errorf("inspect play output parent: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("play output parent is not a directory: %s", parent)
	}
	return nil
}

type playRecord struct {
	target    string
	temp      string
	committed bool
}

func newPlayRecord(path string) (*playRecord, error) {
	if err := validatePlayOutput(path); err != nil {
		return nil, err
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("play output path already exists: %s", path)
		}
		return nil, fmt.Errorf("reserve play output path: %w", err)
	}
	return &playRecord{target: path, temp: path}, nil
}

func (r *playRecord) abort() {
	if r != nil && !r.committed && r.temp != "" {
		_ = os.RemoveAll(r.temp)
	}
}

func (r *playRecord) commit(report Report, packets [][]byte) error {
	if r == nil || r.temp == "" || r.committed {
		return fmt.Errorf("play record is not open")
	}
	if err := writeReport(filepath.Join(r.temp, "report.json"), report); err != nil {
		return fmt.Errorf("write play report: %w", err)
	}
	if len(packets) > 0 {
		if err := writePlayAudio(filepath.Join(r.temp, "audio.ogg"), packets); err != nil {
			return err
		}
	}
	if err := syncDirectory(r.temp); err != nil {
		return fmt.Errorf("sync play record: %w", err)
	}
	r.committed = true
	if err := syncDirectory(filepath.Dir(r.target)); err != nil {
		return fmt.Errorf("sync play record parent: %w", err)
	}
	return nil
}

func writePlayRecord(path string, report Report, packets [][]byte) error {
	record, err := newPlayRecord(path)
	if err != nil {
		return err
	}
	defer record.abort()
	return record.commit(report, packets)
}

func writePlayAudio(path string, packets [][]byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create play audio: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	if err := codecconv.OpusPacketsToOgg(file, 16000, 1, packets); err != nil {
		return fmt.Errorf("encode play audio: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync play audio: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close play audio: %w", err)
	}
	closed = true
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
