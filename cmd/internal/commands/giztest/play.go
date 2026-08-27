package giztest

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/portaudio"
)

const (
	playMaxAudioBytes   = 16 << 20
	playJitterPackets   = 10
	playCueFrequency    = 660
	playCueToneDuration = 120 * time.Millisecond
	playCueTailDuration = 80 * time.Millisecond
)

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
	decoder                 playDecoder
	output                  playOutput
	out                     io.Writer
	packets                 [][]byte
	pending                 [][]byte
	bytes                   int
	client                  string
	role                    string
	utteranceStarted        bool
	cueAt                   time.Time
	firstDownlinkReceivedMS int64
	firstDownlinkPlaybackMS int64
	closed                  atomic.Bool
}

func newPlaySession(outputs ...io.Writer) (*playSession, error) {
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
	out := io.Discard
	if len(outputs) > 0 && outputs[0] != nil {
		out = outputs[0]
	}
	return &playSession{
		decoder: decoder, output: output, out: out,
		firstDownlinkReceivedMS: -1, firstDownlinkPlaybackMS: -1,
	}, nil
}

func (s *playSession) cue() error {
	if s == nil || s.closed.Load() {
		return errors.New("play session is closed")
	}
	const sampleRate = 16000
	toneSamples := int(playCueToneDuration * sampleRate / time.Second)
	tailSamples := int(playCueTailDuration * sampleRate / time.Second)
	pcmBytes := make([]byte, (toneSamples+tailSamples)*2)
	for i := range toneSamples {
		sample := int16(math.Sin(2*math.Pi*playCueFrequency*float64(i)/sampleRate) * 5000)
		binary.LittleEndian.PutUint16(pcmBytes[i*2:], uint16(sample))
	}
	if err := s.writePCM(pcmBytes); err != nil {
		return fmt.Errorf("play start cue: %w", err)
	}
	s.cueAt = time.Now()
	fmt.Fprintln(s.out, "Giztest play started after cue")
	return nil
}

func (s *playSession) observe(client, role string, packet []byte, end bool) error {
	if s == nil || s.closed.Load() {
		return errors.New("play session is closed")
	}
	if (s.client != "" && s.client != client) || (s.role != "" && s.role != role) {
		if err := s.finishUtterance(); err != nil {
			return err
		}
	}
	if s.client == "" {
		s.client, s.role = client, role
	}
	if len(packet) > 0 {
		if s.bytes > playMaxAudioBytes-len(packet) {
			return fmt.Errorf("play audio exceeds fixed %d-byte limit", playMaxAudioBytes)
		}
		copyOfPacket := append([]byte(nil), packet...)
		s.packets = append(s.packets, copyOfPacket)
		s.pending = append(s.pending, copyOfPacket)
		s.bytes += len(copyOfPacket)
		if role == "assistant" && s.firstDownlinkReceivedMS < 0 && !s.cueAt.IsZero() {
			s.firstDownlinkReceivedMS = time.Since(s.cueAt).Milliseconds()
		}
		if !s.utteranceStarted && len(s.pending) >= playJitterPackets {
			s.utteranceStarted = true
			if role == "assistant" && s.firstDownlinkPlaybackMS < 0 && !s.cueAt.IsZero() {
				s.firstDownlinkPlaybackMS = time.Since(s.cueAt).Milliseconds()
				fmt.Fprintf(s.out, "Giztest first downlink: received=%dms playback=%dms jitter_packets=%d client=%s\n", s.firstDownlinkReceivedMS, s.firstDownlinkPlaybackMS, playJitterPackets, client)
			}
		}
		if s.utteranceStarted {
			if err := s.drainPending(); err != nil {
				return err
			}
		}
	}
	if end {
		return s.finishUtterance()
	}
	return nil
}

func (s *playSession) ensureDecoder() error {
	if s.decoder != nil {
		return nil
	}
	decoder, err := newPlayDecoderFn()
	if err != nil {
		return fmt.Errorf("create Opus playback decoder: %w", err)
	}
	s.decoder = decoder
	return nil
}

func (s *playSession) drainPending() error {
	if len(s.pending) == 0 {
		return nil
	}
	if err := s.ensureDecoder(); err != nil {
		return err
	}
	const maxFrameSize = 16000 * 3 / 25
	var pcmBytes []byte
	for _, packet := range s.pending {
		samples, err := s.decoder.Decode(packet, maxFrameSize, false)
		if err != nil {
			return fmt.Errorf("decode Opus packet: %w", err)
		}
		start := len(pcmBytes)
		pcmBytes = append(pcmBytes, make([]byte, len(samples)*2)...)
		for i, sample := range samples {
			binary.LittleEndian.PutUint16(pcmBytes[start+i*2:], uint16(sample))
		}
	}
	s.pending = nil
	if err := s.writePCM(pcmBytes); err != nil {
		return fmt.Errorf("write streaming PCM to PortAudio: %w", err)
	}
	return nil
}

func (s *playSession) writePCM(pcmBytes []byte) error {
	if len(pcmBytes) == 0 {
		return nil
	}
	written, err := s.output.Write(pcmBytes)
	if err != nil {
		return err
	}
	if written != len(pcmBytes) {
		return io.ErrShortWrite
	}
	return nil
}

func (s *playSession) finishUtterance() error {
	if len(s.pending) > 0 {
		s.utteranceStarted = true
		if s.role == "assistant" && s.firstDownlinkPlaybackMS < 0 && !s.cueAt.IsZero() {
			s.firstDownlinkPlaybackMS = time.Since(s.cueAt).Milliseconds()
			fmt.Fprintf(s.out, "Giztest first downlink: received=%dms playback=%dms jitter_packets=%d client=%s\n", s.firstDownlinkReceivedMS, s.firstDownlinkPlaybackMS, len(s.pending), s.client)
		}
		if err := s.drainPending(); err != nil {
			return err
		}
	}
	var err error
	if s.decoder != nil {
		err = s.decoder.Close()
		s.decoder = nil
	}
	s.client, s.role = "", ""
	s.utteranceStarted = false
	return err
}

func (s *playSession) latencySummary() (int64, int64) {
	return s.firstDownlinkReceivedMS, s.firstDownlinkPlaybackMS
}

func (s *playSession) close() error {
	if s == nil || s.closed.Load() {
		return nil
	}
	flushErr := s.finishUtterance()
	if !s.closed.CompareAndSwap(false, true) {
		return flushErr
	}
	var decoderErr error
	if s.decoder != nil {
		decoderErr = s.decoder.Close()
		s.decoder = nil
	}
	return errors.Join(flushErr, s.output.Close(), decoderErr)
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
