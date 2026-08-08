package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/portaudio"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/resampler"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/songs"
)

type mode string

const (
	modeListSongs mode = "list"
	modePlaySong  mode = "play-song"
	modeRecordMic mode = "record-mic"
	modePlayMP3   mode = "play-mp3"
	modeRecordOGG mode = "record-ogg"
	modePlayOGG   mode = "play-ogg"
)

type config struct {
	mode mode

	formatName string
	format     pcm.Format

	deviceID        int
	framesPerBuffer uint32

	volume    float64
	richSound bool
	metronome bool

	songID   string
	songsCSV string

	inputMP3  string
	outputMP3 string
	inputOGG  string
	outputOGG string
	timeout   time.Duration
	bitrate   int

	opusLoopback bool
}

type playbackWriter interface {
	io.Writer
	Close() error
}

type captureReader interface {
	io.Reader
	Close() error
	Config() portaudio.StreamConfig
}

type mp3Encoder interface {
	Write([]byte) (int, error)
	Flush() error
	Close() error
}

type opusEncoder interface {
	Encode(pcm []int16, frameSize int) ([]byte, error)
	Close() error
}

type opusDecoder interface {
	Decode(packet []byte, frameSize int, fec bool) ([]int16, error)
	Close() error
}

type oggStreamWriter interface {
	WritePacket(packet []byte, granulePos uint64, eos bool) (int, error)
}

var (
	openPlaybackFn = func(format pcm.Format, opts portaudio.PlaybackOptions) (playbackWriter, error) {
		return portaudio.OpenPlayback(format, opts)
	}

	openCaptureFn = func(format pcm.Format, opts portaudio.CaptureOptions) (captureReader, error) {
		return portaudio.OpenCapture(format, opts)
	}

	newMP3EncoderFn = func(w io.Writer, sampleRate, channels int, opts ...mp3.EncoderOption) (mp3Encoder, error) {
		return mp3.NewEncoder(w, sampleRate, channels, opts...)
	}

	decodeMP3Fn = mp3.DecodeFull

	newResamplerFn = func(src io.Reader, srcFmt, dstFmt resampler.Format) (io.ReadCloser, error) {
		return resampler.New(src, srcFmt, dstFmt)
	}

	opusRuntimeSupportedFn = opus.IsRuntimeSupported

	newOpusEncoderFn = func(sampleRate, channels int, app opus.Application) (opusEncoder, error) {
		return opus.NewEncoder(sampleRate, channels, app)
	}

	newOpusDecoderFn = func(sampleRate, channels int) (opusDecoder, error) {
		return opus.NewDecoder(sampleRate, channels)
	}

	newOGGStreamWriterFn = func(w io.Writer, serial uint32) (oggStreamWriter, error) {
		return ogg.NewStreamWriter(w, serial)
	}

	readAllOGGPacketsFn = ogg.ReadAllPackets

	nowFn = time.Now
)

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse args failed: %v\n\n", err)
		printUsage(os.Stderr)
		os.Exit(2)
	}

	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
		os.Exit(1)
	}
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("songs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var cfg config
	modeRaw := string(modePlaySong)
	var framesPerBufferRaw uint

	fs.StringVar(&modeRaw, "mode", modeRaw, "Mode: list | play-song | record-mic | play-mp3 | record-ogg | play-ogg")
	fs.StringVar(&cfg.formatName, "format", "16k", "PCM sample rate for playback/recording: 16k | 24k | 48k")
	fs.IntVar(&cfg.deviceID, "device-id", portaudio.DefaultDeviceID, "PortAudio device ID, default -1 means system default")
	fs.UintVar(&framesPerBufferRaw, "frames-per-buffer", 0, "PortAudio frames per buffer, 0 means auto")

	fs.StringVar(&cfg.songID, "song", "twinkle_star", "Single song ID (play-song)")
	fs.StringVar(&cfg.songsCSV, "songs", "", "Song IDs for multi-track mix, comma separated (play-song)")
	fs.Float64Var(&cfg.volume, "volume", 0.5, "Render volume in range [0.0, 1.0]")
	fs.BoolVar(&cfg.richSound, "rich-sound", true, "Enable richer piano timbre")
	fs.BoolVar(&cfg.metronome, "metronome", false, "Overlay metronome during rendering")

	fs.StringVar(&cfg.inputMP3, "input", "", "Input MP3 path (play-mp3)")
	fs.StringVar(&cfg.outputMP3, "output", "", "Output MP3 path (record-mic)")
	fs.StringVar(&cfg.inputOGG, "input-ogg", "", "Input OGG path (play-ogg)")
	fs.StringVar(&cfg.outputOGG, "output-ogg", "", "Output OGG path (record-ogg)")
	fs.DurationVar(&cfg.timeout, "timeout", 5*time.Second, "Recording duration (record-mic)")
	fs.IntVar(&cfg.bitrate, "bitrate", 96, "MP3 encoding bitrate in kbps (record-mic)")

	fs.BoolVar(&cfg.opusLoopback, "opus-loopback", false, "Run Opus encode/decode loopback before playback (without OGG container)")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	pcmFmt, err := parsePCMFormat(cfg.formatName)
	if err != nil {
		return config{}, err
	}
	cfg.format = pcmFmt
	cfg.mode = mode(strings.TrimSpace(modeRaw))
	cfg.framesPerBuffer = uint32(framesPerBufferRaw)

	if err := cfg.validate(); err != nil {
		return config{}, err
	}

	return cfg, nil
}

