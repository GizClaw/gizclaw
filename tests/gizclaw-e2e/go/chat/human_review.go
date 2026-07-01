//go:build gizclaw_e2e

package chat

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/portaudio"
	"github.com/GizClaw/gizclaw-go/pkgs/buffer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

const humanReviewOpusFrameDuration = 20 * time.Millisecond
const humanReviewBeepDuration = 300 * time.Millisecond
const humanReviewOutputPrebuffer = 800 * time.Millisecond
const humanReviewOutputIdleKeepalive = 1500 * time.Millisecond
const humanReviewOutputUnderflowRetries = 3
const humanReviewPlaybackQueueSize = 1024
const humanReviewHistoryReplayMax = 2
const humanReviewHistoryReplayWait = 60 * time.Second

type opusPacketDecoder interface {
	Decode(packet []byte, frameSize int, fec bool) ([]int16, error)
	Close() error
}

type playbackStream interface {
	Write([]byte) (int, error)
	Close() error
}

type humanReviewPCMFrame struct {
	label string
	data  []byte
}

type humanReviewPlayback struct {
	format            pcm.Format
	channels          int
	frameSize         int
	stream            playbackStream
	inputDec          opusPacketDecoder
	outputDec         opusPacketDecoder
	playBuffer        *buffer.RingBuffer[humanReviewPCMFrame]
	playDone          chan error
	closePlayOnce     sync.Once
	closeOnce         sync.Once
	closeErr          error
	playStateMu       sync.Mutex
	playClosed        bool
	mu                sync.Mutex
	underflowWarnings int
	bufferUnderruns   int
	playFrames        int
	droppedFrames     atomic.Int64
}

func runHumanReview(ctx context.Context, d *personaDriver) ([]roundStats, error) {
	if d == nil {
		return nil, fmt.Errorf("persona driver is nil")
	}
	baseNewTransport := d.newTransport
	if baseNewTransport == nil {
		return nil, fmt.Errorf("human review transport factory is required")
	}
	d.useRoundtripUtterances()
	playback, err := newHumanReviewPlayback()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = playback.Close()
	}()
	fmt.Printf("human_review playback backend=%s format=%s\n", portaudio.BackendName(), playback.format)
	if err := playback.PlayBeep(ctx); err != nil {
		return nil, fmt.Errorf("human review playback self-test: %w", err)
	}
	d.newTransport = func() (*chatTransport, error) {
		transport, err := baseNewTransport()
		if err != nil {
			return nil, err
		}
		transport.audioTap = playback
		return transport, nil
	}
	defer func() {
		detachHumanReviewPlayback(d.transport, playback)
		d.newTransport = baseNewTransport
	}()
	rounds, err := d.runConversation(ctx, conversationMode{SkipAssistantAudioASR: true})
	if err == nil {
		_, err = d.runHumanReviewHistoryReplay(ctx, humanReviewHistoryReplayTarget(len(rounds), humanReviewHistoryReplayMax))
	}
	detachHumanReviewPlayback(d.transport, playback)
	return rounds, err
}

func (d *personaDriver) runHumanReview(ctx context.Context) ([]roundStats, error) {
	return runHumanReview(ctx, d)
}

func detachHumanReviewPlayback(transport *chatTransport, playback *humanReviewPlayback) {
	if transport != nil && transport.audioTap == playback {
		transport.audioTap = nil
	}
}

func humanReviewHistoryReplayTarget(rounds, maxReplay int) int {
	if maxReplay < 1 {
		maxReplay = 1
	}
	if rounds > 0 && rounds < maxReplay {
		return rounds
	}
	return maxReplay
}

