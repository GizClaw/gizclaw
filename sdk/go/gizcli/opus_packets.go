package gizcli

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func (c *Client) subscribePeerAudio(size int) (<-chan *genx.Blob, func()) {
	// A single bounded jitter batch can contain 64 real frames and their gaps.
	// Keep that codec burst separate from the caller's output buffer size.
	channel := make(chan *genx.Blob, max(129, size))
	c.packetMu.Lock()
	if c.audioSubscribers == nil {
		c.audioSubscribers = make(map[chan *genx.Blob]struct{})
	}
	c.audioSubscribers[channel] = struct{}{}
	c.packetMu.Unlock()
	var once sync.Once
	return channel, func() {
		once.Do(func() {
			c.packetMu.Lock()
			delete(c.audioSubscribers, channel)
			c.packetMu.Unlock()
		})
	}
}

func (c *Client) dispatchPeerAudio(blob *genx.Blob) {
	c.packetMu.RLock()
	targets := make([]chan *genx.Blob, 0, len(c.audioSubscribers))
	for channel := range c.audioSubscribers {
		targets = append(targets, channel)
	}
	c.packetMu.RUnlock()
	for _, channel := range targets {
		// Advisory only: avoid copying a frame for an already-full subscriber.
		// The select still handles a concurrent producer filling the last slot.
		if len(channel) == cap(channel) {
			continue
		}
		select {
		case channel <- (&genx.MessageChunk{Part: blob}).Clone().Part.(*genx.Blob):
		default:
			// Slow business consumers drop locally without stalling other
			// subscribers or the connection's reliable control traffic.
		}
	}
}

func (c *Client) flushPeerAudio(ctx context.Context) error {
	reader := c.mediaReader.Load()
	if reader == nil {
		return nil
	}
	err := reader.Flush(ctx)
	if errors.Is(err, io.ErrClosedPipe) {
		return nil
	}
	return err
}
