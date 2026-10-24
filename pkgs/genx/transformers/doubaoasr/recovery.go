package doubaoasr

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
)

const (
	asrRecoveryAudioLimit = 15 * time.Second
	asrRecoveryByteLimit  = 2 << 20
	asrRecoveryTimeout    = 15 * time.Second
	asrServerError        = 55000000
	asrCheckpointLimit    = 1024
)

var errASRRecoveryTimeout = errors.New("doubao asr: recovery timeout")
var errASRIncompleteResult = errors.New("doubao asr: transcript ended before a definite result")

type asrSend struct {
	data []byte
	last bool
	done chan error
}

type asrResult struct {
	value      *doubaospeech.ASRV2Result
	err        error
	generation uint64
}

// recoveringSession owns provider generations on one worker. Only that worker
// mutates the replay window, sends audio, or publishes results. Closing a
// generation cancels I/O and joins its reader before opening the replacement.
type recoveringSession struct {
	ctx        context.Context
	cancel     context.CancelFunc
	sends      chan asrSend
	results    chan asrResult
	done       chan struct{}
	usage      atomic.Int64
	generation atomic.Uint64
	recovering atomic.Bool
	onRetry    func()
	err        error // published by closing done

	cfg             doubaoASRSessionConfig
	open            func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error)
	continuous      bool
	timeout         time.Duration
	route           *doubaoASRRouteState
	requestID       string
	started         time.Time
	segment         int
	retry           int
	retrySegment    int
	retryStarted    time.Time
	retrying        bool
	recovered       bool
	recoveredResult *doubaospeech.ASRV2Result
	unsafe          string
	audio           []byte
	base            int64
	sent            int64
	providerBase    int64
	last            bool
	seenUtterances  map[[32]byte]struct{}
	sawInterim      bool
	lastMetadata    *doubaospeech.ASRV2Result
}

func newRecoveringSession(ctx context.Context, cfg doubaoASRSessionConfig, route *doubaoASRRouteState, continuous bool, timeout time.Duration, open func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error), activate ...func(*recoveringSession)) (*recoveringSession, error) {
	id, err := gizlog.NewID()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &recoveringSession{ctx: ctx, cancel: cancel, cfg: cfg, route: route,
		continuous: continuous, timeout: timeout, open: open, requestID: id,
		started: time.Now(), segment: 1, sends: make(chan asrSend), results: make(chan asrResult), done: make(chan struct{})}
	ready := make(chan error, 1)
	if len(activate) > 0 {
		activate[0](s)
	}
	go s.run(ready)
	if err := <-ready; err != nil {
		cancel()
		<-s.done
		return nil, err
	}
	return s, nil
}