func (d *personaDriver) runHumanReviewHistoryReplay(ctx context.Context, target int) ([]historyReplayStats, error) {
	if target <= 0 {
		return nil, nil
	}
	if d == nil {
		return nil, fmt.Errorf("persona driver is nil")
	}
	if d.runtimeClient == nil {
		return nil, fmt.Errorf("human review history replay requires runtime client")
	}
	if d.transport == nil {
		return nil, fmt.Errorf("human review history replay requires active transport")
	}
	items, err := d.waitHumanReviewHistoryReplayItems(ctx, target)
	if err != nil {
		return nil, err
	}
	stats := make([]historyReplayStats, 0, len(items))
	for i, item := range items {
		d.drainTransport()
		fmt.Printf("workspace_progress event=human_review_history_replay_start workspace=%s index=%d history_id=%s text_chars=%d\n", d.cfg.Workspace, i+1, item.Id, runeCount(item.Text))
		play, err := d.runtimeClient.PlayServerRunWorkspaceHistory(ctx, fmt.Sprintf("workspacetest.human_review.history.play.%d", i+1), rpcapi.ServerPlayRunWorkspaceHistoryRequest{
			HistoryId: item.Id,
		})
		if err != nil {
			return stats, fmt.Errorf("human review history replay %q: play: %w", item.Id, err)
		}
		if play == nil || !play.Accepted {
			state := ""
			message := ""
			if play != nil {
				state = string(play.State)
				if play.Message != nil {
					message = *play.Message
				}
			}
			return stats, fmt.Errorf("human review history replay %q rejected state=%s: %s", item.Id, state, message)
		}
		replay, err := d.verifyHistoryReplay(ctx, item)
		if err != nil {
			return stats, fmt.Errorf("human review history replay %q output: %w", item.Id, err)
		}
		stats = append(stats, replay)
		fmt.Printf("human_review_history_replay=%s\n", encodeJSONLine(map[string]any{
			"index":            i + 1,
			"history_id":       item.Id,
			"text":             replay.Text,
			"audio_asr":        replay.AudioASR,
			"downlink_packets": replay.DownlinkPackets,
			"state":            play.State,
		}))
	}
	return stats, nil
}