func (cfg *config) validate() error {
	if cfg == nil {
		return errors.New("nil config")
	}
	if cfg.volume < 0 || cfg.volume > 1 {
		return fmt.Errorf("volume must be in [0,1], got %f", cfg.volume)
	}
	if cfg.bitrate <= 0 {
		return fmt.Errorf("bitrate must be > 0, got %d", cfg.bitrate)
	}

	switch cfg.mode {
	case modeListSongs:
		return nil
	case modePlaySong:
		_, err := parseSongIDs(cfg.songID, cfg.songsCSV)
		return err
	case modeRecordMic:
		if cfg.timeout <= 0 {
			return fmt.Errorf("timeout must be > 0, got %s", cfg.timeout)
		}
		if strings.TrimSpace(cfg.outputMP3) == "" {
			cfg.outputMP3 = defaultRecordingFileName("mp3")
		}
		return nil
	case modePlayMP3:
		if strings.TrimSpace(cfg.inputMP3) == "" {
			return errors.New("input mp3 path is required in play-mp3 mode")
		}
		return nil
	case modeRecordOGG:
		if cfg.timeout <= 0 {
			return fmt.Errorf("timeout must be > 0, got %s", cfg.timeout)
		}
		if strings.TrimSpace(cfg.outputOGG) == "" {
			cfg.outputOGG = defaultRecordingFileName("ogg")
		}
		return nil
	case modePlayOGG:
		if strings.TrimSpace(cfg.inputOGG) == "" {
			return errors.New("input ogg path is required in play-ogg mode")
		}
		return nil
	default:
		return fmt.Errorf("unsupported mode %q", cfg.mode)
	}
}

func run(cfg config) error {
	switch cfg.mode {
	case modeListSongs:
		return listSongs(os.Stdout)
	case modePlaySong:
		return playSongs(cfg)
	case modeRecordMic:
		return recordMicrophoneToMP3(cfg)
	case modePlayMP3:
		return playMP3File(cfg)
	case modeRecordOGG:
		return recordMicrophoneToOGG(cfg)
	case modePlayOGG:
		return playOGGFile(cfg)
	default:
		return fmt.Errorf("unsupported mode %q", cfg.mode)
	}
}

func listSongs(w io.Writer) error {
	if w == nil {
		return errors.New("nil writer")
	}
	fmt.Fprintf(w, "%d built-in songs:\n", len(songs.All))
	for _, s := range songs.All {
		voices := len(s.Voices())
		dur := time.Duration(s.Duration()) * time.Millisecond
		fmt.Fprintf(w, "- %-20s | %-20s | voices=%d | duration=%s\n", s.ID, s.Name, voices, dur)
	}
	return nil
}

func playSongs(cfg config) error {
	ids, err := parseSongIDs(cfg.songID, cfg.songsCSV)
	if err != nil {
		return err
	}

	selected, err := resolveSongs(ids)
	if err != nil {
		return err
	}

	fmt.Printf("Playing songs: %s\n", strings.Join(ids, ", "))

	reader := buildSongReader(selected, cfg)
	if cfg.opusLoopback {
		loopReader, err := newOpusLoopbackReader(reader, cfg.format)
		if err != nil {
			return err
		}
		defer func() {
			_ = loopReader.Close()
		}()
		reader = loopReader
	}

	return playReaderWithPortAudio(cfg, reader)
}