func (s *recoveringSession) SendAudio(ctx context.Context, data []byte, last bool) error {
	request := asrSend{data: data, last: last, done: make(chan error, 1)}
	select {
	case s.sends <- request:
	case <-s.done:
		return s.sendError()
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-request.done:
		return err
	case <-s.done:
		// The worker publishes an accepted send before closing done. Both
		// may be ready when provider finalization completes immediately.
		select {
		case err := <-request.done:
			return err
		default:
			return s.sendError()
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *recoveringSession) sendError() error {
	if s.err != nil {
		return s.err
	}
	return io.ErrClosedPipe
}

func (s *recoveringSession) Recv() iter.Seq2[*doubaospeech.ASRV2Result, error] {
	return func(yield func(*doubaospeech.ASRV2Result, error) bool) {
		for result := range s.results {
			s.generation.Store(result.generation)
			if !yield(result.value, result.err) {
				return
			}
		}
		<-s.done
		if s.err != nil {
			yield(nil, s.err)
		}
	}
}

func (s *recoveringSession) AudioDuration() time.Duration { return time.Duration(s.usage.Load()) }

func (s *recoveringSession) resultGeneration() uint64 { return s.generation.Load() }

func (s *recoveringSession) Close() error {
	s.cancel()
	<-s.done
	return nil
}

func (s *recoveringSession) run(ready chan<- error) {
	defer close(s.done)
	defer s.recovering.Store(false)
	defer close(s.results)
	var provider doubaoASRSession
	var events chan asrResult
	var readerDone chan struct{}
	var attemptCtx context.Context
	var cancelAttempt context.CancelCauseFunc
	var stopClose func() bool
	var closeDone chan struct{}
	var timer *time.Timer
	var sending chan error
	var pending *asrSend
	var generation uint64
	ended := false
	stop := func() {
		if provider == nil {
			return
		}
		cancelAttempt(context.Canceled)
		if stopClose() {
			_ = provider.Close()
		} else {
			<-closeDone
		}
		<-readerDone
		if sending != nil {
			<-sending
			sending = nil
		}
		s.usage.Add(int64(provider.AudioDuration()))
		provider = nil
	}
	defer stop()
	defer func() {
		if pending != nil {
			pending.done <- s.err
		}
	}()
	defer func() {
		if timer != nil {
			timer.Stop()
		}
		if s.retrying {
			s.log("retry failure", s.err, s.lastMetadata, "terminated")
		}
	}()
	start := func() error {
		s.lastMetadata = nil
		attemptCtx, cancelAttempt = context.WithCancelCause(s.ctx)
		readCtx, stopAttempt := attemptCtx, cancelAttempt
		if s.retrying {
			timer = time.AfterFunc(s.timeout, func() { stopAttempt(errASRRecoveryTimeout) })
		}
		var err error
		provider, err = s.open(attemptCtx, s.cfg)
		if err == nil && attemptCtx.Err() != nil {
			err = context.Cause(attemptCtx)
		}
		if err != nil {
			cancelAttempt(err)
			if provider != nil {
				_ = provider.Close()
				provider = nil
			}
			return err
		}
		current := provider
		generation++
		closeDone = make(chan struct{})
		closed := closeDone
		stopClose = context.AfterFunc(attemptCtx, func() { defer close(closed); _ = current.Close() })
		events = make(chan asrResult)
		readerDone = make(chan struct{})
		currentEvents, done := events, readerDone
		go func() {
			defer close(done)
			defer close(currentEvents)
			for value, err := range current.Recv() {
				select {
				case currentEvents <- asrResult{value: value, err: err}:
				case <-readCtx.Done():
					return
				}
			}
		}()
		return nil
	}
	recover := func(cause error) error {
		if s.ctx.Err() != nil || s.route.interruption() != "" {
			return context.Canceled
		}
		if errors.Is(context.Cause(attemptCtx), errASRRecoveryTimeout) {
			return errASRRecoveryTimeout
		}
		if s.continuous && isRecoverableRealtimeSessionError(cause) {
			return cause
		}
		s.log("attempt failed", cause, nil, s.reason())
		apiErr, ok := doubaospeech.AsError(cause)
		if !ok || apiErr.Code != asrServerError || s.reason() != "" {
			return cause
		}
		// Log the original error before changing generation or replaying audio.
		s.recovering.Store(true)
		if s.onRetry != nil {
			s.onRetry()
		}
		if s.ctx.Err() != nil {
			return context.Canceled
		}
		s.retry = 1
		s.retrySegment = s.segment
		s.retrying = true
		s.recovered = false
		s.recoveredResult = nil
		s.retryStarted = time.Now()
		s.log("retry start", cause, nil, "")
		stop()
		s.providerBase = s.base
		if err := start(); err != nil {
			return err
		}
		audio, last, current, sendCtx := slices.Clone(s.audio), s.last, provider, attemptCtx
		packetSize := max(1, s.cfg.sampleRate*s.cfg.channels*(s.cfg.bits/8)/10)
		sending = make(chan error, 1)
		completed := sending
		go func() {
			for packet := range splitDoubaoASRAudio(audio, packetSize) {
				if err := current.SendAudio(sendCtx, packet, false); err != nil {
					completed <- err
					return
				}
			}
			var err error
			if last {
				err = current.SendAudio(sendCtx, nil, true)
			}
			completed <- err
		}()
		return nil
	}
	if err := start(); err != nil {
		s.err = recover(err)
		if s.err != nil {
			ready <- s.err
			return
		}
	}
	ready <- nil
	for {
		if s.ctx.Err() != nil {
			s.err = s.ctx.Err()
			return
		}
		if s.retrying && s.recovered && sending == nil {
			if !timer.Stop() {
				s.err = errASRRecoveryTimeout
				return
			}
			s.log("retry success", nil, s.recoveredResult, "completed")
			s.retrying = false
			s.recovering.Store(false)
			if s.unsafe == "" {
				s.retry = 0
				s.retryStarted = time.Time{}
			}
		}
		if ended && sending == nil {
			return
		}
		requests := s.sends
		if sending != nil {
			requests = nil
		}
		select {
		case <-s.ctx.Done():
			s.err = s.ctx.Err()
			return
		case <-attemptCtx.Done():
			s.err = context.Cause(attemptCtx)
			return
		case request := <-requests:
			s.retain(request.data)
			s.last = request.last
			pending = &request
			sending = make(chan error, 1)
			completed, current, sendCtx := sending, provider, attemptCtx
			go func() { completed <- current.SendAudio(sendCtx, request.data, request.last) }()
		case err := <-sending:
			sending = nil
			if err != nil {
				err = recover(err)
			}
			if sending != nil && err == nil {
				continue
			}
			if pending != nil {
				pending.done <- err
				pending = nil
			}
			if err != nil {
				s.err = err
				return
			}
		case result, ok := <-events:
			if !ok {
				if s.retrying && !s.recovered && s.sawInterim {
					s.err = errASRIncompleteResult
					return
				}
				ended = true
				s.recovered = true
				events = nil
				continue
			}
			if result.err != nil {
				if s.err = recover(result.err); s.err != nil {
					return
				}
				continue
			}
			if result.value == nil {
				continue
			}
			s.lastMetadata = &doubaospeech.ASRV2Result{ReqID: result.value.ReqID, ConnectID: result.value.ConnectID, TraceID: result.value.TraceID, LogID: result.value.LogID}
			if !result.value.IsFinal && strings.TrimSpace(result.value.Text) != "" {
				s.sawInterim = true
			}
			for _, utterance := range result.value.Utterances {
				if !utterance.Definite && strings.TrimSpace(utterance.Text) != "" {
					s.sawInterim = true
				}
			}
			value := s.checkpoint(result.value)
			select {
			case s.results <- asrResult{value: value, generation: generation}:
			case <-s.ctx.Done():
				s.err = s.ctx.Err()
				return
			}
		}
	}
}

func (s *recoveringSession) reason() string {
	if s.retry != 0 {
		return "retry_exhausted"
	}
	if !s.cfg.isPCM() {
		return "unsupported_format"
	}
	return s.unsafe
}

func (s *recoveringSession) retain(data []byte) {
	s.sent += int64(len(data))
	if s.unsafe != "" {
		return
	}
	if len(s.audio)+len(data) > asrRecoveryByteLimit || audioDuration(s.audio, s.cfg)+audioDuration(data, s.cfg) > asrRecoveryAudioLimit {
		s.unsafe = "audio_limit"
		s.audio = nil
		return
	}
	s.audio = append(s.audio, data...)
}

func (s *recoveringSession) checkpoint(result *doubaospeech.ASRV2Result) *doubaospeech.ASRV2Result {
	copy := *result
	copy.Utterances = slices.Clone(result.Utterances)
	bps := int64(s.cfg.sampleRate * s.cfg.channels * (s.cfg.bits / 8))
	if bps <= 0 {
		s.unsafe = "invalid_format"
		return &copy
	}
	offsetMS := int(s.providerBase * 1000 / bps)
	end := s.base
	definite := false
	commits := 0
	valid := true
	for i := range copy.Utterances {
		utt := &copy.Utterances[i]
		localStart, localEnd := utt.StartTime, utt.EndTime
		utt.StartTime += offsetMS
		utt.EndTime += offsetMS
		if utt.Definite && strings.TrimSpace(utt.Text) != "" {
			// Match receiveResults' exact timed-text identity. Cumulative
			// provider frames may contain only previously committed utterances.
			key := sha256.Sum256(fmt.Appendf(nil, "%d:%d:%s", utt.StartTime, utt.EndTime, utt.Text))
			if _, exists := s.seenUtterances[key]; exists {
				continue
			}
			if len(s.seenUtterances) >= asrCheckpointLimit {
				s.unsafe = "checkpoint_limit"
			} else {
				if s.seenUtterances == nil {
					s.seenUtterances = make(map[[32]byte]struct{})
				}
				s.seenUtterances[key] = struct{}{}
			}
			if localStart < 0 || localEnd <= localStart || int64(localEnd) > (s.sent-s.providerBase)*1000/bps || int64(utt.StartTime)*bps/1000 < s.base {
				valid = false
			}
			definite = true
			if int64(utt.EndTime)*bps/1000 > s.base {
				commits++
			}
			end = max(end, int64(utt.EndTime)*bps/1000)
		}
	}
	if !definite && (!result.IsFinal || strings.TrimSpace(result.Text) == "") {
		return &copy
	}
	s.sawInterim = false
	if s.retrying {
		s.recovered = true
		s.recoveredResult = &doubaospeech.ASRV2Result{ReqID: result.ReqID, ConnectID: result.ConnectID, TraceID: result.TraceID, LogID: result.LogID}
	}
	// PTT text can already be visible without a turn EOS. A final result
	// lacking timed utterances has no provable replay boundary either.
	if !s.continuous || !definite || !valid || end <= s.base || end > s.sent {
		s.unsafe = "committed_output"
		return &copy
	}
	if s.unsafe == "" {
		s.audio = slices.Clone(s.audio[end-s.base:])
	}
	s.base = end
	s.segment += commits
	if !s.retrying {
		s.retry = 0
		s.retryStarted = time.Time{}
	}
	return &copy
}

func (s *recoveringSession) log(event string, err error, result *doubaospeech.ASRV2Result, reason string) {
	stream, _ := s.route.current()
	if result == nil {
		result = s.lastMetadata
	}
	segment := s.segment
	if s.retrying {
		segment = s.retrySegment
	}
	attrs := []slog.Attr{slog.String("asr_request_id", s.requestID), slog.String("input_stream_id", stream),
		slog.String("transcript_stream_id", asrSegmentStreamID(stream, segment)), slog.Int("segment_index", segment),
		slog.Int("retry", s.retry), slog.String("result", event), slog.String("reason", reason),
		slog.Int64("duration_ms", time.Since(s.started).Milliseconds())}
	if !s.retryStarted.IsZero() {
		attrs = append(attrs, slog.Int64("retry_duration_ms", time.Since(s.retryStarted).Milliseconds()))
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	if apiErr, ok := doubaospeech.AsError(err); ok {
		attrs = append(attrs, slog.Int("error_code", apiErr.Code), slog.String("upstream_reqid", apiErr.ReqID),
			slog.String("upstream_connect_id", apiErr.ConnectID), slog.String("upstream_trace_id", apiErr.TraceID), slog.String("upstream_log_id", apiErr.LogID))
		if result != nil {
			if apiErr.ReqID == "" && result.ReqID != "" {
				attrs = append(attrs, slog.String("upstream_last_reqid", result.ReqID))
			}
			if apiErr.ConnectID == "" && result.ConnectID != "" {
				attrs = append(attrs, slog.String("upstream_last_connect_id", result.ConnectID))
			}
			if apiErr.TraceID == "" && result.TraceID != "" {
				attrs = append(attrs, slog.String("upstream_last_trace_id", result.TraceID))
			}
			if apiErr.LogID == "" && result.LogID != "" {
				attrs = append(attrs, slog.String("upstream_last_log_id", result.LogID))
			}
		}
	} else {
		if err == nil {
			attrs = append(attrs, slog.Int("error_code", 0))
		} else {
			code := "ASR_STREAM_ERROR"
			switch {
			case errors.Is(err, errASRRecoveryTimeout):
				code = "ASR_RECOVERY_TIMEOUT"
			case errors.Is(err, errASRIncompleteResult):
				code = "ASR_INCOMPLETE_RESULT"
			case errors.Is(err, context.Canceled):
				code = "CANCELED"
			case errors.Is(err, context.DeadlineExceeded):
				code = "DEADLINE_EXCEEDED"
			}
			attrs = append(attrs, slog.String("error_code", code))
		}
		if result != nil {
			attrs = append(attrs, slog.String("upstream_reqid", result.ReqID), slog.String("upstream_connect_id", result.ConnectID),
				slog.String("upstream_trace_id", result.TraceID), slog.String("upstream_log_id", result.LogID))
		}
	}
	slog.LogAttrs(s.ctx, slog.LevelWarn, "doubao asr: "+event, attrs...)
}