func (d *personaDriver) waitHumanReviewHistoryReplayItems(ctx context.Context, target int) ([]rpcapi.PeerRunHistoryEntry, error) {
	if target <= 0 {
		return nil, nil
	}
	limit := 50
	deadline := time.NewTimer(humanReviewHistoryReplayWait)
	defer deadline.Stop()
	for {
		history, err := d.runtimeClient.ListServerRunWorkspaceHistory(ctx, "workspacetest.human_review.history", rpcapi.ServerListRunWorkspaceHistoryRequest{Limit: &limit})
		if err != nil {
			return nil, fmt.Errorf("human review history replay list: %w", err)
		}
		if history != nil && history.Available {
			items := humanReviewReplayItems(history.Items, target)
			if len(items) >= target {
				return items, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("human review history replay has fewer than %d replayable items: %w", target, ctx.Err())
		case <-deadline.C:
			return nil, fmt.Errorf("human review history replay has fewer than %d replayable items", target)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func humanReviewReplayItems(items []rpcapi.PeerRunHistoryEntry, limit int) []rpcapi.PeerRunHistoryEntry {
	if limit <= 0 {
		return nil
	}
	replayable := make([]rpcapi.PeerRunHistoryEntry, 0, limit)
	for _, item := range items {
		if item.Type != rpcapi.PeerRunHistoryEntryTypeAgent || !item.ReplayAvailable || strings.TrimSpace(item.Text) == "" {
			continue
		}
		replayable = append(replayable, item)
	}
	sort.SliceStable(replayable, func(i, j int) bool {
		return replayable[i].CreatedAt.Before(replayable[j].CreatedAt)
	})
	if len(replayable) > limit {
		replayable = replayable[:limit]
	}
	return replayable
}

func newHumanReviewPlayback() (*humanReviewPlayback, error) {
	if !opus.IsRuntimeSupported() {
		return nil, fmt.Errorf("opus runtime is unavailable for human review")
	}
	if !portaudio.NativeRuntimeSupported() {
		return nil, fmt.Errorf("portaudio backend %q is unavailable for human review", portaudio.BackendName())
	}
	format := pcm.L16Mono48K
	channels := 2
	stream, err := portaudio.OpenPlaybackConfig(portaudio.StreamConfig{
		DeviceID:        portaudio.DefaultDeviceID,
		SampleRate:      float64(format.SampleRate()),
		Channels:        channels,
		FramesPerBuffer: uint32(format.SamplesInDuration(humanReviewOpusFrameDuration)),
	})
	if err != nil {
		return nil, fmt.Errorf("open human review playback: %w", err)
	}
	inputDec, err := opus.NewDecoder(format.SampleRate(), format.Channels())
	if err != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("create human review input decoder: %w", err)
	}
	outputDec, err := opus.NewDecoder(format.SampleRate(), format.Channels())
	if err != nil {
		_ = inputDec.Close()
		_ = stream.Close()
		return nil, fmt.Errorf("create human review output decoder: %w", err)
	}
	playback := &humanReviewPlayback{
		format:     format,
		channels:   channels,
		frameSize:  int(format.SamplesInDuration(humanReviewOpusFrameDuration)),
		stream:     stream,
		inputDec:   inputDec,
		outputDec:  outputDec,
		playBuffer: buffer.RingN[humanReviewPCMFrame](humanReviewPlaybackQueueSize),
		playDone:   make(chan error, 1),
	}
	go playback.runPlayback()
	return playback, nil
}

func (p *humanReviewPlayback) PlayBeep(ctx context.Context) error {
	if p == nil {
		return nil
	}
	samples := int(p.format.SamplesInDuration(humanReviewBeepDuration))
	data := make([]int16, samples)
	for i := range data {
		v := math.Sin(2 * math.Pi * 880 * float64(i) / float64(p.format.SampleRate()))
		data[i] = int16(v * 12000)
	}
	if err := p.writePCM(ctx, "human review self-test", int16SamplesToPCM16LE(data)); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	return nil
}

func (p *humanReviewPlayback) TapInputPacket(ctx context.Context, label string, packet []byte) error {
	return p.tapPacket(ctx, label, packet, p.inputDec)
}

func (p *humanReviewPlayback) TapOutputPacket(ctx context.Context, label string, packet []byte) error {
	return p.tapPacket(ctx, label, packet, p.outputDec)
}

func (p *humanReviewPlayback) tapPacket(ctx context.Context, label string, packet []byte, dec opusPacketDecoder) error {
	if p == nil {
		return nil
	}
	frame, err := p.decodePacket(label, packet, dec)
	if err != nil || len(frame.data) == 0 {
		return err
	}
	if err := ctx.Err(); err != nil {
		return ctx.Err()
	}
	if p.playBuffer.Len() >= humanReviewPlaybackQueueSize {
		p.droppedFrames.Add(1)
	}
	if err := p.playBuffer.Add(frame); err != nil {
		if errors.Is(err, io.ErrClosedPipe) {
			return nil
		}
		return fmt.Errorf("%s buffer playback frame: %w", label, err)
	}
	return nil
}

func (p *humanReviewPlayback) decodePacket(label string, packet []byte, dec opusPacketDecoder) (humanReviewPCMFrame, error) {
	if len(packet) == 0 {
		return humanReviewPCMFrame{}, nil
	}
	samples, err := dec.Decode(packet, p.frameSize, false)
	if err != nil {
		return humanReviewPCMFrame{}, fmt.Errorf("%s decode opus: %w", label, err)
	}
	data := int16SamplesToPCM16LE(samples)
	if len(data) == 0 {
		return humanReviewPCMFrame{}, nil
	}
	if p.channels == 2 {
		data = monoPCM16LEToStereo(data)
	}
	return humanReviewPCMFrame{label: label, data: data}, nil
}

func (p *humanReviewPlayback) runPlayback() {
	p.playDone <- p.drainPlayback()
}

func (p *humanReviewPlayback) drainPlayback() error {
	prebufferFrames := int(humanReviewOutputPrebuffer / humanReviewOpusFrameDuration)
	if prebufferFrames < 1 {
		prebufferFrames = 1
	}
	for {
		first, ok, err := p.nextPlaybackFrame()
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		pending, ok := p.collectPlaybackPrebuffer(first, prebufferFrames)
		if !ok {
			return p.flushPlaybackPending(pending)
		}
		if err := p.playbackBurst(pending); err != nil {
			return err
		}
	}
}

func (p *humanReviewPlayback) collectPlaybackPrebuffer(first humanReviewPCMFrame, prebufferFrames int) ([]humanReviewPCMFrame, bool) {
	pending := []humanReviewPCMFrame{first}
	timer := time.NewTimer(humanReviewOutputPrebuffer)
	defer timer.Stop()
	ticker := time.NewTicker(humanReviewOpusFrameDuration)
	defer ticker.Stop()
	for len(pending) < prebufferFrames {
		select {
		case <-ticker.C:
			frame, ok, closed, err := p.tryNextPlaybackFrame()
			if err != nil {
				return pending, false
			}
			if !ok {
				if closed {
					return pending, false
				}
				continue
			}
			pending = append(pending, frame)
		case <-timer.C:
			return pending, true
		}
	}
	return pending, true
}

func (p *humanReviewPlayback) playbackBurst(pending []humanReviewPCMFrame) error {
	for len(pending) > 0 {
		frame := pending[0]
		pending = pending[1:]
		if err := p.writePCM(context.Background(), frame.label, frame.data); err != nil {
			return err
		}
		p.playFrames++
		next, ok, closed, err := p.tryNextPlaybackFrame()
		if err != nil {
			return err
		}
		if ok {
			pending = append(pending, next)
			continue
		}
		if closed {
			return p.flushPlaybackPending(pending)
		}
		select {
		case <-time.After(humanReviewOpusFrameDuration):
			if len(pending) == 0 {
				next, ok, closed, err := p.waitPlaybackContinuation(frame.label, len(frame.data))
				if err != nil {
					return err
				}
				if closed {
					return p.flushPlaybackPending(pending)
				}
				if !ok {
					return nil
				}
				pending = append(pending, next)
			}
		}
	}
	return nil
}

func (p *humanReviewPlayback) waitPlaybackContinuation(label string, frameBytes int) (humanReviewPCMFrame, bool, bool, error) {
	if frameBytes <= 0 {
		return humanReviewPCMFrame{}, false, false, nil
	}
	silence := make([]byte, frameBytes)
	deadline := time.NewTimer(humanReviewOutputIdleKeepalive)
	defer deadline.Stop()
	ticker := time.NewTicker(humanReviewOpusFrameDuration)
	defer ticker.Stop()
	for {
		next, ok, closed, err := p.tryNextPlaybackFrame()
		if err != nil {
			return humanReviewPCMFrame{}, false, false, err
		}
		if ok {
			return next, true, false, nil
		}
		if closed {
			return humanReviewPCMFrame{}, false, true, nil
		}
		select {
		case <-ticker.C:
			p.reportOutputBufferUnderrun()
			if err := p.writePCM(context.Background(), label+" silence", silence); err != nil {
				return humanReviewPCMFrame{}, false, false, err
			}
		case <-deadline.C:
			return humanReviewPCMFrame{}, false, false, nil
		}
	}
}

func (p *humanReviewPlayback) nextPlaybackFrame() (humanReviewPCMFrame, bool, error) {
	frame, err := p.playBuffer.Next()
	if err == nil {
		return frame, true, nil
	}
	if errors.Is(err, buffer.ErrIteratorDone) || errors.Is(err, io.ErrClosedPipe) {
		return humanReviewPCMFrame{}, false, nil
	}
	return humanReviewPCMFrame{}, false, err
}

func (p *humanReviewPlayback) tryNextPlaybackFrame() (humanReviewPCMFrame, bool, bool, error) {
	if p.playBuffer.Len() == 0 {
		if p.playBuffer.Error() != nil || p.isPlaybackClosed() {
			return humanReviewPCMFrame{}, false, true, nil
		}
		return humanReviewPCMFrame{}, false, false, nil
	}
	frame, ok, err := p.nextPlaybackFrame()
	if err != nil {
		return humanReviewPCMFrame{}, false, false, err
	}
	if !ok {
		return humanReviewPCMFrame{}, false, true, nil
	}
	return frame, true, false, nil
}

func (p *humanReviewPlayback) isPlaybackClosed() bool {
	p.playStateMu.Lock()
	defer p.playStateMu.Unlock()
	return p.playClosed
}

func (p *humanReviewPlayback) markPlaybackClosed() {
	p.playStateMu.Lock()
	defer p.playStateMu.Unlock()
	p.playClosed = true
}

func (p *humanReviewPlayback) reportOutputBufferUnderrun() {
	p.bufferUnderruns++
	if p.bufferUnderruns <= 3 {
		fmt.Printf("human_review output_buffer_underrun=%d\n", p.bufferUnderruns)
	} else if p.bufferUnderruns == 4 {
		fmt.Printf("human_review output_buffer_underrun=%d suppressed=true\n", p.bufferUnderruns)
	}
}

func (p *humanReviewPlayback) flushPlaybackPending(pending []humanReviewPCMFrame) error {
	for _, frame := range pending {
		if err := p.writePCM(context.Background(), frame.label, frame.data); err != nil {
			return err
		}
		p.playFrames++
	}
	return nil
}

func (p *humanReviewPlayback) writePCM(ctx context.Context, label string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		n, err := p.stream.Write(data)
		if err != nil {
			if isPortAudioOutputUnderflow(err) && attempt < humanReviewOutputUnderflowRetries {
				p.reportPortAudioUnderflow(err, true)
				time.Sleep(humanReviewOpusFrameDuration)
				if err := ctx.Err(); err != nil {
					return err
				}
				continue
			}
			if isPortAudioOutputUnderflow(err) {
				p.reportPortAudioUnderflow(err, false)
				return nil
			}
			return fmt.Errorf("%s write playback: %w", label, err)
		}
		if n != len(data) {
			return fmt.Errorf("%s write playback: short write %d/%d", label, n, len(data))
		}
		return nil
	}
}

func (p *humanReviewPlayback) reportPortAudioUnderflow(err error, retry bool) {
	p.underflowWarnings++
	if p.underflowWarnings <= 3 {
		fmt.Printf("human_review warning=%q retry=%t\n", err.Error(), retry)
	} else if p.underflowWarnings == 4 {
		fmt.Printf("human_review warning=%q retry=%t suppressed=true\n", err.Error(), retry)
	}
}

func isPortAudioOutputUnderflow(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Output underflowed")
}

func (p *humanReviewPlayback) Close() error {
	if p == nil {
		return nil
	}
	p.closeOnce.Do(func() {
		p.closePlayOnce.Do(func() {
			p.markPlaybackClosed()
			_ = p.playBuffer.CloseWrite()
		})
		err0 := <-p.playDone
		fmt.Printf("human_review playback_stats frames=%d buffer_underruns=%d portaudio_underflows=%d dropped_frames=%d\n", p.playFrames, p.bufferUnderruns, p.underflowWarnings, p.droppedFrames.Load())
		p.mu.Lock()
		defer p.mu.Unlock()
		err1 := p.inputDec.Close()
		err2 := p.outputDec.Close()
		err3 := p.stream.Close()
		switch {
		case err0 != nil:
			p.closeErr = err0
		case err1 != nil:
			p.closeErr = err1
		case err2 != nil:
			p.closeErr = err2
		default:
			p.closeErr = err3
		}
	})
	return p.closeErr
}

func int16SamplesToPCM16LE(samples []int16) []byte {
	out := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}
	return out
}

func monoPCM16LEToStereo(data []byte) []byte {
	out := make([]byte, len(data)*2)
	for i := 0; i+1 < len(data); i += 2 {
		sample := binary.LittleEndian.Uint16(data[i:])
		j := i * 2
		binary.LittleEndian.PutUint16(out[j:], sample)
		binary.LittleEndian.PutUint16(out[j+2:], sample)
	}
	return out
}