func recordMicrophoneToMP3(cfg config) error {
	if err := ensureParentDir(cfg.outputMP3); err != nil {
		return err
	}

	outFile, err := os.Create(cfg.outputMP3)
	if err != nil {
		return fmt.Errorf("create output file failed: %w", err)
	}
	defer func() {
		_ = outFile.Close()
	}()

	capStream, err := openCaptureFn(cfg.format, portaudio.CaptureOptions{
		HasDeviceID:     cfg.deviceID != portaudio.DefaultDeviceID,
		DeviceID:        cfg.deviceID,
		FramesPerBuffer: cfg.framesPerBuffer,
	})
	if err != nil {
		return withPortAudioHint(err)
	}
	defer func() {
		_ = capStream.Close()
	}()

	enc, err := newMP3EncoderFn(outFile, cfg.format.SampleRate(), cfg.format.Channels(), mp3.WithBitrate(cfg.bitrate))
	if err != nil {
		return fmt.Errorf("create mp3 encoder failed: %w", err)
	}
	defer func() {
		_ = enc.Close()
	}()

	chunkBytes := int(capStream.Config().FramesPerBuffer) * cfg.format.Channels() * 2
	if chunkBytes <= 0 {
		chunkBytes = int(cfg.format.BytesInDuration(20 * time.Millisecond))
	}
	buf := make([]byte, chunkBytes)

	fmt.Printf("Recording started: timeout=%s, output=%s\n", cfg.timeout, cfg.outputMP3)
	deadline := time.Now().Add(cfg.timeout)
	var pcmBytes int64

	for time.Now().Before(deadline) {
		n, readErr := capStream.Read(buf)
		if n > 0 {
			pcmBytes += int64(n)
			if _, err := enc.Write(buf[:n]); err != nil {
				return fmt.Errorf("encode mp3 frame failed: %w", err)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return fmt.Errorf("capture read failed: %w", readErr)
		}
	}

	if err := enc.Flush(); err != nil {
		return fmt.Errorf("flush mp3 encoder failed: %w", err)
	}
	if err := outFile.Sync(); err != nil {
		return fmt.Errorf("sync output mp3 failed: %w", err)
	}

	fmt.Printf("Recording completed: pcm=%d bytes, mp3=%s\n", pcmBytes, cfg.outputMP3)
	return nil
}

func playMP3File(cfg config) error {
	reader, err := decodeAndResampleMP3(cfg.inputMP3, cfg.format)
	if err != nil {
		return err
	}
	defer func() {
		_ = reader.Close()
	}()

	playReader := io.Reader(reader)
	if cfg.opusLoopback {
		loopReader, err := newOpusLoopbackReader(playReader, cfg.format)
		if err != nil {
			return err
		}
		defer func() {
			_ = loopReader.Close()
		}()
		playReader = loopReader
	}

	fmt.Printf("Playing MP3: %s\n", cfg.inputMP3)
	return playReaderWithPortAudio(cfg, playReader)
}

func recordMicrophoneToOGG(cfg config) error {
	if !opusRuntimeSupportedFn() {
		return fmt.Errorf("record-ogg requires native opus runtime")
	}

	if err := ensureParentDir(cfg.outputOGG); err != nil {
		return err
	}

	outFile, err := os.Create(cfg.outputOGG)
	if err != nil {
		return fmt.Errorf("create output file failed: %w", err)
	}
	defer func() {
		_ = outFile.Close()
	}()

	capStream, err := openCaptureFn(cfg.format, portaudio.CaptureOptions{
		HasDeviceID:     cfg.deviceID != portaudio.DefaultDeviceID,
		DeviceID:        cfg.deviceID,
		FramesPerBuffer: cfg.framesPerBuffer,
	})
	if err != nil {
		return withPortAudioHint(err)
	}
	defer func() {
		_ = capStream.Close()
	}()

	opusEnc, err := newOpusEncoderFn(cfg.format.SampleRate(), cfg.format.Channels(), opus.ApplicationAudio)
	if err != nil {
		return fmt.Errorf("create opus encoder failed: %w", err)
	}
	defer func() {
		_ = opusEnc.Close()
	}()

	sw, err := newOGGStreamWriterFn(outFile, uint32(nowFn().UnixNano()))
	if err != nil {
		return fmt.Errorf("create ogg stream writer failed: %w", err)
	}

	opusHead, err := buildOpusHeadPacket(cfg.format.SampleRate(), cfg.format.Channels())
	if err != nil {
		return err
	}
	if _, err := sw.WritePacket(opusHead, 0, false); err != nil {
		return fmt.Errorf("write ogg opus head failed: %w", err)
	}
	if _, err := sw.WritePacket(buildOpusTagsPacket("gizclaw-go/examples/songs"), 0, false); err != nil {
		return fmt.Errorf("write ogg opus tags failed: %w", err)
	}

	frameSize := cfg.format.SampleRate() / 50
	frameBytes := frameSize * cfg.format.Channels() * 2
	if frameBytes <= 0 {
		return fmt.Errorf("invalid frame bytes for format %s", cfg.format)
	}

	buf := make([]byte, frameBytes)
	deadline := time.Now().Add(cfg.timeout)
	var pcmBytes int64
	var granulePos uint64
	var pendingPacket []byte
	var pendingGranule uint64

	fmt.Printf("Recording started: timeout=%s, output=%s\n", cfg.timeout, cfg.outputOGG)
	for time.Now().Before(deadline) {
		n, readErr := io.ReadFull(capStream, buf)
		if n > 0 {
			frame := make([]byte, frameBytes)
			copy(frame, buf[:n])

			opusPacket, err := opusEnc.Encode(bytesToInt16(frame), frameSize)
			if err != nil {
				return fmt.Errorf("encode opus frame failed: %w", err)
			}

			granulePos += uint64(frameSize)
			if len(pendingPacket) > 0 {
				if _, err := sw.WritePacket(pendingPacket, pendingGranule, false); err != nil {
					return fmt.Errorf("write ogg opus packet failed: %w", err)
				}
			}
			pendingPacket = opusPacket
			pendingGranule = granulePos
			pcmBytes += int64(n)
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
				break
			}
			return fmt.Errorf("capture read failed: %w", readErr)
		}
	}

	if len(pendingPacket) > 0 {
		if _, err := sw.WritePacket(pendingPacket, pendingGranule, true); err != nil {
			return fmt.Errorf("write ogg final opus packet failed: %w", err)
		}
	} else {
		if _, err := sw.WritePacket(nil, 0, true); err != nil {
			return fmt.Errorf("write ogg empty eos packet failed: %w", err)
		}
	}

	if err := outFile.Sync(); err != nil {
		return fmt.Errorf("sync output ogg failed: %w", err)
	}

	fmt.Printf("Recording completed: pcm=%d bytes, ogg=%s\n", pcmBytes, cfg.outputOGG)
	return nil
}

