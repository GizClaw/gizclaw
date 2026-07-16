package genx

import (
	"container/heap"
	"context"
	"io"
	"strings"
	"sync"
	"time"
)

const (
	defaultRealtimeStreamDelay       = 80 * time.Millisecond
	defaultRealtimeStreamMaxDuration = 2 * time.Minute
)

// RealtimeStreamOption configures a RealtimeStream.
type RealtimeStreamOption func(*realtimeStreamConfig)

type realtimeStreamConfig struct {
	delay       time.Duration
	maxDuration time.Duration
	now         func() time.Time
}

// WithRealtimeStreamDelay configures how long chunks stay reorderable before readout.
func WithRealtimeStreamDelay(delay time.Duration) RealtimeStreamOption {
	return func(cfg *realtimeStreamConfig) {
		cfg.delay = delay
	}
}

// WithRealtimeStreamMaxDuration configures the retained timestamp window.
func WithRealtimeStreamMaxDuration(maxDuration time.Duration) RealtimeStreamOption {
	return func(cfg *realtimeStreamConfig) {
		cfg.maxDuration = maxDuration
	}
}

// WithRealtimeStreamNow configures the clock used for timestamp defaults and read scheduling.
func WithRealtimeStreamNow(now func() time.Time) RealtimeStreamOption {
	return func(cfg *realtimeStreamConfig) {
		cfg.now = now
	}
}

// RealtimeStream is a push-writable, timestamp-ordered genx.Stream.
type RealtimeStream struct {
	mu sync.Mutex

	closed bool
	err    error
	notify chan struct{}

	delay       time.Duration
	maxDuration time.Duration
	now         func() time.Time

	heap        realtimeStreamHeap
	seq         uint64
	lastEmitted int64
	maxSeen     int64
	activeID    string
	activeLabel string
}

var _ Stream = (*RealtimeStream)(nil)

// NewRealtimeStream creates a timestamp ordered realtime stream.
func NewRealtimeStream(opts ...RealtimeStreamOption) *RealtimeStream {
	cfg := realtimeStreamConfig{
		delay:       defaultRealtimeStreamDelay,
		maxDuration: defaultRealtimeStreamMaxDuration,
		now:         func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.delay < 0 {
		cfg.delay = 0
	}
	if cfg.maxDuration <= 0 {
		cfg.maxDuration = defaultRealtimeStreamMaxDuration
	}
	if cfg.now == nil {
		cfg.now = func() time.Time { return time.Now().UTC() }
	}
	s := &RealtimeStream{
		notify:      make(chan struct{}),
		delay:       cfg.delay,
		maxDuration: cfg.maxDuration,
		now:         cfg.now,
		lastEmitted: -1,
		maxSeen:     -1,
	}
	heap.Init(&s.heap)
	return s
}

// Push writes a chunk into the realtime stream.
func (s *RealtimeStream) Push(ctx context.Context, chunk *MessageChunk) error {
	if s == nil {
		return io.ErrClosedPipe
	}
	if chunk == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return io.ErrClosedPipe
	}
	if chunk.IsBeginOfStream() {
		s.dropSupersededStreamLocked(chunk)
		if chunk.Ctrl != nil {
			s.activeID = chunk.Ctrl.StreamID
			s.activeLabel = chunk.Ctrl.Label
		}
	} else if s.isSupersededActiveRouteChunkLocked(chunk) {
		return nil
	}
	ts := s.chunkTimestampLocked(chunk)
	if s.lastEmitted >= 0 && ts < s.lastEmitted {
		if !s.shouldClampLateTimestampLocked(chunk) {
			return nil
		}
		ts = s.lastEmitted + 1
		if chunk.Ctrl == nil {
			chunk.Ctrl = &StreamCtrl{}
		}
		chunk.Ctrl.Timestamp = ts
	}
	s.seq++
	heap.Push(&s.heap, realtimeStreamItem{
		chunk:     chunk,
		seq:       s.seq,
		timestamp: ts,
	})
	if ts > s.maxSeen {
		s.maxSeen = ts
	}
	s.evictOverflowLocked()
	s.signalLocked()
	return nil
}

// Next returns the next timestamp-ready chunk.
func (s *RealtimeStream) Next() (*MessageChunk, error) {
	if s == nil {
		return nil, io.ErrClosedPipe
	}
	for {
		s.mu.Lock()
		s.dropLateLocked()
		if s.heap.Len() > 0 {
			item := s.heap[0]
			now := s.now()
			readyAt := time.UnixMilli(item.timestamp).Add(s.delay)
			if s.closed || !readyAt.After(now) {
				heap.Pop(&s.heap)
				s.lastEmitted = item.timestamp
				s.mu.Unlock()
				return item.chunk, nil
			}
			notify := s.notify
			wait := readyAt.Sub(now)
			s.mu.Unlock()
			if s.waitNext(notify, wait) {
				continue
			}
			continue
		}
		if s.closed {
			err := s.err
			s.mu.Unlock()
			if err == nil {
				err = io.EOF
			}
			return nil, err
		}
		notify := s.notify
		s.mu.Unlock()
		<-notify
	}
}

// Close closes the stream after draining buffered chunks.
func (s *RealtimeStream) Close() error {
	return s.CloseWithError(io.EOF)
}