func playOGGFile(cfg config) error {
	reader, err := decodeAndResampleOGG(cfg.inputOGG, cfg.format)
	if err != nil {
		return err
	}
	defer func() {
		_ = reader.Close()
	}()

	playReader := io.Reader(reader)
	if cfg.opusLoopback {
		loopReader, err := newOpusLoopbackReader(playReader, cfg.format)
		if err != nil {
			return err
		}
		defer func() {
			_ = loopReader.Close()
		}()
		playReader = loopReader
	}

	fmt.Printf("Playing OGG: %s\n", cfg.inputOGG)
	return playReaderWithPortAudio(cfg, playReader)
}

func playReaderWithPortAudio(cfg config, reader io.Reader) error {
	playback, err := openPlaybackFn(cfg.format, portaudio.PlaybackOptions{
		HasDeviceID:     cfg.deviceID != portaudio.DefaultDeviceID,
		DeviceID:        cfg.deviceID,
		FramesPerBuffer: cfg.framesPerBuffer,
	})
	if err != nil {
		return withPortAudioHint(err)
	}
	defer func() {
		_ = playback.Close()
	}()

	if err := pcm.Copy(pcm.ChunkWriter(playback), reader, cfg.format); err != nil {
		return fmt.Errorf("playback copy failed: %w", err)
	}
	return nil
}

func buildSongReader(selected []songs.Song, cfg config) io.Reader {
	if len(selected) == 1 {
		return selected[0].Render(songs.RenderOptions{
			Format:    cfg.format,
			Volume:    clampVolume(cfg.volume),
			Metronome: cfg.metronome,
			RichSound: cfg.richSound,
		})
	}

	mx := pcm.NewMixer(cfg.format, pcm.WithAutoClose())

	trackVolume := clampVolume(cfg.volume / math.Sqrt(float64(len(selected))))
	var wg sync.WaitGroup

	for _, song := range selected {
		s := song
		trackReader := s.Render(songs.RenderOptions{
			Format:    cfg.format,
			Volume:    trackVolume,
			Metronome: cfg.metronome,
			RichSound: cfg.richSound,
		})

		track, ctrl, err := mx.CreateTrack(pcm.WithTrackLabel("song-" + s.ID))
		if err != nil {
			_ = mx.CloseWithError(fmt.Errorf("create mixer track for %s failed: %w", s.ID, err))
			continue
		}

		wg.Add(1)
		go func(songID string, reader io.Reader, track pcm.Track, ctrl *pcm.TrackCtrl) {
			defer wg.Done()
			if err := pcm.Copy(track, reader, cfg.format); err != nil {
				_ = ctrl.CloseWithError(fmt.Errorf("copy track %s failed: %w", songID, err))
				_ = mx.CloseWithError(fmt.Errorf("mixer copy failed on %s: %w", songID, err))
				return
			}
			_ = ctrl.CloseWrite()
		}(s.ID, trackReader, track, ctrl)
	}

	go func() {
		wg.Wait()
		_ = mx.CloseWrite()
	}()

	return mx
}

func decodeAndResampleMP3(path string, outFmt pcm.Format) (io.ReadCloser, error) {
	in, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open mp3 file failed: %w", err)
	}
	defer func() {
		_ = in.Close()
	}()

	decoded, sampleRate, channels, err := decodeMP3Fn(in)
	if err != nil {
		return nil, fmt.Errorf("decode mp3 failed: %w", err)
	}
	if channels != 1 && channels != 2 {
		return nil, fmt.Errorf("unsupported decoded mp3 channels=%d", channels)
	}

	srcFmt := resampler.Format{SampleRate: sampleRate, Stereo: channels == 2}
	dstFmt := resampler.Format{SampleRate: outFmt.SampleRate(), Stereo: outFmt.Channels() == 2}

	rs, err := newResamplerFn(bytes.NewReader(decoded), srcFmt, dstFmt)
	if err != nil {
		return nil, fmt.Errorf("create resampler failed: %w", err)
	}
	return rs, nil
}

func decodeAndResampleOGG(path string, outFmt pcm.Format) (io.ReadCloser, error) {
	in, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open ogg file failed: %w", err)
	}
	defer func() {
		_ = in.Close()
	}()

	packets, err := readAllOGGPacketsFn(in)
	if err != nil {
		return nil, fmt.Errorf("read ogg packets failed: %w", err)
	}
	if len(packets) == 0 {
		return nil, errors.New("empty ogg packet stream")
	}

	srcSampleRate := outFmt.SampleRate()
	srcChannels := outFmt.Channels()
	audioPackets := make([][]byte, 0, len(packets))

	for idx, pkt := range packets {
		if isOpusHeadPacket(pkt.Data) {
			sampleRate, channels, err := parseOpusHeadPacket(pkt.Data)
			if err != nil {
				return nil, fmt.Errorf("parse ogg opus head packet %d failed: %w", idx, err)
			}
			srcSampleRate = sampleRate
			srcChannels = channels
			continue
		}
		if isOpusTagsPacket(pkt.Data) || len(pkt.Data) == 0 {
			continue
		}
		audioPackets = append(audioPackets, append([]byte(nil), pkt.Data...))
	}

	if len(audioPackets) == 0 {
		return nil, errors.New("no opus audio packets found in ogg stream")
	}

	dec, err := newOpusDecoderFn(srcSampleRate, srcChannels)
	if err != nil {
		return nil, fmt.Errorf("create opus decoder failed: %w", err)
	}
	defer func() {
		_ = dec.Close()
	}()

	maxFrameSize := (srcSampleRate * 3) / 50
	if maxFrameSize <= 0 {
		return nil, fmt.Errorf("invalid max frame size from sample rate %d", srcSampleRate)
	}

	var pcmBytes bytes.Buffer
	for idx, packet := range audioPackets {
		samples, err := dec.Decode(packet, maxFrameSize, false)
		if err != nil {
			return nil, fmt.Errorf("decode ogg opus packet %d failed: %w", idx, err)
		}
		if _, err := pcmBytes.Write(int16ToBytes(samples)); err != nil {
			return nil, fmt.Errorf("write decoded pcm buffer failed: %w", err)
		}
	}

	srcFmt := resampler.Format{SampleRate: srcSampleRate, Stereo: srcChannels == 2}
	dstFmt := resampler.Format{SampleRate: outFmt.SampleRate(), Stereo: outFmt.Channels() == 2}
	if srcFmt == dstFmt {
		return io.NopCloser(bytes.NewReader(pcmBytes.Bytes())), nil
	}

	rs, err := newResamplerFn(bytes.NewReader(pcmBytes.Bytes()), srcFmt, dstFmt)
	if err != nil {
		return nil, fmt.Errorf("create resampler failed: %w", err)
	}
	return rs, nil
}