// CloseWithError closes the stream after draining buffered chunks, then returns err from Next.
func (s *RealtimeStream) CloseWithError(err error) error {
	if s == nil {
		return nil
	}
	if err == nil {
		err = io.ErrClosedPipe
	}
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.err = err
		s.signalLocked()
	}
	s.mu.Unlock()
	return nil
}

func (s *RealtimeStream) waitNext(notify <-chan struct{}, wait time.Duration) bool {
	if wait <= 0 {
		return true
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-notify:
		return true
	case <-timer.C:
		return false
	}
}

func (s *RealtimeStream) chunkTimestampLocked(chunk *MessageChunk) int64 {
	if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.Timestamp > 0 {
		return chunk.Ctrl.Timestamp
	}
	ts := s.now().UnixMilli()
	if s.lastEmitted >= 0 && ts <= s.lastEmitted {
		ts = s.lastEmitted + 1
	}
	if chunk != nil && chunk.IsEndOfStream() && s.maxSeen >= 0 && ts <= s.maxSeen {
		ts = s.maxSeen + 1
	}
	if chunk != nil {
		if chunk.Ctrl == nil {
			chunk.Ctrl = &StreamCtrl{}
		}
		chunk.Ctrl.Timestamp = ts
	}
	return ts
}

func (s *RealtimeStream) evictOverflowLocked() {
	if s.maxSeen < 0 || s.maxDuration <= 0 {
		return
	}
	threshold := s.maxSeen - s.maxDuration.Milliseconds()
	for s.heap.Len() > 0 && s.heap[0].timestamp < threshold {
		heap.Pop(&s.heap)
	}
}

func (s *RealtimeStream) dropLateLocked() {
	for s.heap.Len() > 0 && s.lastEmitted >= 0 && s.heap[0].timestamp < s.lastEmitted {
		heap.Pop(&s.heap)
	}
}

func (s *RealtimeStream) dropSupersededStreamLocked(next *MessageChunk) {
	nextID, nextLabel := realtimeChunkRoute(next)
	if nextID == "" || s.heap.Len() == 0 {
		return
	}
	items := s.heap[:0]
	for _, item := range s.heap {
		id, label := realtimeChunkRoute(item.chunk)
		if id != "" && id != nextID && realtimeRouteMatches(nextLabel, label) && isRealtimeAudioChunk(item.chunk) {
			continue
		}
		items = append(items, item)
	}
	s.heap = items
	heap.Init(&s.heap)
	s.recomputeMaxSeenLocked()
}

func (s *RealtimeStream) shouldClampLateTimestampLocked(chunk *MessageChunk) bool {
	if chunk == nil || chunk.Ctrl == nil {
		return false
	}
	return s.activeID != "" && chunk.Ctrl.StreamID == s.activeID
}

func (s *RealtimeStream) isSupersededActiveRouteChunkLocked(chunk *MessageChunk) bool {
	if chunk == nil || chunk.Ctrl == nil || s.activeID == "" || chunk.Ctrl.StreamID == "" || chunk.Ctrl.StreamID == s.activeID {
		return false
	}
	return realtimeRouteMatches(s.activeLabel, chunk.Ctrl.Label) && isRealtimeAudioChunk(chunk)
}

func (s *RealtimeStream) recomputeMaxSeenLocked() {
	maxSeen := s.lastEmitted
	for _, item := range s.heap {
		if item.timestamp > maxSeen {
			maxSeen = item.timestamp
		}
	}
	s.maxSeen = maxSeen
}

func realtimeChunkRoute(chunk *MessageChunk) (streamID, label string) {
	if chunk == nil || chunk.Ctrl == nil {
		return "", ""
	}
	return chunk.Ctrl.StreamID, chunk.Ctrl.Label
}

func realtimeRouteMatches(left, right string) bool {
	return left == "" || right == "" || left == right
}

func isRealtimeAudioChunk(chunk *MessageChunk) bool {
	if chunk == nil {
		return false
	}
	if chunk.Part == nil {
		// A control-only EOS closes the whole route, including audio.
		return chunk.IsEndOfStream()
	}
	mimeType, ok := chunk.MIMEType()
	if !ok {
		return false
	}
	if idx := strings.IndexByte(mimeType, ';'); idx >= 0 {
		mimeType = mimeType[:idx]
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "audio/") {
		return true
	}
	return false
}

func (s *RealtimeStream) signalLocked() {
	close(s.notify)
	s.notify = make(chan struct{})
}

type realtimeStreamItem struct {
	chunk     *MessageChunk
	seq       uint64
	timestamp int64
}

type realtimeStreamHeap []realtimeStreamItem

func (h realtimeStreamHeap) Len() int {
	return len(h)
}

func (h realtimeStreamHeap) Less(i, j int) bool {
	left := h[i]
	right := h[j]
	if left.timestamp != right.timestamp {
		return left.timestamp < right.timestamp
	}
	if left.chunk.IsBeginOfStream() != right.chunk.IsBeginOfStream() {
		return left.chunk.IsBeginOfStream()
	}
	if left.chunk.IsEndOfStream() != right.chunk.IsEndOfStream() {
		return !left.chunk.IsEndOfStream()
	}
	return left.seq < right.seq
}

func (h realtimeStreamHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *realtimeStreamHeap) Push(x any) {
	*h = append(*h, x.(realtimeStreamItem))
}

func (h *realtimeStreamHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}