func buildOpusHeadPacket(sampleRate, channels int) ([]byte, error) {
	if sampleRate <= 0 {
		return nil, fmt.Errorf("invalid opus sample rate %d", sampleRate)
	}
	if channels != 1 && channels != 2 {
		return nil, fmt.Errorf("invalid opus channels %d", channels)
	}

	packet := make([]byte, 19)
	copy(packet[:8], "OpusHead")
	packet[8] = 1
	packet[9] = byte(channels)
	binary.LittleEndian.PutUint16(packet[10:12], 0)
	binary.LittleEndian.PutUint32(packet[12:16], uint32(sampleRate))
	binary.LittleEndian.PutUint16(packet[16:18], 0)
	packet[18] = 0
	return packet, nil
}

func buildOpusTagsPacket(vendor string) []byte {
	vendorBytes := []byte(vendor)
	packet := make([]byte, 8+4+len(vendorBytes)+4)
	copy(packet[:8], "OpusTags")
	binary.LittleEndian.PutUint32(packet[8:12], uint32(len(vendorBytes)))
	copy(packet[12:12+len(vendorBytes)], vendorBytes)
	binary.LittleEndian.PutUint32(packet[12+len(vendorBytes):], 0)
	return packet
}

func isOpusHeadPacket(packet []byte) bool {
	return len(packet) >= 8 && bytes.Equal(packet[:8], []byte("OpusHead"))
}

func isOpusTagsPacket(packet []byte) bool {
	return len(packet) >= 8 && bytes.Equal(packet[:8], []byte("OpusTags"))
}

func parseOpusHeadPacket(packet []byte) (sampleRate, channels int, err error) {
	if !isOpusHeadPacket(packet) {
		return 0, 0, errors.New("not an opus head packet")
	}
	if len(packet) < 19 {
		return 0, 0, fmt.Errorf("opus head packet too short: %d", len(packet))
	}

	channels = int(packet[9])
	if channels != 1 && channels != 2 {
		return 0, 0, fmt.Errorf("unsupported opus channels %d", channels)
	}

	sampleRate = int(binary.LittleEndian.Uint32(packet[12:16]))
	if sampleRate <= 0 {
		return 0, 0, fmt.Errorf("invalid opus sample rate %d", sampleRate)
	}

	return sampleRate, channels, nil
}

func newOpusLoopbackReader(src io.Reader, format pcm.Format) (io.ReadCloser, error) {
	if !opusRuntimeSupportedFn() {
		return nil, fmt.Errorf("opus loopback requires native opus runtime")
	}

	enc, err := newOpusEncoderFn(format.SampleRate(), format.Channels(), opus.ApplicationAudio)
	if err != nil {
		return nil, fmt.Errorf("create opus encoder failed: %w", err)
	}

	dec, err := newOpusDecoderFn(format.SampleRate(), format.Channels())
	if err != nil {
		_ = enc.Close()
		return nil, fmt.Errorf("create opus decoder failed: %w", err)
	}

	frameSize := format.SampleRate() / 50 // 20ms
	frameBytes := frameSize * format.Channels() * 2
	if frameBytes <= 0 {
		_ = enc.Close()
		_ = dec.Close()
		return nil, fmt.Errorf("invalid frame bytes for format %s", format)
	}

	pipeR, pipeW := io.Pipe()

	go func() {
		defer func() {
			_ = enc.Close()
			_ = dec.Close()
			if c, ok := src.(io.Closer); ok {
				_ = c.Close()
			}
		}()

		buf := make([]byte, frameBytes)
		for {
			n, readErr := io.ReadFull(src, buf)
			if n > 0 {
				frame := make([]byte, frameBytes)
				copy(frame, buf[:n])

				pcmSamples := bytesToInt16(frame)
				packet, err := enc.Encode(pcmSamples, frameSize)
				if err != nil {
					_ = pipeW.CloseWithError(fmt.Errorf("opus encode failed: %w", err))
					return
				}

				decoded, err := dec.Decode(packet, frameSize, false)
				if err != nil {
					_ = pipeW.CloseWithError(fmt.Errorf("opus decode failed: %w", err))
					return
				}

				out := int16ToBytes(decoded)
				if n < len(out) {
					trim := max(n-(n%2), 0)
					out = out[:trim]
				}

				if len(out) > 0 {
					if _, err := pipeW.Write(out); err != nil {
						return
					}
				}
			}

			if readErr != nil {
				if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
					_ = pipeW.Close()
					return
				}
				_ = pipeW.CloseWithError(readErr)
				return
			}
		}
	}()

	return pipeR, nil
}

func bytesToInt16(data []byte) []int16 {
	count := len(data) / 2
	out := make([]int16, count)
	for i := range count {
		out[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return out
}

func int16ToBytes(data []int16) []byte {
	out := make([]byte, len(data)*2)
	for i, s := range data {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(s))
	}
	return out
}

func parseSongIDs(defaultSongID, songsCSV string) ([]string, error) {
	if strings.TrimSpace(songsCSV) == "" {
		id := strings.TrimSpace(defaultSongID)
		if id == "" {
			return nil, errors.New("song id is empty")
		}
		return []string{id}, nil
	}

	parts := strings.Split(songsCSV, ",")
	ids := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, raw := range parts {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil, errors.New("songs is empty")
	}

	return ids, nil
}

func resolveSongs(ids []string) ([]songs.Song, error) {
	if len(ids) == 0 {
		return nil, errors.New("empty song ids")
	}

	selected := make([]songs.Song, 0, len(ids))
	for _, id := range ids {
		s := songs.ByID(id)
		if s == nil {
			return nil, fmt.Errorf("song %q not found, use -mode=list to view available songs", id)
		}
		selected = append(selected, *s)
	}

	return selected, nil
}

func parsePCMFormat(raw string) (pcm.Format, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "16k", "16000":
		return pcm.L16Mono16K, nil
	case "24k", "24000":
		return pcm.L16Mono24K, nil
	case "48k", "48000":
		return pcm.L16Mono48K, nil
	default:
		return 0, fmt.Errorf("unsupported format %q, allowed: 16k|24k|48k", raw)
	}
}

func clampVolume(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func defaultRecordingFileName(ext string) string {
	ext = strings.TrimPrefix(strings.TrimSpace(ext), ".")
	if ext == "" {
		ext = "mp3"
	}
	return fmt.Sprintf("recording-%s.%s", nowFn().Format("20060102-150405"), ext)
}

func ensureParentDir(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("path is empty")
	}
	parent := filepath.Dir(path)
	if parent == "." || parent == "" {
		return nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create parent dir %q failed: %w", parent, err)
	}
	return nil
}

func withPortAudioHint(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "unsupported platform") {
		return fmt.Errorf("%w; hint: this example requires cgo on a supported OS/arch (for example, CGO_ENABLED=1)", err)
	}
	return err
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `examples/songs: integration helper for songs + mixer + portaudio + mp3/ogg (optional opus loopback)

Usage:
  cd examples/songs
  go run . -mode list
  CGO_ENABLED=1 go run . -mode play-song -song twinkle_star
  CGO_ENABLED=1 go run . -mode play-song -songs twinkle_star,canon
  CGO_ENABLED=1 go run . -mode record-mic -timeout 5s -output ./out/mic.mp3
  CGO_ENABLED=1 go run . -mode play-mp3 -input ./out/mic.mp3
  CGO_ENABLED=1 go run . -mode record-ogg -timeout 5s -output-ogg ./out/mic.ogg
  CGO_ENABLED=1 go run . -mode play-ogg -input-ogg ./out/mic.ogg

Notes:
  - OGG mode stores Opus packets in OGG container.
  - play-ogg is currently guaranteed for OGG files recorded by this example.
  - Use -opus-loopback to add an extra Opus encode/decode loopback in playback chain.
`)
}
